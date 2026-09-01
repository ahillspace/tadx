# TADX

TADX is a deterministic Tableau lifecycle and development CLI for coding agents and humans.
It targets higher accuracy, lower token use, and lower latency for Tableau lifecycle work.

Use the executable registry to discover the commands available in the current build:

```text
tadx capability list
tadx capability get <id>
```

Blocked and delegated capabilities never become executable TADX commands without their required evidence gates.

## Operator quickstart

Add one non-secret Tableau environment profile that references PAT variables already present in the process environment:

```text
tadx env add dev --url https://example.tableau.com --site example-site --pat-name-env TADX_DEV_PAT_NAME --pat-secret-env TADX_DEV_PAT_SECRET
tadx env default dev
tadx auth status --environment dev
tadx auth check --environment dev
```

Register a unique logical workspace name once, then use the name instead of repeating its machine-local root:

```text
tadx workspace create development --path "<machine-local-workspace-root>"
tadx env update dev --default-workspace development
tadx content workbook pull --environment dev --workspace development --id <workbook-luid>
```

Publish accepts the logical workspace and the relative artifact path returned by pull:

```text
tadx content workbook publish --workspace development --artifact "artifacts/workbook/<artifact-directory>" --environment dev --project "Department/Ops"
```

Preview is the default for consequential remote mutations.
Add `--apply` only after reviewing the exact plan.
Use `--full` when expanded bounded details are needed.

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

Contributors start with `AGENTS.md` and `docs/contributing/build-context.md`.
Slice agents receive only the bounded task card and focused sources named there.
Use these references when the current task requires them:

- `docs/build-order.md` for the required implementation sequence.
- `docs/contributing/adding-a-capability.md` for action and architecture rules.
- `docs/axi.md` for CLI behavior.
- `docs/toon.md` for output behavior.
- `docs/workspaces.md` for logical workspace names and portable artifact selectors.
- `docs/scope-v1.md` for V1 scope.
- `docs/reference/capabilities.md` for the generated capability inventory.

The arc42 and V1 capability contract in the repository root remain the authoritative product and capability sources.
