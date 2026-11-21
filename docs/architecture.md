# Architecture

## Components

```
paddock (CLI)
  │ builds initramfs from bin/paddock-agent, spawns firecracker,
  │ then talks over the vsock UDS
  ▼
firecracker (one process per sandbox, detached)
  │ virtio-vsock
  ▼
paddock-agent (PID 1 in the guest) ── runs commands, streams output
```

The CLI finds the agent at `$PADDOCK_AGENT`, else next to its own binary.

## Data dir

`$PADDOCK_HOME`, else `$XDG_DATA_HOME/paddock`, else `~/.local/share/paddock`.

```
assets/firecracker       firecracker binary
assets/vmlinux           guest kernel
assets/rootfs.ext4       base rootfs: plain Ubuntu base, nothing paddock-specific
cache/initrd-<hash>.cpio initramfs (agent as /init), keyed by agent hash
sandboxes/<id>/
  meta.json              id, pid, cpus, mem, disk, created_at
  config.json            firecracker config
  rootfs.ext4            per-sandbox copy of the base rootfs (reflink when
                         possible), extended (sparse) to the --disk size
  vsock.sock api.sock    firecracker sockets
  firecracker.log        VMM + guest serial console
```

A sandbox is "running" when `meta.pid` is alive, not a zombie, and its cmdline references that sandbox's `api.sock` (guards against PID reuse).

## Host ↔ guest protocol (`internal/proto`)

1. Host connects to `sandboxes/<id>/vsock.sock` and sends `CONNECT 1024\n`; Firecracker answers `OK <port>\n`.
2. Host sends one JSON `Request` (`argv`, `env`, `dir`).
3. Agent replies with frames: `type(1) | len(4, BE) | payload`. Types: `Stdout`, `Stderr`, then exactly one terminal `Exit` (4-byte exit code) or `Error` (command never started).

One connection runs one command. Exit codes of signalled processes are `128 + signal`.

## Guest boot

1. Firecracker boots the kernel with the initramfs (`initrd_path`); the kernel runs the agent as `/init`.
2. The agent mounts `/dev/vda` (ext4) on `/newroot`, deletes `/init` to free memory, moves the mount over `/` and chroots: the same steps as `switch_root(8)`, but the agent stays PID 1.
3. It mounts `/proc`, `/sys`, `/dev`, `/dev/pts`, `/dev/shm`, `/run`, `/tmp`, sets the hostname, starts the reaper.
4. It grows the root filesystem online to the disk size (`EXT4_IOC_RESIZE_FS`). The host only extends the sparse disk file, so it needs no e2fsprogs at runtime.
5. It listens on vsock port 1024. `create` returns once a vsock handshake succeeds.

A failure before step 5 exits PID 1: the kernel panics, the VM reboots (`reboot=k`), firecracker exits, and `create` fails with the log tail.

Because the agent lives in the initramfs, any plain ext4 image can be a sandbox root, and changing the agent never requires rebuilding images.

## Planned (v0.2)

A root daemon (`paddockd`) takes over VM lifecycle and networking; the CLI becomes a client over `/run/paddock/paddock.sock`. VMs run under Firecracker's `jailer`, each with its own tap device and /30, internet via NAT, and LAN/host/metadata ranges blocked by default.
