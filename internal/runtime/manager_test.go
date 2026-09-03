package runtime

import (
	"strings"
	"testing"
)

func TestDockerRunArgsLinuxHostNetworkAvoidsPublishedPorts(t *testing.T) {
	a := dockerRunArgsForNetwork("ipb-test", "secret", "/srv/profile", "kasmweb/chromium:1.18.0", true)
	joined := strings.Join(a, " ")
	for _, want := range []string{"--rm", "--network host", "--memory=1100m", "--cpus=1.5", "VNC_PW=secret", "--kiosk", "--remote-debugging-address=127.0.0.1", "/srv/profile:/home/kasm-user"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
	for _, unwanted := range []string{"127.0.0.1::6901", "127.0.0.1::9222", " -p "} {
		if strings.Contains(" "+joined+" ", unwanted) {
			t.Fatalf("unexpected %q in %s", unwanted, joined)
		}
	}
}

func TestDockerRunArgsBridgeKeepsRandomLoopbackMappings(t *testing.T) {
	a := dockerRunArgsForNetwork("ipb-test", "secret", "/srv/profile", "kasmweb/chromium:1.18.0", false)
	joined := strings.Join(a, " ")
	for _, want := range []string{"127.0.0.1::6901", "127.0.0.1::9222", "--remote-debugging-address=0.0.0.0"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
}

func TestDockerRunArgsAddsBrowserProxyInBridgeMode(t *testing.T) {
	t.Setenv("INPAGE_BROWSER_PROXY", "socks5://host.docker.internal:51837")
	a := dockerRunArgsForNetwork("ipb-test", "secret", "/srv/profile", "kasmweb/chromium:1.18.0", false)
	joined := strings.Join(a, " ")
	for _, want := range []string{"--add-host host.docker.internal:host-gateway", "--proxy-server=socks5://host.docker.internal:51837"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
}

func TestDirectBrowserProxyDoesNotAddProxyServer(t *testing.T) {
	t.Setenv("INPAGE_BROWSER_PROXY", "direct://")
	a := dockerRunArgsForNetwork("ipb-test", "secret", "/srv/profile", "kasmweb/chromium:1.18.0", false)
	joined := strings.Join(a, " ")
	if strings.Contains(joined, "--proxy-server=") {
		t.Fatalf("unexpected proxy server in %s", joined)
	}
}

func TestProfileKeyStableAndOpaque(t *testing.T) {
	a := profileKey("user-1")
	b := profileKey("user-1")
	if a != b || strings.Contains(a, "user-1") || len(a) != 32 {
		t.Fatalf("bad profile key %q", a)
	}
}
