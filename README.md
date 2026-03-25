# LeoRay

LeoRay 是一款专为 macOS 设计的现代化 Xray 代理客户端。它原生集成 Swift UI 与强大的 Go 后端（xray_controller），旨在为你提供轻量级、便携且功能强大的极速代理体验。

## ✨ 特性 (Features)

- **极致原生界面**：基于 SwiftUI 构建，最低支持 macOS 13+，无缝融入系统体验。无论是系统状态栏的快捷菜单，还是完整的可视化仪表盘，操作响应都极为迅速。
- **支持全局代理 (TUN Mode)**：完美支持 TUN 驱动，实现真正的系统级全局代理接管。内置高度配置化的 FakeIP DNS 防泄漏与极速的自动路由劫持（避免死循环崩溃）。
- **真正的便携式封装**：通过脚本一键打包成单独的 `LeoRay.app` 文件。无需繁杂路径与环境配置，所有底层资源（`geoip`、`geosite` 等配置数据包）、引擎组件与配置文件完全内置于 App 的 Resources 中，拷贝走即可运行。
- **无缝节点与规则管理**：开箱即用的订阅同步与策略分流支持。直观地呈现后台运行状态与日志，快速执行节点测速，并自由在不同出口节点间切换。
- **一键授权免繁琐 (Sudoers)**：首次发起底层的网卡挂载请求（例如创建本地 TUN 路由表、挂载虚拟网卡）时会完成系统底层的 `sudoers` 安全授权，在此之后畅享顺滑启动，不产生任何恼人的授权弹窗。

---

## 📦 如何编译与构建 (Build & Package)

我们随代码附带了全自动封包构建的 `build_app.sh` 脚本，开发者可以直接将源码从零拉起拼装为 `.app` 可执行包，彻底摆脱笨重的 Xcode！

### 依赖环境
- **Go** (用于编译后端的 `xray_controller`)
- **Swift / Xcode Command Line Tools** (使用 `swift build` 命令即可，无需完整的 IDE 开启)

### 一键打包流程
在你的终端进入项目根目录，运行：
```bash
./build_app.sh
```

脚本运行过程中会自动处理以下核心流程：
1. 编译 Go 版本的本地控制器服务端，保障与 UI 层稳定通信。
2. 依据 `release` 环境将 Swift UI 编译为高效二进制代码。
3. 整合编译产物与零散的配置、图标资源文件 `LeoRay.icns`。
4. 在当前目录下构建并成功生成组装完整的 `LeoRay.app`！

> ℹ️ **需要变更 App 版本号吗？** 
> 只要在 `build_app.sh` 脚本文件最顶部的 `APP_VERSION` 以及 `BUILD_VERSION` 变量中调整版本号，以后每次打包便会自动渲染新的 `Info.plist` 内核。

---

## 🚀 首次使用注意事项 (First Run & Gatekeeper)

由于我们通常在个人环境下自行打包（没有配置复杂的苹果开发者强力签名），第一次运行构建好的 `.app` 可能会**被 macOS 的 Gatekeeper 或系统隐私安全机制拦截**（例如弹出“已损坏无法打开”）。

你只需要打开 macOS 终端，在 `LeoRay.app` 上方执行一条隔离属性清理命令：
```bash
xattr -cr /路径/前往/LeoRay.app
```
完成后，直接在访达中双击 `LeoRay.app` 即可畅通无阻地启动！

---

## 🧩 目录架构 (Architecture)

- `LeoRayUI/`
  项目的展示层。所有基于 Swift / SwiftUI 编写的菜单栏状态逻辑与主界面都在这里。
- `go/`
  项目的核心调度与控制器（API Server）。负责调用 macOS 路由功能、动态渲染修改底层 Xray 配置热重启等。
- `core/`
  包含可执行的 Xray 引擎内核。
- `data/`
  静态资料库与应用运行时产生的内部文件。包括你的代理节点信息 `custom_nodes.json` 以及 Xray 路由资产（`geosite.dat`, `geoip.dat`）。
- `config/`
  你的分流规则体系的承载者。
- `build_app.sh`
  整个专案唯一的灵魂自动化组装执行档。

# 上面是AI写的，下面我说一说
## 免责声明
- 此项目Code全是VibeCode，仅供学习交流，请勿用于非法用途，后果自负。
- 不保证任何功能可用，不保证任何功能可用，不保证任何功能可用。
- 如果动手能力强的，可以自己拉下来改改优化
- 仅测试了xhttp与Vless enc协议，其他自测
- 因为开启Tun模式，所以需要root权限，需要输入密码
- 内置的规则集随便写的，以及策略可能一团糟，可能需要自己修改策略

## 我知道的
- 项目是用Go写的，UI是SwiftUI，苹果原生平台
- 核心是xray，这个不必多说
- 好像我也不知道啥

# 鸣谢
感谢以下各位的全力支持！


|Google Antigravity|Gemini|Claude|
|---|---|---|
|![1774238285042.png](https://image.leoknox.de/cnmbie/2026/03/23/2c96b1210f6fd2b8dbd947f104a2b6eb.png)|![1774238523721.png](https://image.leoknox.de/cnmbie/2026/03/23/4abde4d49126bb15a9e6691800752735.png)|![1774238404666.png](https://image.leoknox.de/cnmbie/2026/03/23/6bf31f007b4c9587bb6213f9fbba0b7a.png)|
