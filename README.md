# paddock

Lightweight, self-hosted sandboxes for AI agents. Each sandbox is a Firecracker
microVM running Ubuntu 24.04, booting in under a second on your own hardware.

## Requirements

- Linux x86_64 with KVM (`/dev/kvm` readable/writable by your user)
- Go 1.26+ (to build), plus for building the base image: Podman or Docker and
  `mke2fs` (e2fsprogs >= 1.47.1)

## Setup

```sh
make build                                   # ./bin/paddock, ./bin/paddock-agent
./bin/paddock pull                           # firecracker + kernel, sha256-verified
./bin/paddock image build images/ubuntu      # base rootfs from a Dockerfile (no root)
```

`make assets` runs the last two.

Assets and sandbox state live in `$PADDOCK_HOME` (default `~/.local/share/paddock`).

## Usage

```sh
id=$(./bin/paddock create --cpus 2 --mem 1024 --disk 20G)   # disk is sparse
./bin/paddock exec "$id" -- uname -a
./bin/paddock exec "$id" -- sh -c 'echo hi; exit 3'; echo $?   # exit code passes through
./bin/paddock list
./bin/paddock destroy "$id"
```

## How it works

- `paddock create` copies the base rootfs (reflink when supported), extends it
  to the requested disk size, starts `firecracker` detached, and waits for the
  guest agent.
- `paddock-agent` boots from an initramfs, switches root to the sandbox disk,
  runs as PID 1, and executes commands sent over vsock, streaming
  stdout/stderr and the exit code back. Root images stay plain Linux.

## Status

Early. No networking, stop/resume, snapshots, or MCP server yet. See [docs/architecture.md](docs/architecture.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[Apache License 2.0](LICENSE)
