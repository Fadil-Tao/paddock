// Package proto defines the wire protocol between the paddock host and the
// in-guest agent, spoken over a vsock connection.
//
// The host sends one JSON-encoded Request, then reads frames until it gets an
// Exit or Error frame. Each frame is a 1-byte type, a 4-byte big-endian
// length, and the payload.
package proto

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Port is the vsock port the guest agent listens on.
const Port = 1024

// Request asks the agent to run a command.
type Request struct {
	Argv []string `json:"argv"`
	Env  []string `json:"env,omitempty"`
	Dir  string   `json:"dir,omitempty"`
}

// Frame types.
const (
	Stdout byte = 1
	Stderr byte = 2
	Exit   byte = 3 // payload: 4-byte big-endian exit code
	Error  byte = 4 // payload: error message; command did not run
)

// MaxFrame bounds a frame payload. Frames come from the untrusted guest, so
// readers reject anything larger.
const MaxFrame = 1 << 20

// WriteFrame writes one frame. Payloads over MaxFrame are rejected.
func WriteFrame(w io.Writer, typ byte, p []byte) error {
	if len(p) > MaxFrame {
		return fmt.Errorf("frame too large: %d bytes", len(p))
	}
	buf := make([]byte, 5+len(p))
	buf[0] = typ
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(p))) //nolint:gosec // bounded by MaxFrame above
	copy(buf[5:], p)
	_, err := w.Write(buf)
	return err
}

// ReadFrame reads one frame.
func ReadFrame(r io.Reader) (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > MaxFrame {
		return 0, nil, fmt.Errorf("frame too large: %d bytes", n)
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(r, p); err != nil {
		return 0, nil, err
	}
	return hdr[0], p, nil
}

// ExitPayload encodes an exit code. Codes outside int32 are clamped.
func ExitPayload(code int) []byte {
	code = max(math.MinInt32, min(math.MaxInt32, code))
	return binary.BigEndian.AppendUint32(nil, uint32(int32(code))) //nolint:gosec // clamped to int32 above; two's-complement round trip is intended
}

// ParseExit decodes an Exit frame payload.
func ParseExit(p []byte) (int, error) {
	if len(p) != 4 {
		return 0, fmt.Errorf("bad exit frame length %d", len(p))
	}
	return int(int32(binary.BigEndian.Uint32(p))), nil //nolint:gosec // intended two's-complement decode of ExitPayload
}
