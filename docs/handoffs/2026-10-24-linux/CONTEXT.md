# TADX extension handoff context

Original snapshot date: 2026-10-24.
Scope update: 2026-10-25.
Repository: `ahillspace/tadx`.
Working branch: `feat/vscode-artifact-explorer`.
Base commit: `8976a2b42f71c6ec586b64f7dbcd760bb2a27b0f`.

At the original validation baseline, the extension work was complete but uncommitted.
The entire `extensions/` directory and several architecture documents were untracked in the original capture.
The maintainer now plans to commit and push the intended source and handoff files for Git-based transfer.
Verify the reviewed commit includes them; a pull does not transfer untracked files.
Other tracked and untracked changes predate the extension work; preserve them and do not claim ownership of their behavior.
Use the [bundle restoration instructions](README.md) and manifest before making changes.

## Current task and authority

The latest request authorizes repository integration, versioning, CLI compatibility, release packaging, release automation, and upgrade guidance in the current checkout.
The user also explicitly included guided CLI installation and update buttons in this build.
The configuration page and other new product features remain deferred to Linux development.
The [requested-capabilities backlog](REQUESTED-CAPABILITIES.md) separates explicit requests, additional suggestions, exclusions, and undecided cleanup actions.
Use the latest source and validation record for the authorized release task's final result.
Read the current [repository instructions](../../../AGENTS.md) and the maintainer's latest instructions before starting another task.
Continue the assigned branch after restoration rather than resetting its uncommitted work.

The deferred backlog does not authorize a push, publication, hosted CI dispatch, persisted Tableau mutation consent, or PAT storage.
The prior user requested code review and testing for the extension build; do not treat that as indefinite authorization for unrelated reviews.
The `tadx-build` and `no-mistakes` skills require explicit invocation under repository instructions.
Use subagents for substantial independent work, with disjoint file ownership and one shared branch.

## Read first

1. Read the [architecture entry point](../../architecture/README.md) and [extension architecture](../../architecture/editor-extension.md).
2. Read the [extension guide](../../../extensions/vscode/README.md) and [implementation plan](../../vscode-extension-plan.md).
3. Read the [architecture review record](../../architecture/review.md) for known findings and evidence limits.
4. Read [validation and Linux setup](VALIDATION.md) before interpreting historical passes as Linux coverage.
5. Read [requested capabilities](REQUESTED-CAPABILITIES.md) before choosing the next configuration-page task.

## Implemented behavior

- **Sites** lists every configured environment as an independently expandable root.
- Projects nest under their parents, including empty ancestors, with full project paths available in tooltips and pickers.
- Expanding a site loads cached inventory without changing the CLI's saved default site.
- **Local workspaces** includes registered TADX workspaces outside the currently open VS Code folders.
- **Bulk Actions** replaces the user-facing **Open Bulk Inventory** label.
- Bulk download and upload accept mixed workbook, datasource, and flow selections, up to 100 artifacts from one source.
- Downloads use one destination workspace; uploads use one destination environment, project, and conflict mode.
- Type and search filters preserve selections; source changes clear them.
- The extension previews every resource group before one confirmation, then submits per-kind CLI commands with separate Activity receipts.
- An immediate failed, unknown, or canceled submission stops later groups; previously accepted workers continue independently.
- Local move and delete retain CLI previews, containment checks, and dirty-artifact guards.
- Pulse, lineage, and other local artifact kinds remain visible without unsupported transfer actions.

The 2026-10-25 release work adds these behaviors in the current source:

- Activation and explicit reload check advertised CLI command, preview, and batch capabilities.
- Supported commands remain usable when other capabilities are missing, including on older or development CLIs.
- **CLI Status and Setup** provides executable selection and guided installation or update through native controls.
- Guided setup uses an exact release and independent archive hash, explicit Guidance confirmation, and optional PATH or completion changes.
- Setup protects the normal shared user installation and leaves custom, package-manager, development, and newer installations with their owners.
- Source-built and pinned-release CLI read-contract suites complement the synthetic parser, browser, and host fixtures.
- Independent extension versioning and a manual release workflow prepare verified VSIX artifacts for explicitly selected publication.

The prepared extension version is `0.2.0`.
Use the [release guide](../../vscode-extension-release.md) and current validation record for delivery details and verification limits.
Adding release automation does not establish that a release has been published.

The CLI still owns each resource type's transfer semantics.
Mixed upload does not rewrite dependencies, credentials, schedules, or cross-site references.
An acquired artifact's unchanged local state does not prove equality with remote content.

## Permission warning correction

The user's HTTP 403 permission-coverage warning is separate from the Activity inspection defect below.
An existing CLI change skips permission collection for explicitly identified Personal Space workbooks while preserving their inventory.
It uses location metadata, not a bundled workbook name or hard-coded ID.
Genuine permission denials still mark permission coverage incomplete, and existing persisted generations can retain that evidence.

The extension previously copied cache-wide warnings into an artifact view that does not display permissions.
The corrected projection displays diagnostics from projects, workbooks, datasources, and flows, including `source.cache_warning`.
Global cache coverage remains in the model, and relevant inventory warnings, stale observations, and partial results remain visible.
Do not suppress genuine CLI permission errors or change collection rules to address this already-corrected UI issue.

## Architecture boundaries

The TypeScript package is independent of the Go CLI's build and release artifacts.
It calls the configured executable directly with argument arrays, `--full`, and `--json`; it does not use shell command strings.
The extension does not read SQLite, parse private configuration, store Tableau credentials, call Tableau APIs, or enforce remote mutation policy itself.
The CLI remains responsible for persistence, authentication, selectors, previews, mutation policy, and detached operations.

`Inventory` owns loaded observations and source selection; native trees project those observations.
`Operations` owns preview, confirmation, submission, and retained receipts.
`Controller` owns commands and native pickers; panels send bounded, validated actions and host-owned keys.
Keep webview CSP, path containment, exact identifiers, and unknown-outcome handling intact.

The extension runs on the workspace host in remote sessions.
That host owns its executable, configuration, cache, workspaces, and native credential access.
Browser-only and virtual workspaces remain unsupported.
Cached inventory lacks enough authoritative site identity to establish verified remote/local correlation.

## Architecture finding status

The [architecture review](../../architecture/review.md#extension-findings) preserves the original evidence and later resolutions:

- **Activity inspection, resolved 2026-10-25:** the client now accepts a retained failure from a successful inspection of the exact requested operation.
  Invalid identities, malformed outcomes, and failed inspection commands still reject; live transfer recovery remains separate acceptance work.
- **Cache observations:** status and independent resource-list processes can observe different generations without surfacing that combined observation limit.
  Per-kind generations can legitimately differ after scoped refreshes, so completeness and snapshot coherence require separate treatment.
- **CLI compatibility, resolved 2026-10-25:** activation checks capability contracts, and actual source and released CLI read suites complement handwritten fixtures.
  Those checks do not establish every command contract, live Tableau transfers, or native behavior on untested platforms.
- **Distribution, implementation resolved 2026-10-25:** versioned packaging and a manual publication workflow now exist.
  Publisher configuration, explicit release approval, and platform acceptance remain separate requirements.

Reproduce defects through the real CLI and end-user path before changing their implementation.
The cache-generation consistency finding remains open.
Do not infer broader fixes from the completed bulk-transfer reviews or release work.
The wider review also discusses project resolver consolidation and typed publication state; those remain proposals outside this extension handoff.
In particular, workbook `--include-pds` acquisition still has a production caller of the private project resolver fallback.

## Release work and deferred setup

The release implementation prepares Marketplace distribution and versioned VSIX downloads, with separate extension and CLI release ownership.
Stable and opt-in pre-release channels require explicit validation and publishing configuration.
The original capture used extension identity `ahillspace.tadx` and version `0.1.0`.
Use the current package manifest and release documentation for the prepared version and delivery process.

The current setup commands expose **Install TADX CLI**, **Choose TADX Executable**, and **Update TADX CLI** through **CLI Status and Setup**.
The future configuration-page integration is described in [REQUESTED-CAPABILITIES.md](REQUESTED-CAPABILITIES.md).
Prefer one shared user installation for the extension, terminal, and coding agents.
An older compatible CLI remains usable; missing required capabilities should produce actionable guidance and disable affected features.
Respect custom executable paths and require an explicit install or update action initially.

TADX already provides update inspection, installers, checksum validation, and self-update support.
The standalone CLI updater targets the latest release.
The extension instead passes an exact version and archive pin from `cli-release.json` to the maintained installers.
Installers also install bundled agent Guidance; an extension button must account for that side effect.
Detached transfers use the same executable, and multiple binary versions still share configuration, cache, and workspace formats.
Coordinate updates with active work; binary rollback alone does not establish state compatibility.

The configuration backlog does not authorize its implementation, automatic global replacement, Marketplace publication, or changes to Tableau consent.
