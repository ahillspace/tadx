# TDS Edit and datasource tag contracts

The local `datasource.validate-changes` capability checks a typed changes file without contacting Tableau.
Standard builds gate the remote capabilities below pending provider qualification.
Build with `-tags tds_qualification` to test checkout, reads, edits, and tags with the documented provider adapters on an authorized disposable site.
Managed policy, saved exact-site consent, and Tableau authorization apply to the qualification build.
Both save modes remain gated in this build because they need destination and dispatch guarantees.
The wire and action fixtures use simulated responses.
No authorized live observation accompanies this change.

## Provider documentation

All seven TDS methods require REST 3.30 or later and Tableau+ on Cloud or Data Management on Server.
TADX uses PATs. Tableau also documents access scopes for other authentication mechanisms.
A non-administrator needs the source datasource's Connect, Write and VizqlDataApiAccess permissions, including for reads.
Checkout excludes composable and Semantic Data Model sources.
Tableau enforces a per-caller open-copy quota.
The published reference supplies neither a whole-copy discard method nor a method for listing or adopting a caller's unknown copy.
[Tableau datasource methods](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm)

In the table, routes start at `/api/{version}/tds/sites/{authenticated-site}`.
`D` identifies the published source and `C` the work copy.
Both are UUIDs.

| Operation | Request and provider evidence | TADX behavior and verification |
| --- | --- | --- |
| [Create work copy](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#create_work_copy) | `POST datasource/D/workcopy`, no body. HTTP 201 carries `tdsWorkCopy.workCopyLuid`. | Retain the pending attempt before dispatch. Associate the returned copy with the checked-out source. After response loss, recover the attempt by source before allocating again. |
| [Read published elements](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#read_data_source_elements) | `GET datasource/D` with `elementType` and optional exact `name`. HTTP 200 carries `tdsElementList`, including any per-record gRPC errors. | Read `Column`, `FieldDescription` and `TableDescription` by name or bounded collection. Read the published LUID for independent save verification. |
| [Read work-copy elements](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#read_work_copy_elements) | `GET workcopy/C` with the same query and read envelope. For either read route only HTTP 404 with sub-code `404065` establishes a missing named element. | A per-record error or omitted property leaves the value unknown. Verify column identity separately when setting a field description. A present empty description differs from an absent description. |
| [Add descriptions](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#add_work_copy_elements) | `POST workcopy/C` with a nonempty `tdsElementList`. Each entry carries a field or table description, name and description text. Full success returns HTTP 201 and an empty response object. | Add the exact named description after observing absence. The table-description read can establish absence of a description, but cannot independently identify its underlying table. |
| [Update elements](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#update_work_copy_elements) | `PUT workcopy/C` with a description entry or a `Column` entry changing one of `caption`, `type` or `role`. Full success returns HTTP 200 and an empty response object. | Update an existing description or one exact column property. The method rejects unknown enum sentinels. Complete accepted role and type lists require further evidence. |
| [Delete elements](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#delete_work_copy_elements) | `DELETE workcopy/C` with `tdsElementList` entries containing a name and an element type. Supports column, field-description and table-description removal. Full success returns HTTP 200 and an empty response object. | Verify removal by exact named read. Removing a description and removing its column are separate intents with distinct policy IDs. |
| [Publish work copy](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#publish_work_copy) | `POST workcopy/C/publish` with `datasource.name` and optional `datasource.description`. HTTP 201 returns `tdsWorkCopy.datasourceLuid`. | Save as new requires verified create-only behavior and project placement before activation. Save overwrite requires a separately evidenced wire contract. Compare the exact destination and requested properties after an acknowledged save. |

**Element failure handling.** The edit endpoints return failed elements in `tdsElementResult` and use the first failure's translated HTTP status.
TADX submits one ordered logical edit per request after validating the complete plan.
An exact failure receipt identifies a refused item.
After a lost response the exact readback reports current state, while attribution stays unknown.
A matching later read cannot identify which writer caused the change.
The remaining entries stay unattempted.

**Tags.** Published datasource tags use REST 1.0+ paths under `/api/{version}/sites/{site}/datasources/{id}/tags`.
Addition uses PUT with tag labels and returns HTTP 200 with the resulting tags.
Existing labels are ignored.
Removal uses DELETE on the encoded label and returns HTTP 204.
Tableau documents Read permission for addition and a separate removal rule for administrators, owners, project leaders and the original tagger.
The actions verify exact membership on a fresh datasource read and preserve unrelated tags.
The TDS element model exposes no field-tag property.
Tags on Catalog upstream columns have their own asset identity and are already supported by `catalog column update`.
[Datasource tagging](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#add_tags_to_data_source) · [Datasource tag removal](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#delete_tag_from_data_source) · [Catalog column tagging](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_metadata.htm)

## Local fixture evidence

| Boundary | What the tests exercise |
| --- | --- |
| [TDS HTTP adapter](../../internal/tableau/datasource/tds_test.go) and [REST negotiation](rest-version-negotiation-contract.md) | Negotiated route, method, payload, response envelope, named absence, failed records and malformed acknowledgement. |
| [Edit actions](../../actions/datasource/tds_edit_test.go) | Field and table descriptions, each column property, element removal, preflight, stale tokens and stale later edits, per-entry verification and interrupted responses. |
| [Workspace exports](../../actions/datasource/tds_files_test.go) | Inert YAML, multiple activated examples, verified target binding, constrained workspace paths, output conflicts, partial export and concurrent no-replace creation. |
| [Lifecycle](../../actions/datasource/tds_lifecycle_test.go) and [local attempt store](../../internal/operationrun/tds_attempt_test.go) | Checkout and save admission, preview of all affected guard scopes, and recovery after process exit with a simulated provider. |
| [Journal retention and migration](../../internal/operationrun/tds_retention_test.go) | Concurrent checkout and save receipts against near-capacity legacy history, exact caller-scoped provenance after history expiry, interrupted migration and rollback of a failed acknowledgement link. |
| [Tag actions](../../actions/datasource/tags_test.go) and [wire requests](../../internal/tableau/datasource/tags_test.go) | Exact membership, preflight and readback, refusal, mismatches and response loss. |

The [CLI workflow fixtures](../../internal/app/tds_workflow_e2e_test.go) exercise command entry points with a disposable HTTP server and a fixture-scoped provider.
The [session redaction tests](../../internal/auth/session_redaction_test.go) cover environment credentials, a simulated native credential store, saved session adoption and renewed session tokens.
A checkout or save acknowledgement preview includes each matched attempt ID, operation, submit status and guard resource.
An overwrite can clear an earlier overwrite's guard on a different work copy of the same original.
Save preview tokens cover the matched attempts and their guard resources.
The CLI fixture rejects stale approval before publication and accepts a fresh approval with equivalent acknowledgement flags in a different order. [Save approval fixture](../../internal/app/tds_acknowledged_preview_test.go)
The private journal commits the fresh pending attempt with every acknowledgement link in one transaction.
A lifecycle fixture starts with 24,780 acknowledged checkout attempts older than 30 days in a 16,776,733-byte legacy journal and recovers the newly allocated copy by source and attempt ID after reopening the store.
The same fixture resolves an older copy's original source and revision after its attempt history has expired.

An acknowledged checkout receipt retains the exact source LUID, observed name and project, source observation time and acknowledgement time.
A later audit labels those historical observations separately from its current TDS read and optional Metadata API observations of the published source.
Receiptless copies can still identify their own observed fields and export candidates.
The checkout preview reports the negotiated REST version.
Entitlement, permission, source compatibility and quota stay unknown until a provider response supplies evidence for that specific request.
Named and collection reads report their own scope.
The CLI retains known refusal reasons and leaves ambiguous 403 and 429 responses unclassified when the provider supplies insufficient evidence.
The [eligibility fixtures](../../internal/app/tds_eligibility_test.go) exercise these distinctions with simulated responses.

Direct edits and saves check the retained source fields they use against the current session's credentials before building the target or save plan. [Rotated credential fixtures](../../internal/app/datasource_edit_credentials_test.go)
Checkout preserves redacted refusal diagnostics in command output and the saved command receipt.
After a successful lookup under a renewed session, checkout rejects a source name containing the retired token before creating an attempt or allocating a work copy.
The audit masks tokens used earlier in the same command when exporting indexed field descriptions. [Checkout and audit redaction fixtures](../../internal/app/tds_read_diagnostics_test.go)

Audit and template exports use the authenticated session's redactor.
Bindings containing protected values cannot be exported as actionable selectors.

When `datasource.schema` is allowed, the audit requests indexed field properties and calculated-field formulas.
If `lineage.pull` is also allowed, the audit requests referenced fields and downstream sheets.
The audit reads one page of up to 100 indexed fields, with up to 100 upstream columns, referenced fields and downstream sheets per field.
It records each requested connection's reported total, next-page flag and count of omitted identities.
Missing coverage is shown as unknown, and a provider failure remains visible alongside independently observed data.
Explicit starter fields reuse exact column and description rows from the TDS collections.
When those observations are missing, the CLI uses named reads, including a named absence response for a missing explicit description.
The CLI fixtures verify the GraphQL request on the checkout session, indexed fields outside the TDS selection, redacted output, projected limits, partial responses and absence of shared schema cache writes. [Audit CLI fixtures](../../internal/app/tds_audit_e2e_test.go)

## Evidence needed for activation

Obtain permission for the exact disposable site, datasource, project, operations and cleanup.
Record the negotiated REST version, entitlement, permissions, actual method, sanitized request and response, named readback, failure attribution and usable recovery route for each capability being qualified.
Preserve upstream request IDs in private run evidence and remove credentials and private identities from tracked evidence.

- For checkout and reads, establish quota handling, exact element-name and case behavior, duplicate-name handling, collection coverage, missing-element responses and the copy's lifecycle.
- For edits, verify add, update, no-op, deletion and readback independently for each element type and property. Establish role and type constants and the effects of deleting a column on dependent fields.
- For as-new save, prove project placement and create-only behavior on a preexisting name and a name claimed after preflight. Verify a distinct published LUID, full requested destination metadata, selected properties and post-save copy state.
- For overwrite, obtain a released provider request for preserving the original LUID, plus a revision marker covering the complete source definition. Verify stale-source refusal, dependent workbook identity, published readback and the concurrency window between preflight and submission.
- For datasource tags, verify membership before and after add and remove, an empty resulting tag set, ownership-dependent refusals and unrelated tag preservation.
- Field-level tag editing requires an official field-owned write and readback contract. Native datasource file publishing continues to use its own upload, conflict and verification contracts.
