package main

import (
	"sync"

	"frpcgui/internal/frpcman"

	"fyne.io/systray"
)

// trayUI 是系统托盘。
//
// ⚠ Wails v2 没有内置托盘，所以这里用 fyne.io/systray 自己在
// 独立线程上跑一套消息循环（不占用 Wails 的主循环）。
//
// 托盘承担三件事：
//  1. 让程序"关掉窗口但不退出"时有落脚点（否则用户找不到它）
//  2. 一眼看出连接状态（图标颜色 + 悬浮提示）
//  3. 勾选开机自启（需求 #8）
type trayUI struct {
	app *App

	mu    sync.Mutex
	ready bool

	mStatus  *systray.MenuItem
	mStart   *systray.MenuItem
	mStop    *systray.MenuItem
	mRestart *systray.MenuItem
	mAuto    *systray.MenuItem
	mOpen    *systray.MenuItem

	curIcon string
}

func newTrayUI(a *App) *trayUI {
	return &trayUI{app: a, curIcon: ""}
}

// start 在后台启动托盘。systray.Run 会阻塞，所以必须放在 goroutine 里。
func (t *trayUI) start() {
	go systray.Run(t.onReady, func() {})
}

func (t *trayUI) onReady() {
	systray.SetTitle("CikPier")

	t.mStatus = systray.AddMenuItem("状态：未运行", "当前 frp 状态")
	t.mStatus.Disable()

	systray.AddSeparator()
	t.mOpen = systray.AddMenuItem("打开主界面", "显示程序窗口")
	t.mStart = systray.AddMenuItem("启动 frp", "连接服务端并上线所有代理")
	t.mStop = systray.AddMenuItem("停止 frp", "断开连接，代理全部下线")
	t.mRestart = systray.AddMenuItem("重启 frp", "重新加载配置并重连")

	systray.AddSeparator()
	t.mAuto = systray.AddMenuItemCheckbox("开机自动启动", "登录 Windows 后自动启动本程序", t.app.GetAutoStart())

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出程序", "停止 frp 并完全退出")

	// 左键单击托盘图标 = 打开主界面（和大多数 Windows 程序一致）
	systray.SetOnTapped(func() { t.app.ShowWindow() })

	t.mu.Lock()
	t.ready = true
	t.mu.Unlock()

	t.syncAutoStart()
	// 用真实状态初始化，而不是假定"未运行"：
	// 托盘可能比 frpc 晚就绪，这时进程已经在跑了。
	s, _ := t.app.man.State()
	t.applyState(s)

	go t.loop(mQuit)
}

// loop 处理菜单点击。systray 只提供 channel，所以要自己起一个循环。
func (t *trayUI) loop(mQuit *systray.MenuItem) {
	for {
		select {
		case <-t.mOpen.ClickedCh:
			t.app.ShowWindow()

		case <-t.mStart.ClickedCh:
			go func() {
				if err := t.app.StartFrp(); err != nil {
					t.app.TrayToast("启动失败", err.Error(), "err")
				}
			}()

		case <-t.mStop.ClickedCh:
			go func() {
				if err := t.app.StopFrp(); err != nil {
					t.app.TrayToast("停止失败", err.Error(), "err")
				}
			}()

		case <-t.mRestart.ClickedCh:
			go func() {
				if err := t.app.RestartFrp(); err != nil {
					t.app.TrayToast("重启失败", err.Error(), "err")
				}
			}()

		case <-t.mAuto.ClickedCh:
			// 不依赖 systray 的自动勾选，直接读注册表真实状态再决定，
			// 这样失败时菜单不会和现实不一致。
			on := !t.app.GetAutoStart()
			if err := t.app.SetAutoStart(on); err != nil {
				t.syncAutoStart()
				t.app.TrayToast("设置失败", err.Error(), "err")
				continue
			}
			t.syncAutoStart()
			if on {
				t.app.TrayToast("已开启开机自启", "下次登录 Windows 时会自动启动", "ok")
			} else {
				t.app.TrayToast("已关闭开机自启", "", "ok")
			}

		case <-mQuit.ClickedCh:
			t.app.QuitApp()
			return
		}
	}
}

// syncAutoStart 把菜单勾选状态同步为注册表里的真实值。
func (t *trayUI) syncAutoStart() {
	if t.mAuto == nil {
		return
	}
	if t.app.GetAutoStart() {
		t.mAuto.Check()
	} else {
		t.mAuto.Uncheck()
	}
}

// update 由 app.go 的状态变化回调驱动。
func (t *trayUI) update(s frpcman.State, msg string) {
	t.applyState(s)
}

// applyState 更新图标颜色、悬浮提示和菜单可用性。
func (t *trayUI) applyState(s frpcman.State) {
	t.mu.Lock()
	if !t.ready {
		t.mu.Unlock()
		return
	}
	icon, text, running := trayAppearance(s)

	if icon != t.curIcon {
		systray.SetIcon(trayIcon(icon))
		t.curIcon = icon
	}
	systray.SetTooltip("CikPier · " + text)

	if t.mStatus != nil {
		t.mStatus.SetTitle("状态：" + text)
	}
	if t.mStart != nil && t.mStop != nil && t.mRestart != nil {
		if running {
			t.mStart.Disable()
			t.mStop.Enable()
			t.mRestart.Enable()
		} else {
			t.mStart.Enable()
			t.mStop.Disable()
			t.mRestart.Disable()
		}
	}
	t.mu.Unlock()
}

// trayAppearance 把连接状态映射成图标配色、中文说明和"是否在运行"。
func trayAppearance(s frpcman.State) (icon, text string, running bool) {
	switch s {
	case frpcman.StateConnected:
		return "connected", "已连接到服务端", true
	case frpcman.StateStarting:
		return "starting", "正在连接…", true
	case frpcman.StateFailed:
		return "failed", "连接失败", true
	default:
		return "stopped", "未运行", false
	}
}

// stop 关闭托盘图标。
func (t *trayUI) stop() {
	t.mu.Lock()
	ready := t.ready
	t.ready = false
	t.mu.Unlock()
	if ready {
		systray.Quit()
	}
}
