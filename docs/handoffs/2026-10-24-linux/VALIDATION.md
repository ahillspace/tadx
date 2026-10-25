# Validation baseline and Linux setup

Original archive snapshot: 2026-10-24.
Release and setup validation update: 2026-10-25.
The results below distinguish the original archive baseline from the later Windows release checks.
They do not establish Linux runtime coverage.
See the [maintained validation record](../../vscode-extension-validation.md) for regression evidence and review scope.

## Current correction build: 0.2.1

Use extension `0.2.1` from current Git source rather than the earlier packages below.
Its clean source snapshot is `0cf8f54ff79b572d12b7cedac1f9b05f2bc0e08e`.
All 114 extension tests, strict TypeScript, lint, actual source/released CLI contracts, seven browser groups, and both ten-check Windows host suites pass.
The complete normal and race Go suites also pass: 111 tested packages and 6,424 tests/subtests in each run.
The race run finishes in 380.422 seconds under the unchanged 15-minute limit, resolving the earlier timeout.
Formatting, vet, module/generated-file stability, fuzzing, four cross-builds, repository Node fixtures, and the repaired opt-in architecture tools also pass.
Platform and privilege skips remain explicit in the maintained record; they are not counted as passes.
The four review findings are implemented, and fresh independent code review reports no remaining findings in the corrected scopes.
Both installer fixture suites pass on available tools; PowerShell 7 remains unavailable, and Git Bash's emulated directory-link assertion is explicitly skipped.
The Windows native junction fixture passes.

The local package is `extensions/vscode/dist/tadx.vsix`, 63,913 bytes, with SHA-256 `a59ff3a8620d79fd864d6bd1196faa0c3d9c43900a7d82a466309e2afbe30251`.
The clean release package contains eleven verified files and records `sourceDirty=false`.
See the [maintained validation record](../../vscode-extension-validation.md#review-corrections-in-extension-021) for exact artifacts, repository gate results, and remaining platform and tooling limits.
The original offline archive remains historical; transfer the current Git branch for continued work.

## Historical release and setup checks for 0.2.0

The final extension checks use source commit `c10dc23414dddcce5f35017d0e210896c0e32eba` and prepare version `0.2.0`.
Use current Git source for continued development; the original offline ZIP contains the earlier `0.1.0` baseline.

| Check | Recorded result |
| --- | --- |
| Strict TypeScript, ESLint, unit/contract tests, and bundle | Passed; 96 tests passed. |
| Actual webview browser scenarios | Seven groups passed. |
| Native VS Code host | Ten checks passed on Windows for both `1.143.1` and `1.95.3`. |
| Actual source-built and pinned-release CLI adapters | Passed for isolated read contracts; the published binary is Windows amd64 CLI `1.0.0`. |
| Guided setup fixtures | Passed without installing or updating the user's CLI. |
| Installer fixtures | Git Bash and Windows PowerShell 5.1 passed; PowerShell 7 was unavailable. |
| Repository Node fixtures | Twenty passed; one Windows file-symlink privilege check skipped. |
| VSIX packaging and release verification | Eleven expected files; local and versioned packages verified. |

The versioned package is `extensions/vscode/dist/release/tadx-0.2.0.vsix`, with a size of 62,754 bytes.
Its recorded SHA-256 is:

```text
90bf9f201f7d915c9ab0ceafdfb8ed595da021892e7a8b7aa58fabb0f1393c8f
```

The local `dist/tadx.vsix` package is 62,716 bytes with SHA-256 `04ef068c4fc154ef7f835d66117c9c29fa5dc5dc0163fccd2cd42040ac682b41`.
Fresh package bytes can differ; use their own generated checksums.
The release manifest records dirty source, so this local artifact is not eligible for publication.

The [maintained record](../../vscode-extension-validation.md#repository-validation-scope) separates staged-source Go checks from final integrated-source checks.
The complete normal Go suite passes, but the full race command reaches the 15-minute package timeout in `internal/app`.
Other race packages and the final merged-source delta race checks pass; these results do not replace the failed full race gate.
That earlier timeout is resolved by the complete `0.2.1` race run recorded above.
The final merged-source delta suites pass 603 tests and subtests in both normal and race modes, with six native Windows symlink privilege skips.
Separate configured Hyper runs pass five tests and subtests in both modes, covering the initial Hyper skips.
Full formatting, vet, module tidy, and generated-file stability checks pass in the clean merged-source checkout.
Native Linux/macOS/remote hosts, live Tableau behavior, and actual guided installation or update remain untested.
The manual release workflow was not dispatched, and these checks did not publish packages or change Tableau consent.
Two high-severity development-dependency audit findings remain documented in the [release guide](../../vscode-extension-release.md#current-toolchain-advisory).

## Historical archive checks through 2026-10-24

| Check | Recorded result |
| --- | --- |
| Strict TypeScript, ESLint, bundle, and unit/contract tests | `npm run check` passes; 67 tests pass. |
| Actual webview browser scenarios | Seven scenario groups pass with CSP enabled. |
| VS Code 1.143.0 native host | Eight checks pass on Windows. |
| VS Code 1.95.3 native host | Eight checks pass on Windows. |
| Synthetic host fixture | Go vet and formatting checks pass. |
| Focused UI and transfer reviews | No remaining concrete findings in their final reviewed scopes. |
| VSIX contents | Nine expected files; bundle and media match validated source output. |

The historical archive includes extension `0.1.0`, with a size of 41,990 bytes.
Its SHA-256 was checked when the original handoff archive was prepared:

```text
4e0520fe2a149ca01e341d9cc0dcdf6648c412f091f59b5a0a1889c1ead1df49
```

This hash identifies only the archived `0.1.0` package, not the current build or a fresh Linux build.
Packaging timestamps and toolchains can change the archive bytes.
The original bundle includes that package for convenience; use current Git source and rebuild for continued development.

Regression-first evidence includes the blocked mixed-type selection, flattened project hierarchy, unloaded initial site, and incorrect classification of a failed submission with an operation ID.
The permission warning has both a real extension-host fixture and isolated CLI/SQLite regression coverage.
The mixed-transfer client test uses the real TypeScript client with synthetic CLI responses, not a live Tableau transfer.

## Coverage limits

- Native runtime coverage is Windows only; Linux, macOS, SSH, WSL, and container extension hosts remain untested.
- Live Tableau authentication, transfer, and policy behavior are not validated by these fixtures.
- Native tree structure is tested, but native tree pixel rendering was not captured.
- Dark, light, narrow, and high-contrast webview layouts were inspected through browser screenshots.
- Actual Go CLI read contracts and the Activity inspection parser now have regression coverage; live transfer recovery remains separate acceptance work.
- Cache-generation consistency remains an open finding.
- No push, Marketplace publication, hosted CI dispatch, or saved Tableau consent change occurred during this work.

The original broad Go checks encountered ignored historical fixtures under `workflow-evidence` that import deleted packages.
The later release task uses isolated source checkouts for repository-wide validation, separate from the original dirty checkout.
Use the [maintained validation record](../../vscode-extension-validation.md) for those dated results and their exact scope.
Run the required checks on Linux and report actual results.
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
npm run test:cli
npm run test:released-cli
npx playwright install chromium
npm run test:ui
npm run test:host
node scripts/test-host.mjs --version=1.95.3
npm run package
```

Stop after a failed command and investigate its cause.
If Chromium reports missing system libraries, provision them through the machine's approved package-management process.
The default host command tests the stable release available at run time, which can differ from the recorded Windows baseline.
The latest recorded Windows stable host is `1.143.1`; use that explicit version only when reproducing the dated baseline.
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
The source and pinned-release CLI suites cover actual read contracts in isolated fixtures; the released binary is verified before execution.
The pinned-release suite needs GitHub release access and does not contact Tableau.

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
