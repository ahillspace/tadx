# Workbook metadata coverage

## Contract

Workbook inspection, schema, audit, and lineage expose embedded datasource metadata and direct published parents through the existing resource, metadata, and catalog owners.
The workbook REST LUID remains authoritative; embedded datasources and fields retain Metadata IDs and never become independently managed assets.
Published datasource commands retain their behavior.
Local field overrides and published source fields remain distinct, linked by provider identity rather than caption.

Audit uses existing description inheritance and object-tag checks, with separate workbook-local and published-source summaries.
Audit reads only the field descriptions, tags, and parent identities needed for assessment.
Inspection-only sheet, flow, and upstream object relationships do not determine audit completeness.
Unsupported field tags are not missing tags; upstream column tags retain their source identity.
Each published parent is assessed once, while separate local field objects remain separate assessments.
The request-wide audit limit counts assessed objects, not checks.
Unknown metadata, provider failures, and bounds produce explicit incomplete coverage with confirmed evidence retained.
Embedded datasources have no invented independent permission workflow.

Schema groups fields by datasource, applies one global field limit, and supports text, role, field-ID, and exact datasource-name filters.
Ambiguous datasource names and field identifiers fail rather than select arbitrarily.
Continuation binds the request and observed schema; no workbook-schema cache is introduced.
Schema totals identify whether the observed inventory is complete.
Published groups use the existing datasource schema reader and retain its logical-table assignments and field classification.
Embedded groups use Metadata API observations and do not invent logical-table assignments.
Workbook schema filters by datasource group rather than logical table.
Physical upstream tables remain provenance, not substitutes for logical tables.
Compact field output and description and tag projection use the same contracts as datasource schema.
Remote fields reuse confirmed published semantic facts through exact Metadata IDs, including when workbook captions differ.
Cached workbook inspection stays offline and marks datasource detail not captured.
Live inspection caches confirmed REST details independently of optional metadata enrichment failures.

Lineage retains existing upstream shortcuts and adds embedded containment without changing existing asset-depth meaning.
Optional field mode adds API-reported field relationships to the same bounded graph and records the requested mode in artifacts.
Asset evidence takes priority within the existing 500-node and 1,000-edge bounds.
Automatic content pulls remain asset-only at their existing defaults.

## Implementation ownership

- `internal/tableau/metadataassets` and `internal/value/metadata.go`: workbook identity and inventory facts, focused field-description reads, and inspection graph coverage with independent connection pagination.
- `actions/catalog/audit` and `internal/app/catalog.go`: extend existing audit rules and wiring.
- `actions/workbook` and `internal/resources/workbook`: workbook-specific selection, grouped schema, identity validation, and output projection.
- `internal/app/workbook_inventory.go`: inspection enrichment sequencing, REST cache persistence, and schema dependency wiring.
- `internal/tableau/fieldcatalog` and the existing datasource schema adapter: canonical published schema acquisition and classification, plus embedded dependency analysis using confirmed remote semantic facts.
- `internal/value/schema.go`: shared compact field projection, filtering, and description and tag suppression.
- Existing lineage provider, adapter, action, artifact, and command files: embedded nodes and optional field relationships.
- Existing CLI, capability definitions, agent guidance, and generated references: consistent discovery and output documentation.

No new workflow framework, per-verb package, standalone embedded asset, or alternate lineage sidecar is introduced.

## Verification

Write focused tests before behavior changes, then run affected packages and the full suite, formatting, vet, and the architecture gates.
Test local overrides, repeated published parents, scoped field identities, nested pagination, partial responses, graph bounds, cached inspection, and unchanged published behavior.
Use official API schemas as contract evidence and distinguish hermetic verification from live deployment verification.
No Tableau mutations, push, PR, or release are authorized by this build.

### Initial local verification result

The full Windows test suite and full race suite passed in a clean source snapshot containing tracked source and the feature files.
The snapshot excluded unrelated local work; architecture checks passed without changing their rules.
The final removal of unsupported logical-table filtering passed targeted action, command, capability, help, and HTTP-backed tests, including race checks.
Vet, the CLI build, formatting, module metadata, and generated-reference checks passed.
No live Tableau verification, remote mutation, CI run, commit, or push occurred.

### Repair validation

The embedded relation query now passes real GraphQL syntax and schema validation.
Metadata asset and lineage HTTP fixtures validate every generated query against a checked-in schema mechanically derived from Tableau's official current reference.
The schema fixture includes provenance and a reproducible generator; it does not establish compatibility with every deployment version.
Singleton `idWithin` filters preserve exact Metadata ID selection and avoid the scalar-ID execution errors observed during live embedded and field reads.

Repair regressions cover confirmed sibling and parent identity retention, published identity drift across enrichment and pagination, embedded aggregate provenance limits, filtered partial schema results, and field-lineage request and page bounds.
Field enrichment permits at most 1,000 HTTP requests and 100 pages per connection, rejects empty continuing pages and oversized responses, and preserves the failed relationship with confirmed partial graph evidence.
Embedded and published datasource enrichment share the aggregate 10,000-column bound.
Workbook and flow publication help identifies managed artifact directories and standalone supported native files without changing publication behavior.

Rebuilt native inspection succeeds on both affected workbooks, observing 22 fields in one embedded datasource and 19 fields in each of two embedded datasources.
Field lineage captures complete graphs with 24 nodes and 32 edges, and 41 nodes and 44 edges, including calculation references.
Audits assess all 23 and 39 objects, respectively, while keeping 22 and 38 unknown field descriptions distinct from missing metadata.
Published datasource inspection and all 29 schema fields remain verified, and published auditing retains its unknown metadata results.
Published field lineage captures seven columns, one table, and one database in a complete graph.

A workbook containing only a published-parent-backed datasource retains 470 local fields but receives inconsistent native sheet pagination.
The first page returns 100 of 148 sheets with a continuation cursor; the next returns an empty terminal page with the same total.
The earlier inspection and audit paths report incomplete coverage while retaining confirmed identities and findings.
Focused audit reads now exclude the unrelated sheet relationship, while inspection continues to report its graph coverage independently.
Its audit confirms 26 inherited descriptions; direct-only assessment keeps those descriptions unknown.
Its field lineage preserves physical and remote-field relationships in a 304-node graph and explicitly reports the 1,000-edge bound.
These observations do not establish complete published-reference enrichment or complete lineage for that workbook.

The complete feature and focused repairs receive independent source review, with all identified source findings closed.
The full Windows test suite and vet pass in an isolated source snapshot that preserves native Git history and excludes unrelated work.
The unchanged architecture and help-length gates pass, and all four CI platform cross-builds succeed.
The full Windows race suite also passes.

Successful live workbook schema remains unverified because managed policy denies the action.
A live workbook with published references and independent embedded datasources remains unverified pending an available seed.
Positive column-tag values and complete metadata for the workbook with inconsistent sheet pagination remain unverified live.
These gaps prevent an unconditional production-readiness claim.

### Datasource alignment repairs

Published workbook groups use the existing datasource schema adapter and its VDS read, VDS describe, and Metadata API fallback methods.
The canonical field catalog retains hidden dependency semantics separately from display hiding, preserving source exclusions and aggregation requirements for renamed workbook references.
Standalone datasource output retains its existing behavior.
No additional provider requests or parallel classifier are introduced.

CLI HTTP regressions reproduce and verify cache recovery after metadata failure, audit independence from sheet pagination, canonical VDS schema facts, provider retry advice, and hidden dependency semantics.
Focused tests, race tests, and vet pass, and independent comparative reviews sign off on the repaired owners and contracts.
Live audit and inspection checks agree with the downloaded native description counts and datasource identities.
Successful live workbook schema remains blocked by managed policy until an administrator completes the approved policy refresh.
