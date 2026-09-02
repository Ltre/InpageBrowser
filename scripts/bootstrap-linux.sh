#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET_USER="${SUDO_USER:-$(id -un)}"
IMAGE="${INPAGE_BROWSER_IMAGE:-kasmweb/chromium:1.18.0}"
if [ "$(id -u)" -ne 0 ]; then exec sudo -E bash "$0" "$@"; fi
if ! command -v docker >/dev/null 2>&1; then
  echo '[1/5] 安装 Docker Engine...'
  if command -v curl >/dev/null 2>&1; then curl -fsSL https://get.docker.com | sh; else echo '缺少 curl，无法自动安装 Docker' >&2; exit 1; fi
fi
systemctl enable --now docker
if getent group docker >/dev/null; then usermod -aG docker "$TARGET_USER" || true; fi
echo "[2/5] 拉取 $IMAGE ..."
docker pull "$IMAGE"
echo '[3/5] 准备数据目录...'
mkdir -p "$ROOT_DIR/data/profiles" "$ROOT_DIR/bin"
chown -R "$TARGET_USER":"$TARGET_USER" "$ROOT_DIR/data" "$ROOT_DIR/bin"
if ! command -v go >/dev/null 2>&1; then echo '未检测到 Go。请先安装 Go 1.23+，然后重新运行本脚本。' >&2; exit 1; fi
echo '[4/5] 构建 InpageBrowser...'
USER_HOME="$(getent passwd "$TARGET_USER" | cut -d: -f6)"
sudo -u "$TARGET_USER" env HOME="$USER_HOME" bash -lc "cd '$ROOT_DIR' && go build -o bin/inpagebrowser ./cmd/inpagebrowser"
cat >/etc/systemd/system/inpagebrowser.service <<SERVICE
[Unit]
Description=InpageBrowser
After=network-online.target docker.service
Requires=docker.service

[Service]
Type=simple
User=$TARGET_USER
SupplementaryGroups=docker
WorkingDirectory=$ROOT_DIR
Environment=INPAGE_ADDR=0.0.0.0:4002
Environment=INPAGE_DATA_DIR=$ROOT_DIR/data
Environment=INPAGE_BROWSER_IMAGE=$IMAGE
ExecStart=$ROOT_DIR/bin/inpagebrowser
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
SERVICE
systemctl daemon-reload
systemctl enable --now inpagebrowser
echo '[5/5] 完成。'
echo '服务监听: 0.0.0.0:4002'
echo '查看状态: systemctl status inpagebrowser'
