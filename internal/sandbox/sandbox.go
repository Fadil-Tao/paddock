// Package sandbox manages Firecracker microVM sandboxes on the local host.
//
// Layout under the data dir:
//
//	assets/firecracker      firecracker binary
//	assets/vmlinux          guest kernel
//	assets/rootfs.ext4      base root filesystem (contains paddock-agent)
//	sandboxes/<id>/         one dir per sandbox: meta, disk, sockets, log
package sandbox

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Fadil-Tao/paddock/internal/proto"
)

const agentPath = "/usr/sbin/paddock-agent"

type Spec struct {
	CPUs   int
	MemMiB int
}

type Sandbox struct {
	ID        string    `json:"id"`
	PID       int       `json:"pid"`
	CPUs      int       `json:"cpus"`
	MemMiB    int       `json:"mem_mib"`
	CreatedAt time.Time `json:"created_at"`
	Running   bool      `json:"-"`
}

type Manager struct {
	Root string
}

// DefaultRoot returns $PADDOCK_HOME, else $XDG_DATA_HOME/paddock, else
// ~/.local/share/paddock.
func DefaultRoot() string {
	if d := os.Getenv("PADDOCK_HOME"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "paddock")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "paddock")
}

func (m *Manager) asset(name string) string { return filepath.Join(m.Root, "assets", name) }
func (m *Manager) dir(id string) string     { return filepath.Join(m.Root, "sandboxes", id) }

func (m *Manager) Create(ctx context.Context, spec Spec) (_ *Sandbox, err error) {
	for _, a := range []string{"firecracker", "vmlinux", "rootfs.ext4"} {
		if _, err := os.Stat(m.asset(a)); err != nil {
			return nil, fmt.Errorf("missing asset %s (run scripts/build-assets.sh): %w", a, err)
		}
	}
	if spec.CPUs <= 0 {
		spec.CPUs = 2
	}
	if spec.MemMiB <= 0 {
		spec.MemMiB = 1024
	}

	id := newID()
	dir := m.dir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var proc *os.Process
	defer func() {
		if err != nil {
			if proc != nil {
				_ = proc.Kill()
			}
			_ = os.RemoveAll(dir)
		}
	}()

	disk := filepath.Join(dir, "rootfs.ext4")
	if out, err := exec.CommandContext(ctx, "cp", "--reflink=auto", "--sparse=always", m.asset("rootfs.ext4"), disk).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("copy rootfs: %v: %s", err, out)
	}

	cfg := map[string]any{
		"boot-source": map[string]any{
			"kernel_image_path": m.asset("vmlinux"),
			"boot_args":         "console=ttyS0 reboot=k panic=1 pci=off init=" + agentPath,
		},
		"drives": []map[string]any{{
			"drive_id":       "rootfs",
			"path_on_host":   disk,
			"is_root_device": true,
			"is_read_only":   false,
		}},
		"machine-config": map[string]any{
			"vcpu_count":   spec.CPUs,
			"mem_size_mib": spec.MemMiB,
		},
		"vsock": map[string]any{
			"guest_cid": 3,
			"uds_path":  filepath.Join(dir, "vsock.sock"),
		},
	}
	cfgPath := filepath.Join(dir, "config.json")
	if err := writeJSON(cfgPath, cfg); err != nil {
		return nil, err
	}

	logf, err := os.Create(filepath.Join(dir, "firecracker.log"))
	if err != nil {
		return nil, err
	}
	defer logf.Close()

	cmd := exec.Command(m.asset("firecracker"),
		"--api-sock", filepath.Join(dir, "api.sock"),
		"--config-file", cfgPath)
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.Dir = dir
	// New session so the VM outlives this CLI process.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start firecracker: %w", err)
	}
	proc = cmd.Process
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()

	sb := &Sandbox{ID: id, PID: proc.Pid, CPUs: spec.CPUs, MemMiB: spec.MemMiB, CreatedAt: time.Now().UTC()}
	if err := writeJSON(filepath.Join(dir, "meta.json"), sb); err != nil {
		return nil, err
	}

	if err := waitAgent(ctx, dir, exited, 15*time.Second); err != nil {
		return nil, fmt.Errorf("%w\n--- firecracker.log (tail) ---\n%s", err, tail(filepath.Join(dir, "firecracker.log"), 20))
	}
	sb.Running = true
	return sb, nil
}

func waitAgent(ctx context.Context, dir string, exited <-chan struct{}, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		c, err := dialAgent(dir)
		if err == nil {
			c.Close()
			return nil
		}
		select {
		case <-exited:
			return errors.New("firecracker exited during boot")
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("agent not ready after %s: %v", timeout, err)
		}
	}
}

type agentConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *agentConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// dialAgent connects to the guest agent through Firecracker's vsock UDS
// using its "CONNECT <port>" handshake.
func dialAgent(dir string) (*agentConn, error) {
	c, err := net.Dial("unix", filepath.Join(dir, "vsock.sock"))
	if err != nil {
		return nil, err
	}
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := fmt.Fprintf(c, "CONNECT %d\n", proto.Port); err != nil {
		c.Close()
		return nil, err
	}
	r := bufio.NewReader(c)
	line, err := r.ReadString('\n')
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("vsock handshake: %w", err)
	}
	if !strings.HasPrefix(line, "OK ") {
		c.Close()
		return nil, fmt.Errorf("vsock handshake: %q", strings.TrimSpace(line))
	}
	_ = c.SetDeadline(time.Time{})
	return &agentConn{Conn: c, r: r}, nil
}

// Exec runs a command in the sandbox, streaming output, and returns its exit
// code.
func (m *Manager) Exec(ctx context.Context, id string, req proto.Request, stdout, stderr io.Writer) (int, error) {
	sb, err := m.Get(id)
	if err != nil {
		return -1, err
	}
	if !sb.Running {
		return -1, fmt.Errorf("sandbox %s is not running", id)
	}
	c, err := dialAgent(m.dir(id))
	if err != nil {
		return -1, err
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()

	if err := json.NewEncoder(c).Encode(req); err != nil {
		return -1, err
	}
	for {
		typ, p, err := proto.ReadFrame(c)
		if err != nil {
			if ctx.Err() != nil {
				return -1, ctx.Err()
			}
			return -1, fmt.Errorf("agent connection: %w", err)
		}
		switch typ {
		case proto.Stdout:
			stdout.Write(p)
		case proto.Stderr:
			stderr.Write(p)
		case proto.Exit:
			return proto.ParseExit(p)
		case proto.Error:
			return -1, fmt.Errorf("agent: %s", p)
		}
	}
}

func (m *Manager) Get(id string) (*Sandbox, error) {
	if id == "" || strings.ContainsAny(id, `/\.`) {
		return nil, fmt.Errorf("invalid sandbox id %q", id)
	}
	b, err := os.ReadFile(filepath.Join(m.dir(id), "meta.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no such sandbox: %s", id)
	}
	if err != nil {
		return nil, err
	}
	var sb Sandbox
	if err := json.Unmarshal(b, &sb); err != nil {
		return nil, err
	}
	sb.Running = m.alive(&sb)
	return &sb, nil
}

func (m *Manager) List() ([]*Sandbox, error) {
	ents, err := os.ReadDir(filepath.Join(m.Root, "sandboxes"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Sandbox
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		sb, err := m.Get(e.Name())
		if err != nil {
			continue
		}
		out = append(out, sb)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *Manager) Destroy(id string) error {
	sb, err := m.Get(id)
	if err != nil {
		return err
	}
	if sb.Running {
		_ = syscall.Kill(sb.PID, syscall.SIGKILL)
		for i := 0; i < 100 && m.alive(sb); i++ {
			time.Sleep(20 * time.Millisecond)
		}
	}
	return os.RemoveAll(m.dir(id))
}

// alive reports whether sb's firecracker process is still running. It checks
// the command line so a recycled PID is never mistaken for the VM.
func (m *Manager) alive(sb *Sandbox) bool {
	if sb.PID <= 0 {
		return false
	}
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", sb.PID))
	if err != nil {
		return false
	}
	if !bytes.Contains(cmdline, []byte(filepath.Join(m.dir(sb.ID), "api.sock"))) {
		return false
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", sb.PID))
	if err != nil {
		return false
	}
	// State field follows the ")" closing the comm field; Z means zombie.
	if i := bytes.LastIndexByte(stat, ')'); i >= 0 && i+2 < len(stat) && stat[i+2] == 'Z' {
		return false
	}
	return true
}

func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func tail(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
