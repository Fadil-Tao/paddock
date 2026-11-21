# Contributing

## Setup

Requirements: Linux with KVM, Go (see `go.mod`), `golangci-lint` v2, `mke2fs` >= 1.47.1, `curl`. With Nix, `nix develop` provides all of them.

```sh
make assets   # one time: firecracker, kernel, rootfs into ~/.local/share/paddock/assets
make          # lint + unit tests + build into ./bin
make e2e      # boots real VMs
```

## Pull requests

- One logical change per PR, with tests.
- `make lint test` passes; run `make e2e` if you touched the sandbox, protocol or guest agent.
- Commit messages use [Conventional Commits](https://www.conventionalcommits.org/).
- Code conventions and invariants live in [AGENTS.md](AGENTS.md); they apply to humans too.

## License

By contributing you agree your contributions are licensed under the [Apache License 2.0](LICENSE).
