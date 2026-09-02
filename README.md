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

## Windows 开发启动

Windows 本地开发使用：

```bat
scripts\win-dev.start.cmd
```

脚本会自动完成仓库定位、数据目录准备、Go 依赖下载/校验、开发二进制构建，并检查 Docker Desktop 是否可用。若 Docker CLI 已安装但 daemon 尚未启动，会尝试自动启动标准安装位置下的 Docker Desktop并等待 Linux container engine 就绪；Kasm Chromium 镜像仅在本机缺失时才自动拉取。

Windows 开发默认监听 `0.0.0.0:4002`，因此同机可访问 `http://localhost:4002/`，局域网也能访问该端口。需要注意：Passkey/WebAuthn 在另一台设备上通常要求 HTTPS secure context，单纯的 LAN HTTP 地址即使页面可达也可能被浏览器禁止使用 Passkey。

脚本默认使用以下本地代理，并同时设置大小写两套标准代理环境变量：

- HTTP/HTTPS：`http://127.0.0.1:58591`
- SOCKS5：`socks5://127.0.0.1:51837`
- `NO_PROXY`：`127.0.0.1,localhost,::1,host.docker.internal`
- 容器内 Chromium：`socks5://host.docker.internal:51837`

由于 Chromium 运行在 Docker 容器中，容器内的 `127.0.0.1` 不是 Windows 主机，所以浏览器代理单独使用 `host.docker.internal`。Go runtime 在设置 `INPAGE_BROWSER_PROXY` 时会给容器加入 host-gateway，并把该地址写入 Chromium 的 `--proxy-server`。

可在运行脚本前覆盖：

- `INPAGE_HTTP_PROXY`
- `INPAGE_HTTPS_PROXY`
- `INPAGE_ALL_PROXY`
- `INPAGE_NO_PROXY`
- `INPAGE_BROWSER_PROXY`

例如：

```bat
set INPAGE_HTTP_PROXY=http://127.0.0.1:7890
set INPAGE_HTTPS_PROXY=http://127.0.0.1:7890
set INPAGE_ALL_PROXY=socks5://127.0.0.1:7891
set INPAGE_BROWSER_PROXY=socks5://host.docker.internal:7891
scripts\win-dev.start.cmd
```

如果本地浏览器容器应直接联网，可以在启动前显式设置 `INPAGE_BROWSER_PROXY=direct://`。

脚本不会每次运行 `go mod tidy`，避免启动过程改写仓库模块文件；也不会创建固定 Docker container。远程浏览器仍由 InpageBrowser 在用户真正进入后按需创建，并由既有 `--rm` / idle reaper 机制回收。

> Windows 首次开发仍需要先安装 Docker Desktop（WSL2/Linux containers）和 Go 1.23+。脚本负责日常启动 Docker Desktop、检查 daemon、按需拉取镜像，不会替用户修改 Docker Desktop 的全局 GUI 配置。Docker Desktop 的镜像拉取由 daemon 执行；若它没有继承 Windows/system proxy，即使当前 CMD 已设置代理，首次 `docker pull` 仍可能需要让 Docker Desktop 使用系统代理。

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
- `INPAGE_BROWSER_PROXY`：可选 Chromium 代理；为空时 Chromium 直连，Windows 脚本默认设为 `socks5://host.docker.internal:51837`
- `INPAGE_MAX_ACTIVE`：全机同时活跃浏览器数，默认 `1`
- `INPAGE_IDLE_MINUTES`：无页面 heartbeat 后回收分钟数，默认 `10`

## 数据

`data/auth.json` 保存手机号、Passkey 公钥和登录 session；写入权限为 0600。

`data/profiles/<opaque-id>/` 保存远程 Chromium 用户 profile。容器回收不会删除该目录，因此 Cookie、LocalStorage、站点登录态可以延续到下次启动。

## 技术边界

Kasm 官方 Chromium standalone 镜像自身包含 Chromium + KasmVNC；本项目不安装完整 Kasm Workspaces。standalone 模式下音频、上传/下载、麦克风透传等部分宿主设备集成功能并不等同完整 Kasm Workspaces orchestration，当前版本优先保证现代网页在远程 Chromium 中真实运行。
