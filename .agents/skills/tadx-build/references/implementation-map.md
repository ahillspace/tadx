# Find the right implementation layer

All paths below are relative to the repository root.
Choose the closest existing capability for your task; these are starting examples, not a mandatory reading list.
Read only the layers you need to change, then their focused tests.

## Two complete paths to follow

| Layer | Bounded read: `project.list` | Previewable write: `workbook.delete` |
| --- | --- | --- |
| Action input/output and behavior | `actions/project/service.go`, `workflow.go`, `list.go`, `list_types.go` | `actions/workbook/service.go`, `delete.go`, `delete_types.go` |
| Resource adapter | `internal/resources/project/ports.go`, `adapter.go`, `cache.go` | `internal/resources/workbook/mutation_port.go` |
| HTTP provider | `internal/tableau/project/client.go` | `internal/tableau/workbook/client.go` |
| Cobra plumbing | `internal/cli/content/project.go` | `internal/cli/content/workbook_delete.go` |
| Composition | `internal/app/project_provider.go` | `internal/app/content_mutation_provider.go` |
| App/HTTP regression | `internal/app/inventory_all_e2e_test.go` | `internal/app/workbook_delete_e2e_test.go` |

These paths show responsibilities, not a requirement to add a file or forwarding layer at every step.
For a new operation, locate the existing owner and define only the contracts it needs.
For new behavior, write the externally visible failing test, then extend provider behavior and resource normalization where needed.
For refactors, reuse existing tests and characterize uncovered behavior before changing it.
Do not copy the example's inventory collection or destructive behavior into an unrelated operation.
Workbook, datasource, and flow group their operations by resource; share internal records without merging distinct CLI projections.
Admin membership belongs to `actions/admin/group`; permission inspection and mutations belong to `actions/admin/permission`.
Both services keep explicit operation entry points and distinct outcome contracts.
Pulse definitions and metrics, environment profiles, Workspace, and Jobs also use cohesive operation packages.
The `actions/catalog` service owns typed metadata reads, changes, search, and audit, with distinct operation sequences.
Other domains retain assessed boundaries, including `actions/search` and `actions/last`.
The `actions/mutation` service owns exact-site consent status and changes, while `internal/config` owns persistence and locking.
Consult [the repository structure](../../../../docs/repository-structure.md) for the full layout.

## Shared infrastructure

| Need | Start here |
| --- | --- |
| Invocation-scoped clients and sessions | `internal/app/command_runtime.go`, `internal/auth/command_sessions.go` |
| PAT resolution and native credential storage | `internal/auth/auth.go` |
| Credential/configuration ordering and compensation | `actions/auth/persistence.go` |
| HTTP bounds, auth headers, request IDs, upstream errors, redaction | `internal/tableau/transport.go` |
| Structured errors and CLI wrapping | `internal/errs/errs.go`, `internal/cli/clierr/clierr.go` |
| Shared rendering and page presentation | `internal/output/output.go`, `internal/output/page.go` |
| Bounded collection and logical result windows | `internal/paging/collect.go`, `internal/paging/window.go` |
| Metadata traversal retaining partial evidence | `internal/paging/metadata_collect.go` |
| Exact identity and shared resource records | `internal/identity/identity.go`, `internal/value/` |
| Safe follow-up commands | `internal/commandhint/command.go` |
| Live/cache provenance | `internal/readsource/` |
| Shared bounded inventory and scoped cache publication | `internal/inventory/` |
| Accepted Tableau job receipts and observation | `internal/jobmonitor/publication.go` |
| Detached worker state and coordination | `internal/operationrun/coordinator.go`, `worker.go` |
| Saved-operation recovery without resubmission | `actions/job/operation_recovery.go`, `operation_receipts.go` |
| Resource publication and destination confirmation | `actions/workbook/publish_service.go`, `internal/resources/workbook/publish_destination.go` |
| Category-owned help facts and shared rendering metadata | `internal/cli/<category>/help_facts.go`, `internal/cli/helpmeta/` |
| Upstream Catalog metadata and label contracts | `internal/tableau/metadataassets/`, `internal/value/metadata.go` |

## Registration and verification

`internal/capability/definition.go` defines registry types; `definitions.go` and `metadata_definitions.go` contain the manually maintained capability and implementation facts.
`internal/capability/registry.go` validates the registry and declares generation through `cmd/gencapdocs`.
Cobra leaves carry `tadx.capability` annotations; `internal/cli/root.go` derives registrations from the actual tree and `internal/app/app.go` validates bindings.
New command and flag names also need shorthand coverage in `internal/cli/shorthand.go`.

`cache` means local SQLite observations; `catalog` means upstream Tableau metadata.
Metadata GraphQL is read-only; supported metadata edits use released REST methods, and Metadata IDs are not REST LUIDs.
Keep provider errors below the action layer; translate acknowledged-write evidence into structured operation outcomes in actions, not by importing action error types into providers.

Use adjacent action/resource/provider/CLI tests for the changed path.
`internal/cli/preview_contract_test.go` and `mutation_policy_test.go` protect the shared mutation contract.
`internal/capability/generate_test.go` detects stale generated Markdown/JSON.
`internal/architecture/architecture.go` and `architecture_test.go` enforce package import boundaries; do not weaken them to fit misplaced logic.
The build skill's final checks apply after integration; no comprehensive agent review is automatically part of the build.
