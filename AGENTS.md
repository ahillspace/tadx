# TADX agent guide

TADX is a deterministic Tableau lifecycle and development CLI for coding agents and humans.
Its purpose is higher accuracy, lower tokens, and lower latency when operating Tableau.

Read this first, then go to the executable capability registry.

## Ownership in one line

Tableau MCP is the data analyst: it owns querying actual data (VDS/query-datasource), view data and images, and Pulse metric values and insights.
TADX is the development and lifecycle harness: content and artifacts, workspaces, administration, and Pulse definition and configuration lifecycle.
TADX never calls or proxies MCP inside a command. CLI and MCP are peer surfaces; the agent chooses.

## How to work here

- Start with the capability registry: run capability list to discover, capability get to inspect one capability's ownership, selectors, safety, evidence, and blockers.
- Start every build from updated `main` on one new feature branch.
- Use one shared branch and checkout for the build.
- Do not create per-agent worktrees unless the user explicitly requests them.
- Assign agents disjoint files and reserve shared integration files for one coordinator.
- Every executable command is one isolated action package. To understand a command, read its package.
- Cobra is thin plumbing. It parses arguments, invokes an action, renders the output, and maps the exit code. It contains no Tableau behavior.
- Resource adapters isolate Tableau API complexity. Actions call adapters through narrow interfaces the action owns; actions never call raw HTTP.
- Tests define externally visible behavior before implementation.
- Blocked capabilities are registry metadata only. Do not guess a blocked or docs-only capability into existence; a capability needs its upstream contract captured before any live API code is written.

## Non-negotiables

- PAT authentication only.
- Default output is TOON.
- Default output is an explicit compact projection. Add --full to the same command for expanded, bounded details.
- Consequential mutations preview by default and require --apply. --force never means --apply.
- Mutation discovery gating is not authorization.
- Tableau LUIDs are authoritative; ambiguous selectors fail; no fuzzy or interactive resolution.
- Secrets TADX handles are never persisted or printed.
- Persist and render artifact paths relative to the resolved workspace with forward slashes.
- Resolve absolute filesystem paths only at runtime.
- Never put a developer username, home directory, checkout path, private site name, or unrelated local project name in tracked first-party files or fixtures.

## Read only what the task needs

- A slice agent reads the focused `capability get <id> --full` result, its filled task card, one closest code example, the output guidelines, and one task-specific evidence record.
- The coordinator reads `docs/build-order.md`, reconciles authoritative product sources, and owns shared integration files.
- Read `docs/contributing/adding-a-capability.md` or `docs/contributing/adding-an-adapter.md` only for the role being performed.
- Never read the arc42, the full capability contract, or captured API references end to end.
- Use targeted `rg` searches and bounded sections when an authoritative source must be consulted.
- Use `docs/contributing/api-documentation-routing.md` for Tableau API evidence.

## Archived material

Files under `archived/` are obsolete historical inputs.
Do not read them for current implementation guidance.
