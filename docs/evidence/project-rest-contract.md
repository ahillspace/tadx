# Project REST contract evidence

This record freezes the upstream project contract used by `project.list`, `project.inspect`, `project.create`, and `project.update`.
The captured official Tableau REST API help is `Tableau API Documentation/tableau_rest_api.md`.
Its SHA-256 digest is `76d7f050a32ca354660039bdc18143506ce26472ff82f0f12b79fb5aaa584e0f`.

## Captured sections

- Project filter fields and operators are at lines 1999 through 2009.
- Create Project is at lines 13694 through 13771.
- Query Projects is at lines 36768 through 37060.
- Update Project is at lines 45703 through 45785.
- Classic pagination behavior is at lines 1699 through 1751.

## Frozen request contract

Project listing uses `GET /api/{version}/sites/{site-luid}/projects`.
The client sends positive `pageNumber` and `pageSize` parameters.
The maximum upstream page size is 1,000.
Supported project filters include exact `name`, `parentProjectId`, owner fields, `topLevelProject`, `createdAt`, and `updatedAt`.

The response contains a required pagination envelope and zero or more `project` elements.
The authoritative project identity is the REST `id` attribute.
The `parentProjectId` attribute is the authoritative direct parent identity.
Exact slash-delimited paths are constructed locally from the complete authoritative hierarchy.
Duplicate LUID rows must agree exactly.
Missing parents, conflicting rows, and hierarchy cycles are protocol failures.

`project.list` returns one bounded upstream page and does not claim a canonical path for a parent outside that page.
`project.inspect` can scan all pages because exact path resolution requires the complete hierarchy.

## Mutation contract

Project creation uses `POST /api/{version}/sites/{site-luid}/projects` and expects HTTP 201.
The request includes a required name and optional description, direct parent LUID, and content-permission mode.
TADX omits `publishSamples`, project ownership, and every other field outside the admitted V1 contract.
Omitting the parent creates a top-level project and never infers a parent.
Preview rejects a case-insensitive name collision under the same exact parent, while Tableau remains authoritative for any broader conflict rule.
The response must contain one authoritative project LUID and the requested name and parent identity.

Project update uses `PUT /api/{version}/sites/{site-luid}/projects/{project-luid}` and expects HTTP 200.
The request includes only explicit changed name, description, or content-permission attributes.
TADX omits `parentProjectId`, project ownership, `publishSamples`, and the optional duplicate project ID from the body.
This boundary prevents project hierarchy movement through `project.update`.
The response must retain the exact project LUID from the URI.

The admitted content-permission values are `ManagedByOwner`, `LockedToProject`, and `LockedToProjectWithoutNested`.
Tableau documents `LockedToProjectWithoutNested` for REST API 3.8 and later.
The server remains authoritative for version, permission, default-project, and name-conflict enforcement.

Create and update are consequential remote mutations.
Preview performs no POST or PUT.
Apply re-resolves every selected project identity and repeats collision or equal-value checks immediately before mutation.
No generic automatic retry follows an uncertain POST or PUT outcome.

## Shallow project evidence gate

The captured filter table does not establish a shallow project package contract.
Flow listing documents an exact `projectId` filter, but workbook and datasource listing document only `projectName`.
Response project LUIDs can support local direct-membership filtering after bounded page exhaustion.
The repository has no authorized nested Cloud and representative Server fixture that proves child exclusion, cross-branch duplicate-name behavior, and pagination for all three resource kinds.
Project pull and publish remain deferred and non-executable.

## Error contract

The captured read operation documents invalid page number, invalid page size, page-size limit, missing site, and invalid method responses.
Create documents malformed input, unsupported content permissions on older API versions, missing sites, invalid methods, and case-insensitive project-name conflicts.
Update also documents default-project rename rejection, permission failure, project-ID mismatch, missing projects, and name conflicts.
The shared Tableau transport preserves HTTP status, Tableau error code, request ID, retryability, and corrective action.

## Verification status

Hermetic client tests assert the exact read and mutation methods, paths, bodies, statuses, identities, pagination envelopes, and malformed-response handling used by the executable commands.
A build-tagged live test is available for opt-in deployment verification against a configured profile.
That test currently covers reads only and records sanitized request shapes, counts, and hashed identifiers.
Authorized disposable Tableau Cloud verification completed on 2026-09-02 for top-level project creation, explicit name and description update, and authoritative LUID readback.
The disposable project remains on the authorized development site because project deletion is outside the admitted V1 command surface.
