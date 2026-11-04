# Pulse term search contract

This record supports bounded Pulse discovery without enumerating metric inventories under unrelated definitions.
It applies to live search terms on Tableau Cloud; no-term inventory and explicitly selected local cache remain separate operations.

## Upstream sources

- [Content exploration methods](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_content_exploration.htm) documents the independently versioned `GET /api/-/search` endpoint.
- [Pulse API reference](https://help.tableau.com/current/api/rest_api/en-us/REST/TAG/index.html#tag/Pulse-Methods/operation/MetricQueryService_ListDefinitions) documents definition filtering and provider pagination.
- Tableau's public Pulse frontend at `/pulse/assets/App-DTkp3wqL.js` provided the observed `insightmetric` search category and `name:has:<encoded-term>` definition-filter syntax on 2026-11-04.
  That asset is a versioned implementation observation, not a stable public SDK contract.

## Bounded authenticated observations

On 2026-11-04, PAT-authenticated `GET /api/-/search` requests using search-results v2 JSON, the selected site's header, `terms`, `filter=type:eq:insightmetric`, `limit`, and `page=0` returned HTTP 200.
One empty-result term returned zero Pulse hits, while a positive term returned a bounded page with a reported total of 104 hits.
The first positive hit contained `metricId`, `definitionId`, `title`, and `type=insightmetric`.
An exact Pulse metric read confirmed both the returned metric ID and definition ID.
Generic index identifiers were not used as lifecycle identities.

A definitions request with `filter=name:has:test`, `page_size=20`, and a full definition view returned HTTP 200 with no matching definitions.
Bare `filter=test` returned HTTP 400; the documented description alone was insufficient to establish the accepted grammar.
The frontend constructs `name:has:` with an inner `encodeURIComponent` term before the SDK performs ordinary URL query encoding.

These observations establish the tested Cloud deployment's endpoint behavior.
They do not establish the earliest supported Cloud release, a universal rate-limit quota, or that every saved metric variant appears in the search index.
The frontend's own search additionally restricts hits to default or followed metrics; TADX does not add that restriction.
Search remains subject to Tableau permissions, index freshness, provider pagination, and the native 2,000-hit window.

## TADX guarantees and regression evidence

Term-based metric discovery uses indexed native search and never calls the per-definition metric-list endpoint.
Definition terms are sent upstream through `name:has:` and never cause an unfiltered definition inventory scan.
Combined native queries may include content and metrics in one request, while `--type content` excludes Pulse metrics.
`metricId` and `title` are required for native Pulse result identity and display.
Failures remain errors without an automatic inventory or cache fallback, and upstream retry advice is preserved.

Provider tests in `internal/tableau/search/metric_test.go` and `internal/tableau/pulse/search_definitions_test.go` cover mixed request types, authoritative identity, bounded pages, reserved characters, Unicode, and rate-limit metadata.
CLI regressions in `internal/app/search_pulse_server_e2e_test.go` reject unfiltered inventories and every per-definition metric-list request during a term search.
Those regressions failed at runtime against the previous source composition and pass with the new routing.
They also retain explicit no-term metric inventory and content-only behavior.

Application regressions in `internal/app/search_pagination_budget_e2e_test.go` exercise the real CLI composition with 130 indexed metric hits and a failing definition source.
Limits of 101 and 130 return the requested metric prefix without querying definitions, while a limit of 131 still encounters and reports the required definition failure.
A mixed search returning 150 metrics and 1,850 definitions makes 19 definition requests for 19 distinct provider pages, preserving exact identities and order.
The command buffers only its current normalized definition page, without changing stable provider page sizes or cursor fingerprints.
`internal/app/search_definition_buffer_test.go` covers key changes, eviction, cancellation, failed-read recovery, and fresh-command continuation drift.
Availability regressions cover Cloud HTTP 404 and 405 responses with their upstream metadata and Cloud-specific corrective action, while explicit Server Pulse requests remain unsupported.
Independent review also reproduced native lookahead failures after the requested prefix was already available, including default mixed search.
`internal/app/search_native_budget_e2e_test.go` verifies that satisfied native search budgets return their prefix with one native request even when the next page would fail with HTTP 429.
Requesting 101 rows still fetches the required second page and preserves its failure metadata.
Resource-adapter regressions verify deferred-page cursor replay, repeated metric identities, and changed-prefix rejection across commands.
Search now defers classification of trailing datasource connections and overlapping pages until their continuation is consumed, without claiming an unestablished unique datasource total.
An independent compatibility challenge also demonstrated that a legacy unfiltered definition cursor could be accepted by the filtered source when the provider token represents a positional offset.
The live filtered-definition source contract now participates in action cursor identity, rejecting those legacy continuations before a filtered definition request.
`internal/app/search_definition_cursor_e2e_test.go` covers legacy rejection, current filtered continuation, and unchanged no-term and cache cursor contracts.
Review additionally reproduced eager datasource normalization beyond the requested prefix: a mixed search with 101 available metrics failed on a later unused datasource lookup.
`internal/app/search_unused_datasource_e2e_test.go` distinguishes satisfied metric prefixes, which make no datasource lookup, from requests that need the datasource and preserve its HTTP 429 failure.
Raw-page acquisition now stays separate from bounded identity resolution, with datasource batching and command-local reuse retained.

The rebuilt CLI completed the original `search test` reproduction successfully, returning ten content objects and one matching group with no Pulse hits.
Positive CLI metric and definition searches each returned two bounded rows with `more_available=true`; the metric search reported 104 indexed hits.

No credentials, private site identifiers, content payloads, or machine-local paths are retained in this record.
