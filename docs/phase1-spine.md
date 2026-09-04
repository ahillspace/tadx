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

`search.run --catalog` reads one complete local generation from the `<config directory>/catalog` directory.
Aliases containing only lowercase ASCII letters, digits, hyphens, and underscores use `<environment>.json` when the complete filename fits within a 255-byte component, except for reserved Windows filenames.
Other aliases, including aliases too long for a portable filename component, use `~<lowercase SHA-256 hex>.json` as returned by `catalog.GenerationFilename`.
The generation records its ID, generation time, source environment and site, completion state, and bounded content items.
Search rejects incomplete generations and source mismatches, sorts results deterministically, and warns when a generation is older than 12 hours.
Phase 1 does not make catalog refresh executable.

## Artifact contract

A pulled workbook artifact contains the native `.twb` or `.twbx`, `metadata.json`, `lineage.json`, and `view.md`.
Metadata records provenance, the authoritative Tableau LUID, the canonical payload name, a SHA-256 baseline fingerprint, bounded lineage status and counts, and a workspace-relative lineage sidecar pointer.
Artifact identity is scoped by normalized Tableau server origin, authenticated site LUID, resource kind, and workbook LUID.
Environment aliases, site content URLs, and project paths remain provenance labels rather than identity keys.
The recorded source origin is the default publish target, and an explicit environment overrides it.
Clean re-pull warns and replaces.
Dirty re-pull stops unless `--overwrite` is explicit.

Workbook pull captures bounded direct upstream and downstream lineage automatically.
Lineage capture is best effort, and incomplete or unavailable lineage is recorded explicitly without discarding a successfully downloaded workbook.
Compact success output omits lineage details while preserving actionable warnings.

Workbook pull queries the Metadata API for direct published datasource references.
A complete empty result records the workbook as `portable`.
One or more authoritative published datasource LUIDs record the workbook as `source-site-bound`.
Incomplete metadata leaves portability unknown on a normal pull and produces a warning.
`--include-pds` requires complete metadata and acquires each unique direct dependency without recursion.
Acquired dependencies are unchanged `.tds` or `.tdsx` sibling artifacts under `artifacts/datasource/`.
Each sibling uses server origin, site LUID, and datasource LUID as its identity.
The workbook records workspace-relative sibling paths and sets `dependencies_acquired` only when every direct dependency is present.
Workbook and dependency artifacts are preflighted, staged, and committed as one recoverable local transaction.
Workbook `--overwrite` never authorizes replacement of a dirty datasource dependency.
Prepared and committed transaction journals restore the prior bundle or complete cleanup after a process exit.
Dependency artifacts record `composition_status: unknown`, so later datasource publishing does not claim composition authoring support.

## Publish contract

Workbook publish resolves a logical workspace name through the deterministic workspace chain and accepts one exact workspace-relative managed workbook directory.
Absolute machine paths are not public publish selectors and never appear in output.
Workbook publish plans authoritative target and collision reads before mutation.
Mutation is the default when mutation execution is enabled.
`--preview` emits the plan without publishing.
Overwrite remains explicit.
Large files use bounded upload sessions.
Every appended block response must return the expected upload-session identity before publishing can continue.
TWB publish validates the native workbook through Tableau before the final publish request, preserves advisory warnings, and stops on validation errors.
TWBX publish does not claim server-side validation because the validation endpoint accepts only TWB content.
`--as-job` uses bounded internal polling and returns terminal success or failure with request and job IDs when available.
If polling is forbidden, cancelled, or times out after Tableau accepts the publish, TADX reports the outcome as unknown, returns the exact job ID, and directs the operator to inspect that job before attempting another publish.
