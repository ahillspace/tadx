# Validation baseline and Linux setup

Snapshot date: 2026-10-24.
The results below summarize earlier Windows validation and are not new Linux test results.
No product tests run solely to create this handoff.
See the [maintained validation record](../../vscode-extension-validation.md) for regression evidence and review scope.

## Last completed extension checks

| Check | Recorded result |
| --- | --- |
| Strict TypeScript, ESLint, bundle, and unit/contract tests | `npm run check` passes; 67 tests pass. |
| Actual webview browser scenarios | Seven scenario groups pass with CSP enabled. |
| VS Code 1.143.0 native host | Eight checks pass on Windows. |
| VS Code 1.95.3 native host | Eight checks pass on Windows. |
| Synthetic host fixture | Go vet and formatting checks pass. |
| Focused UI and transfer reviews | No remaining concrete findings in their final reviewed scopes. |
| VSIX contents | Nine expected files; bundle and media match validated source output. |

The current package is `extensions/vscode/dist/tadx.vsix`, version `0.1.0`, with a size of 41,990 bytes.
Its SHA-256 hash was rechecked while preparing the handoff:

```text
4e0520fe2a149ca01e341d9cc0dcdf6648c412f091f59b5a0a1889c1ead1df49
```

Treat this hash as the Windows package baseline, not an expected hash for a fresh Linux build.
Packaging timestamps and toolchains can change the archive bytes.
The bundle includes this verified package for convenience; rebuild it when changing source.

Regression-first evidence includes the blocked mixed-type selection, flattened project hierarchy, unloaded initial site, and incorrect classification of a failed submission with an operation ID.
The permission warning has both a real extension-host fixture and isolated CLI/SQLite regression coverage.
The mixed-transfer client test uses the real TypeScript client with synthetic CLI responses, not a live Tableau transfer.

## Coverage limits

- Native runtime coverage is Windows only; Linux, macOS, SSH, WSL, and container extension hosts remain untested.
- Live Tableau authentication, transfer, and policy behavior are not validated by these fixtures.
- Native tree structure is tested, but native tree pixel rendering was not captured.
- Dark, light, narrow, and high-contrast webview layouts were inspected through browser screenshots.
- Real Go CLI compatibility, the Activity inspection finding, and cache-generation evidence remain separate open items.
- No push, Marketplace publication, hosted CI dispatch, or saved Tableau consent change occurred during this work.

The initial broad Go checks encountered ignored historical fixtures under `workflow-evidence` that import deleted packages.
Maintained-package vet passed, and focused architecture and workbook checks passed after targeted corrections.
The complete broad command was not rerun after those corrections, so a full-repository pass is not established.
A clean Linux restoration that excludes those ignored fixtures can behave differently; run the required checks and report actual results.
Do not exclude failing maintained packages or weaken a gate to obtain a pass.

## Prepare Linux

Use Go 1.26 from the repository's `go.mod`, Node.js 22.15 or later, npm, Git, and a supported VS Code desktop runtime.
Install Linux browser and Electron runtime dependencies appropriate for the distribution.
The host runner needs a graphical display; use a desktop session or a configured Xvfb environment for a headless machine.
The browser suite launches headless Chromium.
Run native host checks as a normal user rather than bypassing Electron sandbox checks.

Reinstall Node dependencies on Linux instead of copying Windows `node_modules`.
The host runner builds its synthetic Go executable for the current platform and downloads the selected VS Code version.
Its isolated profile and fixture do not use saved Tableau credentials.
The fixture uses its own nested Go module; root CLI checks do not cover it.

## Build and validate the extension

From the restored repository root, build the Linux CLI:

```sh
go build -trimpath -o ./bin/tadx ./cmd/tadx
```

Then run the extension checks:

```sh
cd extensions/vscode
npm ci
npm run check
npx playwright install chromium
npm run test:ui
npm run test:host
node scripts/test-host.mjs --version=1.95.3
npm run package
```

Stop after a failed command and investigate its cause.
If Chromium reports missing system libraries, provision them through the machine's approved package-management process.
The default host command tests the stable release available at run time, which can differ from the recorded Windows baseline.
To reproduce that baseline version, run `node scripts/test-host.mjs --version=1.143.0` as an additional targeted check.
When no desktop display exists, use `xvfb-run -a npm run test:host` and `xvfb-run -a node scripts/test-host.mjs --version=1.95.3` after provisioning Xvfb.

Validate the synthetic host fixture separately:

```sh
cd tests/fixtures/host
gofmt -l .
go vet ./...
```

Formatting passes only when `gofmt` prints no filenames.
Use [CONTRIBUTING.md](../../../CONTRIBUTING.md) for the complete required CLI checks when modifying CLI contracts or preparing a push.
The fixture is synthetic, so passing host checks does not establish real-CLI or live-site compatibility.

## Focused CLI regressions

From the repository root, these existing tests cover the earlier permission behavior and workspace provenance:

```sh
go test ./internal/app -run '^(TestCachePersonalSpacePermissionsThroughCLI|TestCachePermissionDenialRetainsInventoryThroughCLI|TestWorkspaceStatusFullJSONPreservesArtifactSourceIdentity)$' -count=1
```

These are focused checks, not substitutes for the contribution gates.
No live site or credentials are needed for these fixtures.

## Try the restored build

From the repository root, install the rebuilt package into the Linux VS Code installation:

```sh
code --install-extension ./extensions/vscode/dist/tadx.vsix --force
```

Set **TADX: Executable Path** to the absolute path of the newly built `bin/tadx`, then run **Developer: Reload Window**.
For SSH or container sessions, build and configure the CLI on that workspace host.
Configure environments and workspaces through the CLI using approved credentials and site consent.
Do not copy private configuration, caches, tokens, or Windows credential-store material through the source handoff.
