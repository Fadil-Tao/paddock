// Package initrd builds the initramfs that boots every sandbox: a newc cpio
// archive holding paddock-agent as /init plus the few nodes it needs before
// the root disk is mounted. Keeping the agent out of the rootfs means any
// plain Linux filesystem image can be a sandbox root.
package initrd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Mount point inside the initramfs where the agent mounts the root disk.
// Must match cmd/paddock-agent.
const NewRoot = "/newroot"

const (
	modeDir  = 0o040000
	modeFile = 0o100000
	modeChar = 0o020000
)

type entry struct {
	name      string
	mode      uint32
	rdevMajor uint32
	rdevMinor uint32
	data      []byte
}

// Write writes an initramfs containing agent as /init to w.
func Write(w io.Writer, agent []byte) error {
	entries := []entry{
		{name: "dev", mode: modeDir | 0o755},
		{name: "dev/console", mode: modeChar | 0o600, rdevMajor: 5, rdevMinor: 1},
		{name: "proc", mode: modeDir | 0o755},
		{name: "sys", mode: modeDir | 0o755},
		{name: NewRoot[1:], mode: modeDir | 0o755},
		{name: "init", mode: modeFile | 0o755, data: agent},
	}
	for i, e := range entries {
		if err := writeEntry(w, uint32(i+1), e); err != nil {
			return err
		}
	}
	return writeEntry(w, 0, entry{name: "TRAILER!!!"})
}

func writeEntry(w io.Writer, ino uint32, e entry) error {
	nlink := uint32(1)
	if e.mode&modeDir != 0 {
		nlink = 2
	}
	name := e.name + "\x00"
	var hdr bytes.Buffer
	fmt.Fprintf(&hdr, "070701%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x",
		ino, e.mode, 0, 0, nlink, 0, len(e.data), 0, 0, e.rdevMajor, e.rdevMinor, len(name), 0)
	hdr.WriteString(name)
	pad(&hdr, hdr.Len())
	if _, err := w.Write(hdr.Bytes()); err != nil {
		return err
	}
	if len(e.data) == 0 {
		return nil
	}
	if _, err := w.Write(e.data); err != nil {
		return err
	}
	_, err := w.Write(make([]byte, padLen(len(e.data))))
	return err
}

func padLen(n int) int { return (4 - n%4) % 4 }

func pad(b *bytes.Buffer, n int) { b.Write(make([]byte, padLen(n))) }

// Ensure returns the path of a cached initramfs for the agent binary at
// agentPath, building it under cacheDir if needed. The file name carries the
// agent's hash, so a rebuilt agent gets a fresh initramfs.
func Ensure(cacheDir, agentPath string) (string, error) {
	agent, err := os.ReadFile(agentPath)
	if err != nil {
		return "", fmt.Errorf("read agent: %w", err)
	}
	sum := sha256.Sum256(agent)
	path := filepath.Join(cacheDir, "initrd-"+hex.EncodeToString(sum[:8])+".cpio")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(cacheDir, "initrd-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if err := Write(tmp, agent); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write initrd: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}
