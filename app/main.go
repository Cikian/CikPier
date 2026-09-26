package main

import (
	"context"
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/src
var assets embed.FS

func main() {
	debugLog("main: 启动")
	app := NewApp()
	debugLog("main: NewApp 完成")

	// 调试开关：用来隔离"托盘导致的退出"这类问题
	if os.Getenv("FRPCGUI_NOTRAY") == "1" {
		debugLog("main: 按 FRPCGUI_NOTRAY 禁用托盘")
		app.tray = nil
	}

	err := wails.Run(app.wailsOptions())
	debugLog("main: wails.Run 返回 err=%v", err)
	if err != nil {
		println("启动失败:", err.Error())
	}
}

func (a *App) wailsOptions() *options.App {
	return &options.App{
		Title:     "CikPier",
		Width:     1180,
		Height:    760,
		MinWidth:  1040,
		MinHeight: 640,

		// 设计稿的页面底色，避免窗口初始化时闪白/闪黑
		BackgroundColour: &options.RGBA{R: 247, G: 249, B: 251, A: 1},

		AssetServer: &assetserver.Options{
			Assets: assets,
		},

		// ⚠ 严格单例：双击第二次时不再开新窗口，而是把已有窗口拉到前台。
		// 原因是 GUI 与 frpc 进程是绑定的，两个实例会抢同一个 frpc 和配置文件。
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "frpc-gui-cikian-025",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				debugLog("main: 检测到第二个实例，激活已有窗口")
				a.ShowWindow()
			},
		},

		OnStartup:     a.startup,
		OnBeforeClose: a.beforeClose,
		OnShutdown:    a.shutdown,

		Bind: []interface{}{a},

		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	}
}

// 保留 context 引用，便于后续扩展
var _ = context.Background
