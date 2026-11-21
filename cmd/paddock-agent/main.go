// Command paddock-agent runs inside a sandbox VM. It boots as /init from the
// initramfs: mounts the root disk, switches root to it, grows the filesystem
// to the disk size, does minimal init work (mounts, hostname, zombie reaping),
// then serves command execution requests from the host over vsock.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/Fadil-Tao/paddock/internal/initrd"
	"github.com/Fadil-Tao/paddock/internal/proto"
)

// rootDevice is the sandbox's root disk (the first virtio-blk drive).
const rootDevice = "/dev/vda"

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	log.SetPrefix("paddock-agent: ")
	log.SetFlags(0)

	if os.Getpid() == 1 {
		// A failed switch leaves nothing to serve; exiting PID 1 panics the
		// kernel, the VM reboots (reboot=k) and firecracker exits, so the
		// host's create fails fast with this message in the log.
		if err := switchRoot(rootDevice, initrd.NewRoot); err != nil {
			log.Fatalf("switch root: %v", err)
		}
		initSystem()
		if err := growRootFS(rootDevice); err != nil {
			log.Printf("grow rootfs: %v", err)
		}
	}
	go reap()

	fd, err := listenVsock(proto.Port)
	if err != nil {
		log.Fatalf("listen vsock: %v", err)
	}
	log.Printf("%s listening on vsock port %d", version, proto.Port)
	for {
		nfd, _, err := unix.Accept4(fd, unix.SOCK_CLOEXEC)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			log.Printf("accept: %v", err)
			continue
		}
		go handle(os.NewFile(uintptr(nfd), "vsock"))
	}
}

func initSystem() {
	mounts := []struct{ src, target, fstype string }{
		{"proc", "/proc", "proc"},
		{"sysfs", "/sys", "sysfs"},
		{"devtmpfs", "/dev", "devtmpfs"},
		{"devpts", "/dev/pts", "devpts"},
		{"tmpfs", "/dev/shm", "tmpfs"},
		{"tmpfs", "/run", "tmpfs"},
		{"tmpfs", "/tmp", "tmpfs"},
	}
	for _, m := range mounts {
		_ = os.MkdirAll(m.target, 0o755) //nolint:gosec // standard mount point perms; guest is single-tenant root
		if err := unix.Mount(m.src, m.target, m.fstype, 0, ""); err != nil && !errors.Is(err, unix.EBUSY) {
			log.Printf("mount %s: %v", m.target, err)
		}
	}
	if err := unix.Sethostname([]byte("paddock")); err != nil {
		log.Printf("sethostname: %v", err)
	}
	for k, v := range map[string]string{
		"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME": "/root",
		"TERM": "xterm-256color",
	} {
		_ = os.Setenv(k, v) // only fails on invalid keys
	}
}

// Exit statuses are collected by a single reaper so that, as PID 1, we also
// reap orphaned grandchildren. Start+register and reaping both hold mu, so a
// child can never be reaped before its waiter is registered.
var (
	mu      sync.Mutex
	waiters = map[int]chan unix.WaitStatus{}
)

func reap() {
	sigs := make(chan os.Signal, 32)
	signal.Notify(sigs, unix.SIGCHLD)
	for range sigs {
		mu.Lock()
		for {
			var ws unix.WaitStatus
			pid, err := unix.Wait4(-1, &ws, unix.WNOHANG, nil)
			if err != nil || pid <= 0 {
				break
			}
			if ch, ok := waiters[pid]; ok {
				ch <- ws
				delete(waiters, pid)
			}
		}
		mu.Unlock()
	}
}

func listenVsock(port uint32) (int, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	if err := unix.Bind(fd, &unix.SockaddrVM{CID: unix.VMADDR_CID_ANY, Port: port}); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	if err := unix.Listen(fd, 128); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func handle(c io.ReadWriteCloser) {
	defer c.Close()

	var wmu sync.Mutex
	send := func(typ byte, p []byte) {
		wmu.Lock()
		defer wmu.Unlock()
		_ = proto.WriteFrame(c, typ, p)
	}

	var req proto.Request
	if err := json.NewDecoder(c).Decode(&req); err != nil {
		send(proto.Error, []byte("bad request: "+err.Error()))
		return
	}
	if len(req.Argv) == 0 {
		send(proto.Error, []byte("empty argv"))
		return
	}

	outR, outW, err := os.Pipe()
	if err != nil {
		send(proto.Error, []byte(err.Error()))
		return
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		_ = outR.Close()
		_ = outW.Close()
		send(proto.Error, []byte(err.Error()))
		return
	}

	cmd := exec.Command(req.Argv[0], req.Argv[1:]...)
	cmd.Stdout = outW
	cmd.Stderr = errW
	cmd.Env = append(os.Environ(), req.Env...)
	cmd.Dir = req.Dir
	if cmd.Dir == "" {
		cmd.Dir = "/root"
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	exited := make(chan unix.WaitStatus, 1)
	mu.Lock()
	err = cmd.Start()
	if err == nil {
		waiters[cmd.Process.Pid] = exited
	}
	mu.Unlock()
	_ = outW.Close()
	_ = errW.Close()
	if err != nil {
		_ = outR.Close()
		_ = errR.Close()
		send(proto.Error, []byte(err.Error()))
		return
	}

	var wg sync.WaitGroup
	pump := func(r *os.File, typ byte) {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				send(typ, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}
	wg.Add(2)
	go pump(outR, proto.Stdout)
	go pump(errR, proto.Stderr)

	ws := <-exited
	_ = cmd.Process.Release()

	// Drain remaining output. Background processes may keep the pipes open
	// forever, so don't wait on them indefinitely.
	drained := make(chan struct{})
	go func() { wg.Wait(); close(drained) }()
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
	}
	_ = outR.Close()
	_ = errR.Close()

	code := ws.ExitStatus()
	if ws.Signaled() {
		code = 128 + int(ws.Signal())
	}
	send(proto.Exit, proto.ExitPayload(code))
}

// switchRoot mounts dev at target and makes it the root filesystem, the same
// way switch_root(8) does: move the mount over "/", chroot into it.
func switchRoot(dev, target string) error {
	if err := unix.Mount("devtmpfs", "/dev", "devtmpfs", 0, ""); err != nil {
		return fmt.Errorf("mount /dev: %w", err)
	}
	var err error
	for range 200 {
		if _, err = os.Stat(dev); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("wait for %s: %w", dev, err)
	}
	if err := unix.Mount(dev, target, "ext4", 0, ""); err != nil {
		return fmt.Errorf("mount %s: %w", dev, err)
	}
	// initSystem mounts a fresh /dev inside the new root. Detach this one;
	// our console fds stay valid.
	_ = unix.Unmount("/dev", unix.MNT_DETACH)
	// Free the initramfs copy of this binary; the running image stays mapped.
	_ = os.Remove("/init")

	if err := unix.Chdir(target); err != nil {
		return err
	}
	if err := unix.Mount(".", "/", "", unix.MS_MOVE, ""); err != nil {
		return fmt.Errorf("move root: %w", err)
	}
	if err := unix.Chroot("."); err != nil {
		return fmt.Errorf("chroot: %w", err)
	}
	return unix.Chdir("/")
}

// ext4IocResizeFS is EXT4_IOC_RESIZE_FS: _IOW('f', 16, __u64).
const ext4IocResizeFS = 0x40086610

// growRootFS grows the mounted ext4 root online to fill dev. The host only
// extends the (sparse) disk file; the guest kernel does the resize, so the
// host needs no e2fsprogs. Growing to the current size is a no-op.
func growRootFS(dev string) error {
	f, err := os.Open(dev)
	if err != nil {
		return err
	}
	defer f.Close()
	var devBytes uint64
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&devBytes))) //nolint:gosec // ioctl out-param; pointer valid for the call
	if errno != 0 {
		return fmt.Errorf("BLKGETSIZE64: %w", errno)
	}
	var st unix.Statfs_t
	if err := unix.Statfs("/", &st); err != nil {
		return err
	}
	blocks := devBytes / uint64(st.Bsize) //nolint:gosec // block size is positive
	root, err := os.Open("/")
	if err != nil {
		return err
	}
	defer root.Close()
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, root.Fd(), ext4IocResizeFS, uintptr(unsafe.Pointer(&blocks))) //nolint:gosec // ioctl in-param; pointer valid for the call
	if errno != 0 {
		return fmt.Errorf("EXT4_IOC_RESIZE_FS to %d blocks: %w", blocks, errno)
	}
	return nil
}
