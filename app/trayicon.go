package main

import (
	"frpcgui/internal/icon"
)

// 托盘图标：底板颜色表达连接状态，白色箭头是品牌标识。
//
// 托盘上只有 16px，靠箭头的颜色区分状态太吃力，
// 所以让整块底板承担状态，形状负责"一眼认出是谁"。
var trayColors = map[string]icon.RGB{
	"connected": icon.StateConnected,
	"starting":  icon.StateStarting,
	"stopped":   icon.StateStopped,
	"failed":    icon.StateFailed,
}

// trayIcon 生成指定状态的托盘图标。
// Windows 托盘在高分屏下会取 32px 那一张，所以两个尺寸都给。
func trayIcon(state string) []byte {
	c, okc := trayColors[state]
	if !okc {
		c = trayColors["stopped"]
	}
	return icon.ICOFor(icon.StateScheme(c), 16, 32)
}
