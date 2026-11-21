// Command paddock-agent runs inside a sandbox VM. When started as PID 1 it
// does minimal init work (mounts, hostname, zombie reaping), then serves
// command execution requests from the host over vsock.
package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Fadil-Tao/paddock/internal/proto"
)

func main() {
	log.SetPrefix("paddock-agent: ")
	log.SetFlags(0)

	if os.Getpid() == 1 {
		initSystem()
	}
	go reap()

	fd, err := listenVsock(proto.Port)
	if err != nil {
		log.Fatalf("listen vsock: %v", err)
	}
	log.Printf("listening on vsock port %d", proto.Port)
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
		_ = os.MkdirAll(m.target, 0o755)
		if err := unix.Mount(m.src, m.target, m.fstype, 0, ""); err != nil && !errors.Is(err, unix.EBUSY) {
			log.Printf("mount %s: %v", m.target, err)
		}
	}
	if err := unix.Sethostname([]byte("paddock")); err != nil {
		log.Printf("sethostname: %v", err)
	}
	os.Setenv("PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	os.Setenv("HOME", "/root")
	os.Setenv("TERM", "xterm-256color")
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
		unix.Close(fd)
		return -1, err
	}
	if err := unix.Listen(fd, 128); err != nil {
		unix.Close(fd)
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
		outR.Close()
		outW.Close()
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
	outW.Close()
	errW.Close()
	if err != nil {
		outR.Close()
		errR.Close()
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
	outR.Close()
	errR.Close()

	code := ws.ExitStatus()
	if ws.Signaled() {
		code = 128 + int(ws.Signal())
	}
	send(proto.Exit, proto.ExitPayload(code))
}
