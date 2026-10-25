# VS Code extension validation

Initial build record: 2026-10-22.
Latest release and setup validation: 2026-10-25.
Native validation host: Windows x64.

The extension is implemented in `extensions/vscode` and produces an independent VSIX package.
The [implementation plan](vscode-extension-plan.md) records the architecture and accepted scope.
The [extension guide](../extensions/vscode/README.md) covers installation, operation, and development.

## Review corrections in extension 0.2.1

The corrected source is commit `0cf8f54ff79b572d12b7cedac1f9b05f2bc0e08e`.
Validation uses a clean committed checkout, excluding unrelated worktree changes.
The extension configuration page remains outside this build.

| Check | Result |
| --- | --- |
| Strict TypeScript, ESLint, unit and contract tests, bundle | Passed; 114 tests passed. |
| Source-built and checksum-pinned released CLI contracts | Passed; pinned release `1.0.0` on Windows amd64. |
| Browser scenarios | Seven groups passed; narrow light and mixed-upload dark layouts inspected. |
| Native VS Code hosts | Ten checks passed on each of `1.143.1` and `1.95.3`. |
| Synthetic host fixture | Go vet and formatting passed. |
| Windows PowerShell 5.1 installer fixtures | Passed, including intervening replacement/creation and native junction rejection. |
| Git Bash installer fixtures and syntax | Passed; its directory-link assertion explicitly skips because this host emulates links by copying. |
| PowerShell 7 installer fixtures | Unavailable on this host. |
| Fresh independent code review | No remaining findings in runtime, setup/delivery, source inventory, or capability-registry changes. |
| VSIX construction and verification | Both packages contain eleven expected files matching committed inputs; release metadata records `sourceDirty=false`. |
| Full normal Go suite | Passed; 111 tested packages and 6,424 tests/subtests in 257.891 seconds. |
| Full Go race suite | Passed; the same packages/tests in 380.422 seconds with the unchanged 15-minute timeout. |
| Documented fuzz targets | All three passed five-second runs. |
| Repository formatting, vet, module, and generation checks | Passed; the validation checkout remains clean. |
| Cross-builds with `CGO_ENABLED=0` | Passed for Windows amd64, Linux amd64, macOS amd64, and macOS arm64; these are compile checks. |
| Repository Node fixtures | Twenty passed; one Windows symlink-privilege check skipped. |
| Opt-in architecture inventory and workflow evidence | Combined package passed 119 tests/subtests; one symlink-privilege check skipped. |

The original installer race fixture now rejects replacement and preserves the intervening `1.1.0` executable.
Forced-reload regressions reject superseded data and errors, including clients that ignore cancellation.
Operation regressions preserve unknown startup outcomes, receipts, warnings, confirmed partial evidence, and explicit rejection states.
Both full Go runs report 37 explicit skips: 30 Windows symlink-privilege checks, four administrator-only policy fixtures, two POSIX-permission cases, and one intentional subprocess helper.
Ten additional packages contain no tests.
Configured Python support runs the local Hyper fixtures successfully; no `livecli` tag or live Tableau target is enabled.
The registry performance correction resolves the earlier cumulative race timeout without increasing its limit or weakening assertions.
The review and correction details remain in the [architecture review record](architecture/review.md#pr-35-review-on-2026-10-25).

| Artifact | Bytes | SHA-256 |
| --- | --- | --- |
| `extensions/vscode/dist/tadx.vsix` | 63,913 | `a59ff3a8620d79fd864d6bd1196faa0c3d9c43900a7d82a466309e2afbe30251` |
| Clean-checkout `extensions/vscode/dist/release/tadx-0.2.1.vsix` | 63,952 | `cd7b914b2ae2cd73775e66f8deac251c2a559cea5cb807749eb1f468fd8d880a` |

These checks do not establish native Linux/macOS/remote-host behavior, live Tableau transfers, or an actual user's guided CLI installation.
The stable host log includes a TextMate worker module error under the deep test-checkout path.
The minimum host log includes chat-registry network and utility-process shutdown warnings.
All extension assertions pass, but those logs are not warning-free.
The two high development-tool dependency audit findings remain documented in the [release guide](vscode-extension-release.md#current-toolchain-advisory).
No Marketplace publication, GitHub release, or hosted CI dispatch is part of this validation.

## Historical release and setup checks for 0.2.0

The final extension checks use source commit `c10dc23414dddcce5f35017d0e210896c0e32eba` and prepare extension version `0.2.0`.
They cover independent versioning, capability-based CLI compatibility, guided CLI setup, and the manual release path.
The configuration-page backlog remains unimplemented.

| Check | Result |
| --- | --- |
| Strict TypeScript, ESLint, unit and contract tests, bundle | Passed; 96 tests passed. |
| Actual webview browser scenarios | Seven groups passed. |
| Current stable native VS Code host | Ten checks passed on Windows, version `1.143.1`. |
| Minimum supported native VS Code host | Ten checks passed on Windows, version `1.95.3`. |
| Source-built Go CLI adapter contracts | Passed against isolated local fixtures. |
| Checksum-pinned CLI `1.0.0` adapter contracts | Passed with the Windows amd64 release archive. |
| Guided setup service fixtures | Passed; no actual user installation or update occurred. |
| Git Bash installer fixtures | Passed, including independent archive-pin rejection. |
| Windows PowerShell 5.1 installer fixtures | Passed, including independent archive-pin rejection. |
| PowerShell 7 installer fixtures | Not run; PowerShell 7 is unavailable. |
| Repository Node fixtures | Twenty passed; one Windows file-symlink privilege check skipped. |
| Local and versioned VSIX construction | Passed; both packages contain eleven expected files. |
| Versioned release artifact verification | Passed for `extension-v0.2.0`, stable channel; package contents match the bundle, media, and maintained installers. |

The actual CLI adapter suites verify version and capability discovery, environment and workspace reads, cached inventory, and mutation-status output.
They use isolated configuration and local fixtures without Tableau credentials or network calls to Tableau.
The published CLI archive is checked against the reviewed checksum pin before execution.
Other platform archive hashes identify release assets; they do not establish native execution coverage.

Setup tests cover immutable plans, exact versions and archive pins, target freshness, shared-installation ownership, optional profile changes, environment filtering, and bounded subprocess behavior.
They preserve unknown outcomes after interruption and verify that development, newer, or custom executables are not automatically replaced.
Synthetic installer fixtures and service tests do not establish an actual end-user installation or upgrade.

The Activity parser now accepts a retained failure from successful inspection of the exact requested operation.
Identity, response-shape, and command-failure regressions remain enforced.
The [architecture review](architecture/review.md#extension-findings) records that resolution and the capability/delivery fixes separately from the still-open cache-generation consistency finding.

### Packaged artifacts

| Artifact | Bytes | SHA-256 |
| --- | --- | --- |
| `extensions/vscode/dist/tadx.vsix` | 62,716 | `04ef068c4fc154ef7f835d66117c9c29fa5dc5dc0163fccd2cd42040ac682b41` |
| `extensions/vscode/dist/release/tadx-0.2.0.vsix` | 62,754 | `90bf9f201f7d915c9ab0ceafdfb8ed595da021892e7a8b7aa58fabb0f1393c8f` |

These hashes identify the recorded packages; fresh builds can differ because packaging metadata changes.
The release manifest records `sourceDirty=true` because the checkout contains additional working-tree changes.
This is a local validation artifact and cannot pass the clean-source publication gate.
No release tag, GitHub release, Marketplace publication, or hosted workflow dispatch results from these checks.

### Repository validation scope

Before integration, an isolated checkout of the staged source passes the complete Go test suite, Go vet, module checks, and generated-file consistency checks.
The full race command fails at its configured 15-minute package timeout in `internal/app`.
The active test, `TestReadDiscoveryWithoutPublicCursorsThroughCLI`, had run for four seconds when the package deadline expired; that observation does not identify it as the cause.
Other packages in the race run complete successfully, and the log contains no race-detector report.
This was a failed gate at the `0.2.0` baseline.
The complete `0.2.1` race run above resolves it after the registry performance correction.
All three documented fuzz targets pass five-second runs.
Cross-compilation passes for Windows amd64, Linux amd64, macOS amd64, and macOS arm64.
Cross-compilation does not establish native behavior on those targets.

The final merged-source checkout at `c10dc23414dddcce5f35017d0e210896c0e32eba` passes formatting, full Go vet, module tidy stability, generation, and capability-document stability checks.
Its normal and race delta Go suites each pass 603 tests and subtests across six tested packages.
Six native Windows symlink checks are skipped because the required privilege is unavailable.
Two Hyper parent tests initially skip without configured Python support.
Separate runs with the installed `tableauhyperapi` package pass five tests and subtests in both normal and race modes, covering those initial Hyper skips.
The isolated merged-source checkout remains clean after validation.
The `tests/live/...` package checks run without the `livecli` build tag and do not execute live Tableau qualification cases.
The delta suites do not constitute a complete merged-source test rerun.

### Remaining acceptance limits

Native extension runtime validation remains Windows-only.
Linux, macOS, SSH, WSL, and container hosts, live Tableau transfers, and real guided installation or update remain untested.
The manual release workflow is prepared but has not been dispatched.
No GitHub push, publication, credential persistence, or Tableau mutation-consent change is part of these validation runs.

`npm audit` reports two high-severity development-dependency findings associated with the MSAL advisory documented in the [release guide](vscode-extension-release.md#current-toolchain-advisory).
Those tooling dependencies are excluded from the VSIX runtime package.
The audit is not clean; retain the advisory and supported-update limitation instead of treating it as resolved.

## Historical build checks through 2026-10-24

| Check | Result |
| --- | --- |
| TypeScript strict type checking | Passed |
| ESLint | Passed |
| Extension unit and contract tests | 67 passed, including hierarchy, mixed transfers, and permission-warning regressions |
| Actual webview browser scenarios | 7 groups passed |
| VS Code extension host, current stable | Passed on 1.143.0 |
| VS Code extension host, minimum supported line | Passed on 1.95.3 |
| VSIX packaging and content inspection | Passed |
| Go vet for maintained action, internal, and command packages | Passed |
| Workspace provenance CLI E2E | Passed |
| Focused workspace CLI race tests | Passed |
| Current architecture suite | Passed |
| Workbook package after timing-fixture correction | Passed |
| Corrected protocol test repeated 30 times | Passed |
| Scoped Git whitespace checks | Passed |

Host tests use an isolated profile, workspace, and synthetic CLI executable.
They verify activation, inventory contracts, the three native views, editor webview tabs, local payload opening, picker cancellation, and partial cache recovery.
The final stable host run also verifies Pulse and lineage visibility without transfer actions.

Browser scenarios exercise the actual webview HTML, scripts, and styles with the content security policy intact.
They cover exact artifact messages, bulk selection, filtering, paging, source changes, unavailable inventory, hostile text, keyboard focus, and operation results.
Dark, light, narrow, and high-contrast layouts were inspected.
Screenshots remain in ignored `extensions/vscode/dist/ui` output.

## Review corrections

The requested code review covered the CLI adapter, native coordinator, operation handling, inventory projections, filesystem guards, package boundaries, and provenance addition.
Confirmed findings are resolved:

- Completed downloads, nested deletion results, and interrupted workers use the actual CLI status contracts.
- Confirmation remains nonmodal so the full preview stays readable.
- Local Pulse, lineage, and future artifact kinds remain visible without unsupported transfer actions.

Behavioral tests also caught and corrected mutable requests after preview and cancellation while a preview or confirmation remains pending.
Browser testing corrected control labels, pagination focus, selection-limit controls, duplicate paths, and theme handling.
The final focused review found no remaining concrete defects in the reviewed scope.
The subsequent [architecture refresh](architecture/review.md#extension-findings) identifies a status-inspection contract defect, mixed-cache observation limits, and compatibility-test gaps.
Those findings qualified the original fixture-based validation.
The 2026-10-25 release work resolves the inspection parser and compatibility/delivery gaps; the cache-generation observation limit remains open.

## Repository-wide check limits

The initial `go vet ./...` and `go test -count=1 ./...` commands included ignored historical fixtures under `workflow-evidence`.
Those fixtures imported deleted action packages, so the original working-tree commands did not pass.
Maintained-package vet checks pass.
The broad test run also exposed an architecture traversal timeout and a workbook protocol test with a 20 ms timing ceiling.
The current architecture suite passes without changes from this extension task.
The protocol test now uses a five-second ceiling while preserving protocol assertions and neighboring timeout tests.
Its package and 30 repeated runs pass.
The complete broad command was not repeated at that historical stage.
The later release checks use isolated source checkouts; their results appear in the [2026-10-25 validation scope](#repository-validation-scope).

The extension's tooling-only Go module boundary excludes installed Node dependency sources from root CLI package discovery.
Unrelated working-tree changes remain outside this extension review.

## Acceptance limits

Live Tableau authentication, upload, download, and policy behavior were not exercised against a real site.
Native runtime testing covers Windows only.
SSH, WSL, container, macOS, and Linux extension hosts remain untested.
Browser-only VS Code remains unsupported by design.
Cached inventory cannot provide the authoritative site identity required for verified remote/local correlation.
The extension displays source provenance without asserting such a match.

The build does not publish the extension, push commits, or change saved Tableau mutation consent.

## Permission warning correction

The reported warning concerns incomplete cached permission coverage, separate from the Activity status-inspection finding.
The existing CLI fix skips permission collection for explicitly identified Personal Space workbooks while retaining them in inventory.
Its regression and the genuine permission-denial regression both pass against an isolated HTTP fixture and persistent SQLite cache.

Before the extension correction, a real VS Code 1.143.0 host fixture reproduces the exact HTTP 403 warning alongside complete artifact inventory.
The extension previously copied cache-wide warnings into the site view, including scopes that the view does not display.
The corrected projection uses diagnostics from project, workbook, datasource, and flow reads, including their `source.cache_warning` metadata.
It retains global cache coverage in the model and preserves relevant warnings, stale observations, partial inventory, and read failures.
It does not change the CLI's permission warnings or permission collection rules.

Strict TypeScript checks, lint, and all 47 extension tests pass after the correction.
The new parser regression verifies relevant diagnostics and retained global permission evidence.
The host regression checks the user-visible snapshot with the exact reported warning and an independent workbook diagnostic.
After the correction, all seven host checks pass on VS Code 1.143.0 and 1.95.3.
The focused code review finds no concrete issues in the correction.
The rebuilt VSIX contains the validated bundle and nine expected files; its README links use the extension's repository subdirectory as their base.
These checks use isolated fixtures; live Tableau and pixel rendering are not revalidated for this data-projection change.

## Hierarchy and bulk actions

The site tree lists all configured sites as independent collapsed roots and preserves nested project folders, including empty ancestors.
Eight tree tests cover lazy expansion, filtering, exact identities, missing parents, cycles, and context actions.
The browser regression first reproduces the blocked mixed-type selection, then verifies mixed workbook, datasource, and flow selections in both bulk modes.
Type and text filters preserve selections; source changes clear them.
Dark upload and narrow light layouts receive visual inspection.

Mixed transfers preview every type before one confirmation and retain a receipt for each submitted resource group.
Tests cover shared destinations, frozen inputs, workbook-only options, preview denial, cancellation, skipped groups, partial outcomes, and restored receipts.
A cross-component test verifies the real CLI client's argument construction for three resource types sharing one destination project.
The underlying CLI runner uses synthetic responses and does not contact Tableau.

The focused reviews identify and resolve two defects before packaging:

- **Bulk Actions** initially leaves inventory unloaded when multiple sites have no default or remembered selection.
  A real host fixture reproduces this before the controller selects and loads the initial site.
- A failed first submission with an operation ID is incorrectly classified as partial progress.
  A regression reproduces the incorrect classification before the aggregate uses observed group states instead of receipt presence.

Strict type checking, lint, and all 67 unit and contract tests pass.
All seven browser scenario groups pass, and all eight host checks pass on VS Code 1.143.0 and 1.95.3.
The host fixture passes Go vet and formatting checks.
Final focused UI and transfer reviews report no remaining concrete findings in their scopes.
Live Tableau transfers, remote hosts, and native tree pixel rendering remain outside these fixture-based checks.
