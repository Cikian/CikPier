# 开发文档

> 项目介绍、下载地址和上手步骤在仓库首页的 [`../README.md`](../README.md)。
> 这里放的是**改代码、打包、排错**用的开发资料。

**CikPier** 是给 [frp](https://github.com/fatedier/frp) 做的 Windows 图形客户端。
它是独立项目，**不修改也不依赖 frp 的源码** —— 底层跑的是官方原版的 `frpc.exe`，
所以升级 frp 只需要换掉那一个文件。

---

## 一、文档索引

**如果你是第一次来，按顺序读 01 → 02 → 03。**

| 文档 | 讲什么 | 什么时候读 |
|---|---|---|
| [`docs/01-开发上手.md`](01-开发上手.md) | 装环境、跑起来、改完怎么验 | **第一次接触项目** |
| [`docs/02-代码地图.md`](02-代码地图.md) | 每个文件/函数在哪、一次点击的完整调用链 | 找代码位置 |
| [`docs/03-修改指南.md`](03-修改指南.md) | **改什么 → 去哪改**，按任务组织 | 动手改东西 |
| [`docs/04-换品牌与图标.md`](04-换品牌与图标.md) | 换 logo、改程序名、换作者信息、关于页 | **要变成"自己的"程序** |
| [`docs/05-打包与发布.md`](05-打包与发布.md) | 完整打包操作、参数、发布前检查清单 | 出安装包 |
| [`docs/06-测试与排错.md`](06-测试与排错.md) | 三层测试、调试工具箱、排错手册 | 测试挂了 / 东西坏了 |
| [`dist-staging/使用说明.md`](../dist-staging/使用说明.md) | **给最终用户的说明书**（会打进分发包） | 面向用户 |

> 📦 **打包要用到官方 `frpc.exe`，它不在本仓库里**（一百多 MB，也不该进仓库）。
> 从 [frp Releases](https://github.com/fatedier/frp/releases) 下载后放到
> `frp-bins/win-amd64/`（ARM 用 `win-arm64/`），打包脚本就会去那里找；
> 也可以用 `-FrpcPath` 直接指定路径。arm64 那份缺失时脚本会自动下载。

### 三条最常用的命令

> 下面的路径按**本地工作区**写（本项目在工作区里，`frp-bins/` 等是它的兄弟目录）。
> 单独克隆本仓库的话，把 `D:\Tools\frp_0.71.0\cikpier` 换成你自己的克隆路径即可。

```powershell
cd D:\Tools\frp_0.71.0\cikpier\app
wails dev                                     # 改代码

cd D:\Tools\frp_0.71.0\cikpier
powershell -File tools\package.ps1 -Version 1.0.0   # 一次出 amd64 + arm64 两个包
```

**不需要 Node.js / npm** —— 前端就是三个静态文件，没有构建步骤。

---

## 二、三个关键架构决定

理解这三条，就能理解项目里大部分"为什么这么写"。

### 1. GUI 不导入 frp 的 Go 模块

`internal/frpcapi` 只通过 **HTTP + 泛型 JSON** 和 frpc 通信，
代理定义用 `map[string]any` 透传。

**为什么**：frp 每次升级都可能给代理配置加字段。强类型映射意味着每次升级都要
跟着改 GUI；泛型透传则意味着 GUI 完全不用动。代价是丢掉编译期检查，
所以用集成测试兜底（见 [`docs/06-测试与排错.md`](06-测试与排错.md)）。

### 2. 所有代理都放在 Store 里

代理可以写在 `frpc.toml` 的 `[[proxies]]` 里，也可以放在 `store.path` 指向的 JSON 里。

**只有 Store 里的代理能被启停和编辑**：`/api/store/proxies/{name}` 对静态配置里的
代理一律返回 404。所以界面把代理全部放 Store，并对用户手写的代理给"一键迁移"入口。

### 3. 界面是纯 HTML/CSS/JS，没有构建步骤

Wails 运行时会自动把 `/wails/ipc.js` 和 `runtime.js` 注入 HTML，
所以**不需要生成 `wailsjs/`** 也能调 `window.go.main.App.*`
（`wails build -skipbindings` 就是干这个的）。

---

## 三、踩过的坑

> **这一节是整个项目最值钱的部分。** 下面每一条都是真实排查过的故障，
> 不是"可能会遇到的问题"。改相关代码前扫一眼，能省下大量时间。

### Windows / PowerShell

- **`.ps1` 里有中文就必须带 UTF-8 BOM**，否则 Windows PowerShell 5.1 按 ANSI 代码页解析，
  报的是 `Unexpected token '}'` 这种和真实原因毫无关系的语法错误。
  （**编辑工具重写文件时容易把 BOM 吃掉** —— 改完 `.ps1` 记得确认。）
- **PS 5.1 的 `Get-Content` 默认按 ANSI 读文件**。读 UTF-8 源码必须显式写 `-Encoding UTF8`，
  否则"代理"会变成"浠ｇ悊"，写回去就把源文件毁了。写文件用
  `[System.IO.File]::WriteAllText($p,$c,(New-Object System.Text.UTF8Encoding($false)))`，
  因为 PS 5.1 没有 `utf8NoBOM` 这个枚举。
- **`pwsh`（7）里 `Add-Type -ReferencedAssemblies System.Drawing` 会失败**
  （类型转发到 `System.Drawing.Common`）。要 `System.Drawing` 的脚本得用
  `powershell.exe`（5.1）跑。
- **`wails build -clean` 会因 `Access is denied` 失败**，如果 `build\bin` 里有进程在跑
  （典型是上次 `wails dev` 拉起的 `frpc.exe`）。`tools/package.ps1` 现在会自动清理。
- **PowerShell 变量名大小写不敏感，别和参数撞名。** 给 `build-preview.ps1` 加截图
  目录变量时一开始写成了 `$shots`，而那脚本有 `[switch]$Shots` 参数 ——
  赋值直接报 `Cannot convert value "System.String" to type
  "System.Management.Automation.SwitchParameter"`，脚本整个跑不起来。改名为 `$shotDir`。

### frp

0. **`log.to` 必须是 `console`，否则界面永远停在"正在连接…"**（真实事故，务必先读）

   界面读的是 **frpc 子进程的 stdout 管道**，因为连接状态只能从
   `login to server success` 这行日志推断（frp 的管理接口不提供这个状态）。
   如果 `log.to` 指向文件，管道里什么都读不到 →
   状态永远不变 → 界面永远显示"正在连接…"，**而 frpc 其实早就连上了**。

   这曾经是 `cfgfile.Default()` 的 bug（默认值写成了 `"frpc.log"`），
   于是**每个走完向导的用户都会踩到**。现在三道防线：`Default()` 和 `SaveConfig()`
   都强制 `console`；启动时 `fixLogTarget()` 自愈并告知用户；
   界面上 `noLogs` 提示条直接点破原因。
   回归测试 `TestDefaultUsesConsoleLog`、`TestITRecoversFromFileLogTarget`。

1. **ANSI 颜色转义会毁掉日志解析**（第二坑）

   frp 的 console 日志默认带颜色，**而且输出到管道时照样带**：
   `\x1b[0m\x1b[1;34m2026-09-25 ... [I] [client/service.go:312] try to connect to server...`
   时间戳因此不在行首，所有正则失配 → 状态推断全废。

   两道防线：生成的配置里写 `log.disablePrintColor = true`；
   `ParseLine` 里再剥一次。回归测试 `TestParseStripsANSI`。

2. **`[[proxies]]` 在配置文件里是扁平的，在 Store API 里是嵌套的**

   - `frpc.toml`：`{name, type, localPort, subdomain, ...}` 全部平铺
   - Store API：`{"name":..,"type":"http","http":{localPort, subdomain, ...}}`
   - **Store 的 JSON 文件又是扁平的**（`TypedProxyConfig.MarshalJSON` 直接序列化类型块）

   `flatToDefinition()` 负责这个转换，不转会报
   `exactly one proxy type block is required`。

3. **`store.path` 必须用点号写法** —— `[store]` 表头会把后面的根级键全吃进 store 表，
   frpc 启动时报 `json: unknown field "log"`。

4. **`subDomainHost` 是服务端字段** —— 写进 `frpc.toml` 会启动失败；
   服务端不配它，subdomain 形式的 HTTP 代理会报 `subdomain is not supported...`。

5. **健康检查会卡住代理上线** —— 配了 `healthCheck` 的代理在本机后端通过检查前
   一直是 `new` 状态，**不报错**，只在日志里刷 `do one health check failed`。

6. **`/api/status` 没有"是否已连上服务端"这个信息** —— 未连接时返回空，
   和"没有代理"无法区分。所以连接状态只能从日志推断（见坑 0）。

7. **`serverPort` 默认 7000，但用户可能配了别的** —— `SaveConfig` 对 `ServerPort == 0`
   做保留而不是重置，否则用户只改个名字就把 7500 悄悄变成 7000 了。

8. **HTTP/HTTPS 的 `remote_addr` 带着服务端内部端口** ——
   `server/proxy/http.go:105` 用 `CanonicalAddr(domain, VhostHTTPPort)` 拼出来，
   所以客户端拿到 `web.example.com:18181`。那个端口是 frps 的 vhost 端口，
   通常由 nginx 反代、且被防火墙挡着 —— 用户照着界面加端口必然访问失败
   （实测：域名 200，加端口超时）。**http/https 只显示域名**，
   端口和解释放进悬停提示；tcp/udp/tcpmux 保留端口。

9. **frps 的 `DELETE /api/proxies` 只是清理离线统计**，不会真的删代理；
   `/api/reload` 不作用于全局参数；`PUT /api/config` 既不校验也不重载。

### Wails

10. **`go build` 不带 `-tags production` 会得到一个空壳程序**
    —— `App.Run()` 直接 `return nil`，程序 3 秒后静默退出（**退出码 0**）。

11. **Windows 上的 `MessageDialog` 只有"是 / 否"两个按钮** —— `QuestionDialog` 被硬编码成
    `MB_YESNO`，`options.Buttons` 里的中文按钮文字**会被完全忽略**
    （`internal/frontend/desktop/windows/dialog.go:160`）。所以 `beforeClose` 用
    "是 / 否"表达三个语义：是=最小化到托盘、否=退出、直接关掉对话框（返回 Cancel）=取消。

12. **Wails v2 没有系统托盘** —— 用 `fyne.io/systray` 在独立线程跑自己的消息循环。
    注意它 `init()` 里有 `runtime.LockOSThread()`，`Register()` 只建窗口不跑循环，
    `Run()` 才跑循环 —— 所以用 `go systray.Run(...)`。

13. **单实例锁在 `startup` 之前生效** —— 第二个实例直接被 Wails 退出，
    `OnStartup` 根本不会执行。排查"程序一闪就没了"先想到这个。

14. **WebView2 窗口用 `PrintWindow` 截图会缺内容** —— 走 DirectComposition，
    抓到的经常是残缺的帧。用 `tools/build-preview.ps1` 的无头截图。

15. **exe 属性一片空白，可能是两个独立的坑** ——
    (a) `wails.json` 的 `info` 段不填，`companyName` / `productName` 会取
    `wails.json` 的 `name`（给工具链用的小写标识 `cikpier`）；
    (b) `build/windows/info.json` 的语言块键默认是 **`"0000"`**（语言中立），
    Windows 按具体语言查字符串表，**资源存在也读不出来** —— 要改成 `"0409"`；
    而且字符串表里**必须补上 `FileVersion` 项**，否则"文件版本"一栏永远是空的。
    见 [`docs/04-换品牌与图标.md`](04-换品牌与图标.md) §八。

### PowerShell 调 API 的坑（做工具脚本时会遇到）

16. **手动设了 `Accept-Encoding: gzip` 会导致 Go 不解压** ——
    Go 的 `http.Transport` 只在**它自己**添加这个头时才自动解压。手动设了就跳过解压，
    于是拿到原始 gzip 字节去当 JSON 解析，报
    `Unexpected token '', "� ..." is not valid JSON`。
    `tools/gen-image.ps1` 就是为了绕开这个（用 .NET HttpClient，不主动声明 gzip）。

### 前端状态与渲染

17. **向导第 3 步只渲染一次 → 永远停在"正在连接…"**（用户报的真实问题）

    首次配置向导的「配置完成」页有三条检查结果。原来是在保存配置后
    **只 `refreshState()` 一次**就把结果写死进 DOM，之后再无更新。

    但 frpc 登录需要一两秒，保存那一刻状态必然还是 `starting` ——
    于是**点「开始使用」进主界面才会看到其实已经连上了**，向导页则一直卡着。

    修法：把渲染抽成 `paintWizardDone(st)`，在 `refreshState()` 末尾挂钩子
    （`wzStep === 3 && 向导没关 → 重绘`）。因为后端状态一变就发 `frp:state` 事件，
    **连上的瞬间就会刷新**，不需要额外开轮询定时器。

    ⚠ 这是一个**通用陷阱**：凡是"读取某个异步收敛的状态并一次性渲染"的地方，
    都要问一句"它会不会在我渲染完之后才变"。连接状态、进程启动状态都属于这类。

18. **`finally` 里的 `btn.textContent = '下一步'` 覆盖了 `paintWizard()` 设的文案**

    向导第 3 步的按钮应该叫「开始使用」，但 `finally` 块在 `paintWizard()` 之后
    无条件把文案写回「下一步」。**功能是对的，只有字错了**，所以很难被发现
    （用户截图里第 3 步的按钮就是「下一步」）。
    改成在 `finally` 里按 `wzStep` 决定文案。

### 认证

19. **令牌（`auth.token`）不是必填项 —— 服务端可以不开启认证**

    frp 服务端不配 `auth.token` 时，客户端也必须留空：两边都空时
    `util.GetAuthKey("", ts)` 算出来相同，`VerifyLogin` 的常量时间比较**照样通过**。
    所以「空令牌」是一个**合法状态**，不是"没配好"。

    但这套逻辑一开始在**每个环节**都被写成了必填：
    向导不给下一步、`SaveConfig` 直接报错、`Diagnose` 说配置不完整、
    `GetState().Configured` 判 false —— 结果**无认证服务端的用户根本用不了**。
    更隐蔽的是 `SaveConfig` 里"空令牌就恢复旧值"（照抄了 `ServerPort` 的写法），
    导致令牌**永远清不掉**。

    现在这 6 处都不再把令牌当必填，清单见
    [`docs/03-修改指南.md`](03-修改指南.md) §十二。
    回归测试 `TestITNoAuthToken` —— 真的起一个无认证的 frps 连一遍。

    ⚠ **改通信相关代码时记住这条**：判断"配置是否完整"只看 `serverAddr`。

### 子进程与窗口

20. **起 `frpc.exe` 不隐藏控制台 → 屏幕上黑窗口闪一下**（用户报的真实问题）

    症状：打开「关于」页时**闪过一个黑窗口**，然后页面内容才出来。

    原因：本程序是 **windowsgui** 子系统（自身没有控制台），而 `frpc.exe` 是
    **控制台**程序。用 `exec.Command` 直接起它，Windows 会为它**新分配一个控制台窗口**。

    `about.go` 为了拿 frp 版本号跑了一次 `frpc.exe -v`，但**忘了加隐藏设置** ——
    而 `frpcman.Manager.Start()` 是加了的（长驻进程不隐藏会一直有个黑窗口）。
    结果就是：只有"关于"页会闪，别的地方不会，非常容易漏。

    修法：把隐藏设置抽成 `frpcman.HideConsole(cmd)`
    （`HideWindow: true` + `CreationFlags: CREATE_NO_WINDOW`），
    **所有** 起 frpc 的地方都调它。新增调用点时别忘。

    > 💡 "页面内容才出来"和黑窗口是**同一个根因**：`GetAboutInfo()` 会阻塞等
    > `frpc.exe -v` 返回，而创建控制台窗口本身也要时间。现在 `startup()` 里
    > 用 `go a.frpVersion()` 预热过了，关于页是秒开的。

    回归测试：`TestHideConsoleSetsNoWindow`（标志位）、
    `TestITHiddenFrpcStillReportsVersion`（隐藏后仍能读到 stdout —— 这是主要风险，
    因为 `cmd.Output()` 拿到的是管道，跟有没有控制台无关，但必须证明）。

### 静态结构与行为脱节

21. **样式和 HTML 都写好了，就是没人写那句 JS —— 按钮从做出来就是死的**

    「新建代理」里的「**高级选项（可选）**」点上去毫无反应。查下去发现：

    - `main.css` 定义了状态：`.fold.open .fold-body{display:block}`、箭头 `rotate(90deg)`
    - `index.html` 搭好了骨架：`.fold > .fold-head(button) + .fold-body`
    - **`main.js` 里 `fold` 这个词一次都没出现过** —— 没有任何代码去切换 `.open`

    也就是说 CSS 定义了"怎么显示"，HTML 定义了"长什么样"，
    但**"什么时候切换"这一环整个缺失**。特征是：**不报错、不崩溃，就是没反应**。

    修法：`bindFolds()` 绑点击 + `setFoldOpen()` 切 class（顺手同步 `aria-expanded`）。

    ⚠ **这类"三缺一"很难靠读某一个文件发现** —— 单看 CSS 觉得没问题，
    单看 HTML 觉得没问题，只有**跨文件对照"谁负责改状态"**才会暴露。
    加任何有状态切换的控件时，顺手确认三件事都齐了：
    **结构 / 样式 / 切换逻辑**。

    顺带修的一个体验问题：编辑一条**开着加密或健康检查**的代理时，折叠区默认收起，
    用户**看不到这些设置其实开着**。现在 `syncFoldOpen()` 会在里面有内容时自动展开。

---

## 四、已知限制与待办

**用户能感知到的限制**（完整清单和原因见
[`docs/06-测试与排错.md`](06-测试与排错.md) §八）：

- 访问者没有运行态，界面不假装知道它连上没有
- 端口预检只探 `127.0.0.1`，只作提示不阻止保存
- 开机自启是**登录后**启动（用 HKCU Run 项，好处是不需要管理员）
- **ARM64 包没有在真机验证过**（手上没有 ARM64 设备）

**待办**：

- [ ] 打包成安装程序（Inno Setup / MSIX），带开始菜单快捷方式和卸载项
- [ ] 托盘气泡通知（现在只在界面里弹 toast）
- [ ] 代理流量统计（frps 的 `/api/traffic/{name}` 有数据）
- [ ] 导入 / 导出整套代理配置
- [ ] 深色模式
- [ ] 多语言（现在全部中文）
