# Architecture

## Components

```
paddock (CLI)
  │ spawns, then talks over the vsock UDS
  ▼
firecracker (one process per sandbox, detached)
  │ virtio-vsock
  ▼
paddock-agent (PID 1 in the guest) ── runs commands, streams output
```

## Data dir

`$PADDOCK_HOME`, else `$XDG_DATA_HOME/paddock`, else `~/.local/share/paddock`.

```
assets/firecracker       firecracker binary
assets/vmlinux           guest kernel
assets/rootfs.ext4       base rootfs (Ubuntu base + paddock-agent)
sandboxes/<id>/
  meta.json              id, pid, cpus, mem, created_at
  config.json            firecracker config
  rootfs.ext4            per-sandbox copy of the base rootfs (reflink when possible)
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

Kernel cmdline `init=/usr/sbin/paddock-agent`. The agent mounts `/proc`, `/sys`, `/dev`, `/dev/pts`, `/dev/shm`, `/run`, `/tmp`, sets the hostname, starts the reaper, and listens on vsock port 1024. `create` returns once a vsock handshake succeeds.

## Planned (v0.2)

A root daemon (`paddockd`) takes over VM lifecycle and networking; the CLI becomes a client over `/run/paddock/paddock.sock`. VMs run under Firecracker's `jailer`, each with its own tap device and /30, internet via NAT, and LAN/host/metadata ranges blocked by default.
