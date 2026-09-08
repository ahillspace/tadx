# tadx build TODO

Running backlog of things to build.
Add new items under Backlog; move to Done when shipped.

## Backlog

### Current remediation build

The experiment runs have finished.
Preserve their raw transcripts and evaluate outcomes from recorded commands and final-state evidence.
Agent summaries alone do not establish a pass.
Keep private environments, credentials, raw transcripts, and machine paths out of tracked reports.

One coordinator owns this backlog and shared integration files.
Agents share the existing feature branch and own disjoint implementation paths.
Reproduce confirmed defects through the closest end-user entry point before changing behavior.
Run focused regressions as each fix lands, then the standard integration checks and affected workflow exercises.
The owner handles the comprehensive branch review.

| ID | Work | Status / owner | Completion evidence |
| --- | --- | --- | --- |
| EVAL-01 | Consolidate and deduplicate experiment findings; separate CLI, Guidance, provider, and harness failures. | Complete locally | 26 findings across 90 recorded runs; evidence and rerun matrix in ignored evaluation output. |
| PULSE-01 | Accept eligible dimension fields with COUNT and COUNT_DISTINCT, preserving other eligibility checks. | Implemented; focused tests pass | CLI regression for both counts and rejected incompatible fields. |
| PULSE-02 | Preserve dimension input order while deduplicating. | Implemented; focused tests pass | Preview and serialized request retain intentional order. |
| PULSE-03 | Include hidden calculation dependencies in field eligibility and USER aggregation analysis. | Implemented; focused tests pass | HTTP-backed paginated schema regressions for hidden dependencies. |
| PULSE-04 | Reject a fork period outside the definition's allowed granularities before mutation. | Implemented; focused tests pass | CLI regression verifies no mutation request on incompatibility, including prewrite changes. |
| PULSE-05 | Require at least one adjustable dimension and verify running-total safeguards. | Implemented; focused tests pass | Retain existing running-total restrictions; numeric-only MIN/MAX restrictions need provider evidence. |
| PULSE-06 | Improve duplicate-definition and malformed-create errors; verify uncertain-write identity preservation and reconciliation. | Implemented; focused tests pass | Preserve upstream details; never infer a specific cause from an ambiguous status alone. |
| READ-01 | Remove opaque cursors from normal output and provide usable internal continuation for search, schema, and Pulse inventory. | Implemented; focused tests pass | All render surfaces are cursor-free; content/admin --all reaches complete inventory within 10000 records; defaults remain small. |
| READ-02 | Support valid project names containing a slash without breaking authoritative LUID resolution. | Implemented; focused tests pass | HTTP regression from exact-ID operations; path collisions remain explicit ambiguities. |
| READ-03 | Investigate skipped workbook/flow inventory rows and truthful partial results. | Slash-project cause fixed; focused tests pass | Both live and catalog inventory retain valid slash-named projects and descendants; historical dropped-record totals require raw upstream evidence. |
| READ-04 | Make catalog refresh resilient to individual permission denials. | Implemented; focused tests pass | Useful inventory remains; unavailable permission coverage is explicit and persisted. |
| READ-05 | Return usable catalog status before the first refresh. | Implemented; focused tests pass | Uninitialized state gives refresh guidance without SQL diagnostics. |
| READ-06 | Apply datasource project-name filtering to catalog reads. | Implemented; focused tests pass | Real cached project names resolve by LUID, not display-path suffixes; incomplete coverage requests refresh without fallback. |
| READ-07 | Investigate duplicate native-search identities. | Awaiting raw upstream evidence | Existing code rejects duplicates within and across pages; do not collapse potentially conflicting rows without evidence. |
| UX-01 | Add --env as an alias for --environment. | Implemented; CLI tests pass | Both forms resolve identically, including explicit write targets. |
| UX-02 | Improve recovery for mistaken --site/--terms and unsupported search types. | Implemented; CLI tests pass | Actionable, bounded usage errors without accepting ambiguous syntax. |
| UX-03 | Expose valid permission capabilities and actionable user-create errors. | Implemented; focused tests pass | Correct resource-specific choices and preserved provider diagnostics. |
| UX-04 | Remove unrelated next-command hints from datasource schema output. | Verified | Compact/full schema fixtures pass; unrelated Pulse-creation hint removed. |
| INSTALL-01 | Enable shell completion during installation with opt-out and a simple manual setup path. | Implemented; installer tests pass | Managed hooks/backups, opt-out, idempotence, and uninstall preservation; native Zsh/Fish execution remains untested here. |
| GUIDE-01 | Integrate reviewed Pulse authoring references after CLI repairs. | Implemented; package and recipe tests pass | CLI examples, verified commands, no raw REST JSON or temporary bug workarounds. |
| GUIDE-02 | Improve metric and follower identity discovery routing. | Implemented; recipe tests pass | Exact executable discovery paths reuse existing commands. |
| GUIDE-03 | Preserve the user's root-skill search explanation in the bundled installer package. | Implemented; install tests pass | Installed Guidance matches bundle for Codex, Claude, and Cursor. |
| GUIDE-04 | Clarify datasource inspect/schema versus Tableau MCP routing. | Implemented; recipe tests pass | Structural discovery stays in TADX; analytics routes explicitly. |
| EVAL-02 | Rerun affected stable exercises using the rebuilt CLI and installed Guidance. | Six targeted runs assessed; cleanup verified | Five sessions completed; Pulse created/forked a metric and verified followers but exceeded its time budget. Independent reads verified all 11 run-created remote resources absent; local artifact residue was cleaned. |
| EVAL-03 | Complete suite coverage and strengthen evidence capture. | Implemented; targeted batch recorded | Three missing exercises added separately, preserving the original 42; raw evidence retained, guard help false positives corrected, and final-state checks separated from agent claims. |

### Locked Guidance decisions

- Use DAY unless metadata clearly identifies coarser source grain or the user explicitly requires otherwise.
  Do not query datasource values just to discover minimum granularity.
- Inspect relevant existing definitions early, reuse appropriate conventions, and reuse/fork an equivalent definition when it actually matches the requested meaning.
- Include dimensions reasonably related to the metric and favor inclusion when usefulness is uncertain.
  Exclude clearly unrelated, sensitive, purely technical, or unusable fields, without rejecting a field solely for cardinality or identifier-like appearance.
- Use CLI examples with verified field IDs and flags.
  Do not include raw request JSON or ask runtime agents to repair serializers.
- Require the Pulse create/fork preview and saved-state verification described by the approved authoring workflow.
- Fix CLI defects before removing their workaround text from installed Guidance.
- Each reference starts with available actions, purpose, and relevant flags.
  Keep the root intent-to-reference table explicit about when Tableau MCP applies.
- Preserve the existing mutation environment gate; it is not an independent security boundary.

### Deferred ideas

- Explore a local Tableau MCP installer; do not implement MCP connection management as part of this remediation.
- Consider connected-app direct trust separately from current PAT authentication.
- Datasource composition remains indefinitely deferred; ordinary pull and publish remain supported.
- Long-operation progress, resumability, and automatic background execution remain separate proposals, not part of this remediation.

### Next experiment followups

These are observations to investigate, not newly implemented behavior or confirmed product defects.

- Distinguish agent-requested cleanup from evaluator-only cleanup obligations.
  Preserve original exercise prompts and original failures when evaluator cleanup later succeeds.
- Investigate the sole-default workspace removal experience and distinguish cleanup classes from deleting downloaded artifacts.
  A successful cleanup command that removes zero entries does not establish that a workspace is empty.
- Investigate agents ignoring sequential-PAT Guidance before classifying recovered concurrent authentication failures as expired credentials or a CLI defect.
- Keep schema-backed recommendations separate from claims about actual date coverage or values that were not queried.
  Explain catalog coverage using the actual requested scopes, not an inferred permission failure.
- Preserve Pulse's successful create, fork, and follower subgoals separately from its full-workflow timeout.
  Use the recorded timings to scope the next experiment instead of automatically rerunning the suite.
- Resolve the temporary-user role prerequisite: the requested access/following workflow may conflict with harness instructions against allocating a licensed role.
  Do not infer purchases or billing from a role assignment alone.
- Keep permission-denial hydration and nonempty Pulse dimension-filter forks listed as unexercised live branches.
  The successful workbook-only refresh and timeframe-only fork do not establish that coverage.

## Done

### Auth secret management

Interactive `tadx auth login` validates and stores a PAT in the native OS credential store.
A complete environment-variable pair remains the higher-precedence automation and temporary-override path.
TADX never falls back to plaintext secret storage.

### Workspace location handling

`workspace create` and `workspace clone` use `<home>/TADX/workspaces/<name>` when you omit `--path`.
An explicit `--path` remains authoritative.
`workspace register` still requires the existing root, and artifact moves still require source and destination workspace names.
Workspace names follow portable path-safe rules across Windows, macOS, and Linux.
Workspace create, clone, list, and status expose the registered root only in `--full` output.
