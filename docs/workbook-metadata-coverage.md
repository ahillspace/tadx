# Workbook metadata coverage

## Contract

Workbook inspection, schema, audit, and lineage expose embedded datasource metadata and direct published parents through the existing resource, metadata, and catalog owners.
The workbook REST LUID remains authoritative; embedded datasources and fields retain Metadata IDs and never become independently managed assets.
Published datasource commands retain their behavior.
Local field overrides and published source fields remain distinct, linked by provider identity rather than caption.

Audit uses existing description inheritance and object-tag checks, with separate workbook-local and published-source summaries.
Unsupported field tags are not missing tags; upstream column tags retain their source identity.
Each published parent is assessed once, while separate local field objects remain separate assessments.
The request-wide audit limit counts assessed objects, not checks.
Unknown metadata, provider failures, and bounds produce explicit incomplete coverage with confirmed evidence retained.
Embedded datasources have no invented independent permission workflow.

Schema groups fields by datasource, applies one global field limit, and supports text, role, field-ID, and exact datasource-name filters.
Ambiguous datasource names and field identifiers fail rather than select arbitrarily.
Continuation binds the request and observed schema; no workbook-schema cache is introduced.
Schema totals identify whether the observed inventory is complete.
Workbook schema does not offer logical-table filtering because this metadata path does not report those assignments.
Physical upstream tables remain provenance, not substitutes for logical tables.
Cached workbook inspection stays offline and marks datasource detail not captured.

Lineage retains existing upstream shortcuts and adds embedded containment without changing existing asset-depth meaning.
Optional field mode adds API-reported field relationships to the same bounded graph and records the requested mode in artifacts.
Asset evidence takes priority within the existing 500-node and 1,000-edge bounds.
Automatic content pulls remain asset-only at their existing defaults.

## Implementation ownership

- `internal/tableau/metadataassets` and `internal/value/metadata.go`: one workbook metadata read with shared field interpretation and independent connection pagination.
- `actions/catalog/audit` and `internal/app/catalog.go`: extend existing audit rules and wiring.
- `actions/workbook`, `internal/resources/workbook`, and `internal/app/workbook_inventory.go`: workbook schema and inspection, preserving cached reads.
- `internal/tableau/fieldcatalog` and existing datasource schema helpers: share field classification and filtering where behavior matches.
- Existing lineage provider, adapter, action, artifact, and command files: embedded nodes and optional field relationships.
- Existing CLI, capability definitions, agent guidance, and generated references: consistent discovery and output documentation.

No new workflow framework, per-verb package, standalone embedded asset, or alternate lineage sidecar is introduced.

## Verification

Write focused tests before behavior changes, then run affected packages and the full suite, formatting, vet, and the architecture gates.
Test local overrides, repeated published parents, scoped field identities, nested pagination, partial responses, graph bounds, cached inspection, and unchanged published behavior.
Use official API schemas as contract evidence and distinguish hermetic verification from live deployment verification.
No Tableau mutations, push, PR, or release are authorized by this build.

### Local verification result

The full Windows test suite and full race suite passed in a clean source snapshot containing tracked source and the feature files.
The snapshot excluded unrelated local work; architecture checks passed without changing their rules.
The final removal of unsupported logical-table filtering passed targeted action, command, capability, help, and HTTP-backed tests, including race checks.
Vet, the CLI build, formatting, module metadata, and generated-reference checks passed.
No live Tableau verification, remote mutation, CI run, commit, or push occurred.
