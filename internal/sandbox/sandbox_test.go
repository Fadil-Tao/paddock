package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fadil-Tao/paddock/internal/proto"
)

func TestDefaultRoot(t *testing.T) {
	t.Setenv("PADDOCK_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/home/u")
	if got, want := DefaultRoot(), "/home/u/.local/share/paddock"; got != want {
		t.Errorf("fallback: got %q, want %q", got, want)
	}
	t.Setenv("XDG_DATA_HOME", "/xdg")
	if got, want := DefaultRoot(), "/xdg/paddock"; got != want {
		t.Errorf("XDG: got %q, want %q", got, want)
	}
	t.Setenv("PADDOCK_HOME", "/ph")
	if got, want := DefaultRoot(), "/ph"; got != want {
		t.Errorf("PADDOCK_HOME: got %q, want %q", got, want)
	}
}

func TestGetRejectsInvalidIDs(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	for _, id := range []string{"", "..", "../x", "a/b", `a\b`, "a.b"} {
		if _, err := m.Get(id); err == nil || !strings.Contains(err.Error(), "invalid") {
			t.Errorf("Get(%q): got %v, want invalid id error", id, err)
		}
	}
}

func TestGetMissing(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	if _, err := m.Get("deadbeef"); err == nil || !strings.Contains(err.Error(), "no such sandbox") {
		t.Fatalf("got %v, want no such sandbox", err)
	}
}

func TestListEmpty(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	sbs, err := m.List()
	if err != nil || len(sbs) != 0 {
		t.Fatalf("got %v, %v; want empty", sbs, err)
	}
}

func TestListReportsDeadSandboxes(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	older := &Sandbox{ID: "aaaa0000", PID: os.Getpid(), CreatedAt: time.Unix(100, 0)}
	newer := &Sandbox{ID: "bbbb1111", PID: 0, CreatedAt: time.Unix(200, 0)}
	for _, sb := range []*Sandbox{newer, older} {
		if err := os.MkdirAll(m.dir(sb.ID), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(filepath.Join(m.dir(sb.ID), "meta.json"), sb); err != nil {
			t.Fatal(err)
		}
	}
	sbs, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(sbs) != 2 || sbs[0].ID != older.ID || sbs[1].ID != newer.ID {
		t.Fatalf("got %+v, want [older, newer]", sbs)
	}
	// Our own PID is alive but is not this sandbox's firecracker, so neither
	// sandbox may be reported as running.
	for _, sb := range sbs {
		if sb.Running {
			t.Errorf("%s reported running", sb.ID)
		}
	}
}

func TestDestroyDeadSandboxRemovesDir(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	sb := &Sandbox{ID: "cccc2222"}
	if err := os.MkdirAll(m.dir(sb.ID), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(m.dir(sb.ID), "meta.json"), sb); err != nil {
		t.Fatal(err)
	}
	if err := m.Destroy(sb.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.dir(sb.ID)); !os.IsNotExist(err) {
		t.Fatalf("dir still exists: %v", err)
	}
}

func TestCreateMissingAssets(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	_, err := m.Create(context.Background(), Spec{})
	if err == nil || !strings.Contains(err.Error(), "missing asset") {
		t.Fatalf("got %v, want missing asset error", err)
	}
}

func TestExecNotRunning(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	sb := &Sandbox{ID: "dddd3333"}
	if err := os.MkdirAll(m.dir(sb.ID), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(m.dir(sb.ID), "meta.json"), sb); err != nil {
		t.Fatal(err)
	}
	_, err := m.Exec(context.Background(), sb.ID, proto.Request{Argv: []string{"true"}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("got %v, want not running", err)
	}
}

func TestTail(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(p, []byte("a\nb\nc\nd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := tail(p, 2); got != "c\nd" {
		t.Errorf("got %q", got)
	}
	if got := tail(p, 10); got != "a\nb\nc\nd" {
		t.Errorf("got %q", got)
	}
	if got := tail(filepath.Join(t.TempDir(), "missing"), 2); got != "" {
		t.Errorf("missing file: got %q", got)
	}
}
