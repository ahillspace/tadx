# Phase 1 REST contract evidence

This record freezes the upstream contract used by the Phase 1 remote capabilities.
The captured official Tableau REST API help is `Tableau API Documentation/tableau_rest_api.md`.
The official source is [Tableau REST API help](https://help.tableau.com/current/api/rest_api/en-us/REST/).
The local captures and their line references are historical provenance; see [capture availability](README.md).
Its SHA-256 digest is `76d7f050a32ca354660039bdc18143506ce26472ff82f0f12b79fb5aaa584e0f`.

The implementation uses these captured sections:

- PAT sign-in request, response, and errors at lines 40901 through 41166.
- Classic pagination at lines 1699 through 1751.
- Workbook listing at lines 38650 through 38925.
- Exact workbook reads at lines 27547 through 27610.
- Project listing at lines 36768 through 37060.
- Workbook download at lines 20889 through 20955.
- Datasource download at lines 20063 through 20131.
- Exact datasource reads at lines 35015 through 35168.
- Upload initiation at lines 28079 through 28146.
- Upload append at lines 10432 through 10506.
- Workbook publish at lines 34005 through 34425.
- Workbook delete at lines 19862 through 19916.
- TWB validation at lines 49525 through 49700.
- Asynchronous publish jobs at lines 3475 through 3504.
- Job queries at lines 36548 through 36643.

Contract tests use local HTTP servers and parse the multipart publish and append bodies.
They assert methods, paths, headers, payload identities, exact uploaded bytes, ordered multi-block sequence IDs, pagination, response parsing, validation warnings and errors, terminal jobs, and in-flight polling timeouts.
The local contract tests do not claim live Tableau deployment verification.
The separate live records below cover one successful workbook round trip and one cross-site rejection.

The Query Job endpoint is documented as administrator-only while workbook publish can be available to non-administrator publishers.
Publication selects its lifecycle automatically: verified administrator monitoring uses accepted asynchronous jobs, while unavailable monitoring prerequisites retain synchronous publication.
There is no user-facing job-mode option.
When Tableau accepts an asynchronous publish but observation becomes unavailable or is interrupted, the mutation outcome remains unknown rather than failed.
Active jobs do not fail merely because ten minutes elapsed.
The error retains the job and request IDs and does not advise an automatic retry.
The capture disagrees on a 1,000-block versus 10,000-block upload limit.
TADX applies the conservative 1,000-block limit.

## Workbook delete contract

Workbook deletion uses `DELETE /api/{version}/sites/{site-luid}/workbooks/{workbook-luid}`.
The request has no body, and successful deletion returns HTTP 204 with no body.
The endpoint is available in REST API 2.0 and later.
Documented missing-site and missing-workbook responses use HTTP 404 with upstream codes `404000` and `404006`.

Preview performs no DELETE.
Apply re-resolves the planned authoritative workbook LUID immediately before the final request.
A rename or project move does not change the selected identity.
TADX sends no separate view, connection, schedule, or dependency mutation.
Hermetic client and action tests freeze the exact request, empty response, preview behavior, LUID revalidation, metadata-change behavior, and uncertain non-204 outcome.
Authorized disposable Tableau Cloud verification completed on 2026-09-02 for publishing a workbook with a published datasource binding and deleting the resulting exact workbook by LUID.
The temporary workbook was removed after verification.

## Live workbook publish behavior

A live Tableau Cloud workbook round trip completed on 2026-08-31 against an authorized disposable development site.
The sequence pulled a native workbook package, previewed an exact project target, published it, and received a new authoritative workbook LUID.
This record omits environment aliases, site names, project names, workbook names, LUIDs, and request IDs.

### Cross-site published datasource rejection

A second live test on 2026-09-01 used an unchanged workbook package with a direct published datasource binding.
The source and target were different sites on the same Tableau Cloud pod.
Preview identified the cross-site provenance and retained the explicit target project.
Apply sent the native workbook package unchanged.

Tableau returned HTTP 400 with upstream error code `400011` because the bound published datasource was unavailable on the target site.
TADX preserved the structured upstream error and Tableau request ID in the live CLI response.
This evidence redacts the datasource name and request ID.
The follow-up preview found no workbook collision, which confirmed that the rejected publish did not create the target workbook.

This behavior makes Tableau authoritative for package compatibility at publish time.
TADX warns when artifact provenance differs from the explicit target, but it does not add a separate fail-closed published-datasource gate.

Tableau binds a workbook's published datasource by the content URL in its `sqlproxy` connection `dbname`.
Content URLs are unique per site, and Tableau appends `_<digits>` when a published datasource name collides with an existing content URL.
A repeatable `--datasource-binding <source name or content URL>=<target LUID>` flag maps one workbook reference to an exact target datasource.
TADX reads the target by REST LUID and rewrites only a temporary upload copy: the live and cached `sqlproxy` `dbname`, the connection server, port, and channel when the target host differs, and the datasource repository location.
The source workbook file is never changed.
References without a binding are left to Tableau, and a `400011` rejection is reported as not applied with exact candidates from one bounded datasource listing of the target site.
A candidate has the reference's exact name, its exact content URL, or that content URL followed by `_<digits>`.

Site administrators publish through an asynchronous `PublishWorkbook` job, and Tableau reports a missing published datasource only as a failed job.
A live Tableau Cloud run on 2026-10-28 returned `progress="100"` with `finishCode="1"` and an empty `<statusNotes/>`, so the job carries no reason or error code.
For a failed job, TADX runs the same bounded candidate listing and reports the same explanation as not applied only when one or more unbound content URLs have no datasource on the site.
Without that evidence it keeps the failed-job monitoring error.
The same run confirmed that a datasource published under a collided name received a `_<digits>` content URL and that a workbook published with `--datasource-binding` to that datasource was bound to it on pull.

## Workbook published datasource detection

Workbook pull detects published datasources from the downloaded workbook file, not from the Metadata API.
Each live `sqlproxy` connection under a top-level workbook datasource contributes its `dbname` content URL.
Cached datasource copies, embedded connections, and worksheet-level datasource references are not references.
TADX resolves the distinct content URLs on the source site through the exact datasource reads documented at lines 35015 through 35168 of the captured REST help, using the documented `contentUrl` filter.
One URL uses `contentUrl:eq:<url>`, and several URLs use `contentUrl:in:[...]` in batches of at most 25.
Every content URL must resolve to one datasource with a REST LUID; an unresolved URL makes detection incomplete.
A preview with `--include-pds` downloads the workbook without extracts to perform the same detection, and it fails when detection is incomplete.
A pull without `--include-pds` records portability as unknown with a warning when detection is incomplete.
Artifact `metadata.json`, `view.md`, and full output record each reference's LUID, name, and content URL.
Lineage capture remains Metadata API based; see [the lineage contract](lineage-metadata-contract.md).

Local HTTP contract tests freeze the file parsing, filter encoding, batching, unresolved-URL, and output behavior.
