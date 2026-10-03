# TADX maintainability assessment: baseline and pilot

Status: baseline inventory and three-workflow source-review pilot complete; repository-wide assessment and target approval remain pending.
The subsequent [architecture proposal](architecture-proposal.md) now supplies concrete package and ownership decisions across the repository.
This report retains the original pilot scope; use the proposal for the current approval decision.
Assessment date: 2026-10-03.
Source: TADX `dd22c33bd1d123066f25ec09e2d612cd95717400`.

## Conclusion

The pilot supports reconsidering ownership and dependency boundaries, not merely moving per-verb packages into resource folders.
Project, Workbook, and authentication use different handoff patterns, and composition owns behavior that maintainers must discover outside the action packages.
Required phases also hide behind optional interface assertions.
These differences make navigation and safe substitution harder than necessary.

The tests contain valuable protection, including changed-state rejection, exact native requests, and persistence failure handling.
Some individual tests promise more through their names than their assertions establish.
Refactoring should preserve the strong evidence, strengthen specific gaps, and improve test placement.
The pilot does not justify deleting tests or imposing a file-count target.

The common conventions below informed the subsequent architecture proposal and its broader scoped assessments.
No production migration is authorized by this report.

## Scope and evidence

The assessment reads committed Git blobs, excluding the older working checkout and uncommitted experiments.
The [reference baseline](../maintainability-assessment.md#reference-examples) pins GitHub CLI and Helm and records their strengths and limitations.
Source observations are not runtime reproductions.

| Inventory measure | Count |
| --- | ---: |
| Tracked files in scope | 1,298 |
| Go files excluding `_test.go` | 455 |
| Go `_test.go` files | 618 |
| Physical lines in non-test Go files | 87,337 |
| Physical lines in Go test files | 79,684 |
| Directories containing non-test Go files | 123 |
| Lexical Go test, example, fuzz, and benchmark declarations | 2,364 |
| Pilot test groups | 37 |
| Distinct declarations referenced by pilot groups | 92 |

Non-test Go includes tools and support code; it does not mean shipped production code.
Declarations exclude subtests and do not establish executed cases, build-tag applicability, or coverage.
Some pilot references assess only selected subcases within a declaration.
The pilot samples 46 paths and does not complete any entire category.
The remaining 2,272 declarations have no pilot reference and remain unassessed.

The [inventory](inventory-dd22c33.json) contains the full file and declaration universe.
The [coverage ledger](coverage-ledger.json) accounts for all 67 inventory areas, distinguishing partial review from inventory-only status.
Its path-based areas are navigation groups, not a semantic classification of responsibility.
The next assessment stage must classify responsibilities from behavior, including tests, fixtures, tools, generated files, and documentation.

The pinned revision has [nine successful CI checks](https://github.com/ahillspace/tadx/actions/runs/36669189664).
Those historical results establish an existing execution baseline, not test effectiveness or new local verification.
No TADX tests, external services, reference-project tests, or model tasks ran during this assessment.

## Compare the workflows

| Pilot | Current ownership | Maintainer consequence | Common-direction proposal |
| --- | --- | --- | --- |
| Project create and move | Per-verb actions, repeated records and app bridges, shared hierarchy adapter | Similar resolution and conversion behavior requires following multiple owners | Cohesive resource workflows with shared internal observations and explicit operation contracts |
| Workbook move | Cohesive action package, app translation, runtime-discovered adapter capability | Folder consolidation does not remove opaque handoffs or express the required capability | Explicit workflow dependencies and one owner for native request translation |
| Authentication and persistence | Action sequencing plus substantive cross-store coordination in app | Understanding login/logout requires discovering transaction, compensation, and recovery behavior in composition | Cohesive service workflows with explicit configuration and credential-store boundaries |

Exact traces, findings, and test evidence appear in the [Project](project-pilot.json), [Workbook](workbook-pilot.json), and [authentication](auth-pilot.json) ledgers.
Each ledger identifies unassessed operations and shared consumers.

## Findings that affect the common design

### Composition owns more than construction

Project bridges repeat resolution and record conversion; Workbook translates move requests inside app before forwarding through another adapter.
Authentication coordinates target freshness, cross-store ordering, compensation, and recovery inside app.
The authentication behavior is substantive and must retain an explicit owner, not disappear as supposedly redundant glue.
See `PROJECT-01`, `WORKBOOK-01`, and `AUTH-01`.

Source examples: [Project bridge](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/internal/app/content_remote.go#L536), [Workbook translation](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/internal/app/content_mutations.go#L142), and [credential coordination](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/internal/app/auth_credentials.go#L69).

Recommendation: composition constructs dependencies and binds command scope.
Workflow and boundary components own named behavior through explicit, narrow dependencies.
Keep additional layers only when each owns behavior, policy, or a meaningful representation boundary.

An alternative is retaining separate operation packages with shared contracts.
That can remove duplication, but choosing it only for selected categories preserves the navigation inconsistency.
The full assessment must evaluate one convention across both resources and local services.

### Required phases appear optional

Project actions discover resolution-phase support through interface assertions.
Workbook discovers mutation support at runtime, although the operation requires it.
Authentication discovers pre-prompt preflight through an optional interface.
See `PROJECT-02`, `WORKBOOK-01`, and `AUTH-02`.

Source examples: [Project phase dependency](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/actions/project/move/action.go#L26), [Workbook capability](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/internal/resources/workbook/adapter.go#L46), and [login preflight](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/internal/cli/auth/command.go#L157).

Recommendation: express mandatory phases and capabilities in construction or declared interfaces.
Do not remove freshness phases or combine independent observations merely to shorten execution.
Authentication's locked persistence check is distinct from earlier cached configuration lookup.

### Shared records do not imply shared output

Workbook already shares a normalized internal record while keeping operation-specific projections.
Project has opportunities to share internal observations without removing meaningful create/move differences.
See `WORKBOOK-03` and `PROJECT-01`.
The [Workbook projection test](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/actions/workbook/record_projection_test.go#L12) protects field omission when internal records expand.

Recommendation: share equivalent internal observations, not every type or workflow.
Keep external projections, mutation sequences, unknown outcomes, and recovery contracts explicit.
A universal lifecycle framework is not supported by this pilot.

## Assess tests by what they detect

| Evidence | What it establishes | Disposition |
| --- | --- | --- |
| Workbook move test named for revalidation | Preview suppression and one mutation; immutable observations and ignored mutation arguments do not prove revalidation or correct IDs | Strengthen with drift, exact arguments, and rejected writes |
| Project CLI integration with second-read drift | Changed source produces `target_changed`, two hierarchy reads, and zero writes | Keep; add missing target/payload assertions where needed |
| Native Project mutation tests | Exact HTTP/XML contracts, invalid-input request suppression, and malformed create-response handling | Keep; distinguish scripted acknowledgements from persisted remote state |
| Auth logout preview integration | Both stores survive preview; execution removes only the selected reference and protects secret output | Keep; relocate from broad utility/batch file |
| Configuration durability and replacement tests | Different installed/restored outcomes under controlled failures | Keep; do not claim power-loss or every multi-process sequence is covered |

References: [Workbook test](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/actions/workbook/move_test.go#L35), [Project integration](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/internal/app/command_runtime_e2e_test.go#L121), [Project protocol tests](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/internal/tableau/project/mutation_test.go), and [logout integration](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/internal/app/utility_batch_preview_test.go#L83).

Each ledger group records the protected contract, realistic failure mechanism, assertions, fake limitations, and proposed disposition.
No test is approved for immediate removal.
Search for existing coverage before adding replacements; a weak individual test does not prove a repository-wide gap.
Review-phase names warrant better organization, not automatic deletion.
Controlled seeded failures must demonstrate that critical assertions actually reject the relevant defect.

GitHub CLI provides useful examples of exact argument assertions and predictable command navigation.
Helm provides useful examples of failure injection followed by state assertions.
Neither reference supplies a substitute for TADX's target identity, policy, freshness, and truthful-outcome requirements.

## Keep correctness investigations separate

Two observations warrant isolated reproduction before any corrective change:

- `PROJECT-03`: native clients can return structured uncertain-outcome evidence that higher layers discard alongside errors.
  Existing error fields preserve some identity, so total identity loss is not established.
  Review final error handling and reproduce the user-visible consequence before declaring a defect.
- `AUTH-03`: successful configuration save, failed external deletion, and failed restoration can lose installed-state classification and orphan-specific recovery guidance.
  Reproduce the exact failure ordering and inspect both stores before deciding on a fix.

These are source-supported concerns, not reproduced production bugs.
Their characterization and any behavior correction belong outside a behavior-preserving structural migration.

## Proposed conventions for approval

Approve the following direction for the remaining assessment, not a final directory tree:

1. Use one documented command-to-workflow navigation convention across every category.
2. Group related workflows by resource or service responsibility, retaining explicit named operations and operation contracts.
3. Keep Cobra responsible for parsing, command concerns, invocation, and presentation handoff.
4. Keep composition responsible for construction and scope binding, not hidden domain or cross-store algorithms.
5. Inject narrow dependencies explicitly, including mandatory phases and required capabilities.
6. Give protocol and persistence translation clear boundary owners; avoid obligatory copy-only handoffs.
7. Share equivalent internal observations while preserving operation-specific outputs and truthful outcomes.
8. Organize tests by protected behavior and boundary, with explicit fake limitations and replacement evidence before removal.

Use the same conventions for content, administration, Pulse, workspaces, authentication, installation, and shared systems.
Different operation semantics do not justify unrelated wiring or navigation conventions.
Exceptions require concrete evidence, maintainer approval, and a narrow recorded scope.
Temporary old/new layouts require an exit condition and do not satisfy final consistency acceptance.

The target assessment must also inspect architecture tests and maintained instructions.
Existing rules that encode a superseded layout require explicit approval to change; they must not be weakened to make migration pass.

## Approval and remaining work

The [regression-gate specification](regression-gates.md) defines source, behavior, mutation safety, truthful outcomes, persistence, test effectiveness, architecture, platform, and maintainer gates.
Those additional gates are specified, not implemented or demonstrated.
No finite suite guarantees zero regressions.

The subsequent architecture proposal replaces the pilot-only decision point with the following remaining stages:

1. Approve or revise the concrete architecture proposal and its repository-wide ownership map.
2. Complete detailed contract, test, and shared-consumer verification for each planned migration slice; structural accounting is not exhaustive behavioral review.
3. Approve the resulting migration acceptance requirements and bounded checkpoints.
4. Build the required gates using test-first fixtures and prove that seeded defects fail them.
5. Establish a reliable baseline, then migrate in bounded checkpoints with explicit acceptance and recovery evidence.
6. After preceding gates pass, require Luna at medium reasoning effort to live-test affected actions before checkpoint acceptance and every executable action before final acceptance.

The final live stage verifies actual outcomes against native and affected-state evidence, not the model's final-answer formatting.
Missing fixtures or unavailable providers do not count as passes; required live coverage remains unresolved until completed or explicitly waived by the maintainer.

For implementation, test-first gate development and a separate review pass are recommended before the first production migration.
No production edits, harness changes, live Tableau mutations, installed-binary replacement, pushes, or releases form part of this assessment.

## Reproduce the assessment artifacts

From the repository root, validate committed inventory, source references, exact test symbols, and coverage accounting:

```powershell
python -B docs/maintainability/validate_assessment.py
python -B -m unittest discover -s docs/maintainability -p test_collect_inventory.py
```

To create a fresh inventory artifact, use an unused output path:

```powershell
python -B docs/maintainability/collect_inventory.py --revision dd22c33bd1d123066f25ec09e2d612cd95717400 --output inventory-new.json
```

The collectors refuse to overwrite evidence files.
Automated reference checks validate locations and accounting, not the correctness of human judgments.
Validation completed: the committed inventory and coverage ledger reproduce, all pilot source/test references resolve, and seven mocked inventory-tool tests pass.
These checks validate the assessment tooling, not TADX behavior or the proposed regression gates.
