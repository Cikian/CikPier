package main

// 临时诊断工具：设置 FRPCGUI_DEBUG=1 时把关键节点写入 gui-debug.log。
// 排查完即可删除本文件（不影响其它代码）。

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var debugOn = os.Getenv("FRPCGUI_DEBUG") == "1"

func debugLog(format string, args ...any) {
	if !debugOn {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(filepath.Dir(exe), "gui-debug.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
}
