// Package mcproto implements the slice of the Minecraft Java protocol that
// server-list pings and login attempts exercise.
package mcproto

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

const maxPacket = 1 << 16

// ErrMalformed reports a packet that violates the protocol framing.
var ErrMalformed = errors.New("malformed packet")

// ReadVarInt reads a protocol VarInt.
func ReadVarInt(r io.ByteReader) (int32, error) {
	var out uint32
	for i := 0; i < 5; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		out |= uint32(b&0x7f) << (7 * i)
		if b&0x80 == 0 {
			return int32(out), nil
		}
	}
	return 0, fmt.Errorf("varint too long: %w", ErrMalformed)
}

// AppendVarInt appends v encoded as a VarInt.
func AppendVarInt(dst []byte, v int32) []byte {
	u := uint32(v)
	for u >= 0x80 {
		dst = append(dst, byte(u&0x7f)|0x80)
		u >>= 7
	}
	return append(dst, byte(u))
}

// AppendString appends s as a length-prefixed string.
func AppendString(dst []byte, s string) []byte {
	dst = AppendVarInt(dst, int32(len(s)))
	return append(dst, s...)
}

// ReadString reads a length-prefixed string from a packet body.
func ReadString(body *Reader) (string, error) {
	n, err := body.VarInt()
	if err != nil {
		return "", err
	}
	if n < 0 || int(n) > body.Len() {
		return "", fmt.Errorf("string length %d: %w", n, ErrMalformed)
	}
	return string(body.take(int(n))), nil
}

// Reader walks a packet body.
type Reader struct {
	b []byte
}

// NewReader wraps a packet body.
func NewReader(b []byte) *Reader { return &Reader{b: b} }

// Len returns the unread byte count.
func (r *Reader) Len() int { return len(r.b) }

// ReadByte implements io.ByteReader.
func (r *Reader) ReadByte() (byte, error) {
	if len(r.b) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	c := r.b[0]
	r.b = r.b[1:]
	return c, nil
}

// VarInt reads a VarInt from the body.
func (r *Reader) VarInt() (int32, error) { return ReadVarInt(r) }

// Int64 reads a big-endian int64.
func (r *Reader) Int64() (int64, error) {
	if len(r.b) < 8 {
		return 0, io.ErrUnexpectedEOF
	}
	var v int64
	for _, c := range r.take(8) {
		v = v<<8 | int64(c)
	}
	return v, nil
}

// Skip discards n bytes.
func (r *Reader) Skip(n int) error {
	if len(r.b) < n {
		return io.ErrUnexpectedEOF
	}
	r.take(n)
	return nil
}

func (r *Reader) take(n int) []byte {
	out := r.b[:n]
	r.b = r.b[n:]
	return out
}

// ReadPacket reads one framed packet and returns its id and body.
func ReadPacket(r *bufio.Reader) (int32, *Reader, error) {
	size, err := ReadVarInt(r)
	if err != nil {
		return 0, nil, err
	}
	if size <= 0 || size > maxPacket {
		return 0, nil, fmt.Errorf("packet size %d: %w", size, ErrMalformed)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, nil, err
	}
	body := NewReader(buf)
	id, err := body.VarInt()
	if err != nil {
		return 0, nil, err
	}
	return id, body, nil
}

// WritePacket frames and writes a packet.
func WritePacket(w io.Writer, id int32, payload []byte) error {
	inner := AppendVarInt(nil, id)
	inner = append(inner, payload...)
	frame := AppendVarInt(nil, int32(len(inner)))
	frame = append(frame, inner...)
	_, err := w.Write(frame)
	return err
}

// Handshake is the first packet of every connection.
type Handshake struct {
	Protocol  int32
	Host      string
	Port      uint16
	NextState int32
}

// Next states of a handshake.
const (
	StateStatus = 1
	StateLogin  = 2
)

// ParseHandshake decodes a handshake body.
func ParseHandshake(body *Reader) (Handshake, error) {
	var h Handshake
	var err error
	if h.Protocol, err = body.VarInt(); err != nil {
		return h, fmt.Errorf("handshake protocol: %w", err)
	}
	if h.Host, err = ReadString(body); err != nil {
		return h, fmt.Errorf("handshake host: %w", err)
	}
	hi, err := body.ReadByte()
	if err != nil {
		return h, fmt.Errorf("handshake port: %w", err)
	}
	lo, err := body.ReadByte()
	if err != nil {
		return h, fmt.Errorf("handshake port: %w", err)
	}
	h.Port = uint16(hi)<<8 | uint16(lo)
	if h.NextState, err = body.VarInt(); err != nil {
		return h, fmt.Errorf("handshake next state: %w", err)
	}
	return h, nil
}

// EncodeHandshake builds a handshake body.
func EncodeHandshake(h Handshake) []byte {
	out := AppendVarInt(nil, h.Protocol)
	out = AppendString(out, h.Host)
	out = append(out, byte(h.Port>>8), byte(h.Port))
	return AppendVarInt(out, h.NextState)
}
