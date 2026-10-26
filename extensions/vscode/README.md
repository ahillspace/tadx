# TADX Artifact Explorer

Browse cached Tableau inventory and registered TADX workspaces from VS Code.
Download remote artifacts, upload exact local copies, and manage local artifact locations.

## Requirements

- VS Code 1.95 or later on a desktop or remote Node extension host.
- A compatible TADX CLI, selected from an existing installation or installed through **TADX: CLI Status and Setup**.
- A trusted workspace.
- TADX environments and workspaces configured through the CLI.

Activation checks the CLI's version and advertised command capabilities before loading inventory.
Older and development builds remain usable when their command contracts are compatible.
Unsupported commands report actionable errors while supported commands remain available.
Inventory responses are also checked for required fields.
Build TADX from the same checkout when developing CLI and extension changes together.

With SSH, WSL, containers, or Codespaces, install TADX on the workspace host.
That host owns the CLI configuration, cache, registered workspaces, and native credential access.
Browser-only and virtual workspaces are unsupported.

## Install

Open **Extensions**, select **Install from VSIX**, and choose `dist/tadx.vsix` from this package's build output.
Set **TADX: Executable Path** if `tadx` is unavailable on the extension host's executable search path.
The setting accepts an executable path without shell arguments.
Reload the VS Code window after changing executable settings.

## Set up or update the CLI

Run **TADX: CLI Status and Setup** to inspect the executable, version, compatibility, and workspace host.
Use **Choose TADX Executable** to validate and select an existing installation.
Use **Install TADX CLI** to install the checksum-pinned release into the normal shared user location.
Use **Update TADX CLI** to update or repair that installation to the extension's verified release.
Guided updates recognize the standard shared user location rather than an installation ownership receipt.
Preview rejects other paths, linked executables, development builds, and versions newer than the verified release.
The installer refuses replacement if the approved executable changes after preview.

Setup shows the exact release and destination before confirmation.
It also refreshes bundled agent Guidance, which is included in the confirmation.
PATH and shell-completion changes are optional and initially unselected.
Finish other TADX commands and detached transfers before approving setup.
Cancellation or failed verification can leave partial installation state; inspect the installation before retrying.

The [release and upgrade guide](../../docs/vscode-extension-release.md) explains shared installation ownership, compatibility, extension updates, and publishing.
Setup does not change Tableau credentials, saved mutation consent, environments, or workspace registrations.
The configuration page remains deferred.

## Browse inventory

Select the **TADX** Activity Bar icon.
The **Sites** view lists every configured environment as an expandable root, like **Local workspaces**.
Expand a site to load its cached inventory without changing the saved CLI default.
Projects appear as folders within their parent projects, and artifacts group by workbook, datasource, or flow inside each project.
Project tooltips and destination pickers retain full paths.

**Reload Inventory** reads the local cache and local workspace state.
**Refresh Site Cache** explicitly fetches inventory from Tableau.
Stale, uninitialized, and partial inventory remain visible.
Displayed warnings describe the project, workbook, datasource, and flow inventories.
Use `tadx cache status --full` to inspect cache-wide coverage, including permissions that this extension does not display.
The CLI bounds each list to 10,000 records; the extension does not imply complete coverage when that limit truncates results.

The **Local workspaces** view lists registered TADX workspaces, including workspaces outside the open VS Code folders.
Expand a workspace to scan it.
Pulse definitions, lineage artifacts, and other local kinds remain visible for local file actions.
Upload and download support workbooks, datasources, and flows.
**Unchanged locally** means the artifact matches its acquired baseline.
It does not prove equality with its current remote source.
Source provenance appears when the CLI exposes it.
The extension does not infer remote/local matches from names or content IDs alone.

## Download and upload

Open an artifact to see its details and available actions.
Select **Download** for a cached remote artifact, or **Upload** for an exact local copy.

For a download:

1. Select a registered destination workspace.
2. Select replacement and workbook dependency options.
3. Read the preview in the editor.
4. Confirm the exact operation.

For an upload:

1. Select a destination environment.
2. Search full project paths and select the destination project.
3. Select **Create new content** or **Overwrite matching content**.
4. Read the preview and confirm the operation.

Use **Back** to change the destination before previewing.
Create collisions stop the preview.
To overwrite, start a new upload and explicitly select overwrite.
The extension does not migrate dependencies, saved credentials, schedules, or cross-site references.

TADX checks saved site consent and administrator policy.
If remote mutations are disabled, the extension can show a read-only preview but cannot upload.
The extension never enables consent or stores credentials.

## Use bulk actions

Run **TADX: Bulk Actions**, or use the **Bulk Actions** button in either inventory tree.
Select **Download from site** or **Upload from workspace**.
Choose one source and select up to 100 artifacts, mixing workbooks, datasources, and flows as needed.
Downloads share one destination workspace.
Uploads share one destination environment, project, and conflict behavior.
There is no per-artifact destination mapping.
Selections remain explicit when type filters, search filters, or pages hide selected rows.
Changing the source clears the selection.

TADX commands operate on one resource type, so the extension groups the selection by type.
It previews every group before one confirmation and retains a separate Activity receipt for each submitted group.
Submission stops after an immediate failure, unknown outcome, or cancellation; later groups are reported as not submitted.
Already accepted workers continue independently and are not rolled back.
Uploading related workbooks and datasources together does not rewrite their source references.

## Manage local files

Artifact actions can open the native payload, open metadata, reveal the artifact folder, move it to another registered workspace, or delete it locally.
Move and delete use TADX previews and filesystem guards.
Deleting an artifact with local changes requires an explicit choice to include those changes.
These actions do not move or delete remote Tableau content.

**Open in Tableau** appears only when the CLI provides an authoritative browser URL.
Current CLI projections expose that URL for datasources.
The extension does not construct browser addresses from Tableau LUIDs.

## Check operations

**Activity** retains accepted operation IDs and their available results.
Use **Check Operation** to read current status.
Receipts survive window reloads; restored entries require a status check.
Partial failures keep confirmed successes and failure details in the operation editor.

Canceling an invocation does not roll back an accepted operation.
If an outcome is unknown, check its receipt before retrying.
An unacknowledged worker launch appears as unknown and retains its operation ID for status checks.
The extension does not automatically resubmit failed writes.
After upload completes, use **Refresh Site Cache** to update the cached remote inventory.

## Develop and validate

The package has no runtime npm dependencies.
Its tooling-only Go module boundary keeps root CLI checks out of installed Node dependencies.
Its bundle calls the installed TADX executable directly with argument arrays and JSON output.
It does not read SQLite, parse private configuration files, or call Tableau APIs.

Use Node.js 22.15 or later and Go from the repository's `go.mod` for the host fixture.
From the repository root, run:

```powershell
cd extensions\vscode
npm ci
npm run check
npm run test:cli
npm run test:released-cli
npx playwright install chromium
npm run test:ui
npm run test:host
npm run package
```

To test the minimum supported VS Code release, run:

```powershell
node scripts\test-host.mjs --version=1.95.3
```

The host tests download VS Code and use an isolated profile, workspace, and synthetic CLI executable.
They do not access saved credentials or mutate Tableau content.
The source-built and pinned-release CLI tests consume actual CLI output with isolated configuration, workspace fixtures, and cached reads.
The pinned-release test downloads and verifies the archive before running it; neither contract suite calls Tableau.
Browser tests exercise the actual webview HTML, scripts, and styles with the CSP intact.
Screenshots are written to ignored `dist/ui` output.

To debug, open this package folder in VS Code and run the **TADX Extension** launch configuration.
Use synthetic fixtures for destructive operations.
The first release requires a separate real-site acceptance check before production publishing workflows are treated as validated.

## Package boundary

The [implementation plan](../../docs/vscode-extension-plan.md) records the agreed scope and architecture.
The [editor architecture guide](../../docs/architecture/editor-extension.md) traces current components, state, operations, and trust boundaries.
The [architecture review](../../docs/architecture/review.md) records known contract defects and proposed improvements separately from implemented behavior.
The extension has its own version and VSIX artifact while sharing the repository with its CLI contract.
Packaging does not publish to the Marketplace.
The [release guide](../../docs/vscode-extension-release.md) owns version preparation, validated packages, and explicit GitHub or Marketplace publication.
