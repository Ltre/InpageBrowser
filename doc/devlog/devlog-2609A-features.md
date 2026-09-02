# dev-2609A-step1 开发记录

日期：2026-09-03

## 1. 独立 InpageBrowser 初版

项目从空仓库建立，不再包含 FmlySys 的“查资料”菜单、推荐网址或家庭 Feed。产品界面收敛为登录页，以及登录后的“顶部地址栏 + 下方整块远程网页画面”。

### 1.1 用户体系

使用手机号标识用户，Passkey/WebAuthn 作为注册与登录凭证。首次注册由服务端生成 challenge、user handle 和 RP 信息，浏览器调用 `navigator.credentials.create()`；后续登录根据手机号取出 credential 并调用 `navigator.credentials.get()`。

为了让 Cloudflare HTTPS + 源站 HTTP 能正常工作，WebAuthn Origin 优先依据 `X-Forwarded-Proto`，并兼容 Cloudflare `CF-Visitor`。RP ID 从实际 Host 自动推导，不要求手工配置域名参数。

当前用户与 session 使用服务器本地 `data/auth.json` 原子写入保存，Passkey 仅保存 credential id、公钥 DER 和签名计数，不保存任何私钥。

### 1.2 KasmVNC + Chromium 按需实例

浏览器实例不会在用户注册时预创建。只有用户登录并进入主浏览器页后，前端才调用 `/api/runtime/start`：

- 随机容器名 `ipb-*`；
- 随机 KasmVNC 密码；
- Docker 自动随机分配 6901 与 9222 的 loopback 宿主端口；
- KasmVNC 6901 只映射到 `127.0.0.1`，不会直接暴露公网；
- Chromium DevTools 9222 同样只映射 loopback；
- `--rm` 保证 stop 后容器自动从 Docker 列表删除；
- profile 挂载到 `/home/kasm-user`，独立持久化。

针对 2C/2G RAM + 2G swap，默认限制全机 1 个活跃实例，单实例 1.5 CPU / 1100MB RAM / 1536MB memory+swap / 384MB shm。

页面每 30 秒 heartbeat。默认超过 10 分钟没有 heartbeat 后自动 stop；服务启动时还会清理带 `inpagebrowser.runtime=1` 标签的异常残留容器。

### 1.3 地址栏与远程 Chromium

下方画面不是 HTML 反向代理，而是 KasmVNC 传输的真实 Chromium 画面。Chromium 使用 kiosk 模式隐藏自身浏览器 chrome，使登录后的产品页面视觉上只有 InpageBrowser 顶部地址栏和网页区域。

顶部地址栏通过 Chromium DevTools Protocol 的 `Page.navigate` 控制远程页面。服务端实现了只访问 loopback DevTools 端口的最小 WebSocket client，不额外引入第三方 Go 依赖。

用户直接输入域名时自动补 `https://`；输入包含空格的普通文本时转为 Google 搜索。

服务端每 2 秒检查 Chromium page targets，超过 2 个时关闭多余 target，作为 2 tabs 资源上限。当前 UI 不额外提供标签管理条，2 tabs 主要用于兼容网页自身 `target=_blank` 等行为并限制资源上限。

### 1.4 KasmVNC Gateway

Go 服务把当前登录用户对应的随机 KasmVNC loopback 端口反向代理到同一站点，并注入随机 Basic Auth，用户不需要知道 `kasm_user`、VNC 密码或随机端口。

Kasm standalone 当前以自身 HTTPS/self-signed certificate 提供 6901，内部 ReverseProxy 只连接 `127.0.0.1` 并接受该自签证书；公网 TLS 由 Cloudflare 负责。

### 1.5 Docker 与部署

新增 `scripts/bootstrap-linux.sh`，目标是让部署者不手工维护 Docker/Kasm 参数。脚本自动安装/启动 Docker、拉取官方 Chromium 镜像、构建 Go 程序并建立 systemd service。

Nginx 示例 `deploy/nginx-cloudflare.conf.example` 仅监听 80 并转发到 `127.0.0.1:4002`，同时保留 Cloudflare HTTPS 协议信息和 KasmVNC 所需 WebSocket Upgrade。

### 1.6 验证

提交前执行：

- `gofmt`；
- `go test ./...`；
- `go vet ./...`；
- `bash -n scripts/bootstrap-linux.sh`；
- Docker 参数单元测试确认 `--rm`、loopback 随机映射、资源限制、kiosk/CDP 参数与 profile mount 均存在；
- Cloudflare Origin 推导和 URL 规范化单元测试。

真实 Kasm Chromium 图形会话仍需要具备 Docker daemon 和镜像的 Linux 服务器进行端到端验收；当前执行环境不能把这部分虚报为已验证。

## 2. Windows 开发启动与代理

新增 `scripts/win-dev.start.cmd`，用于 Windows 本地开发，不复用 Linux systemd/bootstrap 流程。

### 2.1 启动职责

脚本在当前 CMD 进程内设置运行参数，自动定位仓库根目录并准备 `data/profiles`、`bin`。默认把 InpageBrowser 监听改为 `0.0.0.0:4002`，便于同机与局域网访问；同时提示跨设备 Passkey 仍需要 HTTPS secure context。

启动前检查：

- Go 是否在 PATH；
- Docker CLI 是否存在；
- Docker daemon 是否可用；
- 若 daemon 未启动且检测到标准安装位置的 Docker Desktop，则自动拉起并最多等待约 90 秒；
- `kasmweb/chromium:1.18.0` 本地不存在时才执行 `docker pull`；
- 之后执行 `go mod download`、`go mod verify` 和 `go build`，再运行 `bin/inpagebrowser-dev.exe`。

没有在每次启动时执行 `go mod tidy`，避免开发启动脚本修改仓库模块文件。

### 2.2 主机代理与容器 Chromium 代理

Windows 脚本默认采用当前开发环境常用代理：

- `HTTP_PROXY/HTTPS_PROXY=http://127.0.0.1:58591`
- `ALL_PROXY=socks5://127.0.0.1:51837`
- `NO_PROXY=127.0.0.1,localhost,::1,host.docker.internal`

同时写入小写变量。默认值可通过 `INPAGE_HTTP_PROXY`、`INPAGE_HTTPS_PROXY`、`INPAGE_ALL_PROXY`、`INPAGE_NO_PROXY` 在脚本启动前覆盖。

仅设置 Windows 进程代理不能让容器内 Chromium 访问 `127.0.0.1:51837`，因为该地址在容器中指向容器自身。因此新增 `INPAGE_BROWSER_PROXY`：Windows 脚本默认设为 `socks5://host.docker.internal:51837`。runtime manager 检测到该变量时，会给 Docker 增加 `host.docker.internal:host-gateway`，并把代理写入 Chromium `APP_ARGS` 的 `--proxy-server`。

Linux 生产默认不设置 `INPAGE_BROWSER_PROXY`，因此保持服务器直连；Windows 也可显式设为 `direct://` 关闭浏览器代理。

Docker Desktop 的镜像拉取由 daemon 侧完成，进程环境变量不保证能覆盖其全局代理行为，因此失败提示明确区分了 Go 下载、Docker image pull 与容器内 Chromium 上网三种网络路径。

### 2.3 Docker 生命周期

Windows 启动脚本不会预建用户容器，也不会留下固定开发容器。它仅保证 Docker daemon 和基础 Chromium 镜像可用；用户真正进入浏览器后仍由 Go runtime manager 随机创建 `--rm` 容器，并沿用空闲回收机制。

补充 `.gitignore` 忽略 `/data/` 和 `/bin/`，避免 Windows/Linux 本地启动生成的认证数据、Chromium profile 和开发二进制污染 Git 状态。

### 2.4 验证

- 复核 batch 脚本所有 `goto` 目标均存在；
- 复核失败路径统一保留非零 exit code；
- 复核代理变量同时包含大小写形式且 loopback 位于 `NO_PROXY`；
- 复核 Docker Desktop 仅在 daemon 不可用时尝试启动，镜像仅在缺失时拉取；
- 用独立 runtime 测试夹具执行 `gofmt` 和 `go test ./internal/runtime`，验证未设置 `INPAGE_BROWSER_PROXY` 时原 Docker 参数不变，设置代理时包含 host-gateway 与 Chromium `--proxy-server`；
- 当前执行环境不是 Windows，无法虚报 `cmd.exe` / Docker Desktop 的真实端到端启动结果。
