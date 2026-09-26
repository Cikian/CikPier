package frpcman

import (
	"regexp"
	"strings"
)

// LogEntry 是一条经过解析和中文翻译的日志。
//
// 界面上默认展示 Summary（中文摘要），Raw（英文原文）折叠在下面。
// 这样普通用户看中文，排查时又能把原文发给管理员。
type LogEntry struct {
	Seq     int64  `json:"seq"`
	Time    string `json:"time"`    // 只保留 HH:MM:SS
	Level   string `json:"level"`   // info / warn / error / debug / trace
	Scope   string `json:"scope"`   // 代理名，可能为空
	Summary string `json:"summary"` // 中文摘要
	Detail  string `json:"detail"`  // 中文补充说明，可能为空
	Raw     string `json:"raw"`     // frp 的英文原文
	Kind    string `json:"kind"`    // 机器可读的事件码，用于状态推断（不展示给用户）
}

// 事件码。用于让上层依据日志可靠地推断连接状态，
// 而不是去匹配中文摘要字符串（那样一改文案就失效）。
const (
	KindConnected     = "connected"      // 登录成功
	KindLoginFailed   = "login_failed"   // 登录被拒
	KindConnectFailed = "connect_failed" // 连不上服务端
	KindProxyUp       = "proxy_up"       // 某个代理上线
	KindProxyError    = "proxy_error"    // 某个代理启动失败
	KindHealthFail    = "health_fail"    // 健康检查失败
	KindReloaded      = "reloaded"       // 配置已重载
	KindStopped       = "stopped"        // frpc 已停止
)

// frp 日志行格式（源码 pkg/util/log）：
//
//	2026-09-24 20:29:29.412 [I] [client/control.go:174] [0ab2ace915055641] [web] start proxy success
//	└── 时间戳 ──────────┘ └级别┘ └── 调用位置 ──────┘ └── run id ────┘ └代理名┘ └── 消息 ──┘
//
// run id 和代理名都可能不存在，所以这里分开解析。
var logLineRe = regexp.MustCompile(
	`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d+)\s+\[([A-Za-z]+)\]\s+\[([^\]]+)\]\s*(.*)$`)

// run id 是 16 位十六进制
var runIDRe = regexp.MustCompile(`^\[([0-9a-f]{8,})\]\s*`)

// 代理名/访问者名：方括号包起来且不是路径形式
var scopeRe = regexp.MustCompile(`^\[([^\]/\s]+)\]\s*`)

// ANSI 颜色转义序列。
//
// ⚠ 必须剥掉。frp 的 console 日志默认带颜色，而且**即使输出到管道也照样带**
//   （实测：`\x1b[0m\x1b[1;34m2026-09-25 02:25:44.742 [I] [client/service.go:312] ...`）。
//   带着这些前缀，时间戳就不在行首，正则匹配不上，
//   结果就是所有状态推断失效 —— 界面永远停在"正在连接…"。
//
// 我们在生成的配置里已经写了 log.disablePrintColor = true 来从源头避免，
// 但用户可能自己改配置、或用旧版 frp，所以这里必须再兜一层。
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI 去掉一行里的 ANSI 颜色转义序列。
func StripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	return ansiRe.ReplaceAllString(s, "")
}

// levelChar 把 frp 的单字母级别映射成完整级别名。
func levelChar(c string) string {
	switch strings.ToUpper(c) {
	case "T":
		return "trace"
	case "D":
		return "debug"
	case "I":
		return "info"
	case "W":
		return "warn"
	case "E":
		return "error"
	default:
		return strings.ToLower(c)
	}
}

// Rule 是一条翻译规则。
//
// Summary / Detail 里可以用 $1 $2 引用正则的捕获组。
// Drop 为 true 时整条日志被丢弃（用于过滤噪音）。
type Rule struct {
	Re      *regexp.Regexp
	Level   string // 非空时覆盖原有级别
	Summary string
	Detail  string
	Kind    string // 事件码，供上层做状态推断
	Drop    bool
}

// rules 是按顺序匹配的翻译规则表。先匹配到的生效。
//
// 维护说明：frp 上游随时可能新增日志文案，匹配不到时会 fallback 到原文，
// 不会丢信息，只是没有中文。
var rules = []Rule{
	// ---- 噪音过滤 ----
	// ⚠ 重要：GUI 每次轮询 /api/status 等接口，frpc 都会记录一条 http request/response。
	// 不过滤的话日志刷屏，用户根本看不到真正有用的信息。
	{Re: regexp.MustCompile(`^http request: \[/`), Drop: true},
	{Re: regexp.MustCompile(`^http response \[/`), Drop: true},

	// ---- 连接与认证 ----
	{
		Re:      regexp.MustCompile(`^try to connect to server\.\.\.`),
		Summary: "正在连接服务端…",
	},
	{
		Re:      regexp.MustCompile(`^login to server success, get run id \[(.+)\]`),
		Level:   "info",
		Summary: "已连接到服务端，认证通过",
		Detail:  "会话 ID：$1",
		Kind:    KindConnected,
	},
	{
		Re:      regexp.MustCompile(`^login to server failed: (.+)`),
		Level:   "error",
		Summary: "登录服务端失败",
		Detail:  "$1",
		Kind:    KindLoginFailed,
	},
	{
		Re:      regexp.MustCompile(`^connect to server error: (.+)`),
		Level:   "error",
		Summary: "无法连接服务端",
		Detail:  "$1",
		Kind:    KindConnectFailed,
	},
	{
		Re:      regexp.MustCompile(`^admin server listen on (.+)`),
		Summary: "本机管理界面已就绪",
		Detail:  "监听地址 $1",
	},

	// ---- 代理生命周期 ----
	{
		Re:      regexp.MustCompile(`^start proxy success$`),
		Level:   "info",
		Summary: "代理已上线",
		Kind:    KindProxyUp,
	},
	{
		Re:      regexp.MustCompile(`^start error: (.+)`),
		Level:   "error",
		Summary: "代理启动失败",
		Detail:  "$1",
		Kind:    KindProxyError,
	},
	{
		Re:      regexp.MustCompile(`^proxy added: \[(.+)\]`),
		Summary: "新增代理：$1",
	},
	{
		Re:      regexp.MustCompile(`^close proxy`),
		Summary: "代理已关闭",
	},

	// ---- 健康检查（高频，容易刷屏）----
	{
		Re:      regexp.MustCompile(`^do one health check failed: (.+)`),
		Level:   "warn",
		Summary: "健康检查失败",
		Detail:  "$1 —— 该代理在健康检查通过前不会发布出去，请检查本机后端服务",
		Kind:    KindHealthFail,
	},
	{
		Re:      regexp.MustCompile(`^health check status change to success`),
		Level:   "info",
		Summary: "健康检查已恢复正常",
	},
	{
		Re:      regexp.MustCompile(`^health check success`),
		Level:   "info",
		Summary: "健康检查通过",
	},

	// ---- 配置与进程 ----
	{
		Re:      regexp.MustCompile(`^success reload conf$`),
		Summary: "配置已重新加载",
		Detail:  "变更的代理已重启，未改动的代理不受影响",
		Kind:    KindReloaded,
	},
	{
		Re:      regexp.MustCompile(`^start frpc service for config file \[(.+)\]`),
		Summary: "frpc 已启动",
		Detail:  "配置文件 $1",
	},
	{
		Re:      regexp.MustCompile(`^frpc service for config file \[(.+)\] stopped`),
		Level:   "warn",
		Summary: "frpc 已停止",
		Kind:    KindStopped,
	},
	{
		Re:      regexp.MustCompile(`^\[(.+)\] The connection to the server has been terminated`),
		Level:   "warn",
		Summary: "与服务端的连接已断开",
		Detail:  "$1",
	},
}

// ParseLine 解析一行 frp 日志。
//
// 返回 (entry, true) 表示这是一条有效日志；
// 返回 (_, false) 表示这行应该被丢弃（空行、噪音、或完全无法解析）。
func ParseLine(line string, seq int64) (LogEntry, bool) {
	line = strings.TrimRight(line, "\r\n")
	// ⚠ 先剥 ANSI：否则时间戳不在行首，下面所有正则都失配
	line = StripANSI(line)
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return LogEntry{}, false
	}

	m := logLineRe.FindStringSubmatch(trimmed)
	if m == nil {
		// 不是标准格式（例如 frpc 的 panic 堆栈）。
		// 这类内容对排查很重要，不能丢，原样保留。
		return LogEntry{
			Seq:     seq,
			Level:   "info",
			Summary: trimmed,
			Raw:     trimmed,
		}, true
	}

	fullTime := m[1]
	level := levelChar(m[2])
	rest := m[4]

	// 去掉 run id
	rest = runIDRe.ReplaceAllString(rest, "")
	// 去掉代理名（如果有）
	scope := ""
	if sm := scopeRe.FindStringSubmatch(rest); sm != nil {
		// 排除把调用位置误判成代理名的情况（调用位置已在 m[3] 里被剥离）
		candidate := sm[1]
		if !strings.Contains(candidate, ":") && !strings.Contains(candidate, "=") {
			scope = candidate
			rest = rest[len(sm[0]):]
		}
	}
	rest = strings.TrimSpace(rest)

	// 时间只保留 HH:MM:SS
	shortTime := fullTime
	if len(fullTime) >= 19 {
		shortTime = fullTime[11:19]
	}

	entry := LogEntry{
		Seq:     seq,
		Time:    shortTime,
		Level:   level,
		Scope:   scope,
		Summary: rest,
		Detail:  "",
		Raw:     trimmed,
	}

	// 应用翻译规则
	for _, r := range rules {
		if !r.Re.MatchString(rest) {
			continue
		}
		if r.Drop {
			return LogEntry{}, false
		}
		if r.Level != "" {
			entry.Level = r.Level
		}
		entry.Kind = r.Kind
		entry.Summary = r.Re.ReplaceAllString(rest, r.Summary)
		if r.Detail != "" {
			entry.Detail = r.Re.ReplaceAllString(rest, r.Detail)
		}
		return entry, true
	}

	// 没匹配到规则：保留原文（不丢信息）
	return entry, true
}

// ---------------------------------------------------------------- 错误消息翻译

// errorRules 把 frp 返回的英文错误消息翻译成中文。
// 这些消息来自 /api/status 的 err 字段，以及 Store API 的错误响应。
//
// ⚠ 无法做到 100% 覆盖：frp 上游随时可能新增文案。
// 匹配不到时调用方应当回退显示原文，并保留"查看原文"入口。
var errorRules = []Rule{
	{
		Re:      regexp.MustCompile(`^port already used$`),
		Summary: "端口已被占用，换一个对外端口",
	},
	{
		Re:      regexp.MustCompile(`^port not allowed$`),
		Summary: "端口超出了服务端允许的范围，请向管理员确认",
	},
	{
		Re:      regexp.MustCompile(`^type \[http\] not supported when vhost http port is not set$`),
		Summary: "服务端未开启 HTTP 域名路由，无法创建 http 代理",
	},
	{
		Re:      regexp.MustCompile(`^type \[https\] not supported when vhost https port is not set$`),
		Summary: "服务端未开启 HTTPS 域名路由，无法创建 https 代理",
	},
	{
		Re:      regexp.MustCompile(`^subdomain is not supported because this feature is not enabled in server$`),
		Summary: "服务端未启用子域名功能，请联系管理员",
	},
	{
		Re:      regexp.MustCompile(`^'\.' and '\*' are not supported in subdomain$`),
		Summary: "子域名不能包含点或星号，只能写一段（例如 web）",
	},
	{
		Re:      regexp.MustCompile(`^custom domain \[(.+)\] should not belong to subdomain host \[(.+)\]$`),
		Summary: "自定义域名 $1 属于服务端的泛域名根 $2，两者不能同时使用",
	},
	{
		Re:      regexp.MustCompile(`^tcpmux with multiplexer httpconnect not supported because this feature is not enabled in server$`),
		Summary: "服务端未开启 TCPMUX 功能",
	},
	{
		Re:      regexp.MustCompile(`^proxy \[(.+)\] already exists$`),
		Summary: "代理名 $1 已被占用",
	},
	{
		Re:      regexp.MustCompile(`^proxy \[(.+)\] is already in use$`),
		Summary: "代理名 $1 已被占用",
	},
	{
		Re:      regexp.MustCompile(`^proxy type not support$`),
		Summary: "服务端不支持该代理类型",
	},
	{
		Re:      regexp.MustCompile(`^visitor connection of \[(.+)\] user \[(.+)\] not allowed$`),
		Summary: "访问者未被授权：用户 $2 不在 $1 的允许列表里",
	},
	{
		Re:      regexp.MustCompile(`^no proxy info found$`),
		Summary: "找不到该代理的信息",
	},
	{
		Re:      regexp.MustCompile(`^localPath is required$`),
		Summary: "使用 static_file 插件时必须填写本地路径",
	},
	{
		Re:      regexp.MustCompile(`^proxy name is required$`),
		Summary: "代理名不能为空",
	},
	{
		Re:      regexp.MustCompile(`^proxy name in URL must match name in body$`),
		Summary: "URL 里的代理名与请求体里的不一致",
	},
	{
		Re:      regexp.MustCompile(`^exactly one proxy type block is required$`),
		Summary: "必须且只能指定一种代理类型",
	},
	{
		Re:      regexp.MustCompile(`^subdomain and custom domains should not be both empty$`),
		Summary: "子域名和自定义域名不能都为空，至少填一个",
	},
	{
		Re:      regexp.MustCompile(`^invalid proxy type: (.+)$`),
		Summary: "不支持的代理类型：$1",
	},
}

// TranslateError 把 frp 的英文错误消息翻译成中文。
//
// 第二个返回值表示是否命中翻译规则。未命中时调用方应回退显示原文。
func TranslateError(msg string) (string, bool) {
	m := strings.TrimSpace(msg)
	if m == "" {
		return "", true
	}
	for _, r := range errorRules {
		if r.Re.MatchString(m) {
			return r.Re.ReplaceAllString(m, r.Summary), true
		}
	}
	return m, false
}

// TranslateStatus 把 frp 的代理状态值翻译成中文。
//
// frp 后端只产出 6 个值（源码 client/proxy/proxy_wrapper.go:39-44），
// 另外前端为 Store 视图额外补充了 2 个（disabled / waiting）。
// 这是有限集合，可以做到 100% 覆盖。
func TranslateStatus(s string) string {
	switch s {
	case "running":
		return "运行中"
	case "new":
		return "等待启动"
	case "wait start":
		return "正在启动"
	case "start error":
		return "启动失败"
	case "check failed":
		return "健康检查失败"
	case "closed":
		return "已关闭"
	case "disabled":
		return "已停用"
	case "waiting":
		return "等待中"
	default:
		return s
	}
}
