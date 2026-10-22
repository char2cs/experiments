package mcproto

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
)

// QueryInfo is the decoded basic-stat response of the UDP query protocol.
type QueryInfo struct {
	MOTD       string
	GameType   string
	Map        string
	NumPlayers int
	MaxPlayers int
	HostPort   uint16
	HostIP     string
}

// Query performs the UDP query handshake and a basic-stat request.
func Query(ctx context.Context, addr string) (QueryInfo, error) {
	var info QueryInfo
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return info, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

	const session = 0x01020304
	req := []byte{0xFE, 0xFD, 0x09}
	req = binary.BigEndian.AppendUint32(req, session)
	resp, err := roundTrip(conn, req)
	if err != nil {
		return info, fmt.Errorf("query handshake: %w", err)
	}
	if len(resp) < 6 || resp[0] != 0x09 {
		return info, fmt.Errorf("query handshake reply: %w", ErrMalformed)
	}
	token, err := strconv.ParseInt(string(bytes.TrimRight(resp[5:], "\x00")), 10, 32)
	if err != nil {
		return info, fmt.Errorf("challenge token: %w", ErrMalformed)
	}

	req = []byte{0xFE, 0xFD, 0x00}
	req = binary.BigEndian.AppendUint32(req, session)
	req = binary.BigEndian.AppendUint32(req, uint32(int32(token)))
	resp, err = roundTrip(conn, req)
	if err != nil {
		return info, fmt.Errorf("query stat: %w", err)
	}
	if len(resp) < 5 || resp[0] != 0x00 {
		return info, fmt.Errorf("query stat reply: %w", ErrMalformed)
	}
	return parseBasicStat(resp[5:])
}

func roundTrip(conn net.Conn, req []byte) ([]byte, error) {
	if _, err := conn.Write(req); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return buf[:n], nil
}

func parseBasicStat(b []byte) (QueryInfo, error) {
	var info QueryInfo
	fields := make([]string, 0, 5)
	for len(fields) < 5 {
		i := bytes.IndexByte(b, 0)
		if i < 0 {
			return info, fmt.Errorf("basic stat fields: %w", ErrMalformed)
		}
		fields = append(fields, string(b[:i]))
		b = b[i+1:]
	}
	if len(b) < 3 {
		return info, fmt.Errorf("basic stat tail: %w", ErrMalformed)
	}
	var err error
	if info.NumPlayers, err = strconv.Atoi(fields[3]); err != nil {
		return info, fmt.Errorf("numplayers: %w", ErrMalformed)
	}
	if info.MaxPlayers, err = strconv.Atoi(fields[4]); err != nil {
		return info, fmt.Errorf("maxplayers: %w", ErrMalformed)
	}
	info.MOTD, info.GameType, info.Map = fields[0], fields[1], fields[2]
	info.HostPort = binary.LittleEndian.Uint16(b[:2])
	info.HostIP = string(bytes.TrimRight(b[2:], "\x00"))
	return info, nil
}
