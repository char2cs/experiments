// Package server is a Minecraft-protocol test server: it answers server-list
// pings and the UDP query protocol, and turns away login attempts.
package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/char2cs/experiments/internal/mcproto"
)

const (
	// ProtocolVersion is the protocol number advertised in the status.
	ProtocolVersion = 767
	connDeadline    = 10 * time.Second
)

// Config holds everything the manifest's variables and ports feed in.
type Config struct {
	Host        string
	Port        int
	QueryPort   int
	MOTD        string
	MaxPlayers  int
	VersionName string
	Logger      *slog.Logger
}

func (c Config) validate() error {
	switch {
	case c.Port < 1 || c.Port > 65535:
		return fmt.Errorf("port %d out of range", c.Port)
	case c.QueryPort < 0 || c.QueryPort > 65535:
		return fmt.Errorf("query port %d out of range", c.QueryPort)
	case c.MaxPlayers < 1:
		return fmt.Errorf("max players %d must be at least 1", c.MaxPlayers)
	case c.VersionName == "":
		return errors.New("version name is empty")
	}
	return nil
}

// Server is a running listener pair.
type Server struct {
	cfg   Config
	log   *slog.Logger
	tcp   net.Listener
	udp   net.PacketConn
	wg    sync.WaitGroup
	conns sync.Map
}

// Listen binds the TCP game port and, when configured, the UDP query port.
func Listen(cfg Config) (*Server, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	tcp, err := net.Listen("tcp", net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port)))
	if err != nil {
		return nil, fmt.Errorf("listen tcp: %w", err)
	}
	s := &Server{cfg: cfg, log: log, tcp: tcp}
	if cfg.QueryPort > 0 {
		udp, err := net.ListenPacket("udp", net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.QueryPort)))
		if err != nil {
			_ = tcp.Close()
			return nil, fmt.Errorf("listen udp: %w", err)
		}
		s.udp = udp
	}
	return s, nil
}

// Addr returns the TCP address.
func (s *Server) Addr() string { return s.tcp.Addr().String() }

// QueryAddr returns the UDP address, or "" when query is disabled.
func (s *Server) QueryAddr() string {
	if s.udp == nil {
		return ""
	}
	return s.udp.LocalAddr().String()
}

// Serve blocks until ctx is cancelled, then closes the listeners and waits for
// in-flight connections to finish.
func (s *Server) Serve(ctx context.Context) error {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		<-ctx.Done()
		_ = s.tcp.Close()
		if s.udp != nil {
			_ = s.udp.Close()
		}
		s.conns.Range(func(k, _ any) bool {
			_ = k.(net.Conn).Close()
			return true
		})
	}()
	if s.udp != nil {
		s.wg.Add(1)
		go s.serveQuery()
	}
	s.log.InfoContext(ctx, "listening", "addr", s.Addr(), "query", s.QueryAddr(), "motd", s.cfg.MOTD, "max_players", s.cfg.MaxPlayers)

	err := s.acceptLoop(ctx)
	s.wg.Wait()
	return err
}

func (s *Server) acceptLoop(ctx context.Context) error {
	for {
		conn, err := s.tcp.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		s.conns.Store(conn, struct{}{})
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer s.conns.Delete(conn)
			defer conn.Close()
			if err := s.handle(ctx, conn); err != nil && ctx.Err() == nil {
				s.log.DebugContext(ctx, "connection ended", "remote", conn.RemoteAddr().String(), "err", err)
			}
		}()
	}
}

func (s *Server) statusJSON() (string, error) {
	var st mcproto.Status
	st.Version.Name = s.cfg.VersionName
	st.Version.Protocol = ProtocolVersion
	st.Players.Max = s.cfg.MaxPlayers
	st.Description.Text = s.cfg.MOTD
	raw, err := json.Marshal(st)
	if err != nil {
		return "", fmt.Errorf("marshal status: %w", err)
	}
	return string(raw), nil
}

func (s *Server) handle(ctx context.Context, conn net.Conn) error {
	_ = conn.SetDeadline(time.Now().Add(connDeadline))
	br := bufio.NewReader(conn)

	id, body, err := mcproto.ReadPacket(br)
	if err != nil {
		return fmt.Errorf("read handshake: %w", err)
	}
	if id != 0x00 {
		return fmt.Errorf("first packet id %#x: %w", id, mcproto.ErrMalformed)
	}
	hs, err := mcproto.ParseHandshake(body)
	if err != nil {
		return fmt.Errorf("parse handshake: %w", err)
	}
	switch hs.NextState {
	case mcproto.StateStatus:
		return s.serveStatus(ctx, conn, br)
	case mcproto.StateLogin:
		return s.refuseLogin(ctx, conn)
	default:
		return fmt.Errorf("next state %d: %w", hs.NextState, mcproto.ErrMalformed)
	}
}

func (s *Server) serveStatus(ctx context.Context, conn net.Conn, br *bufio.Reader) error {
	for {
		id, body, err := mcproto.ReadPacket(br)
		if err != nil {
			return fmt.Errorf("read status packet: %w", err)
		}
		switch id {
		case 0x00:
			raw, err := s.statusJSON()
			if err != nil {
				return err
			}
			if err := mcproto.WritePacket(conn, 0x00, mcproto.AppendString(nil, raw)); err != nil {
				return fmt.Errorf("write status: %w", err)
			}
			s.log.DebugContext(ctx, "status served", "remote", conn.RemoteAddr().String())
		case 0x01:
			token, err := body.Int64()
			if err != nil {
				return fmt.Errorf("ping token: %w", err)
			}
			payload := make([]byte, 8)
			for i := range payload {
				payload[i] = byte(token >> (8 * (7 - i)))
			}
			if err := mcproto.WritePacket(conn, 0x01, payload); err != nil {
				return fmt.Errorf("write pong: %w", err)
			}
			return nil
		default:
			return fmt.Errorf("status packet id %#x: %w", id, mcproto.ErrMalformed)
		}
	}
}

func (s *Server) refuseLogin(ctx context.Context, conn net.Conn) error {
	msg, err := json.Marshal(map[string]string{"text": "This is a Quiver test server. " + s.cfg.MOTD})
	if err != nil {
		return fmt.Errorf("marshal disconnect: %w", err)
	}
	s.log.DebugContext(ctx, "login refused", "remote", conn.RemoteAddr().String())
	if err := mcproto.WritePacket(conn, 0x00, mcproto.AppendString(nil, string(msg))); err != nil {
		return fmt.Errorf("write disconnect: %w", err)
	}
	return nil
}
