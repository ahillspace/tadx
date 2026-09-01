# Adding a resource adapter

This is the end-to-end build guide for one Tableau resource seam.
An adapter is the boundary that isolates all Tableau API complexity for one resource so actions can stay pure orchestration.
It is built once per resource, frozen, and copied by later resources.

The frozen worked example is the workbook resource: the REST client family `internal/tableau/workbook`, the resource adapter `internal/resources/workbook`, and the contract evidence in `docs/evidence/phase1-rest-contract.md`.
Read those exact packages and evidence sections instead of broad product documents.

## The two layers

A resource seam is two packages with a strict split.

- `internal/tableau/<resource>` is the released REST client family.
It speaks HTTP through the shared transport, marshals and unmarshals the exact wire format, and returns normalized Go structs.
It imports only `internal/auth` and the shared `internal/tableau` package.
- `internal/resources/<resource>` is the resource adapter.
It consumes the client through a narrow interface it declares, resolves exact identity, walks pagination, and exposes clean resource methods.
It imports only `internal/identity` and the resource's Tableau client package.

Neither layer imports Cobra, the CLI, or any action package.
The composition root bridges the adapter to the narrow interfaces that actions own.

## The ordered build steps

### 1. Capture the upstream contract first

Do not write any live API code until the exact upstream contract is captured.
Follow `docs/contributing/api-documentation-routing.md`.
Search the relevant local capture with `rg`, read only the matching operation section and shared concepts it cites, and record the canonical Git blob digest plus exact headings or bounded line ranges.
Use current official Tableau web documentation only when the local capture is missing, unclear, contradictory, or version-sensitive.
Record the official Tableau REST sections you rely on the way `docs/evidence/phase1-rest-contract.md` does: pin the source document and its digest, and list the endpoint sections with their line ranges (sign-in, pagination, list, get, download, upload initiate and append, publish, validation, async jobs, and job queries).
State explicitly what is and is not verified, and any conservative choices (for example the conservative 1,000-block upload limit).

### 2. Build the REST client family

Create `internal/tableau/<resource>/client.go`.

- Define a `Client` struct built by `NewClient(transport *tableau.Transport, session auth.Session, serverURL string)`.
- Route every request through the transport with a small helper such as `do` or `doAccept`, passing a stable `Operation` string per call.
The transport applies standard headers, session authorization, correlation and Tableau request IDs, response reading, and upstream error capture.
Never construct `net/http` requests directly in the client.
- Build site-scoped paths from `api/<version>/sites/<siteLUID>/...`, path-escaping each segment (see `sitePath`).
- Marshal requests and unmarshal responses into unexported envelope structs, then project them into normalized exported types (for example `Workbook`, `Project`, `Download`).
- Return `tableau.NewProtocolError(operation, response, cause, retryable)` when a successful HTTP response is structurally invalid (missing pagination, wrong LUID echoed back, undecodable body).
Non-2xx responses already arrive as `*tableau.UpstreamError` from the transport.

### 3. Preserve the pagination envelope

Classic REST list endpoints return `pageNumber`, `pageSize`, and `totalAvailable`.
Normalize them into the shared `tableau.Page` struct as `Number`, `Size`, and `Total` (aliased in the client as `Page`).
Return one normalized page per call, for example `WorkbookPage{Page, Items}`, from `List(ctx, pageNumber, pageSize)`.
Validate the returned pagination against what was requested before trusting it (see `normalizePagination`): reject an unexpected page number, an invalid or oversized page size, and an item count that is inconsistent with the total.
Agent-facing search output uses a different envelope of `returned`, `total`, `limit`, and `next_cursor`; the classic client envelope is the internal REST shape, not the agent-facing one.

### 4. Build the resource adapter

Create `internal/resources/<resource>/adapter.go`.

- Declare the narrow `Client` interface the adapter needs, listing only the client methods it calls.
The adapter owns this interface; the concrete client satisfies it.
- Build the `Adapter` with `NewAdapter(client Client)`.
- Walk all pages internally when resolving or scanning, using a fixed adapter page size (the workbook adapter uses `adapterPageSize = 1000`) and stopping when the accumulated count reaches the reported total or a page is empty.
- Resolve identity as LUID-authoritative and exact.
Prefer an explicit LUID via a direct `Get`.
Otherwise scan, keep only exact name and exact slash-delimited project-path matches, and delegate the final decision to `identity.Resolve`, which returns deterministic zero-match and ambiguous-match errors.
Prove ambiguity from at most two distinct authoritative LUIDs rather than buffering every candidate.
- Reject upstream records that omit an authoritative LUID or that return conflicting records for the same LUID.

### 5. Keep secret redaction and the typed-error contract intact

Secret redaction is enforced by the shared transport, not re-implemented per adapter.
The transport merges any request `Secrets` with the authorization header value and redacts them from every diagnostic string (errors, upstream summary and detail, protocol errors, and the request ID).
Never log, persist, or print a PAT or session token, and never place a secret anywhere except the transport's `Secrets` field or the session's own authorization.

The client and transport surface a typed error contract that the action wraps but does not replace:

- `*tableau.UpstreamError` carries the upstream HTTP status, Tableau code, redacted summary and detail, request ID, and retry advice.
- `*tableau.ProtocolError` carries response context for a structurally invalid success.
- Request and response-read failures carry retryability and corrective action.

Actions convert these into `errs.Error` with a stable `ID` of the form `<domain>.<verb>.<stage>`, using `errs.CompleteRetryAdvice` and `errs.TableauRequestID` to carry advice and the request ID through the chain.
Adapters and clients return the typed errors; they do not build the final `errs.Error`.

### 6. Bridge the adapter to the action in the composition root

Actions do not import the adapter.
Each action declares its own narrow interface (for example a `Reader` or `Resolver`), and `internal/app/app.go` wires a small bridge type that adapts the resource `Adapter` to that interface, mapping resource types to the action's types.
See `pullReader` and `publishAdapter` in `internal/app/app.go` for the pattern.

### 7. Write the contract test against captured evidence

Create `internal/tableau/<resource>/client_test.go` as a contract test.

- Stand up local `httptest` servers that reply with fixtures matching the captured REST evidence.
- Assert the request method, path, and headers; parse and assert the multipart publish and append bodies, including exact uploaded bytes and ordered multi-block sequence IDs; assert pagination normalization, response parsing, validation warnings and errors, terminal job outcomes, and in-flight polling timeouts.
- Do not claim live-deployment verification.
The test proves the client against the frozen contract, and the evidence record states what remains unverified.

## Completion checklist

1. The upstream contract is captured in `docs/evidence/` before any live code, with pinned source, digest, and endpoint sections.
2. `internal/tableau/<resource>/client.go` routes every call through the shared transport and returns normalized types.
3. Successful-but-invalid responses return `tableau.NewProtocolError`; non-2xx responses flow through as `*tableau.UpstreamError`.
4. Pagination is normalized into `tableau.Page` and validated against the request.
5. `internal/resources/<resource>/adapter.go` declares its own narrow `Client` interface, walks all pages, and resolves LUID-authoritative exact identity through `identity.Resolve`.
6. Secret redaction is left to the transport and no secret is logged, persisted, or printed.
7. The composition root bridges the adapter to the action-owned interface; no action imports the adapter.
8. A contract test asserts methods, paths, headers, multipart bodies, pagination, parsing, validation, jobs, and polling against the captured evidence.
9. `go test ./internal/architecture` passes, confirming the adapter's import boundaries.
