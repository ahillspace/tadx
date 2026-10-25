# Contribute to TADX

Use the repository-local [tadx-build skill](.agents/skills/tadx-build/SKILL.md) for implementation standards, architecture guidance, examples, and verification.
Select the skill's guidance that matches your change.

## Start with your agent

Ask your coding agent to use the build skill for a named task and state the outcome, scope, and stopping point.
For example:

```text
$tadx-build Build [the action you need]. The user outcome is [outcome].
Include [scope], exclude [scope], and stop after implementation and automated tests.
```

If your agent does not discover repository-local skills, give it the explicit entry point:

```text
Read .agents/skills/tadx-build/SKILL.md and use it to build [the action you need] in this repository.
```

## Run the current source

With the Go version specified in `go.mod` installed, build a local executable:

```sh
go build -trimpath -o ./bin/ ./cmd/tadx
```

Run the resulting executable to use unreleased changes from this checkout.
The build creates `bin/tadx` on macOS/Linux or `bin/tadx.exe` on Windows.
Release installers and `tadx update` use published releases, not this source tree.

## Validate locally

Run checks locally before pushing or merging.
The [CI workflow](.github/workflows/ci.yml) runs only when manually requested in GitHub Actions; pushes and pull requests do not start it.
Release tags retain their release validation and publication workflow.

Run these commands from the repository root with Go, Node.js, and Git installed.
Stop on a failed command.
Formatting passes only when `gofmt` prints no filenames.

```sh
gofmt -l .
go vet ./...
go test -count=1 ./...
go test -race -count=1 -timeout=15m ./...
go test ./internal/toon -run='^$' -fuzz=FuzzEncodeDecodeStrings -fuzztime=5s
go test ./internal/toon -run='^$' -fuzz=FuzzDecodeNeverPanics -fuzztime=5s
go test ./internal/toon -run='^$' -fuzz=FuzzJSONRoundTrip -fuzztime=5s
node --test scripts/tests/site_test.mjs
node --test .agents/skills/tadx-explore/scripts/explore.test.mjs
```

Check module metadata and generated files without including unrelated local edits in the comparison.
These commands must leave module metadata and generated files unchanged.

```sh
go mod tidy
go generate ./...
go run ./cmd/gencapdocs -out docs/reference/capabilities.md
git diff --exit-code -- go.mod go.sum
git diff
```

Cross-compile `./cmd/tadx` with `CGO_ENABLED=0` for `windows/amd64`, `darwin/amd64`, `darwin/arm64`, and `linux/amd64`.
Use `go build -trimpath -o <temporary-output> ./cmd/tadx` with `GOOS` and `GOARCH` set for each target.
Keep temporary binaries outside tracked source paths.

Run the shell installer fixtures in Bash, including Git Bash on Windows:

```sh
bash -n scripts/install.sh scripts/tests/install_sh_test.sh
bash scripts/tests/install_sh_test.sh
```

On Windows, run the installer fixtures with both PowerShell versions:

```powershell
pwsh -NoProfile -NonInteractive -File scripts\tests\install_ps1_test.ps1
powershell -NoProfile -NonInteractive -ExecutionPolicy Bypass -File scripts\tests\install_ps1_test.ps1
```

Record unavailable tools and skipped tests as verification limits.
Cross-compilation does not verify another operating system's runtime behavior.
Use manual CI for native platform coverage or privileged policy fixtures that require disposable administrator-owned environments.
The [native policy workflow](.github/workflows/native-policy.yml) retains those checks for manual CI and releases.
Do not elevate an ordinary development session to hide privilege-related skips.

For focused qualification against a real Tableau site, use the [selectable live CLI suite](tests/live/README.md).
It requires an explicit target, candidate executable, prepared selection, and existing write authorization.
Ordinary local checks do not run the live contracts.

## Keep changes maintainable

- Keep command plumbing, action behavior, Tableau adapters, and composition wiring within their established responsibilities.
- Preserve distinct operation contracts and mutation sequences, and share mechanisms only when their behavior matches.
- Keep user-facing behavior documented alongside the source of truth, and regenerate generated references with their documented tools.
- Keep credentials, private configuration, downloaded Tableau content, and local test output out of commits.

Use the build skill's [implementation map](.agents/skills/tadx-build/references/implementation-map.md) to find focused references and examples.
Start new work from updated `main`, unless continuing an assigned branch or authorized stack.
Coordinate shared files when multiple contributors work in the checkout.
Use the build skill's verification checklist before handoff.
The maintainer runs agent-based reviews; a build request does not authorize a push, pull request, merge, or release.

## Explore the installed CLI

Use the repository-local [tadx-explore skill](.agents/skills/tadx-explore/SKILL.md) for bounded exploration of a selected installed CLI category, resource, or verb.
