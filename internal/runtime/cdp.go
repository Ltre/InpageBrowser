package runtime

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type cdpTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func (m *Manager) targets(rt *Runtime) ([]cdpTarget, error) {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", rt.DevtoolsPort))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("devtools returned %s", resp.Status)
	}
	var all []cdpTarget
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return nil, err
	}
	pages := make([]cdpTarget, 0, len(all))
	for _, t := range all {
		if t.Type == "page" {
			pages = append(pages, t)
		}
	}
	return pages, nil
}

func (m *Manager) CurrentURL(rt *Runtime) (string, error) {
	targets, err := m.targets(rt)
	if err != nil {
		return "", err
	}
	if len(targets) == 0 {
		return "about:blank", nil
	}
	return targets[0].URL, nil
}

func (m *Manager) Navigate(rt *Runtime, rawURL string) error {
	targets, err := m.targets(rt)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("Chromium 页面尚未就绪")
	}
	payload, _ := json.Marshal(map[string]any{"id": 1, "method": "Page.navigate", "params": map[string]string{"url": rawURL}})
	return websocketCommand(targets[0].WebSocketDebuggerURL, rt.DevtoolsPort, payload)
}

func (m *Manager) enforceTabLimit(rt *Runtime) {
	targets, err := m.targets(rt)
	if err != nil || len(targets) <= m.tabLimit {
		return
	}
	client := http.Client{Timeout: 2 * time.Second}
	for _, t := range targets[m.tabLimit:] {
		req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("http://127.0.0.1:%d/json/close/%s", rt.DevtoolsPort, url.PathEscape(t.ID)), nil)
		resp, err := client.Do(req)
		if err == nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}
}

func websocketCommand(raw string, mappedPort int, payload []byte) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "ws" {
		return errors.New("unsupported devtools websocket scheme")
	}
	addr := fmt.Sprintf("127.0.0.1:%d", mappedPort)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	keyBytes := make([]byte, 16)
	_, _ = rand.Read(keyBytes)
	key := base64.StdEncoding.EncodeToString(keyBytes)
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", path, addr, key)
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.Contains(status, " 101 ") {
		return fmt.Errorf("devtools websocket rejected: %s", strings.TrimSpace(status))
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return err
		}
		if line == "\r\n" {
			break
		}
	}
	frame, err := maskedTextFrame(payload)
	if err != nil {
		return err
	}
	if _, err := conn.Write(frame); err != nil {
		return err
	}
	_ = conn.SetReadDeadline(time.Now().Add(800 * time.Millisecond))
	buf := make([]byte, 256)
	_, _ = conn.Read(buf)
	return nil
}

func maskedTextFrame(payload []byte) ([]byte, error) {
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return nil, err
	}
	out := []byte{0x81}
	n := len(payload)
	switch {
	case n < 126:
		out = append(out, byte(n)|0x80)
	case n <= 65535:
		out = append(out, 126|0x80, byte(n>>8), byte(n))
	default:
		out = append(out, 127|0x80)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		out = append(out, b[:]...)
	}
	out = append(out, mask...)
	for i, b := range payload {
		out = append(out, b^mask[i%4])
	}
	return out, nil
}
