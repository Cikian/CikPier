package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"frpcgui/internal/cfgfile"
	"frpcgui/internal/frpcman"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ================================================================ 关于页面

// AboutInfo 是「关于」页面需要的全部信息。
//
// 大部分来自 runtime/debug.ReadBuildInfo()，也就是说**不需要在构建时注入 ldflags**
// 就能拿到 Go 版本、依赖版本和 VCS 信息。少一个构建脚本要维护。
type AboutInfo struct {
	AppName   string `json:"appName"`
	Version   string `json:"version"`
	BuildTime string `json:"buildTime"`
	Author    string `json:"author"`
	Email     string `json:"email"`
	Website   string `json:"website"`
	GitHub    string `json:"github"`
	Gitee     string `json:"gitee"`
	Tagline   string `json:"tagline"`

	FrpVersion string `json:"frpVersion"`

	GoVersion   string `json:"goVersion"`
	WailsVer    string `json:"wailsVersion"`
	Platform    string `json:"platform"`
	ProcessArch string `json:"processArch"`
	NativeArch  string `json:"nativeArch"`
	Emulated    bool   `json:"emulated"`
	VCSRevision string `json:"vcsRevision"`
	VCSTime     string `json:"vcsTime"`

	License string `json:"license"`

	BaseDir    string `json:"baseDir"`
	ExePath    string `json:"exePath"`
	ConfigPath string `json:"configPath"`

	// 运行时
	Uptime       string `json:"uptime"`
	FrpState     string `json:"frpState"`
	ProxyCount   int    `json:"proxyCount"`
	VisitorCount int    `json:"visitorCount"`
}

// 作者与项目信息。要改就改这里。
const (
	appName     = "CikPier"
	appVersion  = "1.0.0"
	appAuthor   = "Cikian"
	appEmail    = "cikian@126.com"
	appWebsite  = "https://www.cikian.cn"
	appGitHub   = "https://github.com/Cikian"
	appGitee    = "https://gitee.com/Cikian"
	appTagline  = "COURAGE ZENITH JOURNEY"
	appLicense  = "Apache License 2.0"
	frpFallback = "0.71.0"
)

var startTime = time.Now()

// frpVersionOnce 让 frpc -v 只在第一次调用时执行。
var (
	frpVersionOnce sync.Once
	frpVersionVal  string
)

// frpVersion 通过执行 `frpc.exe -v` 得到真实的 frp 版本。
//
// 为什么要真的去问一次：用户完全可能自己换掉 frpc.exe，
// 那这时界面上写死的版本号就是错的。失败时退回内置的默认值。
func (a *App) frpVersion() string {
	frpVersionOnce.Do(func() {
		if !cfgfile.Exists(a.exePath) {
			frpVersionVal = frpFallback + "（未找到 frpc.exe）"
			return
		}
		// ⚠ 必须隐藏控制台窗口。frpc.exe 是**控制台**程序，而本程序是
		//   windowsgui 子系统（自身没有控制台）—— 直接起它，Windows 会
		//   新分配一个控制台窗口，在屏幕上**黑闪一下**。
		//   症状就是"打开关于页时闪过一个黑窗口，然后内容才出来"。
		cmd := exec.Command(a.exePath, "-v")
		frpcman.HideConsole(cmd)
		out, err := cmd.Output()
		if err != nil {
			frpVersionVal = frpFallback
			return
		}
		v := strings.TrimSpace(string(out))
		// 输出可能有多行，取第一行里像版本号的那段
		if i := strings.IndexByte(v, '\n'); i >= 0 {
			v = v[:i]
		}
		v = strings.TrimSpace(v)
		if v == "" {
			v = frpFallback
		}
		frpVersionVal = v
	})
	return frpVersionVal
}

// GetAboutInfo 汇总「关于」页面要显示的信息。
func (a *App) GetAboutInfo() AboutInfo {
	info := AboutInfo{
		AppName:    appName,
		Version:    appVersion,
		Author:     appAuthor,
		Email:      appEmail,
		Website:    appWebsite,
		GitHub:     appGitHub,
		Gitee:      appGitee,
		Tagline:    appTagline,
		License:    appLicense,
		FrpVersion: a.frpVersion(),

		GoVersion:   runtime.Version(),
		Platform:    fmt.Sprintf("%s / %s", runtime.GOOS, runtime.GOARCH),
		BaseDir:     a.baseDir,
		ExePath:     a.exePath,
		ConfigPath:  a.cfgPath,
		Uptime:      humanDuration(time.Since(startTime)),
		VCSRevision: "-",
		VCSTime:     "-",
	}

	// 构建时间用 exe 的修改时间：这是最贴近"这个文件是什么时候编出来的"的证据
	if exe, err := os.Executable(); err == nil {
		if st, err := os.Stat(exe); err == nil {
			info.BuildTime = st.ModTime().Format(time.DateTime)
		}
	}

	// 从构建信息里挖 Go 模块依赖和 VCS 信息
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.GoVersion != "" {
			info.GoVersion = bi.GoVersion
		}
		for _, dep := range bi.Deps {
			if dep == nil {
				continue
			}
			switch dep.Path {
			case "github.com/wailsapp/wails/v2":
				info.WailsVer = dep.Version
			}
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if len(s.Value) >= 7 {
					info.VCSRevision = s.Value[:7]
				} else {
					info.VCSRevision = s.Value
				}
			case "vcs.time":
				info.VCSTime = s.Value
			}
		}
	}
	// 没走 git 构建时（比如直接 wails build）VCS 信息是空的，
	// 用构建时间来兜底，界面上不留 "-"
	if info.VCSRevision == "-" && info.BuildTime != "" {
		info.VCSRevision = "本地构建"
	}
	if info.VCSTime == "-" {
		info.VCSTime = info.BuildTime
	}
	if info.WailsVer == "" {
		info.WailsVer = "-"
	}

	// 架构
	arch := detectArch()
	info.ProcessArch = arch.Process
	info.NativeArch = arch.Native
	info.Emulated = arch.Emulated

	// 运行状态
	state, _ := a.man.State()
	info.FrpState = translateConnState(state)
	if a.externalRunning() {
		info.FrpState = "外部进程运行中"
	}
	if views, err := a.ListProxies(); err == nil {
		info.ProxyCount = len(views)
	}
	if vis, err := a.ListVisitors(); err == nil {
		info.VisitorCount = len(vis)
	}

	return info
}

// humanDuration 把时长写成人话，例如 "2 小时 13 分"。
func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d 秒", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d 分 %d 秒", int(d.Minutes()), int(d.Seconds())%60)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h < 24 {
		return fmt.Sprintf("%d 小时 %d 分", h, m)
	}
	return fmt.Sprintf("%d 天 %d 小时", h/24, h%24)
}

// OpenExternal 在系统默认程序里打开一个链接（邮箱、网址都行）。
//
// ⚠ 只允许 http/https/mailto，避免这里变成一个"执行任意东西"的口子。
func (a *App) OpenExternal(url string) error {
	u := strings.TrimSpace(url)
	if u == "" {
		return fmt.Errorf("链接为空")
	}
	lower := strings.ToLower(u)
	if !strings.HasPrefix(lower, "http://") &&
		!strings.HasPrefix(lower, "https://") &&
		!strings.HasPrefix(lower, "mailto:") {
		return fmt.Errorf("只支持 http / https / mailto 链接")
	}
	if a.ctx == nil {
		return fmt.Errorf("界面还没准备好")
	}
	wruntime.BrowserOpenURL(a.ctx, u)
	return nil
}
