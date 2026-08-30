# TADX V1 Capability Contract

**Document status:** Consolidated contract — architecture locked; API validation incomplete  
**Prepared:** 2026-08-29  
**Product / binary:** `tadx`  
**Canonical scope:** Public and discoverable V1 operations, shared execution invariants, CLI implementation evidence, boundaries, and implementation-blocking verification.

## 1. Status, sources, and validation levels

### 1.1 Source register and precedence

| Label | Attached source | Version / date | Authority in this contract |
| --- | --- | --- | --- |
| **A1** | `tableau-agent-development-harness-arc42-locked.md` | Architecture complete; last reconciled 2026-08-29 | Authoritative for product behavior, ownership, safety, V1 scope, and architecture boundaries. |
| **C1** | `tadx-v1-capability-matrix-current.md` | Current draft; prepared 2026-08-29; validation pending | API research and implementation evidence. It is not architecture authority. |
| **S1** | `Pasted markdown.md` | Consolidation review supplied 2026-08-29 | Authoritative for the requested document shape and structural corrections applied here. |

Precedence is **A1**, then **S1** for consolidation structure, then **C1** for API facts. This contract does not silently replace an A1 decision with an endpoint-driven inference.

C1 refers to `R1`, `M1`, `M2`, `V1`, `X1`, `X2`, and `X3`, but those underlying documents, exact URLs, OpenAPI artifacts, operation IDs, and captured versions are not attached. Consequently:

- C1-derived remote API facts are **docs-only** evidence in this contract.
- No remote capability is marked **live-verified**.
- A remote capability cannot move to **verified** until its exact official source artifact and required contract/live tests are attached to the implementation record.
- This consolidation performs no new web or live-site verification.

### 1.2 Validation levels

| Level | Meaning |
| --- | --- |
| **Architecture-locked** | A1 fixes the user-visible behavior or boundary. Implementation must preserve it. |
| **Local contract** | Deterministic local behavior is fixed by A1 and does not depend on a Tableau API fact. |
| **Docs-only** | C1 states an upstream operation or behavior, but the exact official source capture and live evidence are not present in the attachment set. |
| **Live-verified** | Exact official source plus a captured successful/negative contract test on a named supported deployment. No capability currently has this level. |
| **Blocked** | A V1 commitment cannot be completed safely until the named verification gate in §6 closes. |
| **Delegated** | Discoverable through the registry, but executed by MCP, Tableau/Desktop, or an agent/skill rather than TADX CLI. |
| **Deferred / rejected / out of scope** | Not a V1 executable registry operation. The boundary is recorded in §5. |

A `Blocked` row is still a V1 product commitment; it is not implementation-ready. A `Ship` row with `Docs-only` evidence is admitted but still requires ordinary source capture and contract testing before merge.

### 1.3 Canonicality rule

Section 2 is the sole hand-authored operation inventory. Generated help, ownership views, public command trees, and capability documentation must be derived from the executable registry represented by these rows. Internal adapter calls do not receive public capability rows merely because an endpoint exists.

## 2. Canonical V1 capability registry

The following tables are one logical registry split only for readability. Every row represents exactly one public CLI operation or one delegated operation that `capability get` must be able to describe.

Registry totals: **71 rows** — **66 CLI-owned** and **5 delegated**; **52 Ship**, **14 Blocked**, and **5 Delegated**.

Field rules:

- `Local write`, `Remote mutation`, and `Requires --apply` are independent. Local writes and pulls are not hidden by mutation discovery gating.
- `TADX_ENABLE_MUTATIONS=1` affects discovery only for rows whose `Remote mutation` is `Yes`.
- `Artifact effect` concerns managed Tableau resource artifacts, not config/catalog housekeeping.
- Deferred, rejected, and out-of-scope boundaries are not duplicated here; they are listed once in §5.

### 2.1 Foundation, discovery, and local state

| Capability ID | Public command or delegated surface | Resource and user outcome | Type | V1 status | Owner | MCP overlap | Selectors | Products / availability | Local write | Remote mutation | Requires `--apply` | Safety / guard | Artifact effect | Upstream operation | Evidence | Validation / blocker |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| env.profile.list | `tadx env list` | List named non-secret environment profiles. | Find | Ship | CLI | — | Optional config path | Local / all | No | No | No | Secret redaction | None | `config.yaml` under `os.UserConfigDir()/tadx` | A1 §§1.5, 7.2–7.4, 9.2; C1 §2.1 | Architecture-locked local contract |
| env.profile.get | `tadx env get` | Inspect one resolved non-secret environment profile. | Inspect | Ship | CLI | — | Environment alias | Local / all | No | No | No | Secret redaction; exact alias | None | Read local `config.yaml` | A1 §§5.3, 7.2–7.4; C1 §2.1 | Architecture-locked local contract |
| env.profile.add | `tadx env add` | Add one named environment profile containing secret references, not secret values. | Change | Ship | CLI | — | New environment alias | Local / all | Yes | No | No | Schema validation; collision guard; atomic file replacement | None | Write local `config.yaml` | A1 §§7.2–7.4, ADR-010; C1 §2.1 | Architecture-locked local contract |
| env.profile.update | `tadx env update` | Update explicit fields of one environment profile. | Change | Ship | CLI | — | Environment alias | Local / all | Yes | No | No | Exact alias; secret redaction; atomic file replacement | None | Write local `config.yaml` | A1 §§7.2–7.4; C1 §2.1 | Architecture-locked local contract |
| env.profile.remove | `tadx env remove` | Remove one named environment profile. | Change | Ship | CLI | — | Environment alias | Local / all | Yes | No | No | Exact alias; default-reference guard | None | Write local `config.yaml` | A1 §§7.2–7.4; C1 §2.1 | Architecture-locked local contract |
| env.profile.set-default | `tadx env default` | Set the default read environment. | Change | Ship | CLI | — | Environment alias | Local / all | Yes | No | No | Alias must exist; no secret output | None | Write `default_environment` in local `config.yaml` | A1 §§1.5, 7.2–7.3, 9.2; S1 “one capability row per public operation” | Architecture-locked local contract |
| auth.check | `tadx auth check` | Resolve PAT references, sign in, and verify the selected Tableau site. | Inspect | Ship | CLI | — | Environment alias; site content URL | Cloud / Server | No | No | No | Never persist or echo PAT/token | None | `POST /api/{version}/auth/signin`; optional signout | A1 §§5.4, 7.3–7.4; C1 §§2.1, 5.1 | Docs-only; exact official source capture pending |
| auth.status | `tadx auth status` | Report resolved auth configuration and PAT-reference presence without revealing values. | Inspect | Ship | CLI | — | Optional environment alias | Local / all | No | No | No | Does not claim remote validity; secret redaction | None | Local config/environment resolution | A1 §§1.5, 5.4, 9.2; S1 notes missing distinct row | Architecture-locked; remote-validity semantics intentionally belong to `auth check` |
| capability.list | `tadx capability list` | Return a bounded inventory of discoverable operations and ownership. | Find | Ship | CLI | — | Domain/resource/owner/product/mutation filters | Local / all | No | No | No | Mutation discovery obeys `TADX_ENABLE_MUTATIONS` | None | Executable capability registry | A1 §§1.5, 3.3, 5.2, 6.6; C1 §2.1 | Architecture-locked local contract |
| capability.get | `tadx capability get <id>` | Return focused execution, ownership, selector, safety, and availability guidance for one capability. | Inspect | Ship | CLI | — | Stable capability ID | Local / all | No | No | No | Exact ID; includes delegated surfaces | None | Executable capability registry | A1 §§3.3, 5.2; C1 §2.1 | Architecture-locked local contract |
| catalog.refresh | `tadx catalog refresh` | Hydrate and replace one normalized site inventory generation. | Inspect | Ship | CLI | — | Environment/site; admitted scopes | Cloud / Server | Yes | No | No | Incomplete generations never become current | None | REST list/read endpoints; focused Metadata API reads | A1 §§5.9, 8.11, 11.2; C1 §§2.1, 5.3 | Docs-only remote facts; local generation contract architecture-derived |
| catalog.search | `tadx catalog search` | Search cached inventory with bounded continuation and staleness metadata. | Find | Ship | CLI | — | Text, kind, project path, owner, environment/site, ID | Local / all | No | No | No | Cache is not authoritative for writes | None | Local normalized catalog index | A1 §§5.9, 8.11; C1 §2.1 | Architecture-locked local contract |
| catalog.get | `tadx catalog get` | Inspect one cached resource record by authoritative ID or exact selector. | Inspect | Ship | CLI | — | Tableau LUID or exact name/project path | Local / all | No | No | No | Ambiguity fails; report generation/staleness | None | Local normalized catalog index | A1 §§5.7, 5.9, 8.1; C1 §2.1 | Architecture-locked local contract |
| catalog.status | `tadx catalog status` | Report generation age, completeness, source, and stale state. | Inspect | Ship | CLI | — | Optional environment/site | Local / all | No | No | No | 12-hour stale warning; stale is not invalid | None | Local catalog generation metadata | A1 §§5.9, 12.8; C1 §2.1 | Architecture-locked local contract |
| workspace.create | `tadx workspace create` | Create an explicit named workspace with `tadx.yaml`, `artifacts/`, and `.tadx/`. | Change | Ship | CLI | — | Path/name | Local / all | Yes | No | No | Collision and path-boundary checks; no implicit creation by pull | None | Local filesystem | A1 §§6.1, 7.5–7.6, 9.2; C1 §2.1 | Architecture-locked local contract |
| workspace.list | `tadx workspace list` | List configured or boundedly discoverable workspaces. | Find | Ship | CLI | — | Configured roots/current repository | Local / all | No | No | No | No unbounded filesystem scan | None | Local config/filesystem | A1 §§6.1, 7.2, 9.2; C1 §2.1 | Architecture-locked local contract |
| workspace.status | `tadx workspace status` | Report effective workspace, artifact state, provenance, and dirty/missing status. | Inspect | Ship | CLI | — | `--workspace` or deterministic resolution chain | Local / all | No | No | No | No locking; concurrent races are caller responsibility | Read | Fingerprint managed local content | A1 §§6.1, 6.3–6.4, 8.2, ADR-032; C1 §2.1 | Architecture-locked; C1 lock suggestion rejected |
| workspace.move | `tadx workspace move` | Move one local artifact without changing Tableau identity. | Change | Ship | CLI | — | Artifact path or Tableau ID; destination path | Local / all | Yes | No | No | Collision/path-boundary guard; no remote move | Update | Local filesystem move plus metadata validation | A1 §§7.5, 8.1 and V1 exclusions; C1 §2.1 | Architecture-locked local contract |
| workspace.clean | `tadx workspace clean` | Remove explicitly selected disposable local state while preserving canonical artifacts by default. | Change | Ship | CLI | — | Workspace and cleanup class | Local / all | Yes | No | No | Path-boundary guard; managed payloads preserved by default | None | Local `.tadx/` and temporary state | A1 §§5.8, 7.5–7.6, 9.2; C1 §2.1 | Architecture-locked local contract |
| content.search | `tadx content search` | Search current remote content for lifecycle selection. | Find | Ship | CLI | `search-content` | Terms, type, owner, project, modified time; exact ID after selection | Cloud / Server supporting content search | No | No | No | Bounded output; relevance result cannot directly target a write | None | `GET /api/-/search` | A1 §§1.5, 3.2–3.4, 8.11; C1 §§2.2, 5.2 | Docs-only; exact official source capture pending |
| content.get | `tadx content get` | Resolve and inspect one supported content item through its resource adapter. | Inspect | Ship | CLI | Resource-specific get tools | Resource kind plus LUID or exact name/project path | Cloud / Server; Pulse uses the separate `pulse` domain | No | No | No | Exact resolution; ambiguity fails | None | Resource-specific exact get or exact filtered list | A1 §§3.2, 5.6–5.7, 6.2; C1 §2.2 | Docs-only |
| doctor.run | `tadx doctor` | Diagnose config, PAT presence/validity, connectivity, MCP availability, catalog, workspace, and logging context without mutation. | Inspect | Ship | CLI | — | Optional environment/workspace scopes | Cloud / Server / Desktop / Pulse as configured | No | No | No | Redacted checks; no persistent logging unless `TADX_LOG_LEVEL` is set | None | Local validators plus read-only auth/connectivity probes | A1 §§1.5, 5.12, 7.7, 9.1; C1 §2.1 | Architecture-locked; exact MCP probe is ordinary implementation work |

### 2.2 Workbook, datasource, flow, and project lifecycle

| Capability ID | Public command or delegated surface | Resource and user outcome | Type | V1 status | Owner | MCP overlap | Selectors | Products / availability | Local write | Remote mutation | Requires `--apply` | Safety / guard | Artifact effect | Upstream operation | Evidence | Validation / blocker |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| workbook.list | `tadx content workbook list` | List remote workbooks with bounded lifecycle metadata. | Find | Ship | CLI | `list-workbooks` | Environment/site; project/owner/filter | Cloud / Server | No | No | No | Bounded continuation | None | `GET /api/{version}/sites/{site-id}/workbooks` | A1 §§1.5, 3.2; C1 §§2.3, 5.4 | Docs-only |
| workbook.get | `tadx content workbook get` | Inspect one authoritative workbook and lifecycle metadata. | Inspect | Ship | CLI | `get-workbook` | Workbook LUID or exact name/project path | Cloud / Server | No | No | No | Exact resolution; ambiguity fails | None | `GET /api/{version}/sites/{site-id}/workbooks/{workbook-id}` | A1 §§3.2, 8.1; C1 §§2.3, 5.4 | Docs-only |
| workbook.pull | `tadx content workbook pull` | Download one workbook into a provenance-bearing local artifact. | Deliver | Ship | CLI | `download-workbook` | Workbook LUID/exact path; workspace | Cloud / Server | Yes | No | No | Dirty re-pull requires `--overwrite` | Create / update | `GET /api/{version}/sites/{site-id}/workbooks/{workbook-id}/content` | A1 §§6.3–6.4, 8.2, 8.6; C1 §§2.3, 5.4, 5.13 | Docs-only remote API; artifact contract architecture-locked |
| workbook.publish | `tadx content workbook publish` | Preview and publish one local workbook to an explicit target. | Deliver | Ship | CLI | — | Workspace artifact; explicit environment/site/project; optional exact existing workbook | Cloud / Server; TWB validation API only on API 3.29 / Tableau 2026.2+ per C1 | No | Yes | Yes | Explicit write environment; collision/overwrite explicit; no fuzzy target | Read / publish | `POST /api/{version}/sites/{site-id}/workbooks`; upload sessions; optional internal `validateWorkbook` for TWB | A1 §§5.11, 6.5–6.8, 8.6; C1 §§2.3, 5.4; S1 workbook-check correction | Docs-only; standalone `workbook check` not admitted |
| datasource.list | `tadx content datasource list` | List published datasources with bounded lifecycle metadata. | Find | Ship | CLI | `list-datasources` | Environment/site; project/owner/filter | Cloud / Server | No | No | No | Bounded continuation | None | `GET /api/{version}/sites/{site-id}/datasources` | A1 §§1.5, 3.2; C1 §§2.4, 5.5 | Docs-only |
| datasource.get | `tadx content datasource get` | Inspect one datasource, with bounded field/model/composition detail when requested. | Inspect | Ship | CLI | `list-datasources`; `get-datasource-metadata` | Datasource LUID or exact name/project path; bounded detail options | Cloud / Server; VDS metadata/model Server 2025.1+ per C1 | No | No | No | Record Metadata API permission mode and partial warnings | None | `GET .../datasources/{id}`; `POST /api/metadata/graphql`; optional VDS metadata/model | A1 §§8.3–8.4, 11.4; C1 §§2.4, 5.5, 5.12; S1 internal-capability correction | Docs-only; composed-parent ID mapping remains B2 |
| datasource.pull | `tadx content datasource pull` | Download one datasource while preserving native package and composition provenance. | Deliver | Blocked | CLI | — | Datasource LUID/exact path; workspace | Cloud / Server; composed round-trip requires Tableau 2026.2 behavior per C1 | Yes | No | No | Dirty re-pull requires `--overwrite`; no package-semantic loss | Create / update | `GET /api/{version}/sites/{site-id}/datasources/{datasource-id}/content` plus focused metadata reads | A1 §§6.3–6.4, 8.2–8.4, ADR-022; C1 §§2.4, 5.5, 5.13 | B2 blocks composed-datasource completion |
| datasource.composition.update | `tadx content datasource composition update` | Apply explicit immediate-parent composition changes to a local datasource artifact. | Change | Blocked | CLI | — | Local datasource artifact; exact immediate parent LUIDs/content URLs | Local; resulting publish requires Cloud/Server support for composable datasources | Yes | No | No | Never choose parents or relationships; deterministic no-op on equal normalized state | Update | Local `.tds`/`.tdsx` serialization proven by fixtures | A1 §§4.3, 8.3, ADR-022; C1 §2.4; S1 datasource-update split | B2: safe serialization and round-trip contract unverified |
| datasource.field-description.update | `tadx content datasource field update` | Preview and write an explicit description to one exact published-datasource field. | Change | Blocked | CLI | — | Datasource LUID plus stable exact field ID; explicit environment/site | Cloud / Server subject to released API/license verification | No | Yes | Yes | Never target by ambiguous name/caption; equal value is a no-op when authoritative pre-read exists | Read / publish | Unresolved released published-field write operation | A1 §§8.4, 11.4, Appendix A/B; C1 §§2.4, 5.5; S1 datasource-update split | B1: endpoint and authoritative field-ID mapping not established |
| datasource.publish | `tadx content datasource publish` | Preview and publish one local datasource, including explicit immediate-parent references for composed artifacts. | Deliver | Blocked | CLI | — | Workspace artifact; explicit environment/site/project; optional exact existing datasource; immediate parents for composed artifacts | Cloud / Server; composed path API 3.29 / Tableau 2026.2 per C1 | No | Yes | Yes | Overwrite/append/replace never inferred; explicit write target; stop on terminal failure | Read / publish | `POST /api/{version}/sites/{site-id}/datasources`; upload sessions; `parentDataSourceUrls` for composed path | A1 §§6.5–6.8, 8.3, ADR-022; C1 §§2.4, 5.5 | B2 blocks full composed-path contract; ordinary publish is docs-only |
| flow.list | `tadx content flow list` | List flows with bounded lifecycle metadata. | Find | Ship | CLI | `list-flows` | Environment/site; project/owner/filter | Cloud / Server with flow support; REST API 3.3+ per C1 | No | No | No | Bounded continuation | None | `GET /api/{version}/sites/{site-id}/flows` | A1 §§1.5, 3.4; C1 §§2.5, 5.6 | Docs-only |
| flow.get | `tadx content flow get` | Inspect one authoritative flow and its direct lifecycle metadata. | Inspect | Ship | CLI | `get-flow` | Flow LUID or exact name/project path | Cloud / Server with flow support | No | No | No | Exact resolution; ambiguity fails | None | `GET /api/{version}/sites/{site-id}/flows/{flow-id}` | A1 §§1.5, 3.4; C1 §§2.5, 5.6 | Docs-only |
| flow.pull | `tadx content flow pull` | Download one flow into a provenance-bearing local artifact. | Deliver | Ship | CLI | `get-flow` is metadata-only overlap | Flow LUID/exact path; workspace | Cloud / Server with flow support | Yes | No | No | Dirty re-pull requires `--overwrite` | Create / update | `GET /api/{version}/sites/{site-id}/flows/{flow-id}/content` | A1 §§6.3–6.4; C1 §§2.5, 5.6, 5.13 | Docs-only remote API; artifact contract architecture-locked |
| flow.publish | `tadx content flow publish` | Preview and publish one local TFL/TFLX to an explicit project. | Deliver | Ship | CLI | — | Workspace artifact; explicit environment/site/project; optional exact existing flow | Cloud / Server with flow support | No | Yes | Yes | Explicit write target; overwrite explicit | Read / publish | `POST /api/{version}/sites/{site-id}/flows`; upload sessions for large files | A1 §§6.5–6.8; C1 §§2.5, 5.6 | Docs-only |
| project.list | `tadx content project list` | List projects and their authoritative parent identity. | Find | Ship | CLI | `list-projects` | Environment/site; filters | Cloud / Server | No | No | No | Bounded continuation | None | `GET /api/{version}/sites/{site-id}/projects` | A1 §§1.5, 6.2, 6.10; C1 §§2.6, 5.7 | Docs-only |
| project.get | `tadx content project get` | Resolve and inspect one exact shallow project context. | Inspect | Ship | CLI | `list-projects` | Project LUID or exact slash-delimited path | Cloud / Server | No | No | No | Ambiguous path fails | None | Exact resolution from project list/filter response | A1 §§5.7, 6.2, 6.10; C1 §§2.6, 5.7 | Docs-only |
| project.create | `tadx content project create` | Preview and create one project, optionally under an explicit parent. | Change | Ship | CLI | — | Explicit environment/site; optional parent LUID/path | Cloud / Server | No | Yes | Yes | Same-name collision and ambiguous parent fail | None | `POST /api/{version}/sites/{site-id}/projects` | A1 §§6.5, 6.10; C1 §§2.6, 5.7 | Docs-only |
| project.update | `tadx content project update` | Preview and update bounded project metadata without generic hierarchy migration. | Change | Ship | CLI | — | Project LUID/exact path | Cloud / Server | No | Yes | Yes | Parent-tree move excluded; equal values no-op | None | `PUT /api/{version}/sites/{site-id}/projects/{project-id}` | A1 §§6.5, 6.10 and remote-move exclusion; C1 §§2.6, 5.7 | Docs-only |
| project.pull | `tadx content project pull` | Materialize one project and direct supported content only. | Deliver | Blocked | CLI | — | Exact project; workspace; direct content types | Cloud / Server | Yes | No | No | No child recursion; dirty guard per artifact; stop on first failure | Create / update | Project/direct workbook/datasource/flow list and pull operations | A1 §§6.10, 8.9, 8.12, ADR-021; C1 §§2.6, 5.7 | B4: exact direct-content enumeration contract pending |
| project.publish | `tadx content project publish` | Preview and deliver one shallow project package without hidden mapping decisions. | Deliver | Blocked | CLI | — | Project manifest/direct artifacts; explicit target mappings | Cloud / Server | No | Yes | Yes | Whole-plan preview; stop on first failure; no rollback/resume | Read / publish | Project create/update plus direct resource publish operations | A1 §§6.5–6.10, 8.9, ADR-021; C1 §§2.6, 5.7 | B4 blocks exact enumeration/mapping boundary contract |

### 2.3 Pulse lifecycle

| Capability ID | Public command or delegated surface | Resource and user outcome | Type | V1 status | Owner | MCP overlap | Selectors | Products / availability | Local write | Remote mutation | Requires `--apply` | Safety / guard | Artifact effect | Upstream operation | Evidence | Validation / blocker |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| pulse.definition.list | `tadx pulse definition list` | List Pulse metric definitions with bounded token continuation. | Find | Ship | CLI | `list-all-pulse-metric-definitions`; `list-pulse-metric-definitions-from-definition-ids` | Site; supported filters/IDs | Tableau Cloud / Pulse only; API 3.21+ per C1 | No | No | No | Bounded page token output | None | `GET /api/-/pulse/definitions` | A1 §§8.5, 11.3, ADR-023; C1 §§2.7, 5.8 | Docs-only |
| pulse.definition.get | `tadx pulse definition get` | Retrieve one complete Pulse definition/configuration. | Inspect | Ship | CLI | `list-pulse-metric-definitions-from-definition-ids` | Definition ID | Tableau Cloud / Pulse only | No | No | No | Exact ID | None | `GET /api/-/pulse/definitions/{definition_id}` | A1 §§8.5, 11.3; C1 §§2.7, 5.8 | Docs-only |
| pulse.definition.pull | `tadx pulse definition pull` | Materialize one definition as JSON-backed artifact with provenance and baseline. | Deliver | Ship | CLI | Read overlap only | Definition ID; workspace | Tableau Cloud / Pulse only | Yes | No | No | Dirty re-pull requires `--overwrite` | Create / update | Definition GET plus local artifact manager | A1 §§8.2, 8.5, ADR-023; C1 §§2.7, 5.8, 5.13 | Docs-only API; artifact contract architecture-locked |
| pulse.definition.create | `tadx pulse definition create` | Preview and create one definition from explicit configuration. | Change | Blocked | CLI | — | Explicit site; released request fields or local `resource.json` | Tableau Cloud / Pulse only | No | Yes | Yes | No inferred datasource/field mapping; collision explicit | Read / publish | `POST /api/-/pulse/definitions` | A1 §§6.5, 8.5; C1 §§2.7, 5.8 | B3: exact payload/validation semantics pending |
| pulse.definition.update | `tadx pulse definition update` | Preview and patch one exact definition. | Change | Blocked | CLI | — | Definition ID; explicit patch or artifact diff | Tableau Cloud / Pulse only | No | Yes | Yes | Omitted/null semantics must be preserved; equal state no-op when proven | Read / publish | `PATCH /api/-/pulse/definitions/{definition_id}` | A1 §§6.5, 8.5; C1 §§2.7, 5.8 | B3 |
| pulse.definition.delete | `tadx pulse definition delete` | Preview and delete one exact definition. | Change | Blocked | CLI | — | Definition ID | Tableau Cloud / Pulse only | No | Yes | Yes | Expose known dependent metric/subscription effects; no hidden cascade assumptions | None | `DELETE /api/-/pulse/definitions/{definition_id}` | A1 §8.5; C1 §§2.7, 5.8 | B3: cascade/destructive behavior pending |
| pulse.metric.list | `tadx pulse metric list` | List metrics in one definition with bounded continuation. | Find | Ship | CLI | `list-pulse-metrics-from-metric-definition-id` | Definition ID | Tableau Cloud / Pulse only | No | No | No | Bounded endpoint-specific continuation | None | `GET /api/-/pulse/definitions/{definition_id}/metrics` | A1 §8.5; C1 §§2.7, 5.8 | Docs-only |
| pulse.metric.get | `tadx pulse metric get` | Retrieve one exact Pulse metric specification. | Inspect | Ship | CLI | `list-pulse-metrics-from-metric-ids` | Metric ID | Tableau Cloud / Pulse only | No | No | No | Exact ID | None | `GET /api/-/pulse/metrics/{metric_id}` or documented batch get | A1 §8.5; C1 §§2.7, 5.8 | Docs-only; exact batch/single schema capture pending |
| pulse.metric.create | `tadx pulse metric create` | Preview and create one metric in an exact definition. | Change | Blocked | CLI | — | Definition ID; explicit metric specification | Tableau Cloud / Pulse only | No | Yes | Yes | Plain create not assumed idempotent; get-or-create only under explicit desired-state mode | None | `POST /api/-/pulse/metrics`; optional explicit `POST .../metrics:getOrCreate` mode | A1 §8.5; C1 §§2.7, 5.8 | B3: exact identity/request/idempotency semantics pending |
| pulse.metric.update | `tadx pulse metric update` | Preview and patch one exact metric. | Change | Blocked | CLI | — | Metric ID; explicit patch | Tableau Cloud / Pulse only | No | Yes | Yes | Preserve omitted/null semantics; equal state no-op when proven | None | `PATCH /api/-/pulse/metrics/{metric_id}` | A1 §8.5; C1 §§2.7, 5.8 | B3 |
| pulse.metric.delete | `tadx pulse metric delete` | Preview and delete one exact metric. | Change | Blocked | CLI | — | Metric ID | Tableau Cloud / Pulse only | No | Yes | Yes | Expose subscription/dependency effects; no hidden cascade assumptions | None | `DELETE /api/-/pulse/metrics/{metric_id}` | A1 §8.5; C1 §§2.7, 5.8 | B3 |
| pulse.metric.follow | `tadx pulse metric follow` | Preview and create one exact user/group metric subscription. | Change | Blocked | CLI | `list-pulse-metric-subscriptions` for resolution | Metric ID plus exact user/group ID | Tableau Cloud / Pulse only | No | Yes | Yes | Duplicate behavior must be pinned; no ambiguous subscriber | None | `POST /api/-/pulse/subscriptions` | A1 §§4.2, 8.5; C1 §§2.7, 5.8 | B3: request shape and duplicate semantics pending |
| pulse.metric.unfollow | `tadx pulse metric unfollow` | Preview and remove one exact metric subscription. | Change | Blocked | CLI | `list-pulse-metric-subscriptions` for resolution | Subscription ID, or metric+subscriber resolving to exactly one | Tableau Cloud / Pulse only | No | Yes | Yes | Ambiguity fails; missing-as-no-op only if desired-state contract explicitly chosen | None | `DELETE /api/-/pulse/subscriptions/{subscription_id}` | A1 §§4.2, 8.5; C1 §§2.7, 5.8 | B3 |

### 2.4 Narrow administration

| Capability ID | Public command or delegated surface | Resource and user outcome | Type | V1 status | Owner | MCP overlap | Selectors | Products / availability | Local write | Remote mutation | Requires `--apply` | Safety / guard | Artifact effect | Upstream operation | Evidence | Validation / blocker |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| admin.user.list | `tadx admin user list` | List site users with bounded administration metadata. | Find | Ship | CLI | `list-users` | Environment/site; filters | Cloud / Server | No | No | No | Bounded continuation; secret-free | None | `GET /api/{version}/sites/{site-id}/users` | A1 §§1.5, 8.7; C1 §§2.8, 5.9 | Docs-only |
| admin.user.get | `tadx admin user get` | Inspect one exact site user. | Inspect | Ship | CLI | `list-users` | User LUID or exact username/email where supported | Cloud / Server | No | No | No | Ambiguity fails | None | `GET /api/{version}/sites/{site-id}/users/{user-id}` | A1 §8.7; C1 §§2.8, 5.9 | Docs-only |
| admin.user.create | `tadx admin user create` | Preview and add one user to a site with explicit role/auth settings. | Change | Ship | CLI | — | Explicit site; username | Cloud / Server; fields vary by product/version | No | Yes | Yes | No inferred role/auth setting; quota/license failures preserved | None | `POST /api/{version}/sites/{site-id}/users` | A1 §§6.5, 8.7; C1 §§2.8, 5.9 | Docs-only |
| admin.user.update | `tadx admin user update` | Preview and update supported attributes of one exact user. | Change | Ship | CLI | `update-user` | User LUID | Cloud / Server; fields vary | No | Yes | Yes | Equal values no-op when authoritative pre-read exists | None | `PUT /api/{version}/sites/{site-id}/users/{user-id}` | A1 §§6.5, 8.7; C1 §§2.8, 5.9 | Docs-only |
| admin.user.delete | `tadx admin user delete` | Preview and remove one exact user from a site without hidden ownership reassignment. | Change | Ship | CLI | — | User LUID | Cloud / Server | No | Yes | Yes | Ownership constraints surfaced; never silently transfer content | None | `DELETE /api/{version}/sites/{site-id}/users/{user-id}` | A1 §§6.5, 8.7; C1 §§2.8, 5.9 | Docs-only |
| admin.group.list | `tadx admin group list` | List groups with bounded identity and directory metadata. | Find | Ship | CLI | — | Environment/site; filters | Cloud / Server | No | No | No | Bounded continuation | None | `GET /api/{version}/sites/{site-id}/groups` | A1 §8.7; C1 §§2.8, 5.10 | Docs-only |
| admin.group.get | `tadx admin group get` | Inspect one exact group and, when requested, its direct membership. | Inspect | Ship | CLI | — | Group LUID or exact name | Cloud / Server | No | No | No | Exact group; membership pages normalized | None | `GET .../groups`; `GET .../groups/{group-id}/users` | A1 §8.7; C1 §§2.8, 5.10; S1 internal-members correction | Docs-only |
| admin.group.create | `tadx admin group create` | Preview and create one site group with explicit supported settings. | Change | Ship | CLI | — | Explicit site; group name | Cloud / Server; directory/import fields vary | No | Yes | Yes | Collision and directory-setting failures preserved | None | `POST /api/{version}/sites/{site-id}/groups` | A1 §§6.5, 8.7; C1 §§2.8, 5.10 | Docs-only |
| admin.group.update | `tadx admin group update` | Preview and update group attributes and/or converge direct membership to explicit desired state. | Change | Ship | CLI | — | Group LUID; exact user LUIDs | Cloud / Server | No | Yes | Yes | Full membership diff preview; ordered calls; stop on first failure; no rollback/resume | None | `PUT .../groups/{group-id}`; member pre-read; `POST .../groups/{group-id}/users`; `DELETE .../users/{user-id}` | A1 §§4.2, 6.5, 8.7, 8.12; C1 §§2.8, 5.10; S1 internal-members correction | Docs-only |
| admin.group.delete | `tadx admin group delete` | Preview and delete one exact group without deleting its users. | Change | Ship | CLI | — | Group LUID | Cloud / Server | No | Yes | Yes | Known direct facts only; no claim of full permission-impact analysis | None | `DELETE /api/{version}/sites/{site-id}/groups/{group-id}` | A1 §§6.5, 8.7; C1 §§2.8, 5.10 | Docs-only |
| admin.permission.get | `tadx admin permission get` | Inspect explicit/default permission rules for one supported resource. | Inspect | Ship | CLI | — | Resource kind plus exact LUID; optional principal/capability filters | Cloud / Server | No | No | No | Distinguish direct/default/inherited/unknown; no effective-permission engine | None | Workbook/datasource/flow/project permission GET endpoints | A1 §§8.8, 11.2; C1 §§2.8, 5.11 | Docs-only; tabget normalization review is implementation backlog, not capability blocker |

### 2.5 Delegated discoverable operations

| Capability ID | Public command or delegated surface | Resource and user outcome | Type | V1 status | Owner | MCP overlap | Selectors | Products / availability | Local write | Remote mutation | Requires `--apply` | Safety / guard | Artifact effect | Upstream operation | Evidence | Validation / blocker |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| datasource.query | Tableau MCP `query-datasource` | Run analytical queries against datasource data. | Inspect | Delegated | MCP | Primary tool | Datasource/workbook datasource identity | Cloud / Server with VDS/MCP availability | No | No | No | MCP contract | None | MCP / VDS `query-datasource` | A1 §§3.2–3.4, 8.5, ADR-031; C1 §§2.4, 3 | Delegated; not executed by TADX |
| view.read | Tableau MCP view/custom-view read tools | Retrieve rendered view data or images for analysis/presentation. | Inspect | Delegated | MCP | `get-view-data`, `get-view-image`, `get-custom-view-data`, `get-custom-view-image` | View/custom-view identity | Cloud / Server with MCP availability | No | No | No | MCP contract | None | Tableau MCP / underlying view APIs | A1 §§3.2–3.4; C1 §§2.3, 3 | Delegated; not executed by TADX |
| pulse.metric.values-insights | Tableau MCP Pulse insight tools | Query current Pulse values, insight bundles, and briefs. | Inspect | Delegated | MCP | `generate-pulse-metric-value-insight-bundle`; `generate-pulse-insight-brief` | Metric IDs | Tableau Cloud / Pulse with MCP availability | No | No | No | MCP contract | None | Tableau MCP / Pulse insight APIs | A1 §§3.4, 8.5, ADR-023/031; C1 §§2.7, 3 | Delegated; not executed by TADX |
| workbook.author | Tableau Desktop / Desktop MCP | Create or semantically modify workbook content. | Change | Delegated | Tableau / Desktop MCP | Owning authoring surface | Workbook and authoring context | Tableau Desktop interoperability | N/A outside TADX | N/A outside TADX | N/A outside TADX | Owning surface controls safety | External artifact may become dirty | Tableau authoring surface | A1 §§1.6, 3.4, 8.6; C1 §§2.3, 3 | Delegated; TADX never transforms workbook XML |
| datasource.field-description.generate | Agent reasoning or an explicitly shipped skill; no TADX CLI command | Generate/revise field descriptions using metadata and optional sampled statistics. | Change | Delegated | Agent / Skill | `get-datasource-metadata`; `query-datasource` as needed | Datasource and selected fields | Cloud / Server | Optional change set | No at reasoning stage | No at reasoning stage | Human/agent review before invoking blocked write capability | Optional update | Procedure composed from MCP reads plus `datasource.field-description.update` | A1 §§3.1, 4.4, 8.4, 11.4; C1 §2.4 | Delegated reasoning; remote write remains B1 |

## 3. Shared execution invariants

### 3.1 Identity resolution

Tableau LUIDs are authoritative remote identity. Names, project paths, local paths, and catalog hits are selectors only.

1. Prefer an explicit LUID.
2. Otherwise resolve an exact name and, where applicable, exact slash-delimited project path.
3. Zero matches and multiple matches are deterministic errors.
4. Do not use fuzzy matching, interactive selector prompts, or silent redirection after a remote rename/move.
5. A cached catalog result may help select a resource, but every remote mutation re-resolves the exact target against Tableau.
6. Metadata API IDs are not assumed to equal REST/vizportal LUIDs. Store proven mappings.

### 3.2 Local write versus remote mutation

`Local write` covers config, catalog, workspace, artifact pull, and local artifact editing. These operations do not require `--apply`.

`Remote mutation` covers consequential changes to Tableau content, Pulse configuration, users, or groups. Every such row:

- requires an explicit write environment/site,
- remains preview-only without `--apply`,
- is hidden from default mutation discovery unless `TADX_ENABLE_MUTATIONS=1`,
- is not authorized merely because it is discoverable,
- cannot use `--force` as a synonym for `--apply`.

Authentication/session establishment is not treated as a consequential resource mutation.

### 3.3 Preview and apply

A remote mutation follows this sequence:

```text
resolve exact target
-> validate admitted inputs
-> emit deterministic preview
-> mutate only with --apply
-> report authoritative outcome
```

The preview includes the environment/site, resource identity, destination, changed fields, collision/overwrite mode, and independently meaningful substeps. There is no second confirmation prompt and no special production-only prompt.

TADX does not promise generic optimistic concurrency or remote-change detection. Tableau remains authoritative. There are no generic automatic retries; a capability may add only a bounded retry proven safe and deterministic.

### 3.4 Pagination normalization

Adapters preserve upstream pagination while presenting one bounded public continuation envelope.

| Upstream family | Adapter rule |
| --- | --- |
| Classic REST lists | Preserve `pageNumber` / `pageSize` semantics and endpoint-specific limits. |
| Content search | Preserve `page` / `limit` and returned next/previous/page metadata. |
| Pulse | Preserve page-token semantics and do not assume all Pulse endpoints use an identical token shape. |
| Metadata API | Prefer connection cursors for large lists; never combine `after` and `offset`; preserve the documented maximum page size and warning state. |
| Catalog refresh | May exhaust all admitted upstream pages internally because results go to cache, not model context. A failed page makes the new generation incomplete. |

Default CLI list/search output is bounded. `--full` expands permitted output but does not justify dumping an unbounded site inventory into model context.

### 3.5 Asynchronous polling

When a released Tableau operation returns a job and no user decision is needed mid-flight:

- TADX performs upload-session mechanics and bounded polling internally.
- It returns terminal success, terminal failure, or timeout.
- It includes Tableau request/job IDs where available.
- It does not make the agent repeatedly poll.
- It exposes a separate job handle only when completion cannot reasonably fit one invocation.

Polling does not create a generic retry policy.

### 3.6 Partial warnings and errors

Single-resource atomic operations succeed or fail.

A deterministic multi-resource operation may return partial outcomes only when substeps are independently meaningful:

- shallow project pull/publish,
- group membership convergence.

Such operations stop on the first failed substep, report exact completed and failed work, do not roll back completed remote changes, and do not provide automatic resume in V1.

Metadata API warning-bearing responses are not ordinary complete success. Backfill, linked-result, inheritance, page-size, node-limit, time-limit, permission, sort, and rate-limit conditions must be preserved. Catalog refresh must not mark a warning-incomplete generation complete.

Structured errors include, where available: stable string ID, operation, exact selector/resource identity, environment/site, summary, upstream status/code/detail, retryability, deterministic corrective action, validation details, and request/job ID. Exit codes remain `0` success/no-op, `1` operation/runtime failure, and `2` usage error.

### 3.7 Artifact and re-pull behavior

Every pulled artifact contains:

1. the canonical native payload or `resource.json`,
2. `metadata.json` with source provenance and authoritative Tableau ID,
3. required `view.md`,
4. a baseline fingerprint covering managed canonical content.

Artifacts never persist a publish target. External tools may edit artifacts.

Re-pull behavior is identity-based:

```text
current fingerprint == baseline
    -> warn
    -> replace from remote
    -> update provenance and baseline

current fingerprint != baseline
    -> stop
    -> require --overwrite

--overwrite
    -> warn
    -> replace
    -> update provenance and baseline
```

There is no automatic revision history. Pull is a local write and never requires `--apply`. Native workbook, datasource, and flow packages are preserved; TADX does not silently author workbook or flow semantics.

A1 explicitly rejects workspace/state locking in V1. Concurrent local races are the caller's responsibility; implementations must not introduce advisory locks or collaboration semantics.

## 4. CLI implementation evidence

This section contains implementation evidence only for CLI-owned registry rows. It does not restate delegated or excluded capabilities.

### Shared acceptance-test clauses

Behavioral tests are authored before implementation. Cards below reference these shared clauses:

| ID | Required acceptance behavior |
| --- | --- |
| **A-ID** | LUID succeeds; exact name/path succeeds when unique; zero/multiple matches fail without fuzzy or interactive fallback. |
| **A-PAGE** | Upstream pages/tokens normalize correctly; public output is bounded; continuation retrieves the next stable slice; catalog refresh exhausts pages internally. |
| **A-OUTPUT** | Compact TOON default, expanded `--full`, explicit `--raw` only where supported, stable error/output fields, and golden fixtures. |
| **A-SECRET** | PAT names/secrets/tokens are absent from visible config values, output, logs, artifacts, catalog, fixtures, and diagnostics. |
| **A-LOCAL** | Canonical payload, metadata, view, baseline, external-edit detection, clean/dirty re-pull, and path-boundary behavior are tested. |
| **A-APPLY** | Preview performs no consequential remote mutation; `--apply` executes only the shown plan; explicit write target is required; `--force` never authorizes execution. |
| **A-ASYNC** | Upload/job success, terminal failure, timeout, and request/job-ID reporting are deterministic. |
| **A-PARTIAL** | Multi-step operation stops on first failure, reports exact completed/failed work, and performs no rollback or automatic resume. |
| **A-META** | Metadata permission mode, ID mappings, pagination, warnings, and incomplete-result state are preserved rather than guessed or flattened. |
| **A-ERROR** | HTTP/upstream status, Tableau code/summary/detail, operation, target context, retryability, corrective action, and request/job ID are preserved with secret redaction. |

### 4.1 Foundation, configuration, catalog, workspace, and cross-content

| Capability(s) | Exact operation | Material contract | Variable behavior | Acceptance tests | Evidence / status |
| --- | --- | --- | --- | --- | --- |
| `env.profile.list/get/add/update/remove/set-default` | Read or atomically replace `<os.UserConfigDir>/tadx/config.yaml`. | YAML stores aliases, URL, site ID/content URL, auth type, PAT variable names, default environment/workspace, and non-secret settings. Never store PAT values. `auth status` reports resolved configuration/PAT-reference presence only. | No pagination or async behavior. Local writes do not require `--apply` and must remain discoverable when remote mutations are hidden. | `A-SECRET`; add collision; update exact alias; removing active default fails or explicitly changes it; equal update is exit-0 no-op; malformed YAML never leaves a partial file. | A1 §§7.2–7.4, 9.2; C1 §2.1. Architecture-locked. |
| `auth.check` | `POST /api/{version}/auth/signin`; optional `POST /api/{version}/auth/signout`. | Request carries PAT name, PAT secret, and `site.contentUrl`. Capture credential token, site LUID, and user LUID internally; expose only redacted status and exact target context. | Synchronous. Sign-in is authentication/session establishment, not a consequential resource mutation. Preserve TLS/proxy/auth/site errors and request IDs. | `A-SECRET`, `A-ERROR`; valid PAT/wrong site/expired PAT; token absent from output, logs, artifact, and catalog fixtures. | A1 §§5.4, 7.3–7.4; C1 §5.1. Docs-only; exact official source capture pending. |
| `auth.status` | Local config and environment-variable resolution only. | Return selected alias, auth type, PAT variable names, and whether referenced variables are present. Do not claim the PAT is valid or create a Tableau session. | No pagination/async behavior. | `A-SECRET`; missing variable vs missing profile; status remains usable offline. | A1 §§1.5, 5.4, 9.2; S1 requires a distinct row. Architecture-locked. |
| `capability.list`, `capability.get` | Query the executable capability registry used by command wiring/help. | Return stable ID, surface, owner, overlap, selectors, availability, local-write/remote-mutation/apply flags, and blocker. No separately maintained command tree or ownership map. | Bounded local continuation if needed. `TADX_ENABLE_MUTATIONS=1` controls discovery of rows with `remote_mutation=Yes`; it does not authorize execution. | Every public command has exactly one registry row; IDs unique; delegated rows resolve; local-write rows remain visible with mutation discovery off; generated help matches registry. | A1 §§3.3, 5.2, 6.6; S1 single-source-of-truth correction. Architecture-locked. |
| `catalog.refresh` | Resource list/read endpoints plus only admitted focused Metadata API/permission reads. | Build a new generation with source environment/site, generated-at, counts, and completeness. Make it current only after all required pages/slices complete. Remote Tableau remains authoritative. | Exhaust upstream pagination internally. Metadata warnings or failed pages make the generation incomplete; never flatten to success. No generic retries. | `A-PAGE`, `A-META`, `A-ERROR`; failure on page N leaves prior complete generation current; source environment mismatch is visible; 12-hour stale threshold. | A1 §§5.9, 8.11, 11.2, 12.8; C1 §§5.3, 5.12. Docs-only remote facts. |
| `catalog.search/get/status` | Read the local normalized catalog. | Return bounded records with Tableau LUID, exact source generation, staleness, and provenance. A catalog hit is a selector hint, never write authority. | Local continuation only; no async behavior. | `A-ID`, `A-OUTPUT`; ambiguity fails; stale warning at 12 hours; write command re-resolves remotely despite a cached match. | A1 §§5.7, 5.9, 8.1, 8.11; C1 §2.1. Architecture-locked. |
| `workspace.create/list/status/move/clean` | Local filesystem and `tadx.yaml`/`.tadx/` state. | Resolution order: explicit `--workspace` → containing workspace → environment default → general default → fail. Layout contains canonical payload, `metadata.json`, `view.md`, and hidden state. No workspace/state locking in V1. | Local writes need no `--apply`. Concurrent-process races are caller responsibility. `clean` preserves managed canonical payloads by default. | `A-LOCAL`; no implicit workspace creation by pull; local move preserves Tableau ID; path traversal/collision fail; status detects external edits; no lock files/coordination semantics. | A1 §§6.1, 6.3–6.4, 7.5–7.6, 8.1–8.2, ADR-032; C1 §2.1. Architecture-locked. |
| `content.search` | `GET /api/-/search`. | Inputs: `terms`, supported `filter`, `limit`, `page`, `order_by`. Output: typed hits with LUID/name/location plus continuation fields. Relevance results must be resolved to exact LUID before a write. | Uses page/limit rather than classic REST pagination; C1 records a 2,000-item upstream ceiling. TADX output remains bounded. | `A-PAGE`, `A-ID`, `A-OUTPUT`; page continuation; filter rejection preserved; selected search hit is re-resolved before mutation. | A1 §§3.2–3.4, 8.11; C1 §5.2. Docs-only; exact official source capture pending. |
| `content.get` | Dispatch to the selected resource adapter's exact get or exact filtered-list operation. | Input must include resource kind and one exact selector. Return compact normalized identity and source context; do not create an artifact. | No cross-resource pagination after exact resolution. | `A-ID`, `A-OUTPUT`, `A-ERROR`; unsupported kind; zero match; two exact-name matches in different projects. | A1 §§5.6–5.7, 6.2; C1 §2.2. Docs-only. |
| `doctor.run` | Local validators plus read-only sign-in/connectivity and best-effort MCP availability checks. | Return per-check pass/warn/fail, stable error ID, target context, and corrective action. Persistent logging remains off unless `TADX_LOG_LEVEL` is set. | No mutation; individual check failures do not suppress other independent checks. | `A-SECRET`, `A-ERROR`; offline config-only result; invalid PAT; stale catalog; dirty workspace; MCP probe absence is warning, not hard dependency. | A1 §§5.12, 7.7, 9.1; C1 §2.1. MCP detection mechanism is ordinary implementation work. |

### 4.2 Workbook

| Capability(s) | Exact operation | Material contract | Variable behavior | Acceptance tests | Evidence / status |
| --- | --- | --- | --- | --- | --- |
| `workbook.list` | `GET /api/{version}/sites/{site-id}/workbooks`. | Material inputs: endpoint-supported `pageNumber`, `pageSize`, `filter`, `sort`, `fields`. Return LUID, name/content URL, project, owner, and lifecycle fields used by selectors. | Classic REST pagination; C1 records common default 100 and maximum 1,000 where supported. | `A-PAGE`, `A-OUTPUT`, `A-ERROR`; endpoint-specific unsupported filter is not silently rewritten. | A1 §§1.5, 3.2; C1 §5.4. Docs-only. |
| `workbook.get` | `GET /api/{version}/sites/{site-id}/workbooks/{workbook-id}`. | Return authoritative workbook identity, project, owner, content URL, views/tags where supplied. | Synchronous exact read. | `A-ID`, `A-OUTPUT`; exact LUID and exact name/project path produce the same identity; ambiguity fails before call. | A1 §§5.7, 8.1; C1 §5.4. Docs-only. |
| `workbook.pull` | `GET /api/{version}/sites/{site-id}/workbooks/{workbook-id}/content`. | Preserve `.twb` or `.twbx` bytes. Write canonical payload, provenance, `view.md`, and baseline fingerprint. `includeExtract` is sent only when explicitly supported/requested. | Synchronous download. Clean re-pull warns/replaces; dirty re-pull stops unless `--overwrite`. | `A-ID`, `A-LOCAL`, `A-ERROR`; TWB/TWBX extension preserved; dirty package protected; download permission failure leaves existing artifact intact. | A1 §§6.3–6.4, 8.2, 8.6; C1 §§5.4, 5.13. Docs-only API; artifact contract locked. |
| `workbook.publish` | `POST /api/{version}/sites/{site-id}/workbooks`; large uploads use file-upload session endpoints. Optional internal TWB validation: `POST .../workbooks/validateWorkbook`. | Multipart request carries payload plus file/upload-session ID, workbook type, destination project, name, explicit overwrite, optional async/connection fields. Return workbook identity or pollable job plus request/job IDs. | Preview by default; `--apply` required. Poll jobs internally to bounded terminal result. Standalone `workbook check` is not public. TWB validator is API 3.29/2026.2 per C1 and must not claim TWBX validation or impose a global publish version gate. | `A-APPLY`, `A-ASYNC`, `A-ERROR`; preview makes no publish call; collision without overwrite; chunk order/session failure; terminal job failure; TWB validation warning/error; TWBX path does not claim server validation; unsupported validator follows optimistic-version policy. | A1 §§5.11, 6.5–6.9, 8.6, ADR-017/027; C1 §5.4; S1 workbook-check correction. Docs-only. |

### 4.3 Datasource

| Capability(s) | Exact operation | Material contract | Variable behavior | Acceptance tests | Evidence / status |
| --- | --- | --- | --- | --- | --- |
| `datasource.list` | `GET /api/{version}/sites/{site-id}/datasources`. | Return LUID, name/content URL, project, owner, type, and lifecycle fields used by selectors. | Classic REST pagination; endpoint-specific filters/sorts only. | `A-PAGE`, `A-OUTPUT`, `A-ERROR`. | A1 §§1.5, 3.2; C1 §5.5. Docs-only. |
| `datasource.get` | `GET .../datasources/{id}`; focused `POST /api/metadata/graphql`; optional `POST /api/v1/vizql-data-service/read-metadata` and `/get-datasource-model`. | Base read returns identity/project/owner/type. Bounded detail may return stable ID mappings, names/captions, roles/types/aggregations, formulas, descriptions/inheritance, hidden/folder state, logical tables/relationships, and direct composition facts. | Metadata API connection cursor preferred, max page 1,000 per C1; never combine `after` with `offset`. Record permission mode. Preserve warnings such as backfill/node/time/page limits. VDS detail is version/product dependent. | `A-ID`, `A-PAGE`, `A-META`; metadata ID must not be guessed to equal REST LUID; permission modes yield explicitly different completeness; warning-bearing response is not reported as complete. | A1 §§8.3–8.4, 11.4; C1 §§5.5, 5.12. Docs-only; B2 affects parent mapping. |
| `datasource.pull` | `GET /api/{version}/sites/{site-id}/datasources/{datasource-id}/content` plus focused metadata reads. | Preserve native `.tds`/`.tdsx` package and every direct published-parent reference needed for republish. Write provenance, baseline, human view, and explicit parent mapping. | Clean/dirty re-pull invariant applies. No generic dependency graph or transitive acquisition. | `A-LOCAL`, `A-ID`; ordinary datasource round-trip; multi-parent composed fixture pull→republish; missing/inaccessible parent diagnostics; package bytes/semantics not silently normalized. | A1 §§8.2–8.4, ADR-020/022; C1 §5.5. Blocked by B2 for composed fixtures. |
| `datasource.composition.update` | Local deterministic `.tds`/`.tdsx` transformation; no remote call. | Input is an explicit ordered/set-normalized list of immediate parent LUIDs/content URLs and fixture-proven native fields. The command never selects parents, joins, or relationship semantics for the caller. | Local write only; equal normalized composition is exit-0 no-op. Subsequent publish separately previews/applies remote effects. | `A-LOCAL`; minimal one-parent/add/remove/reorder fixtures; invalid duplicate/missing parent; deterministic serialization; package remains publishable. | A1 §8.3, ADR-022; C1 §2.4; S1 command split. Blocked by B2. |
| `datasource.field-description.update` | Unresolved released API for a published datasource field. Metadata GraphQL is read-only; external-table-column `Update Column` is not an acceptable substitute. | Requires exact datasource LUID, authoritative stable field ID mapping, old/new description preview, explicit target environment/site, and request ID/result on apply. | Remote mutation with `--apply`. Equal authoritative value should be no-op. Never target by name/caption alone. | `A-APPLY`, `A-ID`, `A-ERROR`; renamed/hidden/calculated/remote fields; similarly named external column remains unchanged; unsupported license/version; stale mapping fails safely. | A1 §8.4 and Appendix A/B; C1 §5.5; S1 command split. Blocked by B1. |
| `datasource.publish` | `POST /api/{version}/sites/{site-id}/datasources`; large uploads use file-upload sessions. Composed path sends `parentDataSourceUrls`. | Request includes file/upload-session ID, datasource type, explicit destination/name, and explicit overwrite/append/replace mode. For composed artifacts, send every serialized immediate parent and no transitive parents. | Preview/`--apply`; internal bounded polling for async jobs. Preserve unsupported version, parent access, connector, credential, depth/count, and connection failures. | `A-APPLY`, `A-ASYNC`, `A-ERROR`; ordinary create/overwrite; append/replace never inferred; large upload; one/multiple immediate parents; missing and extra parent URLs; old deployment error preserved. | A1 §§6.5–6.9, 8.3, ADR-017/022/027; C1 §5.5. Ordinary path docs-only; full V1 path blocked by B2. |

### 4.4 Flow

| Capability(s) | Exact operation | Material contract | Variable behavior | Acceptance tests | Evidence / status |
| --- | --- | --- | --- | --- | --- |
| `flow.list` | `GET /api/{version}/sites/{site-id}/flows`. | Return flow LUID, name, project, owner, and lifecycle fields used by selectors. | Classic REST pagination; product/license support is deployment-specific. | `A-PAGE`, `A-OUTPUT`; unsupported deployment surfaces upstream failure. | A1 §§1.5, 3.4; C1 §5.6. Docs-only. |
| `flow.get` | `GET /api/{version}/sites/{site-id}/flows/{flow-id}`. | Return exact identity, project, owner, output steps, and supported metadata. | Synchronous exact read. | `A-ID`, `A-OUTPUT`. | A1 §§3.4, 5.7; C1 §5.6. Docs-only. |
| `flow.pull` | `GET /api/{version}/sites/{site-id}/flows/{flow-id}/content`. | Preserve `.tfl`/`.tflx`; create canonical payload, provenance, human view, and baseline. | Clean/dirty re-pull invariant applies. | `A-LOCAL`, `A-ID`; both package types; permission failure; dirty protection. | A1 §§6.3–6.4; C1 §§5.6, 5.13. Docs-only API. |
| `flow.publish` | `POST /api/{version}/sites/{site-id}/flows`; upload sessions for large files. | Multipart request carries TFL/TFLX or upload-session ID, type, explicit project/name/overwrite, and connection fields when supported. | Preview/`--apply`; no hidden flow authoring. | `A-APPLY`, `A-ERROR`; invalid package; collision; large upload; unsupported license/product. | A1 §§6.5–6.8; C1 §5.6. Docs-only. |

### 4.5 Project

| Capability(s) | Exact operation | Material contract | Variable behavior | Acceptance tests | Evidence / status |
| --- | --- | --- | --- | --- | --- |
| `project.list` | `GET /api/{version}/sites/{site-id}/projects`. | Return project LUID, name, parent identity/path inputs, description, and content-permission state needed by selectors. | Classic REST pagination. | `A-PAGE`, `A-OUTPUT`; nested projects with duplicate names preserve distinct IDs/paths. | A1 §§6.2, 6.10; C1 §5.7. Docs-only. |
| `project.get` | Resolve exact LUID or exact slash-delimited path from authoritative project data. | Return one project only; path is a selector, not identity. | No fuzzy or interactive fallback. | `A-ID`; renamed/moved project old path fails; duplicate name across branches; exact LUID survives rename. | A1 §§5.7, 6.2, 8.1; C1 §5.7. Docs-only. |
| `project.create` | `POST /api/{version}/sites/{site-id}/projects`. | Request includes explicit name, description/content-permission fields, and parent ID only when selected. Return project LUID and authoritative parent/context. | Preview/`--apply`; no inferred parent. | `A-APPLY`, `A-ID`; same-name collision; ambiguous parent; root vs explicit parent. | A1 §§6.5, 6.10; C1 §5.7. Docs-only. |
| `project.update` | `PUT /api/{version}/sites/{site-id}/projects/{project-id}`. | Only bounded metadata admitted in V1. Do not expose generic parent/hierarchy movement. | Preview/`--apply`; equal values no-op when detectable. | `A-APPLY`, `A-ID`; rename/description update; attempted parent move rejected as usage/capability error. | A1 §§6.5, 6.10 and remote-move exclusion; C1 §5.7. Docs-only. |
| `project.pull` | Project read plus direct workbook/datasource/flow enumeration and each resource's pull operation. | Selected project and direct supported content only; no child-project recursion or transitive dependency acquisition. Create a project manifest plus direct artifacts. | Exhaust direct pages internally; stop on first failed resource; report completed and failed items; no rollback/resume. | `A-LOCAL`, `A-PARTIAL`, `A-PAGE`; nested project fixture proves no child inclusion; dirty second artifact stops after reporting first; exact direct membership across Cloud/Server fixture. | A1 §§6.10, 8.9, 8.12, ADR-021; C1 §5.7. Blocked by B4. |
| `project.publish` | Ordered project create/update and direct workbook/datasource/flow publish operations. | Requires explicit target mappings. Preview emits the complete ordered plan. No hidden dependency or migration decisions. | Apply executes sequentially, polls resource jobs, stops on first terminal failure, reports completed/failed, and does not roll back or resume. | `A-APPLY`, `A-ASYNC`, `A-PARTIAL`; missing mapping; second-resource failure; completed remote changes retained and reported; no child recursion. | A1 §§6.5–6.10, 8.9, ADR-021/027; C1 §5.7. Blocked by B4. |

### 4.6 Pulse

| Capability(s) | Exact operation | Material contract | Variable behavior | Acceptance tests | Evidence / status |
| --- | --- | --- | --- | --- | --- |
| `pulse.definition.list` | `GET /api/-/pulse/definitions`. | Inputs include endpoint-supported `page_size`, `page_token`, and filters. Return definition IDs/config summaries and `next_page_token`. | Token pagination; Cloud/Pulse only, API 3.21+ per C1. | `A-PAGE`, `A-OUTPUT`; expired/invalid token; unsupported non-Cloud environment. | A1 §8.5; C1 §5.8. Docs-only. |
| `pulse.definition.get` | `GET /api/-/pulse/definitions/{definition_id}`. | Return complete released definition/configuration fields and associated metric summary only where the endpoint provides it. | Exact ID read. | `A-ID`, `A-OUTPUT`; schema fixture pinned from captured OpenAPI. | A1 §§8.5, 11.3; C1 §5.8. Docs-only. |
| `pulse.definition.pull` | Definition GET plus local artifact manager. | Write `resource.json`, `metadata.json`, `view.md`, and baseline fingerprint. | Clean/dirty re-pull invariant. | `A-LOCAL`, `A-ID`; stable JSON canonicalization; dirty protection; remote missing. | A1 §§8.2, 8.5, ADR-023; C1 §§5.8, 5.13. Docs-only API. |
| `pulse.definition.create` | `POST /api/-/pulse/definitions`. | Use only released request fields; preview exact datasource/field references and created identity/result. | Preview/`--apply`; create not assumed idempotent. | `A-APPLY`, `A-ID`, `A-ERROR`; valid/invalid datasource-field mapping; duplicate/near-duplicate definitions; entitlement failure. | A1 §§6.5, 8.5; C1 §5.8. Blocked by B3. |
| `pulse.definition.update` | `PATCH /api/-/pulse/definitions/{definition_id}`. | Patch only explicit fields; preserve omitted vs null behavior and return authoritative updated definition. | Preview/`--apply`; equal patch no-op only after authoritative comparison. | `A-APPLY`, `A-ID`; omitted/null; stale field reference; equal update. | A1 §§6.5, 8.5; C1 §5.8. Blocked by B3. |
| `pulse.definition.delete` | `DELETE /api/-/pulse/definitions/{definition_id}`. | Preview exact target and all cheaply authoritative dependent facts; never invent cascade guarantees. | Preview/`--apply`; destructive behavior/cascade must be captured. | `A-APPLY`, `A-ERROR`; definition with metrics/subscriptions; repeated delete; permission failure. | A1 §8.5; C1 §5.8. Blocked by B3. |
| `pulse.metric.list` | `GET /api/-/pulse/definitions/{definition_id}/metrics`. | Return metric IDs/specification summaries and endpoint-specific continuation. | Token/page behavior must follow captured schema, not be assumed from definition list. | `A-PAGE`, `A-ID`, `A-OUTPUT`. | A1 §8.5; C1 §5.8. Docs-only. |
| `pulse.metric.get` | `GET /api/-/pulse/metrics/{metric_id}` or captured official batch-get operation. | Return exact metric specification/configuration; do not substitute current value/insight output. | Exact ID read. | `A-ID`; single vs batch schema normalization. | A1 §8.5; C1 §5.8. Docs-only; exact operation schema capture pending. |
| `pulse.metric.create` | `POST /api/-/pulse/metrics`; optional explicit desired-state mode may use `POST /api/-/pulse/metrics:getOrCreate`. | Request carries exact definition identity and released metric specification. Plain create and get-or-create remain distinct internal modes. | Preview/`--apply`; never infer idempotency or identity keys. | `A-APPLY`, `A-ID`; duplicate and near-duplicate metrics; get-or-create created true/false; unstable identity field test. | A1 §8.5; C1 §5.8. Blocked by B3. |
| `pulse.metric.update` | `PATCH /api/-/pulse/metrics/{metric_id}`. | Patch explicit fields and preserve omitted/null semantics. | Preview/`--apply`; equal update no-op when proven. | `A-APPLY`, `A-ID`; omitted/null; definition drift; equal update. | A1 §8.5; C1 §5.8. Blocked by B3. |
| `pulse.metric.delete` | `DELETE /api/-/pulse/metrics/{metric_id}`. | Preview exact target and authoritative subscription/dependency facts available cheaply. | Preview/`--apply`; no hidden cascade assumption. | `A-APPLY`, `A-ERROR`; subscribed metric; repeated delete; permission failure. | A1 §8.5; C1 §5.8. Blocked by B3. |
| `pulse.metric.follow` | `POST /api/-/pulse/subscriptions`. | Request carries exact metric and exact user/group subscriber identity; return subscription ID/config. | Preview/`--apply`; duplicate behavior must be proven. | `A-APPLY`, `A-ID`; user/group shape; duplicate follow; entitlement failure. | A1 §§4.2, 8.5; C1 §5.8. Blocked by B3. |
| `pulse.metric.unfollow` | `DELETE /api/-/pulse/subscriptions/{subscription_id}`. | Use exact subscription ID, or resolve metric+subscriber to exactly one subscription before preview. | Preview/`--apply`; missing state is no-op only under an explicitly selected desired-state contract. | `A-APPLY`, `A-ID`; zero/one/multiple resolution; repeated unfollow semantics. | A1 §§4.2, 8.5; C1 §5.8. Blocked by B3. |

### 4.7 Users, groups, and permissions

| Capability(s) | Exact operation | Material contract | Variable behavior | Acceptance tests | Evidence / status |
| --- | --- | --- | --- | --- | --- |
| `admin.user.list` | `GET /api/{version}/sites/{site-id}/users`. | Return user LUID, username, full name/email, site role, auth setting, and supported status fields. | Classic REST pagination. | `A-PAGE`, `A-OUTPUT`, `A-SECRET`. | A1 §8.7; C1 §5.9. Docs-only. |
| `admin.user.get` | `GET /api/{version}/sites/{site-id}/users/{user-id}`. | Return one authoritative site-user record. | Exact read. | `A-ID`; exact username/email ambiguity; deleted user. | A1 §8.7; C1 §5.9. Docs-only. |
| `admin.user.create` | `POST /api/{version}/sites/{site-id}/users`. | Request includes explicit username, site role, auth setting, and only endpoint-supported fields. Return created site-user ID. | Preview/`--apply`; no role/auth inference. | `A-APPLY`, `A-ERROR`; already-member, quota/license, invalid role/auth. | A1 §§6.5, 8.7; C1 §5.9. Docs-only. |
| `admin.user.update` | `PUT /api/{version}/sites/{site-id}/users/{user-id}`. | Patch explicit supported attributes only; do not expand PAT-only TADX auth into password-management behavior. | Preview/`--apply`; equal authoritative values no-op. | `A-APPLY`, `A-ID`; role/license transition; equal update; unsupported field. | A1 §§6.5, 8.7; C1 §5.9. Docs-only. |
| `admin.user.delete` | `DELETE /api/{version}/sites/{site-id}/users/{user-id}`. | Remove exact site user. Ownership reassignment is never implicit. | Preview/`--apply`; upstream ownership constraint is preserved. | `A-APPLY`, `A-ERROR`; user owns content; repeated removal; exact target. | A1 §§6.5, 8.7; C1 §5.9. Docs-only. |
| `admin.group.list` | `GET /api/{version}/sites/{site-id}/groups`. | Return group ID/name and supported domain/import metadata. | Classic REST pagination. | `A-PAGE`, `A-OUTPUT`. | A1 §8.7; C1 §5.10. Docs-only. |
| `admin.group.get` | Group exact resolution plus `GET /api/{version}/sites/{site-id}/groups/{group-id}/users` when membership requested. | Return one group and bounded direct membership. There is no separate public member-list capability. | Normalize group and member pagination separately. | `A-ID`, `A-PAGE`; duplicate group names; external/directory group membership limitations. | A1 §8.7; C1 §5.10; S1 internal-members correction. Docs-only. |
| `admin.group.create` | `POST /api/{version}/sites/{site-id}/groups`. | Send explicit name and only endpoint-supported directory/import settings; return group ID. | Preview/`--apply`. | `A-APPLY`, `A-ERROR`; duplicate name; unsupported directory setting. | A1 §§6.5, 8.7; C1 §5.10. Docs-only. |
| `admin.group.update` | `PUT .../groups/{group-id}` plus membership pre-read, `POST .../groups/{group-id}/users`, and `DELETE .../groups/{group-id}/users/{user-id}`. | Public operation accepts explicit attribute changes and/or desired/add/remove user LUIDs. Preview exact added/removed/unchanged sets. | Apply executes deterministic ordered calls; stop on first member failure; report completed/failed; no rollback/resume. Equal desired state is exit-0 no-op. | `A-APPLY`, `A-PARTIAL`, `A-ID`; one add/one remove; duplicate inputs normalized; second member failure; directory-managed restriction. | A1 §§4.2, 6.5, 8.7, 8.12; C1 §5.10; S1 internal-members correction. Docs-only. |
| `admin.group.delete` | `DELETE /api/{version}/sites/{site-id}/groups/{group-id}`. | Delete exact group; users remain. Preview only cheaply authoritative direct facts and do not claim complete downstream permission impact. | Preview/`--apply`. | `A-APPLY`, `A-ERROR`; directory group; users retained; repeated delete. | A1 §§6.5, 8.7; C1 §5.10. Docs-only. |
| `admin.permission.get` | Permission GET endpoints for exact workbook, datasource, flow, or project; project-default reads only where admitted. | Normalize principal type/ID, capability, Allow/Deny, resource, source endpoint, and direct/default/inherited/unknown classification. Do not claim a complete effective-permission evaluator. | Endpoint-specific paging/shape. Incomplete principal resolution is explicit. | `A-ID`, `A-META`; direct vs default; locked/inherited/unknown; inaccessible principal; compare normalized fixture to raw payload. | A1 §§8.8, 11.2; C1 §5.11. Docs-only; source-code reuse review is not a blocker. |

## 5. Delegated, deferred, and rejected boundaries

| Boundary | Disposition / owner | Contract | Evidence |
| --- | --- | --- | --- |
| Datasource analytical query / VDS | Delegated to Tableau MCP | Use `query-datasource`. TADX does not expose a standalone query command or invoke MCP internally. | A1 §§3.2–3.4, 8.5, ADR-002/031; C1 §3 |
| Pulse current values, insight bundles, and briefs | Delegated to Tableau MCP | Use Pulse analytical MCP tools. TADX owns definitions/configuration, not analytical values. | A1 §§3.4, 8.5, ADR-023/031; C1 §3 |
| View/custom-view data and images | Delegated to Tableau MCP | Use view data/image tools; no TADX export/read command is admitted for parity. | A1 §§3.2–3.4; C1 §3 |
| Workbook semantic authoring/modification | Delegated to Tableau Desktop / Desktop MCP | TADX may pull/publish native artifacts but does not transform workbook XML to author content. | A1 §§1.6, 8.6; C1 §3 |
| Datasource field-description generation | Agent or selective skill | Reasoning/review may use MCP metadata/query results; only the exact reviewed write is a TADX primitive, and that primitive is blocked by B1. | A1 §§3.1, 4.4, 8.4; C1 §2.4 |
| Flow authoring | Out of scope for TADX; Tableau Prep/product surface | Flow lifecycle is admitted; custom semantic authoring is not. | A1 §§1.6, 8.6 equivalent product boundary; C1 §2.5 |
| Generic workbook/datasource/content deletion | Not admitted to TADX V1; raw REST remains available | Do not label MCP the preferred lifecycle owner merely because `delete-content` exists. Reconsider only through the capability-admission test. | A1 §§3.1–3.4, 9.3; S1 deletion correction |
| Project deletion | Deferred | Upstream deletion cascades project contents and exceeds the bounded shallow-project model. | A1 §§6.10, 8.9; C1 §§2.6, 7 |
| Permission mutation | Deferred | Permission inspection ships; add/delete/replace permission rules do not. | A1 §§1.5–1.6, 8.8, Appendix A; C1 §7 |
| TDS remote work-copy lifecycle, diff, and impact | Deferred fast follow | Wait for mature released work-copy management/listing. Pre-release behavior cannot enter V1 contracts. | A1 §§1.6, 13.3–13.5, ADR-030; C1 §7 |
| Generic lineage/dependency graph | Deferred / rejected as generic V1 subsystem | Preserve resource-native references only. No transitive discovery, acquisition, cycle handling, or graph serialization. | A1 §§8.9, 13.6, ADR-020; C1 §7 |
| Recursive project migration | Deferred | No child-project recursion, transitive dependency migration, rollback, or resume in V1. | A1 §§6.10, 13.6, ADR-021; C1 §7 |
| `pack` / `unpack` and Hyper ↔ CSV | Deferred | Native package expansion/rebuild and Hyper conversion move after V1. | A1 §§1.6, 13.1–13.2, ADR-029; C1 §7 |
| Generic bulk pull/publish | Deferred | Only bounded shallow-project composition is admitted. No generic bulk engine. | A1 §§1.6, 8.12; C1 §7 |
| Generic remote content move / hierarchy migration | Deferred | Local workspace move never mutates Tableau. V1 project update does not expose generic parent movement. | A1 §§1.6, 8.1, 9.3; C1 §7 |
| Flow run/schedule/cancel/task administration | Out of scope V1 | TADX is not a Prep Conductor operations console; selected read tools may remain available through MCP/raw APIs. | A1 capability-admission rule; C1 §§2.5, 7 |
| Long-tail Tableau administration | Not admitted | Schedules, alerts, Bridge, connected apps, identity pools, site/server settings, collections, revisions, acceleration, extensions, and similar endpoints remain raw API/product territory absent a new admitted capability. | A1 §§1.1, 1.6, 9.3; C1 §§1, 7 |
| OAuth/JWT/UAT/Connected App auth | Deferred | PAT-only V1. | A1 §§1.5–1.6, 5.4, 7.3; C1 §7 |
| MCP proxy inside TADX | Rejected invariant | CLI and MCP are peer tools. No TADX verb invokes or proxies MCP. | A1 §§1.3, 3.2, ADR-002/031; C1 §7 |
| Plugin system, background daemon/sync, offline mutation queue, Tableau Next | Deferred or out of scope as specified by A1 | One modular-monolith binary; no hidden replay/background process; Tableau Next excluded. | A1 §§1.6, 2.1, ADR-001; C1 §7 |

## 6. Blocking verification and architecture conflicts

### 6.1 Blocking verification gates

| ID | Affected registry rows | Required proof | Exit condition | Architecture consequence if unprovable |
| --- | --- | --- | --- | --- |
| B1 — Published datasource field-description write | `datasource.field-description.update`; indirectly `datasource.field-description.generate` | Exact released official endpoint/operation, API version, product/license/permission requirements, material request/response/error schema, and authoritative mapping from REST datasource LUID plus Metadata/VDS field record to the write target. A live fixture must cover renamed, hidden, calculated, remote, and similarly named external fields. | Captured official source plus positive/negative contract tests prove that the operation targets a published datasource field. REST external-table-column `Update Column`, private endpoints, and name-only mapping are rejected. | Potential architecture conflict: A1 promises V1 write-back after released-API verification. Escalate only after exhaustive official/live verification proves no supported released path exists. |
| B2 — Composable datasource round-trip and serialization | `datasource.get`, `datasource.pull`, `datasource.composition.update`, `datasource.publish` | A controlled Tableau 2026.2/API 3.29 fixture with multiple immediate parents: publish, download, inspect native references, resolve parent identities through released APIs, edit deterministically, republish to a clean project, and verify composition through released metadata/VDS behavior. | The fixture proves which downloaded fields are authoritative, how they map to `parentDataSourceUrls`, that only immediate parents are sent, and that deterministic local `.tds`/`.tdsx` serialization does not corrupt the package. | Composable datasource fidelity/authoring is an explicit V1 decision. Evidence of impossibility or unsafe private-format dependence must be escalated. |
| B3 — Pulse mutation schemas and destructive/idempotency behavior | `pulse.definition.create/update/delete`; `pulse.metric.create/update/delete/follow/unfollow` | Pinned official OpenAPI/operation artifacts plus live Cloud tests for exact request/response schemas, omitted-versus-null semantics, validation errors, create collisions, `getOrCreate` identity keys, definition/metric deletion cascades or constraints, subscription user/group shape, duplicate follow, and repeated unfollow. | Each mutation has fixture-backed preview fields, deterministic selector/idempotency rules, and negative tests for entitlement, permission, invalid references, and destructive dependencies. | Only the unsupported mutation is blocked; Pulse read/artifact operations remain admitted. A1 already limits mutations to current released API support. |
| B4 — Shallow project direct-content enumeration | `project.pull`, `project.publish` | Nested-project fixtures on Tableau Cloud and representative Server versions demonstrate the exact filters/queries that enumerate only direct workbooks, datasources, and flows in the selected project, with no child-project inclusion. | Contract tests prove bounded direct membership, pagination, duplicate-name handling, and stop-on-first-failure reporting. Explicit target mapping behavior is pinned for publish. | Do not weaken the shallow boundary or introduce recursion. Keep project pull/publish blocked until a deterministic direct-content contract exists. |

### 6.2 Architecture conflicts and resolutions

| Finding | Resolution in this contract |
| --- | --- |
| C1 proposed lightweight per-workspace locking. | Rejected. A1 §7.6 and ADR-032 make concurrent local races the caller's responsibility. No advisory/file lock is part of V1. |
| C1 used one mutation flag for local writes and remote changes. | Resolved by independent `Local write`, `Remote mutation`, and `Requires --apply` fields. Discovery gating applies only to remote mutations. |
| C1 combined public operations in coarse IDs and promoted internal member/composition mechanics. | Every public verb has one operation-level ID. Group member list/add/remove and datasource serialization/mapping remain implementation mechanics unless represented by the explicit public rows. |
| C1 added standalone `workbook check` because a validation endpoint exists. | Not admitted as a public V1 row. Workbook publish preview may invoke TWB validation internally when available; independent command admission requires demonstrated workflow demand. |
| C1 assigned generic workbook/datasource deletion to MCP because `delete-content` exists. | Not admitted to TADX V1 and not assigned to MCP as preferred lifecycle owner. Raw REST remains available; future admission must pass A1's capability test. |
| C1 external references `X1`–`X3` and supplied API aliases lack exact attached source artifacts. | Evidence is downgraded to docs-only. This is a reproducibility gap, not an architecture conflict. |

**Confirmed unresolved architecture conflict:** none. **Verification-sensitive potential conflict:** B1 only, if exhaustive evidence proves that no released supported published-datasource-field write path exists. B2 may also require escalation if native round-trip authoring cannot be made safe through released behavior. All other listed issues are implementation evidence gaps or resolved document-structure conflicts.
