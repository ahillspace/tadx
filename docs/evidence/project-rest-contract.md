# Project REST contract evidence

This record freezes the upstream project contract used by `project.list` and `project.get`.
The captured official Tableau REST API help is `Tableau API Documentation/tableau_rest_api.md`.
Its SHA-256 digest is `76d7f050a32ca354660039bdc18143506ce26472ff82f0f12b79fb5aaa584e0f`.

## Captured sections

- Project filter fields and operators are at lines 1999 through 2009.
- Query Projects is at lines 36768 through 37060.
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
`project.get` can scan all pages because exact path resolution requires the complete hierarchy.

## Error contract

The captured operation documents invalid page number, invalid page size, page-size limit, missing site, and invalid method responses.
The shared Tableau transport preserves HTTP status, Tableau error code, request ID, retryability, and corrective action.

## Verification status

Hermetic client tests assert the exact method, path, query, pagination envelope, identities, and malformed-response handling used by the executable commands.
A build-tagged live test is available for opt-in deployment verification against a configured profile.
That test records only sanitized request shapes, counts, and hashed identifiers.
This evidence record does not claim that the live test has run.
