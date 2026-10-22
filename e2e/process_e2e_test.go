//go:build e2e && !windows

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/char2cs/experiments/internal/mcproto"
)

// The execute and method commands below are the ones in ARROW.md, rendered the
// way Quiver renders them and run through the shell, so a manifest that
// drifts from the server's flags fails here before any Quiver is involved.
func TestProcess_ManifestCommandsDriveTheServer(t *testing.T) {
	m := load(t, "mcserver")
	target := m.Targets["*"]
	dir := t.TempDir()
	buildServer(t, dir)

	gamePort, queryPort := freeTCP(t), freeUDP(t)
	vars := map[string]string{
		"WORKDIR":      dir,
		"INSTALL_PATH": dir,
		"GAME_PORT":    strconv.Itoa(gamePort),
		"QUERY_PORT":   strconv.Itoa(queryPort),
	}
	for _, v := range m.Variables {
		vars[v.Name] = v.Default
	}
	vars["MOTD"] = "Hello from the e2e"
	vars["MAX_PLAYERS"] = "33"
	vars["VERSION_NAME"] = "Quiver e2e"
	vars["LOG_LEVEL"] = "debug"

	shell := func(ctx context.Context, step Step) *exec.Cmd {
		raw, ok := Overrideable(step.Command, "linux/amd64")
		if !ok {
			t.Fatalf("step %q has no unix command", step.Title)
		}
		return exec.CommandContext(ctx, "sh", "-c", Render(raw, vars))
	}

	t.Run("install check prints the stamped version", func(t *testing.T) {
		step := target.Lifecycle["install"][1]
		var out bytes.Buffer
		cmd := shell(ctxFor(t, 20*time.Second), step)
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		if strings.TrimSpace(out.String()) != "e2e" {
			t.Fatalf("version output %q", out.String())
		}
	})

	var logs bytes.Buffer
	run := shell(context.Background(), target.Lifecycle["execute"][0])
	run.Stdout, run.Stderr = &logs, &logs
	run.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- run.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-run.Process.Pid, syscall.SIGKILL) })

	gameAddr := fmt.Sprintf("127.0.0.1:%d", gamePort)
	eventually(t, 10*time.Second, "server answers a ping", func() error {
		_, _, err := mcproto.Ping(ctxFor(t, time.Second), gameAddr)
		return err
	})

	t.Run("variables reach the server list", func(t *testing.T) {
		st, _, err := mcproto.Ping(ctxFor(t, 3*time.Second), gameAddr)
		if err != nil {
			t.Fatal(err)
		}
		if st.Description.Text != "Hello from the e2e" || st.Players.Max != 33 || st.Version.Name != "Quiver e2e" {
			t.Fatalf("status %+v", st)
		}
	})

	t.Run("netbridge ports are the ones the server bound", func(t *testing.T) {
		info, err := mcproto.Query(ctxFor(t, 3*time.Second), fmt.Sprintf("127.0.0.1:%d", queryPort))
		if err != nil {
			t.Fatal(err)
		}
		if int(info.HostPort) != gamePort || info.MaxPlayers != 33 || info.MOTD != "Hello from the e2e" {
			t.Fatalf("query %+v", info)
		}
	})

	for _, name := range []string{"ping", "query"} {
		t.Run("method "+name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := shell(ctxFor(t, 10*time.Second), target.Methods[name].Steps[0])
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Run(); err != nil {
				t.Fatalf("%v\n%s", err, out.String())
			}
			if !strings.Contains(out.String(), "Hello from the e2e") || !strings.Contains(out.String(), "players 0/33") {
				t.Fatalf("output %q", out.String())
			}
		})
	}

	t.Run("graceful stop", func(t *testing.T) {
		if err := syscall.Kill(-run.Process.Pid, syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			t.Fatalf("server did not stop on SIGTERM\n%s", logs.String())
		}
		// The shell wrapper dies from the group signal, so only the server's
		// own shutdown log proves it stopped cleanly rather than was killed.
		if !strings.Contains(logs.String(), "stopped") {
			t.Fatalf("no clean shutdown in logs:\n%s", logs.String())
		}
		if _, _, err := mcproto.Ping(ctxFor(t, time.Second), gameAddr); err == nil {
			t.Fatal("port still answers after stop")
		}
	})
}
