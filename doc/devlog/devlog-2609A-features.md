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
