// Package cfgfile 负责读写 frpc.toml。
//
// 分工说明：
//   - 代理的增删改查**不走这里**，走 frpc 的 Store API（见 frpcapi 包）。
//   - 本包只管"连接相关的全局配置"，也就是首次向导要填的那几项，
//     以及必须在配置文件里才能生效的 webServer / store / log。
//
// 采用"读取时解析既有值 + 保存时按模板重新生成"的策略：
// 这样生成的配置文件始终带有完整的中文注释，用户随时可以打开查看，
// 而不会因为我们只改了某一个值就把整个文件搅乱。
package cfgfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// ConnConfig 是 frpc.toml 里我们关心的全部字段。
//
// 同时带 toml 和 json 两套标签：toml 用于落盘（虽然实际是走模板渲染），
// json 用于和前端交换（Wails 会把结构体编成 JSON）。
type ConnConfig struct {
	// 连接
	ServerAddr string `toml:"serverAddr" json:"serverAddr"`
	ServerPort int    `toml:"serverPort" json:"serverPort"`

	// 认证
	AuthMethod string `toml:"-" json:"authMethod"`
	AuthToken  string `toml:"-" json:"authToken"`

	// 身份
	User     string `toml:"user" json:"user"`
	ClientID string `toml:"clientID" json:"clientID"`

	// 本机管理界面（GUI 靠它和 frpc 通信，用户通常不需要关心）
	WebAddr string `toml:"-" json:"webAddr"`
	WebPort int    `toml:"-" json:"webPort"`
	WebUser string `toml:"-" json:"webUser"`
	WebPass string `toml:"-" json:"webPass"`

	// Store（代理动态管理的必备开关）
	StorePath string `toml:"-" json:"storePath"`

	// 日志
	LogTo      string `toml:"-" json:"logTo"`
	LogLevel   string `toml:"-" json:"logLevel"`
	LogMaxDays int    `toml:"-" json:"logMaxDays"`

	// 行为
	LoginFailExit bool `toml:"-" json:"loginFailExit"`
}

// 默认值。serverPort 官方默认是 7000（源码 pkg/config/v1/client.go Complete）。
const (
	DefaultServerPort = 7000
	DefaultWebAddr    = "127.0.0.1"
	DefaultWebPort    = 7400
	DefaultLogLevel   = "info"
	DefaultLogMaxDays = 3
)

// ConsoleLogTarget 是图形界面唯一支持的日志目标。
const ConsoleLogTarget = "console"

// NormalizeLogTo 把日志目标修正为 console，返回原值和是否发生了修改。
//
// 独立成函数是为了让调用方（App）能在修正时给用户明确提示，
// 而不是悄悄改掉他的配置。
func NormalizeLogTo(c *ConnConfig) (old string, changed bool) {
	if c == nil {
		return "", false
	}
	if strings.EqualFold(strings.TrimSpace(c.LogTo), ConsoleLogTarget) {
		return c.LogTo, false
	}
	old = c.LogTo
	c.LogTo = ConsoleLogTarget
	return old, true
}

// Default 返回一份带默认值的配置。
func Default() *ConnConfig {
	return &ConnConfig{
		ServerPort: DefaultServerPort,
		AuthMethod: "token",
		WebAddr:    DefaultWebAddr,
		WebPort:    DefaultWebPort,
		// ⚠ 必须是 console，不能是文件路径。
		//
		// 图形界面是**通过管道读 frpc 的 stdout** 来判断连接状态和展示日志的
		// （frp 的管理接口不提供"是否已连上服务端"，只能从
		//  "login to server success" 这行日志推断）。
		// 如果这里写成文件，管道里什么都读不到，界面就会永远停在"正在连接…"。
		LogTo:         "console",
		LogLevel:      DefaultLogLevel,
		LogMaxDays:    DefaultLogMaxDays,
		LoginFailExit: false, // ⚠ 必须 false，否则开机网络未就绪就退出且不重试
	}
}

// Exists 判断配置文件是否存在。
func Exists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// rawFile 是用于解析的中间结构，和 frpc.toml 的实际层级一一对应。
type rawFile struct {
	ServerAddr string `toml:"serverAddr"`
	ServerPort int    `toml:"serverPort"`

	User     string `toml:"user"`
	ClientID string `toml:"clientID"`

	Auth struct {
		Method    string `toml:"method"`
		Token     string `toml:"token"`
		TokenSrc  any    `toml:"tokenSource"`
		TokenFile any    `toml:"-"`
	} `toml:"auth"`

	WebServer struct {
		Addr     string `toml:"addr"`
		Port     int    `toml:"port"`
		User     string `toml:"user"`
		Password string `toml:"password"`
	} `toml:"webServer"`

	Store struct {
		Path string `toml:"path"`
	} `toml:"store"`

	Log struct {
		To      string `toml:"to"`
		Level   string `toml:"level"`
		MaxDays int    `toml:"maxDays"`
	} `toml:"log"`

	LoginFailExit *bool `toml:"loginFailExit"`

	Proxies  []map[string]any `toml:"proxies"`
	Visitors []map[string]any `toml:"visitors"`
}

// Load 解析已有的配置文件。
//
// 文件不存在时返回默认配置（不报错），方便首次启动直接进入向导。
func Load(path string) (*ConnConfig, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var rf rawFile
	if err := toml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if rf.ServerAddr != "" {
		cfg.ServerAddr = rf.ServerAddr
	}
	if rf.ServerPort != 0 {
		cfg.ServerPort = rf.ServerPort
	}
	cfg.User = rf.User
	cfg.ClientID = rf.ClientID
	if rf.Auth.Method != "" {
		cfg.AuthMethod = rf.Auth.Method
	}
	cfg.AuthToken = rf.Auth.Token
	if rf.WebServer.Addr != "" {
		cfg.WebAddr = rf.WebServer.Addr
	}
	if rf.WebServer.Port != 0 {
		cfg.WebPort = rf.WebServer.Port
	}
	cfg.WebUser = rf.WebServer.User
	cfg.WebPass = rf.WebServer.Password
	cfg.StorePath = rf.Store.Path
	if rf.Log.To != "" {
		cfg.LogTo = rf.Log.To
	}
	if rf.Log.Level != "" {
		cfg.LogLevel = rf.Log.Level
	}
	if rf.Log.MaxDays != 0 {
		cfg.LogMaxDays = rf.Log.MaxDays
	}
	if rf.LoginFailExit != nil {
		cfg.LoginFailExit = *rf.LoginFailExit
	}
	return cfg, nil
}

// FileProxyNames 返回配置文件里定义的代理名列表。
//
// ⚠ 用途很重要：这些代理**无法通过 Store API 管理**（会返回 not found），
// 界面上必须明确提示用户，并提供"一键迁移到界面"的入口。
func FileProxyNames(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rf rawFile
	if err := toml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	names := make([]string, 0, len(rf.Proxies))
	for _, p := range rf.Proxies {
		if n, ok := p["name"].(string); ok && n != "" {
			names = append(names, n)
		}
	}
	return names, nil
}

// FileProxyDefs 返回配置文件里定义的完整代理定义，用于"一键迁移到 Store"。
func FileProxyDefs(path string) ([]map[string]any, error) {
	rf, err := readRaw(path)
	if err != nil {
		return nil, err
	}
	return rf.Proxies, nil
}

// FileVisitorDefs 返回配置文件里定义的访问者，用途同上。
func FileVisitorDefs(path string) ([]map[string]any, error) {
	rf, err := readRaw(path)
	if err != nil {
		return nil, err
	}
	return rf.Visitors, nil
}

// readRaw 解析配置文件为中间结构。文件不存在时返回零值（不报错）。
func readRaw(path string) (*rawFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &rawFile{}, nil
		}
		return nil, err
	}
	var rf rawFile
	if err := toml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	return &rf, nil
}

// Render 按模板生成带中文注释的 frpc.toml 内容。
func Render(c *ConnConfig) string {
	var b strings.Builder

	b.WriteString("# ==============================================================================\n")
	b.WriteString("#  frpc.toml —— CikPier 配置\n")
	b.WriteString("#\n")
	b.WriteString("#  ⚠ 这个文件由 CikPier 自动维护。\n")
	b.WriteString("#    代理请直接在界面里增删改（数据存在 frpc-store.json），\n")
	b.WriteString("#    不要手写 [[proxies]] —— 手写的代理无法在界面里管理。\n")
	b.WriteString("#\n")
	b.WriteString("#  如果你手动改过本文件，重启程序后界面会读到新值。\n")
	b.WriteString("# ==============================================================================\n")
	b.WriteString("\n")

	b.WriteString("# ------------------------------------------------------------------------------\n")
	b.WriteString("#  连接服务端\n")
	b.WriteString("# ------------------------------------------------------------------------------\n")
	fmt.Fprintf(&b, "serverAddr = %q\n", c.ServerAddr)
	fmt.Fprintf(&b, "# 默认 %d。只有管理员使用了非标准端口时才需要改。\n", DefaultServerPort)
	fmt.Fprintf(&b, "serverPort = %d\n", c.ServerPort)

	if c.User != "" {
		b.WriteString("\n# 你的身份标识，代理在服务端的名字会自动变成 {user}.{代理名}\n")
		fmt.Fprintf(&b, "user = %q\n", c.User)
	}
	if c.ClientID != "" {
		b.WriteString("\n# 本机标识，服务端用它区分同一 user 下的多个客户端\n")
		fmt.Fprintf(&b, "clientID = %q\n", c.ClientID)
	}

	b.WriteString("\n\n# ------------------------------------------------------------------------------\n")
	b.WriteString("#  认证（必须与服务端一致）\n")
	b.WriteString("# ------------------------------------------------------------------------------\n")
	b.WriteString("# 这是访问服务器的钥匙，请勿外传。\n")
	fmt.Fprintf(&b, "auth.method = %q\n", orDefault(c.AuthMethod, "token"))
	fmt.Fprintf(&b, "auth.token = %q\n", c.AuthToken)

	b.WriteString("\n\n# ------------------------------------------------------------------------------\n")
	b.WriteString("#  连接行为\n")
	b.WriteString("# ------------------------------------------------------------------------------\n")
	b.WriteString("# ⚠ 必须保持 false。默认值 true 意味着「启动时连不上服务端就直接退出、不再重试」，\n")
	b.WriteString("#    会导致电脑重启后 frp 静默失效。\n")
	fmt.Fprintf(&b, "loginFailExit = %v\n", c.LoginFailExit)

	b.WriteString("\n\n# ------------------------------------------------------------------------------\n")
	b.WriteString("#  本机管理界面\n")
	b.WriteString("#  图形界面靠这个接口读取状态、增删改代理。只监听本机，不对外暴露。\n")
	b.WriteString("# ------------------------------------------------------------------------------\n")
	fmt.Fprintf(&b, "webServer.addr = %q\n", orDefault(c.WebAddr, DefaultWebAddr))
	fmt.Fprintf(&b, "webServer.port = %d\n", orZero(c.WebPort, DefaultWebPort))
	if c.WebUser != "" {
		fmt.Fprintf(&b, "webServer.user = %q\n", c.WebUser)
	}
	if c.WebPass != "" {
		fmt.Fprintf(&b, "webServer.password = %q\n", c.WebPass)
	}
	b.WriteString("# ⚠ 保持 false。pprof 注册在鉴权中间件之外，开启等于未授权可访问。\n")
	b.WriteString("webServer.pprofEnable = false\n")

	b.WriteString("\n\n# ------------------------------------------------------------------------------\n")
	b.WriteString("#  Store —— 代理的动态管理（界面功能依赖它，请勿删除）\n")
	b.WriteString("#  代理会保存到这里指定的文件，程序重启后自动恢复。\n")
	b.WriteString("# ------------------------------------------------------------------------------\n")
	fmt.Fprintf(&b, "store.path = %q\n", orDefault(c.StorePath, "frpc-store.json"))

	b.WriteString("\n\n# ------------------------------------------------------------------------------\n")
	b.WriteString("#  日志\n")
	b.WriteString("#  ⚠ 图形界面通过管道直接读取 frpc 的输出，所以这里用 console 即可。\n")
	b.WriteString("#    如需额外留存文件，可改成具体路径。\n")
	b.WriteString("# ------------------------------------------------------------------------------\n")
	fmt.Fprintf(&b, "log.to = %q\n", orDefault(c.LogTo, "console"))
	fmt.Fprintf(&b, "log.level = %q\n", orDefault(c.LogLevel, DefaultLogLevel))
	fmt.Fprintf(&b, "log.maxDays = %d\n", orZero(c.LogMaxDays, DefaultLogMaxDays))
	b.WriteString("# ⚠ 保持 true。frp 的彩色输出即使写进管道也会带上 ANSI 转义序列，\n")
	b.WriteString("#    会把日志行搞乱（时间戳不在行首），导致图形界面无法识别日志内容。\n")
	b.WriteString("log.disablePrintColor = true\n")

	return b.String()
}

// Save 备份后写入配置文件。
//
// 备份文件名为 frpc.toml.bak，用户改坏了可以手动回滚。
func Save(path string, c *ConnConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 备份已有文件
	if Exists(path) {
		if old, err := os.ReadFile(path); err == nil {
			_ = os.WriteFile(path+".bak", old, 0o600)
		}
	}

	content := Render(c)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}

// BackupPath 返回某次保存前生成的备份文件路径。
func BackupPath(path string) string { return path + ".bak" }

// BackupInfo 描述一次备份。
type BackupInfo struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	ModTime string `json:"modTime"`
	Size    int64  `json:"size"`
}

// GetBackupInfo 读取备份文件信息，供界面展示"可回滚"提示。
func GetBackupInfo(path string) BackupInfo {
	bp := BackupPath(path)
	info := BackupInfo{Path: bp}
	if st, err := os.Stat(bp); err == nil {
		info.Exists = true
		info.Size = st.Size()
		info.ModTime = st.ModTime().Format(time.DateTime)
	}
	return info
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func orZero(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
