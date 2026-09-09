# Claude kickoff: realistic TADX workflows

Copy the prompt below into a new Claude session.
Fill in the setup block with your test sites and credentials supplied separately.
Use PATs dedicated to this machine, not credentials another experiment is actively using.
This tests TADX workflows, not comparative performance against other products.

---

You are the orchestrator of a practical, budget-conscious TADX usability experiment.
Complete realistic multistep Tableau workflows, leave useful content on the disposable destination, and report what actually worked.
Do not stop at creating a plan, generating prompts, or launching a process.
Own execution through final verification and reporting.

## Setup supplied by the user

- Repository: `ahillspace/tadx`.
- Git revision to test: `<commit or branch; resolve and record its exact SHA>`.
- Source server URL and site content URL: `<source server>` and `<source site>`.
- Destination server URL and site content URL: `<destination server>` and `<destination site>`.
- Source PAT: `<dedicated credentials supplied separately>`.
- Destination PAT: `<different dedicated credentials supplied separately>`.
- Fake-user email domain: `<a domain the user owns>`.
- Time budget: 60 minutes, unless I specify otherwise.
- Model budget: use a low-cost model capable of reliable shell/tool use for workers.

If required setup is missing, ask one compact setup question before starting remote work.
Do not guess a site or borrow an active worker's PAT.
The source and destination must resolve to different server/site identities.

## Authorization and boundaries

The source is read-only: discovery, inventory, inspection, schema, lineage, catalog collection, and artifact downloads are allowed.
Never publish, update, move, delete, change access, or otherwise mutate the source.
All supported remote operations on the specified disposable destination are authorized for this experiment.
Use fake users with the supplied email domain and distinct run-scoped names for new resources.
Preserve pre-existing users, administrators, and unrelated resources.
Leave successful new resources in place unless a workload specifically tests removing something it created.

I authorize setting `TADX_ENABLE_MUTATIONS=1` in this experiment's child processes only.
Do not change persistent user/machine mutation policy or shell profiles.
Do not change the product, its installed Guidance, or its source code during a measured run.
Record product defects and suggested improvements for a later build.

Use separate source and destination credentials dedicated to this machine.
Run authenticated tasks serially unless each worker has its own credentials for every target it uses.
A verification process must not sign in with a worker's PAT while that worker is active.
Never put credentials or session tokens in prompts saved as evidence, command arguments, reports, or repository files.

## Prepare once, outside the timed workloads

1. Fetch the requested revision without discarding unrelated local work.
   Follow the repository's installation instructions to install that version of the actual TADX CLI.
   Verify the executable path and version, and record the source SHA and binary fingerprint.
   Do not assume the executable on PATH matches the checkout.
2. Install the bundled root and Pulse Guidance for Claude using the CLI's supported installer.
   Preserve existing customizations; do not force an upgrade over edited Guidance without permission.
   Record the installed package locations and fingerprints.
3. Verify source and destination authentication and actual site identities sequentially.
   Use an isolated experiment configuration and one registered workspace without changing the user's defaults.
   Preserve exact aliases and paths in a small setup record available to every worker.
4. Launch measured workers from a scratch directory outside the source checkout.
   Give them access to the installed skill packages and normal shell tools, not development docs or implementation source.
   Verify a worker can find and read the installed root skill and a relevant reference.
5. If you use a queue script, test it without Tableau first.
   Run three fake tasks with a deliberately failing middle task and confirm the final task still executes.
   Test resume behavior with failed old records so unfinished tasks cannot be mistaken for completed work.

Separate setup and authentication failures from measured product outcomes.
Start useful work promptly rather than building an evaluation framework.

## How to orchestrate

Use one worker per workload, sequentially, with a fresh task context.
Give the worker the user goal below, target authorization, setup aliases/workspace, run prefix, installed Guidance access, time budget, and a short ledger of established resources.
Do not prescribe commands, flags, exact source objects, or a step-by-step solution.
Do not supply hidden hints from earlier trials.
The installed TADX Guidance is the operating instruction package under test.

Use the least expensive available worker model that passes a basic shell/tool smoke test.
Record its exact model identifier and reasoning setting.
Keep the model and Guidance fixed throughout the campaign.
Allow at most one bounded escalation or retry for a genuinely useful recovery, and report its extra cost separately.

Budget about five minutes per workload, with a documented extension for an active productive task.
Prioritize workloads 1 through 7; do the remaining workloads while budget remains.
Do not spend the entire budget repeatedly attacking one unavailable capability.
Leave time for independent verification and a concise report.

Each worker should return exact saved identities, local artifacts, verified observations, incomplete steps, and a short retrospective:
"What was confusing or took the most effort, and what would have made the task easier?"
Ask it to note unexpected output, irrelevant hints, unnecessary detail, inconsistent flags, unclear errors, and repeated discovery.

A failed task must not halt unrelated work.
After an uncertain write, inspect the exact returned identity or narrowly scoped destination before retrying; do not blindly repeat the mutation.
On timeout, stop that worker and its child processes before starting another with the same PAT.
If termination cannot be confirmed, stop credential-sharing work rather than create overlapping sessions.
Resume from recorded successful identities, not from a wholesale replay.

For dependent tasks, use earlier successful artifacts and resources.
If a prerequisite failed, either create an appropriate replacement within the task's budget or mark that dependency explicitly blocked and continue independent tasks.
If cleanup fails, retain the resource IDs, mark them quarantined, and continue using distinct names without deleting unrelated resources.

## Workload goals

### 1. Sales team and project structure

Set up a small sales analytics team on the destination.
Create a run-named Sales Analytics area with Retail and Operations subprojects, two role-based groups, and three fake users with author, analyst, and viewer responsibilities.
Choose different supported site roles, establish sensible memberships, and verify the structure.
Leave the team ready for subsequent work.

### 2. Sales data foundation

Find two useful, complementary sales or retail datasources on the source.
Inspect enough schema to explain their uses, acquire them locally, and publish copies into the destination's sales analytics area.
Verify their identities and locations, and describe any remaining connection requirements honestly.

### 3. Regional sales dashboard

Give the team a useful regional or retail sales workbook from the source.
Investigate its datasource dependencies and migrate a suitable working set into Retail.
Reuse earlier content where supported.
If an unsupported source-bound dependency prevents migration, record the limitation and choose a suitable self-contained alternative.
Verify the published identity and location without claiming that publication alone proves every visualization works.

### 4. Preparation workflow

Find a useful preparation flow on the source, investigate important dependencies, and bring it into Operations.
Explain what it prepares and what would still be required to execute it.
Verify deployment without unnecessarily running refreshes.

### 5. Team access

Give the analyst group appropriate access to the new datasource and workbook, and give the viewer group suitable workbook access.
Use only run-created principals and content.
Inspect the resulting permission rules and explain whether effective user access has actually been proven.
Do not change site-wide defaults or existing administrators.

### 6. Analyst role change

The temporary viewer is joining the analyst team.
Update their supported site role and group memberships, verify the exact final membership set, and update the Operations project description.
Preserve the user's identity and unrelated access.

### 7. Meaningful sales Pulse metric

Create a meaningful sales-over-time Pulse definition using an appropriate migrated datasource.
Select eligible fields from schema, choose sensible formatting and business meaning, and inspect the saved configuration.
Create a useful timeframe variant and follow it for a run-created analyst if supported.
Leave it available to the team; do not confuse saved configuration verification with numerical correctness.

### 8. Filtered Pulse variant and subscribers

Create a meaningful dimension-filtered variant of the new sales metric using a supported field and a justified member value.
Configure following for a run-created user and group where supported, inspect it, then remove one subscription and verify the final state.
Do not invent datasource member values or use unavailable analytical capabilities as if TADX provided them.

### 9. Release layout

Create a run-named Published area, move a migrated workbook there, and rename it clearly for business users.
Verify that its identity is preserved, its location and name changed, and its associated datasource remains available.
Leave preparation flows in Operations.

### 10. Safe local iteration

Acquire a useful workbook into the workspace, make a harmless local change, and attempt to acquire it again.
Recover without losing the local edit, and verify the resulting local artifact state.
Keep backups and working files distinguishable; do not remotely mutate the source.

### 11. Collision and partial-failure recovery

Use disposable destination content to encounter a publish name collision, then recover without damaging another object.
In a small follow-on setup, deliberately use an invalid selector for a read-only verification step after a successful creation.
Determine what already succeeded, recover where possible, and remove only disposable resources this workload created for recovery testing.

### 12. Analyst handoff and catalog freshness

Prepare a concise handoff for an analyst arriving at the destination.
Use the catalog when useful, account for changes made during the campaign, and identify the best available workbook, datasource, flow, and Pulse definition.
Verify the current project layout and team memberships.
Include exact identities, useful local artifact locations, and honest gaps without inventing missing resources.

## Evidence and success criteria

Evaluate final state, not adherence to one command sequence.
Use TADX for measured workflows; if a requirement belongs outside TADX, identify that boundary and report it rather than silently completing it through a different API.
A separate evaluator may use read-only APIs or CLI reads for independent verification after the measured worker exits.

For each run, retain:

- Workload ID, unique run ID, exact prompt, model, CLI version/SHA/fingerprint, and Guidance fingerprints.
- Start/end times, duration, full raw tool/shell transcript, stdout/stderr, command exit codes, and command durations when available.
- Failed commands and their exact errors, with recovery marked confirmed, unconfirmed, or not attempted.
- Agent final response, retrospective, and exact resource IDs created or modified.
- Token usage from the provider's actual usage events: uncached input, cached input, and output separately.
- Independent verification results and any cleanup leftovers.

Do not infer tokens from context capacity or add cumulative snapshots as though they were separate usage events.
If a provider reports total input inclusive of cached input, subtract the cached amount to calculate uncached input and disclose that convention.
If usage is unavailable, say unavailable instead of fabricating an estimate.
Separate setup, measured worker execution, retries, and verification costs.

Independent checks should establish, as applicable:

- Source-selected identities and acquired local artifacts exist.
- Destination objects have the intended identity, name, project, and datasource linkage where inspectable.
- Users have the requested supported roles; groups have exact memberships.
- Expected explicit permission rules are present or absent.
- Pulse definitions and variants preserve the selected fields and requested saved settings, with expected followers.
- Moves retain identity, removals affect only intended objects, and earlier successful work remains available.
- Freshness and incomplete inventory coverage are disclosed.

Classify outcomes as verified success, partial success, product/capability blocked, agent failure, harness/setup failure, or not attempted.
An exit code of zero or an agent saying "done" is not sufficient for verified success.
Inventory presence does not prove workbook rendering, datasource connectivity, flow execution, effective permissions, or Pulse metric values.
Only claim those stronger results when actual evidence supports them.

## Finish

Do not report "running" as the final outcome and walk away from an unproven queue.
Confirm at least one real task completes and the next real task begins, then monitor at task boundaries or roughly five-minute intervals using saved local logs.
Avoid repeated Tableau authentication merely to check progress.
Stop launching work when the budget would leave insufficient time for verification.

Write a short `REPORT.md` and retain raw evidence outside the source checkout.
Include one table with workload, outcome, duration, uncached input, output, cached input, command count, error count, and independent verification status.
List useful resources left on the destination, unfinished work, and the three most consequential friction patterns.
Separate likely CLI defects, Guidance/discoverability issues, model mistakes, and harness failures.
Finish with completed and partial workloads, verified resource counts, total measured token usage, elapsed time, and the report location.
Do not claim the whole task is complete if the queue stopped early.
