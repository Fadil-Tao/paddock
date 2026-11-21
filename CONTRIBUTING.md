# Contributing

## Setup

Requirements: Linux with KVM, Go (see `go.mod`), `golangci-lint` v2, `mke2fs` >= 1.47.1, and Podman or Docker. With Nix, `nix develop` provides all but the container engine.

```sh
make assets   # one time: paddock pull + paddock image build images/ubuntu
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
