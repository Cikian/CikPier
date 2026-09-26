package cfgfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDefaultUsesConsoleLog 是一条回归测试。
//
// 曾经 Default() 里写的是 "frpc.log"，于是向导生成的配置让 frpc 把日志写进文件，
// 而图形界面是读 frpc 的 stdout 的 —— 结果界面永远停在"正在连接…"，
// 用户看着一切正常却怎么都连不上。
func TestDefaultUsesConsoleLog(t *testing.T) {
	d := Default()
	if d.LogTo != ConsoleLogTarget {
		t.Fatalf("Default().LogTo = %q，必须是 %q", d.LogTo, ConsoleLogTarget)
	}

	out := Render(d)
	if !strings.Contains(out, `log.to = "console"`) {
		t.Errorf("生成的配置里没有 log.to = \"console\"：\n%s", out)
	}
	if strings.Contains(out, `log.to = "frpc.log"`) {
		t.Error("生成的配置把日志写到了文件，图形界面将读不到任何日志")
	}
	// 颜色必须关掉，否则日志行会带 ANSI 前缀（详见 frpcman.StripANSI）
	if !strings.Contains(out, "log.disablePrintColor = true") {
		t.Error("生成的配置里缺少 log.disablePrintColor = true")
	}
	// store 必须用点号写法，写成 [store] 表头会把后面的键全吃进去
	if strings.Contains(out, "[store]") {
		t.Error("store 不能写成表头形式")
	}
	if !strings.Contains(out, `store.path = "`) {
		t.Error("缺少 store.path")
	}
}

func TestNormalizeLogTo(t *testing.T) {
	// 指向文件 → 应该被改成 console
	c := Default()
	c.LogTo = "frpc.log"
	old, changed := NormalizeLogTo(c)
	if !changed || old != "frpc.log" || c.LogTo != ConsoleLogTarget {
		t.Fatalf("修正失败: old=%q changed=%v now=%q", old, changed, c.LogTo)
	}

	// 已经正确 → 不应该改动
	c2 := Default()
	if old, changed := NormalizeLogTo(c2); changed || old != ConsoleLogTarget {
		t.Fatalf("不该有改动: old=%q changed=%v", old, changed)
	}

	// 大小写和空格都要认
	c3 := Default()
	c3.LogTo = "  Console "
	if _, changed := NormalizeLogTo(c3); changed {
		t.Error("大小写/空格差异不该触发修正")
	}

	// nil 不能 panic
	if _, changed := NormalizeLogTo(nil); changed {
		t.Error("nil 不该报告改动")
	}
}

// TestSaveThenLoadRoundTrip 确认保存后能原样读回来（尤其是 serverPort）。
func TestSaveThenLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frpc.toml")

	c := Default()
	c.ServerAddr = "frp.example.com"
	c.ServerPort = 7500 // 非默认端口，必须被保留
	c.User = "zhangsan"
	c.AuthToken = "secret-token"
	c.WebPass = "panelpass"

	if err := Save(path, c); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if got.ServerPort != 7500 {
		t.Errorf("serverPort 没被保留: %d", got.ServerPort)
	}
	if got.ServerAddr != "frp.example.com" || got.User != "zhangsan" {
		t.Errorf("连接信息读回来不对: %+v", got)
	}
	if got.AuthToken != "secret-token" {
		t.Errorf("token 读回来不对: %q", got.AuthToken)
	}
	if got.WebPort != DefaultWebPort || got.WebPass != "panelpass" {
		t.Errorf("webServer 读回来不对: port=%d pass=%q", got.WebPort, got.WebPass)
	}
	if got.LogTo != ConsoleLogTarget {
		t.Errorf("LogTo = %q", got.LogTo)
	}
}

// TestSaveCreatesBackup 保存前必须留下备份，用户改坏了能回滚。
func TestSaveCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frpc.toml")

	c := Default()
	c.ServerAddr = "first.example.com"
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	if Exists(path + ".bak") {
		t.Error("第一次保存不该产生备份")
	}

	c.ServerAddr = "second.example.com"
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	if !Exists(path + ".bak") {
		t.Fatal("第二次保存应该产生 frpc.toml.bak")
	}
	bak, _ := os.ReadFile(path + ".bak")
	if !strings.Contains(string(bak), "first.example.com") {
		t.Error("备份内容不是上一次的配置")
	}
}

// TestFileProxyDefsParsesFlatLayout 确认能读懂配置文件里的扁平代理定义。
func TestFileProxyDefsParsesFlatLayout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frpc.toml")
	content := `serverAddr = "x"
serverPort = 7000
auth.token = "t"

[[proxies]]
name = "web"
type = "http"
localPort = 8080
subdomain = "web"

[[proxies]]
name = "db"
type = "tcp"
localPort = 3306
remotePort = 20001

[[visitors]]
name = "v1"
type = "stcp"
serverName = "db"
secretKey = "k"
bindPort = 13306
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	names, err := FileProxyNames(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "web" || names[1] != "db" {
		t.Fatalf("代理名列表不对: %v", names)
	}

	defs, err := FileProxyDefs(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 2 {
		t.Fatalf("代理定义数量不对: %d", len(defs))
	}
	if defs[0]["subdomain"] != "web" {
		t.Errorf("subdomain 没解析出来: %v", defs[0])
	}
	if _, ok := defs[1]["remotePort"]; !ok {
		t.Errorf("remotePort 没解析出来: %v", defs[1])
	}

	// 访问者走的是另一条路径
	rf, err := readRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rf.Visitors) != 1 || rf.Visitors[0]["serverName"] != "db" {
		t.Fatalf("访问者没解析出来: %v", rf.Visitors)
	}
}

// TestLoadMissingFileReturnsDefaults 配置文件不存在时不该报错，要返回默认值。
func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "not-there.toml"))
	if err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if c.ServerPort != DefaultServerPort || c.WebPort != DefaultWebPort {
		t.Fatalf("默认值不对: %+v", c)
	}
	if c.LogTo != ConsoleLogTarget {
		t.Fatalf("LogTo 默认值不对: %q", c.LogTo)
	}
	if c.LoginFailExit {
		t.Fatal("LoginFailExit 默认必须是 false")
	}
}
