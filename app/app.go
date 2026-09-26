package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"frpcgui/internal/cfgfile"
	"frpcgui/internal/frpcapi"
	"frpcgui/internal/frpcman"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是暴露给前端的所有能力。Wails 会把它的公开方法绑定成 JS 函数。
type App struct {
	ctx context.Context

	baseDir string // frpc.exe / frpc.toml 所在目录（默认为本程序所在目录）
	cfgPath string
	exePath string

	cfg   *cfgfile.ConnConfig
	man   *frpcman.Manager
	apiFn func() *frpcapi.Client // 延迟创建：webServer 端口可能在向导里被改

	settings Settings
	tray     *trayUI

	// configFixed 是"启动时自动修正了配置"的提示，界面显示一次。
	// 之所以放在状态里而不是用事件推送：事件可能早于界面订阅而被丢掉。
	configFixed string

	lastState  frpcman.State
	stateSince time.Time
}

// NewApp 创建应用实例。
//
// 路径策略：frpc.exe 和 frpc.toml 都放在**本程序所在的目录**。
// 用户拿到的就是一个文件夹，双击 exe 即可。
func NewApp() *App {
	exe, err := os.Executable()
	base := "."
	if err == nil {
		base = filepath.Dir(exe)
	}

	a := &App{
		baseDir:   base,
		cfgPath:   filepath.Join(base, "frpc.toml"),
		exePath:   filepath.Join(base, "frpc.exe"),
		lastState: frpcman.StateStopped,
		settings:  Settings{CloseToTray: false, AutoRestart: true},
	}

	a.man = frpcman.New(a.exePath, a.cfgPath)
	a.man.OnStateChange(func(s frpcman.State, msg string) {
		if s != a.lastState {
			a.stateSince = time.Now()
		}
		a.lastState = s
		if a.tray != nil {
			a.tray.update(s, msg)
		}
		if a.ctx != nil {
			wruntime.EventsEmit(a.ctx, "frp:state", map[string]any{
				"state": string(s),
				"error": msg,
			})
		}
	})

	a.tray = newTrayUI(a)

	return a
}

func (a *App) startup(ctx context.Context) {
	debugLog("startup: 进入")
	a.ctx = ctx

	// 读取配置（文件不存在时返回默认值，不报错）
	cfg, err := cfgfile.Load(a.cfgPath)
	if err != nil {
		// 配置损坏时不阻断启动，让界面提示用户重新填写
		cfg = cfgfile.Default()
	}
	a.cfg = cfg
	debugLog("startup: 配置已读 serverAddr=%q webPort=%d", cfg.ServerAddr, cfg.WebPort)

	// ⚠ 自愈：确保 frpc 把日志写到 stdout。
	// 老版本的向导误把 log.to 写成了 frpc.log，那样界面读不到任何输出，
	// 会永远停在"正在连接…"。这里发现就改掉（并备份原文件），同时告诉用户。
	_ = a.fixLogTarget()
	debugLog("startup: 日志目标检查完成，configFixed=%q", a.configFixed)

	a.apiFn = func() *frpcapi.Client {
		return frpcapi.New(a.cfg.WebAddr, a.cfg.WebPort, a.cfg.WebUser, a.cfg.WebPass)
	}

	// 界面偏好（托盘行为、自动重启）
	a.settings = a.GetSettings()
	a.man.SetAutoRestart(a.settings.AutoRestart)

	// 预热 frp 版本号。它要跑一次 `frpc.exe -v`（几十到几百毫秒），
	// 放在后台线程里先跑掉 —— 否则用户点开「关于」页时会看到
	// "整页先空着，然后内容一下子蹦出来"，那是 GetAboutInfo 在等这个子进程。
	go a.frpVersion()

	// 托盘在独立线程上跑，尽早起来，这样启动过程也能看到状态变化
	if a.tray != nil {
		debugLog("startup: 启动托盘")
		a.tray.start()
	}

	// 只有配置齐备且 frpc.exe 存在时才自动启动
	if a.readyToStart() {
		go func() {
			time.Sleep(600 * time.Millisecond)
			if err := a.StartFrp(); err != nil {
				// 失败原因会同时通过界面上的提示条呈现，
				// 这里再推一条 toast，覆盖"用户正好看着窗口"的情况
				a.TrayToast("frp 未启动", err.Error(), "err")
			}
		}()
	}
	debugLog("startup: 完成")
}

// beforeClose 在用户点击窗口关闭按钮时触发。
//
// 两种行为：
//   - 设置里打开了「关闭窗口时最小化到托盘」→ 直接隐藏，不打扰用户
//   - 否则 → 弹窗询问
//
// ⚠ 关于弹窗，这里有一个 Wails v2 的限制（已核对源码
//
//	internal/frontend/desktop/windows/dialog.go:160）：
//	Windows 上 QuestionDialog 被硬编码成 MB_YESNO，
//	options.Buttons 里传的中文按钮文字**会被完全忽略**。
//	所以这里改成用「是 / 否」两个系统按钮表达三个语义：
//	  是   → 最小化到托盘（安全选项，也是默认按钮）
//	  否   → 退出程序
//	  直接关掉对话框 / 按 Esc → MessageBox 返回 Cancel，当作"取消"，窗口保持打开
//
// 返回 true 阻止关闭；返回 false 继续关闭流程。
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	if a.settings.CloseToTray {
		wruntime.WindowHide(ctx)
		return true
	}

	choice, err := wruntime.MessageDialog(ctx, wruntime.MessageDialogOptions{
		Type:  wruntime.QuestionDialog,
		Title: "关闭 CikPier",
		Message: "你想怎么关闭？\n\n" +
			"【是】最小化到托盘 —— 程序继续在后台运行，frp 不会中断，\n" +
			"　　　之后可以从任务栏右下角的托盘图标重新打开。\n\n" +
			"【否】退出程序 —— 停止 frp 并完全退出，所有代理都会下线。\n\n" +
			"（直接关掉本对话框 = 取消，什么都不做）",
	})
	if err != nil {
		return false
	}
	switch choice {
	case "Yes":
		wruntime.WindowHide(ctx)
		return true // 阻止关闭，改为隐藏
	case "No":
		return false // 允许关闭，shutdown 里会停掉 frpc
	default:
		return true // Cancel / 关闭对话框 = 取消
	}
}

// shutdown 在应用退出时调用：必须停掉 frpc，否则会留下孤儿进程，
// 下次启动时端口冲突，用户完全不知道为什么。
func (a *App) shutdown(ctx context.Context) {
	debugLog("shutdown: 进入")
	if a.tray != nil {
		a.tray.stop()
	}
	a.man.MarkStopping()
	_ = a.man.Stop(func() error {
		if a.apiFn == nil {
			return nil
		}
		return a.apiFn().Stop()
	})
	debugLog("shutdown: 完成")
}

// QuitApp 完全退出程序（托盘菜单的「退出程序」用它）。
func (a *App) QuitApp() {
	a.man.MarkStopping()
	_ = a.StopFrp()
	if a.tray != nil {
		a.tray.stop()
	}
	if a.ctx != nil {
		wruntime.Quit(a.ctx)
	}
}

// TrayToast 把一条提示推给界面显示成轻提示。
//
// 窗口隐藏时用户看不到，这是可以接受的 —— 托盘图标颜色本身就反映了状态。
func (a *App) TrayToast(title, desc, kind string) {
	if a.ctx == nil {
		return
	}
	wruntime.EventsEmit(a.ctx, "frp:toast", map[string]any{
		"title": title,
		"desc":  desc,
		"type":  kind,
	})
}

// ---------------------------------------------------------------- 对外数据结构

// AppState 是界面顶栏需要的综合状态。
type AppState struct {
	// frpc 进程
	ProcessRunning bool   `json:"processRunning"`
	State          string `json:"state"`     // stopped / starting / connected / failed
	StateText      string `json:"stateText"` // 中文
	StateError     string `json:"stateError"`

	// 配置
	Configured   bool   `json:"configured"` // 是否已完成首次配置
	ServerAddr   string `json:"serverAddr"`
	ServerPort   int    `json:"serverPort"`
	User         string `json:"user"`
	ExeExists    bool   `json:"exeExists"`
	ExePath      string `json:"exePath"`
	ConfigPath   string `json:"configPath"`
	StoreEnabled bool   `json:"storeEnabled"`

	// 需要用户处理的问题
	NeedSetup       bool     `json:"needSetup"`       // 没配置过
	MissingExe      bool     `json:"missingExe"`      // 找不到 frpc.exe
	FileProxies     []string `json:"fileProxies"`     // 写在 frpc.toml 里、界面管不了的代理
	ExternalRunning bool     `json:"externalRunning"` // 本机已有别的 frpc 在跑（不是本程序拉起的）

	// ConfigFixed 非空时表示启动时自动改过配置文件，界面应提示用户
	ConfigFixed string `json:"configFixed"`

	// 架构相关：程序跑在 ARM 电脑的模拟层里时要提示换包
	ProcessArch string `json:"processArch"`
	NativeArch  string `json:"nativeArch"`
	Emulated    bool   `json:"emulated"`
	ArchHint    string `json:"archHint"`

	// 进程在跑但一条日志都没读到 —— 这是"界面卡在正在连接"的典型症状
	NoLogs bool `json:"noLogs"`
	// 已经"正在连接"了多少秒（用于给用户一个明确的等待感，而不是无限转圈）
	StartingSeconds int `json:"startingSeconds"`
}

// ProxyView 是合并后的代理视图，界面直接渲染它。
//
// 数据来源有三处：
//  1. Store API  → 代理的**定义**（可编辑）
//  2. /api/status → 代理的**运行时状态**
//  3. frpc.toml  → 手写的代理（只能看，界面改不了）
type ProxyView struct {
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	Enabled    bool               `json:"enabled"`
	Status     string             `json:"status"`     // frp 原始状态值
	StatusText string             `json:"statusText"` // 中文
	Err        string             `json:"err"`
	ErrText    string             `json:"errText"`  // 中文（未命中翻译时等于原文）
	ErrKnown   bool               `json:"errKnown"` // 错误是否已翻译
	LocalAddr  string             `json:"localAddr"`
	RemoteAddr string             `json:"remoteAddr"`
	FromFile   bool               `json:"fromFile"` // true = 来自配置文件，界面不可改
	Def        frpcapi.Definition `json:"def"`      // 完整定义，编辑弹窗用
}

// ---------------------------------------------------------------- 状态查询

func (a *App) readyToStart() bool {
	if a.cfg == nil {
		return false
	}
	// ⚠ 只校验地址，**不校验令牌**：服务端可以不配认证（frps.toml 里没有
	//   auth.token），这时客户端也必须留空 —— 两边都空时
	//   GetAuthKey("", ts) 算出来相同，校验照样通过。
	//   所以"令牌为空"是一个合法状态，不能当成没配好。
	if strings.TrimSpace(a.cfg.ServerAddr) == "" {
		return false
	}
	return cfgfile.Exists(a.exePath)
}

// apiReady 表示本机管理接口能通。
//
// 用 /healthz 探活：这个接口不需要认证，且只有 frpc 才会响应。
func (a *App) apiReady() bool {
	if a.apiFn == nil {
		return false
	}
	return a.apiFn().Healthy() == nil
}

// externalRunning 表示"管理端口上有一个 frpc，但它不是本程序拉起的"。
//
// 典型场景：上次程序异常退出留下了孤儿进程，或者用户自己/系统服务已经启动了 frpc。
// 这时再去启动一个会因为管理端口被占用而失败，必须先告诉用户。
func (a *App) externalRunning() bool {
	return !a.man.IsRunning() && a.apiReady()
}

// canManage 表示能否通过管理接口读写代理。
func (a *App) canManage() bool {
	return a.man.IsRunning() || a.apiReady()
}

// GetState 返回顶栏所需的综合状态。前端定时轮询。
func (a *App) GetState() AppState {
	st := AppState{
		ProcessRunning: a.man.IsRunning(),
		ExeExists:      cfgfile.Exists(a.exePath),
		ExePath:        a.exePath,
		ConfigPath:     a.cfgPath,
	}

	state, errMsg := a.man.State()
	st.State = string(state)
	st.StateText = translateConnState(state)
	st.StateError = errMsg

	if a.cfg != nil {
		st.ServerAddr = a.cfg.ServerAddr
		st.ServerPort = a.cfg.ServerPort
		st.User = a.cfg.User
		// 令牌可以是空的：服务端没配 auth.token 时客户端也留空即可连上，
		// 所以"有没有配好"只看服务器地址。
		st.Configured = strings.TrimSpace(a.cfg.ServerAddr) != ""
	}

	st.MissingExe = !st.ExeExists
	st.NeedSetup = !st.Configured || st.MissingExe

	// 检测配置文件里手写的代理（界面管不了的）
	if names, err := cfgfile.FileProxyNames(a.cfgPath); err == nil {
		st.FileProxies = names
	}

	// 管理接口是否可用；以及是不是"别人拉起的" frpc
	apiUp := a.apiReady()
	st.ExternalRunning = apiUp && !a.man.IsRunning()
	st.StoreEnabled = apiUp

	st.ConfigFixed = a.configFixed

	// 架构：跑在模拟层里时给用户明确提示（ARM 电脑上装了 x64 包）
	arch := detectArch()
	st.ProcessArch = arch.Process
	st.NativeArch = arch.Native
	st.Emulated = arch.Emulated
	st.ArchHint = arch.Hint

	// "正在连接"持续了多久
	if state == frpcman.StateStarting && !a.stateSince.IsZero() {
		st.StartingSeconds = int(time.Since(a.stateSince).Seconds())
	}

	// 进程在跑，但一条日志都没有 —— 唯一的原因就是 frpc 没往 stdout 写。
	// 这是"界面永远停在正在连接"的根因，必须在界面上点破。
	// 给 5 秒宽限期：frpc 启动到打第一行日志通常只要几百毫秒。
	if st.ProcessRunning && st.StartingSeconds >= 5 {
		st.NoLogs = len(a.man.LogsTail(1)) == 0
	}

	return st
}

// fixLogTarget 确保 frpc 的日志写到 stdout（这是界面能工作的前提）。
//
// 结果会记到 a.configFixed 上给界面显示；没有需要修正的地方时置空。
// 之所以在**启动时**而不是只在保存时修正：用户可能是从别处拷来的 frpc.toml，
// 也可能用的是旧版本向导生成的配置，不修正的话界面会一直卡在"正在连接…"。
func (a *App) fixLogTarget() string {
	a.configFixed = ""

	old, changed := cfgfile.NormalizeLogTo(a.cfg)
	if !changed || a.cfg == nil {
		return ""
	}
	if err := cfgfile.Save(a.cfgPath, a.cfg); err != nil {
		a.configFixed = "检测到 frpc.toml 里的 log.to 指向文件（" + old +
			"），图形界面需要读取 frpc 的输出才能工作，但自动修正失败：" + err.Error()
		return a.configFixed
	}
	a.configFixed = "已自动修正 frpc.toml：log.to 原来是「" + old +
		"」，已改为「console」。图形界面通过读取 frpc 的输出判断连接状态和显示日志，" +
		"写成文件就读不到了。原文件已备份为 frpc.toml.bak。"
	return a.configFixed
}

func translateConnState(s frpcman.State) string {
	switch s {
	case frpcman.StateStopped:
		return "未运行"
	case frpcman.StateStarting:
		return "正在连接…"
	case frpcman.StateConnected:
		return "已连接到服务端"
	case frpcman.StateFailed:
		return "连接失败"
	default:
		return string(s)
	}
}

// ListProxies 返回合并后的代理列表。
//
// 合并规则（对应官方前端 stores/proxy.ts 的 storeProxyWithStatus）：
//   - 以 Store 定义为骨架（这样被停用的代理也能显示出来）
//   - 用 /api/status 的运行态补充 status / local_addr / remote_addr
//   - 配置文件里手写的代理追加到末尾，并标记 FromFile = true
func (a *App) ListProxies() ([]ProxyView, error) {
	if !a.canManage() {
		// frpc 没跑、管理接口也连不上时，至少把配置文件里的代理列出来
		return a.fileOnlyProxies(), nil
	}

	api := a.apiFn()

	// 1. 运行时状态，按名字索引
	statusMap := map[string]frpcapi.ProxyStatus{}
	if raw, err := api.Status(); err == nil {
		for _, arr := range raw {
			for _, ps := range arr {
				statusMap[ps.Name] = ps
			}
		}
	}

	// 2. Store 定义
	storeDefs, storeErr := api.ListStoreProxies()
	if storeErr != nil && storeErr != frpcapi.ErrStoreDisabled {
		return nil, storeErr
	}

	views := make([]ProxyView, 0, len(storeDefs))
	seen := map[string]bool{}

	for _, def := range storeDefs {
		v := buildView(def, statusMap, false)
		views = append(views, v)
		seen[v.Name] = true
	}

	// 3. 配置文件里的代理（Store 里没有的）
	if defs, err := cfgfile.FileProxyDefs(a.cfgPath); err == nil {
		for _, fd := range defs {
			name, _ := fd["name"].(string)
			if name == "" || seen[name] {
				continue
			}
			views = append(views, buildView(fd, statusMap, true))
			seen[name] = true
		}
	}

	sort.Slice(views, func(i, j int) bool {
		if views[i].FromFile != views[j].FromFile {
			return !views[i].FromFile // Store 的排前面
		}
		return views[i].Name < views[j].Name
	})
	return views, nil
}

func (a *App) fileOnlyProxies() []ProxyView {
	defs, err := cfgfile.FileProxyDefs(a.cfgPath)
	if err != nil {
		return []ProxyView{}
	}
	out := make([]ProxyView, 0, len(defs))
	for _, d := range defs {
		name, _ := d["name"].(string)
		if name == "" {
			continue
		}
		typ, _ := d["type"].(string)
		out = append(out, ProxyView{
			Name:       name,
			Type:       typ,
			Enabled:    true,
			Status:     "",
			StatusText: "未运行",
			FromFile:   true,
			Def:        d,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// buildView 把一个代理定义 + 运行时状态合并成界面视图。
func buildView(def frpcapi.Definition, statusMap map[string]frpcapi.ProxyStatus, fromFile bool) ProxyView {
	name, _ := def["name"].(string)
	typ, _ := def["type"].(string)

	v := ProxyView{
		Name:     name,
		Type:     typ,
		Enabled:  true,
		FromFile: fromFile,
		Def:      def,
	}

	// 类型块形如 {"name":..,"type":"tcp","tcp":{...}}
	block, _ := def[typ].(map[string]any)

	// enabled：不写或 true 都算启用（对应 frp 源码 Enabled *bool 的语义）
	if block != nil {
		if e, ok := block["enabled"].(bool); ok {
			v.Enabled = e
		}
		// 本机/对外地址：优先用运行时状态，没有就自己拼
		if v.LocalAddr == "" {
			lip, _ := block["localIP"].(string)
			if lip == "" {
				lip = "127.0.0.1"
			}
			if lp, ok := toInt(block["localPort"]); ok && lp > 0 {
				v.LocalAddr = fmt.Sprintf("%s:%d", lip, lp)
			}
		}
		if rp, ok := toInt(block["remotePort"]); ok && rp > 0 {
			v.RemoteAddr = fmt.Sprintf(":%d", rp)
		}
		if sd, _ := block["subdomain"].(string); sd != "" {
			v.RemoteAddr = sd // 域名由服务端拼接，这里先显示子域名前缀
		}
	}

	// 运行时状态覆盖
	if ps, ok := statusMap[name]; ok {
		v.Status = ps.Status
		v.StatusText = frpcman.TranslateStatus(ps.Status)
		v.Err = ps.Err
		if ps.Err != "" {
			v.ErrText, v.ErrKnown = frpcman.TranslateError(ps.Err)
		}
		if ps.LocalAddr != "" {
			v.LocalAddr = ps.LocalAddr
		}
		if ps.RemoteAddr != "" {
			v.RemoteAddr = ps.RemoteAddr
		}
		return v
	}

	// 没有运行时状态：区分"被停用"和"还没起来"
	if !v.Enabled {
		v.Status = "disabled"
		v.StatusText = "已停用"
	} else {
		v.Status = "waiting"
		v.StatusText = "未运行"
	}
	return v
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// ---------------------------------------------------------------- 代理操作

// ToggleProxy 单独启用 / 停用一条代理。
//
// 实现方式（与官方前端一致）：读出 Store 定义 → 把类型块里的 enabled 改掉 → PUT 回去。
// 官方会触发增量重载，只有这一条代理会停/启，其他不受影响。
//
// ⚠ 配置文件里手写的代理不支持这个操作，调用方应先检查 FromFile。
func (a *App) ToggleProxy(name string, enabled bool) error {
	if !a.canManage() {
		return fmt.Errorf("frp 未运行")
	}
	api := a.apiFn()

	def, err := api.GetStoreProxy(name)
	if err != nil {
		if err == frpcapi.ErrStoreDisabled || err == frpcapi.ErrNotFound {
			return fmt.Errorf("「%s」不在界面管理范围内（可能写在配置文件里），无法启停", name)
		}
		return err
	}

	typ, _ := def["type"].(string)
	block, _ := def[typ].(map[string]any)
	if block == nil {
		return fmt.Errorf("代理「%s」的定义缺少 %s 配置块", name, typ)
	}
	block["enabled"] = enabled

	if _, err := api.UpdateStoreProxy(name, def); err != nil {
		return fmt.Errorf("保存失败: %w", err)
	}
	return nil
}

// DeleteProxy 删除 Store 中的代理。
func (a *App) DeleteProxy(name string) error {
	if !a.canManage() {
		return fmt.Errorf("frp 未运行")
	}
	if err := a.apiFn().DeleteStoreProxy(name); err != nil {
		if err == frpcapi.ErrNotFound {
			return fmt.Errorf("「%s」不在界面管理范围内，无法删除", name)
		}
		return err
	}
	return nil
}

// SaveProxy 新建或更新一条代理。
//
// def 必须符合 frp 的 ProxyDefinition 结构：
// {"name":"web","type":"http","http":{...}}
// URL 里的 name 必须与 body 里的 name 一致，否则官方返回 400。
func (a *App) SaveProxy(def frpcapi.Definition, isUpdate bool) error {
	if !a.canManage() {
		return fmt.Errorf("frp 未运行")
	}
	name, _ := def["name"].(string)
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("代理名不能为空")
	}
	api := a.apiFn()

	if isUpdate {
		if _, err := api.UpdateStoreProxy(name, def); err != nil {
			return err
		}
		return nil
	}
	if _, err := api.CreateStoreProxy(def); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "Conflict") {
			return fmt.Errorf("代理名「%s」已存在，换一个", name)
		}
		return err
	}
	return nil
}

// GetProxy 读取单条代理的完整定义（编辑弹窗用）。
func (a *App) GetProxy(name string) (frpcapi.Definition, error) {
	if !a.canManage() {
		return nil, fmt.Errorf("frp 未运行")
	}
	return a.apiFn().GetStoreProxy(name)
}

// ---------------------------------------------------------------- 日志

// GetLogs 增量拉取日志。afterSeq 传上次拿到的最大 seq，首次传 0。
func (a *App) GetLogs(afterSeq int64) []frpcman.LogEntry {
	if afterSeq <= 0 {
		return a.man.LogsTail(300)
	}
	logs := a.man.Logs(afterSeq)
	if len(logs) > 500 {
		logs = logs[len(logs)-500:]
	}
	return logs
}

// ClearLogs 清空日志显示（不影响磁盘上的日志文件）。
func (a *App) ClearLogs() {
	a.man.ClearLogs()
}

// ---------------------------------------------------------------- 进程控制

// StartFrp 启动 frpc。
func (a *App) StartFrp() error {
	if !cfgfile.Exists(a.exePath) {
		return fmt.Errorf("找不到 frpc.exe，它应该与本程序放在同一目录：%s", a.exePath)
	}
	if a.man.IsRunning() {
		return nil
	}
	// ⚠ 管理端口上已经有 frpc 在响应，说明本机已有别的实例在跑。
	// 这时再启动一个会因为端口被占用而失败 —— 而且失败信息用户完全看不懂，
	// 所以直接给出明确提示。
	if a.externalRunning() {
		addr := a.cfg.WebAddr
		if strings.TrimSpace(addr) == "" {
			addr = cfgfile.DefaultWebAddr
		}
		return fmt.Errorf("检测到本机已经有一个 frp 进程在运行（管理端口 %s:%d 已被占用）。"+
			"请先在设置页把它停掉，或者直接使用那一个",
			addr, a.cfg.WebPort)
	}
	return a.man.Start()
}

// FixLogTargetNow 重新从磁盘读取配置、修正日志目标并重启 frp。
//
// 用途：用户在程序运行期间手工把 frpc.toml 里的 log.to 改成了文件路径，
// 界面就会读不到日志。这个按钮让用户不用重开程序就能修好。
func (a *App) FixLogTargetNow() error {
	cfg, err := cfgfile.Load(a.cfgPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}
	a.cfg = cfg
	if a.fixLogTarget() == "" {
		return fmt.Errorf("配置文件里的 log.to 已经是 console 了，不需要修正。" +
			"如果界面仍然读不到日志，请到「连接诊断」看看")
	}
	if a.man.IsRunning() {
		return a.RestartFrp()
	}
	return a.StartFrp()
}

// StopExternalFrp 停止"不是本程序拉起"的那个 frpc。
// ⚠ 只在用户显式点击时调用。绝不自动执行 ——
//
//	否则可能把用户自己用系统服务管理的 frpc 悄悄停掉。
func (a *App) StopExternalFrp() error {
	if !a.externalRunning() {
		return fmt.Errorf("没有检测到外部的 frp 进程")
	}
	if err := a.apiFn().Stop(); err != nil {
		return fmt.Errorf("停止失败: %w", err)
	}
	return nil
}

// StopFrp 停止 frpc。
func (a *App) StopFrp() error {
	if a.externalRunning() {
		return fmt.Errorf("当前运行的 frp 不是本程序启动的，请用「停止外部 frp」按钮")
	}
	a.man.MarkStopping()
	return a.man.Stop(func() error {
		if a.apiFn == nil {
			return nil
		}
		return a.apiFn().Stop()
	})
}

// RestartFrp 重启 frpc。
func (a *App) RestartFrp() error {
	_ = a.StopFrp()
	time.Sleep(500 * time.Millisecond)
	return a.StartFrp()
}

// ---------------------------------------------------------------- 配置

// GetConfig 返回当前连接配置（token 打码，界面点"显示"时再取真值）。
func (a *App) GetConfig() cfgfile.ConnConfig {
	if a.cfg == nil {
		return *cfgfile.Default()
	}
	return *a.cfg
}

// SaveConfig 保存连接配置并重启 frpc 使其生效。
//
// ⚠ 全局参数（serverAddr / auth 等）无法通过 /api/reload 生效，必须重启进程。
// ⚠ 用户没展开高级选项时，传入的 serverPort 若是 0，要保留原值而不是重置成默认 7000。
func (a *App) SaveConfig(c cfgfile.ConnConfig) error {
	if a.cfg != nil {
		if c.ServerPort == 0 {
			c.ServerPort = a.cfg.ServerPort
		}
		if c.WebPort == 0 {
			c.WebPort = a.cfg.WebPort
		}
		// ⚠ 这里**故意不**把空令牌恢复成旧值。
		//   "服务端不认证"是合法配置，此时令牌必须为空；
		//   如果照 ServerPort 那样"空就保留旧值"，用户就永远清不掉令牌了。
		//   两个调用方（配置向导、设置页）都是先 Object.assign 展开完整配置
		//   再覆盖字段，所以不会出现"漏传字段被清空"的情况。
		if strings.TrimSpace(c.WebPass) == "" {
			c.WebPass = a.cfg.WebPass
		}
	}

	if strings.TrimSpace(c.ServerAddr) == "" {
		return fmt.Errorf("服务器地址不能为空")
	}
	// 令牌允许为空 —— 表示服务端没有开启认证。

	// GUI 与 frpc 通信用的面板密码，用户不需要关心，没设就自动生成
	if strings.TrimSpace(c.WebPass) == "" {
		c.WebPass = randomPassword()
	}
	if strings.TrimSpace(c.WebUser) == "" {
		c.WebUser = "frpcgui"
	}
	if strings.TrimSpace(c.StorePath) == "" {
		c.StorePath = "frpc-store.json"
	}
	// ⚠ 强制 console：图形界面靠读 frpc 的 stdout 判断连接状态和显示日志。
	// 用户可以自己改成文件，但那样界面就会瞎掉，所以每次保存都拉回来。
	c.LogTo = cfgfile.ConsoleLogTarget
	// ⚠ 强制 false：否则重启后连不上服务端会直接退出，用户完全无感知
	c.LoginFailExit = false

	if err := cfgfile.Save(a.cfgPath, &c); err != nil {
		return err
	}
	a.cfg = &c

	// 重启让全局参数生效
	if a.man.IsRunning() {
		return a.RestartFrp()
	}
	return a.StartFrp()
}

// randomPassword 生成随机面板密码。
func randomPassword() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 24)
	seed := time.Now().UnixNano()
	for i := range b {
		seed = seed*6364136223846793005 + 1442695040888963407
		b[i] = chars[uint64(seed>>33)%uint64(len(chars))]
	}
	return string(b)
}

// ---------------------------------------------------------------- 诊断

// DiagItem 是诊断结果的一项。
type DiagItem struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Warn    bool   `json:"warn"`
	Message string `json:"message"`
}

// Diagnose 依次检查三个关键环节，用于帮非技术用户快速定位问题。
func (a *App) Diagnose() []DiagItem {
	items := []DiagItem{}

	// ① frpc.exe 是否存在
	if !cfgfile.Exists(a.exePath) {
		items = append(items, DiagItem{Name: "程序文件", Message: "找不到 frpc.exe，请确认它和本程序在同一目录"})
		return items
	}
	items = append(items, DiagItem{Name: "程序文件", OK: true, Message: "frpc.exe 就位"})

	// ② 配置是否完整（令牌可以为空：服务端可能没有开启认证）
	if a.cfg == nil || a.cfg.ServerAddr == "" {
		items = append(items, DiagItem{Name: "配置", Message: "连接配置不完整，请先在设置里填写服务器地址"})
		return items
	}
	items = append(items, DiagItem{
		Name: "配置", OK: true,
		Message: fmt.Sprintf("服务器 %s:%d，身份 %s，认证 %s",
			a.cfg.ServerAddr, a.cfg.ServerPort, orDash(a.cfg.User), tokenState(a.cfg.AuthToken)),
	})

	// ③ 进程与连接状态
	if !a.canManage() {
		items = append(items, DiagItem{Name: "frp 进程", Message: "frp 未运行"})
		return items
	}
	if a.externalRunning() {
		items = append(items, DiagItem{
			Name: "frp 进程", Warn: true,
			Message: "正在运行的 frp 不是本程序启动的（管理端口已被占用）。界面可以查看和修改它的代理，但无法启动/停止它。",
		})
	}

	// ②.5 架构：跑在模拟层里就明确点出来
	if arch := detectArch(); arch.Emulated {
		items = append(items, DiagItem{
			Name: "运行架构", Warn: true,
			Message: fmt.Sprintf("程序是 %s 版，但机器是 %s —— 正在模拟层里运行。%s",
				arch.Process, arch.Native, arch.Hint),
		})
	} else {
		items = append(items, DiagItem{
			Name: "运行架构", OK: true,
			Message: fmt.Sprintf("%s（%s），原生运行", arch.Process, arch.Native),
		})
	}

	// ③.5 日志管道 —— 这是最容易出问题、又最难自己发现的环节
	if a.man.IsRunning() {
		if len(a.man.LogsTail(1)) == 0 {
			items = append(items, DiagItem{
				Name: "日志读取",
				Message: "frpc 正在运行，但界面读不到它的任何输出，所以无法判断连接状态。" +
					"原因通常是 frpc.toml 里的 log.to 指向了文件而不是 console。" +
					"把 log.to 改成 \"console\" 后重启 frp 即可。",
			})
		} else {
			items = append(items, DiagItem{Name: "日志读取", OK: true, Message: "正常，界面能读到 frpc 的输出"})
		}
	}
	state, errMsg := a.man.State()
	switch state {
	case frpcman.StateConnected:
		items = append(items, DiagItem{Name: "服务端连接", OK: true, Message: "已连接，认证通过"})
	case frpcman.StateFailed:
		items = append(items, DiagItem{
			Name:    "服务端连接",
			Message: "连接失败：" + orDash(errMsg),
		})
	default:
		items = append(items, DiagItem{Name: "服务端连接", Warn: true, Message: "正在连接…"})
	}

	// ④ 逐条代理：本机后端是否可达
	views, err := a.ListProxies()
	if err == nil {
		for _, v := range views {
			if v.FromFile || !v.Enabled || v.Err == "" {
				continue
			}
			items = append(items, DiagItem{
				Name:    "代理 " + v.Name,
				Message: v.ErrText,
			})
		}
	}

	return items
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// tokenState 把令牌状态写成给人看的一句话。
//
// 令牌为空是合法配置（服务端没开认证），但直接显示成空白会让人以为"没配好"，
// 所以在诊断页明确写出来。
func tokenState(token string) string {
	if strings.TrimSpace(token) == "" {
		return "未设置（服务端未开启认证）"
	}
	return "已设置"
}

// ---------------------------------------------------------------- 辅助操作

// OpenConfigFolder 在资源管理器里打开程序所在目录。
func (a *App) OpenConfigFolder() {
	_ = exec.Command("explorer", a.baseDir).Start()
}

// OpenLogFolder 打开日志所在目录。
func (a *App) OpenLogFolder() {
	_ = exec.Command("explorer", a.baseDir).Start()
}
