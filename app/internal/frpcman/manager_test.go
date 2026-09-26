package frpcman

import (
	"os/exec"
	"testing"
)

// TestHideConsoleSetsNoWindow 守住"起 frpc 不弹黑窗口"这件事。
//
// 回归背景：`about.go` 为了拿 frp 版本号跑了一次 `frpc.exe -v`，但**忘了调这个函数**，
// 结果每次打开「关于」页都会黑窗口闪一下。
//
// 原因：本程序是 windowsgui 子系统（自身没有控制台），而 frpc.exe 是**控制台**程序；
// 用 exec.Command 直接起它，Windows 会为它新分配一个控制台窗口。
// 那两个常量是"不要分配控制台"的开关。
//
// 以后**任何新增的 frpc.exe 调用点**都应该有对应的这一条断言。
func TestHideConsoleSetsNoWindow(t *testing.T) {
	const createNoWindow = 0x08000000 // CREATE_NO_WINDOW

	cmd := exec.Command("frpc.exe", "-v")
	if cmd.SysProcAttr != nil {
		t.Fatalf("前置条件不成立：新建的 Cmd 上 SysProcAttr 应该是 nil，实际 %+v", cmd.SysProcAttr)
	}

	HideConsole(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("HideConsole 应该给 Cmd 设置 SysProcAttr")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow 应该为 true —— 否则控制台窗口会显示出来")
	}
	if cmd.SysProcAttr.CreationFlags != createNoWindow {
		t.Errorf("CreationFlags 应该是 CREATE_NO_WINDOW(0x%X)，实际 0x%X",
			createNoWindow, cmd.SysProcAttr.CreationFlags)
	}
}
