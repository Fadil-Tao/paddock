package image

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSizeMiB(t *testing.T) {
	tests := map[int64]int64{
		0:         64,
		100 << 20: 189,
		80 << 20:  164,
	}
	for in, want := range tests {
		if got := SizeMiB(in); got != want {
			t.Errorf("SizeMiB(%d) = %d, want %d", in, got, want)
		}
	}
}

// fakeEngine installs a shell script named name on PATH that logs its
// arguments and fakes `create` and `export`.
func fakeEngine(t *testing.T, name string) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "calls")
	script := `#!/bin/sh
echo "$*" >> ` + logPath + `
case "$1" in
  create) echo cid123 ;;
  export) tar -cf "$3" -T /dev/null ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestBuildDrivesEngineAndCleansUp(t *testing.T) {
	if testing.Short() {
		t.Skip("runs mke2fs")
	}
	logPath := fakeEngine(t, "fake-engine")
	out := filepath.Join(t.TempDir(), "rootfs.ext4")
	b := &Builder{Engine: "fake-engine"}
	if err := b.Build(context.Background(), "ctxdir", out); err != nil {
		if strings.Contains(err.Error(), "mke2fs not found") {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	if st, err := os.Stat(out); err != nil || st.Size() != 64<<20 {
		t.Fatalf("image: %v, %v", st, err)
	}
	calls, _ := os.ReadFile(logPath)
	got := string(calls)
	for _, want := range []string{"build -t paddock-build-", " ctxdir\n", "create paddock-build-", "export -o ", "rm cid123", "rmi paddock-build-"} {
		if !strings.Contains(got, want) {
			t.Errorf("engine calls missing %q:\n%s", want, got)
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(out), ".image-build-*"))
	if len(leftovers) != 0 {
		t.Errorf("work dir left behind: %v", leftovers)
	}
}

func TestEngineSelection(t *testing.T) {
	fakeEngine(t, "my-engine")
	t.Setenv("PADDOCK_CONTAINER_ENGINE", "my-engine")
	if e, err := (&Builder{}).engine(); err != nil || e != "my-engine" {
		t.Errorf("env override: got %q, %v", e, err)
	}
	if _, err := (&Builder{Engine: "no-such-engine-xyz"}).engine(); err == nil {
		t.Error("expected error for missing engine")
	}
}
