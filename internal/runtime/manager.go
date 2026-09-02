package runtime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Runtime struct {
	UserID       string
	Container    string
	Password     string
	KasmPort     int
	DevtoolsPort int
	LastSeen     time.Time
	StartedAt    time.Time
}

type Manager struct {
	mu        sync.Mutex
	startMu   sync.Mutex
	dataDir   string
	image     string
	maxActive int
	tabLimit  int
	idle      time.Duration
	runtimes  map[string]*Runtime
	stop      chan struct{}
}

func NewManager(dataDir string) *Manager {
	return &Manager{
		dataDir:   dataDir,
		image:     envString("INPAGE_BROWSER_IMAGE", "kasmweb/chromium:1.18.0"),
		maxActive: envInt("INPAGE_MAX_ACTIVE", 1),
		tabLimit:  2,
		idle:      time.Duration(envInt("INPAGE_IDLE_MINUTES", 10)) * time.Minute,
		runtimes:  map[string]*Runtime{},
		stop:      make(chan struct{}),
	}
}

func (m *Manager) Start(ctx context.Context) {
	_ = m.cleanupStale(ctx)
	go func() {
		reapTicker := time.NewTicker(30 * time.Second)
		guardTicker := time.NewTicker(2 * time.Second)
		defer reapTicker.Stop()
		defer guardTicker.Stop()
		for {
			select {
			case <-reapTicker.C:
				m.reap()
			case <-guardTicker.C:
				m.guardTabs()
			case <-m.stop:
				return
			}
		}
	}()
}

func (m *Manager) Close() {
	close(m.stop)
	m.mu.Lock()
	list := make([]*Runtime, 0, len(m.runtimes))
	for _, rt := range m.runtimes {
		list = append(list, rt)
	}
	m.runtimes = map[string]*Runtime{}
	m.mu.Unlock()
	for _, rt := range list {
		_ = dockerStop(rt.Container)
	}
}

func (m *Manager) Ensure(ctx context.Context, userID string) (*Runtime, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	if rt := m.runtimes[userID]; rt != nil {
		rt.LastSeen = time.Now()
		m.mu.Unlock()
		return rt, nil
	}
	if len(m.runtimes) >= m.maxActive {
		m.mu.Unlock()
		return nil, errors.New("服务器当前已有浏览器实例在使用，请稍后再试")
	}
	m.mu.Unlock()
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, errors.New("Docker 尚未安装；请先运行 scripts/bootstrap-linux.sh")
	}
	profile := filepath.Join(m.dataDir, "profiles", profileKey(userID))
	if err := os.MkdirAll(profile, 0777); err != nil {
		return nil, err
	}
	_ = os.Chmod(profile, 0777)
	name := "ipb-" + randomString(8)
	password := randomString(24)
	out, err := exec.CommandContext(ctx, "docker", dockerRunArgs(name, password, profile, m.image)...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("启动 Chromium 容器失败: %v: %s", err, strings.TrimSpace(string(out)))
	}
	kasmPort, err := dockerMappedPort(ctx, name, "6901/tcp")
	if err != nil {
		_ = dockerStop(name)
		return nil, err
	}
	devPort, err := dockerMappedPort(ctx, name, "9222/tcp")
	if err != nil {
		_ = dockerStop(name)
		return nil, err
	}
	rt := &Runtime{UserID: userID, Container: name, Password: password, KasmPort: kasmPort, DevtoolsPort: devPort, StartedAt: time.Now(), LastSeen: time.Now()}
	if err := waitReady(ctx, rt); err != nil {
		_ = dockerStop(name)
		return nil, err
	}
	m.mu.Lock()
	if existing := m.runtimes[userID]; existing != nil {
		m.mu.Unlock()
		_ = dockerStop(name)
		return existing, nil
	}
	m.runtimes[userID] = rt
	m.mu.Unlock()
	return rt, nil
}

func (m *Manager) Get(userID string) (*Runtime, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rt, ok := m.runtimes[userID]
	if ok {
		rt.LastSeen = time.Now()
	}
	return rt, ok
}

func (m *Manager) Heartbeat(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rt := m.runtimes[userID]; rt != nil {
		rt.LastSeen = time.Now()
	}
}

func (m *Manager) StopUser(userID string) {
	m.mu.Lock()
	rt := m.runtimes[userID]
	delete(m.runtimes, userID)
	m.mu.Unlock()
	if rt != nil {
		_ = dockerStop(rt.Container)
	}
}

func (m *Manager) Proxy(rt *Runtime) *httputil.ReverseProxy {
	target, _ := url.Parse(fmt.Sprintf("https://127.0.0.1:%d", rt.KasmPort))
	p := httputil.NewSingleHostReverseProxy(target)
	p.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	orig := p.Director
	p.Director = func(r *http.Request) {
		orig(r)
		r.Host = target.Host
		r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("kasm_user:"+rt.Password)))
	}
	p.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, "远程浏览器连接中断: "+err.Error(), http.StatusBadGateway)
	}
	return p
}

func dockerRunArgs(name, password, profile, image string) []string {
	return []string{
		"run", "-d", "--rm", "--name", name,
		"--label", "inpagebrowser.runtime=1",
		"--shm-size=384m", "--memory=1100m", "--memory-swap=1536m", "--cpus=1.5", "--pids-limit=512",
		"-p", "127.0.0.1::6901", "-p", "127.0.0.1::9222",
		"-e", "VNC_PW=" + password,
		"-e", "LAUNCH_URL=about:blank",
		"-e", "APP_ARGS=--kiosk --no-first-run --no-default-browser-check --disable-session-crashed-bubble --remote-debugging-address=0.0.0.0 --remote-debugging-port=9222 --remote-allow-origins=*",
		"-v", profile + ":/home/kasm-user",
		image,
	}
}

func dockerMappedPort(ctx context.Context, name, port string) (int, error) {
	out, err := exec.CommandContext(ctx, "docker", "port", name, port).Output()
	if err != nil {
		return 0, err
	}
	line := strings.TrimSpace(string(out))
	idx := strings.LastIndex(line, ":")
	if idx < 0 {
		return 0, fmt.Errorf("无法解析 Docker 端口: %q", line)
	}
	return strconv.Atoi(line[idx+1:])
}

func waitReady(ctx context.Context, rt *Runtime) error {
	deadline := time.Now().Add(30 * time.Second)
	kasmClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, Timeout: 2 * time.Second}
	devClient := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d/", rt.KasmPort), nil)
		req.SetBasicAuth("kasm_user", rt.Password)
		resp1, err1 := kasmClient.Do(req)
		if err1 == nil {
			resp1.Body.Close()
		}
		resp2, err2 := devClient.Get(fmt.Sprintf("http://127.0.0.1:%d/json", rt.DevtoolsPort))
		if err2 == nil {
			resp2.Body.Close()
		}
		if err1 == nil && err2 == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return errors.New("Chromium/KasmVNC 在 30 秒内没有就绪")
}

func (m *Manager) reap() {
	now := time.Now()
	var stale []*Runtime
	m.mu.Lock()
	for uid, rt := range m.runtimes {
		if now.Sub(rt.LastSeen) > m.idle {
			stale = append(stale, rt)
			delete(m.runtimes, uid)
		}
	}
	m.mu.Unlock()
	for _, rt := range stale {
		_ = dockerStop(rt.Container)
	}
}

func (m *Manager) guardTabs() {
	m.mu.Lock()
	list := make([]*Runtime, 0, len(m.runtimes))
	for _, rt := range m.runtimes {
		list = append(list, rt)
	}
	m.mu.Unlock()
	for _, rt := range list {
		m.enforceTabLimit(rt)
	}
}

func (m *Manager) cleanupStale(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil
	}
	out, err := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "label=inpagebrowser.runtime=1").Output()
	if err != nil {
		return nil
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil
	}
	args := append([]string{"rm", "-f"}, ids...)
	_ = exec.CommandContext(ctx, "docker", args...).Run()
	return nil
}

func dockerStop(name string) error {
	if name == "" {
		return nil
	}
	return exec.Command("docker", "stop", "-t", "3", name).Run()
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func profileKey(userID string) string {
	sum := sha256.Sum256([]byte("inpagebrowser-profile:" + userID))
	return fmt.Sprintf("%x", sum[:16])
}

func envString(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

func envInt(k string, d int) int {
	v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(k)))
	if err == nil && v > 0 {
		return v
	}
	return d
}
