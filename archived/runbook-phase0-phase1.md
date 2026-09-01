# Archived TADX Phase 0 and Phase 1 build runbook

This runbook is obsolete and retained only as historical build evidence.
Use `docs/build-order.md` and `docs/contributing/build-context.md` for current build routing.

Run these steps in order on the build machine.
Each step is one action. The fenced blocks are copy-paste prompts for a fresh agent.
Do not skip a verify step; the freeze gates are what keep parallel agents from drifting.

---

## Step 0. Place the docs (manual, no agent)

Create the repo, then put the pre-build docs where they belong:

- AGENTS.md -> repo root
- scope-v1.md -> docs/scope-v1.md
- axi.md -> docs/axi.md
- toon.md -> docs/toon.md
- action-guidelines.md -> docs/contributing/adding-a-capability.md
- build-order.md -> docs/build-order.md
- task-template.md -> docs/contributing/task-template.md
- README.md -> docs/README.md (or keep as the pre-build index)

Create a root CLAUDE.md that is a thin pointer (contracts stay in the docs, not duplicated here):

```
# TADX

Read AGENTS.md first. The binding contracts live in docs/.

Never build a capability except through docs/contributing/adding-a-capability.md,
using docs/contributing/task-template.md. Do not weaken or route around an
enforcement gate to make work pass; if a gate blocks correct work, stop and flag it.
```

Commit this as the first commit. Everything after this builds on a repo that already carries the guidelines.

---

## Step 1. Scaffold the foundation (Phase 0)

Give this to one agent. Nothing else runs until its freeze gate closes.

```
You are scaffolding the TADX repository foundation (Phase 0). Build only the foundation; do not build any resource capability.

Read first, in order: AGENTS.md; docs/build-order.md (Phase 0); docs/contributing/adding-a-capability.md; docs/axi.md; docs/toon.md; docs/scope-v1.md. If sources conflict, the arc42 and V1 capability contract win, then AGENTS.md, then the docs; stop and flag conflicts rather than guessing.

Establish and freeze exactly what build-order Phase 0 lists: the modular-monolith package layout and dependency direction with an architecture test that fails on violation; the executable capability registry and its validation; the thin Cobra layer and the per-capability action pattern; the authentication provider seam (PAT hidden behind it); the identity and selector model with the exact-match helper; the configuration model; the output layer and the TOON codec; the structured error type and exit-code mapping; generated capability reference docs with a clean-diff check; CI (format, vet, test, race, cross-compile for all four target platforms, tidy).

Implement only two executable commands: capability list and capability get. No other command becomes executable.

The TOON codec must be chosen or implemented, conformance-tested against the pinned upstream spec version, round-trip tested, fuzzed, and frozen.

Done means: everything above exists, CI is green on all four platforms, and the Phase 0 exit gate in build-order is satisfied. Report the enforcement gates you built and their status.
```

## Step 2. Verify the Phase 0 freeze gate (manual or a quick agent check)

Confirm before going further:
- CI green on all four platforms.
- Architecture test, registry validation, and TOON conformance/round-trip/fuzz all present and passing.
- Only capability list and capability get are executable.
- The six contract docs are checked in.

Do not start Step 3 until this holds.

## Step 3. Build the vertical slice (Phase 1)

One agent. This freezes the shared spine every later action copies.

```
You are building the TADX Phase 1 vertical slice. Build only this slice.

Read first: AGENTS.md; docs/build-order.md (Phase 1); docs/contributing/adding-a-capability.md; docs/axi.md; docs/toon.md.

Build the slice end to end: auth check, then catalog search (or content search), then workbook pull, then workbook publish (preview and apply) - all driven through the registry, rendered in TOON, under the evidence gate.

Building the slice also builds and freezes the shared spine listed in build-order Phase 1: the Tableau transport (port from the trusted Go inventory implementation), the first REST client family and first resource adapter (establishing the adapter pattern others copy), the pagination normalization envelope, the pull artifact contract with re-pull dirty-guard, and the publish preview/apply pattern with bounded internal async polling and upload sessions.

Follow the action-guidelines contract for every action: tests first, narrow owned interfaces, no Cobra/net-http/cross-action imports, TOON output only, LUID identity with hard-fail ambiguity, preview-by-default plus --apply for the publish.

Done means: the slice passes contract and golden tests; the transport, adapter pattern, pagination envelope, artifact contract, and publish pattern are frozen and documented; CI green on all four platforms.
```

## Step 4. Verify the spine is frozen

Confirm the transport, adapter pattern, pagination envelope, artifact contract, and publish preview/apply pattern are documented and frozen. Later waves copy these; they must not be reinvented.

## Step 5. Fan out the waves (A through F, in build-order order)

For each wave, do 5a once, then 5b for every capability in the wave. Waves: A local-contract, B remote read, C deliver in, D deliver out, E administration, F doctor.

### 5a. Build the wave's resource adapter first (one agent, then freeze)

```
You are building the resource adapter for <resource> in TADX, before that resource's actions fan out.

Read first: AGENTS.md; docs/build-order.md (the wave this resource belongs to); docs/contributing/adding-a-capability.md; docs/toon.md.

Build only the adapter and its read path, copying the frozen Phase 1 adapter pattern and pagination envelope. Do not build the resource's action commands yet. The adapter must not import Cobra, CLI, or action packages.

Done means: the adapter compiles, its read path passes contract and golden tests, and it is documented so the wave's action agents can depend on it. CI green.
```

Freeze the adapter before continuing.

### 5b. Build each capability in the wave (one agent per capability, in parallel)

For each capability, copy docs/contributing/task-template.md, fill its eight fields (registry ID, domain/verb, disposition, read-vs-mutation, evidence level, selectors, upstream operation), and hand the filled template to one agent. The template is self-enforcing: done means the enforcement gates pass. A docs-only capability stops at the adapter seam and is not made executable.

---

## Blocked capabilities (do not schedule)

B1 datasource field-description write (published-datasource-field level), B2 composable datasource round-trip, B3 Pulse mutations, B4 shallow project enumeration stay registry metadata only. Do not assign a build task until the gate closes with captured official upstream source and a passing contract test. See build-order.
