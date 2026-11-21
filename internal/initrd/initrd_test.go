package initrd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteListsWithCpio(t *testing.T) {
	cpio, err := exec.LookPath("cpio")
	if err != nil {
		t.Skip("cpio not installed")
	}
	var buf bytes.Buffer
	if err := Write(&buf, []byte("#!/bin/sh\necho agent\n")); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(cpio, "-t", "-v")
	cmd.Stdin = &buf
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cpio -t: %v\n%s", err, out)
	}
	for _, want := range []string{"dev/console", "init", "newroot", "proc", "sys"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("listing missing %q:\n%s", want, out)
		}
	}
}

func TestWriteAlignment(t *testing.T) {
	for n := range 8 {
		var buf bytes.Buffer
		if err := Write(&buf, bytes.Repeat([]byte{'x'}, n)); err != nil {
			t.Fatal(err)
		}
		if buf.Len()%4 != 0 {
			t.Errorf("agent of %d bytes: archive length %d not 4-aligned", n, buf.Len())
		}
		if !bytes.HasSuffix(bytes.TrimRight(buf.Bytes(), "\x00"), []byte("TRAILER!!!")) {
			t.Errorf("agent of %d bytes: missing trailer", n)
		}
	}
}

func TestEnsureCachesByContent(t *testing.T) {
	dir := t.TempDir()
	agent := filepath.Join(dir, "agent")
	if err := os.WriteFile(agent, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "cache")
	p1, err := Ensure(cache, agent)
	if err != nil {
		t.Fatal(err)
	}
	p1again, err := Ensure(cache, agent)
	if err != nil || p1again != p1 {
		t.Fatalf("second Ensure: %q, %v; want %q", p1again, err, p1)
	}
	if err := os.WriteFile(agent, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	p2, err := Ensure(cache, agent)
	if err != nil || p2 == p1 {
		t.Fatalf("changed agent: got %q, %v; want new path", p2, err)
	}
}
