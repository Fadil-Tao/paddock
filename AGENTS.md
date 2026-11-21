# AGENTS.md

paddock: self-hosted Firecracker microVM sandboxes for AI agents. One Go binary on the host (`cmd/paddock`), one static agent inside each VM (`cmd/paddock-agent`).

## Workflow

- `make lint test build` must pass before every commit. `make e2e` boots real VMs: run it when touching `internal/sandbox`, `internal/proto` or `cmd/paddock-agent` (needs `/dev/kvm` and `make assets`; it boots the freshly built `bin/paddock-agent`).
- Commits follow Conventional Commits (`feat:`, `fix:`, `chore:`, `docs:`, `test:`, `ci:`), one logical change each.
- On NixOS or with Nix: `nix develop` provides the full toolchain.

## Conventions

- Every call that can block takes a `context.Context` as its first argument.
- Wrap errors with `fmt.Errorf("doing X: %w", err)`; return them up to `main`, which is the only place that exits.
- Log with `log/slog` in host code. The guest agent logs to the serial console via `log`.
- Keep dependencies to the standard library and `golang.org/x/*`. Propose any other module in the PR description with a reason.
- Every `//nolint` names the linter and gives a reason.
- New behaviour ships with a unit test; anything that needs a real VM goes in `test/e2e` behind `//go:build e2e`.

## Invariants

- **Guest reaping:** `paddock-agent` runs as PID 1 and owns all `wait4` calls. Commands are started and registered in `waiters` under `mu`, and the reaper reaps under the same lock. Use that path for every child process; `cmd.Wait()` would race the reaper.
- **Trust boundary:** everything coming from inside a VM (agent output, guest files) is untrusted input. Host code treats it as data and bounds its size (`proto.MaxFrame`).
- **Pinned downloads:** everything paddock downloads is listed in `internal/assets/assets.json` with a sha256 and verified before install. Bump a version by updating URL and checksum together; base images pin their `FROM` by digest.
- **Plain images:** root filesystem images stay generic Linux with nothing paddock-specific inside. Guest-side behaviour belongs in `paddock-agent`, which boots from the initramfs.
- **Sandbox IDs** are the only identifiers taken from callers; host paths are always derived from the data dir, never accepted as input.

## Design docs

Read [`docs/architecture.md`](docs/architecture.md) before changing how the host, VMs and the agent talk to each other.
