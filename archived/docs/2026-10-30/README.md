# Documentation sweep: 2026-10-30

This record accounts for all 43 tracked files under `docs/` at baseline `9cde64398e71bc43a5b4a2ce194974c9cf5d75e8` on `ahillspace/tadx` main.
Three Luna high agents reviewed disjoint document families, with coordinator integration against that same source baseline.
The task uses one shared clean worktree; unrelated branch changes and local-only private handoffs are excluded from publication.
This is documentation reconciliation with source checks, not a line-by-line code audit or live Tableau qualification.

## Dispositions

Keep means the document still serves a current reader and no material correction was found in the inspected contracts.
Update means current material needs corrected claims, navigation, or procedure.
Archive means historical or superseded material no longer belongs in the current reading path.
The original capability map is also archived, while its canonical path now contains v2.

| Baseline document | Decision | Reader value or correction |
| --- | --- | --- |
| `docs/architecture/README.md` | Update | Current reading path, runtime trace, and source limits replace historical review dependencies. |
| `docs/architecture/dependencies-and-delivery.md` | Update | Current contributor checks replace links to archived validation logs. |
| `docs/architecture/editor-extension.md` | Update | Current ownership and cache-generation limits stand alone without completed plans or historical review links. |
| `docs/architecture/index.html` | Archive | Superseded by the source-linked SVG/Excalidraw and current runtime walkthrough; stale verification footer. |
| `docs/architecture/resources-and-state.md` | Update | Current metadata owners and publication-test links replace stale references. |
| `docs/architecture/review.md` | Archive | Historical dirty-tree audit and PR-era findings; enduring limits now live in current architecture pages. |
| `docs/architecture/runtime.md` | Update | Trace-level verification limits replace dependency on a dated audit. |
| `docs/architecture/search.md` | Keep | Source-selection map still identifies native search, inventory, cache, and query boundaries. |
| `docs/architecture/tadx-architecture.excalidraw` | Keep | Editable source-linked counterpart of the maintained diagram; JSON checked. |
| `docs/architecture/tadx-architecture.svg` | Keep | Current component overview; XML and source links checked. |
| `docs/command-structure.md` | Keep | Current public command organization and help-design contract. |
| `docs/evidence/README.md` | Keep | Canonical evidence index and official upstream-documentation entry point. |
| `docs/evidence/admin-rest-contract.md` | Keep | Admin read/mutation contracts still protect concrete resource adapters. |
| `docs/evidence/content-update-rest-contract.md` | Keep | Content metadata update semantics remain relevant to mutation adapters. |
| `docs/evidence/datasource-lifecycle-rest-contract.md` | Update | Separate historical asynchronous Tableau jobs from the current detached local publisher. |
| `docs/evidence/flow-rest-contract.md` | Keep | Flow ownership, publication, and metadata limitations remain relevant. |
| `docs/evidence/group2-inventory-rest-contract.md` | Keep | Bounded inventory and collector contracts remain implementation evidence. |
| `docs/evidence/lineage-metadata-contract.md` | Keep | REST/Metadata identity and lineage distinctions remain necessary. |
| `docs/evidence/metadata-semantics-contract.md` | Keep | Metadata identity, visibility, and pagination limits remain necessary. |
| `docs/evidence/permission-mutations-rest-contract.md` | Keep | Permission changes retain explicit resource, identity, and rule semantics. |
| `docs/evidence/phase1-rest-contract.md` | Update | Distinguish the recorded upstream job failure from the current workbook publisher. |
| `docs/evidence/project-rest-contract.md` | Keep | Project resolution and mutation semantics still govern implemented behavior. |
| `docs/evidence/pulse-live-contract.md` | Keep | Recorded Pulse observations retain explicit historical scope and limitations. |
| `docs/evidence/rest-version-negotiation-contract.md` | Keep | Automatic REST version selection remains implemented in authentication. |
| `docs/evidence/session-rejection-contract.md` | Keep | Main retains saved sessions and rejection handling; do not substitute unmerged branch behavior. |
| `docs/evidence/tableau-mcp-delegation.md` | Keep | Lifecycle versus analytical/MCP ownership remains a core routing boundary. |
| `docs/getting-started.md` | Keep | Current operator setup, credentials, policy, discovery, previews, and maintenance path. |
| `docs/managed-policy.md` | Keep | Current machine-wide policy templates, protection checks, and recovery runbook. |
| `docs/potential-improvements.md` | Archive | Unapproved hypothetical rewrite discussion, rather than current implementation or contribution guidance. |
| `docs/reference/capabilities.json` | Keep | Fresh generator output matches the current registry; retain machine-readable data. |
| `docs/reference/capabilities.md` | Keep | Fresh generator output matches the current registry; retain readable contracts. |
| `docs/reference/capability-map.html` | Update | Promote v2 with CLI-only command counts/tree, explain both MCP companions, regenerate main data, and repair public links. |
| `docs/reference/shorthand.md` | Update | Complete command and flag aliases from internal/cli/shorthand.go. |
| `docs/repository-structure.md` | Update | Point builders to the maintained architecture diagram and source guide. |
| `docs/resource-refactoring-playbook.md` | Keep | Current responsibility boundaries and behavior-preserving change procedure. |
| `docs/security.md` | Update | Repair the installation-upgrade anchor. |
| `docs/toon.md` | Keep | Current compact output, encoding, expansion, and failure contract. |
| `docs/vscode-extension-plan.md` | Archive | Completed implementation plan superseded by the current editor architecture and extension guide. |
| `docs/vscode-extension-release.md` | Update | Replace time-sensitive repair claims with lockfile evidence and a release-time audit procedure. |
| `docs/vscode-extension-validation.md` | Archive | Dated validation history cannot establish current release acceptance. |
| `docs/website.md` | Update | Replace one-time launch history with the maintained deployment procedure and canonical v2 source. |
| `docs/workbook-metadata-coverage.md` | Archive | Historical coverage assessment superseded by current resource architecture, evidence contracts, and registry. |
| `docs/workspaces.md` | Update | Document artifact move/delete, unregister, deletion guards, and cleanup from current owners. |

## Evidence and canonical owners

The current [documentation index](../../../docs/README.md) provides the maintained reading path.
Source checks cover CLI registration and shorthand, action/resource/provider ownership, authentication and policy, cache and workspace stores, publication, extension CLI consumers, manifests, and delivery workflows.
API evidence remains bounded to its stated captures and official contracts; historical live results do not become new live verification.
The runtime, resource/state, search, delivery, and editor architecture pages retain source owners and their practical limits.
Generated capability references match fresh generator output.
The promoted v2 map uses regenerated main data rather than a different branch's snapshot.

## Preserved originals

Seven original files are preserved byte-for-byte beneath this directory with their original docs-relative paths.
Their internal links retain their original snapshot context and are not maintained navigation.
The repository normally ignores `archived/`; these selected records are deliberately tracked for this requested preservation.

## Scope added during the sweep

The root README is checked after the docs review, including release status and links to the current index and hosted map.
AGENTS, CONTRIBUTING, and the extension README lose links to superseded current docs.
The shipped TADX skills now explain parallel same-action batches, selection-ordered outcomes, and explicit workbook datasource bindings.
The lifecycle reference distinguishes source content URLs or unique exact captions from destination datasource LUIDs, and schema selection from publication binding.
Per-row batch bindings and explicit mappings for every Pulse bundle datasource remain in their focused references.
Current synchronous publication and detached local worker behavior replace stale asynchronous-publication guidance.
The repository build and exploration skills need no corresponding machinery update.
Local-only and unmerged handoffs, private fork tasks, and machine-specific checklists were reviewed separately and contain no missing public current guidance.
Their original checkout files remain untouched and are excluded from this main-only change.

## Verification

- Formatting: `gofmt -l .` returns no filenames; `git diff --check` passes.
- Static analysis: `go vet ./...` passes.
- Tests: `go test -race -count=1 -timeout=15m ./...` passes on Windows.
- The ordinary `go test -count=1 ./...` run passed every package except a bundled-skill comparison whose binary embedded a file during an edit.
  Its exact failing installation test passes after the edit settles; the subsequent complete race suite also passes.
- Capability generation is stable after all map edits; generated Markdown, JSON, and the final map remain byte-identical after regeneration.
- `go mod tidy` leaves module metadata unchanged.
- Static website packaging passes all five tests.
- Headless Edge passes nine browser scenarios at each of 1440, 768, 390, and 320 CSS pixels, including CLI-only totals and branches, filtering, keyboard movement, reset, and authority guidance.
- Final desktop and phone screenshots were inspected, including the companion MCP section; no material layout blocker was found.
- The offline exploration launcher passes 15 tests and skips one file-symlink test because of platform permission.
- Shell installer syntax and fixtures pass in Git Bash; its native directory-link check is skipped because that shell copies link targets.
- Windows PowerShell 5.1 installer fixtures pass; PowerShell 7 is unavailable.

The first sandboxed test attempt was limited by denied native path, credential-lock, VCS, and browser subprocess access.
The Go suite and headless browser checks were rerun as normal user processes, without administrator elevation or weakened product policy.
The Python skill validator could not start in this environment; existing frontmatter is unchanged, current references resolve, and bundled installation equality is checked by the passing Go tests.
Native macOS/Linux runtime checks, privileged policy fixtures, live Tableau operations, cross-compilation, and bounded fuzz runs are outside this documentation-only verification.
Hosted CI, releases, and Pages deployment are not dispatched.
An ordinary push to main updates source; the website's explicit publication workflow remains separate.
