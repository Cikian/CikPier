package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"frpcgui/internal/cfgfile"
	"frpcgui/internal/frpcapi"
	"frpcgui/internal/frpcman"
)

// 端到端集成测试：真的起一个 frps 和一个 frpc，然后走 App 的方法。
//
// 为什么需要它：界面用的是"Store API + 嵌套类型块"这套结构，
// 结构写错时界面看起来一切正常，但保存会失败或保存出错误的代理。
// 只有对着真正的 frpc 跑一遍才能确认契约是对的。
//
// 需要提供二进制目录：
//
//	$env:FRPCGUI_BIN='D:\Tools\frp_0.71.0\frp-bins\win-amd64'; go test -run IT -v
//
// 未设置 FRPCGUI_BIN 时自动跳过，所以 go test ./... 不受影响。
func TestITAppLifecycle(t *testing.T) {
	binDir := os.Getenv("FRPCGUI_BIN")
	if binDir == "" {
		t.Skip("未设置 FRPCGUI_BIN，跳过集成测试")
	}
	frpsExe := filepath.Join(binDir, "frps.exe")
	frpcExe := filepath.Join(binDir, "frpc.exe")
	for _, p := range []string{frpsExe, frpcExe} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("找不到 %s", p)
		}
	}

	const (
		ctlPort   = 17500
		vhostPort = 18080
		adminPort = 17400
		token     = "it-token-please-ignore"
	)

	dir := t.TempDir()
	// 把 frpc.exe 复制到工作目录：App 的约定就是"exe 和配置在同一个目录"
	if err := copyFile(frpcExe, filepath.Join(dir, "frpc.exe")); err != nil {
		t.Fatalf("复制 frpc.exe 失败: %v", err)
	}

	// ---- 服务端 ----
	// ⚠ subDomainHost 必须配：否则服务端不认 subdomain 形式的 HTTP 代理，
	//   会返回 "subdomain is not supported because this feature is not enabled in server"
	frpsCfg := fmt.Sprintf("bindPort = %d\nvhostHTTPPort = %d\nsubDomainHost = \"frp.test\"\nauth.method = \"token\"\nauth.token = %q\nlog.to = \"console\"\n",
		ctlPort, vhostPort, token)
	frpsCfgPath := filepath.Join(dir, "frps.toml")
	if err := os.WriteFile(frpsCfgPath, []byte(frpsCfg), 0o600); err != nil {
		t.Fatal(err)
	}
	frps := exec.Command(frpsExe, "-c", frpsCfgPath)
	if err := frps.Start(); err != nil {
		t.Fatalf("启动 frps 失败: %v", err)
	}
	defer func() { _ = frps.Process.Kill() }()
	waitPort(t, ctlPort, 8*time.Second, true)

	// ---- 客户端配置（含一条"手写在文件里"的代理，用来测迁移）----
	frpcCfg := fmt.Sprintf(`serverAddr = "127.0.0.1"
serverPort = %d
user = "ittest"
clientID = "it-01"

auth.method = "token"
auth.token = %q

loginFailExit = false

webServer.addr = "127.0.0.1"
webServer.port = %d
webServer.user = "frpcgui"
webServer.password = "itpanelpass"
webServer.pprofEnable = false

store.path = "frpc-store.json"

log.to = "console"
log.level = "info"
log.maxDays = 1

[[proxies]]
name = "from-file"
type = "tcp"
localIP = "127.0.0.1"
localPort = 19999
remotePort = 19998
`, ctlPort, token, adminPort)
	cfgPath := filepath.Join(dir, "frpc.toml")
	if err := os.WriteFile(cfgPath, []byte(frpcCfg), 0o600); err != nil {
		t.Fatal(err)
	}

	// ---- 组装 App（同包，可直接访问未导出字段）----
	a := &App{
		baseDir:  dir,
		cfgPath:  cfgPath,
		exePath:  filepath.Join(dir, "frpc.exe"),
		settings: Settings{},
	}
	a.man = frpcman.New(a.exePath, a.cfgPath)
	cfg, err := cfgfile.Load(cfgPath)
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	a.cfg = cfg
	a.apiFn = func() *frpcapi.Client {
		return frpcapi.New(a.cfg.WebAddr, a.cfg.WebPort, a.cfg.WebUser, a.cfg.WebPass)
	}

	if err := a.StartFrp(); err != nil {
		t.Fatalf("启动 frpc 失败: %v", err)
	}
	defer func() { _ = a.StopFrp() }()

	// 等登录成功（状态是从日志推断的，所以要等一会儿）
	waitState(t, a, frpcman.StateConnected, 10*time.Second)

	// ---- 1. 配置文件里的代理应当被识别出来，且标记为 fromFile ----
	views, err := a.ListProxies()
	if err != nil {
		t.Fatalf("ListProxies 失败: %v", err)
	}
	if len(views) != 1 || views[0].Name != "from-file" || !views[0].FromFile {
		t.Fatalf("期望只看到配置文件里的 from-file，实际: %+v", views)
	}
	t.Logf("✓ 识别到配置文件里的代理：%s", views[0].Name)

	// ---- 2. 新建一条 TCP 代理（结构与前端 collectProxyDef 的输出完全一致）----
	tcpDef := frpcapi.Definition{
		"name": "it-tcp",
		"type": "tcp",
		"tcp": map[string]any{
			"name": "it-tcp", "type": "tcp",
			"localIP": "127.0.0.1", "localPort": float64(18081),
			"remotePort": float64(18082),
			"transport":  map[string]any{"useEncryption": true},
		},
	}
	if err := a.SaveProxy(tcpDef, false); err != nil {
		t.Fatalf("SaveProxy(新建 tcp) 失败: %v", err)
	}
	t.Log("✓ 新建 TCP 代理成功")

	// ---- 3. 新建一条 HTTP 代理（子域名形式）----
	httpDef := frpcapi.Definition{
		"name": "it-http",
		"type": "http",
		"http": map[string]any{
			"name": "it-http", "type": "http",
			"localIP": "127.0.0.1", "localPort": float64(18083),
			"subdomain": "itweb",
		},
	}
	if err := a.SaveProxy(httpDef, false); err != nil {
		t.Fatalf("SaveProxy(新建 http) 失败: %v", err)
	}
	t.Log("✓ 新建 HTTP 代理成功")

	// ---- 4. 两条都应当在列表里，并且是 running ----
	views = waitProxy(t, a, "it-tcp", "running", 10*time.Second)
	t.Logf("✓ it-tcp 已上线，对外地址 %s", findView(views, "it-tcp").RemoteAddr)

	views, _ = a.ListProxies()
	if v := findView(views, "it-http"); v == nil || v.Status != "running" {
		t.Fatalf("it-http 未上线: %+v", v)
	}
	t.Logf("✓ it-http 已上线，对外地址 %s", findView(views, "it-http").RemoteAddr)

	// ---- 5. 单独停用 it-tcp，it-http 必须不受影响 ----
	if err := a.ToggleProxy("it-tcp", false); err != nil {
		t.Fatalf("ToggleProxy(false) 失败: %v", err)
	}
	views = waitProxy(t, a, "it-tcp", "disabled", 8*time.Second)
	if v := findView(views, "it-http"); v == nil || v.Status != "running" {
		t.Fatalf("停用 it-tcp 影响了 it-http: %+v", v)
	}
	t.Log("✓ 单独停用成功，且没有影响其它代理")

	// ---- 6. 重新启用 ----
	if err := a.ToggleProxy("it-tcp", true); err != nil {
		t.Fatalf("ToggleProxy(true) 失败: %v", err)
	}
	waitProxy(t, a, "it-tcp", "running", 10*time.Second)
	t.Log("✓ 重新启用成功")

	// ---- 7. 编辑已有代理（改名以外的字段）----
	tcpDef["tcp"].(map[string]any)["remotePort"] = float64(18084)
	if err := a.SaveProxy(tcpDef, true); err != nil {
		t.Fatalf("SaveProxy(更新) 失败: %v", err)
	}
	views = waitProxy(t, a, "it-tcp", "running", 10*time.Second)
	if got := findView(views, "it-tcp").RemoteAddr; got == "" {
		t.Fatalf("更新后拿不到对外地址")
	}
	t.Log("✓ 编辑代理成功")

	// ---- 8. 访问者 ----
	visDef := frpcapi.Definition{
		"name": "it-visitor",
		"type": "stcp",
		"stcp": map[string]any{
			"name": "it-visitor", "type": "stcp",
			"serverName": "it-tcp", "secretKey": "itsecret123",
			"bindAddr": "127.0.0.1", "bindPort": float64(18085),
		},
	}
	if err := a.SaveVisitor(visDef, false); err != nil {
		t.Fatalf("SaveVisitor 失败: %v", err)
	}
	vis, err := a.ListVisitors()
	if err != nil {
		t.Fatalf("ListVisitors 失败: %v", err)
	}
	if len(vis) != 1 || vis[0].Name != "it-visitor" || !vis[0].Enabled {
		t.Fatalf("访问者列表不符合预期: %+v", vis)
	}
	if vis[0].LocalAddr != "127.0.0.1:18085" {
		t.Fatalf("访问者本机映射地址不对: %q", vis[0].LocalAddr)
	}
	t.Logf("✓ 访问者创建成功，映射到 %s", vis[0].LocalAddr)

	if err := a.ToggleVisitor("it-visitor", false); err != nil {
		t.Fatalf("ToggleVisitor 失败: %v", err)
	}
	vis, _ = a.ListVisitors()
	if len(vis) != 1 || vis[0].Enabled || vis[0].Status != "disabled" {
		t.Fatalf("停用访问者后状态不对: %+v", vis)
	}
	t.Log("✓ 访问者停用成功")

	if err := a.DeleteVisitor("it-visitor"); err != nil {
		t.Fatalf("DeleteVisitor 失败: %v", err)
	}
	if vis, _ = a.ListVisitors(); len(vis) != 0 {
		t.Fatalf("访问者未被删除: %+v", vis)
	}
	t.Log("✓ 访问者删除成功")

	// ---- 9. 诊断 ----
	items := a.Diagnose()
	if len(items) == 0 {
		t.Fatal("诊断没有返回任何结果")
	}
	for _, it := range items {
		if it.Name == "服务端连接" && !it.OK {
			t.Fatalf("诊断认为服务端未连接: %+v", it)
		}
	}
	t.Logf("✓ 诊断返回 %d 项", len(items))

	// ---- 10. 迁移配置文件里的代理到 Store ----
	res, err := a.MigrateFileProxies()
	if err != nil {
		t.Fatalf("MigrateFileProxies 失败: %v", err)
	}
	if res.Moved != 1 || len(res.Failed) != 0 {
		t.Fatalf("迁移结果不符合预期: %+v", res)
	}
	views, _ = a.ListProxies()
	if v := findView(views, "from-file"); v == nil || v.FromFile {
		t.Fatalf("迁移后 from-file 应来自 Store: %+v", v)
	}
	if names, _ := cfgfile.FileProxyNames(a.cfgPath); len(names) != 0 {
		t.Fatalf("迁移后 frpc.toml 里不应还有代理: %v", names)
	}
	if !cfgfile.Exists(a.cfgPath + ".bak") {
		t.Fatal("迁移前应自动备份 frpc.toml.bak")
	}
	t.Log("✓ 迁移成功，配置文件里的代理已搬到 Store，并留下了 .bak 备份")

	// ---- 11. 删除代理 ----
	for _, name := range []string{"it-tcp", "it-http", "from-file"} {
		if err := a.DeleteProxy(name); err != nil {
			t.Fatalf("DeleteProxy(%s) 失败: %v", name, err)
		}
	}
	if views, _ = a.ListProxies(); len(views) != 0 {
		t.Fatalf("删除后仍剩 %d 条: %+v", len(views), views)
	}
	t.Log("✓ 全部代理删除成功")

	// ---- 12. 配置读写 ----
	c := a.GetConfig()
	if c.ServerPort != ctlPort || c.User != "ittest" {
		t.Fatalf("GetConfig 读到的值不对: %+v", c)
	}
	t.Logf("✓ 配置读取正确：%s:%d store=%s", c.ServerAddr, c.ServerPort, c.StorePath)

	if err := a.SaveConfig(c); err != nil {
		t.Fatalf("SaveConfig 失败: %v", err)
	}
	if !cfgfile.Exists(a.cfgPath + ".bak") {
		t.Fatal("SaveConfig 应生成 .bak 备份")
	}
	t.Log("✓ 配置保存成功（serverPort 没有被重置成默认值）")

	// ---- 13. 日志 ----
	logs := a.GetLogs(0)
	if len(logs) == 0 {
		t.Fatal("日志为空，应该至少有登录成功那条")
	}
	foundConnected := false
	for _, e := range logs {
		if e.Kind == frpcman.KindConnected {
			foundConnected = true
		}
	}
	if !foundConnected {
		t.Fatal("日志里没有识别出「登录成功」事件")
	}
	t.Logf("✓ 日志解析正常，共 %d 条，且识别到了连接成功事件", len(logs))
}

// TestITRecoversFromFileLogTarget 是这个 bug 的回归测试。
//
// 症状：界面永远停在"正在连接…"，但点开 frpc.log 发现其实早就连上了。
// 原因：配置里 log.to 指向了文件，图形界面读的是 stdout，于是什么都读不到，
//
//	而连接状态恰恰是从 stdout 的 "login to server success" 推断的。
//
// 这个测试模拟"用户拿来一份 log.to = frpc.log 的配置"，
// 验证 App 会在启动时自动修正它，并且修正之后状态真的能变成 connected。
func TestITRecoversFromFileLogTarget(t *testing.T) {
	binDir := os.Getenv("FRPCGUI_BIN")
	if binDir == "" {
		t.Skip("未设置 FRPCGUI_BIN，跳过集成测试")
	}

	const (
		ctlPort   = 17700
		vhostPort = 18110
		adminPort = 17420
		token     = "it-token-2"
	)

	dir := t.TempDir()
	if err := copyFile(filepath.Join(binDir, "frpc.exe"), filepath.Join(dir, "frpc.exe")); err != nil {
		t.Fatal(err)
	}

	frpsCfg := fmt.Sprintf("bindPort = %d\nvhostHTTPPort = %d\nsubDomainHost = \"frp.test\"\nauth.method = \"token\"\nauth.token = %q\nlog.to = \"console\"\n",
		ctlPort, vhostPort, token)
	frpsCfgPath := filepath.Join(dir, "frps.toml")
	if err := os.WriteFile(frpsCfgPath, []byte(frpsCfg), 0o600); err != nil {
		t.Fatal(err)
	}
	frps := exec.Command(filepath.Join(binDir, "frps.exe"), "-c", frpsCfgPath)
	if err := frps.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = frps.Process.Kill() }()
	waitPort(t, ctlPort, 8*time.Second, true)

	// ⚠ 关键：这里故意把日志写成文件，复现用户遇到的问题
	frpcCfg := fmt.Sprintf(`serverAddr = "127.0.0.1"
serverPort = %d
user = "ittest2"
auth.method = "token"
auth.token = %q
loginFailExit = false
webServer.addr = "127.0.0.1"
webServer.port = %d
webServer.user = "frpcgui"
webServer.password = "itpanelpass2"
store.path = "frpc-store.json"
log.to = "frpc.log"
log.level = "info"
log.maxDays = 1
`, ctlPort, token, adminPort)
	cfgPath := filepath.Join(dir, "frpc.toml")
	if err := os.WriteFile(cfgPath, []byte(frpcCfg), 0o600); err != nil {
		t.Fatal(err)
	}

	a := &App{
		baseDir:  dir,
		cfgPath:  cfgPath,
		exePath:  filepath.Join(dir, "frpc.exe"),
		settings: Settings{},
	}
	a.man = frpcman.New(a.exePath, a.cfgPath)
	cfg, err := cfgfile.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	a.cfg = cfg
	a.apiFn = func() *frpcapi.Client {
		return frpcapi.New(a.cfg.WebAddr, a.cfg.WebPort, a.cfg.WebUser, a.cfg.WebPass)
	}

	// 复现"没有修正"的情况：先确认这样启动确实读不到日志
	if err := a.StartFrp(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	time.Sleep(4 * time.Second)
	if logs := a.man.Logs(0); len(logs) != 0 {
		t.Fatalf("log.to 指向文件时本来就应该读不到日志，却读到了 %d 条", len(logs))
	}
	if s, _ := a.man.State(); s != frpcman.StateStarting {
		t.Fatalf("复现失败：状态应该是 starting，实际 %s", s)
	}
	t.Log("✓ 已复现：log.to 指向文件时，界面读不到任何日志，状态卡在 starting")
	_ = a.StopFrp()

	// 现在走"启动时自愈"这条路径
	cfg2, err := cfgfile.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	a.cfg = cfg2
	msg := a.fixLogTarget()
	if msg == "" {
		t.Fatal("应该报告修正了 log.to")
	}
	t.Logf("✓ 自愈提示：%s", msg)

	// 配置真的被改了，而且留了备份
	after, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(after), `log.to = "console"`) {
		t.Fatal("frpc.toml 里的 log.to 没有被改成 console")
	}
	if !cfgfile.Exists(cfgPath + ".bak") {
		t.Fatal("修正配置前应该留下 frpc.toml.bak 备份")
	}

	// 修正之后必须能连上（这就是用户要的结果）
	if err := a.StartFrp(); err != nil {
		t.Fatalf("修正后启动失败: %v", err)
	}
	defer func() { _ = a.StopFrp() }()
	waitState(t, a, frpcman.StateConnected, 12*time.Second)
	t.Log("✓ 修正后成功连接到服务端，状态推断恢复正常")

	// 顺带确认界面上不会再报"读不到日志"
	st := a.GetState()
	if st.NoLogs {
		t.Error("已经能读到日志了，NoLogs 不该为 true")
	}
	if st.ConfigFixed == "" {
		t.Error("ConfigFixed 应该保留给界面显示")
	}
	t.Log("✓ 状态里正确反映了「配置已自动修正」")
}

// TestITManagerCapturesOutput 单独验证"子进程输出能被读到"这件事。
//
// 如果这个测试挂了，说明 frpc 的 stdout 没有被捕获 ——
// 那么所有基于日志的状态推断都会失效（界面会一直卡在"正在连接…"）。
func TestITManagerCapturesOutput(t *testing.T) {
	binDir := os.Getenv("FRPCGUI_BIN")
	if binDir == "" {
		t.Skip("未设置 FRPCGUI_BIN，跳过集成测试")
	}
	dir := t.TempDir()
	if err := copyFile(filepath.Join(binDir, "frpc.exe"), filepath.Join(dir, "frpc.exe")); err != nil {
		t.Fatal(err)
	}
	// 故意指向一个没有服务在监听的端口：frpc 会立刻报连接错误
	cfg := `serverAddr = "127.0.0.1"
serverPort = 17999
auth.method = "token"
auth.token = "x"
loginFailExit = false
webServer.addr = "127.0.0.1"
webServer.port = 17401
store.path = "s.json"
log.to = "console"
log.level = "info"
`
	cfgPath := filepath.Join(dir, "frpc.toml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	m := frpcman.New(filepath.Join(dir, "frpc.exe"), cfgPath)
	if err := m.Start(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer func() { _ = m.Stop(nil) }()

	deadline := time.Now().Add(8 * time.Second)
	var logs []frpcman.LogEntry
	for time.Now().Before(deadline) {
		logs = m.Logs(0)
		if len(logs) > 0 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	for _, l := range logs {
		t.Logf("time=%q level=%q kind=%q raw=%q", l.Time, l.Level, l.Kind, l.Raw)
	}
	if len(logs) == 0 {
		t.Fatal("8 秒内没有从 frpc 读到任何日志 —— stdout 捕获失效")
	}
	state, msg := m.State()
	t.Logf("最终状态: %s (%s)", state, msg)
	if state != frpcman.StateFailed {
		t.Fatalf("连不上服务端时状态应为 failed，实际是 %s", state)
	}
}

// TestITNoAuthToken 验证「服务端没有开启认证」这条路径。
//
// 原理：frp 的 TokenAuthSetterVerifier 用
// util.ConstantTimeEqString(GetAuthKey(token, ts), privilegeKey) 比较。
// 服务端不配 auth.token 时 token 为空，客户端也留空 —— 两边算出来相同，
// **校验照样通过**。所以"空令牌"是合法配置，不是"没配好"。
//
// 这条路径以前整条是断的：向导不给下一步、SaveConfig 直接报错、
// 诊断页说配置不完整，用户根本走不到"连接"这一步。
// 更隐蔽的是 SaveConfig 里"空令牌就恢复旧值"，导致令牌**永远清不掉**。
func TestITNoAuthToken(t *testing.T) {
	binDir := os.Getenv("FRPCGUI_BIN")
	if binDir == "" {
		t.Skip("未设置 FRPCGUI_BIN，跳过集成测试")
	}

	const (
		ctlPort   = 17800
		adminPort = 17430
	)

	dir := t.TempDir()
	if err := copyFile(filepath.Join(binDir, "frpc.exe"), filepath.Join(dir, "frpc.exe")); err != nil {
		t.Fatal(err)
	}

	// ⚠ 服务端故意**完全不配 auth** —— 这就是"没有认证"的服务端
	frpsCfg := fmt.Sprintf("bindPort = %d\nlog.to = \"console\"\n", ctlPort)
	frpsCfgPath := filepath.Join(dir, "frps.toml")
	if err := os.WriteFile(frpsCfgPath, []byte(frpsCfg), 0o600); err != nil {
		t.Fatal(err)
	}
	frps := exec.Command(filepath.Join(binDir, "frps.exe"), "-c", frpsCfgPath)
	if err := frps.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = frps.Process.Kill() }()
	waitPort(t, ctlPort, 8*time.Second, true)

	// 客户端令牌留空（界面上"服务端没认证"时生成的就是这样）
	frpcCfg := fmt.Sprintf(`serverAddr = "127.0.0.1"
serverPort = %d
auth.method = "token"
auth.token = ""
loginFailExit = false
webServer.addr = "127.0.0.1"
webServer.port = %d
webServer.user = "frpcgui"
webServer.password = "itpanelpass3"
store.path = "frpc-store.json"
log.to = "console"
log.level = "info"
log.maxDays = 1
`, ctlPort, adminPort)
	cfgPath := filepath.Join(dir, "frpc.toml")
	if err := os.WriteFile(cfgPath, []byte(frpcCfg), 0o600); err != nil {
		t.Fatal(err)
	}

	a := &App{
		baseDir:  dir,
		cfgPath:  cfgPath,
		exePath:  filepath.Join(dir, "frpc.exe"),
		settings: Settings{},
	}
	a.man = frpcman.New(a.exePath, a.cfgPath)
	cfg, err := cfgfile.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	a.cfg = cfg
	a.apiFn = func() *frpcapi.Client {
		return frpcapi.New(a.cfg.WebAddr, a.cfg.WebPort, a.cfg.WebUser, a.cfg.WebPass)
	}

	// ① 界面不能因为"令牌为空"就认定没配置好
	if cfg.AuthToken != "" {
		t.Fatalf("预期读到空令牌，实际 %q", cfg.AuthToken)
	}
	if !a.readyToStart() {
		t.Error("令牌为空时 readyToStart() 应该是 true —— 否则用户点不了启动")
	}
	st := a.GetState()
	if !st.Configured {
		t.Error("令牌为空时 GetState().Configured 应该是 true")
	}
	if st.NeedSetup {
		t.Error("地址已填、令牌为空时，不该再要求用户走配置向导")
	}
	t.Log("✓ 空令牌被视为「已配置」")

	// ② 真的能连上 —— 这才是最终证据
	if err := a.StartFrp(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	waitState(t, a, frpcman.StateConnected, 12*time.Second)
	t.Log("✓ 空令牌成功连接到「无认证」的服务端")
	_ = a.StopFrp()

	// ③ 令牌必须能被清空（曾经会被 SaveConfig 恢复成旧值）
	withToken := *a.cfg
	withToken.AuthToken = "some-old-token"
	if err := cfgfile.Save(cfgPath, &withToken); err != nil {
		t.Fatal(err)
	}
	a.cfg = &withToken

	cleared := withToken
	cleared.AuthToken = ""
	if err := a.SaveConfig(cleared); err != nil {
		t.Fatalf("清空令牌时 SaveConfig 不该报错: %v", err)
	}
	_ = a.StopFrp()

	back, err := cfgfile.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if back.AuthToken != "" {
		t.Errorf("令牌应该被清空，实际又变回 %q —— SaveConfig 把空值恢复成旧值了", back.AuthToken)
	}
	t.Log("✓ 令牌可以被清空，不会被旧值顶回来")
}

// TestITHiddenFrpcStillReportsVersion 守住「隐藏控制台窗口不影响输出捕获」。
//
// HideConsole 会给子进程加 CREATE_NO_WINDOW。要确认这样起进程**仍然**能从
// stdout 读到版本号 —— 如果读不到，「关于」页就会一直显示兜底的写死版本号，
// 而且很难联想到是"隐藏窗口"造成的。
func TestITHiddenFrpcStillReportsVersion(t *testing.T) {
	binDir := os.Getenv("FRPCGUI_BIN")
	if binDir == "" {
		t.Skip("未设置 FRPCGUI_BIN，跳过集成测试")
	}

	cmd := exec.Command(filepath.Join(binDir, "frpc.exe"), "-v")
	frpcman.HideConsole(cmd) // ← 被测的就是这一句
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("带 HideConsole 执行 frpc -v 失败: %v", err)
	}

	got := strings.TrimSpace(string(out))
	if got == "" {
		t.Fatal("加了 CREATE_NO_WINDOW 之后读不到任何输出 —— 隐藏窗口把 stdout 也弄丢了")
	}
	if !strings.Contains(got, ".") {
		t.Errorf("输出看起来不像版本号: %q", got)
	}
	t.Logf("✓ 隐藏控制台窗口后仍能读到版本号: %q", got)
}

// ---------------------------------------------------------------- 测试辅助

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o700)
}

func waitPort(t *testing.T, port int, d time.Duration, want bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	probe := &App{}
	for time.Now().Before(deadline) {
		if probe.CheckLocalPort(port) == want {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("等待端口 %d 变为 %v 超时", port, want)
}

func waitState(t *testing.T, a *App, want frpcman.State, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if s, _ := a.man.State(); s == want {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	s, msg := a.man.State()
	t.Fatalf("等待状态 %s 超时，当前 %s（%s）", want, s, msg)
}

func waitProxy(t *testing.T, a *App, name, wantStatus string, d time.Duration) []ProxyView {
	t.Helper()
	deadline := time.Now().Add(d)
	var last []ProxyView
	for time.Now().Before(deadline) {
		views, err := a.ListProxies()
		if err == nil {
			last = views
			if v := findView(views, name); v != nil && v.Status == wantStatus {
				return views
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	v := findView(last, name)
	t.Fatalf("等待 %s 变成 %s 超时，当前: %+v", name, wantStatus, v)
	return last
}

func findView(views []ProxyView, name string) *ProxyView {
	for i := range views {
		if views[i].Name == name {
			return &views[i]
		}
	}
	return nil
}
