# Flow automation scope

This document records the V1 boundary for Tableau Prep flow automation.
It is a scope decision, not upstream API evidence.

## V1 capabilities

| Area | V1 decision | Contract |
|---|---|---|
| Inventory and discovery | Yes | List and get flows with exact selectors, bounded pagination, project, owner, tags, parameters, and output-step identifiers. |
| Publish and deploy | Yes | Publish or overwrite `.tfl` and `.tflx` artifacts without rewriting their contents. |
| Content lifecycle | Yes | Download, delete, and move flows between projects. |
| Lineage and impact analysis | Yes | Capture bounded read-only lineage automatically with flow pulls and support an explicit lineage pull action. |

TADX preserves downloaded flow packages unchanged.
A flow can reference published datasources, files, databases, or other supported Tableau inputs.
TADX does not rewrite connections, credentials, or published datasource bindings inside a flow package.
Tableau remains authoritative for accepting or rejecting a published flow package.

Flow ownership changes belong to the later administration work, not the V1 flow lifecycle slice.
Flow deletion in V1 covers the resource action only.
Recycle Bin listing, restoration, permanent purge, and general cleanup workflows remain deferred.

## Deferred capabilities

| Area | Decision | Reason |
|---|---|---|
| Run flows | Not now | TADX does not run all output steps, selected output steps, incremental runs, or parameter overrides in V1. |
| Run orchestration | Not now | TADX does not start scheduled flow tasks or linked-task sequences in V1. |
| Schedule flows | Not now | TADX does not create, change, or attach flow schedules in V1. |
| Monitor executions | Not now | TADX does not list or inspect flow run history in V1. |
| Cancel work | Not now | TADX does not cancel flow runs or their background jobs in V1. |
| Manage connections | Not now | TADX does not edit flow input or output connections in V1. |
| Manage permissions | Not now | TADX does not grant, revoke, or change flow permissions in V1. |
| Tags and governance | Not now | TADX does not mutate flow tags, data labels, or quality warnings in V1. |
| Failure monitoring | Not now | TADX does not configure flow-run-failure quality-warning triggers in V1. |
| Migration credentials | Not now | TADX does not download or upload encrypted flow keychains in V1. |
| Deletion recovery | Not now | TADX does not list, restore, or permanently purge Recycle Bin entries in V1. |

## Lineage behavior

Automatic lineage capture rides with flow downloads.
The artifact metadata records capture status, bounded summary counts, and a relative pointer to the graph sidecar.
The standard successful CLI response omits the graph and other lineage detail.
`--full` returns bounded lineage detail when that command defines it.
Partial or unavailable lineage capture produces a warning and never presents an incomplete graph as complete.

An explicit lineage pull creates the normal metadata structure and lineage sidecar without downloading the native flow package.
Metadata API identifiers remain distinct from REST LUIDs and never identify a write target.

## Evidence rule

Captured local Tableau API documentation is the first discovery source for each admitted action.
An agent searches only the relevant operation or schema section and does not load a complete reference file.
If the local capture is missing, ambiguous, or version-sensitive, use current official Tableau documentation.
No live API implementation starts until the exact upstream contract is recorded and covered by a focused contract test.
