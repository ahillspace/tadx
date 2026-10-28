# Dependencies and delivery

TADX builds a Go CLI from [`cmd/tadx`](../../cmd/tadx/main.go) and a separate VS Code extension from [`extensions/vscode`](../../extensions/vscode).
The CLI embeds its operating Guidance and bootstrap installers, so the released executable does not need a source checkout.
The [runtime guide](runtime.md) explains command execution; this page owns dependency roles and the source-to-installation path.
The [review record](review.md) identifies the examined working tree and verification limits.

## Toolchains and libraries

[`go.mod`](../../go.mod) defines the Go version and selected module versions; [`go.sum`](../../go.sum) records module checksums.
The current module requires Go 1.26.
Use those files as the version authority rather than treating this explanation as a second lockfile.
There is no root npm app or Node.js dependency in the CLI runtime.
Node.js runs repository maintenance scripts, extension development tools, and the VS Code extension host.

| Direct module | Role | Implementation evidence |
| --- | --- | --- |
| `github.com/spf13/cobra` | CLI registration, argument dispatch, help, and shell completion | [`internal/cli/root.go`](../../internal/cli/root.go), [`completion.go`](../../internal/cli/completion.go) |
| `github.com/spf13/pflag` | Flag definitions, shorthand, and bounded batch argument handling | [`internal/cli/arguments.go`](../../internal/cli/arguments.go), [`batch.go`](../../internal/cli/batch.go) |
| `gopkg.in/yaml.v3` | Nonsecret configuration and workspace manifest parsing, including preserved invalid entries | [`internal/config/codec.go`](../../internal/config/codec.go), [`internal/workspace/manager.go`](../../internal/workspace/manager.go) |
| `modernc.org/sqlite` | Embedded, in-process cache database | [`internal/cache/store.go`](../../internal/cache/store.go) |
| `github.com/zalando/go-keyring` | Native credential storage for explicitly approved PAT persistence | [`internal/auth/credentials.go`](../../internal/auth/credentials.go) |
| `golang.org/x/sys` | Native locks, filesystem replacement, process handling, and managed-policy protection | [`internal/lock`](../../internal/lock), [`internal/fsreplace`](../../internal/fsreplace), [`internal/managedpolicy`](../../internal/managedpolicy), [`internal/operationrun/launch_windows.go`](../../internal/operationrun/launch_windows.go) |
| `golang.org/x/term` | Terminal input without echo and terminal-aware policy installation | [`internal/app/auth_prompt.go`](../../internal/app/auth_prompt.go), [`internal/managedpolicy/elevation_unix.go`](../../internal/managedpolicy/elevation_unix.go) |
| `github.com/mattn/go-isatty` | Terminal detection for progress | [`internal/cli/progress/progress.go`](../../internal/cli/progress/progress.go) |
| `github.com/vektah/gqlparser/v2` | Test-time validation of Metadata API queries against retained schemas | [`internal/tableau/metadata/lineage_client_test.go`](../../internal/tableau/metadata/lineage_client_test.go), [`internal/tableau/metadataassets/client_test.go`](../../internal/tableau/metadataassets/client_test.go) |

The GraphQL parser is a direct module dependency because tests import it.
The Windows CLI dependency graph inspected during this audit does not include it; production Metadata API clients construct requests through TADX's own clients.
Do not infer runtime participation from the direct `require` block alone.

Important transitive groups include SQLite's `modernc.org` runtime and supporting utilities, the keyring's Windows credential and Linux D-Bus clients, and Cobra's platform and flag support.
The GraphQL parser brings a test-related edit-distance dependency.
`go mod graph` shows module relationships, including modules outside a particular executable's compiled graph.
Use package dependencies to distinguish those relationships from code actually linked for a selected platform:

```sh
go list -deps ./cmd/tadx
go mod graph
```

The first command reflects the current `GOOS` and `GOARCH`.
Platform-specific files select other native implementations on other systems.
[`THIRD_PARTY_NOTICES.md`](../../THIRD_PARTY_NOTICES.md) retains existing distribution notices.
This architecture review does not constitute a complete license, vulnerability, or supply-chain audit.

## External systems and optional tools

| Dependency | When required | Boundary |
| --- | --- | --- |
| Tableau Cloud or Server | Live content, administration, Catalog, and Pulse operations | Resource clients under [`internal/tableau`](../../internal/tableau); server version, permissions, licenses, and feature availability still apply |
| Tableau REST API | PAT sessions, content lifecycle, administration, and supported metadata changes | [`internal/tableau/transport.go`](../../internal/tableau/transport.go) and resource-specific clients; REST version negotiation precedes sign-in |
| Tableau Metadata API | Upstream graph, schema enrichment, lineage, and Catalog reads | [`metadata`](../../internal/tableau/metadata), [`metadataassets`](../../internal/tableau/metadataassets), and [`fieldcatalog`](../../internal/tableau/fieldcatalog); Metadata IDs remain distinct from REST LUIDs |
| Tableau Pulse API | Pulse definition, metric configuration, and follower lifecycle | [`internal/tableau/pulse`](../../internal/tableau/pulse); analytical values and generated insights remain outside TADX ownership |
| Native credential backend | Approved persistent PAT storage | Windows Credential Manager, macOS Keychain, or Linux Secret Service through `go-keyring`; unavailable storage must not fall back to plaintext |
| GitHub releases | Installation, explicit update checks, updates, and eligible passive notices | [`internal/version`](../../internal/version), [`internal/update`](../../internal/update), [`internal/releasenotice`](../../internal/releasenotice), and bootstrap scripts |
| GitHub CLI | Optional authenticated release lookup and installer access | [`internal/update/runtime.go`](../../internal/update/runtime.go) tries `gh` for release metadata before its public lookup; this does not provide Tableau authentication |
| Native shell and archive utilities | Platform bootstrap installation and self-update | [`scripts/install.ps1`](../../scripts/install.ps1), [`scripts/install.sh`](../../scripts/install.sh); ordinary CLI operations do not invoke the installers |
| Chromium | Optional static-site browser checks | Supplied executable for scripts documented in [website maintenance](../website.md); not a required CLI runtime dependency |

TADX does not call or proxy Tableau MCP.
See the [capability boundary](../../README.md#tadx-and-tableau-mcp) and [upstream evidence index](../evidence/README.md) for external contract references.
The [resource guide](resources-and-state.md) explains provider selection and state ownership.
The [security guide](../security.md) owns credential, transport, local-data, and platform limitations.

## Build and generated content

Build the current checkout with the command in [CONTRIBUTING.md](../../CONTRIBUTING.md#run-the-current-source):

```sh
go build -trimpath -o ./bin/ ./cmd/tadx
```

The executable entry point starts the command runtime and handles process-level notices and exit behavior.
The composition root is [`internal/app`](../../internal/app), not a resident server.
[`scripts/embed.go`](../../scripts/embed.go) embeds both platform installers.
[`internal/agent/install.go`](../../internal/agent/install.go) embeds the `tadx` and `tadx-pulse` packages from [`internal/agent/skills`](../../internal/agent/skills).
Changes to those embedded inputs require rebuilding the executable.

[`internal/capability/definitions.go`](../../internal/capability/definitions.go) and [`metadata_definitions.go`](../../internal/capability/metadata_definitions.go) own capability facts.
[`registry.go`](../../internal/capability/registry.go) validates those facts and declares generation through [`cmd/gencapdocs`](../../cmd/gencapdocs/main.go).
The generator writes the Markdown and JSON references and updates the capability-map data block.
Run its maintained generation path rather than editing generated references by hand.
The architecture explanations and diagram assets are maintained separately; capability generation does not update architectural prose.

| Deliverable | Source owner | Distribution boundary |
| --- | --- | --- |
| CLI executable | [`cmd/tadx`](../../cmd/tadx/main.go), [`internal/app`](../../internal/app), and Go module packages | Platform archives contain the executable, license, and both installers |
| Operating Guidance | [`internal/agent/skills`](../../internal/agent/skills) | Embedded in the CLI, then installed into supported agent homes |
| Repository engineering and exploration skills | [`.agents/skills`](../../.agents/skills) | Checkout tooling; not embedded or installed by `tadx agent install` |
| Capability references | [`internal/capability`](../../internal/capability), [`cmd/gencapdocs`](../../cmd/gencapdocs/main.go) | Generated Markdown, JSON, and the data block inside the authored capability map |
| Public website | [`site`](../../site), [`buildSite`](../../scripts/build-site.mjs) | Explicit six-file public input list, plus generated `.nojekyll` |
| VS Code extension | [`extensions/vscode`](../../extensions/vscode) | Independent VSIX containing bundled JavaScript, media, maintained installers, manifest, README, and license |

The root Go module and extension have separate tooling boundaries.
[`extensions/vscode/go.mod`](../../extensions/vscode/go.mod) prevents root Go package discovery from entering installed Node dependencies.
The extension's synthetic CLI host fixture has its own [Go module](../../extensions/vscode/tests/fixtures/host/go.mod).
Neither nested module adds a production Go component to the extension.
The architecture check's [`inspectSources`](../../internal/architecture/architecture.go) respects nested module boundaries when checking production Go imports.
Root Go checks do not replace the extension's TypeScript, browser, or extension-host checks.

## Installation and Guidance ownership

The bootstrap scripts select a published platform archive, obtain its checksum manifest, verify SHA-256, and install the binary.
They support PATH and completion controls and invoke the installed binary to deploy bundled Guidance.
Their exact options and recovery procedures belong in [installation maintenance](../getting-started.md#maintain-the-installation).
Checksum verification detects mismatched release content; it is not a claim that the bootstrap script independently authenticates an untrusted distribution origin.

[`internal/agenttarget/targets.go`](../../internal/agenttarget/targets.go) is the shared registry of supported agent skill roots.
[`Installer.installAuto`](../../internal/agent/auto.go) refreshes detected agent homes and uses the generic shared target when none exists.
Detection does not create client configuration or prove that a host loaded the Guidance.
The installer stages owned packages, checks their fingerprints and ownership receipts, and retains recoverable backups when needed.
[`receipt.go`](../../internal/agent/receipt.go) and [`legacy.go`](../../internal/agent/legacy.go) provide persisted ownership and supported legacy migration.
Keep personal skills separate from TADX-owned package names.

The installation sequence has distinct transaction boundaries:

1. The bootstrap script verifies the archive before staging the executable beside its destination.
2. An installation-directory lock prevents concurrent binary replacement; the previous binary remains available for recovery.
3. The new binary installs Guidance through [`Installer.Install`](../../internal/agent/install.go).
4. Each agent target locks its paths, stages changed packages, rechecks fingerprints, commits package moves, and writes the ownership receipt.
5. Failed package commits attempt rollback; incomplete rollback returns partial package state and recovery locations.

Automatic installation processes targets separately and retains earlier target outcomes when a later target fails.
The bootstrap scripts attempt binary rollback after Guidance failure; completed Guidance targets are not rolled back across targets.
PATH and completion changes follow binary and Guidance installation, so the complete setup is not one filesystem transaction.
Uninstall preserves edited Guidance as backups and reports cleanup warnings instead of claiming that every file disappeared.
These recovery contracts are exercised by [package recovery fixtures](../../internal/agent/install_recovery_test.go) and [uninstall fixtures](../../internal/agent/uninstall_recovery_test.go).

[`internal/update/runtime.go`](../../internal/update/runtime.go) reuses the embedded installer for self-update.
It resolves the installed executable, validates release identity, and withholds configured PAT environment variables from updater subprocesses.
`runProcess` bounds updater output and stops its process scope on cancellation.
Canceled installation reports potentially partial state and directs inspection of the installed version and retained backups; it does not promise rollback.
The [runtime guide](runtime.md) explains optional notices; update and Guidance notices do not participate in Tableau operations.

## Agent packages and maintenance tools

The bundled [`tadx`](../../internal/agent/skills/tadx/SKILL.md) skill owns shared operating guidance and routes readers to focused concept references.
The bundled [`tadx-pulse`](../../internal/agent/skills/tadx-pulse/SKILL.md) skill adds Pulse authoring guidance and refers back to the neighboring root skill.
[`skills_test.go`](../../internal/agent/skills_test.go) checks metadata, local references, installed package completeness, and help entry points.
[`agent_guidance_test.go`](../../internal/architecture/agent_guidance_test.go) enforces dependency boundaries around installation, update, and notices.

The repository's [`tadx-build`](../../.agents/skills/tadx-build/SKILL.md) skill owns engineering instructions and focused implementation references.
The [`tadx-explore`](../../.agents/skills/tadx-explore/SKILL.md) skill owns bounded installed-CLI exploration.
Its [`createLauncher`](../../.agents/skills/tadx-explore/scripts/explore.mjs) writes ignored run manifests, worker payloads, reports, journals, and summaries under `.tadx-explore/`.
The launcher emits worker requests; it does not launch agents or operate Tableau itself.
It validates report identities and resource journals, and stops continuation after uncertain outcomes, unresolved cleanup, or exhausted budgets.
Reported calls and coverage are worker accounts, not instrumentation of CLI activity.
The offline launcher fixtures run in manual CI; hosted release quality does not include that Node test suite.

[`token-audit.py`](../../scripts/token-audit.py) is an optional Python development tool with a pinned tokenizer.
It compares six local help journeys using isolated configuration and records binary hashes and available build provenance.
Its measurements do not establish model billing or live Tableau behavior.
[`check-capability-map.mjs`](../../scripts/check-capability-map.mjs) and the [website browser scripts](../../scripts/tests) provide optional rendered-page checks.
Their browser executables and the token-audit Python dependencies are development prerequisites, not CLI dependencies.

## Validation and publication

[CONTRIBUTING.md](../../CONTRIBUTING.md#validate-locally) owns the local validation procedure.
Tests span action contracts, provider HTTP fixtures, composition-level end-to-end paths, import boundaries, generated references, installers, and static-site packaging.
Passing local fixtures does not establish live Tableau compatibility or another operating system's native behavior.
Native policy checks need their documented platform and privilege environments.

[`ci.yml`](../../.github/workflows/ci.yml) runs only through manual dispatch.
[`release.yml`](../../.github/workflows/release.yml) also runs on version tags and retains its own quality and native-policy gates.
Release builds cover Windows, macOS, and Linux on amd64 and arm64 with `CGO_ENABLED=0`.
The release workflow injects `internal/version.BuildVersion`, builds deterministic archives, publishes checksums, and adds provenance attestations when the repository is public.
Its explicit existing-release refresh path preserves previous assets and publishes the new checksum manifest last.
These are inspected workflow definitions, not evidence that a particular release or attestation exists.
The CLI release workflow publishes ten assets: six platform archives, both installers, the license, and the checksum manifest.
It does not package or publish the VS Code extension.
The separately dispatched [extension workflow](../../.github/workflows/vscode-extension.yml) owns extension validation, versioned VSIX assets, and explicitly selected publication.
Its `extension-vX.Y.Z` tags and independent versions keep extension releases separate from CLI release discovery.

The public website uses plain static HTML.
[`scripts/build-site.mjs`](../../scripts/build-site.mjs) copies an explicit allowlist of pages, capability data, and installers into a new or empty output directory.
It does not publish the repository documentation tree.
[`pages.yml`](../../.github/workflows/pages.yml) builds on manual dispatch and deploys only when its publication conditions are met.
See [website maintenance](../website.md) for deployment prerequisites and browser checks.

## Editor integration

The [VS Code extension](../../extensions/vscode/README.md) is a separate TypeScript package with its own VSIX artifact and Node.js development dependencies.
The [editor architecture](editor-extension.md) owns runtime composition, CLI contracts, identities, state, and operation safety.

[`package.json`](../../extensions/vscode/package.json) declares VS Code `^1.95.0` and Node.js `>=22.15.0` constraints and no runtime npm dependencies.
Its development dependencies include TypeScript, esbuild, ESLint, TypeScript test execution, VS Code host testing, packaging, and browser fixtures.
[`package-lock.json`](../../extensions/vscode/package-lock.json) pins the development dependency graph for `npm ci`.
The Node.js constraint applies to package tooling; the VS Code installation supplies its own extension-host runtime.
The manifest selects a workspace extension host and excludes untrusted and virtual workspaces.
The CLI must be installed on that host, including SSH, WSL, container, and Codespaces hosts.

[`build.mjs`](../../extensions/vscode/scripts/build.mjs) bundles `src/extension.ts` as CommonJS targeting Node.js 20 and leaves the `vscode` module external.
It also copies the maintained shell and PowerShell installers into the VSIX inputs for guided CLI setup.
[`vscode:prepublish`](../../extensions/vscode/package.json) rebuilds the bundle before VSIX packaging.
The [VSIX allowlist](../../extensions/vscode/.vscodeignore) excludes source, test fixtures, dependency trees, and source maps.
VSIX packaging does not itself publish to Marketplace or include the TADX executable.
[`cli-release.json`](../../extensions/vscode/cli-release.json) pins the guided setup release and per-platform archive hashes.
The installers verify those independent hashes as well as the release checksum manifest before extraction or replacement.
Setup requires explicit confirmation and preserves CLI ownership of credentials, configuration, and shared installation state.

The [extension README](../../extensions/vscode/README.md#develop-and-validate) owns the exact contributor commands and prerequisite versions.
The validation boundaries are distinct:

| Check | What it covers | What it does not establish |
| --- | --- | --- |
| `npm run check` | Strict type checking, lint, unit tests, and bundling | Browser rendering, real VS Code activation, or packaging |
| `npm run test:cli` | Source-built Go CLI read contracts consumed by the actual extension adapter | Live Tableau operations or all CLI commands |
| `npm run test:released-cli` | Checksum-pinned released CLI read contracts on the current platform | Other platforms or live transfer behavior |
| `npm run test:ui` | Actual webview HTML, scripts, styles, and CSP in a browser fixture | VS Code host behavior or actual CLI output |
| `npm run test:host` | VS Code activation and commands in an isolated profile using a synthetic CLI | The current CLI implementation, remote hosts, or live Tableau |
| Minimum-version host invocation | The explicitly selected VS Code version | Every supported VS Code release |
| `npm run package` | VSIX construction with the package allowlist | Tests, release approval, or Marketplace publication |
| Release validation and packaging | Manifest/tag/channel agreement, allowed VSIX contents, source revision, and artifact checksums | Permission to publish or an existing Marketplace publisher configuration |

[`test-host.mjs`](../../extensions/vscode/scripts/test-host.mjs) defaults to stable VS Code and accepts an explicit version.
It compiles a separate synthetic Go executable and retains failed-run artifacts for inspection.
The [build validation record](../vscode-extension-validation.md) distinguishes historical Windows checks from later release validation and its limits.
The manual extension workflow gates publication on package, browser, host, source CLI, released CLI, and installer checks.
It publishes the verified package without rebuilding it; GitHub release and Marketplace publication require an explicit selected action.
The [release and upgrade guide](../vscode-extension-release.md) owns versioning, release channels, publishing identity, setup, and artifact verification.
The [review record](review.md) preserves the original delivery finding and its 2026-10-25 resolution.

## Keep this map current

Update this page when dependency roles, embedded inputs, supported distribution targets, or delivery ownership change.
Keep exact dependency versions in the manifests and exact public command syntax in executable help.
Update the [architecture index](README.md) and [review coverage](review.md) when adding a maintained ecosystem or a new process boundary.
