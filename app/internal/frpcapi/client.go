// Package frpcapi 是 frpc 内置管理接口（Admin API）的客户端。
//
// 设计原则：本包**不导入 frp 的 Go 包**，只通过 HTTP + 通用 JSON 与 frpc 通信。
// 这样 GUI 与 frp 版本完全解耦 —— 升级 frp 只需要替换 frpc.exe 文件，
// GUI 一行代码都不用改。
//
// 接口清单（来自 frp v0.71.0 源码 client/api_router.go）：
//
//	GET    /healthz                      无需认证，用于探活
//	GET    /api/status                   所有代理的运行时状态
//	GET    /api/config                   读取配置文件原文（纯文本，非 JSON）
//	PUT    /api/config                   写入配置文件（不校验、不重载）
//	GET    /api/reload                   重读配置文件并增量应用
//	POST   /api/stop                     优雅停止
//	GET    /api/proxy/{name}/config      单个代理的完整定义
//	GET/POST       /api/store/proxies            Store 代理列表 / 新建
//	GET/PUT/DELETE /api/store/proxies/{name}     Store 代理 读取/更新/删除
//	GET/POST       /api/store/visitors           Store 访问者列表 / 新建
//	GET/PUT/DELETE /api/store/visitors/{name}    Store 访问者 读取/更新/删除
//
// 注意：Store 相关的路由只在 frpc 配置了 store.path 时才注册，
// 未启用时返回 404（对应 frp 内部的 ErrStoreDisabled）。
package frpcapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrStoreDisabled 表示 frpc 没有启用 Store（未配置 store.path）。
// 官方实现里这种情况返回 404，这里转换为明确的语义错误。
var ErrStoreDisabled = errors.New("Store 未启用：请在 frpc 配置中添加 store.path")

// ErrNotFound 表示目标资源不存在。
var ErrNotFound = errors.New("未找到该资源")

// ErrUnauthorized 表示认证失败（token 或面板密码不对）。
var ErrUnauthorized = errors.New("认证失败：请检查面板用户名密码")

// ProxyStatus 对应 frp 的 model.ProxyStatusResp。
//
// 注意 Status 字段的值来自 frp 的 ProxyPhase 常量，只有 6 个：
//
//	new / wait start / start error / running / check failed / closed
//
// 另外前端还会自己造两个值用于 Store 视图：disabled / waiting。
type ProxyStatus struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Err        string `json:"err"`
	LocalAddr  string `json:"local_addr"`
	Plugin     string `json:"plugin"`
	RemoteAddr string `json:"remote_addr"`
	Source     string `json:"source,omitempty"` // "store" 表示来自 Store
}

// Definition 表示一个代理或访问者的完整定义。
//
// 用 map 而不是强类型结构体，是为了与 frp 的版本解耦：
// frp 新增字段时 GUI 不需要改代码，直接透传即可。
// 结构形如：{"name":"web","type":"http","http":{...}}
type Definition map[string]any

// Client 是 frpc Admin API 的客户端。
type Client struct {
	baseURL string
	user    string
	pass    string
	hc      *http.Client
}

// New 创建一个客户端。addr/port 是 frpc 的 webServer 配置，通常为 127.0.0.1:7400。
func New(addr string, port int, user, pass string) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://%s:%d", addr, port),
		user:    user,
		pass:    pass,
		// 超时设短一些：这是本机回环通信，正常都在毫秒级。
		// 超时过长会让界面在 frpc 挂掉时卡住。
		hc: &http.Client{Timeout: 5 * time.Second},
	}
}

// do 发起请求并处理通用的错误映射。
func (c *Client) do(method, path string, body []byte, contentType string) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, c.baseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	if c.user != "" || c.pass != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 frpc 管理接口失败: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return data, nil
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		// 官方的 Store 未启用时也返回 404，这里靠调用方区分，
		// 统一先返回 ErrNotFound，Store 相关方法内部会转成 ErrStoreDisabled。
		return nil, ErrNotFound
	default:
		// 官方错误体是 {"Code":<int>,"Msg":"<text>"}（注意字段名首字母大写）
		var ge struct {
			Code int
			Msg  string
		}
		if json.Unmarshal(data, &ge) == nil && ge.Msg != "" {
			return nil, fmt.Errorf("%s", ge.Msg)
		}
		return nil, fmt.Errorf("frpc 返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
}

// ---------------------------------------------------------------- 探活与状态

// Healthy 探活。返回 nil 表示 frpc 的管理接口可访问（不代表已连上服务端）。
func (c *Client) Healthy() error {
	_, err := c.do(http.MethodGet, "/healthz", nil, "")
	return err
}

// Status 获取所有代理的运行时状态。
//
// 返回的是 map[代理类型][]状态，例如 {"http":[...], "tcp":[...]}。
// 注意：被禁用的代理不会出现在这里（它在到达 proxy manager 之前就被过滤掉了）。
func (c *Client) Status() (map[string][]ProxyStatus, error) {
	data, err := c.do(http.MethodGet, "/api/status", nil, "")
	if err != nil {
		return nil, err
	}
	out := map[string][]ProxyStatus{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("解析 /api/status 响应失败: %w", err)
	}
	return out, nil
}

// ---------------------------------------------------------------- 配置文件

// GetConfig 读取配置文件原文（纯文本 TOML，不是 JSON）。
func (c *Client) GetConfig() (string, error) {
	data, err := c.do(http.MethodGet, "/api/config", nil, "")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PutConfig 写入配置文件。
//
// ⚠ 官方实现是"只写文件、不校验语法、不重载"（源码 client/http/controller.go PutConfig）。
// 所以调用方必须先自行校验，写完再调 Reload 才会生效。
func (c *Client) PutConfig(content string) error {
	_, err := c.do(http.MethodPut, "/api/config", []byte(content), "text/plain")
	return err
}

// Reload 让 frpc 重读配置文件并增量应用。
//
// 官方语义（源码 client/config_manager.go）：
//   - 重新读取配置文件 → 与 Store 重新合并 → 校验 → 增量 diff
//   - 只有"新增或被修改"的代理会重启，未变动的不受影响
//   - ⚠ 不会应用全局参数（serverAddr / auth 等），那些必须重启进程
func (c *Client) Reload() error {
	_, err := c.do(http.MethodGet, "/api/reload", nil, "")
	return err
}

// Stop 优雅停止 frpc。
func (c *Client) Stop() error {
	_, err := c.do(http.MethodPost, "/api/stop", nil, "")
	return err
}

// ---------------------------------------------------------------- 单个代理定义

// GetProxyConfig 读取任意来源（配置文件或 Store）的单个代理定义。
func (c *Client) GetProxyConfig(name string) (Definition, error) {
	return c.getDef("/api/proxy/" + pathEscape(name) + "/config")
}

// GetVisitorConfig 读取任意来源的单个访问者定义。
func (c *Client) GetVisitorConfig(name string) (Definition, error) {
	return c.getDef("/api/visitor/" + pathEscape(name) + "/config")
}

func (c *Client) getDef(path string) (Definition, error) {
	data, err := c.do(http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	var d Definition
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("解析代理定义失败: %w", err)
	}
	return d, nil
}

// ---------------------------------------------------------------- Store：代理

// ListStoreProxies 列出 Store 中的全部代理。
//
// ⚠ 只返回 Store 里的条目，配置文件（frpc.toml）里的代理不在这里。
// Store 未启用时返回 ErrStoreDisabled。
func (c *Client) ListStoreProxies() ([]Definition, error) {
	data, err := c.do(http.MethodGet, "/api/store/proxies", nil, "")
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrStoreDisabled
		}
		return nil, err
	}
	var wrap struct {
		Proxies []Definition `json:"proxies"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("解析 Store 代理列表失败: %w", err)
	}
	if wrap.Proxies == nil {
		wrap.Proxies = []Definition{}
	}
	return wrap.Proxies, nil
}

// GetStoreProxy 读取 Store 中单个代理。
func (c *Client) GetStoreProxy(name string) (Definition, error) {
	return c.getStoreDef("/api/store/proxies/" + pathEscape(name))
}

// CreateStoreProxy 在 Store 中新建代理。成功后会立即生效并落盘。
func (c *Client) CreateStoreProxy(def Definition) (Definition, error) {
	return c.postStoreDef("/api/store/proxies", def)
}

// UpdateStoreProxy 更新 Store 中的代理。
//
// ⚠ URL 里的 name 必须与 body 里的 name 一致，
//   否则官方返回 400 "proxy name in URL must match name in body"。
//
// 这也是"单独启停某个代理"的实现方式：
// 读出定义 → 把对应类型块里的 enabled 改成 true/false → PUT 回去。
// 官方会触发增量重载，只有这一条代理会停/启，其他不受影响。
func (c *Client) UpdateStoreProxy(name string, def Definition) (Definition, error) {
	return c.putStoreDef("/api/store/proxies/"+pathEscape(name), def)
}

// DeleteStoreProxy 删除 Store 中的代理。
//
// ⚠ 如果配置文件里有同名代理，删除后会"复活"（Store 覆盖配置文件的遮蔽解除）。
func (c *Client) DeleteStoreProxy(name string) error {
	return c.del("/api/store/proxies/" + pathEscape(name))
}

// ---------------------------------------------------------------- Store：访问者

// ListStoreVisitors 列出 Store 中的全部访问者。
func (c *Client) ListStoreVisitors() ([]Definition, error) {
	data, err := c.do(http.MethodGet, "/api/store/visitors", nil, "")
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrStoreDisabled
		}
		return nil, err
	}
	var wrap struct {
		Visitors []Definition `json:"visitors"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("解析 Store 访问者列表失败: %w", err)
	}
	if wrap.Visitors == nil {
		wrap.Visitors = []Definition{}
	}
	return wrap.Visitors, nil
}

// GetStoreVisitor 读取 Store 中单个访问者。
func (c *Client) GetStoreVisitor(name string) (Definition, error) {
	return c.getStoreDef("/api/store/visitors/" + pathEscape(name))
}

// CreateStoreVisitor 在 Store 中新建访问者。
func (c *Client) CreateStoreVisitor(def Definition) (Definition, error) {
	return c.postStoreDef("/api/store/visitors", def)
}

// UpdateStoreVisitor 更新 Store 中的访问者。
func (c *Client) UpdateStoreVisitor(name string, def Definition) (Definition, error) {
	return c.putStoreDef("/api/store/visitors/"+pathEscape(name), def)
}

// DeleteStoreVisitor 删除 Store 中的访问者。
func (c *Client) DeleteStoreVisitor(name string) error {
	return c.del("/api/store/visitors/" + pathEscape(name))
}

// ---------------------------------------------------------------- 内部工具

func (c *Client) getStoreDef(path string) (Definition, error) {
	data, err := c.do(http.MethodGet, path, nil, "")
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrStoreDisabled
		}
		return nil, err
	}
	var d Definition
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("解析定义失败: %w", err)
	}
	return d, nil
}

func (c *Client) postStoreDef(path string, def Definition) (Definition, error) {
	body, err := json.Marshal(def)
	if err != nil {
		return nil, err
	}
	data, err := c.do(http.MethodPost, path, body, "application/json")
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrStoreDisabled
		}
		return nil, err
	}
	var out Definition
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("解析返回结果失败: %w", err)
	}
	return out, nil
}

func (c *Client) putStoreDef(path string, def Definition) (Definition, error) {
	body, err := json.Marshal(def)
	if err != nil {
		return nil, err
	}
	data, err := c.do(http.MethodPut, path, body, "application/json")
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrStoreDisabled
		}
		return nil, err
	}
	var out Definition
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("解析返回结果失败: %w", err)
	}
	return out, nil
}

func (c *Client) del(path string) error {
	_, err := c.do(http.MethodDelete, path, nil, "")
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// pathEscape 对路径参数做转义。
// frp 的 v2 路由用 UseEncodedPath + url.PathUnescape，所以这里按路径段转义即可。
func pathEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "?", "%3F"), "#", "%23")
}
