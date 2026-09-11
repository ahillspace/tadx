# TADX agent guide

TADX is a deterministic Tableau lifecycle and development CLI for coding agents and humans.
It owns content lifecycle, artifacts, workspaces, administration, and Pulse definition lifecycle.
Tableau MCP separately owns data queries, view data and images, and Pulse metric values and insights.
TADX commands never call or proxy MCP.

Use [CONTRIBUTING.md](CONTRIBUTING.md) as the human contribution entry point.
The repository-local [.agents/skills/tadx-build/SKILL.md](.agents/skills/tadx-build/SKILL.md) contains the maintained engineering standards and focused implementation references.
Invoke `$tadx-build` only when the user explicitly requests it for a named build task.
Keep repository-wide agent instructions here and task-specific development instructions in that skill, not in plans or parallel agent files.

Every executable command is one isolated action package.
Cobra is thin plumbing, actions own narrow interfaces, resource adapters isolate Tableau APIs, and the composition root performs wiring.
Tests define externally visible behavior before implementation.
Blocked and docs-only capabilities remain non-executable until bounded upstream evidence closes the gate.
Do not weaken an enforcement gate to make work pass; flag a gate that blocks correct work.

Default output is compact TOON, and `--full` returns expanded bounded details for the same operation.
`--json` changes encoding only; preserve parseable results, redaction, and partial failures.
Support bounded same-action batches where useful, reusing command-scoped sessions and ordered per-item outcomes rather than inventing workflow orchestration.
Keep routine pull enrichment diagnostics in metadata and full output; surface unmet explicit requests.
Report known failure phases, mutation outcomes, missing prerequisites, and confirmed partial results through the shared error contract.
Do not infer safe retries from transport retryability, invent missing evidence, or discard confirmed identities after verification fails.
Keep follow-up commands contextual and coverage limitations explicit; shared rendering handles configuration preservation and bounded recovery output.
Consequential mutations run by default when enabled and support `--preview` for a read-only plan.
Supported read-only `--preview` operations remain available when the mutation gate is off.
Mutation discovery and previews do not authorize execution, and `--force` does not bypass mutation policy.
Before changing `TADX_ENABLE_MUTATIONS` or the saved mutation setting through `tadx mutation set`, agents must obtain explicit user permission for that setting change and its scope.
This applies to enabling, disabling, or unsetting it through any mechanism, including command overrides, process or session environments, wrappers, scripts, shell profiles, and persistent user or machine settings.
A request to perform a Tableau operation does not authorize changing this flag.
Reuse prior permission only when it explicitly covers the same setting change and scope; session permission does not authorize persistence.
Ask one short question, for example: "May I enable remote mutations for this session, allowing TADX to create, change, or delete Tableau resources?"
For a persistent change, name its scope and explain that it affects future shells.
Tableau LUIDs are authoritative, ambiguous selectors fail, and resolution is never fuzzy or interactive.
Tableau authentication uses PATs only.
PATs can persist only after explicit user approval in the native OS credential store.
Configuration stores only an opaque credential reference.
PATs and session tokens never appear in configuration values, output, logs, artifacts, caches, fixtures, or diagnostics.
Persist and render artifact paths relative to the resolved workspace with forward slashes.
Never put a developer username, home directory, checkout path, private site name, or unrelated local project name in tracked files or fixtures.

Start builds from updated `main` on one feature branch shared by all assigned agents.
Assign disjoint paths and reserve shared integration files for one coordinator.
Do not create per-agent worktrees unless the user requests them.
Do not read `archived/` for current guidance.
Local API captures are optional and ignored; [docs/evidence/README.md](docs/evidence/README.md) links the official documentation available to fresh clones.

The maintainer runs agent-based code reviews in ChatGPT.
Run automated tests and CI, but do not launch agent-based reviews unless explicitly requested.
After every GitHub push, provide a concise, paste-ready ChatGPT review prompt with the repository, branch, base and head commit IDs, review scope, and material exclusions or verification limits.
Do not repeat review methodology in that prompt; the maintainer supplies it separately.
