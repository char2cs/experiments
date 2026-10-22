//go:build e2e && !windows

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/char2cs/experiments/internal/mcproto"
)

const (
	releaseBase = "https://github.com/char2cs/experiments/releases/download/${REF}"
	namespace   = "github.com/char2cs/experiments/mcserver@v0.0.1"

	minecraftNS = "github.com/char2cs/experiments/minecraft@v0.0.1"
	// javaNS is the identity minecraft's tools: edge declares.
	javaNS = "github.com/char2cs/experiments/java@v0.*"
)

// quiver is a real quiver binary driven through its CLI against an isolated
// home. Its daemon boots on first use, like for a user.
type quiver struct {
	t    *testing.T
	bin  string
	home string
}

func (q *quiver) run(args ...string) (string, error) {
	q.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, q.bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+q.home, "QUIVER_HOME="+filepath.Join(q.home, ".quiver"), "NO_COLOR=1")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func (q *quiver) must(args ...string) string {
	q.t.Helper()
	out, err := q.run(args...)
	if err != nil {
		q.t.Fatalf("quiver %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func (q *quiver) status(ns string) map[string]any {
	q.t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(q.must("status", ns, "-o", "json")), &doc); err != nil {
		q.t.Fatalf("decode status: %v", err)
	}
	return doc
}

// find returns the first string value stored under key anywhere in doc.
func find(doc any, key string) (string, bool) {
	switch v := doc.(type) {
	case map[string]any:
		if s, ok := v[key].(string); ok {
			return s, true
		}
		for _, child := range v {
			if s, ok := find(child, key); ok {
				return s, true
			}
		}
	case []any:
		for _, child := range v {
			if s, ok := find(child, key); ok {
				return s, true
			}
		}
	}
	return "", false
}

func startQuiver(t *testing.T) *quiver {
	t.Helper()
	bin := os.Getenv("QUIVER_BIN")
	if bin == "" {
		t.Skip("QUIVER_BIN is not set; build quiver.core and point it at the binary to run the daemon e2e")
	}
	abs, err := filepath.Abs(bin)
	if err != nil {
		t.Fatal(err)
	}
	// Unix socket paths are capped near 100 bytes, so the home must be short.
	home, err := os.MkdirTemp("", "qx")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".quiver"), 0o700); err != nil {
		t.Fatal(err)
	}
	q := &quiver{t: t, bin: abs, home: home}
	t.Cleanup(func() {
		if raw, err := os.ReadFile(filepath.Join(home, ".quiver", "quiver.pid")); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
				_ = syscall.Kill(pid, syscall.SIGTERM)
			}
		}
		time.Sleep(300 * time.Millisecond)
		_ = os.RemoveAll(home)
	})
	return q
}

// releaseServer serves the freshly built server and its checksums the way a
// GitHub release does, and returns the base URL the manifest should use.
func releaseServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin, err := os.ReadFile(buildServer(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	asset := fmt.Sprintf("mcserver-%s-%s", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(bin)
	files := map[string][]byte{
		"/" + asset:      bin,
		"/checksums.txt": []byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n"),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func pingPort(t *testing.T, port string) (mcproto.Status, error) {
	t.Helper()
	st, _, err := mcproto.Ping(ctxFor(t, 2*time.Second), "127.0.0.1:"+port)
	return st, err
}

// TestQuiver_FullLifecycle drives a real quiver.core daemon through the whole
// arrow: seed the manifest, install it (fetch + checksum), let Netbridge
// allocate the ports, run it with answers, talk to it like a Minecraft client,
// call its methods, stop it and uninstall it.
func TestQuiver_FullLifecycle(t *testing.T) {
	q := startQuiver(t)

	doc, err := ReadArrow("mcserver")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(doc, []byte(releaseBase)) {
		t.Fatal("arrows/mcserver.md no longer uses the release base this test rewrites")
	}
	seed := filepath.Join(t.TempDir(), "mcserver.md")
	rewritten := strings.ReplaceAll(string(doc), releaseBase, releaseServer(t))
	if err := os.WriteFile(seed, []byte(rewritten), 0o600); err != nil {
		t.Fatal(err)
	}

	step := func(name string, fn func(t *testing.T)) {
		t.Helper()
		if !t.Run(name, func(t *testing.T) {
			q.t = t
			fn(t)
		}) {
			t.FailNow()
		}
	}

	step("seed the manifest", func(t *testing.T) {
		q.must("arrow", "seed", namespace, "--file", seed)
		if out := q.must("info", namespace, "-o", "json"); !strings.Contains(out, "Quiver Test Minecraft Server") {
			t.Fatalf("info does not show the arrow:\n%s", out)
		}
	})

	step("install fetches, verifies and checks the binary", func(t *testing.T) {
		q.must("install", namespace)
		st := q.status(namespace)
		if st["state"] != "ready" {
			t.Fatalf("state %v", st["state"])
		}
		workdir, ok := find(st, "WORKDIR")
		if !ok {
			t.Fatal("no WORKDIR in status")
		}
		if _, err := os.Stat(filepath.Join(workdir, "mcserver")); err != nil {
			t.Fatalf("binary not installed: %v", err)
		}
	})

	step("netbridge allocates the declared ports", func(t *testing.T) {
		st := q.status(namespace)
		for _, name := range []string{"GAME_PORT", "QUERY_PORT"} {
			v, ok := find(st, name)
			if !ok {
				t.Fatalf("%s was never allocated: the daemon is not handing netbridge ports to the variable assembler", name)
			}
			if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 65535 {
				t.Fatalf("%s = %q", name, v)
			}
		}
	})

	var gamePort, queryPort string
	step("run starts the server with the answers it was given", func(t *testing.T) {
		q.must("run", namespace, "--detach", "--data", "MOTD=Hello from Quiver", "--data", "MAX_PLAYERS=33", "--data", "VERSION_NAME=Quiver e2e")
		eventually(t, 30*time.Second, "arrow reports running", func() error {
			st := q.status(namespace)
			if st["state"] != "running" {
				return fmt.Errorf("state %v", st["state"])
			}
			var ok bool
			if gamePort, ok = find(st, "GAME_PORT"); !ok {
				return fmt.Errorf("no GAME_PORT while running")
			}
			queryPort, _ = find(st, "QUERY_PORT")
			return nil
		})
	})

	step("a Minecraft client sees the variables", func(t *testing.T) {
		var st mcproto.Status
		eventually(t, 15*time.Second, "server answers a ping", func() error {
			var err error
			st, err = pingPort(t, gamePort)
			return err
		})
		if st.Description.Text != "Hello from Quiver" || st.Players.Max != 33 || st.Version.Name != "Quiver e2e" {
			t.Fatalf("status %+v", st)
		}
	})

	step("the query port is the allocated udp port", func(t *testing.T) {
		info, err := mcproto.Query(ctxFor(t, 3*time.Second), "127.0.0.1:"+queryPort)
		if err != nil {
			t.Fatal(err)
		}
		if strconv.Itoa(int(info.HostPort)) != gamePort || info.MaxPlayers != 33 {
			t.Fatalf("query %+v, game port %s", info, gamePort)
		}
	})

	step("custom methods run against the live server", func(t *testing.T) {
		q.must(namespace, "ping")
		q.must(namespace, "query")
	})

	step("stop shuts the server down", func(t *testing.T) {
		q.must("stop", namespace)
		eventually(t, 30*time.Second, "arrow back to ready", func() error {
			if s := q.status(namespace)["state"]; s != "ready" {
				return fmt.Errorf("state %v", s)
			}
			return nil
		})
		if _, err := pingPort(t, gamePort); err == nil {
			t.Fatal("port still answers after stop")
		}
	})

	step("uninstall removes the binary", func(t *testing.T) {
		workdir, _ := find(q.status(namespace), "WORKDIR")
		q.must("uninstall", namespace)
		if workdir != "" {
			if _, err := os.Stat(filepath.Join(workdir, "mcserver")); err == nil {
				t.Fatal("binary still present after uninstall")
			}
		}
	})
}

func arrowFile(name string) string { return filepath.Join("..", "arrows", name+".md") }

// TestQuiver_MinecraftWithJavaDependency installs the real Minecraft server
// arrow with no URL rewritten: the Java arrow of the same collection is pulled
// in through tools:, Mojang's jar is verified against the manifest checksum,
// and a real Minecraft status ping proves the variables took effect. It
// downloads about 110 MB, so it needs E2E_REAL=1.
func TestQuiver_MinecraftWithJavaDependency(t *testing.T) {
	if os.Getenv("E2E_REAL") != "1" {
		t.Skip("set E2E_REAL=1 to download the real Java runtime and Minecraft server")
	}
	q := startQuiver(t)

	step := func(name string, fn func(t *testing.T)) {
		t.Helper()
		if !t.Run(name, func(t *testing.T) {
			q.t = t
			fn(t)
		}) {
			t.FailNow()
		}
	}

	step("seed both arrows of the collection", func(t *testing.T) {
		q.must("arrow", "seed", javaNS, "--file", arrowFile("java"))
		q.must("arrow", "seed", minecraftNS, "--file", arrowFile("minecraft"))
	})

	step("installing minecraft installs its java dependency first", func(t *testing.T) {
		q.must("install", minecraftNS)
		if s := q.status(javaNS)["state"]; s != "ready" {
			t.Fatalf("java dependency state %v", s)
		}
		if s := q.status(minecraftNS)["state"]; s != "ready" {
			t.Fatalf("minecraft state %v", s)
		}
		workdir, _ := find(q.status(minecraftNS), "WORKDIR")
		if _, err := os.Stat(filepath.Join(workdir, "server.jar")); err != nil {
			t.Fatalf("server.jar not installed: %v", err)
		}
	})

	step("netbridge allocates the declared ports", func(t *testing.T) {
		st := q.status(minecraftNS)
		for _, name := range []string{"GAME_PORT", "QUERY_PORT"} {
			if _, ok := find(st, name); !ok {
				t.Fatalf("%s was never allocated: the daemon is not handing netbridge ports to the variable assembler", name)
			}
		}
	})

	step("the server does not start without the EULA", func(t *testing.T) {
		out, err := q.run("run", minecraftNS)
		if err == nil {
			t.Fatalf("run succeeded without ACCEPT_EULA:\n%s", out)
		}
	})

	var gamePort string
	step("run starts the real server with the answers it was given", func(t *testing.T) {
		q.must("run", minecraftNS, "--detach", "--data", "ACCEPT_EULA=true", "--data", "MOTD=Hello from Quiver",
			"--data", "MAX_PLAYERS=13", "--data", "ONLINE_MODE=false", "--data", "MEMORY_MB=768")
		eventually(t, 60*time.Second, "arrow reports running", func() error {
			st := q.status(minecraftNS)
			if st["state"] != "running" {
				return fmt.Errorf("state %v", st["state"])
			}
			var ok bool
			if gamePort, ok = find(st, "GAME_PORT"); !ok {
				return fmt.Errorf("no GAME_PORT while running")
			}
			return nil
		})
	})

	step("a Minecraft client sees the variables", func(t *testing.T) {
		var st mcproto.Status
		eventually(t, 3*time.Minute, "real server answers a ping", func() error {
			var err error
			st, err = pingPort(t, gamePort)
			return err
		})
		if st.Description.Text != "Hello from Quiver" || st.Players.Max != 13 {
			t.Fatalf("status %+v", st)
		}
	})

	step("stop saves the world and frees the port", func(t *testing.T) {
		q.must("stop", minecraftNS)
		eventually(t, 90*time.Second, "arrow back to ready", func() error {
			if s := q.status(minecraftNS)["state"]; s != "ready" {
				return fmt.Errorf("state %v", s)
			}
			return nil
		})
		if _, err := pingPort(t, gamePort); err == nil {
			t.Fatal("port still answers after stop")
		}
	})

	step("uninstall removes the server and keeps the dependency", func(t *testing.T) {
		q.must("uninstall", minecraftNS)
		if s := q.status(javaNS)["state"]; s != "ready" {
			t.Fatalf("java state %v after uninstalling its dependent", s)
		}
	})
}
