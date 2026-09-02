那么如何重启这整套服务？
还有如果要整套服务卸载，包括docker残留，怎么操作



可以分成两个层次：**重启 InpageBrowser 整套运行环境**，以及**彻底卸载 InpageBrowser + 它带来的 Docker/Kasm 残留**。

### 重启整套 InpageBrowser

正常情况下只需要：

```bash
systemctl restart inpagebrowser
```

然后看状态：

```bash
systemctl status inpagebrowser
```

实时看日志：

```bash
journalctl -u inpagebrowser -f
```

这个 restart 已经基本相当于你说的“整套服务重启”：

```text
systemd 停止 InpageBrowser
        ↓
Go 程序收到 SIGTERM
        ↓
当前 KasmVNC/Chromium 容器被 stop
        ↓
因为容器用了 --rm，Docker 自动删掉容器
        ↓
重新启动 InpageBrowser
        ↓
下次用户进入浏览器时重新创建新容器
        ↓
原来的 profile / Cookie / 登录状态继续保留
```

所以**一般不需要手工 restart Docker**。

如果 Docker 自己出了问题，再用：

```bash
systemctl restart docker
systemctl restart inpagebrowser
```

也可以一次：

```bash
systemctl restart docker && systemctl restart inpagebrowser
```

注意，重启 Docker 会影响这台机器上**所有 Docker 容器**。

---

## 只卸载 InpageBrowser，保留 Docker

如果这台服务器以后还可能用 Docker，我更建议先用这种。

停止并取消开机启动：

```bash
systemctl disable --now inpagebrowser
```

删除 systemd 服务：

```bash
rm -f /etc/systemd/system/inpagebrowser.service
systemctl daemon-reload
systemctl reset-failed
```

保险起见，把 InpageBrowser 遗留的浏览器容器全部删掉：

```bash
docker ps -aq --filter "label=inpagebrowser.runtime=1" | xargs -r docker rm -f
```

删除 Kasm Chromium 镜像：

```bash
docker image rm kasmweb/chromium:1.18.0
```

如果提示镜像还被容器占用，先执行前面的 `docker rm -f`。

然后进入项目目录，例如你截图里是：

```bash
cd /root/mydir/go-app/InpageBrowser
```

删除运行数据和编译产物：

```bash
rm -rf data
rm -rf bin
```

这里要注意：

```text
data/
```

里面包含：

* 用户手机号/Passkey 公钥信息
* Session
* 每个用户的 Chromium profile
* Cookie
* LocalStorage
* 网站登录状态

如果以后还想恢复账号和浏览状态，**不要删 `data/`**。

最后可清理 Docker 无引用缓存：

```bash
docker image prune -f
docker builder prune -f
```

这样 InpageBrowser 基本就清干净了，但 Docker 本身仍然保留。

---

# 连 Docker 也彻底卸载

如果这台服务器**就是专门为了 InpageBrowser 才安装 Docker**，而且确认没有其他程序使用 Docker，可以继续。

先执行前面“InpageBrowser 卸载”的全部步骤。

然后：

```bash
systemctl disable --now docker
systemctl disable --now containerd
```

如果是我们这个 `bootstrap-linux.sh` 通过 Docker 官方安装脚本安装的，在常见 Debian/Ubuntu 系统上可以：

```bash
apt-get purge -y \
  docker-ce \
  docker-ce-cli \
  containerd.io \
  docker-buildx-plugin \
  docker-compose-plugin \
  docker-ce-rootless-extras
```

再：

```bash
apt-get autoremove -y
```

然后删除 Docker 的全部运行数据：

```bash
rm -rf /var/lib/docker
rm -rf /var/lib/containerd
rm -rf /etc/docker
```

这一步才是真正意义上的：

> **Docker 镜像、layer、container、build cache 全部删除。**

如果还想把 Docker 官方 apt 源也删掉：

```bash
rm -f /etc/apt/sources.list.d/docker.list
rm -f /etc/apt/keyrings/docker.asc
rm -f /etc/apt/keyrings/docker.gpg
apt-get update
```

最后检查：

```bash
docker --version
```

正常应该提示：

```text
command not found
```

再检查：

```bash
systemctl status inpagebrowser
systemctl status docker
```

两者都应该已经不存在或停止。

---

### 如果你要“一条龙完全清空”

确认**这台机器没有其他 Docker 项目**后，可以按这个顺序：

```bash
systemctl disable --now inpagebrowser 2>/dev/null || true

docker ps -aq --filter "label=inpagebrowser.runtime=1" | xargs -r docker rm -f
docker image rm kasmweb/chromium:1.18.0 2>/dev/null || true

rm -f /etc/systemd/system/inpagebrowser.service
systemctl daemon-reload

cd /root/mydir/go-app/InpageBrowser
rm -rf data bin

systemctl disable --now docker 2>/dev/null || true
systemctl disable --now containerd 2>/dev/null || true

apt-get purge -y \
  docker-ce docker-ce-cli containerd.io \
  docker-buildx-plugin docker-compose-plugin \
  docker-ce-rootless-extras

apt-get autoremove -y

rm -rf /var/lib/docker
rm -rf /var/lib/containerd
rm -rf /etc/docker

rm -f /etc/apt/sources.list.d/docker.list
rm -f /etc/apt/keyrings/docker.asc
rm -f /etc/apt/keyrings/docker.gpg

apt-get update
```

但这一套我只建议在**明确 Docker 没有承载其他服务**时执行，因为 `/var/lib/docker` 一删，整台服务器所有 Docker 数据都会一起消失。

另外，从实际维护体验来说，这个项目后面最好再补两个脚本：

```text
scripts/restart-linux.sh
scripts/uninstall-linux.sh
```

尤其 `uninstall-linux.sh` 可以区分：

```text
仅卸载 InpageBrowser
彻底卸载 InpageBrowser + Docker
```

避免以后还要自己记这些命令。
