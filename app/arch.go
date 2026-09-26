//go:build windows

package main

import (
	"fmt"
	"runtime"

	"golang.org/x/sys/windows"
)

// 为什么需要这个文件：Windows 11 ARM64 能用模拟层跑 x64 程序，
// 所以用户很可能把 x64 包装到 ARM 电脑上，而且**能启动、能连上**，
// 表面上一切正常 —— 但 WebView2 在"ARM64 系统 + x64 目标"这个组合下有个
// 至今未修的 bug：消息一大就死锁甚至崩溃
// （MicrosoftEdge/WebView2Feedback#4589，open）。
//
// 我们没法在编译期知道用户会用在哪台机器上，但**运行期可以判断自己在不在模拟层里**，
// 然后直接告诉用户"你装错包了"。这比让他自己去猜架构靠谱得多。

const (
	machineUnknown = 0x0000
	machineX86     = 0x014c
	machineAMD64   = 0x8664
	machineARM64   = 0xAA64
)

func machineName(m uint16) string {
	switch m {
	case machineUnknown:
		return "unknown"
	case machineX86:
		return "x86"
	case machineAMD64:
		return "amd64"
	case machineARM64:
		return "arm64"
	default:
		return fmt.Sprintf("0x%04X", m)
	}
}

// ArchInfo 描述"这个进程是什么架构"和"这台机器是什么架构"。
type ArchInfo struct {
	Process  string `json:"process"`  // 程序自身的架构
	Native   string `json:"native"`   // 机器本身的架构
	Emulated bool   `json:"emulated"` // 是不是在模拟层里跑
	Hint     string `json:"hint"`     // 需要用户处理时给出的中文提示
}

// detectArch 判断当前进程是否跑在模拟层里。
//
// ⚠ IsWow64Process2 有个反直觉的地方：**没有跑在模拟层里时，
//
//	processMachine 返回的是 IMAGE_FILE_MACHINE_UNKNOWN，而不是自身架构。**
//	所以不能拿 processMachine 当"我的架构"，要分两种情况处理。
func detectArch() ArchInfo {
	info := ArchInfo{Process: runtime.GOARCH, Native: runtime.GOARCH}

	var proc, native uint16
	if err := windows.IsWow64Process2(windows.CurrentProcess(), &proc, &native); err != nil {
		// Win7 / Win8 没有这个 API，退回到"架构就是编译时的目标架构"
		return info
	}

	info.Native = machineName(native)

	if proc == machineUnknown {
		// 没在模拟层里 —— 自身架构就是编译目标架构
		info.Process = runtime.GOARCH
		info.Emulated = false
	} else {
		// 在模拟层里跑：proc 是程序架构，native 是机器真实架构
		info.Process = machineName(proc)
		info.Emulated = true
	}

	if info.Emulated && info.Native == "arm64" {
		info.Hint = "当前运行的是 " + info.Process + " 版本，但它正跑在 ARM 电脑的模拟层里。" +
			"Windows 虽然能模拟，但 WebView2 在「ARM 系统 + x64 程序」这个组合下" +
			"有已知的崩溃问题（消息一大会死锁），而且性能有损失。" +
			"建议换成文件名带 -arm64 的那个版本。"
	}
	return info
}
