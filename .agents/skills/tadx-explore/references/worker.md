# Luna worker protocol

This reference applies to the spawned Luna worker for a TADX exploration run.
Read it before using TADX.
Use the generated run files as authoritative for scope, selectors, budgets, and report structure.

## Operate as an end user

Operate the installed TADX CLI as a user.
Read the installed TADX operating skill and focused command help before exploring.
Start with the requested category, resource, or verb.
Invent realistic journeys and useful flag or output variations instead of following an authored test list.
Use discovery, bounded reproduction, cleanup, and reporting as the overall shape.
Do not read repository source as the initial shortcut.
Do not broaden into unrelated command discovery, benchmarks, harness work, or source hunts.
Do not edit product source, policy code, or documentation.
Do not spawn subagents, launch recursive scouts, or apply automatic source fixes.
Run one Luna scout per fresh pass.
Do not write to the shared `tadx-feedback` ledger.
Use default compact TOON output plus meaningful `--full`, `--json`, flag, invalid-input, and error variations.
Cover meaningful behavior, not exhaustive combinations.
Use small batches and do not run load, concurrency stress, credential, or session experiments.
Record each scenario's outcome and honest coverage limits.
Tally and self-report CLI calls, without presenting the tally as command telemetry.

## Apply authorization and safety

Assume remote read-only behavior unless the run manifest explicitly records fixture-write authorization.
Do not change authentication, PATs, site consent, mutation policy, installed software, or ordinary user configuration.
Do not use global mutation overrides.
If site consent blocks a requested mutation, keep read-only coverage and record a blocker requiring explicit permission for the exact server, site, and persisted setting.
A request to test an operation does not authorize a consent change.
Read-only work can create downloads, caches, and scratch fixtures only in isolated, run-owned paths.
Never overwrite a source workspace, original configuration, or shared state.
If an action cannot be isolated safely, report it as blocked or preview-only.

For an authorized fixture write, follow these rules:

- Create unique, run-scoped names for every fixture.
- Publish fixture content from the selected workspace under those unique names.
- Recheck the target immediately before each write.
- Record planned intent in `journal.json` before remote creation.
- Record confirmed identities, locations, and job IDs promptly after creation.
- Test delete or move behavior only against copies created by this run.
- Use run-owned identities, including previously deleted fixture IDs, for negative destructive cases.
- Leave pre-existing remote resources and workspace originals untouched.
- Do not overwrite an existing resource or delete and republish it for restoration.
- Keep cleanup limited to resources created by this run.
- Journal disposable local files, caches, and scratch workspaces before creation.

Preserve every confirmed identity after a failed or unverified write.
Do not blindly retry a pending or unknown outcome.
Record an unknown identity or location as uncertain and stop further writes that depend on it.
Stop further passes when cleanup is incomplete or unresolved.
Treat deletion as unresolved until the outcome is confirmed or explicitly recorded as uncertain.
Report each remaining identity and location exactly enough for safe follow-up.
Respect the cleanup reserve in the run manifest and do not broaden cleanup targets.

## Respect pass budgets

Read `previous_brief` in the pass manifest and vary coverage without claiming regression evidence that you did not collect.
Obey `call_limit`, `exploration_calls`, `cleanup_reserve`, and `deadline` from the pass budget.
Tally setup, help, exploration, reproduction, and cleanup calls against the pass limit.
Stop before the deadline or limit, and report the self-accounted count.

## Use the generated report contract

Open the generated pass `manifest.json`, `report.json`, and `journal.json` before acting.
Edit those drafts in place rather than creating a parallel report format.
Use the generated report scaffold as the source of truth for required keys and allowed values.
The helper rejects unknown keys, so preserve the scaffold identity fields and add no custom fields.
Read the contract at `paths.report_contract` in the pass manifest before editing coverage, findings, or resources.
Use that generated contract for item shapes, enums, bounds, and nullable fields instead of inventing a schema.
Every report includes `tadx_version` and `repository_revision` keys.
Use null for either value when evidence is unavailable.
A completed report with missing version evidence stops continuation, so use a blocker or uncertain status when that evidence prevents a completed conclusion.
Keep the draft status pending while working and finish with the supported terminal status.
Record meaningful scenarios in coverage with their variations, outcomes, and limits.
Classify findings with the supported kind, severity, and confidence values.
Separate observed behavior, expected behavior, impact, reproduction, and recommendation for each finding.
Record blockers when authorization, environment, or an unmet prerequisite limits coverage.
Use the optional structured error section only with `exit_code`, `code`, `phase`, `outcome`, or `excerpt`, and keep details bounded and sanitized.
Keep an error excerpt at or below 4000 characters.
Never record credentials, PATs, session tokens, or other secrets.
Set the TADX version and repository revision when available, and use null when unavailable.
Describe output size qualitatively unless a named tokenizer actually measures tokens.
Do not label byte or character counts as token counts.
Keep evidence optional and limited to meaningful findings under the generated pass `evidence/` directory.
Do not attach bulk logs, screenshots, or command transcripts.
Keep the resource journal empty with cleanup status `not_needed` only when the pass created no disposable local or remote resource.
Journal and clean disposable local resources with confirmed outcomes, while retaining generated reports and evidence.
Before finishing, update cleanup status and resolve every planned, created, or uncertain journal entry.
Only `removed` or `not_created` entries permit the helper to advance to another pass.

Return a handoff of 120 words or fewer with the pass status, key finding counts, cleanup state, and report path.
