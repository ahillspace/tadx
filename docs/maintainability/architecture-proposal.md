# Proposed TADX architecture

Status: approved by the maintainer on 2026-10-03; implementation and gate evidence remain pending.
Source baseline: `dd22c33bd1d123066f25ec09e2d612cd95717400`.
The working checkout and its uncommitted experiments are not the assessed source.

## Decision in brief

Adopt cohesive resource and service action packages across the whole repository.
Remove composition-layer operation facades and duplicate conversions by letting substantive resource adapters implement action contracts directly.
Give shared inventory, persistence, and publication coordination explicit owners instead of leaving those algorithms in app.
Keep native protocol clients and meaningful storage/security boundaries separate.

This is a structural refactor, not a collection of interface cleanups.
It changes where operations live, which packages can depend on each other, where records are defined, and which layers a maintainer must trace.
The [exact package and app-responsibility map](architecture-decision-map.json) is the authoritative proposed disposition; supporting ledgers explain the source evidence.

## Major changes

| Decision | Current structure | Proposed structure |
| --- | --- | --- |
| D1: consolidate workflows | 45 action package directories mix resource, service, and verb boundaries | 29 consolidated owners, plus one explicit site-consent owner: 30 action packages |
| D2: remove conversion bridges | Action ports often reach native clients through app adapters and resource adapters | Resource adapters implement action-owned ports directly; remove intermediate app conversions and forwarding-only wrappers |
| D3: reduce composition ownership | App contains credential transactions, inventory publication, resource rules, and recovery algorithms | App retains process startup, construction, command lifetime, and dispatch; named workflow/mechanism owners absorb the behavior |
| D4: consolidate shared mechanisms | Similar live/cache readers and publication coordination are spread through resource-specific app files | One neutral inventory coordinator; existing jobmonitor, operationrun, and lastcommand own their matching mechanisms |
| D5: unify internal observations | Some operations copy equivalent resource records across action/app/adapter boundaries | Share equivalent internal observations; retain native wire types and distinct operation output projections |
| D6: normalize navigation | Command construction, resource help, and tests sometimes live under unrelated categories or historical phases | One command-to-action convention, resource-named files, colocated command facts, and tests named for protected behavior |
| D7: replace obsolete dependency rules | Current rules prohibit adapters from importing their consumer action contracts | Explicitly permit that direction and selected meaningful infrastructure dependencies; continue rejecting reverse imports, cycles, CLI leakage, and direct action HTTP |

Every current non-test Go package directory has a proposed disposition.
Every one of the 42 app source files has a named ownership split.
The map projects 123 current package directories into 111 proposed directories, including five new owners.
These are structural counts, not promised code-size savings or quality targets.
Merging packages does not imply merging all their files or deleting distinct behavior.

## What the references contribute

GitHub CLI makes command navigation explicit and documents an execution trace from command name to implementation.
It places command-specific behavior near the command rather than in global utility packages.
Adopt that predictability and contributor documentation, not its exact per-command package tree.
See the pinned [GitHub CLI project guide](https://github.com/cli/cli/blob/6fc1c29d5477bfe71da7af290eb481c0df7811f1/docs/project-layout.md).

Helm separates command construction from named lifecycle actions and gives those actions explicit collaborators.
Adopt cohesive lifecycle ownership and state-oriented testing, not one giant action package for unrelated Tableau domains.
See the pinned [Helm action implementation](https://github.com/helm/helm/blob/53dfa521e019f7b832497066032406b9cda9d5d4/pkg/action/action.go) and the [previously assessed examples](../maintainability-assessment.md#reference-examples).

Neither reference means fewer packages at any cost.
The TADX-specific recommendation is fewer duplicated responsibilities and intermediary translations, with consistent ownership across categories.

## Target layout

The following tree lists the proposed action owners and the main surrounding boundaries.
Nested admin and Pulse directories are domain namespaces, not extra execution layers.

```text
cmd/
  tadx/                    Process entry point
  gencapdocs/              Generated capability references
actions/
  admin/
    user/                  User operations and user policy
    group/                 Group operations, including membership
    permission/            Permission inspection and changes
    labelcategory/         Shared label-category definitions
    labelvalue/            Shared label-value definitions
  pulse/
    definition/            Definition lifecycle and bundles
    metric/                Metric-variant lifecycle
    subscription/          Subscription discovery
  workbook/ datasource/ flow/ project/
  catalog/ contentlabel/ lineage/ search/ job/
  auth/ env/ workspace/ cache/ mutation/ policy/
  agent/ update/ version/ doctor/ session/ capability/ last/
internal/
  app/                     Construction, invocation lifetime, process dispatch
  cli/                     Cobra tree, parsing, command help, presentation binding
    content/ admin/ catalog/ pulse/ auth/ ...
  resources/               Substantive provider adapters implementing action ports
    workbook/ datasource/ flow/ project/ admin/
    lineage/ search/ pulse/ job/ contentlabel/
  tableau/                 Native HTTP/GraphQL contracts and response validation
  inventory/               Shared bounded collection and cache publication
  auth/ config/            Credential/session and configuration mechanisms
  artifact/ workspace/     Rooted artifacts and workspace persistence
  cache/                   Local storage, schema, and atomic generation handling
  jobmonitor/ operationrun/ Remote receipts versus detached process state
  lastcommand/ output/ toon/
  managedpolicy/ agent/ update/
  value/ identity/ paging/  Shared observations and bounded mechanics
  ...                      Other retained named leaf mechanisms
```

The complete map explicitly accounts for the packages abbreviated in this tree.
Do not create an empty adapter package for every action or a wrapper merely to complete a directory pattern.
Shared provider packages can implement several related consumer ports when they genuinely share native resolution behavior.
For example, resources/admin serves user, group, permission, and label-definition contracts without becoming their workflow owner.
Remove resources/catalog: its six tag wrappers can disappear when explicit catalog operations use the existing shared LabelTarget contract.
App can bind the native metadata client directly to those narrow value-based ports; the action does not import the client package.

### One action package convention

Each resource or service package owns named operations, their narrow dependencies, input validation, sequencing, outcomes, and operation projections.
Use a small resource/service constructor with named operation entry points and explicit dependencies, not a global runtime object.
Operations can retain separate files and distinct input/output types.
Required phases and capabilities must appear in declared contracts, not optional type assertions.
Acquire target-bound providers lazily after input validation; help and invalid input must not authenticate or contact Tableau.
App supplies construction functions where lazy setup is necessary, not a forwarding method for every operation.

Use consumer-owned interfaces at external or substitution boundaries, not an interface for every internal function.
Local services can consume meaningful infrastructure APIs through narrow dependencies without inventing remote-style adapter layers.
No action imports another action package or dispatches another CLI command.
Cross-resource workflows use neutral observations and injected operations rather than importing sibling workflows.

Catalog is a cohesive metadata service rather than a verb grouping.
Its existing audit traverses databases, tables, columns, and datasource descriptions; its search and identity/page contracts also span those resource kinds.
Consolidate its role-based packages into catalog, keeping typed resource operations in separate files.
Do not introduce a new generic kind-switch runner.
This follows the same resource-or-service ownership rule as authentication and session overview; it does not exempt a category from dependency conventions.

## Exact action consolidation

| Existing packages | Proposed owner | Behavior retained |
| --- | --- | --- |
| project/create, delete, inspect, list, move, update | actions/project | All six explicit operations, freshness phases, collisions, hierarchy semantics, and distinct projections |
| auth/check, login, logout, status | actions/auth | Preflight, authentication, local status, storage ordering, compensation, and refusal behavior |
| agent/install, uninstall | actions/agent | Separate installation/removal sequences and partial outcomes |
| cache/refresh, status | actions/cache | Named management workflows, not every resource's cached-read implementation |
| capability/get, list | actions/capability | Discovery and readiness, independent of Cobra syntax |
| catalog/audit, read, search, update | actions/catalog | Typed metadata operations, graph audit/search, tags, and identity contracts |
| admin/group/member plus admin/group | actions/admin/group | Membership operations remain explicit and separately testable |
| admin/permission/inspect plus admin/permission | actions/admin/permission | Inspection and rule mutation retain different contracts |
| doctor/run; env/profile; lineage/pull; session/overview; version/get | actions/doctor; env; lineage; session; version | Remove redundant namespace levels; retain existing public CLI routes |
| pulse/subscription/list | actions/pulse/subscription | Subscription responsibility, not a single-verb namespace |
| Existing resource-cohesive packages | Same owners | Workbook, datasource, flow, admin user/labels, Pulse definition/metric, workspace, and other cohesive services remain |
| Site-consent behavior currently in app | actions/mutation | Status and saved consent changes; distinct from managed policy |

The map contains each exact source package, including every retained action owner.
Do not retain deprecated forwarding packages merely to preserve the old directory shape.
Before changing exported Go import paths, identify any supported external Go consumers and approve the compatibility boundary explicitly.
Public CLI behavior, persisted formats, and native operation contracts remain protected regardless of internal package movement.

## Which handoffs disappear

The Workbook move pilot exposes the difference between consolidation and renaming:

```text
Current execution:
  command -> app operation facade -> action
          -> app mutation adapter -> resource forwarding adapter -> native client

Proposed execution:
  command -> resource action -> substantive resource adapter -> native client

Construction, outside the operation chain:
  app creates/binds the action and its providers for one invocation
```

Remove app-owned workbook/datasource mutation conversions and equivalent forwarding-only MutationAdapter objects after their provider contracts are implemented directly.
Keep substantive schema identity validation, hierarchy traversal, project-filter pagination, and API-to-observation conversion.
These behaviors do not become unnecessary merely because the surrounding types look similar.
See the [content evidence](content-domains-assessment.json) and [Workbook pilot](workbook-pilot.json).

Adapters depend on the narrow consumer action contracts; actions do not depend on those adapters.
This is a Go import direction, not the direction of runtime calls through interfaces.
Where a native client already satisfies a value-based port without semantic translation, inject it directly and omit an adapter layer.
Resources must not call action workflow entry points.
Native clients must not import actions, CLI, or resources.
The present [resource import rule](https://github.com/ahillspace/tadx/blob/dd22c33bd1d123066f25ec09e2d612cd95717400/internal/architecture/architecture.go#L240) must change through an approved gate update before migration.

Moving adapters into action packages was considered and rejected for this proposal.
That would couple the whole Go workflow package to provider implementations without eliminating required normalization or shared hierarchy behavior.
The direct-port approach removes an actual intermediate layer while retaining a useful boundary.

## Responsibilities removed from app

| Current responsibility | Proposed owner | Why it is not just wiring |
| --- | --- | --- |
| Login/logout configuration and keyring coordination | actions/auth, using config and core auth dependencies | Ordering, compensation, target freshness, and recovery affect user outcomes |
| Environment profile operations and auth-status projection | actions/env and actions/auth respectively | Configuration changes and credential handling have separate behavioral contracts |
| Administrative rules, membership planning, permission sequencing | Matching actions/admin resource | These determine what to change or refuse |
| Pulse definition/metric workflows and provider conversions | Matching Pulse action plus resources/pulse | Preserve field validation, reconciliation, and exact payload contracts |
| Live/cache collection, completeness, and publication | New internal/inventory; typed read ports in resource adapters | Partial results and cache replacement decisions need one shared implementation |
| Accepted native-job receipts and observation coordination | Existing internal/jobmonitor, with injected native observers | Acceptance must survive cancellation without reissuing writes |
| Worker launch/liveness/persisted execution state | Existing internal/operationrun; app keeps private process dispatch | Detached process state is not a Tableau job or resource state |
| Publish completion and destination verification | Resource workflows and resource adapters | Each resource owns its requested final effect and artifact updates |
| User job recovery and result reconciliation | actions/job, resources/job, and shared output mechanisms | Combines evidence without confusing accepted, pending, failed, and verified effects |
| Saved-result capture/persistence | lastcommand with snapshot callbacks; output retains encoding | Saving a receipt must not redefine the remote operation's outcome |
| Site consent status/set | New actions/mutation, backed by config | Canonical site consent is a workflow and authorization contract |

The [app file map](architecture-decision-map.json) gives destinations and split rules for all 42 source files.
Multiple destinations mean extract responsibilities, not copy the file into several packages.
Invocation-scoped session/client construction stays in app; no new global service locator or replacement runtime monolith is proposed.

The new inventory coordinator depends on neutral records and injected collection/storage contracts, not action-specific outputs.
Move or reuse shared observations when necessary instead of creating parallel copies solely to satisfy import rules.
Keep Tableau collection/retry mechanics in tableau/cache and durable storage in internal/cache.
Preserve filtered-upsert versus complete-scope replacement and explicit partial-coverage behavior.

Publication extraction uses existing mechanism owners, not a new universal workflow engine.
Progress becomes an injected observer; mechanism packages do not import CLI presentation.
Resource-specific completion must stay explicit, including synchronous fallback, accepted-job monitoring, and artifact state updates.
Detailed worker/recovery state-machine characterization is required before moving this high-risk slice.

## Records and output

Give equivalent project observations one shared internal definition, following existing workbook/datasource/flow observations where their semantics match.
Use shared value records only when there are real cross-boundary consumers.
Keep operation-local input, plan, and output types with their action.
Native XML/GraphQL request/response types stay with native clients when they express different protocol semantics.

Do not merge compact/full projections into internal records.
Preserve nil versus empty, omitted versus explicitly supplied values, confirmed identities, request IDs, and partial/unknown outcomes.
Structural consolidation cannot silently fix the pilot's unconfirmed correctness concerns or change output contracts.

## Commands, tests, and contributor navigation

Retain category-level CLI packages and use resource-named files consistently within them.
Split long files by resource and operation, not historical review phase.
Move catalog-mounted lineage and label constructors from cli/content into cli/catalog without changing the command paths.
Move resource-specific help facts/examples beside their constructors; common help rendering stays shared.
Keep capability policy/readiness metadata separate from executable syntax and validate their bindings.
Do not create a second command registry as part of this refactor.

Keep operation tests with action owners and protocol tests with clients/adapters.
Keep app.Run integration tests at the process boundary, organized by user behavior or resource.
Do not relocate an integration test into an action package where it would import app and create a cycle.
Move meaningful assertions from stage-named tests only after recording the protected contract and replacement location.
No test removal is approved by this proposal.

Add a human-readable architecture guide and an end-to-end contributor walkthrough linked from CONTRIBUTING.
Show one read, one mutation, and one local-state change, including test locations and dependency direction.
Update AGENTS and maintained engineering references to the same approved model.
Replace content-only migration guidance with repository-wide completion criteria.
Regenerate marked documentation through its existing tooling; do not hand-edit generated artifacts.

## Proposed complete traces

| User operation | Workflow owner | Provider/mechanism owner | Evidence that must survive |
| --- | --- | --- | --- |
| content workbook list | actions/workbook | resources/workbook + inventory + Tableau/cache providers | Correct target, bounded rows, provenance, completeness, and unchanged projections |
| content project move | actions/project | resources/project -> tableau/project | Fresh phases, collision/cycle refusal, exact parent, confirmed mutation, optional path enrichment |
| admin user update | actions/admin/user | resources/admin -> tableau/admin | Exact user and requested fields, platform/caller restrictions, truthful returned effects |
| pulse definition publish | actions/pulse/definition | resources/pulse + artifact + native Pulse clients | Valid field identities, intended definition effect, reconciliation, and saved bundle evidence |
| auth login/logout | actions/auth | config + core auth + injected native authentication | Pre-prompt validation, target freshness, explicit credential persistence approval, compensation, and redaction |
| workspace remove | actions/workspace | workspace + artifact + config mechanisms | Exact rooted scope, unmanaged-file protection, atomic registry behavior, and accurate partial outcome |

Every trace starts at a thin category command and uses the same ownership rules.
Different providers and safety sequences are behavior differences, not category-specific architectures.

## Migration sequence and approval boundaries

The maintainer approved this sequence on 2026-10-03.
Gate construction and effectiveness demonstrations precede production migration.

1. Approve D1-D7, the package map, and supported compatibility boundaries; revalidate the pinned baseline against the chosen implementation revision.
2. Build G0-G9 verification infrastructure and contract fixtures before migration; prove earlier gates reject controlled seeded failures.
3. Consolidate remaining action namespaces and their contracts across categories, retaining unchanged behavior and migration tracking until all categories conform.
4. Establish direct adapter ports in one bounded resource slice, then apply the same pattern across content, administration, catalog, and Pulse.
5. Extract local service coordination and shared inventory behavior, with shared-consumer tests before each checkpoint.
6. Extract publication, worker, and recovery responsibilities after their failure-state fixtures and cross-resource contracts are demonstrated.
7. Finish CLI/help/test organization and contributor documentation, then perform the repository-wide consistency and maintainer-navigation acceptance.
8. Complete the full final-build Luna-medium live sweep and accept only with the required evidence.

Every production checkpoint requires its preceding gates, then Luna at medium effort on every affected executable action, including shared-dependency consumers.
The final sweep covers every executable action on the exact final build, not accumulated results from earlier builds.
Native acknowledgement and affected-state evidence determine completion; answer formatting alone does not determine failure.
Excluded or unavailable cases never count as passes.
Use the approved fixture, consent, cleanup, and waiver rules in [G9](regression-gates.md#final-live-verification).

Changing an acceptance rule requires maintainer approval and replacement rejection tests.
No blanket rule relaxation, skipped gates, or self-approved golden updates can make a migration acceptable.
No finite suite guarantees zero regressions.

Test-first gate construction precedes the first production migration.
The maintainer delegates orchestration and review to this session, with independent in-session review instead of a separate ChatGPT review.
This delegation does not waive verification requirements or authorize changes to live fixture consent.
Temporary dual structures need bounded migration entries and exit conditions, not permanent category exceptions.
The end condition removes obsolete bridges and per-verb packages wherever the approved map consolidates them.

## Evidence and limits

The proposal reconciles the original pilot with [content](content-domains-assessment.json), [remote domains](remote-domains-assessment.json), [local services](local-services-assessment.json), and [shared architecture](shared-architecture-assessment.json) evidence.
Those ledgers record inspected paths, concrete decisions, representative tests, and limitations.
The [package map](architecture-decision-map.json) accounts for every current package directory; accounting does not establish exhaustive implementation review.
The original [pilot coverage ledger](coverage-ledger.json) remains a pilot-only record and is not relabeled as a completed test audit.

Remaining verification includes exhaustive contract-to-test mapping for each migration slice, compiler-level dependency validation, platform/runtime behavior, and live execution.
No source changes, Go tests, live Tableau requests, model tasks, installer operations, or pushes occurred during the original proposal task.
The [implementation record](implementation-status.md) tracks subsequent work and outstanding acceptance requirements.
Assessment-tool checks validate artifact accounting and source references, not the proposed application's behavior.

Reproduce structural and evidence accounting from the repository root:

```powershell
python -B docs/maintainability/validate_architecture.py
python -B docs/maintainability/validate_assessment.py
python -B -m unittest discover -s docs/maintainability -p "test_*.py"
```

The structural validator checks all 123 package assignments, all 42 app-file assignments, evidence locations, and named test references.
It does not compile hypothetical package moves or establish that the proposed dependencies work at runtime.

Approval of this document settles the target architecture and migration direction.
It does not establish that gates pass or authorize live mutations, credential persistence, releases, or unbounded cleanup.
