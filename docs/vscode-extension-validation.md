# VS Code extension validation

Validation date: 2026-10-22.
Native validation host: Windows x64.

The extension is implemented in `extensions/vscode` and produces an independent VSIX package.
The [implementation plan](vscode-extension-plan.md) records the architecture and accepted scope.
The [extension guide](../extensions/vscode/README.md) covers installation, operation, and development.

## Completed checks

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
Those later findings remain recommendations and qualify the earlier fixture-based validation; this record does not claim they are fixed.

## Repository-wide check limits

The initial `go vet ./...` and `go test -count=1 ./...` commands include ignored historical fixtures under `workflow-evidence`.
Those fixtures import deleted action packages, so the commands do not pass for this working tree.
Maintained-package vet checks pass.
The broad test run also exposed an architecture traversal timeout and a workbook protocol test with a 20 ms timing ceiling.
The current architecture suite passes without changes from this extension task.
The protocol test now uses a five-second ceiling while preserving protocol assertions and neighboring timeout tests.
Its package and 30 repeated runs pass.
The complete broad command was not repeated after those focused corrections.

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
