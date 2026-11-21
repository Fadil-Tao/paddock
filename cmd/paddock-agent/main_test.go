package main

import (
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/Fadil-Tao/paddock/internal/proto"
)

func TestMain(m *testing.M) {
	go reap()
	os.Exit(m.Run())
}

type result struct {
	stdout, stderr string
	exit           int
	errMsg         string
}

// run sends req (or raw bytes if req is a string) to handle over an in-memory
// connection and collects the frames it returns.
func run(t *testing.T, req any) result {
	t.Helper()
	host, guest := net.Pipe()
	defer host.Close()
	go handle(guest)

	var err error
	if raw, ok := req.(string); ok {
		_, err = host.Write([]byte(raw))
	} else {
		err = json.NewEncoder(host).Encode(req)
	}
	if err != nil {
		t.Fatalf("send request: %v", err)
	}

	var r result
	var out, errOut strings.Builder
	for {
		typ, p, err := proto.ReadFrame(host)
		if err != nil {
			t.Fatalf("ReadFrame: %v", err)
		}
		switch typ {
		case proto.Stdout:
			out.Write(p)
		case proto.Stderr:
			errOut.Write(p)
		case proto.Exit:
			r.exit, err = proto.ParseExit(p)
			if err != nil {
				t.Fatal(err)
			}
			r.stdout, r.stderr = out.String(), errOut.String()
			return r
		case proto.Error:
			r.errMsg = string(p)
			return r
		default:
			t.Fatalf("unexpected frame type %d", typ)
		}
	}
}

func TestHandleStreamsOutputAndExitCode(t *testing.T) {
	r := run(t, proto.Request{
		Argv: []string{"sh", "-c", "echo out; echo err >&2; exit 3"},
		Dir:  t.TempDir(),
	})
	if r.errMsg != "" {
		t.Fatalf("unexpected error frame: %s", r.errMsg)
	}
	if r.stdout != "out\n" || r.stderr != "err\n" || r.exit != 3 {
		t.Errorf("got stdout=%q stderr=%q exit=%d", r.stdout, r.stderr, r.exit)
	}
}

func TestHandleEnvAndDir(t *testing.T) {
	dir := t.TempDir()
	r := run(t, proto.Request{
		Argv: []string{"sh", "-c", `printf '%s %s' "$FOO" "$PWD"`},
		Env:  []string{"FOO=bar"},
		Dir:  dir,
	})
	if want := "bar " + dir; r.stdout != want {
		t.Errorf("got %q, want %q", r.stdout, want)
	}
}

func TestHandleSignalExitCode(t *testing.T) {
	r := run(t, proto.Request{Argv: []string{"sh", "-c", "kill -9 $$"}, Dir: t.TempDir()})
	if r.exit != 128+9 {
		t.Errorf("got exit %d, want 137", r.exit)
	}
}

func TestHandleErrors(t *testing.T) {
	tests := map[string]any{
		"unknown command": proto.Request{Argv: []string{"paddock-no-such-binary"}, Dir: t.TempDir()},
		"empty argv":      proto.Request{},
		"bad json":        "{not json\n",
	}
	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			if r := run(t, req); r.errMsg == "" {
				t.Errorf("expected error frame, got %+v", r)
			}
		})
	}
}

func TestHandleConcurrent(t *testing.T) {
	const n = 8
	done := make(chan result, n)
	for range n {
		go func() {
			done <- run(t, proto.Request{Argv: []string{"sh", "-c", "sleep 0.1; echo ok"}, Dir: t.TempDir()})
		}()
	}
	for range n {
		if r := <-done; r.stdout != "ok\n" || r.exit != 0 {
			t.Errorf("got %+v", r)
		}
	}
}
