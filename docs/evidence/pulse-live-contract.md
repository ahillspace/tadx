# Pulse live contract evidence

## Scope

This record captures a bounded positive Tableau Pulse lifecycle on a disposable Tableau Cloud development site.
The verification ran on 2026-09-05 with PAT authentication and an authenticated site LUID.
The site, user, datasource, definition, metric, subscription, and local paths are intentionally omitted.

## Source requirements

The test used one published datasource with a numeric measure, a date field, and a categorical dimension.
The authenticated user had a site administrator creator role.
Tableau documentation requires Connect, View, and Create Metric Definitions permission capabilities for the datasource.
Tableau documentation also requires suitable measure, time, and filter fields.

Official sources:

- [Tableau Pulse REST API methods](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_pulse.htm).
- [Set up your data for Tableau Pulse](https://help.tableau.com/current/online/en-us/pulse_data_source_requirements.htm).
- [Set up your site for Tableau Pulse](https://help.tableau.com/current/online/en-us/pulse_set_up.htm).

## Verified lifecycle

The following operations completed successfully against Tableau Cloud:

- Listed Pulse definitions through `GET /api/-/pulse/definitions`.
- Created a definition through `POST /api/-/pulse/definitions` and received HTTP 201 with an authoritative definition identity.
- Resolved the Tableau-created default metric through bounded definition-scoped polling.
- Inspected the definition through `GET /api/-/pulse/definitions/{definition_id}`.
- Pulled the definition into a provenance-bearing local artifact.
- Listed metrics through `GET /api/-/pulse/definitions/{definition_id}/metrics`.
- Inspected the default metric through `GET /api/-/pulse/metrics/{metric_id}`.
- Forked a metric through `POST /api/-/pulse/metrics:getOrCreate` and reconciled its definition, datasource, and site ownership.
- Followed the forked metric through `POST /api/-/pulse/subscriptions:batchCreate`.
- Listed the exact subscription through `GET /api/-/pulse/subscriptions?metric_id={metric_id}`.
- Removed the subscription through `DELETE /api/-/pulse/subscriptions/{subscription_id}` and received HTTP 204.
- Deleted the forked metric through `DELETE /api/-/pulse/metrics/{metric_id}` and received HTTP 204.
- Deleted the definition through `DELETE /api/-/pulse/definitions/{definition_id}` and received HTTP 204.

All remote resources created by this verification were removed.
The run used compact, bounded CLI output and preserved Tableau request identifiers without recording them in this repository.

## Datasource eligibility boundary

This run proves the TADX request and response contracts for one Pulse-eligible published datasource.
It does not prove that every published datasource with discoverable fields is eligible for Pulse.
Tableau remains authoritative for datasource permissions, connection access, field suitability, data availability, and other deployment-specific eligibility rules.
Tableau can reject a schema-valid definition request for a datasource that fails those rules.
TADX must preserve that upstream rejection and its request identifier.

## Site identity boundary

Every unversioned Pulse request carries `X-Tableau-Site-Id` from the authenticated session.
TADX rejects Pulse requests before network access when the authenticated session lacks a site LUID.
PATs, session tokens, private site names, machine paths, and private content identities never appear in this evidence record.

## Local regression coverage after agent workflow testing

The current CLI has HTTP-backed regressions for dimension COUNT/COUNT_DISTINCT eligibility, adjustable-dimension ordering and presence, hidden calculation dependencies, and fork granularity compatibility.
Create and fork tests verify rejected input causes no mutation request and preserves existing identity and eligibility checks.
Fork compatibility is checked again against the current definition before the write.
Opaque 400/409 responses retain provider details and give read/reconciliation guidance without inventing a specific provider diagnosis.
Uncertain create outcomes are not advertised as safe automatic retries.

These checks extend local behavioral evidence, not the live verification recorded above.
The source examples remain in `internal/app/pulse_validation_e2e_test.go`, `internal/app/pulse_fork_granularity_e2e_test.go`, `internal/app/pulse_diagnostics_e2e_test.go`, and `internal/tableau/fieldcatalog/client_test.go`.

## Authenticated-user subscription discovery

The [Tableau Pulse subscription API](https://help.tableau.com/current/api/rest_api/en-us/REST/TAG/index.html#list-subscriptions) supports `GET /api/-/pulse/subscriptions` filtered by `user_id`, with `page_size` and `page_token` for pagination.
TADX takes that user identity from the authenticated PAT session and never omits it to fall back to site-wide discovery.
Names and saved configuration come from bounded `metrics:batchGet` and `definitions:batchGet` requests for the returned identities only.
Definition names are not independent metric-variant names.

The documented user filter does not explicitly establish whether subscriptions inherited through group membership are included.
TADX reports that coverage as unverified and does not enumerate groups to infer additional subscriptions.
The new command's HTTP-backed CLI regressions are in `internal/app/pulse_subscription_e2e_test.go`.
These regressions do not establish live group-membership behavior.

On 2026-09-24, a controlled Tableau Cloud test created a disposable group containing only the authenticated user and followed a disposable metric through that group.
The authenticated-user subscription command returned the group subscription, and the exact metric's follower listing returned the same subscription as its sole follower.
This confirms inclusion for the tested user, site, and group relationship, not a universal guarantee about nested groups or complete group-derived coverage.
The temporary subscription, newly created metric, and group were removed, and the final user-scoped listing retained only the original subscription.
