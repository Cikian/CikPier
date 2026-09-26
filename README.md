# CikPier

**中文** · [English](README.en.md)

**给 frp 做的 Windows 图形客户端 —— 不碰配置文件、不出黑窗口、全中文。**

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows-0078D4.svg)](#已知限制)
[![frp](https://img.shields.io/badge/frp-0.71.0-10B981.svg)](https://github.com/fatedier/frp)
[![Wails](https://img.shields.io/badge/built%20with-Wails%20v2%20%2B%20Go-5A67D8.svg)](https://wails.io)

![主界面](docs/images/main.png)

---

## 这是什么

[frp](https://github.com/fatedier/frp) 很好用，但它面向的是会看文档、会写配置的人。
给不懂技术的朋友配一次，光一份 `frpc.toml` 就得讲半小时 ——
什么叫控制端口、什么叫"子域名只写最左边一段"。

**CikPier 把这件事变成：解压 → 双击 → 跟着向导填三样东西。**

没有命令行、没有配置文件、不会弹黑窗口。装好之后代理在界面上逐条管理，
出问题有一键中文诊断。

> **它不修改也不依赖 frp 的源码**，底层跑的是官方原版的 `frpc.exe`。
> 所以升级 frp 只需要换掉那一个文件，不用重新编译本程序。

## 主要功能

| | |
|---|---|
| **零配置向导** | 第一次打开自动引导填服务器地址 / 端口 / 令牌，填完自动生成配置并启动 |
| **全中文** | 界面是中文，frp 原本的英文日志也被翻译成中文摘要（点一下可以展开看原文） |
| **一键连接诊断** | 逐项检查程序文件、配置、进程、服务端连接，以及**每条代理指向的本机端口通不通**，给出中文结论 |
| **代理逐条开关** | 列表里每条代理都能单独启用/停用，不用停掉整个 frp |
| **支持无认证服务端** | 服务端没配 `auth.token` 时，令牌留空即可 —— 不会被拦着不让下一步 |
| **装错架构会提醒** | ARM 电脑上跑了 x64 版会主动提示换哪个包（WebView2 在这个组合下有已知崩溃问题） |
| **系统托盘 / 开机自启** | 托盘图标按连接状态变色；自启走 HKCU，**不需要管理员权限** |
| **八种代理类型** | TCP / UDP / HTTP / HTTPS / TCPMUX / STCP / SUDP / XTCP，外加访问者 |
| **端口预检** | 新建代理时顺手检查本机端口有没有服务在监听 |

## 下载使用

**系统要求**：Windows 10 / 11。Windows 11 自带 WebView2；Win10 可能需要[单独安装](https://developer.microsoft.com/microsoft-edge/webview2/)。

去 [Releases](https://github.com/Cikian/CikPier/releases/latest) 下载对应架构的包：

| 你的电脑 | 下载哪个 |
|---|---|
| 普通电脑（Intel / AMD 处理器） | `CikPier-v1.0.0-amd64.zip` |
| ARM 电脑（骁龙处理器、Surface Pro X 等） | `CikPier-v1.0.0-arm64.zip` |

不确定是哪种？打开 **设置 → 系统 → 关于**，看「系统类型」那一行。

然后：

1. **解压到任意文件夹** —— 建议不要放 `C:\Program Files\`，那里每次操作都要管理员权限
2. **双击 `CikPier.exe`**
3. 跟着向导填 **服务器地址 / 服务器端口 / 认证令牌**（这三样找 frp 服务端管理员要）

顶栏变成 🟢 **已连接到服务端** 就成功了。

> 包里只有 4 个文件：`CikPier.exe`、`frpc.exe`、`使用说明.md`、`LICENSE`。
> 详细的图文说明见包里的 `使用说明.md`，或者[在线版](dist-staging/使用说明.md)。

## 截图

首次配置向导 —— 三步填完就能用：

![首次配置向导](docs/images/wizard.png)

新建代理 —— 选类型、填本机端口，高级选项默认收起：

![新建代理](docs/images/new-proxy.png)

## 与 frp 的关系

CikPier 是 frp 生态里的**第三方客户端**，和 frp 官方没有隶属关系。

- 它**不修改、不 fork** frp，通过子进程驱动官方原版 `frpc.exe`
  （起进程、生成配置、读它的输出、调它自带的 HTTP 管理接口）
- 因此 frp 升级时，只要把 `frpc.exe` 换成新的就行
- 连接状态是从 frpc 日志推断出来的（frp 的管理接口不提供"是否已连上服务端"），
  这也是为什么程序要求 `log.to = console`

## 已知限制

诚实列一下，免得你白折腾：

- **只有 Windows**。没有 macOS / Linux 版本
- **arm64 的包没有在真机上验证过** —— 手上没有 ARM 设备。
  目前只确认了编译产物架构正确、图标正常嵌入。有 ARM 机器的话欢迎帮忙试一下
- **界面只有中文**
- 还**没有安装程序**（免安装绿色版，解压即用），没有流量统计、没有深色模式

## 自己编译 / 改造

需要 **Go 1.25+** 和 **Wails CLI**（**不需要 Node.js / npm** —— 前端是三个静态文件，没有构建步骤）。

```powershell
git clone https://github.com/Cikian/CikPier.git
cd CikPier/app
wails dev          # 改代码，热重载
go test ./...      # 单元测试
```

打包（一次出 amd64 + arm64 两个包）：

```powershell
cd CikPier
powershell -ExecutionPolicy Bypass -File tools/package.ps1 -Version 1.0.0
```

> 打包需要官方 `frpc.exe`，它不在仓库里。从
> [frp Releases](https://github.com/fatedier/frp/releases) 下载后放到
> `frp-bins/win-amd64/`（ARM 用 `win-arm64/`），或者用 `-FrpcPath` 指定路径。

完整文档见 [`docs/`](docs/README.md)：开发上手 / 代码地图 / 修改指南 /
换品牌与图标 / 打包与发布 / 测试与排错。

## 许可

[Apache License 2.0](LICENSE)。随便用、随便改。

分发包里的 `frpc.exe` 来自 [fatedier/frp](https://github.com/fatedier/frp)，
同样以 Apache-2.0 授权。
