package mcproto

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"
)

func ctx2(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// fakeTCP serves one connection with script and returns the listener address.
func fakeTCP(t *testing.T, script func(net.Conn, *bufio.Reader)) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		script(c, bufio.NewReader(c))
	}()
	return l.Addr().String()
}

func TestPing_Errors(t *testing.T) {
	testCases := []struct {
		name string
		addr func(t *testing.T) string
	}{
		{"nothing listening", func(t *testing.T) string {
			l, _ := net.Listen("tcp", "127.0.0.1:0")
			a := l.Addr().String()
			l.Close()
			return a
		}},
		{"bad address", func(t *testing.T) string { return "no-port" }},
		{"closes immediately", func(t *testing.T) string {
			return fakeTCP(t, func(net.Conn, *bufio.Reader) {})
		}},
		{"wrong status id", func(t *testing.T) string {
			return fakeTCP(t, func(c net.Conn, r *bufio.Reader) {
				_, _, _ = ReadPacket(r)
				_, _, _ = ReadPacket(r)
				_ = WritePacket(c, 0x05, nil)
			})
		}},
		{"status not a string", func(t *testing.T) string {
			return fakeTCP(t, func(c net.Conn, r *bufio.Reader) {
				_, _, _ = ReadPacket(r)
				_, _, _ = ReadPacket(r)
				_ = WritePacket(c, 0x00, nil)
			})
		}},
		{"status not json", func(t *testing.T) string {
			return fakeTCP(t, func(c net.Conn, r *bufio.Reader) {
				_, _, _ = ReadPacket(r)
				_, _, _ = ReadPacket(r)
				_ = WritePacket(c, 0x00, AppendString(nil, "nope"))
			})
		}},
		{"no pong", func(t *testing.T) string {
			return fakeTCP(t, func(c net.Conn, r *bufio.Reader) {
				_, _, _ = ReadPacket(r)
				_, _, _ = ReadPacket(r)
				_ = WritePacket(c, 0x00, AppendString(nil, "{}"))
			})
		}},
		{"wrong pong token", func(t *testing.T) string {
			return fakeTCP(t, func(c net.Conn, r *bufio.Reader) {
				_, _, _ = ReadPacket(r)
				_, _, _ = ReadPacket(r)
				_ = WritePacket(c, 0x00, AppendString(nil, "{}"))
				_, _, _ = ReadPacket(r)
				_ = WritePacket(c, 0x01, make([]byte, 8))
			})
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Ping(ctx2(t), tc.addr(t)); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestPing_BadPort(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	// A resolvable name with an out-of-range numeric port never dials, so the
	// port parse is exercised through a service name the dialer accepts.
	if _, _, err := Ping(ctx2(t), "127.0.0.1:http"); err == nil {
		t.Fatal("want error")
	}
}

func fakeUDP(t *testing.T, replies ...[]byte) string {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	go func() {
		buf := make([]byte, 1500)
		for _, rep := range replies {
			_, from, err := c.ReadFrom(buf)
			if err != nil {
				return
			}
			if rep != nil {
				_, _ = c.WriteTo(rep, from)
			}
		}
	}()
	return c.LocalAddr().String()
}

func TestQuery_Errors(t *testing.T) {
	good := []byte{0x09, 1, 2, 3, 4, '5', 0}
	testCases := []struct {
		name string
		addr func(t *testing.T) string
	}{
		{"bad address", func(t *testing.T) string { return "no-port" }},
		{"handshake timeout", func(t *testing.T) string { return fakeUDP(t, nil) }},
		{"handshake wrong type", func(t *testing.T) string { return fakeUDP(t, []byte{0x07, 1, 2, 3, 4, '5', 0}) }},
		{"handshake short", func(t *testing.T) string { return fakeUDP(t, []byte{0x09}) }},
		{"challenge not numeric", func(t *testing.T) string { return fakeUDP(t, []byte{0x09, 1, 2, 3, 4, 'x', 0}) }},
		{"stat timeout", func(t *testing.T) string { return fakeUDP(t, good, nil) }},
		{"stat wrong type", func(t *testing.T) string { return fakeUDP(t, good, []byte{0x09, 1, 2, 3, 4, 5}) }},
		{"stat short", func(t *testing.T) string { return fakeUDP(t, good, []byte{0x00}) }},
		{"stat garbage", func(t *testing.T) string { return fakeUDP(t, good, []byte{0x00, 1, 2, 3, 4, 'a', 0}) }},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			if _, err := Query(ctx, tc.addr(t)); err == nil {
				t.Fatal("want error")
			}
		})
	}
}
