//go:build e2e

// Package e2e boots real Firecracker VMs. It needs /dev/kvm, the assets
// built by `make assets` (default paddock data dir, or PADDOCK_ASSETS) and the
// agent built by `make build` (bin/paddock-agent, or PADDOCK_AGENT).
package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fadil-Tao/paddock/internal/proto"
	"github.com/Fadil-Tao/paddock/internal/sandbox"
)

func newManager(t *testing.T) *sandbox.Manager {
	t.Helper()
	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skipf("no KVM: %v", err)
	}
	assets := os.Getenv("PADDOCK_ASSETS")
	if assets == "" {
		assets = filepath.Join(sandbox.DefaultRoot(), "assets")
	}
	if _, err := os.Stat(filepath.Join(assets, "rootfs.ext4")); err != nil {
		t.Skipf("assets not built (run make assets): %v", err)
	}
	// Isolated data dir per test, sharing the built assets.
	root := t.TempDir()
	if err := os.Symlink(assets, filepath.Join(root, "assets")); err != nil {
		t.Fatal(err)
	}
	agent := os.Getenv("PADDOCK_AGENT")
	if agent == "" {
		agent = filepath.Join("..", "..", "bin", "paddock-agent")
	}
	if _, err := os.Stat(agent); err != nil {
		t.Fatalf("agent not built (run make build): %v", err)
	}
	return &sandbox.Manager{Root: root, Agent: agent}
}

func TestLifecycle(t *testing.T) {
	m := newManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	sb, err := m.Create(ctx, sandbox.Spec{CPUs: 1, MemMiB: 256, DiskBytes: 3 << 30})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = m.Destroy(sb.ID) })

	var out, errOut bytes.Buffer
	code, err := m.Exec(ctx, sb.ID, proto.Request{
		Argv: []string{"sh", "-c", "hostname; echo oops >&2; touch /root/f && ls /root; exit 5"},
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if code != 5 || out.String() != "paddock\nf\n" || errOut.String() != "oops\n" {
		t.Errorf("got code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}

	// The root filesystem was grown online to the requested disk size.
	out.Reset()
	if _, err := m.Exec(ctx, sb.ID, proto.Request{Argv: []string{"df", "-B1", "--output=size", "/"}}, &out, &errOut); err != nil {
		t.Fatalf("df: %v", err)
	}
	var size int64
	if _, err := fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out.String()), "1B-blocks")), "%d", &size); err != nil {
		t.Fatalf("parse df %q: %v", out.String(), err)
	}
	// df excludes ext4 metadata, so a 3GiB disk reports ~2.8GiB.
	if size < 2700<<20 {
		t.Errorf("root fs is %d bytes, want ~3GiB", size)
	}

	sbs, err := m.List()
	if err != nil || len(sbs) != 1 || !sbs[0].Running {
		t.Fatalf("List: %+v, %v", sbs, err)
	}

	if err := m.Destroy(sb.ID); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if sbs, _ := m.List(); len(sbs) != 0 {
		t.Fatalf("after Destroy: %+v", sbs)
	}
}
