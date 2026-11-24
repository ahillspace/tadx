# TDS Edit and datasource tag contracts

The local `datasource.validate-changes` capability checks a typed changes file without contacting Tableau.
The remote capabilities below await provider qualification.
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
| [Lifecycle](../../actions/datasource/tds_lifecycle_test.go) and [local attempt store](../../internal/operationrun/tds_attempt_test.go) | Checkout and save admission, identity preservation and interrupted attempts against simulated provider and local state. |
| [Tag actions](../../actions/datasource/tags_test.go) and [wire requests](../../internal/tableau/datasource/tags_test.go) | Exact membership, preflight and readback, refusal, mismatches and response loss. |

The [CLI workflow fixtures](../../internal/app/tds_workflow_e2e_test.go) exercise command entry points with a disposable HTTP server and a fixture-scoped provider.
The [session redaction tests](../../internal/auth/session_redaction_test.go) cover environment credentials, a simulated native credential store, saved session adoption and renewed session tokens.
An acknowledged checkout receipt retains the exact source LUID, observed name and project, source observation time and acknowledgement time.
A later audit labels those historical observations separately from its current TDS read and optional published-source schema enrichment.
Receiptless copies can still identify their own observed fields and export candidates.
Audit and template exports use the authenticated session's redactor.
Bindings containing protected values cannot be exported as actionable selectors.

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
