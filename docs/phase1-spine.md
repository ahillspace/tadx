# Phase 1 shared spine

Phase 1 freezes the shared patterns that later capability slices copy.

## Transport

`internal/tableau` owns HTTP request construction, standard headers, session authorization, correlation IDs, response reading, upstream error parsing, redaction, and opportunistic Tableau request-ID capture.
It carries forward the trusted Go inventory transport's standard-library HTTP execution and bounded-response pattern.
Buffered responses default to a 256 MiB ceiling, and individual released clients can select a stricter positive bound.
Phase 1 workbook pulls use that ceiling and fail without writing an artifact when a native workbook exceeds it.
Supporting larger pulls requires a future streamed artifact contract rather than unbounded buffering.
Response-read failures preserve the HTTP status and request ID.
Structured action errors separately preserve the upstream HTTP status, Tableau code, summary, detail, and request ID.
Overlapping secrets are redacted as merged intervals so partial secret values cannot survive replacement order.
The transport performs no generic retries.

## Adapter pattern

`internal/resources/workbook` is the first resource adapter.
It isolates workbook and project API details, normalizes pages, resolves exact identities, and exposes narrow behavior to action-owned interfaces through the composition root.

## Pagination envelope

Classic REST clients preserve `pageNumber`, `pageSize`, and `totalAvailable` as `page_number`, `page_size`, and `total`.
Agent-facing search uses `returned`, `total`, `limit`, and `next_cursor`.
All public pages remain bounded.

## Catalog generation contract

`catalog.search` reads one complete local generation from the `<config directory>/catalog` directory.
Aliases containing only lowercase ASCII letters, digits, hyphens, and underscores use `<environment>.json` when the complete filename fits within a 255-byte component, except for reserved Windows filenames.
Other aliases, including aliases too long for a portable filename component, use `~<lowercase SHA-256 hex>.json` as returned by `catalog.GenerationFilename`.
The generation records its ID, generation time, source environment and site, completion state, and bounded content items.
Search rejects incomplete generations and source mismatches, sorts results deterministically, and warns when a generation is older than 12 hours.
Phase 1 does not make catalog refresh executable.

## Artifact contract

A pulled workbook artifact contains the native `.twb` or `.twbx`, `metadata.json`, and `view.md`.
Metadata records provenance, the authoritative Tableau LUID, the canonical payload name, and a SHA-256 baseline fingerprint.
Artifact identity is scoped by normalized Tableau server origin, authenticated site LUID, resource kind, and workbook LUID.
Environment aliases, site content URLs, and project paths remain provenance labels rather than identity keys.
Artifacts never store a publish target.
Clean re-pull warns and replaces.
Dirty re-pull stops unless `--overwrite` is explicit.

## Publish contract

Workbook publish plans authoritative target and collision reads before mutation.
Preview is the default.
`--apply` runs only the plan produced in the same invocation.
Overwrite remains explicit.
Large files use bounded upload sessions.
Every appended block response must return the expected upload-session identity before publishing can continue.
TWB publish validates the native workbook through Tableau before the final publish request, preserves advisory warnings, and stops on validation errors.
TWBX publish does not claim server-side validation because the validation endpoint accepts only TWB content.
`--as-job` uses bounded internal polling and returns terminal success or failure with request and job IDs when available.
If polling is forbidden, cancelled, or times out after Tableau accepts the publish, TADX reports the outcome as unknown, returns the exact job ID, and directs the operator to inspect that job before attempting another publish.
