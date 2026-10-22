package server

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/char2cs/experiments/internal/mcproto"
)

func start(t *testing.T, mutate func(*Config)) (*Server, context.CancelFunc, <-chan error) {
	t.Helper()
	cfg := Config{
		Host:        "127.0.0.1",
		Port:        freeTCPPort(t),
		QueryPort:   freeUDPPort(t),
		MOTD:        "hello quiver",
		MaxPlayers:  42,
		VersionName: "Quiver 9.9",
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if mutate != nil {
		mutate(&cfg)
	}
	srv, err := Listen(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	t.Cleanup(cancel)
	return srv, cancel, done
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func ctx5(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestServer_Ping_ReportsConfiguredIdentity(t *testing.T) {
	srv, _, _ := start(t, nil)
	st, rtt, err := mcproto.Ping(ctx5(t), srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	if st.Description.Text != "hello quiver" || st.Players.Max != 42 || st.Players.Online != 0 {
		t.Fatalf("unexpected status %+v", st)
	}
	if st.Version.Name != "Quiver 9.9" || st.Version.Protocol != ProtocolVersion {
		t.Fatalf("unexpected version %+v", st.Version)
	}
	if rtt <= 0 {
		t.Fatalf("rtt %v", rtt)
	}
}

func TestServer_Query_ReportsConfiguredIdentity(t *testing.T) {
	srv, _, _ := start(t, nil)
	info, err := mcproto.Query(ctx5(t), srv.QueryAddr())
	if err != nil {
		t.Fatal(err)
	}
	if info.MOTD != "hello quiver" || info.MaxPlayers != 42 || info.NumPlayers != 0 || info.GameType != "SMP" {
		t.Fatalf("unexpected info %+v", info)
	}
	if int(info.HostPort) != srv.cfg.Port || info.HostIP != "127.0.0.1" {
		t.Fatalf("unexpected host %+v", info)
	}
}

func TestServer_Query_DisabledWhenPortZero(t *testing.T) {
	srv, _, _ := start(t, func(c *Config) { c.QueryPort = 0 })
	if srv.QueryAddr() != "" {
		t.Fatalf("query addr %q", srv.QueryAddr())
	}
}

func TestServer_Query_IgnoresGarbage(t *testing.T) {
	srv, _, _ := start(t, nil)
	conn, err := net.Dial("udp", srv.QueryAddr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	for _, pkt := range [][]byte{
		{1, 2, 3},
		{0xFE, 0xFD, 0x07, 0, 0, 0, 1},
		{0xFE, 0xFD, 0x00, 0, 0, 0, 1, 0, 0, 0, 99},
		{0xFE, 0xFD, 0x00, 0, 0, 0, 1},
	} {
		if _, err := conn.Write(pkt); err != nil {
			t.Fatal(err)
		}
		if n, err := conn.Read(make([]byte, 64)); err == nil {
			t.Fatalf("packet %v got a %d byte reply", pkt, n)
		}
	}
}

func TestServer_Login_IsRefusedWithMessage(t *testing.T) {
	srv, _, _ := start(t, nil)
	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	hs := mcproto.EncodeHandshake(mcproto.Handshake{Protocol: 767, Host: "x", Port: 1, NextState: mcproto.StateLogin})
	if err := mcproto.WritePacket(conn, 0, hs); err != nil {
		t.Fatal(err)
	}
	id, body, err := mcproto.ReadPacket(bufio.NewReader(conn))
	if err != nil || id != 0 {
		t.Fatalf("id %d err %v", id, err)
	}
	msg, err := mcproto.ReadString(body)
	if err != nil || !strings.Contains(msg, "Quiver test server") || !strings.Contains(msg, "hello quiver") {
		t.Fatalf("msg %q err %v", msg, err)
	}
}

func TestServer_BadTraffic_ClosesConnection(t *testing.T) {
	srv, _, _ := start(t, nil)
	testCases := []struct {
		name string
		send func(net.Conn)
	}{
		{"wrong first packet", func(c net.Conn) { _ = mcproto.WritePacket(c, 0x05, nil) }},
		{"bad handshake body", func(c net.Conn) { _ = mcproto.WritePacket(c, 0x00, []byte{1}) }},
		{"unknown next state", func(c net.Conn) {
			_ = mcproto.WritePacket(c, 0, mcproto.EncodeHandshake(mcproto.Handshake{Host: "h", NextState: 9}))
		}},
		{"unknown status packet", func(c net.Conn) {
			_ = mcproto.WritePacket(c, 0, mcproto.EncodeHandshake(mcproto.Handshake{Host: "h", NextState: mcproto.StateStatus}))
			_ = mcproto.WritePacket(c, 0x09, nil)
		}},
		{"ping without token", func(c net.Conn) {
			_ = mcproto.WritePacket(c, 0, mcproto.EncodeHandshake(mcproto.Handshake{Host: "h", NextState: mcproto.StateStatus}))
			_ = mcproto.WritePacket(c, 0x01, []byte{1})
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := net.Dial("tcp", srv.Addr())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			tc.send(conn)
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			if _, err := io.ReadAll(conn); err != nil {
				t.Fatalf("connection was not closed cleanly: %v", err)
			}
		})
	}
}

func TestServer_Shutdown_ClosesIdleConnectionsAndStops(t *testing.T) {
	srv, cancel, done := start(t, nil)
	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not return after cancel")
	}
	if _, _, err := mcproto.Ping(ctx5(t), srv.Addr()); err == nil {
		t.Fatal("ping after shutdown should fail")
	}
}

func TestListen_Validation(t *testing.T) {
	good := Config{Port: 25565, MaxPlayers: 1, VersionName: "v"}
	testCases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"port zero", func(c *Config) { c.Port = 0 }},
		{"port too big", func(c *Config) { c.Port = 70000 }},
		{"query port negative", func(c *Config) { c.QueryPort = -1 }},
		{"query port too big", func(c *Config) { c.QueryPort = 70000 }},
		{"no slots", func(c *Config) { c.MaxPlayers = 0 }},
		{"no version", func(c *Config) { c.VersionName = "" }},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := good
			tc.mutate(&cfg)
			if _, err := Listen(cfg); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestListen_PortInUse(t *testing.T) {
	srv, _, _ := start(t, nil)
	_, err := Listen(Config{Host: "127.0.0.1", Port: srv.cfg.Port, MaxPlayers: 1, VersionName: "v"})
	if err == nil {
		t.Fatal("tcp conflict should fail")
	}
	_, err = Listen(Config{Host: "127.0.0.1", Port: freeTCPPort(t), QueryPort: srv.cfg.QueryPort, MaxPlayers: 1, VersionName: "v"})
	if err == nil {
		t.Fatal("udp conflict should fail")
	}
}

func TestBasicStat_FallsBackToLoopbackForNonIPHost(t *testing.T) {
	s := &Server{cfg: Config{Host: "localhost", Port: 25565, MOTD: "m", MaxPlayers: 3}}
	if got := string(s.basicStat()); !strings.HasSuffix(got, "127.0.0.1\x00") {
		t.Fatalf("got %q", got)
	}
}
