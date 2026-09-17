# TADX V1 build continuation

Updated: 2026-09-17.
Status: recommendation review and output-context audit complete; the user authorized implementation after review and commitment of the pre-existing work.

## Start here

The V1 direction is settled.
No new product decision emerged from this audit.
The next step is a targeted build, not another broad recommendation review.

The audit covers all 116 implemented actions across 22 action families.
It identifies eight additional output-context findings across 11 actions, alongside previously approved work.
These additions retain facts TADX already has; none requires an extra remote enrichment request.

Only S39 remains evidence-limited.
Its unresolved flow name-lookup cause does not block the other changes or exact-ID publication confirmation.

No CLI implementation occurred during this review.
No builds, tests, Tableau calls, mutation-setting changes, commits, or pushes occurred during the output-context audit.
Earlier bounded investigation checks are described separately below.

Overhead updates remain paused at the user's request.
Do not update the board, delivery metadata, or patches until the user resumes that workflow.
This document records the later chat approvals and audit results without changing Overhead.

Use these sections:

- [Build sequence](#build-sequence) for the recommended implementation order.
- [Additional useful-context findings](#additional-useful-context-findings) for the eight new locations.
- [Investigation outcomes](#investigation-outcomes) for resolved causes and the remaining evidence limit.
- [Shared decisions](#shared-decisions) and [Remaining-category resolutions](#remaining-category-resolutions) for the complete settled scope.
- [Detailed output audit](#detailed-output-audit) and [Action coverage](#action-coverage) for command-level evidence.
- [Source recommendation coverage](#source-recommendation-coverage) for all 89 final-category recommendations.
- [Workspace handoff](#workspace-handoff) for repository state and continuation limits.

## Authority and scope

Latest user decisions override historical recommendation wording and initial triage labels.
The reviewed records include shortlist.md, remaining-decisions.md, remaining-recommendation-map.json, the three bounded investigation reports, and v1-principles-plan.md.
Their operative decisions and audit results are consolidated here so implementation does not depend on private evidence bundles.

The final ten categories originally contained 89 recommendations.
The initial split was 78 recorded resolutions, two product choices, and nine recommendations consolidated into eight investigations.
An earlier flow investigation brought the investigation count to nine.
Both original product choices are approved.
Seven investigations closed under clarified requirements or established contracts; PULSE-06 received a separate chat approval.
Only S39 remains open for evidence, not a pending product choice.

The last observed Overhead identity is 2026-09-17_07-12-58-tadx-remaining-suite-recommendations, titled TADX - remaining suite recommendations.
Its delivery file records revision 6 and feedback cursor 14.
That snapshot predates the final PULSE-06 approval and intentionally remains stale.
Do not treat its open-decision counts as current authority.

The original audit authorized this handoff, not implementation.
The subsequent user request authorizes reviewing and committing the pre-existing work, then implementing the accepted recommendations and compact response augmentation.
It does not authorize a push, release, live Tableau mutation, or mutation-setting change.
Do not request approval again for settled product choices unless new evidence materially changes their scope.

The build concerns TADX CLI behavior and installed help/Guidance.
It does not include test-harness changes, a broad refactor, speculative command redesign, or unrelated security work.

## Build execution record

The shared build branch is `build/v1-suite-recommendations`, based on updated `origin/main` at `33de8d4cefe928215967c21114ecd9ea9b176653`.
The prerequisite review covers the pre-existing search identity, help navigation, column-description, and documentation changes.
The code-reviewer pass found one confirmed defect: acknowledged metadata updates reported read-back mismatches as submission failures.
The correction preserves the confirmed identity and protocol cause while reporting the verification phase.
A CLI HTTP-fixture regression fails before the correction and passes afterward.
No blocking findings remain in that prerequisite review.
Live column-clearing evidence remains limited to the recorded Cloud observation, not every Server version or license combination.

Prerequisite verification passes:

- Full `go test ./...` and `go test -race ./...` before the narrow review correction.
- Normal tests, race tests, and vet after the correction for column update, catalog CLI, and metadata asset adapters.
- `go vet ./...`, generated capability consistency, module consistency, and `git diff --check`.
- TOON fuzz smoke checks, installer shell tests, Windows PowerShell installer tests, and site tests.
- Native build and Windows amd64, macOS amd64/arm64, and Linux amd64 cross-builds.

PowerShell 7 is unavailable locally, so its installer check remains unrun.
Hosted CI requires a push and has not run.
An unrelated local log and empty root npm lockfile are preserved outside the checkout and excluded from the commit.
An ignored archived scratch file has a formatting issue; current tracked and pending Go files are formatted.
No live Tableau request or mutation-setting change occurs during this review.

Implementation coverage follows in this section as verified slices finish.
The settled decisions below remain the acceptance criteria, not a claim of completed implementation.

## Settled operating principles

All four overarching principles belong in V1.
Optimize total work to reach a correct outcome: commands, API requests, help reads, recovery, and response size together.

- Return the requested result and the smallest useful next context: exact identities, scope, usable paths, confirmed changed values, and operation state.
- Align equivalent selector, flag, batching, completeness, and output meanings across command families.
- Make Guidance conditional on known IDs, supplied configuration, and unresolved meaning.
- Distinguish no write, accepted pending work, confirmed success, incomplete verification, partial success, and unknown outcome.

Apply these rules through targeted changes:

- Use authoritative write confirmation when sufficient.
  Otherwise, perform supported bounded confirmation automatically within the action.
  Preserve confirmed effects if verification fails.
  Never blindly repeat an uncertain write.
- Treat metadata indexing delay as a visibility limitation, not failure of an acknowledged write.
- Keep complete resource help and sufficient verb help generated from the same definitions.
  A verb reference is a focused subset and adds nothing after the complete resource reference.
- Prefer TOON for direct reading and JSON for parsing or requested files.
  Use immediate saved-result expansion when the current result was saved.
- Preserve environment, exact selectors, workspace, filters, configuration, encoding, and cache mode in applicable follow-ups.
- Use one automatic publication lifecycle.
  Save acceptance before waiting; do not expose as-job, no-wait, or polling-configuration choices.
- Add --project-id to workbook and datasource lists and exact-name inspection.
  Return useful known project IDs and reuse existing project inspect.
- Treat an absent optional cache as informational when live prerequisites are healthy.
  Keep real cache problems and missing coverage visible.
  Never refresh automatically.
- Validate supplied Pulse fields and material semantics directly for a fully specified configuration.
  Reserve broad dimension discovery for design work or unresolved meaning.
- Accept TADX's own Pulse exports.
  Preserve meaningful settings and let Tableau enforce restrictions on faithfully representable supported requests.
  Retain necessary parsing and conversion checks.
- Support prepared .hyper inputs only for datasource append/replace in V1.
  Package unpacking and local extract editing remain deferred.
- Support only 7, 14, 30, 60, and 90 for Pulse Last Days.
  Last week and last month are separate date methods.
  Reject 45 and other unsupported counts without substitution.
- Return available static capability information with a specific configuration failure.
  Keep the nonzero exit, unavailable effective mutation policy, and blocked configuration-dependent operations.

## Build sequence

Keep each slice reviewable and preserve existing action boundaries.
Use shared infrastructure only when the underlying behavior is genuinely shared.
Do not introduce a universal result-schema rewrite.

For a non-trivial build, start with end-user reproductions and behavior tests.
A maintainer review after automated verification is recommended.
Do not launch an agent-based review unless requested.

### Establish truthful outcomes and recovery

Start with S01, S02, S07, S12, S28, S31, and PULSE-06.
Use existing error, output, saved-result, and command-hint infrastructure.

Cover fabricated label failure payloads, executed failures retaining preview advice, configuration causes, and checks blocked before domain work.
Preserve independently completed diagnostics and confirmed batch siblings.
Return static capability rows alongside configuration errors without inferring mutation permission.

Keep last-result saving separate from operation success.
A failed save produces a structured warning and marks the previous saved result potentially stale.
It must not turn a successful write into failure or encourage replay.

Acceptance checks:

- Compact, full, JSON, and saved output agree on identity, effects, phase, outcome, and coverage.
- Unknown writes receive read-only reconciliation.
- Failed verification preserves confirmed writes and exact recovery identities.
- Known no-write failures never suggest recovery for a possibly submitted mutation.
- Suggested commands preserve original scope and expose the fields they claim to verify.

### Return useful known facts

Apply S03, S09, S11, S27, S29, S30, and S33, including the eight additional findings below.
Start with local projections and adapter fields already available.
Keep optional remote enrichment separate from automatic mutation confirmation.

Cover artifact paths and source provenance, workspace lifecycle identity, confirmed changed settings, exact parent IDs, and known empty values.
Preserve actual backup locations and update destinations.
Show requested settings separately from observed saved state.

Implement domain confirmation where write responses remain insufficient.
Permission mutations and catalog metadata changes are concrete audit examples.
Use existing exact read paths; do not require routine caller verification after a completed action.

Acceptance checks:

- Ordinary continuation does not rediscover facts the action already obtained.
- Empty collections and root state remain distinct from omitted, unavailable, or failed observations.
- Provider-omitted booleans do not become false.
- Explicit complete administrative requests retain all fetched pages and avoid arbitrary truncation.
- Partial batches keep ordered input selectors, successful identities, paths, and individual failures.

### Align help, selectors, and installed Guidance

Apply S04-S06, S08, S10, S13-S24, S31, S38, and the relevant remaining-category resolutions.
Add approved project-ID read selectors without introducing another lookup command.

Keep one maintained help source.
Render complete resource references and focused action-level subsets from it.
Reject invalid help paths before returning unrelated successful help or blaming otherwise valid flags.

Condition Guidance on the requested operation and known facts.
Known IDs use exact operations.
Closed Pulse configurations get focused field and semantic validation.
Local capability, help, version, and Guidance tasks avoid unrelated remote setup.

Acceptance checks:

- Each known action can be understood from its verb help.
- Complete resource help includes those same definitions and constraints.
- Corrective commands retain scope and use real supported forms.
- Cursor continuation advances pages; larger filtered catalog expansion is not described as disjoint pagination.
- JSON changes encoding only, and full output changes detail rather than inventing rows.
- Explicitly requested follow-up reads remain available; redundant routine reads disappear.

### Apply bounded domain fixes

Implement CL-IDENTITY normalization, Pulse semantic comparison, Pulse Last Days validation, export round-tripping, and prepared-Hyper publication.
Apply cache compatibility and partial-lineage preservation under S25 and S26.
Keep the project literal-slash safety guard and expose its path-unavailable reason.

Reproduce each bug through the closest practical end-user command before changing behavior.
Use retained cases and source findings to construct narrow regressions.
Do not rerun the original suite wholesale.

Acceptance checks:

- Documented label type variants match canonical types; genuine ID or type mismatches still fail.
- Equivalent Pulse typed filters compare equal without weakening meaningful differences.
- Export round trips preserve variants and meaningful configuration.
- Preview, execution, batch validation, help, and Guidance agree on supported trailing-day values.
- Append/replace accept prepared compatible .hyper inputs and retain specific upstream rejection details.
- Refresh and read compatibility agree, and failed lineage capture preserves confirmed partial evidence.

### Complete automatic publication and job recovery

Implement S34-S37 through one automatic lifecycle.
Do not make unrelated output and help fixes wait for uncertain upstream behavior.

Persist accepted job identity, source, target, and environment/site before monitoring.
Each publication needs durable recovery independent of the single replaceable last result.
Immediate acknowledgement does not mean the command exits before completion.

More than one requested item is bulk and enters pooled monitoring immediately.
A single-item action uses first-minute fast checks, then joins the same pool.
Pool actual active jobs across processes, not separate timers that merely look coordinated.

Coordinate credential use during fast and pooled checks.
Release coordination during idle waits, prioritize foreground work, and establish a valid session for later checks.
Do not assume a session remains valid after another process signs in with the same PAT.

Continue monitoring when authoritative status still reports an active job beyond ten minutes.
Bound individual requests and recovery from repeated status-read failures.
Distinguish interrupted monitoring from remote failure.

Use concise acceptance, material state changes, and aggregate batch progress.
After ten minutes, suppress repetitive notices while retaining terminal and interruption output.
A sixty-second pooled round is a recorded recommendation, not a fixed approved cadence.
Choose exact early cadence and fairness through focused validation.

Provide the approved general job inspection, monitoring, and supported cancellation surface.
Exact naming remains an implementation design detail within that approved scope.
Stopping local monitoring must remain distinct from cancelling remote work.
Never claim every job type supports cancellation.

Verify flow asynchronous support and bulk job-list coverage before relying on either.
A bulk status API must expose the accepted publish job IDs and sufficient authoritative state.
Missing rows, partial pages, permission limits, or malformed results do not prove success or failure.
Use exact job reads where needed, with bounded recovery and independent per-job outcomes.

Earlier Migration SDK inspection found ordinary publish requests and per-job polling for a separate import path.
It did not establish reusable cross-process PAT coordination or bulk publish-job monitoring.
Do not import that SDK's cadence or mutation retry behavior as a TADX requirement.

Required validation includes:

- A two-item bulk boundary, a representative mixed-size project transfer, and one long-running single item.
- Immediate bulk registration and the one-minute single-item transition.
- Request volume, throughput, session reuse, fairness, and responsiveness of another same-PAT command.
- Durable acceptance before interruption, exact saved paths, independent failures, and read-only recovery.
- Continued active-job monitoring beyond ten minutes with low-noise progress.
- Distinct client transfer progress and remote job state without fabricated job IDs.

### Close the build

Use the repository's current contribution instructions and maintained test commands.
Run focused regressions, relevant integration tests, lint, and required CI.
Record commands and actual results; do not substitute this audit for implementation verification.

Compare representative before-and-after command and API counts alongside correctness.
No performance or task-turn reduction has been measured by this audit.

Leave S39 evidence-limited unless comparable request captures establish a cause.
Do not add sleeps, retries, cache purges, or republishing to make that symptom disappear.

## Additional useful-context findings

These eight findings extend the existing S33 audit scope.
They are concrete improvement locations, not new product approvals.
The source proves each omission; the next-step benefit is inferred from command contracts unless earlier behavior is cited.

User clarification on 2026-09-17: the useful-context additions are acceptable provided output stays compact and includes only genuinely needed information.
Add context only when it helps identify the result, understand the outcome, or perform the likely next step.
Keep supporting detail in --full or immediate saved-result expansion; do not promote fields merely because they are available.
Avoid duplicate table qualifiers, repeated shared scope, and full configuration or verification dumps in compact receipts.
Preserve explicitly requested facts and complete results; compactness must not hide required information.
This clarification changes neither implementation authorization nor the pause on Overhead updates.

| Audit ID | Location | Useful information to retain |
| --- | --- | --- |
| CTX-A06 | Group create | Provider-returned minimum site role and on-demand access setting, with omission marked unverified. |
| CTX-A07 | Label category, value, and attachment lists | Already-known total and a supported scope-preserving expansion command. |
| CTX-A08 | Content label list | Requested target type, exact target ID, and selected categories, including empty results. |
| CTX-R01 | Catalog audit | Parent table LUID for each physical-column finding, usable by column update. |
| CTX-R02 | Table list, inspect, and catalog search | Available schema or qualified-name context needed to distinguish same-named physical tables. |
| CTX-R03 | Live top-level search | Selected environment and site, once per response rather than repeated per row. |
| CTX-R04 | Version check failure | Installed version already known when the release lookup fails. |
| CTX-R05 | Local update | Resolved installation target and known version/release context, including failed installation. |

These changes need no additional remote call merely to return that information.
Some require carrying facts across existing adapter or runtime interfaces.
Do not claim provider fields exist when omitted, or label an update target as verified final binary state.

## Deliberate limits and rejected expansion

Full-only fields are not automatically defects.
Ordinary datasource, workbook, and flow owner detail can remain in full output with immediate saved-result expansion.
An ownership mutation still returns the confirmed changed owner under S33.

The env add receipt already saves full profile information.
Do not add every profile field to compact output or suggest another env get solely to recover that saved information.
Explicit env get has its separately approved compact completeness requirement.

Keep routine request IDs, fingerprints, inventory descriptions, search owner/timestamps, cache file paths, and full lineage graph nodes expanded unless the task needs them.

Project list does not receive canonical paths from its upstream list response.
Its exact resolver derives paths through a hierarchy index.
Keep path retrieval as an exact-ID read when needed; do not add unbounded hierarchy enrichment to list.

Direct subscription-ID unfollow does not resolve metric and follower context.
Do not invent those identities or introduce broad discovery to decorate its receipt.
The metric/follower selector form already retains that known context.
Any added enrichment needs a supported exact read and separate justification.

Permission create/delete are different: confirmation of the requested change belongs inside the action when the write response is insufficient.
Reuse the existing exact rule reader and preserve confirmed write effects if verification fails.

Update runtime currently exposes no structured per-target Guidance completion or rollback receipt.
Returning its known destination does not authorize fabricated target successes, backup paths, or final-state verification.

## Investigation outcomes

The following findings reuse retained evidence.
They do not claim fresh live Tableau reproductions during the output audit.

### CL-IDENTITY: Normalize native types before strict identity checks

Status: investigation closed; implementation pending.

The captured native response returns DATASOURCE, while provider and action guards compare it literally with datasource.
The label and target IDs match, so the type representation causes the false rejection.

Map documented native types to canonical CLI types before strict comparison, retain genuine mismatch refusal, and include requested and returned identity values in diagnostics.

Evidence: internal/tableau/metadataassets/labels.go:64-99 passes native contentType through before literal comparison.
The same guard appears in actions/contentlabel/{inspect,list,update,delete}/action.go.
The captured label and target IDs match; DATASOURCE versus datasource explains the false rejection.
Regression scope includes inspect, list, preview, update, and delete, plus genuine target and type mismatches.

### CP-PROJECT-PATH: Explain deliberately unavailable paths

Status: investigation closed; implementation pending.

A local guard deliberately omits mutation-result paths when a project name contains a literal slash.
Exact-ID inspection can return a display path, but path selection still requires a unique match.

Preserve successful state, exact project and parent IDs, and the slash safety guard; report a structured path-unavailable reason under S33.
Do not diagnose remote lag, invent escaping, redesign resolution, or retry the move.

Evidence: internal/app/content_mutations.go:24-48 deliberately stops path enrichment for a literal slash in the returned name.
Existing exact-ID and ambiguous-display-path tests appear in internal/resources/project/adapter_test.go and slash_paths_http_test.go.
Preserve success and exact parent identity.
Do not remove the guard merely because exact-ID inspection can display a path.

### INV-PLATFORM-01: Report doctor execution dependencies

Status: investigation closed; implementation pending.

Doctor invokes all six checker interfaces, but configuration-dependent checks can fail before their domain work; logging remains independent.

Apply S12 to identify blocked prerequisites, preserve the exact cause, and distinguish actual failures from checks that could not run.

Evidence: actions/doctor/run/action.go:61-68 invokes six interfaces.
Configuration-dependent work lives in internal/app/doctor.go:38-110; logging remains independent at :113-116.
Calling an interface does not prove its connectivity, credential, cache, or workspace check actually ran.
The retained invalid-profile fixture reports five dependent failures and one logging pass.
The fix reports blocked dependencies instead.

### INV-PLATFORM-02: Reject the unknown config help path

Status: investigation closed; implementation pending.

The exact config --help invocation succeeds with root help, but config without help fails as unknown.
Registration and dispatch show no config child command or compatibility mapping.

Apply S04 and S10 with an invalid-path error and the existing env, global --config, and doctor routes.

The retained local invocation config --help exits 0 with root help.
The same path without help exits 1 as unknown.
Evidence: internal/cli/root.go:217-247 registers --config, not a config child; internal/app/app.go:164-174 performs command lookup.
Keep the exact argv as a regression.
Use env help, global --config, and doctor recovery rather than inventing a config command.

### INV-PLATFORM-03: Expand full capability list contracts

Status: investigation closed; implementation pending.

The ordered local comparison confirms that list --full and its saved result contain summaries, while get returns contract details.

Apply S18 and S07 to reuse the existing contract representation for returned full-list rows and saved output, preserving filters and compact output.

The retained sequence compares capability list --full, immediate last, then capability get.
List and last contain summaries; get contains contracts.
Evidence: actions/capability/list/projection.go:14-24 and internal/app/app.go:654-716.
Reuse the existing get representation for returned full-list rows and saved output without per-row remote requests.

### PULSE-03: Compare equivalent typed filter representations

Status: investigation closed; implementation pending.

The exact C01 plan and read-back differ only by the provider's additional values array, which the current byte comparator retains.
Removing that property makes the captured JSON equal, proving the mechanism for this case.

Compare equivalent typed members and their derived representation without hiding genuine differences in fields, types, operators, null policy, literal text, inherited filters, comparisons, or periods.
Keep confirmed creation separate from failed verification under S28 and S33.

The retained C01 requested/read-back specifications differ only by an additional read-back values array.
Removing that property makes this captured pair equal.
Evidence: internal/tableau/pulse/metric_reconcile.go:91-126 currently retains the extra representation during byte comparison.
The correct fix compares equivalent typed semantics; it must not blindly discard a conflicting values representation.
Preserve field, operator, type, null policy, literal whitespace/comma text, inheritance, comparison, period, and genuine member differences.
The exact C01 mechanism does not prove every retained C02-C04 case shares that cause.
Earlier focused reconciliation, action-fork, and app-fork tests passed but lacked these equivalence regressions.
Those passes are historical checks, not validation of an implemented fix.

### PULSE-04: Use supported trailing-day windows

Status: investigation closed; implementation pending.

The user clarifies that Pulse Last Days supports only 7, 14, 30, 60, and 90 days; there is no 45-day option.

Align trailing-day fork validation, preview, help, and installed Guidance with that finite set.
Reject unsupported values before submission, list supported choices, and never substitute another duration.
Keep calendar periods such as last week and last month separate from trailing-day counts.

The user explicitly settled Last Days to 7, 14, 30, 60, and 90.
The earlier 1-through-3650 mapping and advertised 45-day path are not the V1 contract.
Evidence locations: actions/pulse/metric/fork/validation.go, actions/pulse/metric/fork/action.go:210-224, and internal/cli/pulse/command.go:361-387.
Test missing --days separately from supplied zero, 45, and other unsupported values.
Test all five accepted counts, previews, execution, batches, and separate calendar-period methods.
Do not generalize this finite set into arbitrary rejection of unrelated portable configuration fields.

### PULSE-06: Return partial capability output after configuration failure

Status: approved in chat; implementation pending.

A fixture with a valid selected profile and workspace plus one unused invalid profile reproduces the original failure.
Both unfiltered and mutation=false capability lists fail because mutation-policy resolution validates the entire configuration before filtering.
Workspace listing requires the same validated configuration; doctor reports five dependent failures while independent logging passes.

Preserve the exact invalid-profile cause and report blocked checks under S12 and S15.
Approved V1 resolution: return static capability rows as partial diagnostic data alongside the configuration error, retaining a nonzero exit and unavailable effective mutation policy.
Keep workspace and environment operations dependent on validated configuration, and never treat unavailable policy as permission to mutate.

The retained fixture has a valid selected profile and workspace plus one unused invalid profile.
Both ordinary and mutation=false capability lists fail before filtering because policy resolution validates all configuration.
Evidence: internal/config/config.go:145-166, internal/cli/capability/command.go:71-87, and internal/app/mutation_setting.go:12-44.
Static registry rows exist independently in internal/app/app.go:654-671.
The user approved returning those rows alongside the specific configuration error, keeping a failure exit and unavailable effective policy.
Workspace, environment, and policy-dependent operations remain blocked.
Do not silently ignore malformed profiles or enable mutations.

### S39: Resolver request comparison evidence

Status: evidence-limited; no speculative fix.

Existing evidence lacks comparable raw name-only, exact-ID, and name-plus-project request/response captures.

Continue acknowledged-LUID confirmation; no resolver, retry, sleep, cache purge, or republish change is supported.

The retained flow case shows accepted creation, immediate name/project lookup failure, successful exact-ID inspection, and a later matching filtered list.
Evidence: internal/resources/flow/adapter.go:86-151 uses name-only listing plus local project-path filtering.
The ordinary list route sends name and project filters through internal/app/content_remote.go:286-287 and internal/tableau/flow/client.go:438-456.
The evidence has no comparable raw HTTP captures, so timing and filter behavior remain unresolved.
A bounded authorized reproduction must capture name-only list, exact-ID GET, and name-plus-project list with filters, pages, timestamps, and request identities.
Continue acknowledged-LUID confirmation in the build.
Do not require a new live publication merely to finish unrelated work.

## Shared decisions

S01-S38 describe settled scope, not completed implementation.
S39 remains an evidence task.
The build sequence above supplies the latest publication and automatic-confirmation interpretation.

### S01: Failure responses

Omit fabricated empty entities and success statuses when inspection fails; preserve requested identity and selector type, exact absence evidence, and read-specific recovery advice.
Preserve specific configuration-validation and cache-compatibility causes across entry points, naming the actual profile/field/environment and appropriate recovery.
Retain available sanitized provider details.

### S02: Partial mutation outcomes

Report confirmed or upstream-reported effects separately from overall failure, including user unlicensing after refused deletion; include an exact inspection route.

### S03: Coverage reporting

Expose existing per-scope requested/completed cache coverage; distinguish inventory counts from permission-rule counts without new remote reads.
Report independently retained observations separately from shared inventory generation, preserving their own timestamps and coverage even when the generation is uninitialized.

### S04: Help path validation

Reject unknown command paths with a usage error and supported route before rendering help or blaming flags valid on the supported route; keep valid parent help and avoid adding guessed aliases.

### S05: Shared help notes and navigation

Support concise explanatory notes and related-command pointers in the shared help system, with one maintained source reused by complete and relevant verb help.
Add the permission, membership, label, and noninteractive auth setup routes specified below; preserve the canonical command hierarchy.
Include direct completion-output capture and shell-appropriate verification examples in completion help or its existing focused reference.

### S06: Exact identity selectors

Accept inspect --username as an exact-login alias for --name, consistent with adjacent user operations; preserve selector exclusivity and reject conflicts.

### S07: Output selection and reuse

Prefer default TOON for direct agent reading; reserve JSON for programmatic parsing, requested JSON files, or a concrete reliability need.
Document direct native JSON capture, the last-result envelope, and its replaceable single-result lifetime, including later recorded usage failures.
Replace ambiguous detail hints with immediate last --full expansion only when the current result was saved; preserve chosen encoding.
If saving fails, emit a structured warning, retain recovery identities, and identify last as potentially stale without misreporting a successful operation as failed or suggesting mutation replay.

### S08: Workflow guidance

Add concise membership and batched user-verification examples using existing exact selectors and returned IDs; retain necessary verification.

### S09: Requested facts and administrative completeness

Include explicitly requested core facts in compact output; distinguish observed empty collections from unrequested fields, and retain --full for supporting detail.
Emit observed-empty tags as [] and preserve returned empty descriptions, while keeping provider-omitted values unavailable.
Administrative requests for every member or item must retrieve all pages without arbitrary result caps or silent truncation.

### S10: Shared help rendering and routing

Keep the owning command/resource help complete; render verb help as a focused subset from the same definitions, including applicable shared flags and constraints.
Update root help and installed guidance, including Pulse and auth, to direct known actions straight to verb help and avoid rereading a subset after the complete reference.
Skip category navigation when the resource is already known.
Preserve tadx status and tadx -h navigation as distinct entry points.

### S11: Requested versus observed settings

Expose provider-omitted settings as not reported or unverified, distinct from false; distinguish requested settings from observed saved state in create/inspect guidance.
Explain the documented on-demand access licensing condition for this group field without asserting an unverified site license.
Preserve boolean semantics rather than inferring false from omission.

### S12: Diagnostic dependencies

Distinguish failed prerequisites from checks blocked by them; preserve the actual configuration cause, continue independent checks, and avoid credential/network/cache repair advice without evidence of those failures.
Doctor exposes non-secret PAT reference names, presence, and source in full output; distinguish local credential resolution from attempted live authentication/connectivity, without treating a configured credential-store reference as verified credentials.

### S13: Authentication provenance

Return the credential source actually used by auth check, including failed attempts after source selection, using existing context without another authentication or credential read.
Explain existing environment-over-store precedence in shared auth help.

### S14: Local readiness and next steps

Identify auth status as local readiness without live authentication; show missing configured variable names and setup guidance when incomplete, and make a live check an optional next step only when ready.
Preserve local-only behavior and successful diagnostic exit semantics for incomplete readiness.

### S15: Dependency-aware agent guidance

Resolve already-required task conditions before dependent discovery or execution; reuse known policy/help, parallelize independent work, and retain independently requested reads and authorized previews.
Make overview/skill setup hints conditional on the operation: remote operations need their environment, managed artifacts need their workspace, and local Guidance installation/removal needs neither.
Preserve configuration validation and policy reporting; add no new mandatory preflight or policy gate.

### S16: Interactive login under environment overrides

Preserve environment-over-store credential precedence.
When the effective source is environment credentials, stop auth login before prompting for, validating, or storing a new PAT; identify the override and explain that the configured PAT variables must be cleared before stored-credential login.
Never clear variables or change precedence automatically.
Successful login retains the identity validated using its supplied PAT.

### S17: Explicit inventory counts

Separate returned/matched capability and out-of-scope counts while preserving existing combined totals and per-record execution eligibility; section ownership does not prove executability.

### S18: Detailed capability lists

Make capability list --full reuse the contract representation from get for returned CLI and external records, including effects, selectors, constraints, availability, and ownership.
Preserve filters and row selection; this expands detail only.

### S19: Required capability constraints

State the required exactly-one publish mode in datasource.publish metadata: --create, --overwrite, --append, or --replace; retain execution policy and read-only preview distinctions, and point to the owning help for complete syntax.

### S20: Installed-version spelling

Accept root --version as an alias of installed-only tadx version, using the same implementation and output; no release lookup or update.

### S21: Continuation meaning and context

Document the existing capability cursor and return an explicit next_command that advances to the next page, preserving filters, --json, and --full.
Replace larger-prefix continuation suggestions.
Include the short cue: "When more results are needed, use next_command." Preserve compact output and avoid exposing private cursor state globally as an incidental change.

### S22: Local diagnostic routing

Permit capability-only, help-only, and installed-version tasks to skip the tadx session overview and unrelated Tableau/workspace setup.
Retain the overview before Tableau/workspace work.
Describe capability inventory as local implementation metadata, not proof of site access or entitlement; a Pulse filter alone does not require Pulse authoring guidance.

### S23: Verification evidence boundaries

Define lineage complete as the selected visible bounded capture, not every real-world dependency or a current metadata index.
Require retrieved relationship evidence between exact identities before claiming a dependency; otherwise describe it as unconfirmed.
Preserve absent-edge and scope limitations.
Distinguish remote metadata inspection from native artifact inspection: file type and publication acknowledgement do not establish native configuration correctness, and --full expands metadata rather than retrieving payloads.

### S24: Filtered catalog expansion

Return a command that preserves environment, name, and parent selectors when requesting more catalog matches.
Use the existing mutually exclusive --all/--limit forms correctly; a filtered --all retrieves all matching rows, not a disjoint next page.
Retain REST/Metadata identity distinctions and avoid broadening to site-wide discovery.

### S25: Preserve partial lineage captures

Finalize and retain collected nodes and edges when a relationship query fails; carry the partial capture and typed provider/relation failure through the collector, adapter, action, and artifact.
Mark incomplete and explain the missing evidence.
Distinguish failure before any root was collected from an observed empty relationship set; never merge independent observer evidence into the native capture.

### S26: Cache refresh/read compatibility

Make refresh and reads validate the same schema markers; verify the refreshed cache satisfies reader compatibility before reporting success.
Explicitly reject unsupported inconsistent markers with a specific recovery diagnosis, without circular identical-refresh advice.
Preserve coherent supported upgrades and verify them separately; do not infer a general upgrade regression or silently rebuild on read.

### S27: Recovery paths in compact receipts

Include each actual home-relative backup path in compact agent install/uninstall receipts when a package was preserved.
Preview describes planned preservation without inventing a generated backup name.
Explain existing install backups, and remove expansion advice when the path is already present.
Preserve current backup and compatibility-only force behavior.

### S28: Phase-aware mutation recovery

Generate status and next steps from actual lifecycle evidence, distinguishing not submitted, submitted/waiting, write confirmed, write confirmed with verification incomplete, and outcome unknown.
Offer execution only after a successful preview, never as stale help inside executed failures.
Preserve exact target/environment and returned resource/job/request identities.
After an unknown write, reconcile read-only; one immediate not-found does not establish failure or authorize replay.
Preserve existing collision protection and bounded recovery.

### S29: Actionable artifact and batch receipts

Keep returned artifact/canonical payload paths, resolved workspace context, source environment/site/LUID, and original batch selectors in compact receipts when needed for follow-up or recovery.
Preserve workspace-relative artifact paths, ordered partial successes, and individual failures without relying on ordinal labels or prose-only identities.
Do not introduce automatic batch replay.

### S30: Exact ambiguous-artifact candidates

Preserve ambiguous-selector refusal, but return bounded exact workspace-relative candidate paths with their source environment/site/LUID.
Identify truncation and the existing full workspace-status route when necessary.
Never choose a candidate automatically or add fuzzy matching.

### S31: Context-preserving recovery commands

Preserve project filters, exact target selectors, environment, and cache mode through errors and generated follow-ups.
Use exact reads for already-known IDs.
Cache-only recovery stays local, distinguishes incomplete/uninitialized coverage, and presents live refresh only as an explicit alternative.
Retain completeness gates and generation-replacement disclosure.

### S32: Prepared Hyper append/replace

Support already-prepared .hyper files for existing datasource append/replace modes, with mode-specific input preparation and preview validation.
Keep .tds/.tdsx create/overwrite behavior; do not unpack packages or add local extract editing for these modes in V1.
Explain the specific Tableau incompatibility, including schema requirements and multi-table layouts unsupported by the append endpoint.
Preserve exact targeting and unknown-write reconciliation; do not generalize append restrictions to other modes without evidence.

### S33: Useful action results and automatic confirmation

Audit every command for the requested result plus the smallest useful context for consuming it or continuing the task.
TADX handles mechanical resolution, validation, execution, supported waiting, and automatic confirmation within the action.
For every mutation that can be verified, use sufficient authoritative write confirmation or automatically perform the necessary bounded read-back.
Return confirmed changed values, resulting identities, usable paths, and material lifecycle state without routine caller verification or rediscovery.
Distinguish acknowledgement, requested values, observed state, and verification limits.
Generate contextual follow-ups only where further work remains.

### S34: Automatic publication lifecycle and durable receipt

Provide one automatic publication lifecycle: supported submission, immediate acknowledgement when accepted, monitoring, and confirmation.
Remove --as-job as a caller execution-mode choice; do not introduce --no-wait, no-polling, or polling-configuration switches.
Persist accepted job identity and source/target context before waiting, then record the final result.
Preserve honest pending receipts across interruption and distinguish upload, submission, and status-request limits.
Never resubmit a publish to recover its status.

### S35: Cooperative pooled job monitoring

Automatically register active pollable jobs with shared coordination across TADX processes.
Actions requesting more than one item are bulk and join pooled monitoring immediately.
Single-item actions use fast checks for their first minute, then join the pool; exact early cadence remains to be chosen.
Pool actual active jobs across commands, grouping checks by credential/site, preserving per-job outcomes, and giving foreground work priority.
Release credential coordination during idle waits and establish valid sessions for later checks.
Continue monitoring confirmed active jobs beyond ten minutes while suppressing repetitive progress output.
Test both item-count paths and same-PAT concurrency before finalizing the mechanism.

### S36: Reusable job recovery and controls

Provide a general job-status/monitoring surface for supported Tableau job types, using exact job identity and environment/site.
Include supported cancellation with a clear distinction between stopping local monitoring and cancelling remote work.
Return available status, last-check time, target context, and usable recovery/control instructions.
Preserve independent per-job outcomes for multi-job checks; missing or unavailable observations never imply success or job failure.

### S37: Bulk and single-item monitoring validation

Test the accepted item-count rule: more than one requested item enters pooled monitoring immediately; one item transitions from fast checks to the pool after one minute.
Cover a two-item boundary case, a representative mixed-size project transfer, and a single long-running operation.
Measure request volume, throughput, authentication/session reuse, aggregate progress, exact saved paths, per-item failure/interruption outcomes, and responsiveness of another same-PAT command.
Keep client-side transfer progress separate from remote job polling; do not invent unsupported job IDs or new command inputs.

### S38: Constraint-specific validation diagnostics

Report the actual invalid choice with a concrete supported correction: conflicting flags, duplicate batch rows and normalized selectors, or incompatible global/per-item naming.
Preserve existing validation gates and supported per-row names; do not auto-correct, deduplicate, select defaults, or suggest unrelated environment repairs.
Reuse command/batch constraint metadata where available.

### S39: Post-publish name visibility investigation

Reproduce the flow name-lookup miss immediately after creation while capturing name-only list, exact-ID GET, and name-plus-project list requests/responses.
Compare filters, pagination, timestamps, and request identities before attributing cause or changing resolver/retry behavior.
Use the acknowledged LUID for automatic confirmation; do not invent cache invalidation, sleeps, or write retries.

### Shared help details

Reuse these accepted related-command notes where relevant:

| Owning help | Related route or constraint |
| --- | --- |
| Content workbook, datasource, flow, and project | tadx admin permission -h with the corresponding --kind. |
| Admin group | tadx admin group-member -h for individual membership changes. |
| Admin group membership replacement | --set-members replaces all direct members; --member-id requires it; omitted member IDs clear the membership. |
| Catalog label | tadx admin label-value -h for shared definitions; tadx admin label-category -h for categories. |
| Admin label-value and label-category | tadx catalog label -h for attachments to content. |
| Auth setup and non-TTY login errors | Existing env add/update PAT-variable-reference options; flags take variable names, not secrets. |
| Shell completion | Direct completion-output capture and shell-appropriate verification examples. |

Do not duplicate destination syntax across these pointers.

Use exact returned group IDs for membership changes.
Repeated user --id selectors provide the existing same-action batch verification route.
Historic before/after examples do not override S33: the mutation itself performs necessary confirmation.
Keep separately requested membership reads available without requiring routine duplicate verification.

The shared output cue remains:
"Prefer default TOON when reading results directly.
Use --json when another program will parse the output or a JSON file is requested."

When the required verb is known, go directly to its help.
After reading the complete owning reference, skip verb help because it adds no information.

### Approved product choices

CP-PROJECT-ID-READ adds --project-id to workbook and datasource list and exact-name inspect.
Preserve selector exclusivity and exact identity semantics.
Return useful project IDs already available in existing results.
Reuse project inspect --id for project lookup; do not add a lookup command.

DOCTOR-OPTIONAL-CACHE makes an absent optional cache informational when live prerequisites are healthy.
Actual cache failure, incompatibility, incomplete coverage, and explicitly required cache state remain visible.
Do not add automatic refresh.

PULSE-06 separately approves partial static capability data beside a configuration failure.
Its complete outcome and policy constraints appear in the investigation section.

## Remaining-category resolutions

These 78 entries preserve the individually recorded resolutions from the final category review.
They overlap shared decisions intentionally, so no source recommendation loses its disposition.
The two product choices and investigation outcomes above complete that review.

Historical aliases map as follows:

- CP-AUTH-SCOPE maps to DEC-CONTENT-005.
- PULSE-01 maps to DEC-PW-001 and the later export-compatibility approval.
- PULSE-02 maps to DEC-PW-006 and DEC-PW-013.
- PULSE-05 maps to DEC-PW-019.

### DEC-CONTENT-001: Omit fabricated label failure payloads

Use the shared S01 failure contract: preserve the real error, phase, outcome, and known selectors, and omit uninitialized label items and plans from the response and last result.
Source: content-label recommendations.md: CL-R02; shared decision S01.

### DEC-CONTENT-002: Route unsupported label help paths clearly

Apply S04 and S10: reject the unknown path with a usage error and point to catalog label help.
Keep catalog label canonical, do not add an execution alias, and keep valid verb help as a focused subset from the same source.
Source: content-label recommendations.md: CL-R03; shared decision S04.

### DEC-CONTENT-003: Show label guard selectors as a pair

Update complete label help to show one optional target ID and type group, state that the two flags must be supplied together, and retain the valid ID-only form.
Source: content-label recommendations.md: CL-R05; shared decision S10.

### DEC-CONTENT-004: Keep label identity fields stable and expose editable message

Use consistent core identity fields in compact and full output, preserving required compatibility.
Include the editable message in compact single-label inspection and keep supplementary fields in full output.
Source: content-label recommendations.md: CL-R06; shared decision S33.

### DEC-CONTENT-005: Keep read task scope explicit in Guidance

Update Guidance so necessary mechanical steps for the requested outcome remain authorized, but spare input values cannot turn a READ into a rename or write.
Do not require explicit verb-by-verb permission.
Source: content-project recommendations.md: R01; shared decision CP-AUTH-SCOPE.

### DEC-CONTENT-006: Make project and workbook ambiguity errors truthful

Use S01 error reporting to name the ambiguous resource level, label candidate IDs with their resource type, preserve refusal to choose, and give an exact-ID recovery route.
Source: content-project recommendations.md: R02; shared decision S01.

### DEC-CONTENT-007: Show project deletion cascade risk in help

Add the existing cascade warning beside delete syntax through shared help notes.
Explain that no child projects does not prove no content, and keep preview non destructive without adding mandatory confirmation.
Source: content-project recommendations.md: R03; shared decision S05.

### DEC-CONTENT-008: Reuse saved project results for detail and capture

Apply S07: explain that last returns the saved envelope and result, that the slot is replaceable, and that full changes detail rather than row limits.
Use shell redirection on the initial JSON command when a file is requested.
Source: content-project recommendations.md: R05; shared decision S07.

### DEC-CONTENT-009: Represent known empty project and collection values

Apply S09: emit known empty strings, explicit root nulls, and empty arrays for successful zero-row collections, while preserving unavailable and failed states as distinct values.
Source: content-project recommendations.md: R06; shared decision S09.

### DEC-CONTENT-010: Make resource help complete and routing direct

Apply S10: mark owning resource help as complete, route known resources directly, and render verb help as a focused subset from the same definitions.
Do not repeat the full reference in verb help.
Source: content-project recommendations.md: R07; shared decision S10.

### DEC-CONTENT-011: Give project flag errors targeted corrections

Apply S38 and S04: name the submitted constraint, show the supported parent ID, project path, move parent, or limit alternative, and preserve not attempted status without silently retrying or changing scope.
Source: content-project recommendations.md: R08; shared decision S38.

### DEC-CONTENT-012: Return destination identity after workbook publish

Apply S33 and S34: retain the accepted job identity and use authoritative publish data plus bounded automatic confirmation for the destination workbook ID; do not require routine caller rediscovery.
If authoritative data remains unavailable, return a valid exact name, project, and environment lookup instead of an empty command.
Source: content-workbook recommendations.md: R01; shared decision S33.

### DEC-CONTENT-013: Remove execution advice from failed workbook batch items

Apply S28: generate recovery text from the actual phase and outcome, retain exact per-item selectors, suppress preview execution advice on failures, and preserve successful sibling receipts.
Source: content-workbook recommendations.md: R02; shared decision S28.

### DEC-CONTENT-014: Stop destructive collision workarounds

Apply S28: name the requested destination and unresolved candidate, allow one bounded diagnostic read, then stop proposing equivalent publish retries or deletion and recreation.
Keep collision refusal fail closed.
Source: content-workbook recommendations.md: R03; shared decision S28.

### DEC-CONTENT-015: Validate owner ID shape before workbook previews

Apply S38: validate the owner ID grammar locally, identify the invalid field and value, and omit execute guidance on failure.
Keep syntax checks separate from live owner existence and permission checks.
Source: content-workbook recommendations.md: R04; shared decision S38.

### DEC-CONTENT-016: Diagnose workspace manifest misuse as configuration error

Apply S12: preserve the config path and decoder cause, explain that global application config differs from workspace selection, and direct the caller to use the registered workspace without editing its manifest.
Source: content-workbook recommendations.md: R05; shared decision S12.

### DEC-CONTENT-017: Distinguish managed and native workbook sources

Apply S29 and S33: document the existing source forms, show source kind and resolved path in the plan, and never infer that a native file is the requested managed artifact or that workspace selection changes file semantics.
Source: content-workbook recommendations.md: R06; shared decision S29.

### DEC-CONTENT-018: Explain workbook compact and full result reuse

Apply S07: document compact omissions, full detail, last full expansion, and the replaceable saved result.
Keep full opt in and avoid adding another export store or discovery step.
Source: content-workbook recommendations.md: R07; shared decision S07.

### DEC-CONTENT-019: Keep pull paths in compact workbook receipts

Apply S29: include the managed directory and canonical payload paths in compact receipts, clearly label which path is usable by managed artifact publish, and apply the same projection to successful members of partial batches.
Source: content-workbook recommendations.md: R08; shared decision S29.

### DEC-CONTENT-020: Scope workbook flags and corrections by verb

Apply S04 and S38: attach flags to the verbs that accept them, explain remote versus local scope and project-name leaf semantics, and preserve the submitted environment and target in corrections.
Source: content-workbook recommendations.md: R09; shared decision S04.

### DEC-CONTENT-021: Reject nonexistent help paths and show local inventory route

Apply S04 and S10: validate the full help path, return a usage error with the nearest supported route, and link workspace artifact help to workspace status for local inventory.
Add no aliases.
Source: content-workbook recommendations.md: R10; shared decision S04.

### DEC-CONTENT-022: Verify supplied destination IDs directly

Apply S31: when a destination ID is supplied and verification is warranted, inspect that ID in the stated environment.
Use bounded name lookup only for name-only requests, and reserve broad inventory for discovery tasks.
Source: content-workbook recommendations.md: R11; shared decision S31.

### DEC-PLATFORM-01: Preserve configuration error provenance

Carry the precise configuration cause through compact errors and overview results, distinguish the requested target from the failing profile, and describe --config as the CLI settings file.
Source: environment recommendations.md: environment R02; shared decision S01/S12.

### DEC-PLATFORM-02: Route known environment help directly

Keep one complete environment reference.
Go directly to verb help when the verb is known, or env -h for the complete reference.
Verb help is a focused subset from the same definitions and adds no information after that complete reference.
Source: environment recommendations.md: environment R05; shared decision S10.

### DEC-PLATFORM-03: Make last-result lifetime explicit

Document latest-result replacement, show that compact operations can save full receipts, and direct users to capture requested durable JSON output at the time it is produced.
Source: environment recommendations.md: environment R06; shared decision S07.

### DEC-PLATFORM-04: Give exact usage recovery

Keep the usage errors and add exact non-mutating corrections for env default and workspace status, preserving output, configuration, and selector context.
Source: environment recommendations.md: environment R07; shared decision S04.

### DEC-PLATFORM-05: Return resolved environment facts

Include the requested non-secret profile facts and resolved cache concurrency in env get, keep list bounded, retain --full for expanded detail, and never save inherited defaults.
Source: environment recommendations.md: environment R01; shared decision S09/S33.

### DEC-PLATFORM-06: Explain effective versus saved mutation policy

Add source-setting and persistence acknowledgement fields without changing existing keys or precedence.
Acknowledge persistence only after the save succeeds, and never clear an override automatically.
Source: environment recommendations.md: environment R08; shared decision S33.

### DEC-PLATFORM-23: Make compact environment inspection complete

Include API version, auth type, default workspace, resolved cache concurrency, and PAT variable-reference names in compact env get; keep env list bounded, retain --full for expanded detail, and never expose credential values.
Source: environment recommendations.md: environment R04; shared decision S09/S33.

### DEC-PLATFORM-07: Classify installer retrieval failures truthfully

Use not found only when the response supports it, otherwise report the requested version and asset with a bounded sanitized transport cause while preserving verification refusal.
Source: installer%29 recommendations.md: installer%29 1; shared decision S01.

### DEC-PLATFORM-08: Support root local version spelling

Route root --version through the existing installed-version handler and document that it does not check releases, update, authenticate, or contact Tableau.
Source: installer%29 recommendations.md: installer%29 2; shared decision S20.

### DEC-PLATFORM-09: Return local write destinations

Return resolved workspace, installed-skill, and canonical workbook paths in compact receipts with source context, while keeping fingerprints and larger provenance in full output.
Source: installer%29 recommendations.md: installer%29 3; shared decision S29.

### DEC-PLATFORM-10: State workspace prerequisites in environment flows

Document the registration prerequisite, include the missing workspace and selected settings path in recovery, preserve custom configuration, and keep reads non-creating.
Source: installer%29 recommendations.md: installer%29 4; shared decision S01/S15.

### DEC-PLATFORM-11: Scope workspace flags to applicable actions

Show workspace requirements beside managed pull and publish forms.
Standalone native-file publication does not require a managed workspace.
Keep remote inspect workspace-free and preserve its selector and environment in error corrections.
Source: installer%29 recommendations.md: installer%29 5; shared decision S10/S15.

### DEC-PLATFORM-12: Document independent installer switches

Document the install-directory precedence, explain each switch independently, and show --no-modify-path --no-completion for installation without profile edits.
Source: installer%29 recommendations.md: installer%29 6; shared decision S05/S10.

### DEC-PLATFORM-13: Make installer write failures actionable

Report the resolved target and failed stage, state when replacement did not occur, and offer an explicit alternative install directory without elevation, permission changes, or silent relocation.
Source: installer%29 recommendations.md: installer%29 7; shared decision S01.

### DEC-PLATFORM-14: Add persistence acknowledgement to mutation receipts

Name the effective source setting and return an additive persisted acknowledgement after saving, while preserving enabled, source, scope, and saved_enabled semantics.
Source: mutation recommendations.md: mutation MUT-TADX-01; shared decision S33.

### DEC-PLATFORM-15: Distinguish CLI settings from workspace manifests

Describe --config as a CLI settings file, recognize the workspace-manifest shape for a specific recovery message, and never load or silently discard the explicit override.
Source: mutation recommendations.md: mutation MUT-TADX-02; shared decision S01/S10.

### DEC-PLATFORM-16: Label capability section counts

Add named section counts while retaining combined pagination totals, and define the combined unit in capability help without calling every row executable.
Source: mutation recommendations.md: mutation MUT-TADX-03; shared decision S17.

### DEC-PLATFORM-17: Bound capability inventory to local evidence

State the local eligibility and remote-verification boundary in the shared response and help contract, without adding a speculative per-row schema field or a connection check.
Source: other recommendations.md: other 1; shared decision S22.

### DEC-PLATFORM-18: Make Guidance discovery scope-first

Read supplied scope and identities first, go directly to known resource help, reuse loaded help, and enumerate other resources only when the task needs them.
Source: other recommendations.md: other 2; shared decision S10/S15.

### DEC-PLATFORM-19: Use cursor-advancing capability continuation

Return next_command with the next cursor, preserving filters, --json, and --full.
Do not use a larger prefix based only on the known total, and do not expose private cursor state globally.
Source: other recommendations.md: other 3; shared decision S21.

### DEC-PLATFORM-20: Scope workspace requirements in content help

Scope workspace requirements to the forms that actually use managed artifacts.
Preserve workspace-free remote inspect and list, and standalone native-file publication without a managed workspace.
Source: other recommendations.md: other 4; shared decision S10/S15.

### DEC-PLATFORM-21: Show required one-of choices inline

Render the existing mutually exclusive choices as required groups in action syntax and retain only explanatory constraints that add information.
Do not change parser behavior.
Source: other recommendations.md: other 5; shared decision S10.

### DEC-PLATFORM-22: Advertise label-value create through update

Describe label-value update as create or update, place its category and description conditions beside the syntax, and add a concise create-via-update pointer.
Source: other recommendations.md: other 6; shared decision S05/S10.

### DEC-PW-001: Accept TADX Pulse exports as publication inputs

Allow TADX's own Pulse exports to be published again, handling known bookkeeping fields while preserving saved variants and meaningful settings.
When TADX can faithfully submit the configuration through a supported Tableau API, let Tableau enforce its restrictions rather than adding speculative local rejection rules.
If TADX cannot interpret or preserve a setting, report that exact limitation instead of silently dropping it.
Explicitly approved and refined by the user on 2026-09-17.
Source: pulse-definition recommendations.md: pulse-definition R01; shared decision PULSE-01.

### DEC-PW-002: Make incomplete preview guidance safe

Generate next actions from phase and review state; offer execution only after complete review and otherwise direct the caller to obtain the missing evidence.
Source: pulse-definition recommendations.md: pulse-definition R02; shared decision S28.

### DEC-PW-003: Reject repeated scalar schema queries

Reject repeated scalar queries with a usage correction and direct known-field checks to repeatable --field-id.
Source: pulse-definition recommendations.md: pulse-definition R03; shared decision S38.

### DEC-PW-004: Aggregate portable validation diagnostics

Report local parsing or conversion problems together with exact payload paths and locations.
Do not label faithfully representable configuration unsupported merely because local business-rule validation is incomplete; let Tableau evaluate its restrictions.
Never silently remove meaningful configuration.
Source: pulse-definition recommendations.md: pulse-definition R04; shared decision S38.

### DEC-PW-005: Report no-write outcomes for local Pulse rejection

Mark known local rejection paths as validation or setup with outcome not_attempted, direct correction and preview, and preserve separate unknown-write recovery.
Source: pulse-definition recommendations.md: pulse-definition R05; shared decision S28.

### DEC-PW-006: Use targeted checks for closed Pulse configurations

For a closed supplied configuration, validate the requested fields and meaningful dependencies directly; reserve broad inventory discovery for open-ended design or unresolved semantics.
Source: pulse-definition recommendations.md: pulse-definition R06; shared decision PULSE-02.

### DEC-PW-007: Mark resource help as the complete Pulse reference

Add a resource scope cue, route known resources directly, and keep verb help as the same focused subset.
Source: pulse-definition recommendations.md: pulse-definition R07; shared decision S10.

### DEC-PW-008: Make managed Pulse artifact roots usable

Show workspace-relative roots in compact inventory, describe --artifact as a managed directory, and report the verified root when a payload file is supplied.
Source: pulse-definition recommendations.md: pulse-definition R08; shared decision S29/S30.

### DEC-PW-009: Preserve source-site provenance on portable artifacts

Carry source environment, server, site, and LUID context with portable identities and make 404 recovery check that provenance before server configuration.
Source: pulse-definition recommendations.md: pulse-definition R09; shared decision S29.

### DEC-PW-010: Add saved configuration to compact definition inspection

Include a bounded saved-configuration summary and make exact full inspection or immediate last-result expansion the primary detail route.
Source: pulse-definition recommendations.md: pulse-definition R10; shared decision S33/S07.

### DEC-PW-011: Normalize successful empty Pulse collections

Serialize known successful empty collections as [] while retaining pagination and coverage metadata and reserving null for unknown or inapplicable values.
Source: pulse-definition recommendations.md: pulse-definition R11; shared decision S09.

### DEC-PW-012: Align Pulse fork recovery with execution state

Generate recovery from mode, phase, and outcome; use exact full inspection for confirmed identities and read-only reconciliation for unknown writes, never an automatic replay.
Source: pulse-metric recommendations.md: pulse-metric R02; shared decision S28.

### DEC-PW-013: Use targeted checks for closed metric slicer sets

Apply the shared closed-configuration route: validate supplied slicers and dependencies directly, and keep full discovery for open-ended design.
Source: pulse-metric recommendations.md: pulse-metric R04; shared decision PULSE-02.

### DEC-PW-014: Put the Pulse no-drilldown rule at the resource boundary

Go directly to known metric verbs when appropriate.
Keep the complete resource reference available, and render verb help as a focused subset that adds no information after the complete reference.
Source: pulse-metric recommendations.md: pulse-metric R05; shared decision S10.

### DEC-PW-015: Document verified get-or-create completion

Document that verified created:false reuse completes the requested action; inspect again only when verification is missing, failed, or independently requested.
Source: pulse-metric recommendations.md: pulse-metric R06; shared decision S33.

### DEC-PW-016: Make full metric retrieval and last-result retention explicit

Document full list and inspect recipes, clarify --json, --all, and --full, and direct callers to capture last immediately instead of rerunning.
Source: pulse-metric recommendations.md: pulse-metric R07; shared decision S07.

### DEC-PW-017: Distinguish missing and unsupported trailing-day counts

Track whether --days was supplied separately from its value.
Report an omitted value differently from an unsupported value, including zero or 45.
List 7, 14, 30, 60, and 90 as the supported Last Days choices, applying the same constraint to preview and batch validation.
Keep calendar periods and other date methods separate.
The user's 2026-09-17 clarification resolves the supported set under PULSE-04.
Source: pulse-metric recommendations.md: pulse-metric R08; shared decision S38.

### DEC-PW-018: Explain disallowed Pulse filters precisely

Name the offending field, source metric, and controlling definition, and return the allowed set or exact inspection route without widening the definition.
Source: pulse-metric recommendations.md: pulse-metric R09; shared decision S38.

### DEC-PW-019: Validate metric and user LUID shape locally

Reject malformed documented LUID selectors locally with the supplied token and required shape, while retaining remote checks for well-formed IDs.
Do not apply UUID rules to datasource field IDs or captions.
Source: pulse-metric recommendations.md: pulse-metric R10; shared decision PULSE-05.

### DEC-PW-020: Use resource-specific recovery for missing metrics

Report the exact metric as missing or inaccessible in the selected environment, retain provider status and identity, and reserve configuration repair for configuration evidence.
Source: pulse-metric recommendations.md: pulse-metric R11; shared decision S01.

### DEC-PW-021: Explain default metric deletion restrictions

Preflight and report the non-default-only rule with metric and definition IDs in both preview and execution, without suggesting shared-definition deletion.
Source: pulse-metric recommendations.md: pulse-metric R12; shared decision S38.

### DEC-PW-022: Give command-specific selector corrections

Return the valid selector and scope correction for each command and preserve the existing command hierarchy without adding ignored aliases.
Source: pulse-metric recommendations.md: pulse-metric R13; shared decision S38.

### DEC-PW-023: Correct all versus limit recovery guidance

Tell callers to remove --limit for all rows or remove --all for a bounded result.
Preserve the supplied definition and environment, and do not introduce an unadvertised cursor.
Source: pulse-metric recommendations.md: pulse-metric R14; shared decision S38.

### DEC-PW-024: Route workspace configuration failures accurately

Preserve the selected configuration path and redacted cause, identify manifest versus CLI-config mismatches, and never rewrite or auto-register in recovery.
Source: workspace recommendations.md: workspace R01; shared decision S01/S12.

### DEC-PW-025: Expose doctor causes and blocked checks

Include the selected cause and path, mark blocked checks with their prerequisite, and separate blocked checks from tested failures.
Source: workspace recommendations.md: workspace R02; shared decision S12.

### DEC-PW-026: Correct workspace artifact usage routing

List the actual artifact child verbs, keep the group non-mutating, and add the existing workspace status inventory route.
Source: workspace recommendations.md: workspace R03; shared decision S04/S10.

### DEC-PW-027: Make workspace identity inventory one command

Document workspace list --full as the identity route and preserve its config, filters, limits, and pagination in expansion hints.
Source: workspace recommendations.md: workspace R04; shared decision S29.

### DEC-PW-028: Preserve workspace identity in lifecycle receipts

Return workspace name, ID, and root in ordinary receipts and include a correctly quoted path-only registration command after unregister without re-registering.
Source: workspace recommendations.md: workspace R05; shared decision S29/S33.

### DEC-PW-029: Explain invalid artifacts in full status

Add a per-artifact reason code, exact sidecar or canonical path, and redacted parse or validation cause while preserving valid rows.
Source: workspace recommendations.md: workspace R06; shared decision S01.

### DEC-PW-030: Show bounded cleanup scope in compact receipts

Include paths_to_remove in previews and removed paths in receipts, with help mapping temporary, cache, logs, and all to the established roots.
Source: workspace recommendations.md: workspace R07; shared decision S33.

### DEC-PW-031: Mark the workspace owning-help boundary

Add concise scope cues to workspace and workspace artifact help and route guidance to those owning references without duplicating syntax.
Source: workspace recommendations.md: workspace R08; shared decision S10.

### DEC-PW-032: Make workspace collision refusals actionable

Return exact conflicting workspace, path, and identity values with read-only reconciliation guidance; never suggest overwrite, deletion, or identity rewriting as automatic repair.
Source: workspace recommendations.md: workspace R09; shared decision S01/S31.

### DEC-PW-033: Keep empty workspace status structurally complete

Serialize artifacts.artifacts as [] and artifacts.total as 0 for complete empty scans while preserving incomplete-scan coverage semantics.
Source: workspace recommendations.md: workspace R10; shared decision S09.

## Detailed output audit

The audit examines current working-tree action outputs, compact/full projections, adapters, and selected shared rendering.
It covers every action.go package and matches every implemented CLI entry in the generated capability reference.
It does not audit undocumented provider behavior or certify every action as bug-free.

The 44 notes below include eight additional findings, 35 instances of settled work, and one optional project-path enrichment boundary.
Some notes cover multiple actions.
A separate subscription-unfollow enrichment limit appears above and in the coverage table.

"No added enrichment call" means the proposed output uses already available facts.
It does not waive the approved S33 confirmation requirement when a mutation needs an exact read-back.
Future implementation tests must verify the chosen authoritative confirmation per action.

### CTX-A01: Remove fabricated label failure results

Type: Correctness bug.

Finding: Failure paths return initialized success-shaped label payloads: zero Item, empty Items, or an incomplete Plan, instead of omitting unobserved data.

Recommendation: Apply DEC-CONTENT-001 and S01 at every label failure boundary: omit uninitialized item, items, and plan fields while retaining phase, outcome, and exact selectors.

Next use: When a label read or validation fails, preserve the actual failure and known selectors without treating an empty payload as observed state or an executable plan.

Existing scope: DEC-CONTENT-001, S01.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/contentlabel/delete/action.go:29-36,59-73
- actions/contentlabel/inspect/action.go:22-27,66-81
- actions/contentlabel/list/action.go:25-32,79-97
- actions/contentlabel/update/action.go:35-53,85-99

### CTX-A02: Keep label identities stable and show the message

Type: Output convention improvement.

Finding: Compact label inspection emits id and target_id, while full output emits luid and target_luid; compact output also omits the editable message.

Recommendation: Apply DEC-CONTENT-004: keep stable identity keys compatible across projections and include message in compact single-label inspection, with supplementary fields remaining full-only.

Next use: Capture an inspect result, then update a label or parse its exact identity without requiring an avoidable --full read.

Existing scope: DEC-CONTENT-004, S33.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/contentlabel/inspect/action.go:22-50
- internal/value/metadata.go:100-112

### CTX-A03: Keep explicitly requested group members complete

Type: Completeness defect.

Finding: The adapter retrieves all group members through its bounded page loop, but FullOutput retains only the first 100 and reports members_omitted.

Recommendation: Honor S09 complete administrative results when --members is explicitly requested: return the already fetched member collection for that request.
Keep default compact output bounded and honest, and do not add a new cap, flag, or speculative continuation form.
Explicit --members must expose the requested members; bounded default means an inspection that did not request membership.

Next use: Use a full group inspection to verify membership or review the next membership update without losing already fetched members.

Existing scope: S09.

Calls: no added enrichment call for these retained facts.

Evidence:

- internal/resources/admin/adapter.go:247-264
- actions/admin/group/inspect/action.go:61-81

### CTX-A04: Keep explicitly requested permission rules complete

Type: Completeness defect.

Finding: The permission reader supplies the complete rule collection, but FullOutput truncates rules to 200 and reports rules_omitted; compact rule_count still counts the pre-truncation collection.

Recommendation: Honor S03 and S09 complete administrative results for the requested permission inspection: return the already fetched rule collection.
Keep default compact output bounded and honest, and do not add a new cap, flag, or speculative continuation form.

Next use: Inspect a resource with many permission rules and continue or verify a rule beyond the first 200 without a misleading compact count.

Existing scope: S03, S09.

Calls: no added enrichment call for these retained facts.

Evidence:

- internal/app/admin.go:626-634
- actions/admin/permission/inspect/action.go:48-67

### CTX-A05: Separate requested and observed user settings

Type: Output and confirmation improvement.

Finding: The create plan records email, identity_pool_name, language, and locale, but the result User type and app adapter retain only LUID, name, site role, auth setting, and IdP configuration ID.

Recommendation: Apply S11 and S33: carry provider-returned optional settings into the confirmed result, or label them not reported or unverified; never echo plan values as observations.

Next use: After user creation, distinguish requested optional settings from provider-observed values before an exact inspect or update.

Existing scope: S11, S33.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/admin/user/create/action.go:16-24,26-43,60-70
- internal/app/admin.go:475-487
- internal/tableau/admin/types.go:21-25,34-37

### CTX-A06: Retain returned group settings

Type: Additional output improvement.

Finding: The create plan records minimum_site_role and external_user_enabled, but the result and app adapter retain only status, group_luid, and request ID even though the provider Group has those returned fields.

Recommendation: Add provider-returned group settings to the confirmed result, or mark them unverified when omitted; do not infer observed settings from the create plan.

Next use: After group creation, know which requested group settings were confirmed before an exact inspect or follow-up update.

Scope: applies the settled useful-context principle; no new product approval is recorded.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/admin/group/create/action.go:17-22,27-40,57-71
- internal/app/admin.go:535-547
- internal/tableau/admin/types.go:49-54

### CTX-A07: Expose known label totals and expansion

Type: Additional output improvement.

Finding: Each action fetches the full collection, slices it to the requested limit, and exposes returned plus more_available, but omits the known total and any continuation or next command.

Recommendation: Expose the already-known total and a bounded selector-preserving next command or limit hint; do not change the compact default into an unbounded return-all operation.
A larger limit expands a prefix rather than advancing a cursor; describe that accurately and preserve current supported bounds.

Next use: When more_available is true, retrieve the omitted definitions or asset labels with an exact, bounded follow-up.

Scope: applies the settled useful-context principle; no new product approval is recorded.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/admin/labelcategory/list/action.go:22-29,61-94
- actions/admin/labelvalue/list/action.go:22-29,67-100
- actions/contentlabel/list/action.go:25-32,83-116

### CTX-A08: Retain label-list target context

Type: Additional output improvement.

Finding: The list input requires type, target_id, and optional categories, but both compact and full output omit that requested target context, including when the observed item collection is empty.

Recommendation: Return a small requested-target object containing type, target_id, and selected categories when present; keep observed items and returned counts separate from the request.

Next use: Use a saved empty-list result to identify the exact asset before deciding whether to inspect or add a label.

Scope: applies the settled useful-context principle; no new product approval is recorded.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/contentlabel/list/action.go:12-17,25-32,47-61,79-116

### CTX-A09: Confirm permission mutations when needed

Type: Confirmation gap under S33.

Finding: The mutation writer returns status, resource LUID, and request ID only; each action then copies the requested plan target into result.rule without a post-write exact permission read.

Recommendation: If the write response is insufficient, perform one bounded post-write read through the existing GetPermissionRule and permissionRuleMode path.
Preserve the confirmed write outcome, return observed rule or absence separately from the requested target, and report verification limits if that read fails.

Next use: Confirm the exact rule after a create or deletion and distinguish the requested rule from observed server state or verification limits.

Existing scope: S33.

Calls: use bounded exact post-write confirmation only when the write response is insufficient; this is already approved under S33.

Evidence:

- actions/admin/permission/create/action.go:141-157
- actions/admin/permission/delete/action.go:141-157
- internal/app/admin_permission_mutations.go:46-48,117-119

### CTX-C01: Reuse saved datasource ownership detail

Type: Existing Guidance improvement; no new compact field.

Finding: Datasource models carry owner_luid, but compact list and inspect projections omit it; full output retains it.
This is the same owner-detail omission covered for workbook and flow reads, not evidence for asymmetric datasource enrichment.

Recommendation: Keep datasource owner_luid full-only under the existing S07/S33 guidance and saved-result route, consistently with workbook and flow.
Ownership mutations must return the confirmed owner and exact target under S33; do not add a compact owner-ID exception from this audit.

Next use: A caller with an ownership question can use --full or last --full for the same read, while an ownership mutation returns the confirmed owner under S33.

Existing scope: S07, S33.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/datasource/list/types.go:42-63 defines owner_luid; :93-101 and :127-133 omit it from compact output.
- actions/datasource/inspect/types.go:25-48 defines owner_luid; :62-70 and :95-98 omit it from compact output.

### CTX-C02: Return pull paths and source context

Type: Receipt improvement.

Finding: Pull inputs include source environment and site, and writers receive them, but pull Output has no such fields; compact artifacts hide path and canonical_path while full output retains paths.

Recommendation: Apply S29: return source environment/site plus managed artifact and canonical payload paths in compact pull receipts, including successful batch members.

Next use: A caller can carry the source context and both local paths directly into cross-site publishing, recovery, or provenance reporting.

Existing scope: S29.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/datasource/pull/types.go:12-16 has Environment/Site; :84-95 Output drops them; :97-104 marks compact Path json:"-"; :116-129 exposes paths only in FullArtifact.
- actions/workbook/pull/types.go:16-24 has Environment/Site; :174-187 Output drops them; :197-207 marks compact Path json:"-"; :220-238 exposes paths only in FullArtifact.
- actions/flow/pull/types.go:11-15 has Environment/Site; :68-78 Output drops them; :80-86 marks compact Path json:"-"; :96-107 exposes paths only in FullArtifact.
- actions/datasource/pull/action.go:73-76, actions/flow/pull/action.go:69, and actions/workbook/pull/action.go:149-151 pass source context to writers, while result construction at datasource/pull/action.go:95, flow/pull/action.go:82, and workbook/pull/action.go:228 omits it.

### CTX-C03: Identify publication source kind and path

Type: Receipt and Guidance improvement.

Finding: Publish plans resolve artifact paths, but compact plans discard the path; workbook and datasource plans also lack an explicit managed_artifact versus native_file source kind.

Recommendation: Apply S29/S33: include source kind and the usable resolved source path in ordinary publish plans.
Keep managed artifact paths workspace-relative.
Do not invent managed workspace requirements for standalone native files.
Keep fingerprints and substeps expanded.

Next use: A caller can identify the exact local source and avoid treating a convenient native file as the requested managed artifact before a publish or retry.

Existing scope: S29, S33.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/datasource/publish/types.go:54-70 has ArtifactPath and source fields; :75-85 sets CompactPlan ArtifactPath json:"-"; :130-152 omits source kind.
- actions/workbook/publish/types.go:84-99 has ArtifactPath and source fields; :142-155 sets CompactPlan ArtifactPath json:"-"; :215-230 omits the path from the encoded compact plan.
- actions/flow/publish/types.go:37-52 has ArtifactPath; :88-109 CompactPlan has no artifact path field.

### CTX-C04: Reuse saved workbook and flow ownership detail

Type: Existing Guidance improvement; no new compact field.

Finding: Workbook and flow models include owner_luid, but compact inspect and list identity records omit it; full output has it and the saved-result route can expand the same read.
DEC-CONTENT018 settles workbook owner detail as full/last-full reuse, and S07/S33 provides the same bounded route for flow.

Recommendation: Keep workbook and flow owner_luid full-only under the settled S07/S33 help and saved-result route.
Do not promote it here: the datasource candidate is justified by its distinct reviewed compact next-use, and shortlist.md:274-278 rejects a blanket owner-ID expansion.

Next use: A caller with an ownership question can use --full or last --full without searching admin users or rerunning a broader inventory; this audit does not claim different owner behavior across resources.

Existing scope: S07, S33.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/workbook/inspect/types.go:23-35 and actions/workbook/list/types.go:26-38 define owner_luid; compact projections at inspect/types.go:48-55 and list/types.go:63-70 omit it.
- actions/flow/inspect/types.go:33-49 and actions/flow/list/types.go:26-39 define owner_luid; compact projections at inspect/types.go:58-65 and list/types.go:64-71 omit it.

### CTX-C05: Avoid empty destination hints after workbook publication

Type: Invalid recovery hint and lifecycle gap.

Finding: A successful asynchronous workbook result can contain tableau_job_id without workbook_luid, while Execute still generates an inspect-by-ID hint from result.WorkbookLUID.

Recommendation: Apply S33/S34: save accepted job identity, monitor automatically, and use authoritative destination data or bounded automatic confirmation.
Never generate inspect --id with an empty ID.
If destination identity remains unresolved, preserve the job and exact target context with honest verification limits and a valid name/project/environment recovery route.

Next use: The caller receives a valid destination identity or a truthful bounded name/project/environment lookup instead of an empty inspect command.

Existing scope: S33, DEC-CONTENT-012.

Calls: automatic monitoring and necessary confirmation belong to the approved publication lifecycle, not optional receipt enrichment.

Evidence:

- actions/workbook/publish/types.go:124-131 permits JobID with an empty WorkbookLUID; :159-165 compactly retains JobID but not request diagnostics.
- actions/workbook/publish/action.go:225-231 returns the result and always builds inspect --id from result.WorkbookLUID.

### CTX-C06: Represent known empty content and root state

Type: Empty-state correctness and convention defect.

Finding: Known empty descriptions, root parent state, zero-row resource collections, and empty workspace inventories can serialize as omitted fields or null instead of explicit empty values.

Recommendation: Apply S09: emit known-empty strings, explicit root null or top-level state, empty arrays, and zero totals only for complete observed scans.
Preserve unavailable and incomplete states.

Next use: Machine consumers can distinguish observed empty state from unavailable data without defensive resource-specific parsing.

Existing scope: S09.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/project/inspect/types.go:22-38 and actions/project/list/types.go:34-49 use omitempty on description, parent_luid, and top_level; full projections at inspect/types.go:74-81 and list/types.go:99-107 retain the model but cannot restore omitted empty values.
- actions/workbook/list/types.go:85-93 and actions/datasource/list/types.go:116-124 encode collection fields that can be nil; actions/workspace/status/action.go:45-59 uses omitempty for Inventory.Total and Items.

### CTX-C07: Explain the intentional missing project path

Type: Intentional behavior; structured explanation improvement.

Finding: Project move intentionally leaves the mutation-result path empty when the returned project name contains a literal slash; composition adds a human-readable path-unavailable warning and exact-ID inspect hint, but the result has no structured availability reason.

Recommendation: Keep the literal-slash guard, successful status, exact IDs, and exact-ID inspect recovery.
Apply S33 by exposing the existing path-unavailable reason as structured data; do not add retries, sleeps, or a remote-delay interpretation.

Next use: The caller can distinguish a successful move from unconfirmed hierarchy enrichment and run the exact-ID read only when path confirmation remains necessary.

Existing scope: S33.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/project/move/types.go:38-46 returns Project in Result, and :59-66 copies its empty path into compact output without a structured availability reason.
- internal/app/content_mutations.go:24-48 deliberately returns no path for names containing '/', while :121-135 appends the existing human-readable warning and exact-ID inspect hint.

### CTX-C08: Show workspace cleanup scope

Type: Receipt improvement.

Finding: Workspace clean preview and execution receipts contain bounded cleanup counts, but compact output omits the already available paths_to_remove or removed paths.

Recommendation: Apply S33 and workspace R07: include the bounded selected roots in compact previews and removed roots in compact execution receipts.
Keep canonical artifacts preserved.

Next use: A caller can review or report the exact selected disposable roots without requesting full output or inspecting the local tree.

Existing scope: S33.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/workspace/clean/action.go:24-33 stores Removed; :41-49 compactOutput has no paths field; :52-65 omits it from compact execution output.
- actions/workspace/clean/action.go:68-81 exposes paths_to_remove only when full is requested.

### CTX-C09: Return workspace artifact paths and empty inventory

Type: Receipt and empty-state improvement.

Finding: Compact workspace status reduces each inventory item to kind, LUID, name, and state, omitting exact artifact paths; complete empty scans can also omit artifacts and total.

Recommendation: Apply S29/S33 for bounded workspace-relative paths and S09 for complete empty inventories.
Keep fingerprints and larger provenance full-only.

Next use: A caller can select an exact managed artifact path for publishing or deletion and can parse a completed empty inventory consistently.

Existing scope: S09, S29, S33.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/workspace/status/action.go:34-43 defines Artifact.Path and provenance fields; :90-99 compact status replaces each item with only Kind, LUID, Name, and State.
- actions/workspace/status/action.go:46-59 marks Inventory.Items and Total omitempty, so empty complete scans can lose the array and total.

### CTX-C10: Preserve workspace lifecycle identity and root

Type: Receipt improvement.

Finding: Create, clone, and unregister results have an authoritative workspace root that compact output omits.
Register's manager record also has the resolved root, but the app mapper drops it; successful register output therefore has ID and name without a root or path, while preview retains the input path.
Unregister also uses a <path> placeholder in its recovery hint.

Recommendation: Apply S29: include resolved workspace name, stable ID, and root in ordinary lifecycle receipts, and use the resolved root in unregister recovery guidance.

Next use: A caller can report or recover the exact local workspace after registration changes without a status or list lookup.

Existing scope: S29.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/workspace/create/action.go:19-27 and actions/workspace/clone/action.go:21-29 define ID and Root; compact projections at create/action.go:36-52 and clone/action.go:39-55 retain only Name.
- actions/workspace/register/action.go:16-25 exposes top-level Path only for the preview shape, and :57-65 preserves it on preview; successful :86-89 construction omits Path, while :36-52 compactly retains only Name.
- internal/workspace/manager.go:122-161 returns Record.Root for registration, but internal/app/workspace.go:140-148 maps only Name, ID, ManifestVersion, and Registered into the action workspace, discarding item.Root.
- actions/workspace/unregister/action.go:15-25 has ID and Root; :28-35 compactly retains only Name, and :74-75 emits register --path <path>.

### CTX-C11: Keep project-list path enrichment separate

Type: Optional enrichment boundary; no new implementation scope.

Finding: The Tableau project list model and XML response contain no canonical path.
The resource adapter's direct ListProjects path is normalized with an empty path, and the app mapper then projects only the list fields; canonical paths are derived only by the adapter's all-pages path index for exact resolution.

Recommendation: Keep this as a separate exact-ID read boundary.
Treat the path as unavailable from project list because the upstream list does not supply it and direct list normalization does not derive it; do not claim the action DTO alone proves an extra API call is required.

Next use: Use exact project-ID inspection when the next action requires the canonical path; do not infer it from a list row or add a path without upstream evidence.

Scope: applies the settled useful-context principle; no new product approval is recorded.

Calls: canonical path can require separate exact-ID resolution; no automatic list enrichment is proposed.

Evidence:

- internal/tableau/project/types.go:5-21 defines Project without Path; internal/tableau/project/client.go:355-393 parses and normalizes project list XML without a path attribute.
- internal/resources/project/adapter.go:94-113 maps direct list items with normalize(item, ""), while :134-180 and :334-443 derive paths only for exact resolution from an all-pages hierarchy index.
- internal/app/content_remote.go:505-516 maps the resource page into actions/project/list, whose Project at actions/project/list/types.go:34-49 also has no Path; exact inspect maps the derived Path at internal/app/content_remote.go:518-522.

### CTX-P01: Report the authentication source used

Type: Provenance improvement.

Finding: auth.check returns authenticated environment, server, site, site LUID, and user LUID, but its Authenticator seam and output have no credential-source field.

Recommendation: Capture the selected credential source at authentication time and expose it in compact, full, and provider-error results without reading credentials again.

Next use: Identify which effective credential authenticated the returned Tableau identity without correlating a separate status or profile read.

Existing scope: S13.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/auth/check/action.go:47-50
- actions/auth/check/types.go:17-34

### CTX-P02: Retain validated login identity

Type: Receipt improvement.

Finding: auth.login stores the validated PAT and retains site and user LUIDs in Output, but CompactOutput omits both identity fields.

Recommendation: Include the site and user identities validated by the supplied PAT in the default successful login receipt.
Do not perform a second sign-in through default credential resolution.
Apply S16's environment-override refusal before prompting or storing.

Next use: Separate the interactive PAT validation receipt from a later effective-source check when environment credentials override the stored PAT.

Existing scope: S16.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/auth/login/action.go:83-88
- actions/auth/login/types.go:61-92

### CTX-P03: Distinguish local authentication readiness

Type: Diagnostic and Guidance improvement.

Finding: auth.status full output carries local profile references, while compact output omits those references and both projections always suggest auth.check without an auth-verification marker.

Recommendation: Add an explicit not-checked verification state, expose available non-secret reference names in incomplete compact output, and condition help on local readiness.

Next use: Tell an offline caller which local references are incomplete, and tell a ready caller that Tableau verification remains a separate optional read.

Existing scope: S14.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/auth/status/action.go:25-49
- actions/auth/status/types.go:19-72

### CTX-P04: Return resolved environment facts

Type: Requested-fact completeness improvement.

Finding: env get compact output contains only alias, default, server, and site; full profile projections carry stored cache concurrency, which remains zero when the inherited default is unset. env list uses the same unresolved profile value.

Recommendation: Return API version, auth type, default workspace, resolved cache concurrency, and PAT variable-reference names in compact env get.
Keep env list bounded and expanded profiles truthful.
Never expose credential values or persist inherited defaults.

Next use: Inspect one local profile, including PAT references and effective cache concurrency, without a second full read or reconstruction of the inherited default.

Existing scope: DEC-PLATFORM-05, DEC-PLATFORM-23, S09, S33.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/env/profile/get/types.go:20-47
- actions/env/profile/list/types.go:25-59
- internal/app/environment.go:108-121
- internal/config/config.go:307-334

### CTX-P05: Summarize saved Pulse definition configuration

Type: Requested-fact completeness improvement.

Finding: Pulse definition inspect compact output contains only definition identity and datasource identity; full output contains the saved measure, time field, dimensions, granularity, and configuration.

Recommendation: Add a bounded saved-configuration summary to compact inspect and make full inspect or immediate last-result expansion the detail route.

Next use: Answer a saved-configuration inspection request without pulling a local artifact or issuing another remote read solely for detail.

Existing scope: DEC-PW-010, S07, S33.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/pulse/definition/inspect/types.go:41-80
- actions/pulse/definition/inspect/action.go:48-51

### CTX-P06: Reuse expanded Pulse metric detail

Type: Existing Guidance improvement; bounded detail remains expanded.

Finding: Pulse metric inspect compact output retains identity and default status, and metric list compact rows retain identity and default status; periods, filters, and full specifications remain only in full output.

Recommendation: Keep compact defaults bounded, document the full inspect/list recipes, and direct callers to immediate last-result expansion before rerunning a read.

Next use: Retrieve requested variant configuration from the saved full result instead of repeating remote reads after a compact discovery response.

Existing scope: S07, DEC-PW-016.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/pulse/metric/inspect/types.go:30-60
- actions/pulse/metric/list/types.go:47-84

### CTX-P07: Preflight default metric deletion

Type: Constraint diagnostic defect.

Finding: metric.delete reads target.IsDefault into the plan, but sends every non-preview target through the deleter and supplies only a generic dependency warning before a provider refusal.

Recommendation: Preflight IsDefault in preview and execution, return a specific no-write refusal with metric and definition IDs, and preserve remote checks for non-default targets.

Next use: Explain the non-default-only deletion rule before a default metric reaches the provider, while retaining the exact metric and definition identities.

Existing scope: DEC-PW-021, S38.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/pulse/metric/delete/types.go:16-20
- actions/pulse/metric/delete/action.go:33-65

### CTX-P08: Condition Pulse preview guidance on review state

Type: Incorrect next-action guidance.

Finding: definition.create computes compact requires_full and review_complete flags from bounded dimensions, but its preview action always returns help that says to run without --preview.

Recommendation: Condition preview help on review completeness: direct incomplete summaries to full or last-result review, and offer execution only after complete review.

Next use: Expand and review an incomplete compact preview before any authorized execution.

Existing scope: DEC-PW-002, S28.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/pulse/definition/create/action.go:170-174
- actions/pulse/definition/create/types.go:185-251

### CTX-P09: Retain portable Pulse source provenance

Type: Receipt improvement.

Finding: definition.pull full output returns definition ID/name, artifact paths, metric count, and request ID, but no source environment, site label, workspace, or datasource identity; the targeted artifact writer persists the supplied source metadata separately.

Recommendation: Project the existing source server origin, site label, environment, workspace, and datasource LUID with the pull receipt, while preserving the durable artifact metadata.

Next use: Carry a portable source identity beside the returned artifact path when selecting the source environment or mapping the source datasource for publication.

Existing scope: DEC-PW-009, S29, S33.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/pulse/definition/pull/types.go:59-113
- actions/pulse/definition/pull/action.go:81-81
- internal/app/pulse.go:472-482
- internal/artifact/pulse_definition.go:25-33

### CTX-P11: Normalize known empty follower collections

Type: Empty-state representation defect.

Finding: metric.followers compact output constructs a non-nil empty subscription slice, while FullOutput returns the raw Output and can preserve a nil slice from the reader as JSON null on a successful zero-follower result.

Recommendation: Normalize known successful empty follower collections to [] in full output while retaining error and coverage distinctions.

Next use: Distinguish a confirmed zero-follower collection from an unavailable or omitted collection in full output.

Existing scope: DEC-PW-011, S09.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/pulse/metric/followers/action.go:39-47
- actions/pulse/metric/followers/types.go:23-63

### CTX-P12: Remove stale preview advice after Pulse fork execution

Type: Incorrect next-action guidance.

Finding: metric.fork initializes preview help to run without --preview and returns that same output object with a confirmed metric ID when reconciliation fails, so an execution failure can retain preview-only next-action text.

Recommendation: Generate help from execution mode, lifecycle phase, and outcome; use exact full inspect for confirmed IDs and read-only reconciliation for unknown writes.

Next use: Inspect or reconcile a confirmed or uncertain outcome without interpreting stale preview text as permission to replay the mutation.

Existing scope: DEC-PW-012, S28.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/pulse/metric/fork/action.go:58-88
- actions/pulse/metric/fork/types.go:96-123

### CTX-R01: Retain the parent table in catalog findings

Type: Additional output improvement.

Finding: The exact parent table ID for each physical-column finding.

Recommendation: Return the known table LUID on physical-column findings in compact and full results, especially for database-wide audits containing several tables.
Preserve distinct Metadata and REST identities; do not invent an upstream column for a datasource field.

Next use: Fix a reported column description or tag using catalog column update, which requires both the column ID and table ID.

Scope: applies the settled useful-context principle; no new product approval is recorded.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/catalog/audit/action.go:34 omits parent identity from Finding.
- actions/catalog/audit/action.go:155-170 already has tableID and validates the returned column parent before discarding it.
- actions/catalog/audit/action.go:265-266 projects only the column identity and assessment.
- actions/catalog/column/update/action.go:73-78 requires --table-id.

### CTX-R02: Distinguish same-named physical tables

Type: Additional output improvement.

Finding: An available table schema or qualified name that distinguishes same-named physical tables.

Recommendation: Retain available schema or qualified-name context for table selection, particularly same-name candidates.
Keep authoritative IDs and parent IDs; never fabricate qualification when the provider omits it.
Avoid duplicating both fields when one supplies the needed distinction.

Next use: Choose the intended exact table ID before inspecting or changing metadata when database and short name are identical.

Scope: applies the settled useful-context principle; no new product approval is recorded.

Calls: no added enrichment call for these retained facts.

Evidence:

- internal/value/metadata.go:50-59 retains FullName and Schema on MetadataTable.
- internal/tableau/metadataassets/graphql.go:104 requests fullName and schema; :284 maps them.
- internal/tableau/metadataassets/client.go:239 maps the same observed REST fields.
- actions/catalog/table/list/action.go:49-72 and actions/catalog/table/inspect/action.go:40-48 omit them from compact output.
- actions/catalog/search/action.go:158 drops them while converting an already retrieved table into a search Item.

### CTX-R03: Retain live search scope

Type: Additional output improvement.

Finding: The selected environment and site attached once to a live search result.

Recommendation: Return the known environment/site once per live search response.
Keep compact rows limited to useful identities; cache generation provenance already supplies this scope and need not be repeated per row.

Next use: Reuse returned IDs in the correct environment, including saved search results after the active default changes.

Scope: applies the settled useful-context principle; no new product approval is recorded.

Calls: no added enrichment call for these retained facts.

Evidence:

- internal/app/search.go:47-93 resolves and passes the environment alias and site into the action.
- actions/search/types.go:50-80 has no selected environment/site fields in live Output, CompactResult, or FullResult.
- actions/search/action.go:111 only retains the environment in a generated hint for the first suitable content item.
- internal/output/output.go:67-84 and internal/app/app.go:200-211 add no general source-scope envelope.

### CTX-R04: Preserve installed version on check failure

Type: Additional partial-result improvement.

Finding: The installed version remains known when the remote release check fails.

Recommendation: Preserve the known installed version alongside the release-check error.
Leave latest version and update availability unverified; retain the failure exit and do not claim an up-to-date installation.

Next use: Report the installed version or diagnose update availability without issuing the advised second offline version command.

Scope: applies the settled useful-context principle; no new product approval is recorded.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/version/get/action.go:47-51 obtains current and constructs the installed-version result before the remote check.
- actions/version/get/action.go:58-60 discards it on check failure and recommends tadx version without --check.

### CTX-R05: Preserve the update target and known release context

Type: Additional receipt and partial-result improvement.

Finding: The resolved installation destination and already obtained version/release context.

Recommendation: Carry the already resolved destination into the update receipt and preserve known version/release context on installation failure.
Label it as the installation target, not proof of final binary state.
Do not invent successful Guidance targets or backup paths from a generic refreshed string.

Next use: Locate the binary targeted by an update and assess a failed installation without rediscovering its destination or repeating the release check.

Scope: applies the settled useful-context principle; no new product approval is recorded.

Calls: no added enrichment call for these retained facts.

Evidence:

- internal/update/runtime.go:60-76 resolves the executable and uses its directory as the installation target.
- actions/update/action.go:19-25 has no installation-path field.
- actions/update/action.go:56-61 already holds current/release context before Install but returns a zero result on failure.
- internal/update/runtime.go:80-119 only returns an error and does not expose a structured per-target Guidance receipt.

### CTX-R06: Return actual Guidance destinations and backups

Type: Receipt improvement.

Finding: Known package destinations and actual preservation backup paths.

Recommendation: Apply the accepted local-destination and backup receipt changes, retaining actual home-relative paths without inventing preview backup names.
Keep hashes and file counts expanded.

Next use: Read installed Guidance or recover a preserved package directly.

Existing scope: S27, DEC-PLATFORM-09.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/agent/install/action.go:19-28 retains Path and Backup, but :46-71 projects only target, name, and status.
- actions/agent/uninstall/action.go:17-23 retains Path and Backup, but :37-52 omits them in compact output.

### CTX-R07: Expose retained cache scope coverage

Type: Coverage improvement.

Finding: Requested, implicit, and completed cache coverage and counts by scope.

Recommendation: Apply S03 to expose bounded coverage states, reusing retained local coverage.
Keep cache files and operational diagnostics expanded where they do not help the ordinary next command.

Next use: Determine whether a subsequent cache-only command has the needed coverage without another inventory request.

Existing scope: S03, S26.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/cache/refresh/types.go:51-65 retains scope details but :93-130 reduces compact generation to aggregate state.
- actions/cache/status/types.go:11-34 carries only aggregate generation state at the action boundary.

### CTX-R08: Expand capability contracts and partial diagnostics

Type: Approved contract and diagnostic improvement.

Finding: Existing contract details, meaningful counts, contextual continuation, and available static rows on configuration failure.

Recommendation: Implement the already approved full-list contracts, continuation, and partial static results.
Keep the complete single-capability result as the adequate comparison case.

Next use: Select and invoke a supported command without repeated individual capability lookups.

Existing scope: S17, S18, S21, PULSE-06.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/capability/list/projection.go:14-25 makes full output identical to the compact summary.
- actions/capability/get/types.go:10-39 already defines the detailed contract.

### CTX-R09: Expose requested catalog facts and truthful gaps

Type: Requested-fact, failure, and Guidance improvement.

Finding: Requested metadata facts, observed empty values, precise failed selectors, and usable expansion guidance.

Recommendation: Apply the existing requested-fact, empty-state, failure, and saved-result guidance decisions.
Keep broad table inventories and column inventories bounded; their parent IDs already support exact next operations.

Next use: Read or edit the requested metadata without mistaking absence for missing evidence or rediscovering an exact selector.

Existing scope: S01, S07, S09, S11, S24.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/catalog/database/inspect/action.go:31-49, actions/catalog/table/inspect/action.go:31-50, and actions/catalog/column/inspect/action.go:34-53 retain richer objects but project identity-only compact items.
- actions/catalog/search/action.go:57-74 retains bounded identities and coverage but uses a generic --full marker.

### CTX-R10: Retain confirmed metadata changes

Type: Confirmation-result improvement.

Finding: Observed changed values when returned by the write response, distinct from the plan's requested values.

Recommendation: Apply S33 to retain sufficient returned confirmation.
If the provider omits a requested postcondition, use the separately approved bounded confirmation rule and never relabel a plan value as observed.

Next use: Use the resulting description, contact, or tag state without treating a request echo as saved confirmation.

Existing scope: S28, S33.

Calls: no added enrichment call for these retained facts. Missing postconditions still use the approved bounded confirmation rule.

Evidence:

- actions/catalog/database/update/action.go:51-64, actions/catalog/table/update/action.go:51-64, and actions/catalog/column/update/action.go:51-64 return identity plus completed phase names.

### CTX-R11: Expose actual diagnostic causes and dependencies

Type: Diagnostic defect.

Finding: Exact prerequisite cause, independently completed observations, and useful correction details.

Recommendation: Apply the settled diagnostic contract, making the actual cause and necessary next correction visible while retaining detailed non-secret observations in full output.

Next use: Repair the actual failed setting without diagnosing network or credential checks that never reached their domain work.

Existing scope: S01, S12, INV-PLATFORM-01.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/doctor/run/action.go:61-68 invokes all checker interfaces; the check handlers replace error details with broad summaries.
- actions/doctor/run/types.go:113-122 also hides corrective actions from compact output.

### CTX-R12: Return lineage paths and preserve partial capture

Type: Receipt improvement plus existing partial-capture defect.

Finding: The known canonical lineage payload path and source/workspace context; confirmed partial graph evidence where available.

Recommendation: Apply S29/S33 to the lineage receipt as well as content pulls, keeping paths workspace-relative.
Retain S25's partial-capture fix separately from the projection change.

Next use: Open or share the retained graph in the correct workspace without another directory inspection or capture.

Existing scope: S25, S29, S33.

Calls: no added enrichment call for these retained facts.

Evidence:

- actions/lineage/pull/types.go:89-103 retains canonical LineagePath and source provenance.
- actions/lineage/pull/types.go:176-185 compact output exposes only the artifact directory and graph summary; :189-215 exposes the canonical path and provenance in full output.
- actions/lineage/pull/action.go:79-90 discards graph content on capture failure, as already investigated under S25.

### CTX-R13: Acknowledge saved mutation policy separately

Type: Persistence receipt improvement.

Finding: The persisted setting acknowledgement, distinct from effective process policy.

Recommendation: Apply the existing persistence-receipt decision.
Mutation status already returns the useful saved/effective distinction and needs no extra generic fields.

Next use: Understand why the current effective setting differs from the saved change without repeating the setter.

Existing scope: DEC-PLATFORM-06, DEC-PLATFORM-14.

Calls: no added enrichment call for these retained facts.

Evidence:

- internal/value/mutation.go:4-9 exposes enabled, source, saved_enabled, and scope.
- internal/app/mutation_setting.go:35-41 writes then rereads effective state without returning a distinct persistence acknowledgement.

## Action coverage

Each action appears exactly once below.
These classifications describe this output-context audit, not total implementation readiness.

- Adequate: no additional output-context change identified; existing shared decisions can still apply.
- Known gap: covered by a previously settled output, help, or Guidance change.
- Additional gap: one of the eight newly identified locations, still within approved S33 scope.
- Separate read: distinguishes missing optional enrichment from already approved necessary confirmation.

Totals: 49 adequate, 52 known gaps, 11 additional-gap actions, and four separate-read cases.
The four separate-read cases are permission create/delete, optional project-list path resolution, and direct subscription-ID unfollow.
Permission confirmation is required when needed; the other two are enrichment limits, not new mandatory requests.

| Command | Action package | Assessment | Audit notes |
| --- | --- | --- | --- |

| tadx admin group create | actions/admin/group/create | additional gap | CTX-A06 |
| tadx admin group delete | actions/admin/group/delete | adequate | None |
| tadx admin group inspect | actions/admin/group/inspect | known gap | CTX-A03 |
| tadx admin group list | actions/admin/group/list | adequate | None |
| tadx admin group-member add | actions/admin/group/member/add | adequate | None |
| tadx admin group-member remove | actions/admin/group/member/remove | adequate | None |
| tadx admin group update | actions/admin/group/update | adequate | None |
| tadx admin label-category create | actions/admin/labelcategory/create | adequate | None |
| tadx admin label-category delete | actions/admin/labelcategory/delete | adequate | None |
| tadx admin label-category inspect | actions/admin/labelcategory/inspect | adequate | None |
| tadx admin label-category list | actions/admin/labelcategory/list | additional gap | CTX-A07 |
| tadx admin label-category update | actions/admin/labelcategory/update | adequate | None |
| tadx admin label-value delete | actions/admin/labelvalue/delete | adequate | None |
| tadx admin label-value inspect | actions/admin/labelvalue/inspect | adequate | None |
| tadx admin label-value list | actions/admin/labelvalue/list | additional gap | CTX-A07 |
| tadx admin label-value update | actions/admin/labelvalue/update | adequate | None |
| tadx admin permission create | actions/admin/permission/create | needs separate read | CTX-A09 |
| tadx admin permission delete | actions/admin/permission/delete | needs separate read | CTX-A09 |
| tadx admin permission inspect | actions/admin/permission/inspect | known gap | CTX-A04 |
| tadx admin user create | actions/admin/user/create | known gap | CTX-A05 |
| tadx admin user delete | actions/admin/user/delete | adequate | None |
| tadx admin user inspect | actions/admin/user/inspect | adequate | None |
| tadx admin user list | actions/admin/user/list | adequate | None |
| tadx admin user update | actions/admin/user/update | adequate | None |
| tadx agent install | actions/agent/install | known gap | CTX-R06 |
| tadx agent uninstall | actions/agent/uninstall | known gap | CTX-R06 |
| tadx auth check | actions/auth/check | known gap | CTX-P01 |
| tadx auth login | actions/auth/login | known gap | CTX-P02 |
| tadx auth logout | actions/auth/logout | adequate | None |
| tadx auth status | actions/auth/status | known gap | CTX-P03 |
| tadx cache refresh | actions/cache/refresh | known gap | CTX-R07 |
| tadx cache status | actions/cache/status | known gap | CTX-R07 |
| tadx capability get <id> | actions/capability/get | adequate | None |
| tadx capability list | actions/capability/list | known gap | CTX-R08 |
| tadx catalog audit | actions/catalog/audit | additional gap | CTX-R01 |
| tadx catalog column inspect | actions/catalog/column/inspect | known gap | CTX-R09 |
| tadx catalog column list | actions/catalog/column/list | adequate | None |
| tadx catalog column update | actions/catalog/column/update | known gap | CTX-R10 |
| tadx catalog database inspect | actions/catalog/database/inspect | known gap | CTX-R09 |
| tadx catalog database list | actions/catalog/database/list | adequate | None |
| tadx catalog database update | actions/catalog/database/update | known gap | CTX-R10 |
| tadx catalog search | actions/catalog/search | additional gap | CTX-R02, CTX-R09 |
| tadx catalog table inspect | actions/catalog/table/inspect | additional gap | CTX-R02, CTX-R09 |
| tadx catalog table list | actions/catalog/table/list | additional gap | CTX-R02 |
| tadx catalog table update | actions/catalog/table/update | known gap | CTX-R10 |
| tadx catalog label delete | actions/contentlabel/delete | known gap | CTX-A01 |
| tadx catalog label inspect | actions/contentlabel/inspect | known gap | CTX-A01, CTX-A02 |
| tadx catalog label list | actions/contentlabel/list | additional gap | CTX-A01, CTX-A07, CTX-A08 |
| tadx catalog label update | actions/contentlabel/update | known gap | CTX-A01 |
| tadx content datasource delete | actions/datasource/delete | adequate | None |
| tadx content datasource inspect | actions/datasource/inspect | known gap | CTX-C01 |
| tadx content datasource list | actions/datasource/list | known gap | CTX-C01, CTX-C06 |
| tadx content datasource move | actions/datasource/move | adequate | None |
| tadx content datasource publish | actions/datasource/publish | known gap | CTX-C03 |
| tadx content datasource pull | actions/datasource/pull | known gap | CTX-C02 |
| tadx content datasource schema | actions/datasource/schema | adequate | None |
| tadx content datasource update | actions/datasource/update | adequate | None |
| tadx doctor | actions/doctor/run | known gap | CTX-R11 |
| tadx env add | actions/env/profile/add | adequate | None |
| tadx env get | actions/env/profile/get | known gap | CTX-P04 |
| tadx env list | actions/env/profile/list | known gap | CTX-P04 |
| tadx env remove | actions/env/profile/remove | adequate | None |
| tadx env default | actions/env/profile/setdefault | adequate | None |
| tadx env update | actions/env/profile/update | adequate | None |
| tadx content flow delete | actions/flow/delete | adequate | None |
| tadx content flow inspect | actions/flow/inspect | known gap | CTX-C04 |
| tadx content flow list | actions/flow/list | known gap | CTX-C04 |
| tadx content flow move | actions/flow/move | adequate | None |
| tadx content flow publish | actions/flow/publish | known gap | CTX-C03 |
| tadx content flow pull | actions/flow/pull | known gap | CTX-C02 |
| tadx content flow update | actions/flow/update | adequate | None |
| tadx last | actions/last | adequate | None |
| tadx catalog lineage pull | actions/lineage/pull | known gap | CTX-R12 |
| tadx mutation set | actions/mutation/set | known gap | CTX-R13 |
| tadx mutation status | actions/mutation/status | adequate | None |
| tadx content project create | actions/project/create | adequate | None |
| tadx content project delete | actions/project/delete | adequate | None |
| tadx content project inspect | actions/project/inspect | known gap | CTX-C06, CTX-C07 |
| tadx content project list | actions/project/list | needs separate read | CTX-C06, CTX-C11 |
| tadx content project move | actions/project/move | known gap | CTX-C07 |
| tadx content project update | actions/project/update | adequate | None |
| tadx pulse definition create | actions/pulse/definition/create | known gap | CTX-P08 |
| tadx pulse definition delete | actions/pulse/definition/delete | adequate | None |
| tadx pulse definition inspect | actions/pulse/definition/inspect | known gap | CTX-P05 |
| tadx pulse definition list | actions/pulse/definition/list | adequate | None |
| tadx pulse definition publish | actions/pulse/definition/publish | adequate | None |
| tadx pulse definition pull | actions/pulse/definition/pull | known gap | CTX-P09 |
| tadx pulse metric delete | actions/pulse/metric/delete | known gap | CTX-P07 |
| tadx pulse metric follow | actions/pulse/metric/follow | adequate | None |
| tadx pulse metric followers | actions/pulse/metric/followers | known gap | CTX-P11 |
| tadx pulse metric fork | actions/pulse/metric/fork | known gap | CTX-P12 |
| tadx pulse metric inspect | actions/pulse/metric/inspect | known gap | CTX-P06 |
| tadx pulse metric list | actions/pulse/metric/list | known gap | CTX-P06 |
| tadx pulse metric unfollow | actions/pulse/metric/unfollow | needs separate read | Direct-ID enrichment limit |
| tadx search [term] | actions/search | additional gap | CTX-R03 |
| tadx | actions/session/overview | adequate | None |
| tadx update | actions/update | additional gap | CTX-R05 |
| tadx version | actions/version/get | additional gap | CTX-R04 |
| tadx content workbook delete | actions/workbook/delete | adequate | None |
| tadx content workbook inspect | actions/workbook/inspect | known gap | CTX-C04 |
| tadx content workbook list | actions/workbook/list | known gap | CTX-C04, CTX-C06 |
| tadx content workbook move | actions/workbook/move | adequate | None |
| tadx content workbook publish | actions/workbook/publish | known gap | CTX-C03, CTX-C05 |
| tadx content workbook pull | actions/workbook/pull | known gap | CTX-C02 |
| tadx content workbook update | actions/workbook/update | adequate | None |
| tadx workspace artifact delete | actions/workspace/artifact/delete | adequate | None |
| tadx workspace clean | actions/workspace/clean | known gap | CTX-C08 |
| tadx workspace clone | actions/workspace/clone | known gap | CTX-C10 |
| tadx workspace create | actions/workspace/create | known gap | CTX-C10 |
| tadx workspace delete | actions/workspace/delete | adequate | None |
| tadx workspace list | actions/workspace/list | adequate | None |
| tadx workspace artifact move | actions/workspace/move | adequate | None |
| tadx workspace register | actions/workspace/register | known gap | CTX-C10 |
| tadx workspace set-default | actions/workspace/setdefault | adequate | None |
| tadx workspace status | actions/workspace/status | known gap | CTX-C06, CTX-C09 |
| tadx workspace unregister | actions/workspace/unregister | known gap | CTX-C10 |

## Source recommendation coverage

All 89 final-category source recommendations remain accounted for.
The following mapping preserves source IDs while replacing stale initial decision labels with the latest approved disposition.
Source bundles are evidence inputs, not executable build instructions.
The 78 resolutions, approved choices, and investigation sections above contain the applicable behavior.

| Source category | Source recommendation | Current scope |
| --- | --- | --- |

| content-label | CL-R01 | CL-IDENTITY |
| content-label | CL-R02 | S01 |
| content-label | CL-R03 | S04 |
| content-label | CL-R04 | CL-IDENTITY |
| content-label | CL-R05 | S10 |
| content-label | CL-R06 | S33 |
| content-project | R01 | CP-AUTH-SCOPE |
| content-project | R02 | S01 |
| content-project | R03 | S05 |
| content-project | R04 | CP-PROJECT-ID-READ (approved) |
| content-project | R05 | S07 |
| content-project | R06 | S09 |
| content-project | R07 | S10 |
| content-project | R08 | S38 |
| content-project | R09 | CP-PROJECT-PATH |
| content-workbook | R01 | S33 |
| content-workbook | R02 | S28 |
| content-workbook | R03 | S28 |
| content-workbook | R04 | S38 |
| content-workbook | R05 | S12 |
| content-workbook | R06 | S29 |
| content-workbook | R07 | S07 |
| content-workbook | R08 | S29 |
| content-workbook | R09 | S04 |
| content-workbook | R10 | S04 |
| content-workbook | R11 | S31 |
| environment | R01 | S33 |
| environment | R02 | S01/S12 |
| environment | R03 | INV-PLATFORM-01 |
| environment | R04 | S09/S33 |
| environment | R05 | S10 |
| environment | R06 | S07 |
| environment | R07 | S04 |
| environment | R08 | S33 |
| environment | R09 | INV-PLATFORM-02 |
| installer | 1 | S01 |
| installer | 2 | S20 |
| installer | 3 | S29 |
| installer | 4 | S01/S15 |
| installer | 5 | S10/S15 |
| installer | 6 | S05/S10 |
| installer | 7 | S01 |
| installer | 8 | DOCTOR-OPTIONAL-CACHE (approved) |
| mutation | MUT-TADX-01 | S33 |
| mutation | MUT-TADX-02 | S01/S10 |
| mutation | MUT-TADX-03 | S17 |
| mutation | MUT-TADX-04 | INV-PLATFORM-03 |
| other | 1 | S22 |
| other | 2 | S10/S15 |
| other | 3 | S21 |
| other | 4 | S10/S15 |
| other | 5 | S10 |
| other | 6 | S05/S10 |
| pulse-definition | R01 | PULSE-01 |
| pulse-definition | R02 | S28 |
| pulse-definition | R03 | S38 |
| pulse-definition | R04 | S38 |
| pulse-definition | R05 | S28 |
| pulse-definition | R06 | PULSE-02 |
| pulse-definition | R07 | S10 |
| pulse-definition | R08 | S29/S30 |
| pulse-definition | R09 | S29 |
| pulse-definition | R10 | S33/S07 |
| pulse-definition | R11 | S09 |
| pulse-metric | R01 | PULSE-03 |
| pulse-metric | R02 | S28 |
| pulse-metric | R03 | PULSE-04 |
| pulse-metric | R04 | PULSE-02 |
| pulse-metric | R05 | S10 |
| pulse-metric | R06 | S33 |
| pulse-metric | R07 | S07 |
| pulse-metric | R08 | S38 |
| pulse-metric | R09 | S38 |
| pulse-metric | R10 | PULSE-05 |
| pulse-metric | R11 | S01 |
| pulse-metric | R12 | S38 |
| pulse-metric | R13 | S38 |
| pulse-metric | R14 | S38 |
| workspace | R01 | S01/S12 |
| workspace | R02 | S12 |
| workspace | R03 | S04/S10 |
| workspace | R04 | S29 |
| workspace | R05 | S29/S33 |
| workspace | R06 | S01 |
| workspace | R07 | S33 |
| workspace | R08 | S10 |
| workspace | R09 | S01/S31 |
| workspace | R10 | S09 |
| workspace | R11 | PULSE-06 (approved in chat) |

## Workspace handoff

Repository: ahillspace/tadx.
Audit branch: docs/search-investigation.
Audit HEAD: 33de8d4cefe928215967c21114ecd9ea9b176653.
This handoff does not assert that HEAD matches the current remote main branch.

The worktree already contained 37 unrelated modified or untracked paths.
Preserve them.
Source line references in this document refer to that working tree and can move during integration.

Only build-continuation.md is added to the repository by this handoff.
The detailed audit JSON and condensation notes remain with the external recommendation records.
No Overhead artifact changes occur.

Before a subsequent build:

- Read AGENTS.md and CONTRIBUTING.md.
- Respect the user's requirement to invoke tadx-build only when explicitly requested for a named build.
- Recheck status and reconcile existing work without destructive checkout, reset, cleanup, or silent overwrites.
- Follow the repository's updated-main and shared-feature-branch requirements after preserving the dirty work.
- Assign disjoint implementation paths and keep shared integration files under one coordinator.
- Preserve mutation settings; a build request does not authorize enabling or changing them.
- Do not commit this handoff or unrelated files unless the user authorizes the corresponding Git workflow.

The highest overlap risks are help/navigation, search identity, metadata assets, catalog column update, app tests, and Pulse authoring Guidance.
Several existing dirty files are directly inside the proposed implementation areas.
Do not attribute their changes to this review.

### Pre-existing changed paths

The following snapshot excludes this new handoff.

```text
 M README.md
 M actions/catalog/column/update/action.go
 M actions/catalog/column/update/action_test.go
 M docs/architecture/README.md
 M docs/command-structure.md
 M docs/evidence/metadata-semantics-contract.md
 M internal/agent/skills/tadx-pulse/references/authoring-contract.md
 M internal/app/app_test.go
 M internal/app/cache_review_e2e_test.go
 M internal/app/content_help_pilot_test.go
 M internal/app/testdata/capability-list-help.txt
 M internal/cli/catalog/column.go
 M internal/cli/help_content.go
 M internal/cli/help_datasource.txt
 M internal/cli/help_examples_test.go
 M internal/cli/help_flow.txt
 M internal/cli/help_project.txt
 M internal/cli/help_reference.go
 M internal/cli/help_test.go
 M internal/cli/help_values.go
 M internal/cli/help_workbook.txt
 M internal/cli/search.go
 M internal/resources/search/native.go
 M internal/resources/search/native_test.go
 M internal/tableau/metadataassets/client.go
 M internal/tableau/metadataassets/client_test.go
 M internal/tableau/search/client.go
?? docs/architecture/search.md
?? docs/search-go-guidelines-audit.md
?? hyperd.log
?? internal/app/help_audit_e2e_test.go
?? internal/app/search_identity_test.go
?? internal/cli/catalog/column_description_integration_test.go
?? internal/cli/help_navigation.go
?? internal/cli/help_navigation_test.go
?? internal/resources/search/native_identity_test.go
?? package-lock.json
```

### Verification performed for this handoff

- Matched 116 distinct action packages to 116 implemented CLI capability entries.
- Checked all audit finding references and coverage rows.
- Consolidated 39 shared decisions, 78 remaining resolutions, two original approved choices, and nine investigation dispositions.
- Retained the later PULSE-06 approval and finite Pulse Last Days choices.
- Preserved every one of the 89 final-category source recommendation mappings.
- Checked this Markdown for private absolute paths, missing internal sections, and unsupported completion claims.
- Confirmed no CLI source or Overhead file was changed by the handoff.

No build or runtime test result is implied.
Earlier investigation tests and local reproductions are historical evidence only.

### Completion criteria for the next build

Implementation is complete only after the selected changes, relevant regressions, installed help/Guidance, and required CI agree.
Report any remaining unsupported provider behavior or verification limits explicitly.
Keep S39 open unless new comparable evidence closes it.

If a GitHub push is later authorized, provide the required concise ChatGPT review prompt after each push.
Include repository, branch, base and head commit IDs, review scope, exclusions, and verification limits.
Do not launch that agent review yourself or claim an unrun check passed.

The current handoff ends at a completed evidence-backed shortlist and command-wide output audit.
The recommended next action is to begin the targeted build from the outcome/recovery and useful-known-context slices.
