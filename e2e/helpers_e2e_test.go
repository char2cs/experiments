//go:build e2e

package e2e

import (
	"context"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func exeName() string {
	if runtime.GOOS == "windows" {
		return "mcserver.exe"
	}
	return "mcserver"
}

// buildServer compiles cmd/mcserver into dir and returns the binary path.
func buildServer(t *testing.T, dir string) string {
	t.Helper()
	out := filepath.Join(dir, exeName())
	cmd := exec.Command("go", "build", "-ldflags", "-X main.version=e2e", "-o", out, "../cmd/mcserver")
	cmd.Env = append(cmd.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mcserver: %v\n%s", err, b)
	}
	return out
}

func freeTCP(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func freeUDP(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func ctxFor(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

// eventually polls fn until it returns nil or the deadline passes.
func eventually(t *testing.T, d time.Duration, what string, fn func() error) {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		err := fn()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %v", what, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
