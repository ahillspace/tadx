# tadx handoff: CLI agent-ergonomics work

Date: 2026-09-03
Base commit at handoff: see `git log -1` (was `c09e4cd` when this was written)
Branch: main

## Why this file exists

A prior session studied how an unaided agent drives the `tadx` CLI and wrote a grounded improvement proposal, `CLI-IMPROVEMENTS.md`, at the repo root.
This session reviewed that proposal with the repo owner and narrowed it to a concrete plan.
No source was changed in either session.
Implementation happens on the owner's own build machine, not in this environment.

Read `CLI-IMPROVEMENTS.md` first for the full evidence and file:line references.
Read this file for the decisions layered on top of it.
Note: the file:line references in the proposal came from an earlier checkout, so line numbers may have drifted.
Trust the symbol, command, and flag names over the exact line numbers, and re-confirm before editing.

## The decisions made this session

The owner reviewed all eleven items from the proposal and split them into build-now and shelf.
The two big framing changes from the original proposal:

1. This machine is not the build machine.
Do not implement here.
Produce specs and plans that the owner hands to their own coding agent.

2. The write-mutation gate is being redesigned after V1.
The owner intends to change the gate so it actually blocks write actions, but wants to wait until the app is built and all V1 actions are done before deciding how that works.
So every item about mutation discoverability and gating is shelved, not because it is wrong, but because it will be reworked wholesale during the gate redesign.

## Build now (hand to the coding agent)

1. Fix the `&` / `,` collision bug.
Pure correctness bug.
The datasource collision check rejects any datasource whose name contains an ampersand or comma instead of encoding it for the lookup filter.
It permanently blocked a real datasource ("Hubbell Global Sales & Pipeline Final") in the migration test.
Fix: URL-encode the name in the `name:eq:<name>` filter value (the surrounding `url.Values` would encode it correctly if the guard did not fail first).
Optionally add a `--skip-collision` escape hatch.
Guard is at `internal/tableau/datasource/client.go` (`if strings.ContainsAny(field.value, ",&")`); all four collision modes hit it via `actions/datasource/publish/action.go`.
Add a test that publishes a datasource whose name contains `&`.
No dependency on anything else.

2. Automatic idiot-proof publish.
When a workbook was pulled together with its published datasources, sequence the publish automatically: publish the datasources first, then the workbook.
No flag, no unpackaging, no manual ordering.
Use the dependency data already persisted in the artifact `metadata.json` under `published_datasources` (luid, name, source_site, local_artifact_path); see `internal/artifact/workbook.go`.
If a datasource is genuinely missing at publish time, return a named error that says which one, instead of the generic `workbook.publish.failed`; suggested id `workbook.publish.missing-datasource`, mapped from Tableau 400011 / PublishingException in `actions/workbook/publish/action.go`.
This folds together proposal items 5, 2, and 7.
The design goal the owner stated: the tool should make publishing just work, not teach the LLM the inner workings of Tableau.

3. Automatic async for extract-bound operations.
Auto-detect that an artifact carries an extract (the tool already detects this at pull time) and default to a non-blocking handle so no single call ever exceeds the agent shell timeout (~120s).
Two mechanisms, chosen automatically: submit as a Tableau server-side job where Tableau supports it (the existing `--as-job` primitive on publish), or local-detach for operations Tableau will not run as a server job (for example a pull/download).
Return the job or handle id plus the exact poll command the agent should run next.
Keep a `--wait` flag for humans who want to block.
Safe to build now for `pull` (read-only); the `publish` side rides the same machinery and is coupled to the gate below.
This is proposal item 4.

4. Normalize project-selection flags.
Accept `--project`, `--project-name`, and `--project-id` as aliases on every verb that selects a project.
Today `list` uses `--project-name`, `get` uses `--project` (slash path), and `publish` uses `--project` plus `--project-id`, which produced "unknown flag" retries.
Cheap papercut fix, no dependency on the gate or on V1.
This is proposal item 6.

### One coupling to remember

Items 2 and the publish half of 3 are mutations, so their live behavior depends on the write gate that is being redesigned after V1.
The orchestration logic (sequencing, async, error naming) does not care how the gate works, only that publish exists, so it is safe to build now.
It just will not be exercised until the gate is settled.

## Shelved until V1 is locked

- Write-command discoverability and visible-but-inert gated verbs (proposal items 1 and 3).
Deferred into the gate redesign the owner is planning after V1.

- Top-level help and `AGENTS.md` authoring (proposal item 9).
The owner will rewrite it once the app is locked, so writing it now is wasted.
`AGENTS.md` already exists at the repo root; expanding it is the eventual task, not creating it.

- Read-only `content workbook deps` command (proposal item 8).
Nice to have; the automatic publish in build-now item 2 removes most of the need to inspect deps by hand.

- Partial-failure-tolerant `--include-pds` (proposal item 10).
The owner is a server admin, so the permission-scoped 401 that triggered this in the migration test is unlikely to bite them.
Robustness-only, low priority.

- TDS unpack/repackage and standalone datasource authoring.
The owner's own roadmap item.
They already have an app that unpacks and repackages `.tds`/`.tdsx` and are deliberately waiting on V1 before adding workspace manipulation.
Post-V1.

## Two architecture questions the owner raised, answered

Shell timeouts and auto-backgrounding.
The tool can and should decide, not the agent.
A synchronous publish or pull carrying a large extract runs past the agent shell's hard wait limit and gets killed even though it would have succeeded.
"Background" means either a Tableau server-side job (instant job id, work continues on Tableau's servers, agent polls status) or a local detached child (parent exits immediately printing an output-file path, agent polls the file).
tadx can auto-pick based on the extract it already detects.
The one thing automation cannot remove is that a genuinely multi-minute operation still needs a second call to collect the result; you cannot compress it into one sub-timeout call.
But you can guarantee no single call ever hangs and have the tool print the exact next command.
That is build-now item 3.

Command nesting depth.
The three-level shape `tadx content workbook publish` is not a mistake.
It matches `gcloud compute instances create`, `aws ec2 describe-instances`, and `kubectl config set-context`.
More importantly, tadx already has a flat capability registry with ids like `content.workbook.publish`, so the deep tree is a human-friendly skin over a flat namespace, and agents should drive off `capability list` and `capability get <id>` rather than walking the tree.
The friction the migration test hit was hidden commands and inconsistent flags, not depth.
The only thing worth a later look is whether the `content` top group earns its level or just swallows most verbs (the "catalog-vs-content split" already flagged in `TODO.md`), which is a naming refinement, not a rearchitecture.
Recommendation: keep the tree, lean on the flat registry for agents, fix the flags.

## Repo build conventions the coding agent must follow

From `AGENTS.md` and `CONTRIBUTING.md`:

- Tests define externally visible behavior before implementation (TDD-first).
- Every executable command is one isolated action package under `actions/<domain>/<verb>`; actions do not import Cobra, `net/http`, another action, or a concrete resource adapter.
- Default output is compact TOON; `--full` is a bounded superset; keep separate compact and full golden fixtures for detail-bearing output.
- Generated files are never hand-edited: `internal/capability/registry_gen.go` and `docs/reference/capabilities.md`.
Change the authoritative source, then run `go generate ./...` and `go run ./cmd/gencapdocs -out docs/reference/capabilities.md`.
- Making hidden verbs visible would change CLI output that golden or snapshot tests may assert on; check for those before flipping any `Hidden`.
Not relevant to the build-now set, but relevant if the shelved gate work is picked up.
- Before pushing: run `gofmt` (CI has a separate gofmt gate that a green local build, vet, and test will not catch), then `git diff --check`, `go vet ./...`, and `go test ./...`.

## Standing owner constraints

- No em dashes in human-facing writing; use a plain dash.
- Sentence per line in long markdown.
- Never add agent or Claude authorship as co-author on commits or PRs.
- Do not touch auto-generated files.
- The owner handles their own PATs and secrets; never persist or print them.

## Housekeeping done at handoff

Committed the proposal (`CLI-IMPROVEMENTS.md`), the backlog (`TODO.md`), the capability-map diagram, and this handoff.
Removed throwaway artifacts (`.DS_Store` files and a completed council-review scratch tracker under `data/scratch/`) and added `.DS_Store` and `/data/` to `.gitignore`.
