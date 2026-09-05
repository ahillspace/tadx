# TADX agent guide

TADX is a deterministic Tableau lifecycle and development CLI for coding agents and humans.
It owns content lifecycle, artifacts, workspaces, administration, and Pulse definition lifecycle.
Tableau MCP separately owns data queries, view data and images, and Pulse metric values and insights.
TADX commands never call or proxy MCP.

Use [CONTRIBUTING.md](CONTRIBUTING.md) as the day-to-day human build guide.
Use `$tadx-build` only when the user explicitly invokes it for a named build task.

Every executable command is one isolated action package.
Cobra is thin plumbing, actions own narrow interfaces, resource adapters isolate Tableau APIs, and the composition root performs wiring.
Tests define externally visible behavior before implementation.
Blocked and docs-only capabilities remain non-executable until bounded upstream evidence closes the gate.

Default output is compact TOON, and `--full` returns expanded bounded details for the same operation.
Consequential mutations run by default when enabled and support `--preview` for a read-only plan.
Mutation discovery is not authorization, and `--force` does not bypass mutation policy.
Tableau LUIDs are authoritative, ambiguous selectors fail, and resolution is never fuzzy or interactive.
Tableau authentication uses PATs only.
PATs and session tokens are never persisted or printed.
Persist and render artifact paths relative to the resolved workspace with forward slashes.
Never put a developer username, home directory, checkout path, private site name, or unrelated local project name in tracked files or fixtures.

Start builds from updated `main` on one feature branch shared by all assigned agents.
Assign disjoint paths and reserve shared integration files for one coordinator.
Do not create per-agent worktrees unless the user requests them.
Do not read `archived/` for current guidance.
