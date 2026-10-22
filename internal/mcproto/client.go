package mcproto

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"
)

// Status is the decoded server-list response.
type Status struct {
	Version struct {
		Name     string `json:"name"`
		Protocol int32  `json:"protocol"`
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
	} `json:"players"`
	Description Chat `json:"description"`
}

// Chat is a text component. Servers send a bare string for plain text and an
// object for anything styled or composed, so both forms decode to Text.
type Chat struct {
	Text string `json:"text"`
}

// UnmarshalJSON accepts a JSON string or a component object, flattening the
// object's text and its extra children.
func (c *Chat) UnmarshalJSON(b []byte) error {
	var plain string
	if err := json.Unmarshal(b, &plain); err == nil {
		c.Text = plain
		return nil
	}
	var obj struct {
		Text  string `json:"text"`
		Extra []Chat `json:"extra"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return fmt.Errorf("chat component: %w", err)
	}
	c.Text = obj.Text
	for _, e := range obj.Extra {
		c.Text += e.Text
	}
	return nil
}

// Ping performs a server-list ping against addr and returns the status
// together with the round trip of the ping/pong exchange.
func Ping(ctx context.Context, addr string) (Status, time.Duration, error) {
	var st Status
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return st, 0, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return st, 0, fmt.Errorf("split %s: %w", addr, err)
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return st, 0, fmt.Errorf("port %q: %w", portStr, err)
	}

	hs := EncodeHandshake(Handshake{Protocol: -1, Host: host, Port: uint16(port), NextState: StateStatus})
	if err := WritePacket(conn, 0x00, hs); err != nil {
		return st, 0, fmt.Errorf("write handshake: %w", err)
	}
	if err := WritePacket(conn, 0x00, nil); err != nil {
		return st, 0, fmt.Errorf("write status request: %w", err)
	}

	br := bufio.NewReader(conn)
	id, body, err := ReadPacket(br)
	if err != nil {
		return st, 0, fmt.Errorf("read status response: %w", err)
	}
	if id != 0x00 {
		return st, 0, fmt.Errorf("status response id %#x: %w", id, ErrMalformed)
	}
	raw, err := ReadString(body)
	if err != nil {
		return st, 0, fmt.Errorf("status json: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return st, 0, fmt.Errorf("decode status: %w", err)
	}

	start := time.Now()
	token := start.UnixNano()
	payload := make([]byte, 8)
	for i := 0; i < 8; i++ {
		payload[i] = byte(token >> (8 * (7 - i)))
	}
	if err := WritePacket(conn, 0x01, payload); err != nil {
		return st, 0, fmt.Errorf("write ping: %w", err)
	}
	id, body, err = ReadPacket(br)
	if err != nil {
		return st, 0, fmt.Errorf("read pong: %w", err)
	}
	got, err := body.Int64()
	if id != 0x01 || err != nil || got != token {
		return st, 0, fmt.Errorf("pong mismatch: %w", ErrMalformed)
	}
	return st, time.Since(start), nil
}
