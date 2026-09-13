# Upstream catalog metadata

## Available actions

These commands are live-only: they do not accept `--cache` or populate the local cache.
All accept `--env`, `--full`, and `--json`; remote writes infer the environment only when exactly one is configured.
Use `--preview` for a read-only mutation plan, including while execution is disabled.
For many assets, use repeated primary selectors or per-item `--batch-file` settings as described in [batching](batching.md).
Brackets below mark optional flags.

| Action | What it does | Selectors and optional flags |
| --- | --- | --- |
| `tadx catalog database list` | Discover upstream database/file identities. | [`--name <exact>`] [`--limit 1..10000` or `--all`] |
| `tadx catalog database inspect` | Inspect one database/file. | `--id <rest-luid>` or `--metadata-id <metadata-id>` |
| `tadx catalog database update` | Edit supported description, contact, or tags. | `--id <rest-luid>`; [`--description <text>`] [`--contact-id <user-luid>`] [repeated `--add-tag <tag>`] [repeated `--remove-tag <tag>`] [`--preview`] |
| `tadx catalog table list` | Discover upstream tables, optionally inside a database. | [`--database-id <rest-luid>`] [`--name <exact>`] [`--limit 1..10000` or `--all`] |
| `tadx catalog table inspect` | Inspect one table and its database identity. | `--id <rest-luid>` or `--metadata-id <metadata-id>` |
| `tadx catalog table update` | Edit supported table description, contact, or tags. | `--id <rest-luid>`; [`--description <text>`] [`--contact-id <user-luid>`] [repeated `--add-tag <tag>`] [repeated `--remove-tag <tag>`] [`--preview`] |
| `tadx catalog column list` | Discover columns inside one exact table. | `--table-id <rest-luid>` [`--name <exact>`] [`--limit 1..10000` or `--all`] |
| `tadx catalog column inspect` | Inspect one upstream column. | `--table-id <rest-luid> --id <column-rest-luid>` or `--metadata-id <metadata-id>` |
| `tadx catalog column update` | Edit a column description or tags. | `--table-id <rest-luid> --id <column-rest-luid>`; [`--description <text>`] [repeated `--add-tag <tag>`] [repeated `--remove-tag <tag>`] [`--preview`] |
| `tadx catalog search <query>` | Search database/table metadata; optionally scan one table's columns. | [repeated `--type database\|table\|column`] [`--table-id <rest-luid>` required for columns] [`--limit 1..10000` or `--all`] |
| `tadx catalog audit` | Find description/tag gaps in one database, table, or datasource scope. | `--type database\|table\|datasource --id <rest-luid>` [repeated `--check descriptions\|tags`] [`--direct-only`] [`--limit 1..10000`] |
| `tadx catalog label list` | List labels attached to one asset. | `--type <asset-type> --target-id <rest-luid>` [repeated `--category <name>`] [`--limit 1..10000`] |
| `tadx catalog label inspect` | Inspect an exact label attachment. | `--id <label-luid>` [`--type <asset-type> --target-id <rest-luid>`] |
| `tadx catalog label update` | Apply a label or edit one attachment. | `--id <label-luid>` or `--type <asset-type> --target-id <rest-luid> --value <name>`; [`--value <name>`] [`--message <text>`] [`--active=true\|false`] [`--elevated=true\|false`] [`--preview`] |
| `tadx catalog label delete` | Remove an attachment, not the asset or shared vocabulary. | `--id <label-luid>` [`--type <asset-type> --target-id <rest-luid>`] [`--preview`] |
| `tadx admin label value list` | List shared label values. | [`--limit 1..10000`] |
| `tadx admin label value inspect` | Inspect a shared value by exact name. | `--name <name>` |
| `tadx admin label value update` | Create or update a shared value; category is immutable once created. | `--name <name>` [`--new-name <name>`] [`--category <name>` for creation] [`--description <text>`] [`--preview`] |
| `tadx admin label value delete` | Delete a shared value definition. | `--name <name>` [`--preview`] |
| `tadx admin label category list` | List shared label categories. | [`--limit 1..10000`] |
| `tadx admin label category inspect` | Inspect a category by exact name. | `--name <name>` |
| `tadx admin label category create` | Create a shared category. | `--name <name> --description <text>` [`--preview`] |
| `tadx admin label category update` | Rename or describe a shared category. | `--name <name>` [`--new-name <name>`] [`--description <text>`] [`--preview`] |
| `tadx admin label category delete` | Delete a shared category. | `--name <name>` [`--preview`] |

## Identity and scope

`metadata_id` is a GraphQL identity, not a REST LUID; only use it with `--metadata-id` for inspection.
Use returned `luid` values for mutations and parent selectors; a readable asset without a REST LUID is not a supported write target.
Database resources can represent files as well as database connections.
Supported edits use released REST methods; Metadata GraphQL itself is read-only.
These commands do not delete databases, tables, or columns, edit published datasource fields, or mutate virtual connections.

Catalog list/search defaults to 25 matches, while label lists default to 20.
`--all` is a bounded collection, not a request to refresh TADX's cache.
Database/table search uses Metadata text filters, not Tableau's site-content search ranking.
Column search matches names/descriptions locally inside the selected table; its scan is bounded and reports incomplete coverage rather than claiming absence.
`more_available` or `complete: false` means the returned set is not exhaustive.

## Start from a datasource

`content datasource inspect --id <luid>` includes available upstream database/table names and identities.
`--full` expands already-fetched descriptions and metadata; it does not trigger additional requests.
Use `content datasource schema --id <luid> --descriptions` for direct field descriptions and distinct inherited/upstream descriptions with source identities.
Add `--tags` for upstream column tags, not invented published-field tags.
Keep ordinary schema filters and limits to narrow the field set.
Unavailable or incomplete metadata is not an empty dependency set.

## Change only the intended metadata

Omitted update properties remain unchanged; adding or removing tags preserves unrelated tags.
Description/contact clearing is not verified and is rejected; do not substitute asset deletion or an empty contact identifier.
Compound updates can partly succeed: preserve returned identities and completed properties, then resolve the failed step without blindly repeating successful writes.
Content label targets are database, table, column, datasource, or flow, not workbook/project/view.
Attachment LUIDs identify applied labels; exact value/category names identify shared administrative definitions.
Creating a missing label value through `value update` requires its category and a nonempty description; existing values keep their category.
System monitoring labels and built-in vocabulary have additional restrictions; do not treat them as ordinary editable labels.

Audit requires one explicit scope and defaults to all supported checks there, not a site-wide scan.
Database/table scopes include bounded descendants; datasource scope includes its fields for description assessment.
Inherited descriptions count unless `--direct-only` is selected.
Unknown, inaccessible, or truncated observations are not counted as missing metadata.
Compact audit output shows gaps and unknowns; `--full` also includes successful assessments from the same reads.
