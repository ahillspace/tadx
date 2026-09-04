# Administration REST contract evidence

This record freezes the upstream REST contracts used by the TADX administration action seams.
The captured official Tableau REST API help is `Tableau API Documentation/tableau_rest_api.md`.
Its SHA-256 digest is `76d7f050a32ca354660039bdc18143506ce26472ff82f0f12b79fb5aaa584e0f`.

## Captured sections

- Add User to Group starts at line 8878.
- Add User to Site starts at line 9210.
- Create Group starts at line 13032.
- Delete Group starts at line 17545.
- Get Users in Group starts at line 26915.
- Get Users on Site starts at line 27035.
- List Data Source Permissions starts at line 29255.
- List Default Permissions starts at line 29672.
- List Flow Permissions starts at line 30007.
- List Project Permissions starts at line 30712.
- List Workbook Permissions starts at line 32582.
- Query Groups starts at line 36394.
- Query User On Site starts at line 37744.
- Remove User from Group starts at line 39587.
- Remove User from Site starts at line 39780.
- Update Group starts at line 44518.
- Update User starts at line 47728.
- Classic pagination behavior is captured at lines 1699 through 1751.

## Frozen user contract

User listing uses `GET /api/{version}/sites/{site-luid}/users` with positive `pageNumber` and `pageSize` values no greater than 1,000.
Exact LUID reads use `GET /api/{version}/sites/{site-luid}/users/{user-luid}`.
Exact username or email resolution scans bounded user pages and fails when more than one authoritative LUID matches.
Create uses `POST /api/{version}/sites/{site-luid}/users` with explicit role and authentication settings.
Update uses `PUT /api/{version}/sites/{site-luid}/users/{user-luid}` with only explicitly selected attributes.
Delete uses `DELETE /api/{version}/sites/{site-luid}/users/{user-luid}` without `mapAssetsTo`, so TADX never silently reassigns owned content.
Reads and updates require HTTP 200, creates require HTTP 201, and deletes require an empty HTTP 204 response.
Unexpected successful status or body shapes are treated as unknown mutation outcomes and retain the Tableau request ID.

## Frozen group contract

Group listing uses `GET /api/{version}/sites/{site-luid}/groups` with classic pagination.
Exact identity resolution scans bounded group pages and treats the group LUID as authoritative.
Direct membership uses `GET /api/{version}/sites/{site-luid}/groups/{group-luid}/users` with classic pagination.
Create and metadata update use the documented group POST and PUT endpoints.
Membership convergence uses one ordered call per user: POST to the group users collection for additions and DELETE to the exact group user path for removals.
Delete uses `DELETE /api/{version}/sites/{site-luid}/groups/{group-luid}` and never deletes users.
Group and membership writes use the same exact documented success statuses, including empty HTTP 204 responses for removals.

## Frozen permission contract

Direct permission reads use the documented workbook, datasource, flow, and project permission endpoints.
Project default permission reads require an explicit supported content kind and use `projects/{project-luid}/default-permissions/{content-kind}`.
The normalized result records `direct`, `default`, or `inherited` source facts.
An upstream parent element marks an inherited locked-project fact.
The action does not calculate effective permissions and reports `unknown` when the upstream payload cannot establish a stronger source fact.

## Verification status

Hermetic client tests assert methods, escaped paths, pagination, mutation payloads, membership ordering primitives, permission normalization, and malformed response handling.
Resource adapter tests assert exact identity, ambiguity failure, bounded full-page scans, and normalized membership.
Authorized disposable Tableau Cloud verification completed on 2026-09-02.
The live matrix covered user list, inspect, create, update, and delete; group list, inspect, create, update, direct-member add and remove, and delete; and permission reads for workbook, datasource, flow, and project resources.
Temporary users and groups were deleted after verification.
The live check exposed and then verified the repair of a double-wrapped add-member XML request.
