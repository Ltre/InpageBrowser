package runtime

import (
	"strings"
	"testing"
)

func TestDockerRunArgsAreEphemeralAndBoundToLoopback(t *testing.T) {
	a := dockerRunArgs("ipb-test", "secret", "/srv/profile", "kasmweb/chromium:1.18.0")
	joined := strings.Join(a, " ")
	for _, want := range []string{"--rm", "127.0.0.1::6901", "127.0.0.1::9222", "--memory=1100m", "--cpus=1.5", "VNC_PW=secret", "--kiosk", "/srv/profile:/home/kasm-user"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
}

func TestProfileKeyStableAndOpaque(t *testing.T) {
	a := profileKey("user-1")
	b := profileKey("user-1")
	if a != b || strings.Contains(a, "user-1") || len(a) != 32 {
		t.Fatalf("bad profile key %q", a)
	}
}
