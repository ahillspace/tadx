# tadx handoff: CLI agent-ergonomics work

Date: 2026-09-03
Base commit at handoff: see `git log -1` (was `c09e4cd` when this was written)
Branch: main

## Why this file exists

A prior session studied how an unaided agent drives the `tadx` CLI and wrote a grounded improvement proposal, `CLI-IMPROVEMENTS.md`, at the repo root.
The maintainer reviewed that proposal and narrowed it to the decisions below.
Implementation now occurs on the feature branch `feat/cli-discoverability`.

Read `CLI-IMPROVEMENTS.md` first for the full evidence and file:line references.
Read this file for the decisions layered on top of it.
Note: the file:line references in the proposal came from an earlier checkout, so line numbers may have drifted.
Trust the symbol, command, and flag names over the exact line numbers, and re-confirm before editing.

## The decisions made this session

The maintainer accepted mutation visibility and execution gating as the first CLI discoverability change.
Mutation commands and capability rows remain visible regardless of policy state.
When `TADX_ENABLE_MUTATIONS=1` is absent, a mutation command returns a stable actionable error before its action runs.
When the flag is present, mutation commands run by default and support `--preview` for a read-only plan.

## Build now

1. Keep all mutation commands and capability rows discoverable.
Report whether execution is enabled.
Use one centralized registry-driven execution policy for current and future remote mutations.
Expose `--preview` on consequential mutations, which otherwise run by default when enabled.

2. Normalize exact project selectors.
Use `--project` for canonical paths and `--project-id` for authoritative LUIDs on project inspect and update.
Keep list-only `--project-name` semantics explicit rather than treating different selectors as universal aliases.

3. Explain the CLI operating model in root and command help.
Name capability discovery, compact TOON, `--full`, the mutation policy, optional preview, logical workspaces, and portable artifact selectors.

4. Fix datasource collision checks for names containing ampersands or commas.
Do not weaken collision safety or add a bypass flag.
Use bounded unfiltered pagination and exact name plus project-LUID comparison when Tableau filter grammar cannot safely represent the name.

## Tabled

- Keep all published datasource guidance, automatic dependency publication, dependency commands, and partial acquisition unchanged until the maintainer makes a separate decision.
- Keep shell timeout, asynchronous operation, progress, and resumability changes out of this branch.
- Do not expand `AGENTS.md` or the repository skill until CLI discoverability is stable.
- Keep TDS unpacking, repackaging, and standalone datasource authoring deferred.
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
