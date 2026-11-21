# paddock

Lightweight, self-hosted sandboxes for AI agents. Each sandbox is a Firecracker
microVM running Ubuntu 24.04, booting in under a second on your own hardware.

## Requirements

- Linux x86_64 with KVM (`/dev/kvm` readable/writable by your user)
- Go 1.26+, `curl`, `mke2fs` (e2fsprogs >= 1.47.1)

## Setup

```sh
./scripts/build-assets.sh   # fetch firecracker + kernel, build rootfs
go build -o paddock ./cmd/paddock
```

Assets and sandbox state live in `$PADDOCK_HOME` (default `~/.local/share/paddock`).

## Usage

```sh
id=$(./paddock create --cpus 2 --mem 1024)
./paddock exec "$id" -- uname -a
./paddock exec "$id" -- sh -c 'echo hi; exit 3'; echo $?   # exit code passes through
./paddock list
./paddock destroy "$id"
```

## How it works

- `paddock create` copies the base rootfs (reflink when supported), starts
  `firecracker` detached, and waits for the guest agent.
- `paddock-agent` runs as PID 1 inside the VM and executes commands sent over
  vsock, streaming stdout/stderr and the exit code back.

## Status

Early. No networking, stop/resume, snapshots, or MCP server yet.
