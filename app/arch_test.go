//go:build windows

package main

import (
	"runtime"
	"testing"
)

// TestDetectArchInvariants 校验架构探测的几条不变量。
//
// 重点验证 IsWow64Process2 那个反直觉的行为：
// **不在模拟层里时 processMachine 返回 IMAGE_FILE_MACHINE_UNKNOWN**，
// 如果直接拿它当"我的架构"，真机上会得到 "unknown"。
// 这条测试在 x64 机器上跑时会走"非模拟"分支，正好覆盖那个坑。
func TestDetectArchInvariants(t *testing.T) {
	info := detectArch()

	if info.Process == "" || info.Native == "" {
		t.Fatalf("架构字段不该为空：%+v", info)
	}
	if info.Process != runtime.GOARCH {
		t.Errorf("Process 应该等于编译目标 %s，实际 %s", runtime.GOARCH, info.Process)
	}

	if !info.Emulated {
		// 真机原生运行：不该有提示，Process 就是编译目标
		if info.Hint != "" {
			t.Errorf("原生运行时不该给换包提示，实际：%s", info.Hint)
		}
	} else {
		// 在模拟层里：程序架构和机器架构必然不同
		if info.Process == info.Native {
			t.Errorf("报告为模拟运行时，两个架构不该相同：%+v", info)
		}
	}

	// Hint 只在「ARM 机器 + 非 arm64 进程」这一种情况下出现
	wantHint := info.Emulated && info.Native == "arm64"
	if wantHint != (info.Hint != "") {
		t.Errorf("Hint 出现的条件不对：emulated=%v native=%s hint=%q",
			info.Emulated, info.Native, info.Hint)
	}

	t.Logf("进程=%s 机器=%s 模拟=%v", info.Process, info.Native, info.Emulated)
}

func TestMachineName(t *testing.T) {
	cases := map[uint16]string{
		0x0000: "unknown",
		0x014c: "x86",
		0x8664: "amd64",
		0xAA64: "arm64",
		0x1234: "0x1234",
	}
	for in, want := range cases {
		if got := machineName(in); got != want {
			t.Errorf("machineName(0x%04X) = %q，期望 %q", in, got, want)
		}
	}
}
