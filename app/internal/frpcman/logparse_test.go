package frpcman

import "testing"

func TestParseRealFrpLines(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantLvl  string
		wantKind string
		wantTime string
	}{
		{
			name:     "登录成功",
			line:     `2026-09-25 02:24:44.510 [I] [client/service.go:332] [8b33842ddee7ad0b] login to server success, get run id [8b33842ddee7ad0b]`,
			wantLvl:  "info",
			wantKind: KindConnected,
			wantTime: "02:24:44",
		},
		{
			name:     "连接失败",
			line:     `2026-09-25 02:25:10.442 [W] [client/service.go:323] connect to server error: dial tcp 127.0.0.1:17999: connectex: No connection could be made because the target machine actively refused it.`,
			wantLvl:  "error",
			wantKind: KindConnectFailed,
			wantTime: "02:25:10",
		},
		{
			name:     "管理接口就绪",
			line:     `2026-09-25 02:24:44.503 [I] [client/service.go:254] admin server listen on 127.0.0.1:17400`,
			wantLvl:  "info",
			wantTime: "02:24:44",
		},
		{
			name:     "代理上线（带代理名）",
			line:     `2026-09-24 20:29:29.520 [I] [client/control.go:174] [0ab2ace915055641] [web] start proxy success`,
			wantLvl:  "info",
			wantKind: KindProxyUp,
			wantTime: "20:29:29",
		},
		{
			name:     "健康检查失败",
			line:     `2026-09-24 20:30:39.421 [W] [health/health.go:128] [0ab2ace915055641] [tcp-demo] do one health check failed: dial tcp 127.0.0.1:22: connectex: refused`,
			wantLvl:  "warn",
			wantKind: KindHealthFail,
			wantTime: "20:30:39",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, ok := ParseLine(c.line, 1)
			if !ok {
				t.Fatalf("行被丢弃了: %q", c.line)
			}
			t.Logf("time=%q level=%q scope=%q kind=%q\n  summary=%q\n  raw=%q", e.Time, e.Level, e.Scope, e.Kind, e.Summary, e.Raw)
			if e.Time != c.wantTime {
				t.Errorf("Time = %q, 期望 %q", e.Time, c.wantTime)
			}
			if e.Level != c.wantLvl {
				t.Errorf("Level = %q, 期望 %q", e.Level, c.wantLvl)
			}
			if e.Kind != c.wantKind {
				t.Errorf("Kind = %q, 期望 %q", e.Kind, c.wantKind)
			}
		})
	}
}

// TestParseStripsANSI 是最重要的一条回归测试。
//
// frp 的 console 日志默认带颜色，而且输出到管道时**依然带**颜色
// （实测前缀 `\x1b[0m\x1b[1;34m`）。如果不剥掉，
// 时间戳就不在行首，正则全部失配，界面会永远停在"正在连接…"。
func TestParseStripsANSI(t *testing.T) {
	const colored = "\x1b[0m\x1b[1;34m2026-09-25 02:25:44.742 [I] [client/service.go:312] try to connect to server..."
	e, ok := ParseLine(colored, 1)
	if !ok {
		t.Fatal("带颜色的行被丢弃了")
	}
	if e.Time != "02:25:44" {
		t.Errorf("Time = %q，期望 02:25:44（说明 ANSI 没被剥掉）", e.Time)
	}
	if e.Raw != "2026-09-25 02:25:44.742 [I] [client/service.go:312] try to connect to server..." {
		t.Errorf("Raw 里仍残留转义字符: %q", e.Raw)
	}
	if e.Summary != "正在连接服务端…" {
		t.Errorf("Summary = %q，期望「正在连接服务端…」", e.Summary)
	}

	// 颜色前缀 + 连接失败，必须能推断出 connect_failed
	const coloredErr = "\x1b[0m\x1b[1;33m2026-09-25 02:25:44.743 [W] [client/service.go:323] connect to server error: dial tcp 127.0.0.1:17999: connectex: refused"
	e2, _ := ParseLine(coloredErr, 2)
	if e2.Kind != KindConnectFailed {
		t.Errorf("Kind = %q，期望 %q", e2.Kind, KindConnectFailed)
	}
}

func TestStripANSI(t *testing.T) {
	in := "\x1b[0m\x1b[1;34mhello\x1b[0m world"
	if got := StripANSI(in); got != "hello world" {
		t.Errorf("StripANSI = %q", got)
	}
	// 不含转义时应原样返回（快路径）
	plain := "no escapes here"
	if got := StripANSI(plain); got != plain {
		t.Errorf("StripANSI 改动了纯文本: %q", got)
	}
}

// TestParseDropsPollNoise 界面轮询会引发大量 http 日志，必须被丢掉。
func TestParseDropsPollNoise(t *testing.T) {
	lines := []string{
		`2026-09-25 02:26:00.001 [I] [http/http.go:41] http request: [/api/status]`,
		`2026-09-25 02:26:00.002 [I] [http/http.go:41] http response [/api/status]`,
	}
	for _, l := range lines {
		if _, ok := ParseLine(l, 1); ok {
			t.Errorf("轮询噪音没有被丢弃: %q", l)
		}
	}
}
