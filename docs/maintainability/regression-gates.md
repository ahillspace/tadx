# Regression gates for the proposed refactor

Status: approved target and gate sequence on 2026-10-03; implementation and effectiveness demonstrations remain pending.
Production migration remains blocked until the required gates demonstrate effectiveness.
This specification does not authorize live execution now.
The [architecture proposal](architecture-proposal.md) and [package map](architecture-decision-map.json) supply the approved G6 target.
No finite test suite guarantees zero regressions.
The objective is reproducible evidence against explicit contracts, with known limits visible.

## Existing baseline

TADX revision: `dd22c33bd1d123066f25ec09e2d612cd95717400`.
The [CI run for this revision](https://github.com/ahillspace/tadx/actions/runs/36669189664) reports nine successful jobs.
Those results are historical execution evidence, not a fresh local run or a test-effectiveness assessment.

The pinned [CI workflow](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/.github/workflows/ci.yml) includes:

- Linux and Windows formatting, vet, tests, race checks, and generated/module consistency.
- Bounded TOON fuzzing on Linux.
- Shell and PowerShell installer checks, including Windows PowerShell 5.1.
- Offline exploration-launcher checks and Linux website-packaging tests.
- Cross-compilation for Windows, Linux, and both listed macOS architectures.

The [native-policy workflow](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/.github/workflows/native-policy.yml) adds Linux, Windows, and macOS policy fixtures.
Full application tests on macOS are not shown by these workflows.
Cross-compilation is not native runtime verification.
The real keyring smoke test is opt-in; ordinary green CI does not establish its execution.

## Controlled loop

1. Approve one bounded migration contract and its expected effects.
2. Start from an accepted, recoverable revision and capture the applicable baseline.
3. Implement the structural change without silently correcting unrelated behavior.
4. Run G0-G7 and inspect their evidence.
5. Complete G8 review of behavior, test changes, and conformance to the common architecture.
6. After G0-G8 pass, complete G9 task-driven live verification on the exact candidate build.
7. Accept a checkpoint only when required gates pass, coverage gaps have recorded maintainer waivers, and findings are resolved or explicitly accepted.

A failed or missing required check blocks checkpoint acceptance.
Do not skip a check, loosen an assertion, or regenerate expected output merely to make a candidate pass.
An intentional behavior change needs its own approved contract and tests.
Known baseline defects remain visible; differential tests must not turn them into permanent requirements.
When baseline execution is unreliable, resolve or explicitly bound the problem before using it as a comparison oracle.

## Gate specification

| Gate | Required evidence | Demonstration that the gate works |
| --- | --- | --- |
| G0: source and scope | Baseline/candidate commit IDs, clean isolated source snapshots, toolchains, fixture identities, declared changed paths | An out-of-scope source change or mismatched fixture prevents acceptance |
| G1: external contracts | Exact flag/default/exit behavior; parsed JSON and TOON; stdout/stderr separation; saved results and persisted formats | Seed a wrong exit code, missing required field, or extra output field and observe a failure |
| G2: mutation safety | Exact target and values, preview without writes, policy refusal, fresh observations, bounded requests | Seed a wrong destination, skipped prewrite check, or preview write and observe a failure |
| G3: truthful outcomes | Native acknowledgement, requested-field verification, partial/unknown effects, identities, request IDs, recovery guidance | Return HTTP success with unchanged requested values; accept a write then fail transport; tests must reject false success |
| G4: local persistence | Installed/restored configuration, external-store state, locking, replacement, and compensation order | Fail external commit followed by failed restoration; assert surviving state and accurate recovery advice |
| G5: test effectiveness | Contract-to-test mapping, faithful fixtures, distinct coverage, and approved replacement before removal | Remove an important guard in an isolated candidate and prove the mapped test rejects it |
| G6: common architecture | Approved dependency and ownership rules applied across categories; explicit temporary migrations and approved exceptions | Add a forbidden dependency or new copy-only path in a disposable candidate and observe rejection |
| G7: platform and release | Existing required checks plus risk-based native behavior on supported platforms; reproducible build/packaging | Required platform jobs cannot be replaced by a successful cross-build or silently skipped fixture |
| G8: maintainer acceptance | Fresh reviewer traces a read, mutation, and local-state operation; identifies edits and tests without coaching | Document wrong turns and ownership ambiguity; unresolved material navigation problems block final sign-off |
| G9: Luna-medium live verification | Task-driven actual CLI use covers every affected executable action per checkpoint and the complete final command/action catalog | Native acknowledgements and affected-state evidence establish requested outcomes; missing required coverage blocks acceptance unless explicitly waived |

G0-G7 combine deterministic checks with evidence review.
G8 requires judgment and cannot be reduced to a passing lint score.
For G6, validate the approved consumer-port import direction, prohibited reverse dependencies, removed bridges, and every category's conformance, not just directory names.
LLM reviews and Jev evaluations can aid triage but are not the sole oracle for correctness or deletion decisions.
Seeded-defect demonstrations run only in isolated source and fixture environments, never against live services.
G9 uses separately authorized live fixtures without introducing seeded production defects.

## Final live verification

G9 starts only after G0-G8 pass for the exact candidate source and build.
Use Luna with medium reasoning for task-driven actual CLI execution, not code inspection or simulated command output.
Record the requested model, actual provider/model identifier, and actual reasoning effort; an unavailable configuration blocks execution rather than silently substituting another model.
Each checkpoint covers every affected executable command/action, including consumers affected through shared dependencies.
Final acceptance requires a complete executable command/action catalog sweep against the exact final candidate build, not accumulated results from different builds.
Map catalog identifiers to tasks, evidence, classifications, and any explicit waivers; omissions are coverage gaps.
Representative sampling does not satisfy required action coverage.
Exercise local, installation, and authentication actions in real isolated local environments; do not add unnecessary Tableau calls.
Use approved disposable fixtures, bounded resource scopes, and recorded authority for consequential actions.
Confirm the approved scope and fixture authority for each checkpoint before execution.
Tableau mutations require explicit consent for the selected server and exact site; a test plan does not authorize changing saved consent.
Keep PATs, session tokens, and other credentials out of prompts, transcripts, logs, reports, and fixtures.

Determine outcomes from task scope, actual commands and values, native acknowledgements, and affected-resource readback or local-state evidence.
An honest native refusal completes an intentional refusal task; final-answer formatting alone never determines failure.
Assign each case exactly one classification:

- `completed`: the requested outcome or intended refusal is verified in scope.
- `completed_with_workaround`: the outcome is verified, but a material avoidable workaround adds friction.
- `failed`: usable execution and sufficient evidence show that the requested outcome did not occur.
- `excluded`: running, setup-only, no-activity, harness, fixture, platform, provider, missing-evidence, or cleanup-only failures prevent grading.

Excluded cases never count as passes and leave required coverage unresolved.
Unavailable required cases block acceptance unless the maintainer records a specific waiver; a waiver is not a passed case.
Correctness failures block acceptance; review workaround friction rather than automatically treating it as a regression.
Track cleanup status separately from task outcome, retain affected-resource evidence, and block acceptance on unresolved harmful residual state.
Retain every attempt and retry, with sanitized task transcripts, CLI commands, request/response evidence, native acknowledgements, and verification references.
Record source/build hashes, platform, fixture provenance, model configuration, and task wall time, tokens, turns, and tool calls.
Use `null` for unavailable telemetry; do not infer missing values or combine older runs with the current candidate.
After any candidate change, rerun affected earlier gates before repeating live verification; previous evidence remains attached to its original build.
Keep live-task grading distinct from harness and provider errors, and preserve disagreement explanations when existing labels differ from native evidence.

## Pilot-derived fixtures required before risky migration

| Workflow | Essential scenarios |
| --- | --- |
| Project create/move | Parent or source changes after planning; late sibling collision; descendant cycle; top-level parent encoding; successful write with unavailable path enrichment |
| Workbook move | Exact source and destination arguments; preview suppression; changed owner/name/project at prewrite; late collision; already-at-destination no-op; returned state disagrees with request |
| Authentication/persistence | Failed authentication prevents storage; target changes before locked persistence; preview preserves both stores; selected-reference deletion; installed-but-not-durable save; external failure plus failed restore |
| Shared outcome handling | Transport failure after request acceptance; decoded identity alongside protocol mismatch; partial evidence survives every adapter and output boundary |
| Output projections | Compact/full JSON and TOON retain required fields and omit intentionally private or irrelevant fields; internal record expansion does not widen output |

These scenarios require a coverage search before adding tests.
Existing assertions can satisfy a scenario when their fixture and observation actually demonstrate it.
Do not duplicate the same assertion at every layer.
Use parser tests for argument mapping, workflow tests for decisions, boundary tests for protocol/state effects, and CLI integration for composition.

## Comparison rules

Run baseline and candidate with equivalent initial state in separate test-owned directories.
Do not let one run's mutations become the next run's fixtures.
Compare both results and effects: request transcript, accepted identities, persisted files, and dependency state.
Normalize only documented nondeterministic fields, such as fixture-generated timestamps or temporary roots.
Do not normalize away meaningful ordering, field omission, target IDs, unknown outcomes, or request sequence changes.
Preserve nil-versus-empty and omitted-versus-explicit values when they are part of the contract.
Keep stdout and stderr assertions separate.
Record skipped cases, fixture failures, and provider unavailability separately; none constitutes a passed gate.

## Approval ownership

The maintainer approves the target structure, exceptions, behavior changes, and required verification matrix.
On 2026-10-03, the maintainer approved the target and delegated orchestration and review to the implementation session.
G8 uses an independent in-session reviewer instead of a separate ChatGPT review, retaining its navigation and correctness acceptance criteria.
Automatic checks enforce reproducible contracts and produce inspectable evidence.
Reviewers verify that test changes do not conceal regression and that every removed responsibility has a surviving owner.
An implementation agent cannot approve its own change to acceptance criteria merely because a gate fails.
Gate policy changes require a recorded reason and maintainer approval.

## Evidence and limits

Each checkpoint records revision IDs, gate versions, executed commands, outcomes, platform, fixture identity, and verification limits.
Keep raw logs separate from the concise summary and exclude credentials and machine-specific private paths.
Retain recoverable checkpoints until the migration is accepted.
The repository-wide exit condition includes all categories, shared consumers, maintained docs, and the relevant regression matrix.
Gate implementation, seeded-defect demonstrations, baseline/candidate executions, and the G9 live catalog sweep remain pending.
