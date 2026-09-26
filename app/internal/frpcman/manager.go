// Package frpcman 负责管理 frpc 子进程的生命周期。
//
// 核心职责：
//   - 以**隐藏窗口**方式启动官方的 frpc.exe（不弹黑窗口）
//   - 捕获它的 stdout/stderr，解析成结构化日志并翻译成中文
//   - 从日志推断"是否已连上服务端"（frp 的 HTTP API 不提供这个状态）
//   - 异常退出时按退避策略自动重启
//
// 注意：本包不解析 frpc.toml，也不调用 admin API —— 那些分别由
// cfgfile 和 frpcapi 包负责，保持单一职责。
package frpcman

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// State 是与服务端的连接状态。
//
// ⚠ 这里的状态是从 frpc 的日志推断出来的，不是官方 API 提供的。
// 原因：frpc 的 admin API 没有"是否已连上服务端"这个接口
// （/api/status 在未连接时返回空，无法与"没有代理"区分）。
type State string

const (
	StateStopped   State = "stopped"   // 进程未运行
	StateStarting  State = "starting"  // 进程已启动，尚未连上
	StateConnected State = "connected" // 已连上服务端
	StateFailed    State = "failed"    // 连接失败（含原因）
)

const (
	maxLogEntries   = 2000 // 日志环形缓冲容量
	maxRestartTries = 5    // 连续重启次数上限
	restartDelay    = 3 * time.Second
)

// Manager 管理一个 frpc 子进程。
type Manager struct {
	mu sync.RWMutex

	exePath string
	cfgPath string

	cmd     *exec.Cmd
	running bool
	state   State
	lastErr string

	logs   []LogEntry
	logSeq int64

	// 自动重启
	restartCount int
	stopping     bool
	autoRestart  bool

	// 回调（由上层注入，用于把状态变化推给界面）
	onState func(State, string)
}

// New 创建管理器。exePath 是 frpc.exe 的绝对路径，cfgPath 是 frpc.toml 的绝对路径。
func New(exePath, cfgPath string) *Manager {
	return &Manager{
		exePath:     exePath,
		cfgPath:     cfgPath,
		state:       StateStopped,
		logs:        make([]LogEntry, 0, 256),
		autoRestart: true, // 默认开启：用户误关或崩溃时自动恢复
	}
}

// SetAutoRestart 开关"意外退出时自动重启"。
func (m *Manager) SetAutoRestart(on bool) {
	m.mu.Lock()
	m.autoRestart = on
	m.mu.Unlock()
}

// AutoRestart 返回当前是否开启自动重启。
func (m *Manager) AutoRestart() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.autoRestart
}

// OnStateChange 注册状态变化回调。
func (m *Manager) OnStateChange(fn func(State, string)) {
	m.mu.Lock()
	m.onState = fn
	m.mu.Unlock()
}

// IsRunning 返回进程是否在运行。
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// State 返回当前连接状态与错误原因。
func (m *Manager) State() (State, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state, m.lastErr
}

// setState 更新状态并在变化时触发回调。调用方不得持有锁。
func (m *Manager) setState(s State, errMsg string) {
	m.mu.Lock()
	changed := m.state != s || m.lastErr != errMsg
	m.state = s
	m.lastErr = errMsg
	fn := m.onState
	m.mu.Unlock()

	if changed && fn != nil {
		fn(s, errMsg)
	}
}

// Logs 返回 seq 大于 afterSeq 的日志条目。
// 界面用递增的 seq 做增量拉取，避免重复渲染。
func (m *Manager) Logs(afterSeq int64) []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]LogEntry, 0, 64)
	for _, e := range m.logs {
		if e.Seq > afterSeq {
			out = append(out, e)
		}
	}
	return out
}

// LogsTail 返回最后 n 条日志（用于界面首次加载）。
func (m *Manager) LogsTail(n int) []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if n <= 0 || n > len(m.logs) {
		n = len(m.logs)
	}
	out := make([]LogEntry, n)
	copy(out, m.logs[len(m.logs)-n:])
	return out
}

// ClearLogs 清空日志缓冲（只清界面显示，不动磁盘上的 frpc.log）。
func (m *Manager) ClearLogs() {
	m.mu.Lock()
	m.logs = m.logs[:0]
	m.mu.Unlock()
}

// appendLog 解析并追加一条日志行。
func (m *Manager) appendLog(line string) {
	m.mu.Lock()
	m.logSeq++
	entry, ok := ParseLine(line, m.logSeq)
	if !ok {
		m.mu.Unlock()
		return
	}
	m.logs = append(m.logs, entry)
	if len(m.logs) > maxLogEntries {
		// 丢弃最旧的四分之一，避免频繁切片
		drop := len(m.logs) - maxLogEntries + maxLogEntries/4
		m.logs = append(m.logs[:0], m.logs[drop:]...)
	}
	m.mu.Unlock()

	// 从日志推断连接状态
	m.inferState(entry)
}

// inferState 依据日志的事件码推断连接状态。
//
// 用事件码而不是匹配中文摘要，这样改文案不会影响状态判断。
func (m *Manager) inferState(e LogEntry) {
	switch e.Kind {
	case KindConnected:
		m.mu.Lock()
		m.restartCount = 0 // 连上了，重置重启计数
		m.mu.Unlock()
		m.setState(StateConnected, "")
	case KindLoginFailed:
		m.setState(StateFailed, e.Detail)
	case KindConnectFailed:
		m.setState(StateFailed, e.Detail)
	}
}

// HideConsole 让子进程不要弹出控制台窗口。
//
// ⚠ **Windows 上凡是起 frpc.exe 的地方都要调它**，否则会"黑窗口闪一下"：
// 本程序是 windowsgui 子系统（自身没有控制台），而 frpc.exe 是**控制台**程序。
// 用 exec.Command 直接起它，Windows 会为它**新分配一个控制台窗口** ——
// 那个窗口在屏幕上闪一下就消失，非常显眼，而且看起来像程序出了毛病。
//
// CREATE_NO_WINDOW(0x08000000) 是"不要分配控制台"，HideWindow 再兜一层。
// 两个都设是保险，单独用其中一个在部分场景下仍可能闪。
//
// 注意：这**不影响输出捕获**。cmd.Output() / StdoutPipe() 拿到的是管道，
// 跟子进程有没有控制台无关 —— 有回归测试守着这一点。
func HideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
}

// Start 启动 frpc 进程。
//
// Windows 上使用 CREATE_NO_WINDOW + HideWindow，保证不弹控制台窗口。
func (m *Manager) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("frpc 已在运行")
	}
	m.stopping = false
	m.mu.Unlock()

	cmd := exec.Command(m.exePath, "-c", m.cfgPath)
	HideConsole(cmd)

	// 捕获输出。frpc 默认 log.to = "console"，所以日志都在 stdout 上。
	// 我们把它接过来：既能推断状态，又能喂给界面做中文展示。
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建 stdout 管道失败: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("创建 stderr 管道失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		m.setState(StateFailed, fmt.Sprintf("启动 frpc 失败: %v", err))
		return fmt.Errorf("启动 frpc 失败: %w", err)
	}

	m.mu.Lock()
	m.cmd = cmd
	m.running = true
	m.mu.Unlock()

	m.setState(StateStarting, "")

	// 两路输出分别读，避免互相阻塞
	go m.readLoop(stdout)
	go m.readLoop(stderr)
	go m.waitLoop(cmd)

	return nil
}

// readLoop 按行读取子进程输出。
func (m *Manager) readLoop(r io.Reader) {
	sc := bufio.NewScanner(r)
	// frp 的日志行可能较长（错误堆栈），把缓冲放大到 1MB
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		m.appendLog(sc.Text())
	}
}

// waitLoop 等待进程退出，并在异常退出时按退避策略自动重启。
func (m *Manager) waitLoop(cmd *exec.Cmd) {
	err := cmd.Wait()

	m.mu.Lock()
	m.running = false
	m.cmd = nil
	stopping := m.stopping
	tries := m.restartCount
	autoRestart := m.autoRestart
	m.mu.Unlock()

	if stopping {
		// 主动停止，不重启
		m.setState(StateStopped, "")
		return
	}

	reason := "frpc 进程已退出"
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		reason = fmt.Sprintf("frpc 异常退出（退出码 %d）", exitCode)
	}

	if !autoRestart {
		m.setState(StateStopped, reason)
		return
	}

	m.setState(StateStopped, reason)

	if tries >= maxRestartTries {
		m.appendLog(fmt.Sprintf("frpc 连续退出 %d 次，已停止自动重启", tries))
		m.setState(StateFailed, fmt.Sprintf("连续启动失败 %d 次，请检查配置或查看日志", tries))
		return
	}

	m.mu.Lock()
	m.restartCount++
	m.mu.Unlock()

	m.appendLog(fmt.Sprintf("将在 %v 后自动重启（第 %d 次）", restartDelay, tries+1))
	time.AfterFunc(restartDelay, func() {
		m.mu.RLock()
		stillStopping := m.stopping
		running := m.running
		m.mu.RUnlock()
		if stillStopping || running {
			return
		}
		if err := m.Start(); err != nil {
			m.appendLog("自动重启失败：" + err.Error())
		}
	})
}

// Stop 优雅停止 frpc。
//
// 优先调用 admin API 的 /api/stop（让 frpc 自己优雅退出），
// 超时后再强杀。这样能避免留下僵尸进程。
//
// ⚠ 这里**不能**调用 cmd.Wait() —— 进程回收由 waitLoop 独家负责，
// 重复调用 Wait 会返回 "exec: Wait was already called"。
// 所以这里改为轮询 running 标志。
func (m *Manager) Stop(graceful func() error) error {
	m.mu.Lock()
	if !m.running || m.cmd == nil {
		m.mu.Unlock()
		m.setState(StateStopped, "")
		return nil
	}
	m.stopping = true // 阻止自动重启
	cmd := m.cmd
	m.mu.Unlock()

	// 第一步：请 frpc 自己优雅退出
	if graceful != nil {
		_ = graceful()
	}

	// 第二步：等 waitLoop 回收进程
	if m.waitExit(5 * time.Second) {
		m.setState(StateStopped, "")
		return nil
	}

	// 第三步：超时则强杀
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	m.waitExit(3 * time.Second)
	m.setState(StateStopped, "")
	return nil
}

// waitExit 轮询等待进程退出，返回是否在超时前退出。
func (m *Manager) waitExit(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if !m.IsRunning() {
			return true
		}
		time.Sleep(80 * time.Millisecond)
	}
	return !m.IsRunning()
}

// MarkStopping 标记为主动停止，阻止自动重启。
// 在调用 Stop 之前或进程退出前调用。
func (m *Manager) MarkStopping() {
	m.mu.Lock()
	m.stopping = true
	m.mu.Unlock()
}
