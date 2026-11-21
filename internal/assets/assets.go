// Package assets downloads the host-side runtime pieces paddock needs
// (firecracker, guest kernel) as pinned in the embedded manifest, verifying
// every download against its sha256 before installing it.
package assets

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed assets.json
var manifestJSON []byte

// Artifact is one downloadable file for one architecture.
type Artifact struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	// Extract, if set, names the member to install from a .tgz download.
	Extract string `json:"extract,omitempty"`
}

// Asset is a named file installed into the assets dir.
type Asset struct {
	Name       string              `json:"name"`
	Version    string              `json:"version"`
	Executable bool                `json:"executable,omitempty"`
	Arch       map[string]Artifact `json:"arch"`
}

type Manifest struct {
	Assets []Asset `json:"assets"`
}

// Default returns the manifest embedded in the binary.
func Default() (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		return nil, fmt.Errorf("parse embedded manifest: %w", err)
	}
	return &m, nil
}

// Puller installs manifest assets into Dir.
type Puller struct {
	Dir    string
	Arch   string // GOARCH name: amd64, arm64
	Client *http.Client
	Log    io.Writer
}

// Pull installs every asset for p.Arch, skipping ones already installed from
// the same verified download.
func (p *Puller) Pull(ctx context.Context, m *Manifest) error {
	if err := os.MkdirAll(p.Dir, 0o750); err != nil {
		return err
	}
	for _, a := range m.Assets {
		art, ok := a.Arch[p.Arch]
		if !ok {
			return fmt.Errorf("%s %s: no build for %s", a.Name, a.Version, p.Arch)
		}
		if p.installed(a, art) {
			p.logf("%s %s: up to date\n", a.Name, a.Version)
			continue
		}
		p.logf("%s %s: downloading %s\n", a.Name, a.Version, art.URL)
		if err := p.install(ctx, a, art); err != nil {
			return fmt.Errorf("%s %s: %w", a.Name, a.Version, err)
		}
	}
	return nil
}

func (p *Puller) logf(format string, args ...any) {
	if p.Log != nil {
		fmt.Fprintf(p.Log, format, args...)
	}
}

// stampPath records which verified download an installed file came from.
func (p *Puller) stampPath(a Asset) string { return filepath.Join(p.Dir, "."+a.Name+".sha256") }

func (p *Puller) installed(a Asset, art Artifact) bool {
	stamp, err := os.ReadFile(p.stampPath(a))
	if err != nil || strings.TrimSpace(string(stamp)) != art.SHA256 {
		return false
	}
	_, err = os.Stat(filepath.Join(p.Dir, a.Name))
	return err == nil
}

func (p *Puller) install(ctx context.Context, a Asset, art Artifact) error {
	dl, err := os.CreateTemp(p.Dir, "."+a.Name+"-*.download")
	if err != nil {
		return err
	}
	defer os.Remove(dl.Name())
	defer dl.Close()

	if err := p.download(ctx, art, dl); err != nil {
		return err
	}
	if _, err := dl.Seek(0, io.SeekStart); err != nil {
		return err
	}

	out, err := os.CreateTemp(p.Dir, "."+a.Name+"-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	if art.Extract != "" {
		err = extractTgz(dl, art.Extract, out)
	} else {
		_, err = io.Copy(out, dl)
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if a.Executable {
		mode = 0o755
	}
	if err := os.Chmod(out.Name(), mode); err != nil {
		return err
	}
	if err := os.Rename(out.Name(), filepath.Join(p.Dir, a.Name)); err != nil {
		return err
	}
	return os.WriteFile(p.stampPath(a), []byte(art.SHA256+"\n"), 0o600)
}

// download fetches art into w and fails unless the content matches its sha256.
func (p *Puller) download(ctx context.Context, art Artifact, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, art.URL, nil)
	if err != nil {
		return err
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", art.URL, resp.Status)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(w, h), resp.Body); err != nil {
		return fmt.Errorf("GET %s: %w", art.URL, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != art.SHA256 {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", art.URL, got, art.SHA256)
	}
	return nil
}

func extractTgz(r io.Reader, member string, w io.Writer) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%s not found in archive", member)
		}
		if err != nil {
			return err
		}
		if hdr.Name == member && hdr.Typeflag == tar.TypeReg {
			_, err := io.Copy(w, tr) //nolint:gosec // archive is sha256-verified before extraction
			return err
		}
	}
}
