# Flow REST contract evidence

This record freezes the upstream flow lifecycle contract.
The captured official Tableau REST API help is `Tableau API Documentation/tableau_rest_api.md`.
Its SHA-256 digest is `76d7f050a32ca354660039bdc18143506ce26472ff82f0f12b79fb5aaa584e0f`.

## Captured sections

- Flow filter fields and operators are at lines 1914 through 1923.
- Delete Flow is at lines 17363 through 17418.
- Download Flow is at lines 20315 through 20376.
- Publish Flow is at lines 33678 through 33859.
- Query Flow is at lines 35768 through 35908.
- Query Flows for a Site is at lines 36005 through 36231.
- Update Flow is at lines 44046 through 44175.
- Initiate File Upload is at lines 28079 through 28146.
- Append to File Upload is at lines 10432 through 10506.

## Read contract

Flow listing uses `GET /api/{version}/sites/{site-luid}/flows` with bounded classic pagination.
Supported flow filters are `createdAt`, `name`, `ownerName`, `projectId`, `projectName`, and `updatedAt` with the documented operators.
Exact flow reads use `GET /api/{version}/sites/{site-luid}/flows/{flow-luid}`.
The authoritative identity is the REST `id` attribute.
The response supplies direct project and owner identities plus lifecycle metadata.

Native flow download uses `GET /api/{version}/sites/{site-luid}/flows/{flow-luid}/content`.
The payload is a `.tfl` or `.tflx` file.
TADX preserves the response bytes unchanged and never unpacks, converts, or rewrites the package.

## Publish contract

Flow publish uses `POST /api/{version}/sites/{site-luid}/flows`.
The optional `overwrite` query flag is explicit and defaults to false.
Files at or below 64 MB use one multipart request with `request_payload` and `tableau_flow` parts.
Larger files use initiate upload, ordered append blocks, and a final publish request with `uploadSessionId` and `flowType`.
The final request contains the exact flow name and destination project LUID.
Only `.tfl` and `.tflx` payloads are accepted.

Preview performs no upload or remote mutation.
Apply re-resolves the destination and collision immediately before the final publish request.
An uncertain final POST outcome is not automatically retried.

## Move contract

Flow move uses `PUT /api/{version}/sites/{site-luid}/flows/{flow-luid}`.
The body contains only `<flow><project id="destination-project-luid"/></flow>`.
TADX does not include an owner element.
Source and destination stay on the same site.
Apply re-resolves both identities immediately before the PUT.

## Delete contract

Flow delete uses `DELETE /api/{version}/sites/{site-luid}/flows/{flow-luid}` and expects HTTP 204 with no body.
Apply re-resolves the authoritative flow immediately before deletion.
TADX does not perform a separate cascade, schedule operation, run-history operation, or dependency mutation.

## Verification status

Hermetic tests assert exact requests, native bytes, multipart parts, upload-session identity, pagination, response parsing, and structured errors used by the executable commands.
A build-tagged lifecycle test is available for opt-in deployment verification against a configured profile.
The live test uses uniquely named disposable flows and removes its disposable content.
Its output omits authentication headers, raw packages, names, site identifiers, project identifiers, and unhashed LUIDs.
This evidence record does not claim that the live lifecycle test has run.
