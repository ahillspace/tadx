# Datasource lifecycle REST evidence

This record bounds the datasource pull, publish, and delete implementation seams.
It does not make blocked capabilities executable and it does not authorize live mutations.

## Local source snapshots

The official references are [Tableau REST API help](https://help.tableau.com/current/api/rest_api/en-us/REST/) and the [Metadata API guide](https://help.tableau.com/current/api/metadata_api/en-us/).
The identifiers below preserve historical evidence; see [capture availability](README.md).

- `Tableau API Documentation/tableau_rest_api.md`, Git blob `e127a2b3ed8dad9eddaac8be60e7b0b306a4df30`.
- `Tableau API Documentation/tableau_virtual_connections.md`, Git blob `655d9fefcaa1b29bb0c5cccf939e78bd522efc7d`.
- `Tableau API Documentation/metadata_api.md`, Git blob `5b7a6763b00fa42ce7d76b43bc5b2f5450c22db3`.

The older `metadata_api.md` and `tableau_virtual_connections.md` snapshots are absent from the current local capture set.
The Metadata API guide is a current reference entry point, not a claim that it reproduces the older snapshot.
No original official source URL for the virtual-connections snapshot is recorded here.
Its historical claims require renewed source verification before supporting a new capability.

## Pull contract

The REST reference section `Download Data Source` begins at line 20063 of the captured REST file.
It defines `GET /api/api-version/sites/site-luid/datasources/datasource-luid/content` and documents preservation of TDS or TDSX content.
The implementation therefore stores the returned package bytes unchanged and rejects unsupported response filenames.
Composition classification reads the serialized `datasource-url` attributes without rewriting the TDS or TDSX package.
Unreadable definitions remain `unknown`, which preserves the authoritative download but prevents unsafe publication.

## Publish contract

The REST reference section `Publish Data Source` begins at line 33245 of the captured REST file.
The base publish method is available in REST API 2.0 and later on Tableau Cloud and Tableau Server.
The method accepts direct multipart publication and upload-session publication for TDS, TDSX, Hyper, and TDE payloads.
The documented direct upload size boundary is 64 MB, so larger payloads use Initiate File Upload and Append to File Upload requests before the final publish request.
The `sequenceID` ordering parameter is used only with REST API 3.27 and later because the captured reference says earlier releases assemble blocks by arrival time.
When `asJob=true` is selected, the accepted `PublishDatasource` job identity is validated and polled internally to a bounded terminal state.
Indeterminate, cancelled, or timed-out outcomes preserve the exact job and latest request IDs and are never presented as safe to retry.
Create is represented by leaving overwrite, append, and replace false.
Overwrite, append, and replace are explicit mutually exclusive modes.
Replace is documented as available beginning with Tableau 2025.1.
The captured version matrix maps Tableau 2025.1 to REST API 3.25, so replace is rejected on older REST versions.

The `Composed data sources and extended data sources` subsection begins at line 33275.
Publishing composed and extended data sources requires REST API 3.29, Tableau Cloud 262, or Tableau Server 2026.2.
A composed publish includes one repeated `parentDataSourceUrls` element for every immediate parent referenced by a serialized `datasource-url` attribute in the TDS definition.
An extended datasource is published through the ordinary TDS workflow without parent URL elements.
The released method deploys an already-authored composition.
It does not provide a first-class mutation for creating or changing the composition relationship itself.

## Delete contract

The REST reference section `Delete Data Source` begins at line 16621 of the captured REST file.
It defines `DELETE /api/api-version/sites/site-luid/datasources/datasource-luid` and documents an empty HTTP 204 success response.
The transport accepts only that exact success shape and treats any response-body or status mismatch as an unknown mutation result.
The action deletes by default when mutation execution is enabled and supports `--preview` for a read-only plan.
Before deletion, it re-resolves the exact datasource and stops if any authoritative target field changes.

## Unsupported mutation boundaries

The REST reference section `Update Data Source` begins at line 42682.
That released API updates metadata and connection-related properties, not datasource composition.
The captured Metadata API is read-only and cannot update field descriptions or composition.
The captured virtual datasource guidance for Tableau 2026.2 describes re-publishing the latest composed datasource rather than mutating composition through a released relationship API.
Datasource composition authoring is deferred indefinitely and has no executable action or transport method.
Datasource field-description updates are deferred.

## Verification status

Hermetic action, artifact, resource-adapter, REST transport, and CLI composition tests cover the implemented seams.
The repository owner has authorized disposable Tableau Cloud fixtures for live verification.
Live results are recorded separately from the standard test suite and never expose credentials or machine-specific paths.
Authorized disposable Tableau Cloud verification completed on 2026-09-02 for pull, synchronous and asynchronous create publication, exact deletion, compact output, and cleanup.
The native datasource package round trip preserved its authoritative identity and project provenance without exposing machine-specific paths in compact output.
The live asynchronous check proved the HTTP 202 acceptance contract and Tableau's terminal job response without an embedded datasource identity.
TADX resolved the completed publication through a bounded exact-name and exact-project lookup, returned one authoritative LUID, and deleted the temporary datasource.
