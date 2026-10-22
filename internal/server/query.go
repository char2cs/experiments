package server

import (
	"encoding/binary"
	"net"
	"strconv"
)

// Query packet types of the GameSpy4 protocol Minecraft implements.
const (
	queryStat      = 0x00
	queryHandshake = 0x09
)

var queryMagic = [2]byte{0xFE, 0xFD}

// challengeFor derives the challenge token handed out to a session. The
// protocol only requires that it round-trips, so it is a pure function of the
// session id rather than server state.
func challengeFor(session uint32) int32 {
	return int32(session&0x0f0f0f0f) + 1
}

func (s *Server) serveQuery() {
	defer s.wg.Done()
	buf := make([]byte, 1500)
	for {
		n, from, err := s.udp.ReadFrom(buf)
		if err != nil {
			return
		}
		reply := s.queryReply(buf[:n])
		if reply != nil {
			_, _ = s.udp.WriteTo(reply, from)
		}
	}
}

func (s *Server) queryReply(pkt []byte) []byte {
	if len(pkt) < 7 || pkt[0] != queryMagic[0] || pkt[1] != queryMagic[1] {
		return nil
	}
	kind := pkt[2]
	session := binary.BigEndian.Uint32(pkt[3:7])
	head := append([]byte{kind}, pkt[3:7]...)

	switch kind {
	case queryHandshake:
		return append(append(head, strconv.Itoa(int(challengeFor(session)))...), 0)
	case queryStat:
		if len(pkt) < 11 || int32(binary.BigEndian.Uint32(pkt[7:11])) != challengeFor(session) {
			return nil
		}
		return append(head, s.basicStat()...)
	default:
		return nil
	}
}

func (s *Server) basicStat() []byte {
	var out []byte
	for _, f := range []string{s.cfg.MOTD, "SMP", "quiver", "0", strconv.Itoa(s.cfg.MaxPlayers)} {
		out = append(out, f...)
		out = append(out, 0)
	}
	out = binary.LittleEndian.AppendUint16(out, uint16(s.cfg.Port))
	host := s.cfg.Host
	if host == "" || net.ParseIP(host) == nil {
		host = "127.0.0.1"
	}
	out = append(out, host...)
	return append(out, 0)
}
