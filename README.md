# InpageBrowser

基于 KasmVNC + Chromium 容器的独立页内浏览器。登录后的主页面只保留顶部地址栏和下方远程 Chromium 画面。

## 当前形态

- Go 服务默认监听 `127.0.0.1:4002`。
- 手机号作为用户身份标识，Passkey/WebAuthn 完成首次注册和后续登录。
- 用户登录后才进入浏览器页；第一次进入时才创建其 Chromium 容器和持久 profile。
- 每个运行实例的容器名、VNC 密码、宿主映射端口均随机产生。
- 容器使用 `--rm`；空闲默认 10 分钟后停止并自动从 Docker 列表删除，profile 保留。
- 服务启动时会清理上次异常退出残留、带 `inpagebrowser.runtime=1` 标签的容器。
- 服务器默认最多同时运行 1 个浏览器实例；针对 2C/2G RAM + 2G swap 设计。
- Chromium 最多保留 2 个 page target；后台定期清理超出的标签页。
- 默认容器限制：1.5 CPU、1100MB RAM、1536MB memory+swap、384MB `/dev/shm`。

## 一键准备 Docker / Kasm Chromium

在仓库目录执行：

```bash
sudo ./scripts/bootstrap-linux.sh
```

脚本会自动：

1. 检查 Docker；没有时用 Docker 官方 convenience script 安装；
2. 启动并设置 Docker 开机启动；
3. 拉取 `kasmweb/chromium:1.18.0`；
4. 构建 Go 程序；
5. 创建并启动 systemd 服务。

不需要手工创建 Kasm 用户、密码、Docker 容器、端口或 compose 文件。

> 安装脚本仍需要一次 root/sudo，这是安装 Docker daemon 和 systemd service 无法避免的宿主机权限边界。

## Nginx + Cloudflare 回源

项目不要求 Nginx 在源站监听 HTTPS。示例见：

`deploy/nginx-cloudflare.conf.example`

核心要求：

- `listen 80`；
- `proxy_pass http://127.0.0.1:4002`；
- 保留 `Host`；
- 保留 Cloudflare 的 `X-Forwarded-Proto` / `CF-Visitor`，否则 Passkey Origin 会被误判成 HTTP；
- 转发 WebSocket `Upgrade` / `Connection`，KasmVNC 画面依赖 WebSocket；
- 关闭 proxy buffering，并使用较长 read/send timeout。

## 环境变量

- `INPAGE_ADDR`：监听地址，默认 `127.0.0.1:4002`
- `INPAGE_DATA_DIR`：数据目录，默认 `./data`
- `INPAGE_BROWSER_IMAGE`：Kasm Chromium 镜像，默认 `kasmweb/chromium:1.18.0`
- `INPAGE_MAX_ACTIVE`：全机同时活跃浏览器数，默认 `1`
- `INPAGE_IDLE_MINUTES`：无页面 heartbeat 后回收分钟数，默认 `10`

## 数据

`data/auth.json` 保存手机号、Passkey 公钥和登录 session；写入权限为 0600。

`data/profiles/<opaque-id>/` 保存远程 Chromium 用户 profile。容器回收不会删除该目录，因此 Cookie、LocalStorage、站点登录态可以延续到下次启动。

## 技术边界

Kasm 官方 Chromium standalone 镜像自身包含 Chromium + KasmVNC；本项目不安装完整 Kasm Workspaces。standalone 模式下音频、上传/下载、麦克风透传等部分宿主设备集成功能并不等同完整 Kasm Workspaces orchestration，当前版本优先保证现代网页在远程 Chromium 中真实运行。
