package proto

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	frames := []struct {
		typ byte
		p   []byte
	}{
		{Stdout, []byte("hello")},
		{Stderr, []byte{}},
		{Exit, ExitPayload(7)},
	}
	for _, f := range frames {
		if err := WriteFrame(&buf, f.typ, f.p); err != nil {
			t.Fatalf("WriteFrame: %v", err)
		}
	}
	for _, want := range frames {
		typ, p, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("ReadFrame: %v", err)
		}
		if typ != want.typ || !bytes.Equal(p, want.p) {
			t.Errorf("got (%d, %q), want (%d, %q)", typ, p, want.typ, want.p)
		}
	}
	if _, _, err := ReadFrame(&buf); !errors.Is(err, io.EOF) {
		t.Errorf("after last frame: got %v, want io.EOF", err)
	}
}

func TestWriteFrameTooLarge(t *testing.T) {
	if err := WriteFrame(io.Discard, Stdout, make([]byte, MaxFrame+1)); err == nil {
		t.Fatal("expected error for oversized frame")
	}
}

func TestReadFrameRejectsOversizedHeader(t *testing.T) {
	hdr := []byte{Stdout, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(hdr[1:], MaxFrame+1)
	if _, _, err := ReadFrame(bytes.NewReader(hdr)); err == nil {
		t.Fatal("expected error for oversized length")
	}
}

func TestReadFrameTruncated(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, Stdout, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	short := buf.Bytes()[:buf.Len()-2]
	if _, _, err := ReadFrame(bytes.NewReader(short)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("got %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestExitCodes(t *testing.T) {
	tests := []struct{ in, want int }{
		{0, 0},
		{1, 1},
		{137, 137},
		{-1, -1},
		{math.MaxInt32 + 10, math.MaxInt32},
		{math.MinInt32 - 10, math.MinInt32},
	}
	for _, tt := range tests {
		got, err := ParseExit(ExitPayload(tt.in))
		if err != nil {
			t.Fatalf("ParseExit(%d): %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("round trip %d: got %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseExitBadLength(t *testing.T) {
	for _, p := range [][]byte{nil, {1, 2, 3}, {1, 2, 3, 4, 5}} {
		if _, err := ParseExit(p); err == nil {
			t.Errorf("ParseExit(%v): expected error", p)
		}
	}
}
