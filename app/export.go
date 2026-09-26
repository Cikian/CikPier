package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"frpcgui/internal/cfgfile"
	"frpcgui/internal/frpcapi"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows/registry"
)

// ================================================================ 访问者

// VisitorView 是访问者列表的一行。
//
// ⚠ 与代理不同，frpc 的 /api/status 只返回**代理**的运行态，
// 不包含访问者。所以这里的状态只能反映 Store 里的 enabled 开关，
// 不能像代理那样显示 "running / start error"。
// 界面上用「已启用 / 已停用」表达，不做无根据的推断。
type VisitorView struct {
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	Enabled    bool               `json:"enabled"`
	Status     string             `json:"status"` // enabled / disabled
	StatusText string             `json:"statusText"`
	Err        string             `json:"err"`
	ServerUser string             `json:"serverUser"`
	ServerName string             `json:"serverName"`
	LocalAddr  string             `json:"localAddr"`
	Def        frpcapi.Definition `json:"def"`
}

// ListVisitors 列出 Store 里的全部访问者。
func (a *App) ListVisitors() ([]VisitorView, error) {
	out := []VisitorView{}
	if !a.canManage() {
		return out, nil
	}
	defs, err := a.apiFn().ListStoreVisitors()
	if err != nil {
		if err == frpcapi.ErrStoreDisabled {
			return out, nil
		}
		return out, err
	}
	for _, d := range defs {
		name, _ := d["name"].(string)
		typ, _ := d["type"].(string)
		v := VisitorView{Name: name, Type: typ, Enabled: true, Def: d}

		if block, ok := d[typ].(map[string]any); ok {
			if e, ok := block["enabled"].(bool); ok {
				v.Enabled = e
			}
			v.ServerUser, _ = block["serverUser"].(string)
			v.ServerName, _ = block["serverName"].(string)
			addr, _ := block["bindAddr"].(string)
			if addr == "" {
				addr = "127.0.0.1"
			}
			if port, ok := toInt(block["bindPort"]); ok && port > 0 {
				v.LocalAddr = net.JoinHostPort(addr, strconv.Itoa(port))
			}
		}

		if v.Enabled {
			v.Status, v.StatusText = "enabled", "已启用"
		} else {
			v.Status, v.StatusText = "disabled", "已停用"
		}
		out = append(out, v)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ToggleVisitor 启用 / 停用一条访问者。做法与代理完全一致。
func (a *App) ToggleVisitor(name string, enabled bool) error {
	if !a.canManage() {
		return fmt.Errorf("frp 未运行")
	}
	api := a.apiFn()
	def, err := api.GetStoreVisitor(name)
	if err != nil {
		if err == frpcapi.ErrStoreDisabled || err == frpcapi.ErrNotFound {
			return fmt.Errorf("「%s」不在界面管理范围内，无法启停", name)
		}
		return err
	}
	typ, _ := def["type"].(string)
	block, _ := def[typ].(map[string]any)
	if block == nil {
		return fmt.Errorf("访问者「%s」的定义缺少 %s 配置块", name, typ)
	}
	block["enabled"] = enabled
	if _, err := api.UpdateStoreVisitor(name, def); err != nil {
		return fmt.Errorf("保存失败: %w", err)
	}
	return nil
}

// SaveVisitor 新建或更新一条访问者。
func (a *App) SaveVisitor(def frpcapi.Definition, isUpdate bool) error {
	if !a.canManage() {
		return fmt.Errorf("frp 未运行")
	}
	name, _ := def["name"].(string)
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("访问者名称不能为空")
	}
	api := a.apiFn()
	if isUpdate {
		if _, err := api.UpdateStoreVisitor(name, def); err != nil {
			return err
		}
		return nil
	}
	if _, err := api.CreateStoreVisitor(def); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "Conflict") {
			return fmt.Errorf("访问者名「%s」已存在，换一个", name)
		}
		return err
	}
	return nil
}

// DeleteVisitor 删除一条访问者。
func (a *App) DeleteVisitor(name string) error {
	if !a.canManage() {
		return fmt.Errorf("frp 未运行")
	}
	if err := a.apiFn().DeleteStoreVisitor(name); err != nil {
		if err == frpcapi.ErrNotFound {
			return fmt.Errorf("「%s」不在界面管理范围内，无法删除", name)
		}
		return err
	}
	return nil
}

// ================================================================ 迁移

// MigrateResult 是一次"导入配置文件里的代理"的结果。
type MigrateResult struct {
	Moved  int      `json:"moved"`
	Failed []string `json:"failed"`
}

// MigrateFileProxies 把 frpc.toml 里手写的 [[proxies]] / [[visitors]] 搬进 Store。
//
// 为什么要搬：写在配置文件里的代理走的是"静态配置"路径，
// Store API 对它们一律返回 404，因此界面无法启停或编辑它们。
//
// 策略：全部成功才重写配置文件（并自动备份为 frpc.toml.bak）；
// 只要有一个失败，就把已经建好的回滚掉，保证两种状态不会混杂。
func (a *App) MigrateFileProxies() (MigrateResult, error) {
	res := MigrateResult{Failed: []string{}}
	if !a.canManage() {
		return res, fmt.Errorf("frp 未运行，请先启动 frp 再执行导入")
	}

	proxies, err := cfgfile.FileProxyDefs(a.cfgPath)
	if err != nil {
		return res, err
	}
	visitors, err := cfgfile.FileVisitorDefs(a.cfgPath)
	if err != nil {
		return res, err
	}
	if len(proxies) == 0 && len(visitors) == 0 {
		return res, nil
	}

	api := a.apiFn()
	createdProxies := []string{}
	createdVisitors := []string{}

	for _, flat := range proxies {
		def, err := flatToDefinition(flat)
		if err != nil {
			res.Failed = append(res.Failed, describeFlat(flat)+"："+err.Error())
			continue
		}
		name, _ := def["name"].(string)
		if _, err := api.CreateStoreProxy(def); err != nil {
			res.Failed = append(res.Failed, name+"："+err.Error())
			continue
		}
		createdProxies = append(createdProxies, name)
		res.Moved++
	}

	for _, flat := range visitors {
		def, err := flatToDefinition(flat)
		if err != nil {
			res.Failed = append(res.Failed, describeFlat(flat)+"："+err.Error())
			continue
		}
		name, _ := def["name"].(string)
		if _, err := api.CreateStoreVisitor(def); err != nil {
			res.Failed = append(res.Failed, name+"："+err.Error())
			continue
		}
		createdVisitors = append(createdVisitors, name)
		res.Moved++
	}

	if len(res.Failed) > 0 {
		// 回滚，避免同一条服务同时存在于配置文件和 Store（会互相遮蔽，很难排查）
		for _, n := range createdProxies {
			_ = api.DeleteStoreProxy(n)
		}
		for _, n := range createdVisitors {
			_ = api.DeleteStoreVisitor(n)
		}
		res.Moved = 0
		return res, fmt.Errorf("导入未完成，已回滚。失败原因：%s", strings.Join(res.Failed, "；"))
	}

	// 全部成功：重写配置文件（Render 不会写 proxies，等于把静态定义摘掉）
	if a.cfg != nil {
		if err := cfgfile.Save(a.cfgPath, a.cfg); err != nil {
			return res, fmt.Errorf("代理已导入，但重写配置文件失败: %w", err)
		}
	}
	_ = api.Reload()
	return res, nil
}

// flatToDefinition 把配置文件里的"扁平"代理定义转成 Admin API 需要的嵌套结构。
//
// 配置文件里是：
//
//	[[proxies]]
//	name = "web"
//	type = "http"
//	localPort = 8080
//
// 而 API 需要：
//
//	{"name":"web","type":"http","http":{"localPort":8080}}
//
// 这个差异如果不处理，创建时会报 "exactly one proxy type block is required"。
func flatToDefinition(flat map[string]any) (frpcapi.Definition, error) {
	name, _ := flat["name"].(string)
	typ, _ := flat["type"].(string)
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("缺少 name 字段")
	}
	if strings.TrimSpace(typ) == "" {
		return nil, fmt.Errorf("缺少 type 字段")
	}
	if !isKnownType(typ) {
		return nil, fmt.Errorf("不支持的类型 %q", typ)
	}

	block := make(map[string]any, len(flat))
	for k, v := range flat {
		if k == "name" || k == "type" {
			continue
		}
		block[k] = v
	}
	// 类型块内部同样带 name/type（对应 frp 的 ProxyBaseConfig）
	block["name"] = name
	block["type"] = typ

	return frpcapi.Definition{"name": name, "type": typ, typ: block}, nil
}

func describeFlat(flat map[string]any) string {
	name, _ := flat["name"].(string)
	typ, _ := flat["type"].(string)
	if name == "" {
		return "（无名条目）"
	}
	return name + "[" + typ + "]"
}

func isKnownType(t string) bool {
	switch t {
	case "tcp", "udp", "http", "https", "tcpmux", "stcp", "sudp", "xtcp":
		return true
	}
	return false
}

// ================================================================ 端口预检

// CheckLocalPort 检测本机某个端口是否有服务在监听。
//
// 用途：用户新建代理时，在他填错端口的那一刻就给出提示，
// 而不是等他保存后困惑为什么代理一直起不来。
//
// ⚠ 只探测 127.0.0.1。若服务只监听 ::1 或某个具体网卡地址，会误报为"没监听"，
// 所以界面上这条只作为提示，不阻止保存。
func (a *App) CheckLocalPort(port int) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	conn, err := net.DialTimeout("tcp",
		net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 400*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// ================================================================ 配置文件与日志

// GetConfigText 返回 frpc.toml 的原文，供界面查看。
func (a *App) GetConfigText() (string, error) {
	data, err := os.ReadFile(a.cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("读取配置文件失败: %w", err)
	}
	return string(data), nil
}

// ExportLogs 把界面上的日志导出为文本文件。
//
// 用户点了取消时返回 "已取消"，前端会静默处理。
func (a *App) ExportLogs() (string, error) {
	target, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "导出日志",
		DefaultFilename: "frpc-logs-" + time.Now().Format("20060102-150405") + ".txt",
		Filters: []wruntime.FileFilter{
			{DisplayName: "文本文件 (*.txt)", Pattern: "*.txt"},
			{DisplayName: "全部文件 (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(target) == "" {
		return "", fmt.Errorf("已取消")
	}

	logs := a.man.LogsTail(0) // 0 → 全部
	var b strings.Builder
	fmt.Fprintf(&b, "# CikPier 日志导出\n")
	fmt.Fprintf(&b, "# 导出时间：%s\n", time.Now().Format(time.DateTime))
	fmt.Fprintf(&b, "# 配置：%s\n", a.cfgPath)
	if a.cfg != nil {
		fmt.Fprintf(&b, "# 服务器：%s:%d\n", a.cfg.ServerAddr, a.cfg.ServerPort)
		fmt.Fprintf(&b, "# 身份：%s\n", orDash(a.cfg.User))
	}
	b.WriteString("#\n\n")
	for _, e := range logs {
		fmt.Fprintf(&b, "[%s] %-5s %s\n", e.Time, strings.ToUpper(e.Level), e.Raw)
		if e.Summary != "" && e.Summary != e.Raw {
			fmt.Fprintf(&b, "          → %s\n", e.Summary)
		}
	}

	if err := os.WriteFile(target, []byte(b.String()), 0o600); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	return target, nil
}

// ================================================================ 界面设置

// Settings 是只属于图形界面的偏好，不存在 frpc.toml 里。
type Settings struct {
	CloseToTray bool `json:"closeToTray"`
	AutoRestart bool `json:"autoRestart"`
}

func (a *App) settingsPath() string {
	return filepath.Join(a.baseDir, "gui-settings.json")
}

// GetSettings 读取界面偏好。文件不存在时返回默认值。
//
// 默认 CloseToTray = false：也就是"点关闭按钮时弹窗询问"，
// 这是需求里明确选定的行为。用户可以在设置里打开它，之后就不再询问。
func (a *App) GetSettings() Settings {
	s := Settings{CloseToTray: false, AutoRestart: true}
	if data, err := os.ReadFile(a.settingsPath()); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	a.settings = s
	if a.man != nil {
		a.man.SetAutoRestart(s.AutoRestart)
	}
	return s
}

// SaveSettings 写入界面偏好。
func (a *App) SaveSettings(s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.settingsPath(), data, 0o600); err != nil {
		return fmt.Errorf("保存设置失败: %w", err)
	}
	a.settings = s
	if a.man != nil {
		a.man.SetAutoRestart(s.AutoRestart)
	}
	return nil
}

// ================================================================ 开机自启

const (
	runKeyName   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "CikPier"
)

// legacyRunValueNames 是程序以前用过的注册表项名。
//
// 程序改过名（frp 客户端 → CikPier）。旧名字留下的自启项会指向一个
// 已经不存在的 exe —— 用户看不到任何报错，只会觉得"开机怎么不启动了"。
// 所以每次读写自启设置时顺手清理一次。
var legacyRunValueNames = []string{"frp客户端"}

// purgeLegacy 删掉历史遗留的自启项名。失败不报错（本来没有也很正常）。
func purgeLegacy(k registry.Key) {
	for _, name := range legacyRunValueNames {
		_ = k.DeleteValue(name)
	}
}

// GetAutoStart 判断是否已设置开机自启。
//
// 实现方式：写当前用户的 Run 注册表项（HKCU），
// 这样**不需要管理员权限**，普通用户双击一下就能开关。
func (a *App) GetAutoStart() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyName,
		registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	purgeLegacy(k)

	v, _, err := k.GetStringValue(runValueName)
	return err == nil && strings.TrimSpace(v) != ""
}

// SetAutoStart 开启 / 关闭开机自启。
func (a *App) SetAutoStart(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyName, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("无法写入注册表（可能被安全软件拦截）: %w", err)
	}
	defer k.Close()
	purgeLegacy(k)

	if !on {
		if err := k.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("关闭开机自启失败: %w", err)
		}
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法定位程序路径: %w", err)
	}
	if err := k.SetStringValue(runValueName, `"`+exe+`"`); err != nil {
		return fmt.Errorf("设置开机自启失败: %w", err)
	}
	return nil
}

// ================================================================ 官方管理页面

// PanelInfo 是 frpc 自带管理页面（webServer）的访问信息。
//
// 为什么要把密码显示出来：这个用户名和密码是程序自动生成的，
// 用户从来没见过。点「打开官方管理页面」时浏览器会弹 Basic 认证框，
// 用户不知道填什么，只能一脸茫然地关掉。
type PanelInfo struct {
	URL       string `json:"url"`
	User      string `json:"user"`
	Password  string `json:"password"`
	Addr      string `json:"addr"`
	Port      int    `json:"port"`
	Running   bool   `json:"running"`
	LocalOnly bool   `json:"localOnly"`
}

// GetPanelInfo 返回官方管理页面的地址与凭据。
func (a *App) GetPanelInfo() PanelInfo {
	info := PanelInfo{
		Addr:      cfgfile.DefaultWebAddr,
		Port:      cfgfile.DefaultWebPort,
		LocalOnly: true,
	}
	if a.cfg != nil {
		if strings.TrimSpace(a.cfg.WebAddr) != "" {
			info.Addr = a.cfg.WebAddr
		}
		if a.cfg.WebPort != 0 {
			info.Port = a.cfg.WebPort
		}
		info.User = a.cfg.WebUser
		info.Password = a.cfg.WebPass
	}
	info.URL = fmt.Sprintf("http://%s:%d", info.Addr, info.Port)
	info.Running = a.apiReady()
	// 只有监听回环地址才算"仅本机"
	info.LocalOnly = info.Addr == "127.0.0.1" || info.Addr == "localhost" || info.Addr == "::1"
	return info
}

// OpenOfficialUI 在系统浏览器里打开 frpc 自带的管理页面。
//
// ⚠ 浏览器会弹 Basic 认证框，凭据用 GetPanelInfo 取（界面上会先展示并复制）。
func (a *App) OpenOfficialUI() {
	if a.ctx == nil {
		return
	}
	wruntime.BrowserOpenURL(a.ctx, a.GetPanelInfo().URL)
}

// ================================================================ 窗口

// ShowWindow 把窗口从托盘恢复到前台。
func (a *App) ShowWindow() {
	if a.ctx == nil {
		return
	}
	wruntime.WindowShow(a.ctx)
	wruntime.WindowUnminimise(a.ctx)
}

// HideToTray 隐藏窗口（程序继续在托盘运行）。
func (a *App) HideToTray() {
	if a.ctx == nil {
		return
	}
	wruntime.WindowHide(a.ctx)
}
