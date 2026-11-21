package assets

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func tgz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type server struct {
	*httptest.Server
	hits atomic.Int32
}

func serve(t *testing.T, files map[string][]byte) *server {
	t.Helper()
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		b, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestEmbeddedManifestCoversSupportedArches(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, a := range m.Assets {
		names[a.Name] = true
		for _, arch := range []string{"amd64", "arm64"} {
			art, ok := a.Arch[arch]
			if !ok {
				t.Errorf("%s: no %s build", a.Name, arch)
				continue
			}
			if !strings.HasPrefix(art.URL, "https://") || len(art.SHA256) != 64 {
				t.Errorf("%s/%s: bad url or sha256: %+v", a.Name, arch, art)
			}
		}
	}
	for _, want := range []string{"firecracker", "vmlinux"} {
		if !names[want] {
			t.Errorf("manifest missing %s", want)
		}
	}
}

func TestPullInstallsVerifiesAndSkips(t *testing.T) {
	kernel := []byte("kernel-bytes")
	archive := tgz(t, map[string]string{"rel/fc": "fc-binary", "rel/fc.debug": "debug"})
	srv := serve(t, map[string][]byte{"/vmlinux": kernel, "/fc.tgz": archive})

	m := &Manifest{Assets: []Asset{
		{Name: "firecracker", Version: "v1", Executable: true, Arch: map[string]Artifact{
			"amd64": {URL: srv.URL + "/fc.tgz", SHA256: sum(archive), Extract: "rel/fc"},
		}},
		{Name: "vmlinux", Version: "1", Arch: map[string]Artifact{
			"amd64": {URL: srv.URL + "/vmlinux", SHA256: sum(kernel)},
		}},
	}}
	dir := t.TempDir()
	p := &Puller{Dir: dir, Arch: "amd64"}
	if err := p.Pull(context.Background(), m); err != nil {
		t.Fatal(err)
	}

	fc, err := os.ReadFile(filepath.Join(dir, "firecracker"))
	if err != nil || string(fc) != "fc-binary" {
		t.Fatalf("firecracker: %q, %v", fc, err)
	}
	if st, _ := os.Stat(filepath.Join(dir, "firecracker")); st.Mode().Perm() != 0o755 {
		t.Errorf("firecracker mode %v, want 0755", st.Mode().Perm())
	}
	if k, _ := os.ReadFile(filepath.Join(dir, "vmlinux")); !bytes.Equal(k, kernel) {
		t.Errorf("vmlinux: %q", k)
	}

	hits := srv.hits.Load()
	if err := p.Pull(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if srv.hits.Load() != hits {
		t.Error("second Pull re-downloaded up-to-date assets")
	}

	leftovers, _ := filepath.Glob(filepath.Join(dir, ".*-*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestPullRejectsChecksumMismatch(t *testing.T) {
	srv := serve(t, map[string][]byte{"/vmlinux": []byte("tampered")})
	m := &Manifest{Assets: []Asset{{Name: "vmlinux", Arch: map[string]Artifact{
		"amd64": {URL: srv.URL + "/vmlinux", SHA256: sum([]byte("original"))},
	}}}}
	dir := t.TempDir()
	err := (&Puller{Dir: dir, Arch: "amd64"}).Pull(context.Background(), m)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("got %v, want checksum mismatch", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vmlinux")); !os.IsNotExist(err) {
		t.Error("tampered file was installed")
	}
}

func TestPullErrors(t *testing.T) {
	archive := tgz(t, map[string]string{"other": "x"})
	srv := serve(t, map[string][]byte{"/a.tgz": archive})
	tests := map[string]Artifact{
		"http 404":       {URL: srv.URL + "/missing", SHA256: sum(nil)},
		"missing member": {URL: srv.URL + "/a.tgz", SHA256: sum(archive), Extract: "nope"},
	}
	for name, art := range tests {
		t.Run(name, func(t *testing.T) {
			m := &Manifest{Assets: []Asset{{Name: "x", Arch: map[string]Artifact{"amd64": art}}}}
			if err := (&Puller{Dir: t.TempDir(), Arch: "amd64"}).Pull(context.Background(), m); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	m := &Manifest{Assets: []Asset{{Name: "x", Arch: map[string]Artifact{"amd64": {}}}}}
	if err := (&Puller{Dir: t.TempDir(), Arch: "riscv64"}).Pull(context.Background(), m); err == nil {
		t.Fatal("expected error for unsupported arch")
	}
}
