
问：


我测试了页内浏览器，看起来几乎不能用，只能有限加载看起来很静态的东西。我看到一些虚拟桌面系统，里面开启了一个虚拟的浏览器，然后能在里面运行功能完整的网页。怎么说？


答：

对。你看到的那类“网页里打开一个虚拟桌面/虚拟浏览器”，正确的技术路线叫 **Remote Browser / Remote Browser Isolation（RBI，远程浏览器隔离）**，或者更底层一点叫 **Web Remote Desktop**。

它跟我上一轮给 FmlySys 做的方案区别非常大。上一轮是：

> 用户浏览器 → FmlySys Go 代理 → 下载目标网页 → 修改 HTML/CSS/链接 → 再塞进 iframe

这种东西天然会被现代网页打得很惨。React/Vue SPA、Service Worker、WebSocket、WASM、OAuth、第三方 Cookie、复杂 CSP、跨域请求、Google/YouTube 等等，随便一个都可能坏。为了安全，我又主动禁了第三方 JS，所以实际上更接近“网页阅读器”，而不是浏览器。你现在测试觉得“几乎不能用”，这个判断是对的。

而你看到的虚拟桌面方案实际是：

> 手机/电脑 → FmlySys 页面 → **远程浏览器画面流** → 境外服务器上的真正 Chromium → Internet

网页的 JavaScript、Cookie、LocalStorage、IndexedDB、WebSocket、视频、Canvas、WebGL 等全部是在**境外服务器上的真正 Chromium** 里面运行。你本机收到的主要是浏览器画面，再把鼠标、触摸、键盘事件发回服务器。Kasm 对自己的 Browser Isolation 描述就是：浏览活动实际在隔离容器中执行，只把视觉输出流式传给客户端。([Kasm Workspaces][1])

所以对目标网站而言：

**它面对的是一台真的运行 Chrome/Chromium 的境外 Linux 机器。**

而不是面对我们自己写的一个蹩脚 HTTP 代理。

这就能解释你看到的“虚拟桌面里 Chrome 怎么什么都能打开”。

### FmlySys 最适合怎么改

我认为应该直接**废掉现在 `/research/proxy` 这条 HTML 改写路线作为主浏览模式**，改成：

* 境外 FmlySys 服务器运行 Chromium。
* 每个登录家庭成员获得自己的远程浏览器 Session。
* `/research` 页面嵌入这个远程 Chromium。
* 地址栏、标签页其实直接操作远程 Chromium。
* **5 标签限制直接限制远程 Chromium 最多 5 tabs**，而不是创建 5 个 Chromium。
* 网页的全部联网请求自然从境外服务器出去，不需要我们再逐个代理 HTML/CSS/JS。
* Google 登录、YouTube、地图、复杂 SPA 等兼容性会从现在的“阅读器水平”跃升到接近真正桌面 Chrome。

比较值得考虑的是下面三种路线：

| 方案                        | 原理                                                 | 对 FmlySys        |
| ------------------------- | -------------------------------------------------- | ---------------- |
| **KasmVNC + Chromium 容器** | Chromium 真正在 Docker/Linux 中运行，通过浏览器远程桌面协议显示        | **我最推荐**         |
| **Kasm Workspaces**       | 在 KasmVNC 上再加完整的 Session/容器/用户/权限管理                | 功能最完整，但对小型家庭系统略重 |
| **BrowserBox**            | 专门做“browser inside browser”，远端 Chrome + 可嵌入 Web UI | 非常贴合，但要考虑目前许可证   |

Kasm Workspaces 官方本身就支持容器化浏览器 Session，并明确提供开发 API 用于集成进自己的 Web 应用；KasmVNC 则是它使用的 web-native 远程桌面技术。([Kasm Workspaces][2])

BrowserBox 更直接，它甚至把自己的定位描述成类似：

> 一个可以嵌进网页的远程 WebView / 不受普通 iframe 跨域限制的 browser-in-browser。

它是真正远程运行 Chrome，然后提供浏览器 UI。([GitHub][3])

### 我更倾向 KasmVNC，而不是完整虚拟桌面

其实你不需要真的给每个成员启动一台“虚拟电脑”。

没必要：

> Linux Desktop
> ↓
> 桌面窗口管理器
> ↓
> Chrome

完全可以直接：

> Docker Container
> ↓
> Chromium
> ↓
> KasmVNC
> ↓
> FmlySys `/research`

用户看到的仍然像“网页里面嵌了一台浏览器”，但服务器资源开销会小很多。

而且 **一个成员 = 一个 Chromium Session**，里面允许最多 5 tabs 就行。

### 有个需求要重新理解：你之前说“缓存到自己设备”

换成远程 Chromium 后，这句话需要拆开。

真正的：

* Cookie
* LocalStorage
* IndexedDB
* HTTP cache
* 登录状态
* 网站 Session

都应该存在**远程 Chromium profile**，因为网页实际上运行在服务器上。

不能再指望把这些网页运行状态缓存到手机 Chrome 里——否则就重新回到了本地执行网页。

但我们仍然可以把：

* 标签列表
* 当前 URL
* 浏览历史
* 收藏
* 最近访问
* 阅读快照
* 截图
* Feed 状态

缓存在用户自己的设备。

也就是说，我建议把它改成：

> **网页运行状态：服务器端按家庭成员隔离保存。**
> **FmlySys 浏览器 UI 状态：当前设备本地保存。**

这比原来的设计实际上更合理。

服务器端还可以选择两种 Profile 模式：

**持久模式**：成员 A 下次进“查资料”，Google 登录、网站 Cookie 等还在。

**隐私模式**：退出“查资料”后销毁整个 Chromium 容器，下次全新环境。

FmlySys 甚至可以以后给用户一个按钮：

`普通会话 | 无痕境外浏览`

### 这条路线解决不了的少数东西

它不是百分之百等于实体电脑 Chrome。例如 DRM 视频、WebAuthn/Passkey、本地 USB、摄像头/麦克风、某些 GPU/WebGL、文件下载上传，需要另外做远程设备桥接或策略。

但普通网页、新闻、Google 搜索、复杂 JS 网站、论坛、管理后台、绝大部分 Web App，兼容性都会远高于现在的反向代理。

所以如果按你原来对“查资料”的设想继续开发，我会把技术方向改成：

**FmlySys 不再“代理网页”，而是“托管一台位于境外的真正 Chromium，并把它远程嵌入 `/research`”。**

这才是你看到那些虚拟桌面系统为什么“里面的浏览器就真的像浏览器”的根本原因。Kasm 这类系统甚至明确把浏览器 Session 做成临时容器，只把画面流给用户。([Kasm Workspaces][1])

就 FmlySys 这个项目规模，我目前首选的具体组合会是：

**Go/FmlySys 负责登录、成员身份、推荐 Feed、Session 调度 → Docker 中 Chromium + KasmVNC 负责真正浏览网页 → `/research` 嵌入远程浏览器视图。**

这比继续修现在那个 `/research/proxy` 值得得多。

[1]: https://trust.kasmweb.com/solutions/web-isolation/browser-isolation?utm_source=chatgpt.com "Browser Isolation | Kasm Workspaces"
[2]: https://trust.kasmweb.com/app-isolation?utm_source=chatgpt.com "Application Isolation | Kasm Workspaces"
[3]: https://github.com/openrbi2/openrbi_BrowserBox?utm_source=chatgpt.com "GitHub - openrbi2/openrbi_BrowserBox: 📷 BrowserBox - Remote isolated browser API for security, automation visibility and interactivity. Run on our cloud, or bring your own. Full scope double reverse web proxy with multi-tab, mobile-ready browser UI frontend. Plus co-browsing, advanced adaptive streaming, secure document viewing and more! But only in the Pro version. Get BB today! Secure your document needs and internet, today! · GitHub"





问：


KasmVNC + Chromium 容器  方案运行一个用户的浏览器实例会占用多少资源，需要额外启动什么程序吗



答：




不需要为每个用户启动一台完整虚拟机。**KasmVNC + Chromium 容器本质是 Linux 容器里跑一个真正的 Chromium，再跑一个虚拟显示/VNC 流媒体服务**，所以开销比“虚拟桌面 + 完整 VM”低很多。Kasm 官方也是用 Docker 容器来提供浏览器 Workspace。([Kasm Workspaces][1])

对 FmlySys 这种场景，我会按下面这个量级估算一个**正在运行的单用户浏览器实例**：

| 使用状态                  |            内存大致占用 |                    CPU |
| --------------------- | ----------------: | ---------------------: |
| 空白页 / 1 个轻网页          |    **300–500 MB** |                几乎 0，偶发 |
| 普通浏览，3–5 标签           |    **500–900 MB** |           通常 0.1–0.5 核 |
| 较重网页，Google/地图/复杂 SPA | **700 MB–1.5 GB** |             瞬时可吃 1 核以上 |
| YouTube / 动画 / 视频     |       **1–2 GB+** | 无 GPU 时可能长期占 1–2 核甚至更多 |

所以如果你问我**实际部署应该按多少预算**，我不会按官方给 KasmVNC 整台机器的“2 核 2 GB minimum”直接套；那个是服务器级推荐下限，不是单 session 实测值。([Kasm Workspaces][2])

### 对你这个 FmlySys，我会按 700 MB/活跃用户设计

例如你的服务器是：

* `2C / 2GB`：一个用户基本可用，但视频/重网页容易吃紧。
* `2C / 4GB`：**1–2 个家庭成员同时浏览比较舒服**。
* `4C / 8GB`：大约可以承担 **4–6 个普通活跃浏览 session**。
* `8C / 16GB`：家庭使用已经非常宽裕。

注意这是**同时在线**数量，不是家庭账号数量。

比如家庭有 8 个成员，并不意味着服务器常驻 8 × 700 MB。

完全可以：

> 成员点击“查资料”
> → 启动 Chromium Container
> → 浏览
> → 离开 10～20 分钟
> → 自动停止 Container

这样没人使用时，基本没有浏览器 RAM 开销。

---

## 一个 Chromium Container 里面到底要启动什么？

最小化以后，其实没有很多东西。

大致是：

```text
Docker container
│
├── Xvnc / KasmVNC
│    ├── 虚拟 X11 显示器
│    ├── VNC server
│    └── WebSocket / Web client streaming
│
├── 很轻的窗口管理器
│    └── Openbox / XFCE 某部分
│
└── Chromium
     ├── browser process
     ├── renderer processes
     ├── network process
     ├── GPU process
     └── utility processes
```

KasmVNC 的 `Xvnc` 本身同时扮演**虚拟 X Server + VNC Server**，不需要真正连接显示器。官方文档也是这么定义的。([Kasm Workspaces][3])

所以不是：

```text
QEMU
→ Ubuntu VM
→ GNOME
→ VNC
→ Chrome
```

而只是：

```text
Docker
→ Xvnc
→ Chromium
```

中间最多加一个很轻的窗口管理器。

---

# 甚至可以不启动完整桌面

这一点对 FmlySys 很重要。

Kasm 官方本身支持把 Chromium 直接作为 Workspace 应用运行，也支持 kiosk / app mode，而不是必须显示一个完整 Linux 桌面。([Kasm Workspaces][4])

我们可以让用户看到：

```text
┌──────────────────────────────────┐
│ ← → ⟳   https://google.com       │
├──────────────────────────────────┤
│                                  │
│           Google 网页            │
│                                  │
└──────────────────────────────────┘
```

而不是：

```text
┌──────── Ubuntu Desktop ──────────┐
│ Applications    Files            │
│                                  │
│ ┌──── Chromium ────────────────┐ │
│ │                             │ │
│ │                             │ │
│ └─────────────────────────────┘ │
│                           03:42  │
└──────────────────────────────────┘
```

前一种明显更适合 FmlySys，而且还省一些 RAM。

---

# Host 上额外需要什么？

如果**不用完整 Kasm Workspaces**，只采用我说的：

**KasmVNC + Chromium 容器**

那么宿主机主要需要：

```text
FmlySys
Docker
Nginx
```

然后每个活跃用户动态产生：

```text
fmly-research-member-12
    └── Chromium + KasmVNC

fmly-research-member-18
    └── Chromium + KasmVNC
```

不必安装：

* VMware
* VirtualBox
* QEMU/KVM
* Windows
* 完整虚拟机管理系统

而且我们甚至**没必要安装完整的 Kasm Workspaces Server**。

Kasm Workspaces 是一整套：

* 用户系统
* Workspace 管理
* Agent
* API
* Session scheduler
* Docker orchestration
* Web UI
* 数据库等等。

FmlySys 自己本来就已经有：

* 家庭成员身份
* 登录
* Session
* 权限
* Go 后端

再安装整套 Kasm Workspaces，会出现大量功能重叠。

因此我更倾向：

> **只借 Kasm 的 Chromium Docker image + KasmVNC streaming 技术。**

FmlySys 自己负责 session 生命周期。

---

# 一个用户进去时实际发生什么

我设想以后 `/research` 是：

```text
用户打开 /research
        ↓
FmlySys 检查 member_id=5
        ↓
检查 member-5 Chromium 是否已运行
       ↙             ↘
    没运行            已运行
       ↓                ↓
docker start/create     直接连接
       ↓
Chromium + KasmVNC
       ↓
WebSocket
       ↓
/research 页面显示浏览器
```

退出以后：

```text
无操作 15 分钟
       ↓
FmlySys
       ↓
docker stop
       ↓
RAM 释放
```

但 Chromium profile 可以保存：

```text
/data/research/profiles/member-5/
```

因此下次重新启动：

* Cookie 还在；
* 登录状态还在；
* 浏览历史还在；
* LocalStorage 还在；
* IndexedDB 还在；
* 收藏还在。

**停止容器 ≠ 删除浏览器数据。**

这是非常适合你这种家庭系统的。

---

## 5 个标签也不会乘以 5 个容器

你之前要求最多打开 5 个标签页。

正确实现应该是：

```text
成员 A
└── 1 Chromium Container
     ├── Tab 1
     ├── Tab 2
     ├── Tab 3
     ├── Tab 4
     └── Tab 5
```

而不是：

```text
Tab1 → Container1
Tab2 → Container2
...
```

Chromium 自己虽然也是多进程架构，但很多基础资源可以共享。

因此：

**5 tabs ≠ 5 × 500 MB。**

更可能是：

```text
基础 Chromium + KasmVNC
约 300～400 MB

每个普通网页 tab
再增加几十～一两百 MB
```

所以普通五标签大约落在前面说的 **500～900 MB** 是比较现实的容量规划。

---

# 视频是这里最吃资源的部分

这里有一个容易忽略的问题。

普通 Chrome 播视频：

```text
服务器解码视频
↓
服务器显示 Chrome
↓
KasmVNC 又把画面编码
↓
网络传给你
↓
你的手机再解码远程画面
```

相当于比直接 YouTube 多了一层远程桌面视频编码。

所以：

**查资料、Google、Wikipedia、论坛、新闻、后台系统、普通网页：很适合。**

但如果主要需求是：

> 用它连续看 1080p/4K YouTube

那么纯 CPU 小 VPS 就未必划算。

Kasm 的 Chromium/Chrome image 本身也支持 GPU 配置。([Kasm Workspaces][5])

不过就你的“查资料”定位，我**第一版完全不会要求 GPU**。没有必要为了偶尔打开视频把硬件门槛抬高。

---

# 我认为你的最低服务器可以很低

如果 FmlySys 本身仍然是 Go + SQLite，而且家庭成员人数有限，我会把第一版最低目标压在：

**2 vCPU + 2 GB RAM。**

同时只允许：

* 1 个活跃 Remote Chromium；
* 5 tabs；
* 15 分钟空闲自动 stop；
* 容器 memory limit 大约 `1.2GB`；
* 必要时配 1～2 GB swap。

能跑。

更舒服的是：

**2 vCPU + 4 GB RAM。**

这样 FmlySys + Nginx + SQLite 自身只吃很少，剩下绝大部分可以留给 Chromium。

所以这条技术路线没有我前面提“虚拟桌面”时听起来那么重。**我们不是每个人启动 Ubuntu 虚拟机，而是按需启动一个 Chromium + Xvnc 容器。**

对 FmlySys，我现在反而认为这是一个资源成本可以接受、而体验比当前 `/research/proxy` 高几个数量级的方案。

[1]: https://www.kasmweb.com/docs/develop/guide/workspaces.html?utm_source=chatgpt.com "Workspaces — Kasm 1.17.0 documentation"
[2]: https://www.kasmweb.com/kasmvnc/docs/latest/serverside.html?utm_source=chatgpt.com "Server Side — KasmVNC 1.0.0 documentation"
[3]: https://kasmweb.com/kasmvnc/docs/master/man/Xvnc.html?utm_source=chatgpt.com "xvnc(1) - the X VNC server — KasmVNC 1.0.0 documentation"
[4]: https://www.kasmweb.com/docs/develop/how_to/web_app_gateway.html?utm_source=chatgpt.com "Web App Gateway — Kasm 1.17.0 documentation"
[5]: https://www.kasmweb.com/docs/1.12.0/guide/custom_images.html?utm_source=chatgpt.com "Default Docker Images — Kasm 1.12.0 documentation"





问：


目前我服务器就是2核2G ram+2G vram。tabs限制开2个吧。你按KasmVNC + Chromium 容器 方案开发，不过不要让我一个个手动配置kasm那些信息, 尽量参数随机分配给成员用户，不是给每个成员立即分配，而是他们点击"查资料"按钮后，再初始化过程中才分配。也不要让我曲配置docker这些，最好是成员随用随建、不用了就定期回收，不要把docker列表搞得太乱。最后就是nginx的问题，我不知道你具体需要怎么配置，你要讲清楚，才能继续开发




答：



你这台 **2 核 / 2GB RAM + 2GB 虚拟内存（如果这里的 vram 指 swap）**，我建议把方案进一步收紧：**单成员最多 2 tabs，并且全机默认最多同时运行 1 个“查资料”远程浏览器实例**。第二个成员同时打开时先提示“当前浏览器资源正在使用”，而不是硬开第二个容器把 2GB 内存顶爆。这个并发上限以后服务器升级再改。

Kasm 官方的 Chromium 镜像本身就支持 standalone 方式运行：容器内已经包含 Chromium + KasmVNC，默认通过 6901 提供浏览器访问，不需要安装完整 Kasm Workspaces。官方示例也是一个 Docker 容器直接启动，并通过 `VNC_PW` 设置密码。([Docker Hub][1])

## 我准备采用的运行方式

不会给每个成员预先创建 Docker、端口、密码、目录。

成员第一次点击：

**查资料 → 初始化中**

此时 FmlySys 才做：

```text
member_id = 8
        ↓
检查是否已有活动 research session
        ↓ 没有
创建成员浏览器 profile 目录
        ↓
随机生成：
    session token
    Docker container name
    KasmVNC password
    localhost 映射端口
        ↓
启动 kasmweb/chromium 容器
        ↓
等待 6901 健康
        ↓
把远程 Chromium 嵌入“查资料”
```

例如内部实际可能随机成：

```text
container:
fmly-rb-a83e591fc4

Kasm password:
dXd8qP0...随机...

host:
127.0.0.1:43827
        ↓
container:6901
```

这些东西**全部由程序生成，成员根本看不到，也不用配置**。

而且下一次这个成员再打开时，容器名字、端口、VNC 密码都可以重新随机。

只有浏览器 profile 是持久的，例如：

```text
/var/lib/fmlysys/research/profiles/
    47d09d5b9a2f.../
```

这里我不会直接使用 `member-8` 这种名字，而是用服务器 secret + member id 派生的不可读目录名。

因此：

```text
Docker Container
临时

Chromium Profile
持久
```

成员退出后容器可以销毁，但 Google 登录、Cookie、网页 LocalStorage、浏览历史等还可以保留。Kasm 官方本身也支持通过 volume mount 保存用户 profile。([Kasm Workspaces][2])

---

# Docker 列表不会越积越多

这一点可以做得很干净。

每次创建容器都设置成类似：

```text
AutoRemove = true
```

也就是 Docker CLI 里的：

```text
--rm
```

所以：

```text
成员使用
↓
docker ps
fmly-rb-a83e591fc4

闲置
↓
stop

立即自动删除
↓
docker ps -a
也没有这个东西
```

不会出现：

```text
fmly-member-1
fmly-member-2
fmly-member-3
fmly-member-4
...
几十个 Exited 容器
```

镜像则所有成员共享**一个**：

```text
fmlysys-research-browser:1
```

它会基于：

```text
kasmweb/chromium:1.18.0
```

做一层很薄的 FmlySys 定制。

当前 1.18.0 Chromium 镜像压缩体积约 1.18GB，所以服务器磁盘至少还要预留几 GB 给展开后的镜像和用户 profile。([Docker Hub][3])

---

# 2 tabs 怎么真正限制

不能只在 FmlySys 外层画两个标签按钮，因为用户进入真正 Chromium 后还是可以按：

```text
Ctrl+T
中键
右键 → 新标签页
网页 target=_blank
```

所以我会在 Chromium 镜像里自带一个很小的强制扩展。

它监控 Chromium Tabs API：

```text
tabs <= 2
    正常

第三个 tab 出现
    ↓
立即关闭第三个
    ↓
提示：
“查资料最多同时打开 2 个标签页”
```

同时限制多窗口，避免：

```text
2 tabs × 5 windows
```

绕过限制。

因此真正限制的是**远程 Chromium 内部最多两个页面**。

---

# 针对你 2G 内存，我会这样限资源

我不会让 Docker 随便吃。

一个 Research 容器默认大致限制：

```text
Memory:
约 1.1 GB

CPU:
最大约 1.5 核

共享内存 /dev/shm:
约 384～512 MB

远程画面:
默认 1280×720

帧率:
约 24 FPS

Tabs:
2

全机同时活跃 Research Session:
1
```

官方 standalone 示例使用 `--shm-size=512m`；注意这个是上限，不是启动就直接吃掉 512MB。([Docker Hub][1])

这样服务器还有余量给：

```text
Linux
Nginx
FmlySys Go
SQLite
SSH
```

以及必要的 swap。

我还会限制 Chromium disk cache，例如约：

```text
128 MB
```

否则成员浏览几个月后 profile 很容易膨胀。

Cookie、登录状态、LocalStorage 不跟普通 cache 一起删。

---

# 回收策略

我准备让 FmlySys 自己每分钟跑一次轻量 reaper。

比如：

```text
用户正在 /research
    ↓
浏览器页面每 30 秒 heartbeat

最后 heartbeat < 5 分钟
    ↓
保持运行

超过 10 分钟没有 heartbeat
    ↓
docker stop

AutoRemove
    ↓
container 自动消失

profile
    ↓
继续保存
```

下一次再进：

```text
重新随机创建 container
↓
挂载原来的 profile
↓
Chromium 恢复这个成员自己的状态
```

如果 FmlySys 异常崩溃导致留下容器，启动时再扫描：

```text
label:
fmlysys.research=1
```

把已经失联的临时 Research Container 清理掉。

所以不会依赖人肉 `docker rm`。

---

# Docker 我也不准备让你手工配一堆东西

这一块开发时我会加一个：

```text
scripts/setup-research-runtime.sh
```

第一次部署只需要一次服务器级 bootstrap。

脚本自动做：

```text
检查 Docker
→ 没安装则安装
→ enable/start docker
→ 拉取 Kasm Chromium 基础镜像
→ 构建 fmlysys-research-browser
→ 创建 profile 根目录
→ 设置权限
→ 做一次 Chromium/KasmVNC smoke test
```

之后日常**完全不用管 Docker**。

但这里有一个我需要说清楚的边界：

**Docker daemon 本身是 root 级基础设施，不可能让一个普通 Go Web 请求凭空、安全地安装。**

所以第一次还是要有一次 root 操作，例如：

```bash
sudo ./scripts/setup-research-runtime.sh
```

我认为这已经是合理的“一键安装”，而不是让你去学 Docker、写 compose、建 network、配置 Kasm 用户。

---

# 最关键的 Nginx：我建议单独用一个子域名

这里我不建议：

```text
https://fmly.example.com/research/runtime/xxxxx/
```

虽然看起来漂亮，但 KasmVNC 的 Web client / WebSocket 对路径前缀并不是特别适合。历史上 `/websockify` 在 path-prefix reverse proxy 下就出现过问题；而官方 KasmVNC 的反向代理示例也是直接把站点根路径 `/` 转发给 KasmVNC。([Kasm Workspaces][4])

所以我建议：

```text
FmlySys:
https://family.example.com

Research Runtime:
https://research.family.example.com
```

**还是同一台服务器。**

不需要第二台机器。

这会让整个结构干净很多。

---

# Nginx 实际怎么走

注意，我也**不会让 Nginx 直接找随机 Docker 端口**。

不是：

```text
Nginx
→ 43827
→ container A

Nginx
→ 45281
→ container B
```

否则就要动态重载 Nginx，很恶心。

真正结构是：

```text
                        ┌→ member A container : random port
Browser
   ↓
research.family.com
   ↓
Nginx
   ↓
FmlySys :4001
   ↓
Research Gateway
                        └→ member B container : random port
```

也就是说：

**Nginx 永远只知道 FmlySys 的 4001。**

哪个成员对应哪个随机 Docker 端口，是 **Go 自己处理**。

因此以后成员创建/销毁容器：

> 完全不需要 nginx reload。

---

# 我需要你配置的 Nginx 实际只有这个

假设：

```text
FmlySys:
family.example.com

Research:
research.family.example.com

Go:
127.0.0.1:4001
```

先在 Nginx `http {}` 层有：

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}
```

然后增加：

```nginx
server {
    listen 80;
    server_name research.family.example.com;

    return 301 https://$host$request_uri;
}
```

HTTPS：

```nginx
server {
    listen 443 ssl http2;
    server_name research.family.example.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:4001;

        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;

        # KasmVNC 必须支持 WebSocket
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;

        # Remote Browser 是长连接
        proxy_read_timeout 1800s;
        proxy_send_timeout 1800s;
        proxy_connect_timeout 30s;

        proxy_buffering off;
        proxy_request_buffering off;

        client_max_body_size 50m;
    }
}
```

KasmVNC 官方反向代理配置同样明确要求 WebSocket Upgrade、HTTP/1.1、长 read/send timeout 和关闭 buffering。([Kasm Workspaces][4])

**就这些。**

你不需要配置：

```text
6901
6902
6903
用户 A
用户 B
Kasm 密码
Docker 容器名字
Docker upstream
```

都不用。

---

# 为什么 research 子域名还能识别 FmlySys 成员

我也不会把你现在的：

```text
fmly_session
```

Cookie 粗暴改成整个父域名共享。

那样安全边界反而变差。

流程会是：

```text
family.example.com/research
↓
FmlySys 已知道 member_id

点击启动
↓
POST /research/session

FmlySys 生成一次性 ticket
↓
随机、短时有效、只能消费一次

iframe 打开：
https://research.family.example.com/connect?ticket=xxxx
↓
Research Gateway 验证 ticket

设置：
fmly_research_session=<随机值>

然后进入远程 Chromium
```

所以 Research 子域名拿不到主站的普通登录 Cookie。

这是我认为比较正确的隔离方式。

---

# 一个需要你提前知道的 Kasm standalone 限制

网页本身：

* JavaScript
* React/Vue
* WebSocket
* Canvas
* WebGL
* Cookie
* Google 登录
* 网站自己的复杂交互

这些是在真正 Chromium 里运行，所以跟现在那个 `/research/proxy` 完全不是一个等级。

但官方也明确注明：**standalone Kasm Chromium 下，音频、文件上传/下载、麦克风透传等某些宿主设备集成功能，没有完整 Kasm Workspaces orchestration 时并非全部直接可用。** ([Docker Hub][1])

所以我要把“网页完整运行”和“本机设备完整透传”区分开。

第一版我要保证的是：

> **完整现代网页可以真正运行。**

至于：

> 本机选文件上传到远程网页
> 从远程网页下载文件到本机
> 麦克风
> 摄像头
> 音频

这些可以单独做 FmlySys ↔ Remote Browser 的桥接，不能在这一版里假装 standalone Kasm 天然全有。

---

## 最终我建议定成这个架构

| 项目             | 方案                                 |
| -------------- | ---------------------------------- |
| Browser        | `kasmweb/chromium:1.18.0` 派生镜像     |
| 每成员容器          | **用时创建，不预创建**                      |
| Tabs           | **最多 2**                           |
| 全机并发           | **默认 1 个活跃 Remote Browser**        |
| Container      | 随机名、随机密码、随机 loopback port          |
| Container 生命周期 | `AutoRemove`，闲置约 10 分钟回收           |
| Profile        | 第一次点击时才创建，成员隔离、持久保存                |
| Docker         | 一次性 setup 脚本自动安装/拉镜像/构建            |
| Nginx          | **固定 research 子域名 → FmlySys:4001** |
| Nginx 动态配置     | **完全不需要**                          |
| Container 路由   | FmlySys Go Research Gateway 动态完成   |
| 主站认证           | 一次性 ticket，不跨子域共享主 Cookie          |

所以你真正需要提前准备的只有：

1. 给 `research.<你的 FmlySys 域名>` 加一个指向同一服务器的 DNS 记录；
2. 给这个子域名配 HTTPS；
3. 加上上面的 Nginx `server` 块。

**除此之外，Kasm、随机端口、密码、Docker container、回收机制都应该由这次代码完成，而不是让你手工维护。**

按这个架构继续开发，Nginx 以后基本不需要因为成员增减或 Session 创建而再改。

[1]: https://hub.docker.com/r/kasmweb/chromium?utm_source=chatgpt.com "kasmweb/chromium - Docker Image"
[2]: https://www.kasmweb.com/docs/develop/guide/persistent_data/persistent_profiles.html?utm_source=chatgpt.com "Persistent Profiles — Kasm 1.17.0 documentation"
[3]: https://hub.docker.com/layers/kasmweb/chromium/1.18.0/images/sha256-1d7dfc6c6a756dcd78a4d8bcbcae538bf0da7fb7fd5deec3721e9d2b459fe79c?utm_source=chatgpt.com "Image Layer Details - kasmweb/sha256-1d7dfc6c6a756dcd78a4d8bcbcae538bf0da7fb7fd5deec3721e9d2b459fe79c:1.18.0"
[4]: https://kasmweb.com/kasmvnc/docs/master/how_to/reverse_proxy.html?utm_source=chatgpt.com "Running behind a reverse proxy — KasmVNC 1.0.0 documentation"





问：



那我觉得不如直接新开个项目，按KasmVNC + Chromium 容器 方案独立开发，不过不要让我一个个手动配置kasm那些信息, 尽量参数随机分配给成员用户，不是给每个成员立即分配，而是他们点击"查资料"按钮后，再初始化过程中才分配。也不要让我曲配置docker这些，最好是成员随用随建、不用了就定期回收，不要把docker列表搞得太乱。至于用户体系，同样跟fmly一样采用手机号+passkey注册登录。端口监听4002，nginx不需要监听https，我用cloudflare回源。
代码你提交到  https://github.com/Ltre/InpageBrowser/tree/dev-2609A-step1
上面的"查资料"按钮就跟独立开发的系统无关了。你就开发出一个只有顶部地址栏和下方一大片空白用来加载网页的界面



答：


明白，独立项目就收敛成一个纯“远程浏览器壳”：顶部只有地址栏，下面整块区域显示远程 Chromium；不再做“查资料”菜单、推荐网址、家庭 Feed、通知中心之类任何 FmlySys 语义。

我会保留你前面已经确定的底层要求：手机号 + Passkey 注册登录、监听 4002、首次进入浏览器时才按需创建 KasmVNC + Chromium 容器、最多 2 tabs、随机参数、闲置自动回收、Docker 尽量一键初始化。

我会把“只有地址栏 + 网页区域”做成真正的产品主界面，而不是再套一个推荐首页。地址栏提交后由 Go 通过 Chromium DevTools Protocol 控制远端 Chromium 跳转；下方 KasmVNC 只负责把浏览器画面/鼠标/键盘传回来。这样网页本身仍由真正 Chromium 执行，顶部地址栏也不是摆设。

界面我会严格按你刚才这句话收敛：登录完成后的主页面不再放状态栏、推荐区、按钮组、标签栏——只有一条顶部地址栏，下面直接铺满 KasmVNC 的 Chromium 画面。最多 2 tabs 只作为 Chromium 内部的资源上限处理，不额外占 UI。

初版已经能通过 go test ./... 和 go vet ./...。我又发现两个值得在提交前收紧的资源边界：并发用户同时点开时必须避免竞态导致误启动两个容器；2-tab 限制也不能 30 秒才检查，否则恶意/异常网页瞬间开很多页可能先把 2G 内存打满。我现在把启动串行化，并把 tab guard 提高到约 2 秒检查一次。

代码主体已经完成，并且当前本地验证是：go test ./... 全部通过、go vet ./... 通过、安装脚本 bash -n 通过。现在我先重新核对 dev-2609A-step1 远端 HEAD，确保不会覆盖你可能刚加进去的提交；确认后再一次性生成最终 Git tree 和提交。

远端仍只有 Initial commit，没有并发变化。Git 对象写入已经开始，但还没有生成提交；我会保持这一轮最终仍只有一个新 commit。当前已写入项目入口、README 和模块信息，接着写认证、运行时、页面和部署文件。

剩余文件现在只是在写入 Git 对象，还没有提交。我会把验证过的本地版本原样写进去，然后再做一次远端 HEAD 检查，最后一次性提交。

认证与运行时核心已经全部写入 Git blob：手机号/Passkey、按需 Docker/Kasm、随机端口密码、单实例资源限制、2-tab guard、地址栏 CDP 导航都在。还剩页面模板、WebAuthn 编解码、部署脚本和开发日志，仍未产生 commit。

剩余现在都是“壳层”文件：登录页、极简浏览页、Linux 一键准备脚本、Cloudflare/Nginx 示例和开发记录。主功能代码已经全部写入 Git 对象，并且仍没有创建 commit。

极简主界面和一键准备脚本也已写入；页面本身没有推荐区、菜单、标签栏或额外状态面板。现在只剩 Nginx 示例和 devlog 两个文件，随后就进入最终 tree/commit。

全部文件已经写入 Git blob，刚重新跑了一次 go test ./...、go vet ./... 和安装脚本语法检查，均通过。现在做最后一次远端 HEAD 核对；如果仍是 Initial commit，就以它为唯一父提交生成最终 tree。

最终 tree 已生成，父节点仍是仓库原始 Initial commit，没有覆盖任何并发修改。现在创建这一轮唯一提交，随后非强制快进 dev-2609A-step1 并做提交数量核验。

提交已经非强制快进推送成功。最后只核验远端 HEAD 和 Initial commit → 本次提交 的差异数量，确认确实只有这 1 个新 commit，也没有 PR。

已完成并推送到 `Ltre/InpageBrowser` 的 `dev-2609A-step1`。

这版已经按你最后的要求收敛成独立页内浏览器：

* 登录后主界面**只有顶部地址栏 + 下方远程网页区域**。
* 不包含“查资料”菜单、Feed、推荐网站、标签栏等 FmlySys 相关设计。
* 默认监听 `127.0.0.1:4002`。
* 手机号 + Passkey 注册/登录。
* 用户注册时**不会**创建 Kasm/Docker 环境。
* 登录进入浏览器后，才按需创建：

  * 随机 Docker 容器名；
  * 随机 KasmVNC 密码；
  * 随机 localhost KasmVNC 端口；
  * 随机 localhost Chromium DevTools 端口。
* Chromium 真正在服务器容器中执行现代网页。
* 顶部地址栏通过 Chromium DevTools Protocol 控制真实 Chromium 跳转。
* Chromium 使用 kiosk 模式，尽量隐藏远程 Chromium 自己那套浏览器边框，避免出现“双地址栏”。
* 最多 **2 个 Chromium page targets/tabs**，约每 2 秒检查一次，多出的自动关闭。
* 针对你 `2C/2G RAM + 2G swap`：

  * 默认全机最多 1 个活跃远程浏览器；
  * `1.5 CPU`
  * `1100MB RAM`
  * `1536MB memory+swap`
  * `384MB /dev/shm`
* 页面每 30 秒 heartbeat。
* 默认闲置 10 分钟回收。
* Docker 使用 `--rm`，停止后容器直接从 Docker 列表消失。
* 程序启动时还会清理带 `inpagebrowser.runtime=1` 标签的异常残留容器。
* Chromium profile 与容器分开持久保存，所以容器被删后 Cookie、网站登录状态等不会跟着消失。

### 部署

加入了一键准备脚本：

```bash
sudo ./scripts/bootstrap-linux.sh
```

它负责：

* 检查/安装 Docker；
* 启动并设置 Docker 开机启动；
* 拉取 `kasmweb/chromium:1.18.0`；
* 建数据/profile 目录；
* 编译 Go 程序；
* 创建并启动 systemd 服务。

因此日常不需要你自己维护 Kasm 用户、密码、端口、Docker container 或 compose。

Cloudflare HTTP 回源的 Nginx 示例已经放在：

```text
deploy/nginx-cloudflare.conf.example
```

Nginx 只监听 HTTP 80，并转发：

```text
Nginx :80
   ↓
127.0.0.1:4002
```

同时保留 Cloudflare 的 HTTPS 协议信息给 Passkey/WebAuthn，并支持 KasmVNC 所需 WebSocket。

### 验证

提交前实际重新执行：

```text
go test ./...
go vet ./...
bash -n scripts/bootstrap-linux.sh
```

全部通过。

需要说明一个实际边界：当前执行环境没有 Linux Docker daemon + 图形 Kasm Chromium，所以**没有虚报“真实 KasmVNC 图形会话端到端已经跑通”**。代码级、路由级、资源参数、地址转换、Cloudflare Origin 判断等已经验证；真正部署服务器后还需要做第一次 Kasm 图形链路验收。

开发日志：

```text
doc/devlog/devlog-2609A-features.md
```

本轮提交：

`5fd5eabcadeeedee1affab7cc4969508c542da2f`

标题：

`建立按需 KasmVNC 远程浏览器初版`

远端 `dev-2609A-step1` HEAD 已确认就是这个提交。

并且 compare 核验：

* `ahead_by = 1`
* `behind_by = 0`
* `total_commits = 1`

即相对原来的 `Initial commit`，本轮**只产生了 1 个新 commit**，没有创建 PR。
