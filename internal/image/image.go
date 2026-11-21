// Package image builds sandbox root filesystems from Dockerfiles: build with
// podman or docker, export the container filesystem as a tarball, then write
// it into an ext4 image with `mke2fs -d`, which preserves ownership without
// root.
package image

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Builder turns a Dockerfile build context into an ext4 rootfs image.
type Builder struct {
	// Engine is the container CLI ("podman" or "docker"); empty means
	// $PADDOCK_CONTAINER_ENGINE, else podman, else docker.
	Engine string
	Log    io.Writer
}

// Build builds contextDir and writes the resulting rootfs to out.
func (b *Builder) Build(ctx context.Context, contextDir, out string) error {
	engine, err := b.engine()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("mke2fs"); err != nil {
		return errors.New("mke2fs not found (install e2fsprogs >= 1.47.1)")
	}
	work, err := os.MkdirTemp(filepath.Dir(out), ".image-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	tag := "paddock-build-" + randHex()
	if err := b.run(ctx, engine, "build", "-t", tag, contextDir); err != nil {
		return fmt.Errorf("build: %w", err)
	}
	defer func() { _ = b.run(context.WithoutCancel(ctx), engine, "rmi", tag) }()

	cid, err := b.output(ctx, engine, "create", tag)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	defer func() { _ = b.run(context.WithoutCancel(ctx), engine, "rm", cid) }()

	tarPath := filepath.Join(work, "rootfs.tar")
	if err := b.run(ctx, engine, "export", "-o", tarPath, cid); err != nil {
		return fmt.Errorf("export: %w", err)
	}
	st, err := os.Stat(tarPath)
	if err != nil {
		return err
	}
	sizeMiB := SizeMiB(st.Size())
	b.logf("writing ext4 image (%d MiB)\n", sizeMiB)
	img := filepath.Join(work, "rootfs.ext4")
	if err := b.run(ctx, "mke2fs", "-q", "-t", "ext4", "-L", "rootfs", "-d", tarPath, img, fmt.Sprintf("%dM", sizeMiB)); err != nil {
		return fmt.Errorf("mke2fs: %w", err)
	}
	if err := os.Chmod(img, 0o644); err != nil { //nolint:gosec // image is world-readable like other assets
		return err
	}
	return os.Rename(img, out)
}

// SizeMiB returns the image size for a rootfs tarball: content plus 25% for
// filesystem overhead and 64 MiB headroom. Sandboxes grow their copy to the
// requested disk size at create time.
func SizeMiB(tarBytes int64) int64 {
	return (tarBytes>>20)*5/4 + 64
}

func (b *Builder) engine() (string, error) {
	candidates := []string{"podman", "docker"}
	if e := b.Engine; e != "" {
		candidates = []string{e}
	} else if e := os.Getenv("PADDOCK_CONTAINER_ENGINE"); e != "" {
		candidates = []string{e}
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("no container engine found (tried %s)", strings.Join(candidates, ", "))
}

func (b *Builder) run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stdout = b.Log
	cmd.Stderr = &stderr
	if b.Log != nil {
		cmd.Stderr = io.MultiWriter(b.Log, &stderr)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, args[0], err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (b *Builder) output(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("%s %s: %w: %s", name, args[0], err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (b *Builder) logf(format string, args ...any) {
	if b.Log != nil {
		fmt.Fprintf(b.Log, format, args...)
	}
}

func randHex() string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
