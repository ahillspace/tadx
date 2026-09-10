# Find the right implementation layer

All paths below are relative to the repository root.
Choose the closest existing capability for your task; these are starting examples, not a mandatory reading list.
Read only the layers you need to change, then their focused tests.

## Two complete paths to follow

| Layer | Bounded read: `workbook.list` | Previewable write: `workbook.delete` |
| --- | --- | --- |
| Action input/output and behavior | `actions/workbook/list/types.go`, `action.go` | `actions/workbook/delete/types.go`, `validation.go`, `action.go` |
| Resource adapter | `internal/resources/workbook/adapter.go` | Same file: exact delete boundary |
| HTTP provider | `internal/tableau/workbook/client.go` | Same file: delete through shared transport |
| Cobra plumbing | `internal/cli/content/workbook_inventory.go` | `internal/cli/content/workbook_delete.go` |
| Composition | `internal/app/workbook_inventory.go`, `content_remote.go` | `internal/app/content_remote.go` |
| App/HTTP regression | `internal/app/inventory_all_e2e_test.go` | `internal/app/group3_workbook_delete_e2e_test.go` |

For a new action, define its typed input/output and narrow dependency interface in the action package first.
Write the externally visible failing test, implement provider behavior and resource normalization where needed, then wire the existing seams through the app.
Do not copy the example's inventory collection or destructive behavior into an unrelated operation.
Package depth follows the existing domain: `actions/search`, `actions/last`, and `actions/admin/group/member/add` are also valid isolated actions.

## Shared infrastructure

| Need | Start here |
| --- | --- |
| Invocation-scoped clients and sessions | `internal/app/command_runtime.go`, `internal/auth/command_sessions.go` |
| PAT provider and credential coordination | `internal/auth/auth.go` |
| HTTP bounds, auth headers, request IDs, upstream errors, redaction | `internal/tableau/transport.go` |
| Structured errors and CLI wrapping | `internal/errs/errs.go`, `internal/cli/clierr/clierr.go` |
| Shared rendering and page presentation | `internal/output/output.go`, `internal/output/page.go` |
| Bounded collection and logical result windows | `internal/paging/collect.go`, `internal/paging/window.go` |
| Exact identity and shared resource records | `internal/identity/identity.go`, `internal/value/` |
| Safe follow-up commands | `internal/commandhint/command.go` |
| Live/catalog provenance | `internal/readsource/` |

## Registration and verification

`internal/capability/definition.go` defines registry types; `definitions.go` is the manually maintained source of capability and implementation facts.
`internal/capability/registry.go` validates the registry and declares generation through `cmd/gencapdocs`.
Cobra leaves carry `tadx.capability` annotations; `internal/cli/root.go` derives registrations from the actual tree and `internal/app/app.go` validates bindings.
New command and flag names also need shorthand coverage in `internal/cli/shorthand.go`.

Use adjacent action/resource/provider/CLI tests for the changed path.
`internal/cli/preview_contract_test.go` and `mutation_policy_test.go` protect the shared mutation contract.
`internal/capability/generate_test.go` detects stale generated Markdown/JSON.
`internal/architecture/architecture.go` and `architecture_test.go` enforce package import boundaries; do not weaken them to fit misplaced logic.
The build skill's final checks apply after integration; no comprehensive agent review is automatically part of the build.
