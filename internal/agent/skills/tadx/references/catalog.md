# Upstream metadata concepts

Tableau Catalog represents upstream databases, files, tables, columns, and lineage; TADX's `cache` is separate local storage.
Catalog reads are live-only and do not populate that local cache.
Metadata GraphQL is read-only; supported edits use released REST methods.
TADX does not delete upstream databases/tables/columns or compose datasources.

## Identities and description ownership

A `metadata_id` is a GraphQL identity, not a REST LUID.
Readable metadata without a REST LUID is not automatically a supported mutation target.
Database records can represent files as well as database connections.

Datasource inspection exposes upstream database/table identities; schema enrichment connects published fields to their upstream descriptions and tags.
A published-field description and an upstream-column description can both exist, with different meanings and owners.
Keep direct and inherited descriptions distinct and retain source identities; updating one does not update the other.
Tags exposed on fields come from upstream columns, not an invented published-field tag API.
Expanded output presents fetched evidence; explicit enrichment requests determine additional metadata reads.

Metadata indexing can lag a successful edit.
Unavailable or incomplete metadata is not an empty dependency set, and an old observation is not proof that a write failed.
Metadata search matches its own indexed text, not the site-content search ranking.

## Bounded enrichment and audits

Edit only the requested properties; omitted properties and unrelated tags remain unchanged.
Do not use asset deletion to clear a description or contact when clearing is unsupported.
Compound updates can partly succeed; preserve completed properties and resolve the failed step before repeating writes.

Audits assess one selected scope, not an implicit site-wide scan.
Inherited descriptions can satisfy coverage unless direct ownership is required.
Unknown, inaccessible, or truncated observations are not missing metadata.

## Labels

An attachment LUID identifies a label applied to an asset; exact value/category names identify shared administrative vocabulary.
Asset labels and shared label definitions have separate scopes and lifecycles.
An existing value retains its category; creating a missing value requires its category and description.
Built-in vocabulary and system monitoring labels have additional restrictions.
Do not generalize a failed attachment edit into a shared-vocabulary change.
