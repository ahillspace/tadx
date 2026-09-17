# Search: Modern Go Guidelines audit

Updated: 2026-09-14.
Baseline: `33de8d4`, inspected before the search identity repair; recommendation locations refer to that baseline.
Status: the separate search identity repair is implemented and validated in the working tree, not released; optional audit recommendations remain unapplied.
Tool: `modern-go-guidelines` plugin 1.1.1, underlying `github.com/JetBrains/go-modern-guidelines` CLI v0.1.1.
Target: Go 1.26, as declared in `go.mod`.

The complete, unfiltered `list --go-version 1.26` output was read, followed by `explain` for the relevant guideline IDs below.
This is a manual application of the plugin's guidance, not an automated source-code analyzer report.
No Go changes were made for this audit.
All recommendations are low priority modernization; none establishes the cause of the datasource search bug or a measured performance problem.

## Recommendations

| Guideline ID | Locations | Proposed change and constraint |
| --- | --- | --- |
| `testing_t_context` | `actions/search/action_test.go:21`; `internal/resources/search/adapter_test.go:30`; `internal/resources/search/native_test.go:42`; `internal/tableau/search/client_test.go:65`; `internal/tableau/search/live_contract_test.go:29,62`; `internal/app/search_test.go:27,324`; `internal/app/cache_review_test.go:18` | Replace ordinary `context.Background()` uses throughout these reviewed tests with `t.Context()`, including live authentication and HTTP calls. This gives work a test-lifetime cancellation boundary. It does not add an HTTP timeout or replace deliberate cancellation-test contexts. |
| `errors_as_type` | `actions/search/action_test.go:103,119,213`; `internal/app/search_test.go:42` | Use `typed, ok := errors.AsType[*errs.Error](err)` for these concrete error assertions. This removes pointer-to-target temporaries while preserving wrapped error matching. Do not mechanically extend this to the marker interfaces discussed below. |
| `new_expression` | `actions/search/action.go:50,224`; `actions/search/validation.go:12`; `actions/catalog/search/action.go:247` | Replace these `errs.Bool(false)` calls with `new(false)`. The helper at `internal/errs/errs.go:111` only returns the address of its argument, so there is no additional behavior to preserve. Keep the resulting non-nil false pointer; a nil pointer has a different optional-field meaning. Removing the shared helper repository-wide is outside this audit. |
| `slices_sort`, `slices_sort_func` | `internal/resources/search/adapter.go:73,119`; `internal/resources/search/native.go:135`; `internal/tableau/search/client.go:167`; `internal/app/search.go:511`; `internal/tableau/datasource/client.go:91,494` | Use `slices.Sort` for existing string sorting, including datasource tags. Replace the adapter's `sort.Slice` with `slices.SortFunc`, comparing `Name` and then `LUID` with `cmp.Compare`. Preserve case-sensitive lexical order, both comparison keys, and the existing clone before sorting. Do not sort native relevance results. |
| `slices_clone` | `internal/app/search.go:507`; `internal/resources/search/native_test.go:25` | Replace `append([]string(nil), values...)` copies with `slices.Clone(values)`. These copies preserve nil today, so the helper preserves that behavior. Several other append copies deliberately produce non-nil empty slices and are excluded below. |
| `cmp_or` | `actions/search/action.go:102`; `internal/resources/search/native.go:108`; `internal/tableau/search/client.go:281,285,289,296,300,304`; `actions/catalog/search/action.go:110,183` | Express simple existing fallbacks as `cmp.Or`, such as source-or-`"live"`, owner-name-or-owner-LUID, name-or-title, nested identity fields, default limit, and metadata-ID-or-LUID. Preserve fallback precedence and trim only where the original code trims. Keep the conditional project-container fallback and catalog `--all` override explicit. |
| `range_over_int` | `actions/search/action.go:144`; `actions/catalog/search/action.go:139` | Use `for pages := range 100` and `for pageNumber := range 1000` for these fixed-count scans. Preserve cancellation, continuation checks, early exits, and final-page errors. The adapter's condition-based traversal and the datasource resolver's stepped batch loop are not equivalent candidates. |
| `min_max` | `internal/tableau/datasource/client.go:94` | Replace the batch-end clamp with `end := min(start+maxContentURLResolveBatch, len(unique))`. Preserve the batch step and REST page size. |
| `json_omitzero` | `actions/search/types.go:15`; `internal/resources/search/adapter.go:48`; `internal/app/search.go:493,741` | Use `omitzero` for these optional integer fields (`Total`, `Offset`, `SourceLimit`, `DefinitionIndex`). Zero omission is unchanged for integers. Preserve field names and order, and verify existing output and cursor serialization fixtures if implemented. Keep `omitempty` for empty strings, slices, and optional pointer fields. |
| `maps_keys_values_iter` | `internal/app/search.go:335` | `for observation := range maps.Values(lister.observations)` expresses value-only iteration. This is a small style change with no promised allocation improvement over the current direct map loop. Map iteration remains unordered, and generation-consistency checks must remain intact. |

## Patterns deliberately left alone

- **Marker-interface error matching:** `actions/search/action.go:68`, `internal/app/search.go:378`, and `internal/tableau/search/client.go:349` use narrow interfaces such as `interface{ HTTPStatus() int }` and `interface{ InvalidSearchCursor() bool }`.
  Local `go doc errors.AsType` confirms its constraint is `E error`, which these interfaces do not satisfy because they omit `Error() string`.
  Retain `errors.As` here and in the equivalent marker-interface assertions in the reviewed tests unless the interface contracts are deliberately redesigned.
  Embedding `error` just to apply the guideline changes the matched interface type and can affect custom `As` implementations.
- **Empty versus nil slices:** `actions/search/action.go:87,106`, `internal/resources/search/adapter.go:72,111`, and `internal/resources/search/native.go:122` use non-nil empty append destinations.
  A bare `slices.Clone(nil)` would return nil instead.
  Search tests explicitly require non-nil empty items at `actions/search/action_test.go:133,201`; changing that can alter `[]` versus `null` output and page digests.
  Keep the current copies unless an equally explicit normalization is retained.
- **Fallback semantics:** `internal/app/search.go:787` falls back from a whitespace-only metric name; `cmp.Or` alone tests zero strings and would not preserve that condition.
  Likewise, moving `TrimSpace` inside the raw-content fallback expressions changes which values win.
- **Parsing and HTTP tests:** `internal/tableau/search/client.go:336` indexes the first two components of an API version; it does not iterate over split output, so `strings_split_seq` does not apply.
  The HTTP tests use a single `http.HandlerFunc` with explicit unexpected-request assertions; there is no existing `ServeMux` router to modernize using `http_servemux_patterns`.
- **Existing modern patterns:** search already uses `any`, range over slices, and built-in `min` in bounded aggregation; catalog search already uses `slices.Contains`.
  Slice projection loops transform values and should not become clones.
  No applicable wait-group, atomic, timer, benchmark-loop, or reflection-type modernization was found in the reviewed scope.

## Coverage and limits

Read in full: `actions/search/{action,types,validation,action_test}.go`, `internal/resources/search/{adapter,native,adapter_test,native_test}.go`, `internal/tableau/search/{client,client_test,live_contract_test}.go`, `internal/cli/search.go`, `internal/app/{search,search_test,cache_review_test}.go`, and `actions/catalog/search/action.go`.
Additional inspected excerpts: datasource REST types, `ResolveContentURLs`, filter construction, and normalization (`internal/tableau/datasource/client.go:1-135,425-538`); Pulse client/list operations (`internal/tableau/pulse/client.go:1-173`); admin REST client/list operations (`internal/tableau/admin/client.go:1-168`); app workbook inventory (`internal/app/workbook_inventory.go:1-130`), admin user listing (`internal/app/admin.go:64-159`), and catalog search wiring (`internal/app/catalog.go:152-163`); output JSON normalization (`internal/output/output.go:265-278`); and `errs.Bool` (`internal/errs/errs.go:111`).
The inspected CLI wiring, Pulse/admin list projections, and inventory/catalog composition excerpts yielded no additional plugin-based changes.
Referenced dependencies not read in full are not certified by this audit.
Tests were read, not run as part of this documentation-only audit; functional repair validation is separate.

This sample supports using the plugin during focused edits and suggests a repository-wide scan would mostly identify mechanical modernization.
It does not establish that a blanket rewrite would improve correctness or speed.
The interface constraint and nil-slice examples show why each proposed replacement still needs contextual review.
