//go:build e2e && !windows

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/char2cs/experiments/internal/mcproto"
)

const javaExport = "github.com/char2cs/experiments/java.java"

// serverJar downloads the pinned Mojang jar named by the manifest, verifies
// it against the manifest's own checksum and caches it between runs.
func serverJar(t *testing.T, step Step) string {
	t.Helper()
	url, _ := Overrideable(step.URL, "linux/amd64")
	checksum, _ := Overrideable(step.Checksum, "linux/amd64")
	want := strings.TrimPrefix(checksum, "sha256:")
	cache := filepath.Join(os.TempDir(), "quiver-experiments-cache")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cache, want+".jar")
	if data, err := os.ReadFile(path); err == nil && sumHex(data) == want {
		return path
	}
	ctx := ctxFor(t, 5*time.Minute)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("download %s: %v", url, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("download %s: status %d err %v", url, resp.StatusCode, err)
	}
	if got := sumHex(data); got != want {
		t.Fatalf("manifest checksum is stale: %s hashes to %s, manifest says %s", url, got, want)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func sumHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// TestMinecraft_ManifestRunsTheRealServer renders the minecraft arrow's own
// execute steps and runs them against Mojang's real server, with the java on
// PATH standing in for the java arrow's export. It needs the network, a Java
// runtime and about a minute, so it only runs with E2E_REAL=1.
func TestMinecraft_ManifestRunsTheRealServer(t *testing.T) {
	if os.Getenv("E2E_REAL") != "1" {
		t.Skip("set E2E_REAL=1 to download and run the real Minecraft server")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("no java on PATH")
	}
	m := load(t, "minecraft")
	target := m.Targets["*"]

	dir := t.TempDir()
	jar, err := os.ReadFile(serverJar(t, target.Lifecycle["install"][0]))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server.jar"), jar, 0o600); err != nil {
		t.Fatal(err)
	}

	gamePort, queryPort := freeTCP(t), freeUDP(t)
	vars := map[string]string{
		"WORKDIR":      dir,
		"INSTALL_PATH": dir,
		"GAME_PORT":    strconv.Itoa(gamePort),
		"QUERY_PORT":   strconv.Itoa(queryPort),
		javaExport:     java,
	}
	for _, v := range m.Variables {
		vars[v.Name] = v.Default
	}
	vars["MOTD"] = "Hello from Quiver"
	vars["MAX_PLAYERS"] = "7"
	vars["DIFFICULTY"] = "hard"
	vars["GAME_MODE"] = "creative"
	vars["ONLINE_MODE"] = "false"
	vars["MEMORY_MB"] = "768"

	shell := func(ctx context.Context, step Step) *exec.Cmd {
		raw, ok := Overrideable(step.Command, "linux/amd64")
		if !ok {
			t.Fatalf("step %q has no unix command", step.Title)
		}
		return exec.CommandContext(ctx, "sh", "-c", Render(raw, vars))
	}
	execute := target.Lifecycle["execute"]

	t.Run("the server refuses to start until the EULA is accepted", func(t *testing.T) {
		vars["ACCEPT_EULA"] = "false"
		var out bytes.Buffer
		cmd := shell(ctxFor(t, 15*time.Second), execute[0])
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err == nil {
			t.Fatal("config step succeeded without ACCEPT_EULA=true")
		}
		if !strings.Contains(out.String(), "ACCEPT_EULA") {
			t.Fatalf("no explanation: %q", out.String())
		}
		if _, err := os.Stat(filepath.Join(dir, "eula.txt")); err == nil {
			t.Fatal("eula.txt was written without consent")
		}
	})

	vars["ACCEPT_EULA"] = "true"
	if out, err := shell(ctxFor(t, 15*time.Second), execute[0]).CombinedOutput(); err != nil {
		t.Fatalf("config step: %v\n%s", err, out)
	}

	t.Run("variables become server.properties", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(dir, "server.properties"))
		if err != nil {
			t.Fatal(err)
		}
		props := string(raw)
		for _, want := range []string{
			fmt.Sprintf("server-port=%d", gamePort), fmt.Sprintf("query.port=%d", queryPort), "enable-query=true",
			"motd=Hello from Quiver", "max-players=7", "difficulty=hard", "gamemode=creative", "online-mode=false",
		} {
			if !strings.Contains(props, want+"\n") {
				t.Errorf("server.properties lacks %q:\n%s", want, props)
			}
		}
		if eula, _ := os.ReadFile(filepath.Join(dir, "eula.txt")); !strings.Contains(string(eula), "eula=true") {
			t.Errorf("eula.txt %q", eula)
		}
	})

	var logs bytes.Buffer
	run := shell(context.Background(), execute[1])
	run.Stdout, run.Stderr = &logs, &logs
	run.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- run.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-run.Process.Pid, syscall.SIGKILL) })

	gameAddr := fmt.Sprintf("127.0.0.1:%d", gamePort)
	var st mcproto.Status
	eventually(t, 3*time.Minute, "real server answers a ping\n"+logs.String(), func() error {
		select {
		case err := <-exited:
			return fmt.Errorf("server exited early (%v):\n%s", err, logs.String())
		default:
		}
		var err error
		st, _, err = mcproto.Ping(ctxFor(t, 2*time.Second), gameAddr)
		return err
	})

	t.Run("a client sees the variables", func(t *testing.T) {
		if st.Description.Text != "Hello from Quiver" || st.Players.Max != 7 {
			t.Fatalf("status %+v", st)
		}
		if !strings.HasPrefix(st.Version.Name, "1.21") {
			t.Fatalf("version %q", st.Version.Name)
		}
	})

	t.Run("the query port is the allocated udp port", func(t *testing.T) {
		var info mcproto.QueryInfo
		eventually(t, 20*time.Second, "query answers", func() error {
			var err error
			info, err = mcproto.Query(ctxFor(t, 2*time.Second), fmt.Sprintf("127.0.0.1:%d", queryPort))
			return err
		})
		if int(info.HostPort) != gamePort || info.MaxPlayers != 7 {
			t.Fatalf("query %+v", info)
		}
	})

	t.Run("graceful stop saves the world and exits", func(t *testing.T) {
		stoppedAt := time.Now()
		if err := syscall.Kill(-run.Process.Pid, syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		select {
		case <-exited:
		case <-time.After(60 * time.Second):
			t.Fatalf("server did not stop on SIGTERM\n%s", logs.String())
		}
		// The server's shutdown hook saves the world but log4j is already torn
		// down by then, so the proof is the world file rewritten after the signal.
		info, err := os.Stat(filepath.Join(dir, "world", "level.dat"))
		if err != nil {
			t.Fatalf("world was not saved: %v", err)
		}
		if info.ModTime().Before(stoppedAt) {
			t.Fatalf("level.dat last written %s, before the stop at %s", info.ModTime(), stoppedAt)
		}
		if _, _, err := mcproto.Ping(ctxFor(t, time.Second), gameAddr); err == nil {
			t.Fatal("port still answers after stop")
		}
	})
}
