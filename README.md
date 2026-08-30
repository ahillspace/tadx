# TADX

TADX is a deterministic Tableau lifecycle and development CLI for coding agents and humans.
It targets higher accuracy, lower token use, and lower latency for Tableau lifecycle work.

Phase 0 provides the frozen repository foundation and two executable discovery commands:

```text
tadx capability list
tadx capability get <id>
```

All other V1 capabilities remain registry metadata until their ordered build phase completes.
Blocked and delegated capabilities never become executable TADX commands without their required evidence gates.

## Build the CLI

Use Go 1.26 or later:

```shell
go build ./cmd/tadx
```

## Validate the foundation

Run the local freeze gates:

```shell
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
go mod tidy -diff
go generate ./...
go run ./cmd/gencapdocs -out docs/reference/capabilities.md
```

The CI workflow also cross-compiles `windows/amd64`, `darwin/amd64`, `darwin/arm64`, and `linux/amd64`.

## Read the contracts

Start with `AGENTS.md`, then read these files:

- `docs/build-order.md` for the required implementation sequence.
- `docs/contributing/adding-a-capability.md` for action and architecture rules.
- `docs/axi.md` for CLI behavior.
- `docs/toon.md` for output behavior.
- `docs/scope-v1.md` for V1 scope.
- `docs/reference/capabilities.md` for the generated capability inventory.

The arc42 and V1 capability contract in the repository root remain the authoritative product and capability sources.
