package mcproto

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestVarInt_RoundTrip(t *testing.T) {
	for _, v := range []int32{0, 1, 127, 128, 255, 25565, 2097151, 2147483647, -1, -2147483648} {
		enc := AppendVarInt(nil, v)
		got, err := ReadVarInt(bytes.NewReader(enc))
		if err != nil || got != v {
			t.Fatalf("round trip %d: got %d err %v", v, got, err)
		}
	}
}

func TestVarInt_TooLong(t *testing.T) {
	_, err := ReadVarInt(bytes.NewReader([]byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x01}))
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("want ErrMalformed, got %v", err)
	}
}

func TestVarInt_Truncated(t *testing.T) {
	if _, err := ReadVarInt(bytes.NewReader([]byte{0x80})); !errors.Is(err, io.EOF) {
		t.Fatalf("want EOF, got %v", err)
	}
}

func TestReadString_Errors(t *testing.T) {
	testCases := []struct {
		name string
		in   []byte
	}{
		{"empty", nil},
		{"negative length", AppendVarInt(nil, -1)},
		{"longer than body", append(AppendVarInt(nil, 10), 'a')},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ReadString(NewReader(tc.in)); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestReadString_OK(t *testing.T) {
	got, err := ReadString(NewReader(AppendString(nil, "héllo")))
	if err != nil || got != "héllo" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestReader_Int64AndSkip(t *testing.T) {
	r := NewReader([]byte{0, 0, 0, 0, 0, 0, 1, 2, 9})
	v, err := r.Int64()
	if err != nil || v != 258 {
		t.Fatalf("got %d err %v", v, err)
	}
	if err := r.Skip(2); err == nil {
		t.Fatal("skip past end should fail")
	}
	if err := r.Skip(1); err != nil || r.Len() != 0 {
		t.Fatalf("skip: %v len %d", err, r.Len())
	}
	if _, err := r.Int64(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("want unexpected EOF, got %v", err)
	}
}

func TestPacket_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := WritePacket(&buf, 0x07, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	id, body, err := ReadPacket(bufio.NewReader(&buf))
	if err != nil || id != 7 || string(body.b) != "payload" {
		t.Fatalf("id %d body %q err %v", id, body.b, err)
	}
}

func TestReadPacket_Errors(t *testing.T) {
	testCases := []struct {
		name string
		in   []byte
	}{
		{"empty stream", nil},
		{"zero size", []byte{0}},
		{"oversize", AppendVarInt(nil, maxPacket+1)},
		{"truncated body", []byte{5, 0, 1}},
		{"no packet id", []byte{1, 0x80}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := ReadPacket(bufio.NewReader(bytes.NewReader(tc.in))); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestHandshake_RoundTrip(t *testing.T) {
	want := Handshake{Protocol: 767, Host: "localhost", Port: 25565, NextState: StateStatus}
	got, err := ParseHandshake(NewReader(EncodeHandshake(want)))
	if err != nil || got != want {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestParseHandshake_Truncated(t *testing.T) {
	full := EncodeHandshake(Handshake{Protocol: 1, Host: "h", Port: 2, NextState: 1})
	for cut := 0; cut < len(full); cut++ {
		if _, err := ParseHandshake(NewReader(full[:cut])); err == nil {
			t.Fatalf("cut at %d should fail", cut)
		}
	}
}

func TestParseBasicStat_Errors(t *testing.T) {
	testCases := []struct {
		name string
		in   []byte
	}{
		{"missing fields", []byte("a\x00b\x00")},
		{"missing tail", []byte("a\x00b\x00c\x001\x002\x00")},
		{"bad numplayers", []byte("a\x00b\x00c\x00x\x002\x00\x01\x00h\x00")},
		{"bad maxplayers", []byte("a\x00b\x00c\x001\x00x\x00\x01\x00h\x00")},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseBasicStat(tc.in); !errors.Is(err, ErrMalformed) {
				t.Fatalf("want ErrMalformed, got %v", err)
			}
		})
	}
}
