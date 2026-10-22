package main

import (
	"bytes"
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
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

func TestRun_Dispatch_Errors(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"unknown command", []string{"dance"}},
		{"serve bad flag", []string{"serve", "--nope"}},
		{"serve bad log level", []string{"serve", "--log-level", "loud"}},
		{"serve bad port", []string{"serve", "--port", "0"}},
		{"ping bad flag", []string{"ping", "--nope"}},
		{"ping nothing listening", []string{"ping", "--port", strconv.Itoa(freePort(t)), "--timeout", "300ms"}},
		{"query bad flag", []string{"query", "--nope"}},
		{"query nothing listening", []string{"query", "--port", strconv.Itoa(freeUDP(t)), "--timeout", "300ms"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if err := run(context.Background(), tc.args, &out, &errOut); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestRun_Version(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"version"}, &out, &out); err != nil || strings.TrimSpace(out.String()) != "dev" {
		t.Fatalf("out %q err %v", out.String(), err)
	}
}

func TestRun_ServeThenPingAndQuery(t *testing.T) {
	port, qport := freePort(t), freeUDP(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	var logs bytes.Buffer
	go func() {
		done <- run(ctx, []string{
			"serve", "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--query-port", strconv.Itoa(qport),
			"--motd", "from main", "--max-players", "7", "--log-level", "debug",
		}, &bytes.Buffer{}, &logs)
	}()

	var pingOut, queryOut bytes.Buffer
	deadline := time.Now().Add(3 * time.Second)
	for {
		pingOut.Reset()
		err := run(ctx, []string{"ping", "--port", strconv.Itoa(port)}, &pingOut, &pingOut)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never answered: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(pingOut.String(), "from main") || !strings.Contains(pingOut.String(), "players 0/7") {
		t.Fatalf("ping output %q", pingOut.String())
	}
	if err := run(ctx, []string{"query", "--port", strconv.Itoa(qport)}, &queryOut, &queryOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryOut.String(), "from main") || !strings.Contains(queryOut.String(), "players 0/7") {
		t.Fatalf("query output %q", queryOut.String())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not stop")
	}
	if !strings.Contains(logs.String(), "stopped") {
		t.Fatalf("logs %q", logs.String())
	}
}
