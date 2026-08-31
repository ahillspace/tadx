# Tableau agent development harness

**Document type:** Lean arc42 system definition  
**Status:** Architecture complete — implementation handoff  
**Working repository / product / binary:** `tadx`  
**Primary implementation language:** Go  
**Primary interface:** AXI-style CLI  
**Secondary interface:** VS Code extension, deferred until CLI contracts stabilize  
**Supported product families:** Tableau Cloud, Tableau Server, Tableau Desktop interoperability, and Tableau Pulse  
**Excluded product family:** Tableau Next  
**Last reconciled:** 2026-08-29

## Document purpose

This document is the authoritative architecture definition for TADX.

The architecture interview is complete. There are no remaining product or architecture questions that require another user decision before implementation can begin. Items that still require API verification, implementation choices, or later product work are explicitly classified as implementation work or deferred work rather than unresolved architecture.

The governing product outcome is:

> **Higher accuracy. Lower tokens. Faster.**

TADX exists to make coding agents substantially better at operating Tableau by giving them deterministic, discoverable, context-efficient tools around Tableau's existing APIs, MCP surfaces, artifacts, and product behavior.

This document intentionally does not specify every low-level API payload, every exact response field, or every implementation package. Coding agents may decide ordinary implementation details as long as they preserve the contracts and boundaries defined here.

## Source precedence

When sources disagree, use this order:

1. Confirmed decisions from the design conversation.
2. This fully reconciled arc42 document.
3. The scoped product PRD.
4. The earlier detailed PRD.
5. External reference implementations and local prototype repositories.
6. Implementation inferences.

External repositories and prototype applications are evidence and reusable code sources, not architecture authorities.

## Status markers

- `[DECIDED]` — implementation must preserve this behavior or boundary.
- `[IMPLEMENTATION]` — coding agents may choose the exact mechanism without reopening architecture unless the choice changes user-visible behavior or a major boundary.
- `[VERIFY]` — factual API behavior must be checked during implementation, but the product decision is already made.
- `[DEFERRED]` — valid work intentionally excluded from V1.
- `[OUT OF SCOPE]` — not part of the product direction.

No `[OPEN]` architecture items remain.

---

# 1. Introduction and goals

## 1.1 Product thesis

`[DECIDED]` Build an open-source, agent-native Tableau development harness.

Tableau already exposes powerful capabilities through REST APIs, Metadata API, Pulse APIs, Tableau Desktop integrations, MCP servers, and Tableau artifacts. The problem is not absence of capability. The problem is that those capabilities are fragmented, verbose, inconsistent, and expensive for coding agents to repeatedly rediscover and operate directly.

TADX provides the deterministic development/tooling layer around those surfaces.

It does not replace Tableau.

It does not replace MCP.

It does not become an agent.

It does not become a workflow engine.

## 1.2 Product manifesto

`[DECIDED]`

> **Higher accuracy. Lower tokens. Faster.**

These are product outcomes:

1. **Higher accuracy**
   - Reduce hallucinated operations.
   - Reduce ambiguous resource targeting.
   - Reduce brittle ad hoc API construction.
   - Reduce repeated reinterpretation of raw Tableau responses.
   - Make failures explicit and deterministic.

2. **Lower token/context use**
   - Keep discovery bounded.
   - Avoid loading complete API schemas or full site inventories into model context.
   - Return compact structured output.
   - Let agents discover only the capability detail they need.
   - Prefer deterministic CLI primitives over repeated prompt-level API reasoning.

3. **Lower latency**
   - Reduce tool switching.
   - Reduce unnecessary agent turns.
   - Reduce repeated resource discovery.
   - Hide mechanical polling or multi-call mechanics inside deterministic primitives when no decision point exists.

## 1.3 Layer ownership invariant

`[DECIDED]`

```text
TOOLS        -> CLI / MCP
WORKFLOWS    -> Skills
REASONING    -> Agent
PRESENTATION -> VS Code
```

Interpretation:

- **Coding agent** owns reasoning, planning, selection, and composition.
- **Skills** own reusable multi-command procedures that require branching, judgment, mapping, review, or policy.
- **Tableau MCP** is the data-analyst surface: it owns analysis, querying actual file/datasource data, and Pulse metric value/insight retrieval.
- **TADX CLI** owns the remaining Tableau development/lifecycle surface, including content, artifacts, administration, and Pulse definition/configuration lifecycle.
- **VS Code** is a later human presentation layer over the same core behavior.
- **Raw Tableau APIs** remain available for long-tail operations not worth promoting into the CLI.

The CLI never proxies or invokes MCP.

A single CLI verb should not secretly combine MCP and CLI execution. If a future deterministic TADX primitive requires a Tableau API such as VDS internally, TADX may call that Tableau API directly if the capability passes the admission test; it still does not invoke MCP.

## 1.4 Primary users

`[DECIDED]` TADX is agent-first but not agent-only.

Initial users include:

- Tableau administrators using coding agents.
- Tableau developers and solution engineers using coding agents.
- Humans using the CLI directly.
- Coding agents operating Tableau programmatically.
- Future VS Code users inspecting and manipulating the same TADX workspace/state.

Humans must be able to understand default CLI output, artifacts, provenance, failures, and intended remote effects without needing to decode agent-only payloads.

## 1.5 V1 scope

`[DECIDED]` V1 is a capability platform, not one minimal workflow.

V1 must include enough coherent deterministic primitives that coding agents can perform common Tableau lifecycle and development tasks without repeatedly constructing raw API calls.

### Core

- Go.
- One Go module.
- One primary self-contained TADX binary.
- Modular-monolith internal architecture.
- Cobra for command/flag plumbing.
- AXI as the behavioral contract.
- Shared executable capability registry.
- Compact TOON output.
- Human-usable CLI behavior.
- Windows AMD64.
- macOS AMD64.
- macOS ARM64.
- Linux AMD64.

### Environment and authentication

- Named Tableau environment profiles.
- Tableau Cloud and Tableau Server.
- PAT authentication.
- Environment-variable / local `.env` secret resolution.
- Read-only default environment support.
- Explicit target environment for remote writes.
- `doctor` diagnostics.

### Capability discovery

- `capability list`.
- `capability get`.
- Capability ownership metadata.
- Mutation capabilities hidden from default discovery.
- Persistent environment-variable switch for mutation discovery.
- Bounded, agent-efficient discovery.
- MCP-only / MCP-preferred capability guidance when TADX is not the preferred execution surface.

### Catalog and discovery

- Explicit site/catalog hydration.
- Cached normalized inventory.
- Projects.
- Content items.
- Read-only permissions.
- Users/groups where useful.
- First-class lifecycle-oriented content search.
- Bounded agent-facing list/search results.
- Full site hydration may consume the complete remote inventory internally because it is written to cache rather than returned wholesale to the model.

### Workspace and artifacts

- Local named workspaces.
- Multiple workspaces per repository/project.
- Artifacts from multiple Tableau environments in one workspace.
- Visible provenance.
- Canonical payload.
- Required human-readable view.
- Baseline fingerprint.
- Local modification detection.
- Local move/organization/status.
- External tooling may modify local artifacts.

### Content lifecycle

- Workbooks.
- Datasources.
- Flows.
- Projects.
- Search.
- Get/inspect.
- Pull.
- Publish.
- Preview/apply.
- Re-pull with dirty-state protection.
- Shallow project behavior.
- Composable datasource round-trip fidelity.
- Composable datasource authoring.
- Datasource field metadata read/enrichment.
- Datasource field-description write-back using released APIs after API verification.

### Pulse

- Separate CLI domain.
- Pulse definition/configuration discovery.
- Pulse definition/configuration retrieval.
- Pulse definition/configuration materialization as local JSON-backed artifacts.
- Deterministic Pulse definition/metric lifecycle mutations that current released APIs support.
- MCP remains primary for querying Pulse metric values, insights, and analytical results.

### Narrow administration

- Users.
- Groups.
- Group membership through deterministic resource update semantics.
- Permission discovery/inspection.
- No permission mutation in V1.

### Skills

- A small number of first-party skills for common multi-command orchestration patterns that TADX intentionally does not abstract into one CLI verb.
- Most CLI primitives do not need a reference skill.
- Skills are added when several primitives must be composed and a meaningful workflow remains above them.

## 1.6 V1 exclusions and fast follows

### Explicitly deferred after V1

- `pack` / `unpack`.
- Hyper <-> CSV conversion.
- TDS remote datasource work-copy editing.
- TDS work-copy diff.
- TDS staged-change impact analysis.
- Lineage/downstream traversal as a general CLI capability.
- Recursive project migration.
- Generic bulk pull/publish.
- Permission mutation.
- Deep semantic diffing.
- Generic remote content move.
- Plugin architecture.
- Background daemons/synchronization.
- Tableau Next.
- Connected App / OAuth / JWT authentication.
- Native package-manager distribution.
- Embedded auto-update.
- Automatic artifact revision history.
- Linux ARM release binary.
- Destructive-operation policy hook framework.

### Out of scope

- Replace Tableau Desktop.
- Build custom workbook authoring.
- Build custom flow authoring.
- Build a general BI agent.
- Build a custom chat product.
- Replace `tabcmd`.
- Recreate every Tableau API endpoint.
- Build a complete Tableau administration console.
- Route MCP through TADX.
- Build a generic dependency-management system.
- Phone-home telemetry.
- Offline mutation queues.
- A special air-gapped execution mode.
- FIPS certification in V1.

---

# 2. Architecture constraints

## 2.1 Go and process model

`[DECIDED]`

- Go is the primary implementation language.
- One Go module initially.
- One primary TADX binary.
- Modular monolith.
- No plugin system in V1.
- No unnecessary service/process boundaries.
- Commands stay thin.
- Reusable Tableau behavior lives in internal packages.
- External Tableau tools are not required for core V1 behavior.

Post-V1 capabilities may depend on external Tableau tooling when justified. Tableau Desktop MCP is an expected near-term peer integration. Hyper API may be used for a future capability if equivalent functionality cannot reasonably be implemented in Go.

TSC, Tableau Document API, and Tableau Migration SDK remain references unless a later admitted capability explicitly chooses one as a dependency.

## 2.2 CLI framework and AXI

`[DECIDED]`

Cobra is implementation plumbing.

AXI controls:

- command semantics,
- progressive disclosure,
- bounded context,
- discovery behavior,
- output shape,
- composability,
- deterministic errors,
- safety,
- agent ergonomics.

Reference implementations such as `gh-axi` and `tasks-axi` may inform TADX behavior. TADX documents its own specification rather than instructing contributors to copy another repository verbatim.

## 2.3 Platforms

`[DECIDED]`

V1 release binaries:

```text
windows/amd64
darwin/amd64
darwin/arm64
linux/amd64
```

`[DEFERRED]` Linux ARM.

Desktop-specific integrations may be narrower where Tableau itself imposes platform limitations.

## 2.4 Tableau version policy

`[DECIDED]`

TADX has no global minimum Tableau Cloud, Server, or Desktop version gate.

TADX does not proactively inspect a Tableau product version and block commands that might otherwise work.

Behavior is optimistic:

1. Resolve and validate the command.
2. Attempt the admitted supported API operation.
3. If Tableau rejects the operation because that deployment does not support it, return the normal actionable failure.

Users of older Tableau Server deployments may discover that some TADX capabilities work and others do not. TADX does not artificially prevent supported calls from working on an older environment.

## 2.5 Enterprise networking

`[DECIDED]`

Enterprise proxy/custom-CA support is not a V1 product requirement. TADX does not build special proxy or certificate-management behavior beyond ordinary platform/Go HTTP behavior.

## 2.6 Cryptographic compliance

`[DECIDED]`

No FIPS or equivalent cryptographic-compliance guarantee in V1.

Use standard Go TLS/crypto correctly. Add a compliance-specific build only if an actual deployment later requires it.

## 2.7 Licensing

`[DECIDED]`

- All code in the user's current programs is available for reuse in this open-source project.
- Existing programs should be actively mined for implementation context, behavior, tests, and reusable code.
- Reused code must still be reviewed for quality, simplicity, latency, and fit.
- External dependencies/runtime components receive ordinary license review before redistribution.
- If an external license is incompatible, reproduce the behavior rather than incorporating the implementation.
- The TADX repository is intended to be open source; MIT is the expected license unless repository setup selects another permissive license.

This is no longer an architecture blocker.

---

# 3. Context and capability ownership

## 3.1 Capability admission test

A proposed TADX capability must pass these questions:

1. Does it materially improve agent accuracy, token/context use, latency, or local development ergonomics?
2. Is it deterministic enough for a tool?
3. Does it require a meaningful decision, branch, review, or judgment in the middle?
   - If yes, keep the workflow in the agent/skill and expose only the missing primitives.
   - If no, a deterministic pseudo-primitive may combine the mechanical steps.
4. Does MCP already expose the capability effectively?
5. Is the operation common enough across lifecycle workflows that intentional overlap materially reduces tool switching or repeated agent reasoning?
6. Would implementing it recreate substantial Tableau product functionality?
7. Can it have a stable deterministic contract?
8. Which layer owns it?

## 3.2 MCP / CLI ownership rule

`[DECIDED]`

Use a concrete responsibility boundary rather than an overlap threshold:

> **MCP is the data analyst. TADX CLI is the development/lifecycle harness.**

MCP owns analysis and querying the actual data in files/datasources. This includes VDS/data queries and Pulse metric values/insights.

TADX CLI owns deterministic Tableau development/lifecycle operations outside analytical data querying. This includes content discovery and lifecycle, files/artifacts, workspaces, administration, and Pulse definition/configuration lifecycle.

If the user needs to query the actual data in a file or datasource for any reason, prefer MCP. Otherwise there should generally be a TADX CLI capability when the operation passes the capability-admission test.

TADX never calls MCP inside a CLI verb.

## 3.3 Low-context ownership discovery

`[DECIDED]`

The capability registry stores ownership metadata such as:

```text
owner
preferred_surface
overlap
reason
products
mutation
selectors
```

`capability list` returns a compact inventory.

`capability get <capability>` returns focused detail even when the capability is MCP-only or MCP-preferred.

An MCP-only lookup should tell the agent, in bounded structured output, that TADX does not execute the operation and which surface is preferred.

The repository's `AGENTS.md` is the high-level agent-routing guide. Focused capability discovery provides deeper tool-specific guidance and explicitly points the agent to MCP when MCP owns or is preferred for an analytical/querying capability.

## 3.4 Current directional ownership

| Area | Primary surface | TADX position |
|---|---|---|
| VDS / analytical query | MCP | Do not wrap as a standalone V1 query surface |
| Pulse values / insights | MCP | MCP-primary |
| Pulse definitions/configuration | TADX CLI | Retrieve, materialize, and mutate where deterministic |
| Desktop workbook authoring/modification | Desktop MCP / Tableau | TADX does not author workbooks |
| Content search | TADX CLI + deliberate MCP overlap | CLI-first for lifecycle |
| Workbook pull/publish | TADX CLI | V1 |
| Datasource pull/publish | TADX CLI | V1 |
| Flow pull/publish | TADX CLI | V1 where released APIs support it |
| Project shallow lifecycle | TADX CLI | V1 |
| Composable datasource lifecycle | TADX CLI | V1 |
| Datasource metadata enrichment/write-back | TADX CLI | V1 released path |
| Users/groups | TADX CLI | Narrow deterministic admin |
| Permissions | TADX CLI | Read/inspect only |
| Generic lineage traversal | Later | Deferred |
| TDS work-copy editing | Later | Fast follow after API maturity |

---

# 4. Solution strategy

## 4.1 Operation taxonomy

The internal capability-analysis categories remain:

```text
Find
Inspect
Change
Deliver
```

These are architecture categories, not mandatory public command groups.

### Find

Locate and resolve resources.

### Inspect

Read authoritative or explicitly cached state without changing the selected Tableau resource.

### Change

Perform an explicit mutation.

### Deliver

Move or publish resource representations between local and remote contexts.

## 4.2 Approved public verb vocabulary

`[DECIDED]`

Prefer a small repetitive vocabulary:

```text
list
get
search
create
update
delete
pull
publish
follow
unfollow
status
refresh
check
```

Use resource-specific verbs only when they describe a genuinely distinct high-leverage operation.

Do not create separate `add-member` / `remove-member` verbs merely because group membership changes exist. Membership changes should normally be expressed through group update semantics unless API/ergonomic evidence later makes a separate verb clearly better.

`pack` and `unpack` are intentionally deferred until after V1.

TDS work-copy verbs are intentionally deferred until after V1.

## 4.3 Deterministic pseudo-primitives

`[DECIDED]`

A command may perform several mechanical steps if:

- every step always belongs together,
- there is no meaningful mid-flow decision,
- intermediate state does not require agent judgment,
- failure semantics are deterministic.

Example: `pull` may resolve, download, create the artifact directory, write provenance, calculate the baseline fingerprint, and render the human-readable view.

This is not a workflow-engine exception. It is a tool abstraction over deterministic mechanics.

## 4.4 First-party skills

`[DECIDED]`

TADX may ship first-party skills, but only for common orchestration patterns involving several CLI/MCP calls that have intentionally not been abstracted into one deterministic command.

Most primitives do not get companion skills.

Skills remain visible, editable, and forkable procedures.

---

# 5. Building block view

## 5.1 CLI shell

- Cobra command/flag plumbing.
- Configuration/context resolution.
- Output mode selection.
- Exit-code mapping.
- Thin command handlers.

## 5.2 Capability registry

`[DECIDED]` The executable source of truth for:

- capabilities,
- ownership,
- preferred surface,
- overlap metadata,
- product support,
- selectors,
- mutation visibility,
- safety metadata,
- help/discovery generation.

Do not maintain a second hand-authored runtime capability inventory.

## 5.3 Configuration and environment resolution

Owns:

- environment aliases,
- Tableau URLs,
- site IDs,
- auth type,
- optional username,
- default read environment,
- environment-scoped default workspace,
- general default workspace,
- non-secret settings.

## 5.4 Authentication

PAT-only V1.

Owns:

- PAT name/secret variable resolution,
- sign-in,
- token validation,
- clear expiry/invalid credential failures,
- redaction.

## 5.5 Tableau clients

Direct released API clients for admitted capabilities.

No MCP proxy layer.

## 5.6 Resource adapters

Resource-specific behavior for:

- workbooks,
- datasources,
- flows,
- projects,
- Pulse definitions/metrics,
- users,
- groups,
- permissions inspection.

## 5.7 Identity resolver

- Tableau IDs are authoritative.
- Names and paths are selectors.
- Ambiguity is an error.
- No fuzzy mutation targeting.
- No interactive selector prompt.

## 5.8 Workspace/artifact manager

Owns:

- workspace discovery,
- deterministic layout,
- artifact provenance,
- baseline fingerprints,
- local modification detection,
- local move/status/cleanup,

## 5.9 Catalog subsystem

Owns:

- site hydration,
- normalized cache,
- explicit refresh,
- staleness metadata,
- bounded search/get/status output.

The catalog may internally hydrate a complete site inventory. That full inventory is cache input, not model output.

SQLite is a preferred starting implementation because the existing Go `tabget` implementation already supports it effectively. Exact schema/index tuning remains an implementation decision.

## 5.10 Output/error layer

Owns:

- TOON rendering,
- `--full`,
- `--raw`,
- structured errors,
- pagination metadata,
- partial outcomes where meaningful,
- exit codes,
- secret redaction.

## 5.11 Delivery/validation

Owns:

- explicit target resolution,
- preview,
- `--apply`,
- capability-specific validation,
- remote collision handling,
- optional native revision checks where Tableau exposes trustworthy concurrency state.

It does not promise a generic optimistic-concurrency system.

## 5.12 Doctor

Non-mutating diagnostics for:

- configuration,
- PAT variable presence,
- PAT validity,
- Tableau connectivity,
- relevant MCP availability,
- catalog status,
- workspace status,
- optional log/diagnostic context.

---

# 6. Runtime behavior

## 6.1 Workspace resolution

`[DECIDED]`

```text
explicit --workspace
    ->
workspace containing current directory
    ->
selected environment's configured default workspace
    ->
general configured default workspace
    ->
fail
```

No automatic workspace creation as a `pull` fallback.

## 6.2 Selector resolution

`[DECIDED]`

Use ordinary CLI flags.

Common selectors:

- `--environment`
- `--workspace`
- `--id`
- `--name`
- `--project`

Project paths use exact slash-delimited human-readable selectors.

No custom locator URI/DSL.

No custom escaping language beyond normal shell argument quoting.

A remote move/rename does not change Tableau identity. If an old human path no longer resolves, it fails rather than being silently redirected.

## 6.3 Pull

A content pull:

1. Resolves one remote resource.
2. Resolves the workspace.
3. Selects a deterministic local artifact directory.
4. Retrieves the canonical representation.
5. Writes provenance metadata.
6. Writes a baseline fingerprint.
7. Writes the required human-readable representation.
8. Applies re-pull rules if the same authoritative resource already exists.

## 6.4 Re-pull

`[DECIDED]`

Match by authoritative Tableau identity.

```text
local fingerprint == baseline
    -> warn
    -> replace with fresh remote representation
    -> update provenance/baseline

local fingerprint != baseline
    -> stop
    -> require --overwrite

--overwrite
    -> warn
    -> replace
    -> update provenance/baseline
```

No automatic revision history.

## 6.5 Consequential remote mutations

`[DECIDED]`

Every consequential remote mutation follows:

```text
resolve exact target
validate
preview by default
--apply to mutate
report deterministic outcome
```

No second confirmation prompt.

No extra production-only confirmation.

`--force` never means `--apply`.

`--force` has no universal global semantics. It may exist only for a specific capability-specific guard. It never bypasses:

- ambiguity,
- authentication,
- required write-target resolution,
- Tableau's own permission enforcement,
- `--apply`.

## 6.6 Mutation discovery gating

`[DECIDED]`

Mutation gating changes discovery only.

The persistent switch is:

```text
TADX_ENABLE_MUTATIONS=1
```

When unset/false:

- mutation commands are absent from default agent-facing help/capability discovery.

When set:

- mutation capabilities appear in discovery/help.

It does not authorize execution.

A caller that already knows the command may invoke it regardless of discovery visibility; remote mutation still requires `--apply`.

The switch may be made persistent by the user through normal shell/OS environment configuration. Do not store mutation-discovery state in a workspace.

## 6.7 Remote conflict behavior

`[DECIDED]`

TADX does not perform optimistic-concurrency or remote-change detection before publish/mutation.

It validates the requested target and operation, previews by default, and on `--apply` invokes the Tableau operation. TADX does not compare current remote state with the state observed at pull time and does not block publication merely because the remote resource may have changed.

Tableau remains authoritative. API-level conflicts or rejected updates are surfaced as normal actionable operation failures.

## 6.8 Async Tableau operations

`[DECIDED]`

Avoid making the LLM poll.

If a Tableau API operation returns an asynchronous job and the CLI can deterministically wait for a terminal result:

- TADX polls internally.
- Polling is bounded by a timeout.
- The CLI returns the terminal success/failure result.
- The Tableau job/request ID is included where available.

Expose a separate job/status handle only when the operation cannot reasonably complete in one CLI invocation.

This is a valid deterministic pseudo-primitive because there is no meaningful decision in the polling loop.

## 6.9 Retries

`[DECIDED]`

No generic automatic retries.

A capability may add a bounded internal retry only if the retry is demonstrably safe, deterministic, and materially improves reliability.

Otherwise the caller owns retry policy.

## 6.10 Shallow project behavior

`[DECIDED]`

V1 project operations are shallow:

- selected project,
- direct supported content items,
- no child-project recursion,
- no generic dependency graph traversal.

TADX does not attempt to understand every transitive dependency across project boundaries.

Resource-native dependencies already encoded in a workbook/datasource remain part of that resource's own canonical semantics.

### Partial failure

On failure:

- stop the operation,
- report resources that succeeded,
- report the failed resource/operation,
- do not roll back completed remote changes,
- do not provide automatic resume in V1.

A future version may add resumability if completed work can be identified deterministically.

---

# 7. Deployment, configuration, and local state

## 7.1 Distribution

`[DECIDED]`

Initial distribution:

- GitHub Releases.
- Self-contained TADX binaries.
- SHA-256 checksums.

No V1 binary signing.

No embedded auto-update.

No native package-manager requirement.

No silent system modification.

## 7.2 Configuration files

`[DECIDED]`

Human-edited configuration uses YAML.

### User-global config

Use the platform's standard user configuration directory (`os.UserConfigDir`) under a `tadx` directory:

```text
<tadx user config dir>/config.yaml
```

Examples are platform-dependent and resolved by Go rather than hardcoded into CLI semantics.

### Workspace config

A workspace root contains:

```text
tadx.yaml
```

The workspace config is visible and versionable because it contains no secrets.

### Config model

Illustrative contract:

```yaml
version: 1

default_environment: production
default_workspace: ./workspaces/default

environments:
  production:
    url: https://example.tableau.com
    site_id: example-site
    auth:
      type: pat
      pat_name_env: TADX_PRODUCTION_PAT_NAME
      pat_secret_env: TADX_PRODUCTION_PAT_SECRET
    default_workspace: ./workspaces/prod
```

Environment aliases are map keys.

Environment-scoped workspace default takes precedence over the general default.

## 7.3 PAT variable naming

`[DECIDED]`

Default variable convention:

```text
TADX_<ENV_ALIAS>_PAT_NAME
TADX_<ENV_ALIAS>_PAT_SECRET
```

Normalize environment aliases for variable names by uppercasing and replacing non-alphanumeric characters with `_`.

Example:

```text
environment alias: production-us
TADX_PRODUCTION_US_PAT_NAME
TADX_PRODUCTION_US_PAT_SECRET
```

Visible configuration may override the variable names.

PATs are not refreshed. Invalid or expired PATs fail clearly and must be replaced.

## 7.4 Secrets

`[DECIDED]`

TADX itself never hardcodes or persists secrets in:

- visible config,
- workspace metadata,
- artifact metadata,
- catalog cache,
- CLI output,
- logs,
- diagnostics.

Secrets live in environment variables or a local git-ignored `.env`.

TADX is not a general secret-policing/DLP product.

It does not scan or reject arbitrary user-authored Tableau files merely because the user chose to embed a credential or sensitive value in their own content.

The boundary is:

> TADX must not leak or unnecessarily persist secrets that TADX itself handles.

## 7.5 Workspace layout

`[DECIDED]`

High-level form:

```text
workspace/
  tadx.yaml
  artifacts/
    <resource-kind>/
      <human-name>/
        <canonical-payload>
        metadata.json
        view.md
  .tadx/
```

Conventions:

- `.tadx/` is hidden implementation state.
- `metadata.json` stores machine-readable provenance/baseline metadata.
- `view.md` is the required human-readable view.
- Native packaged/file resources preserve their native extension for the canonical payload.
- API-native resources use `resource.json` as the canonical payload.
- Exact safe-name sanitization is an implementation detail; local path is never authoritative identity.

## 7.6 Local state concurrency

`[DECIDED]`

TADX does not provide workspace/state locking or concurrent-process coordination in V1.

If multiple TADX processes race while modifying the same local workspace/state, that race is the caller's responsibility. Do not add advisory locks, distributed locks, collaboration semantics, or other coordination machinery.

## 7.7 Logging

`[DECIDED]`

Persistent logging is off by default.

Enable it with an environment variable:

```text
TADX_LOG_LEVEL=<level>
```

Recommended levels:

```text
error
warn
info
debug
trace
```

When enabled:

- logs are local only,
- logs are redacted,
- logs may be written to ordinary local text files,
- each invocation receives a correlation ID,
- Tableau request/job IDs are included when available.

A SQLite log sink may be added later if useful; it is not required by the architecture.

No logs are uploaded automatically.

`doctor` may later package local diagnostics if troubleshooting evidence makes that valuable.

---

# 8. Cross-cutting resource concepts

## 8.1 Authoritative identity

`[DECIDED]`

Tableau IDs are authoritative remote identity.

Names, project paths, and local paths are selectors/labels.

Local directory renames do not mutate Tableau.

## 8.2 Artifact model

Every pulled artifact contains:

1. Canonical resource representation.
2. Machine-readable metadata.
3. Required human-readable representation.

Provenance includes, where applicable:

```text
kind
name
tableau_id
source_environment
source_site
source_project_name
source_project_id
pulled_at
local_baseline_fingerprint
```

The recorded source (`source_environment`, `source_site`, `name`, `tableau_id`) is the default publish target when the artifact is later published; an explicit `--environment` overrides it, for example to promote to a different environment. Apply re-resolves the recorded LUID against Tableau and fails deterministically if it was renamed, moved, or deleted rather than overwriting a different resource. A recorded source environment absent from local configuration is a deterministic error.

## 8.3 Composable datasources

`[DECIDED]` Composable datasource support is V1.

TADX V1 must support:

- pulling existing composed datasources without losing direct parent datasource references,
- preserving the published-datasource composition semantics needed for round-trip republish,
- deterministic creation/modification of published-datasource composition relationships through released APIs,
- local workspace management of datasource artifacts used in composition,
- explicit publishing of the resulting datasource.

The coding agent decides *what* should be composed and how.

TADX executes explicit deterministic composition inputs.

This is not a reasoning workflow inside the CLI.

## 8.4 Datasource field metadata

`[DECIDED]`

V1 includes deterministic datasource metadata capabilities built on released APIs:

- retrieve field metadata,
- materialize optional richer field metadata during pull,
- maintain field ID mappings,
- retrieve descriptions and relevant semantics,
- write field descriptions back through the released metadata/catalog path after verification against the live API.

Generation of descriptions remains agent reasoning.

TDS Edit is not required for V1 field-description write-back.

## 8.5 Pulse

`[DECIDED]`

Separate two Pulse concerns:

### CLI-owned

- metric definition/configuration discovery,
- retrieval,
- canonical JSON materialization,
- human-readable view generation,
- deterministic create/update/delete/following operations where current released API support is verified.

### MCP-owned

- querying metric values,
- analytical Pulse reads,
- insights.

Implementation should use the existing local `juju-local` Pulse work as a behavioral/API reference while explicitly avoiding its Tableau Next portions.

## 8.6 Workbook modification

`[DECIDED]`

TADX owns workbook lifecycle pull/publish.

Workbook authoring/modification belongs to Tableau Desktop/Desktop MCP or other supported Tableau authoring surfaces.

Do not add custom workbook mutation logic to TADX merely because an XML transformation is technically possible.

## 8.7 Users, groups, and membership

`[DECIDED]`

This is narrow REST-backed administration.

Preferred vocabulary:

- list,
- get,
- create,
- update,
- delete.

Group membership is handled through group update semantics unless implementation evidence proves a separate verb materially better.

## 8.8 Permissions

`[DECIDED]`

Permissions are read/inspect only in V1.

Reuse the existing Go `tabget` inventory/permissions behavior aggressively where appropriate.

No independent permission inference engine.

No permission mutation.

## 8.9 Dependency management

`[DECIDED]`

Do not build a generic dependency-management subsystem.

TADX does not own:

- transitive dependency discovery for project migration,
- generic cycle handling,
- dependency graph serialization,
- automatic acquisition of external dependencies.

A resource's own native references remain part of that resource and must be preserved for fidelity.

The user/agent is responsible for understanding higher-level dependencies unless a later specific capability justifies a dedicated deterministic primitive.

## 8.10 Output model

`[DECIDED]`

Default output is compact TOON.

`--full` returns expanded/untruncated TOON where applicable.

`--raw` returns the underlying raw payload/body only when explicitly requested.

YAML is not a CLI output format.

TADX conforms to the upstream TOON specification rather than creating a private dialect.

JSON interoperability uses the standard upstream TOON tooling/specification. TADX does not need a separate custom `format` or conversion command domain.

## 8.11 Pagination

`[DECIDED]`

Agent-facing list/search results are bounded.

Use explicit continuation/page metadata.

Exception:

- catalog hydration may process the complete remote site inventory internally because it is writing to the local catalog/cache and not returning the entire inventory to the LLM.

## 8.12 Partial outcomes

`[DECIDED]`

Single-resource atomic operations succeed or fail.

Partial result structures are used only when a deterministic operation legitimately contains independently meaningful substeps.

Project/package operations report per-resource success/failure before stopping.

## 8.13 Exit codes

`[DECIDED]`

```text
0 = success, including successful no-op
1 = operation/runtime failure
2 = usage error
```

Detailed error type remains structured data rather than more numeric exit codes.

## 8.14 Error contract

Errors should include, where available:

- stable string identifier,
- operation,
- selector/resource identity,
- environment/site,
- summary,
- upstream cause,
- retryability,
- deterministic corrective action,
- validation details,
- Tableau request/job ID.

Fail loudly and predictably.

Do not suggest fuzzy guesses.

## 8.15 Pre-1.0 compatibility

`[DECIDED]`

No formal compatibility guarantee before 1.0.

Prefer additive evolution.

Avoid gratuitous breaking changes.

Document deliberate breaking changes.

---

# 9. Capability surface

## 9.1 Top-level CLI spine

`[DECIDED]`

```text
tadx
  env
  auth
  capability
  catalog
  workspace
  content
  pulse
  admin
  doctor
```

## 9.2 Baseline command shape

The exact API-to-command mapping is implementation work, but command designers must stay within the approved vocabulary unless a new verb clearly represents distinct semantics.

```text
tadx env
  list
  get
  add
  update
  remove
  default

tadx auth
  check
  status

tadx capability
  list
  get

tadx catalog
  refresh
  search
  get
  status

tadx workspace
  create
  list
  status
  move
  clean

tadx content
  search
  get
  pull
  publish
  workbook ...
  datasource ...
  flow ...
  project ...

tadx pulse
  definition ...
  metric ...

tadx admin
  user ...
  group ...

tadx doctor
```

`pack` and `unpack` are not V1 commands.

## 9.3 Command-admission guidance

Do not add a verb because an endpoint exists.

Do add a verb when:

- it is common across many workflows,
- it closes a repeated raw-API gap,
- it strongly reduces model context/tool switching,
- it is a deterministic local/workspace primitive,
- it provides meaningful safety/normalization over Tableau's raw API.

---

# 10. Testing and quality

## 10.1 Quality priorities

`[DECIDED]`

1. Accuracy.
2. Token/context efficiency.
3. Latency.

Subject to:

- safety,
- determinism,
- reliability,
- maintainability,
- compatibility,
- security,
- human readability.

## 10.2 Test-first implementation

`[DECIDED]`

Behavioral tests are written before the feature implementation.

Important capability slices start from the expected externally visible behavior and failure cases.

Coding agents should not implement a capability first and invent its tests afterward.

## 10.3 Test strategy

Use:

- unit tests for pure deterministic logic,
- contract tests for normalized API/output behavior,
- end-to-end tests where they provide important evidence,
- fuzz testing for parsers, selectors, config, redaction, artifact parsing, and TOON boundaries,
- golden fixtures for stable CLI rendering.

The dedicated Tableau Cloud environment is available for API verification and end-to-end testing.

`[DECIDED]` A successful live Tableau test run is **not** a mandatory tagged-release gate.

Live tests remain a valuable development/verification tool, but release qualification does not depend on live credentials being available.

## 10.4 Release gate

V1 release qualification is:

- required unit/contract tests pass,
- supported-platform builds pass,
- packaging validation passes,
- required generated artifacts are present,
- SHA-256 checksums are produced.

No additional certification process.

## 10.5 Evaluation benchmarks

Accuracy, token, and end-to-end latency benchmarking are product-owner work outside the architecture handoff.

They are not an implementation blocker or release gate.

Future results may be used to tune capability admission and overlap decisions.

---

# 11. Existing code and implementation references

## 11.1 Reuse policy

`[DECIDED]`

Code from the user's existing applications may be lifted directly.

Every reused component is still reviewed for:

- code quality,
- latency,
- simplicity,
- maintainability,
- fit with the modular-monolith design,
- unnecessary UI/runtime coupling.

Behavior may be ported instead of code when that produces a cleaner TADX implementation.

## 11.2 `tabget`

Primary trusted Go foundation.

Use aggressively as the starting point for:

- PAT auth/transport,
- site inventory,
- permissions discovery,
- catalog hydration,
- collectors,
- normalization,
- SQLite cache/output patterns,
- throttling/pagination behavior where suitable.

`tabget` is the existing implementation with the strongest confidence level.

## 11.3 Pulse reference

Use the existing `~projects/tableau/juju-local` Pulse implementation as a behavioral/API reference.

Be careful to distinguish the Pulse implementation from the unrelated/similar Tableau Next application in that repository.

The repository also contains a Markdown reference for the Pulse metric-create API flow.

## 11.4 Datasource metadata enrichment reference

Use the existing metadata enrichment application as the reference for:

- field metadata,
- ID translation,
- released field-description write-back behavior,
- VDS/statistics behavior where relevant.

Port domain logic to Go rather than application UI/queue architecture.

## 11.5 TDS datasource API reference

The existing datasource SDK/work-copy code is a fast-follow reference.

Do not let pre-release TDS work-copy APIs leak into V1 committed behavior.

## 11.6 Other Tableau SDKs/tools

TSC, Document API, and Migration SDK may be studied for behavior and edge cases.

They are not required V1 dependencies.

---

# 12. Risks and mitigations

## 12.1 Workflow-engine creep

**Risk:** deterministic pseudo-primitives grow into hidden reasoning workflows.

**Mitigation:** if a meaningful decision exists in the middle, keep the procedure in a skill/agent.

## 12.2 MCP/CLI parity creep

**Risk:** intentional overlap becomes duplicate platform coverage.

**Mitigation:** overlap only for high-frequency lifecycle primitives or clear gaps; keep analytics/VDS MCP-primary.

## 12.3 Local data loss

**Risk:** re-pull destroys local edits.

**Mitigation:** baseline fingerprints and explicit `--overwrite` for dirty artifacts.

## 12.4 Identity ambiguity

**Risk:** names/paths collide or change.

**Mitigation:** Tableau IDs authoritative; ambiguous selectors fail.

## 12.5 Mutation safety drift

**Risk:** discovery enablement or `--force` becomes a hidden authorization bypass.

**Mitigation:** discovery gating only; `--apply` invariant; `--force` capability-specific.

## 12.6 Secret leakage

**Risk:** TADX outputs/logs/artifacts expose credentials it handles.

**Mitigation:** env/.env-only secrets, mandatory redaction, no debug secret escape hatch.

TADX does not attempt to police arbitrary user-authored Tableau content.

## 12.7 Tableau API fragmentation

**Risk:** capabilities differ across products/versions.

**Mitigation:** resource adapters, API verification, runtime failures, no fake global compatibility gate.

## 12.8 Catalog staleness

**Risk:** cached inventory differs from Tableau.

**Mitigation:** generation timestamps, 12-hour stale warning, explicit refresh, remote Tableau remains authoritative.

## 12.9 Partial shallow-project delivery

**Risk:** a project operation changes several resources and then fails.

**Mitigation:** stop on failure, report exact successes/failure, no rollback/resume promise in V1.

## 12.10 Pre-release TDS API instability

**Risk:** work-copy APIs change and leave unmanageable remote drafts.

**Mitigation:** do not ship the TDS work-copy feature until the required work-copy management/listing API is released and verified.

## 12.11 Composable datasource API complexity

**Risk:** round-trip/publish semantics can lose published-parent relationships.

**Mitigation:** make composition fidelity a first-class V1 acceptance requirement and verify against current public APIs before implementation completion.

## 12.12 Existing-code coupling

**Risk:** prototype UI/runtime assumptions enter the core.

**Mitigation:** direct reuse only after review; prefer clean internal interfaces.

---

# 13. Deferred roadmap

## 13.1 Pack / unpack

`[DEFERRED]` After V1.

Target direction:

- `unpack` extracts `.twbx` / `.tdsx` contents into an expanded editable local representation.
- `pack` deterministically rebuilds the package.
- Hyper data may be converted to/from CSV where practical.

An existing application already demonstrates the desired product behavior.

## 13.2 Hyper integration

`[DEFERRED]` After V1 with `pack` / `unpack`.

Decision rule:

1. Investigate a native Go integration/binding.
2. If the native binding and cross-platform packaging are medium effort, build it.
3. If disproportionately difficult, keep Hyper conversion optional and require Python + Tableau Hyper API for that subfeature only.
4. Core TADX remains usable without Python.

## 13.3 TDS work-copy editing

`[DEFERRED]` Fast follow after the relevant API matures.

Current known behavior:

- remote work copies have their own LUIDs,
- work copies disappear after roughly 24 hours,
- the current API does not provide a reliable way to query/list created work copies,
- the Tableau team has been informed and is expected to improve this.

TADX waits for the work-copy management API before shipping this feature.

When admitted:

- track TADX-created work copies,
- use hidden harness state only as needed,
- surface orphan/health information,
- provide deterministic create/read/mutate/publish primitives,
- keep API details isolated behind adapters.

## 13.4 VDS against work copies

Current factual direction:

- a TDS work copy can be queried through VDS,
- verify how well existing MCP query tooling operates against a work-copy LUID before designing a reference skill.

No need to test this before V1 because the parent TDS work-copy feature is deferred.

## 13.5 Work-copy diff and impact

After TDS work copies are admitted:

- factual stable-ID diff may be a deterministic primitive,
- factual affected-resource emission may be a deterministic primitive if Metadata API supports it,
- semantic classification such as breaking/visible/safe remains skill/agent reasoning.

## 13.6 Lineage

`[DEFERRED]` Not V1.

Reconsider only if a concrete lifecycle/change workflow proves a dedicated bounded traversal primitive materially valuable and MCP remains insufficient.

---

# 14. Glossary

## Agent-native

Designed for deterministic discovery, invocation, compact results, explicit failure handling, and low-context coding-agent use while remaining understandable to humans.

## AXI-style CLI

A discoverable, composable, deterministic, progressively disclosed, structured, context-efficient CLI designed for agent ergonomics.

## Artifact

A local TADX representation of one selected Tableau resource containing canonical payload, metadata/provenance, and a human-readable view.

## Baseline fingerprint

A deterministic hash recorded at pull time and compared to current managed local content to detect local modification.

## Capability registry

The executable source of truth for CLI capability metadata, ownership, discovery, safety, and command wiring.

## Catalog

A point-in-time normalized local cache of Tableau discovery information. It is not authoritative deployed state and is not the set of resources present in a workspace.

## Composable datasource

A Tableau published datasource whose data model composes/references other published datasource parents. TADX treats composition fidelity and authoring as V1 datasource lifecycle concerns.

## Deployed state

Authoritative Tableau state.

## Environment

A named Tableau Cloud/Server endpoint/site configuration with non-secret settings and references to secret environment variables.

## MCP-preferred capability

A capability that TADX can describe in discovery but which the agent should normally execute through Tableau MCP.

## Mutation discovery gating

The mechanism controlled by `TADX_ENABLE_MUTATIONS` that changes which mutation capabilities appear in agent-facing discovery/help. It is not execution authorization.

## Project

A Tableau hierarchical container. TADX V1 project lifecycle is shallow.

## Pull

Retrieve one remote Tableau resource into a local workspace and create its artifact/provenance/baseline/human-readable representation.

## Publish

Send one local resource/artifact to an explicitly selected Tableau environment/destination. Preview is default; `--apply` executes.

## Skill

A reusable agent procedure that composes CLI/MCP capabilities and can contain judgment, policy, branching, or review.

## TOON

The upstream compact structured text format used by default for TADX CLI output.

## Workspace

A named local artifact-oriented working area. It may contain resources from multiple environments. Each artifact records its own source origin, which is the default publish target for that artifact.

---

# Appendix A. V1 capability disposition

| Capability | V1 disposition | Owner |
|---|---|---|
| Capability discovery | Ship | TADX |
| Environment profiles | Ship | TADX |
| PAT auth | Ship | TADX |
| Catalog/site hydration | Ship | TADX |
| Content lifecycle search | Ship | TADX |
| Workbook pull/publish | Ship | TADX |
| Workbook authoring/modification | Do not build | Desktop MCP/Tableau |
| Datasource pull/publish | Ship | TADX |
| Composable datasource round-trip | Ship | TADX |
| Composable datasource authoring | Ship | TADX |
| Datasource field metadata read | Ship | TADX |
| Datasource field-description write-back | Ship after released-API verification | TADX |
| Flow pull/publish | Ship where released API supports it | TADX |
| Shallow project lifecycle | Ship | TADX |
| Recursive project migration | Deferred | TADX later |
| Pulse definition/config retrieval | Ship | TADX |
| Pulse definition/config artifacts | Ship | TADX |
| Pulse deterministic lifecycle mutations | Ship where current API supports them | TADX |
| Pulse values/insights query | Delegate | MCP |
| VDS analytics | Delegate | MCP |
| Users | Ship narrow lifecycle | TADX |
| Groups | Ship narrow lifecycle | TADX |
| Group membership | Ship via group update semantics | TADX |
| Permission inspection | Ship | TADX |
| Permission mutation | Deferred | TADX later |
| Generic lineage traversal | Deferred | Re-evaluate later |
| `pack` / `unpack` | Deferred | TADX later |
| Hyper <-> CSV | Deferred | TADX later/optional Python fallback |
| TDS work-copy lifecycle | Deferred fast follow | TADX later |
| TDS work-copy diff | Deferred | TADX later |
| TDS impact emitter | Deferred | TADX later |
| Tableau Next | Out of scope | N/A |

---

# Appendix B. Implementation verification backlog

These are implementation tasks, not architecture questions.

No user decision is required unless evidence contradicts an architecture decision.

## B.1 Tableau capability verification

- Verify current official MCP capability inventory.
- Verify the API used for lifecycle-oriented content search.
- Verify workbook download/publish contracts.
- Verify datasource download/publish contracts.
- Verify composed datasource parent/reference publish requirements.
- Verify released API for composable datasource authoring.
- Verify flow download/publish behavior.
- Verify shallow project lifecycle API calls.
- Verify field metadata/ID mapping/write-back API behavior.
- Verify Pulse definition/metric lifecycle APIs using `juju-local` and current official docs.
- Verify user/group/group-membership REST endpoints.
- Verify permission inspection behavior using `tabget`.
- Record runtime differences as capability notes, not as a global version gate.

## B.2 Code reuse review

- Start with `tabget` for REST/auth/catalog/permissions where possible.
- Review each reused component for quality, latency, simplicity, and unnecessary coupling.
- Port behavior rather than scaffolding from non-Go apps.
- Review external dependency licenses before redistribution.

## B.3 Output

- Select an upstream-conformant Go TOON implementation.
- Pin/test a known upstream TOON version per release.
- Add conformance tests.
- Add golden output fixtures.

## B.4 Config/state

- Implement `config.yaml` under `os.UserConfigDir()/tadx`.
- Implement workspace `tadx.yaml`.
- Implement `.tadx/`.
- Implement `metadata.json` and `view.md`.
- Implement default workspace resolution.

## B.5 Logging

- Implement `TADX_LOG_LEVEL`.
- Keep logging off by default.
- Redact all secrets.
- Include correlation IDs.
- Include Tableau request/job IDs where available.

---

# Appendix C. Engineering completion rules

A capability is ready to merge when:

1. Its capability-registry metadata exists.
2. Ownership is correct.
3. Tests describing the expected behavior were authored before implementation.
4. Selector ambiguity behavior is tested.
5. Human and TOON output are tested.
6. Error/exit behavior is tested.
7. Secret redaction is tested where applicable.
8. Mutation discovery behavior is tested if mutating.
9. Preview/`--apply` behavior is tested if consequential.
10. Relevant local artifact/provenance behavior is tested.
11. Reused prototype code has been reviewed for fit.
12. API-specific behavior has been verified against current evidence.
13. Documentation/discovery is updated.

---

# Appendix D. Decision ledger

## ADR-001 — Go modular monolith

Use Go, one module, one primary binary, thin Cobra commands, reusable internal packages.

## ADR-002 — MCP and CLI are peer surfaces

The agent chooses. TADX never proxies MCP.

## ADR-003 — Tools / workflows / reasoning / presentation separation

```text
TOOLS        -> CLI / MCP
WORKFLOWS    -> Skills
REASONING    -> Agent
PRESENTATION -> VS Code
```

## ADR-004 — AXI controls CLI behavior

Cobra is plumbing; AXI is the UX/agent contract.

## ADR-005 — High-frequency lifecycle overlap is intentional

Get/list/search/publish/auth-style primitives may overlap MCP because they recur across many workflows. VDS remains MCP-primary.

## ADR-006 — Local artifact-oriented workspaces

Visible artifacts/provenance; minimal hidden state; multi-environment; no persistent publish target.

## ADR-007 — Deterministic workspace resolution

Explicit -> cwd -> environment default -> general default -> fail.

## ADR-008 — Tableau IDs are authoritative

Names and paths are selectors; ambiguity fails.

## ADR-009 — Re-pull protects local edits

Baseline fingerprint; unchanged repull refreshes with warning; dirty repull requires `--overwrite`.

## ADR-010 — Consequential writes use preview/apply

Preview by default; `--apply` mutates; no second confirmation; production is not special-cased.

## ADR-011 — Mutation gating is discovery-only

`TADX_ENABLE_MUTATIONS=1` persistently exposes mutation capabilities; it is not execution authorization.

## ADR-012 — Minimal exit codes

0 success/no-op, 1 operation/runtime error, 2 usage error.

## ADR-013 — TOON is default output

Compact TOON by default, `--full`, explicit `--raw`, upstream TOON conformance.

## ADR-014 — No telemetry

No phone home. Local redacted diagnostics only.

## ADR-015 — Pre-1.0 additive best effort

No formal compatibility guarantee; prefer additive evolution.

## ADR-016 — Platform release matrix

Windows AMD64; macOS AMD64/ARM64; Linux AMD64.

## ADR-017 — No global Tableau version gate

Attempt admitted capabilities optimistically; surface upstream failure.

## ADR-018 — GitHub Releases + SHA-256

No V1 signing or auto-update.

## ADR-019 — No special offline/air-gap mode

Local commands naturally work offline; remote commands require network.

## ADR-020 — No generic dependency engine

Preserve resource-native references; higher-level dependency reasoning stays with user/agent.

## ADR-021 — Shallow project stop-on-failure

Direct content only; stop on failure; report success/failure; no rollback/resume.

## ADR-022 — Composable datasources are V1

Preserve and author published-datasource composition relationships through deterministic CLI capabilities.

## ADR-023 — Pulse definition lifecycle is CLI-owned

Definition/config retrieval and materialization belong in TADX; metric values/insights stay MCP-primary.

## ADR-024 — First-party skills are selective

Ship skills only for common multi-command orchestration patterns not intentionally abstracted into deterministic CLI commands.

## ADR-025 — Test-first capability implementation

Tests defining externally visible behavior are written before feature implementation. Live Tableau execution is verification evidence, not a mandatory release gate.

## ADR-026 — No generic secret policing

TADX protects secrets it handles; it does not act as a DLP scanner over arbitrary user-authored content.

## ADR-027 — Async polling belongs in CLI where deterministic

The CLI waits/polls internally for bounded asynchronous Tableau jobs rather than making the LLM repeatedly poll.

## ADR-028 — Reuse user-owned code freely but review it

Local/user-owned repositories are reusable. `tabget` is the preferred trusted foundation; every lift still gets quality/latency/simplicity review.

## ADR-029 — `pack` / `unpack` move after V1

Keep V1 focused. Hyper conversion moves with this fast-follow capability.

## ADR-030 — TDS work-copy editing waits for API maturity

Do not ship until work-copy management can be implemented safely with the released API.

---

## ADR-031 — MCP is the data analyst; CLI owns lifecycle

MCP owns analysis, actual file/datasource data querying, and Pulse metric values/insights. TADX owns deterministic Tableau development/lifecycle operations, including Pulse definitions/configuration.

## ADR-032 — Concurrent local races are caller responsibility

V1 provides no workspace/state locking. Concurrent processes racing on the same local state are the caller's responsibility.

## ADR-033 — No special enterprise networking requirement

V1 adds no product-specific proxy/custom-CA subsystem beyond ordinary platform/Go HTTP behavior.

## ADR-034 — Agent routing starts in AGENTS.md

`AGENTS.md` provides high-level ownership guidance; capability discovery provides focused detail and points agents to MCP for analytical/querying tools.

## ADR-035 — Logging is environment-variable opt-in

Persistent local logging is off by default and enabled with `TADX_LOG_LEVEL`.

## ADR-036 — External Tableau tooling may be admitted after V1

Tableau Desktop MCP is an expected peer integration; Hyper API may be used later if equivalent Go implementation is impractical.


# Appendix E. Handoff status

**Architecture status:** COMPLETE.

Coding agents may begin implementation without another architecture interview.

They should not ask the product owner to decide ordinary package structure, API wiring, exact struct definitions, cache schemas, test library selection, or other implementation mechanics already delegated in this document.

Escalate only if implementation evidence shows that a confirmed architecture decision is impossible, unsafe, or materially inconsistent with a released Tableau capability.
