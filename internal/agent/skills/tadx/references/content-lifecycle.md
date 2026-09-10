# Manage content lifecycle

## Available actions

All reads are live by default.
Add `--catalog` only for local catalog reads, and add `--full` only when expanded bounded detail is needed.
Use `--env` as an alias for any listed `--environment` flag.

| Action | What it does | Selectors and useful optional flags |
|---|---|---|
| `tadx search [term]` | Search resources, or run typed inventory when the term is omitted. | `--type workbook\|datasource\|flow\|project\|user\|group\|definition\|metric\|content\|admin\|pulse`, `--environment`, `--catalog`, `--limit 1..2000` |
| `tadx catalog refresh` | Refresh inventory; include permissions only when explicitly selected. | `--environment`, repeated `--scope users\|groups\|projects\|workbooks\|datasources\|flows\|views\|permissions` |
| `tadx catalog status` | Inspect cache freshness, scope coverage, and partial results. | `--environment`, `--full` |
| `tadx content workbook list` | List workbooks, or collect the selected inventory with `--all`. | `--environment`, `--name`, `--owner`, `--project-name`, `--tag`, `--catalog`, `--limit 1..10000` or `--all` |
| `tadx content workbook inspect` | Inspect one exact workbook. | `--id`, or `--name` with `--project`; `--environment`, `--catalog` |
| `tadx content workbook pull` | Download one or up to 100 workbooks into a workspace. | Repeat `--id`, or use `--name` with `--project`; `--environment`, `--workspace`, `--include-extract=false`, `--include-pds`, `--overwrite` |
| `tadx content workbook publish` | Publish a native file or managed workbook artifacts. | `--file`, `--id`, `--artifact-name`, or repeated `--artifact`; `--workspace`, `--environment`, `--project-id` or `--project`, `--name`, `--overwrite`, `--as-job`, `--preview` |
| `tadx content workbook move` | Move one workbook to another project on the same site. | `--environment`; `--id`, or `--name` with `--project`; `--destination-project-id` or `--destination-project`; `--preview` |
| `tadx content workbook update` | Rename a workbook, change its owner, or both. | `--environment`; `--id`, or `--name` with `--project`; `--new-name`, `--owner-id`, `--preview` |
| `tadx content workbook delete` | Delete one remote workbook. | `--environment`; `--id`, or `--name` with `--project`; `--preview` |
| `tadx content datasource list` | List published datasources, or collect the selected inventory with `--all`. | `--environment`, `--name`, `--owner`, `--project-name`, `--type`, `--tag`, `--updated-after`, `--updated-before`, `--catalog`, `--limit 1..10000` or `--all` |
| `tadx content datasource inspect` | Inspect one exact published datasource. | `--id`, or `--name` with `--project`; `--environment`, `--catalog` |
| `tadx content datasource schema` | List logical tables and return matching field metadata. | `--id`; `--environment`, `--query`, `--role measure\|dimension\|date\|excluded`, `--table`, repeated `--field-id`, `--catalog`, `--limit 1..10000` or `--all` |
| `tadx content datasource pull` | Download one or up to 100 native datasource artifacts. | Repeat `--id`, or use `--name` with `--project`; `--environment`, `--workspace`, `--overwrite` |
| `tadx content datasource publish` | Publish a native file or managed datasource artifacts. | `--file`, `--id`, `--artifact-name`, or repeated `--artifact`; `--workspace`, `--environment`, `--project-id` or `--project`, `--name`; exactly one of `--create`, `--overwrite`, `--append`, or `--replace`; `--as-job`, `--preview` |
| `tadx content datasource move` | Move one datasource to another project on the same site. | `--environment`; `--id`, or `--name` with `--project`; `--destination-project-id` or `--destination-project`; `--preview` |
| `tadx content datasource update` | Rename a datasource, change its owner, or both. | `--environment`; `--id`, or `--name` with `--project`; `--new-name`, `--owner-id`, `--preview` |
| `tadx content datasource delete` | Delete one remote datasource. | `--environment`; `--id`, or `--name` with `--project`; `--preview` |
| `tadx content flow list` | List flows, or collect the selected inventory with `--all`. | `--environment`, `--name`, `--owner`, `--project-id`, `--project-name`, `--catalog`, `--limit 1..10000` or `--all` |
| `tadx content flow inspect` | Inspect one exact flow. | `--id`, or `--name` with `--project`; `--environment`, `--catalog` |
| `tadx content flow pull` | Download one or up to 100 native flow artifacts. | Repeat `--id`, or use `--name` with `--project`; `--environment`, `--workspace`, `--overwrite` |
| `tadx content flow publish` | Publish a native file or managed flow artifacts. | `--file`, `--id`, `--artifact-name`, or repeated `--artifact`; `--workspace`, `--environment`, `--project-id` or `--project`, `--name`, `--overwrite`, `--preview` |
| `tadx content flow move` | Move one flow to another project on the same site. | `--environment`; `--id`, or `--name` with `--project`; `--destination-project-id` or `--destination-project`; `--preview` |
| `tadx content flow update` | Change one flow owner. | `--environment`; `--id`, or `--name` with `--project`; `--owner-id`, `--preview` |
| `tadx content flow delete` | Delete one remote flow. | `--environment`; `--id`, or `--name` with `--project`; `--preview` |
| `tadx content project list` | List projects, or collect the selected inventory with `--all`. | `--environment`, `--name`, `--parent-id`, `--owner`, `--top-level`, `--catalog`, `--limit 1..10000` or `--all` |
| `tadx content project inspect` | Inspect one exact project. | `--id` or `--project`; `--environment`, `--catalog` |
| `tadx content project create` | Create a top-level or nested project. | `--environment`, `--name`; `--description`, `--content-permissions`, `--parent-id` or `--parent`, `--preview` |
| `tadx content project update` | Change project name, description, or content-permission mode. | `--environment`; `--id` or `--project`; `--new-name`, `--description`, `--content-permissions`, `--preview` |
| `tadx content project move` | Reparent a project or move it to the top level. | `--environment`; `--id` or `--project`; `--parent-id`, `--parent`, or `--top-level`; `--preview` |
| `tadx content project delete` | Delete one remote project. | `--environment`, `--id`, `--preview` |
| `tadx content lineage pull` | Save bounded lineage without downloading native content. | `--kind workbook\|datasource\|flow`; `--id`, or `--name` with `--project`; `--environment`, `--workspace`, `--direction upstream\|downstream\|both`, `--depth 1..3`, `--overwrite` |

## Discovery and read source

Use `search` when a term, business concept, or approximate name is known.
Use a typed search with no term, or a resource `list`, only when inventory is required.
Use `datasource schema` for table and field discovery in TADX.
Search defaults to 20 returned results and accepts `--limit` up to 2,000.
When `more_available` is true, increase the limit or narrow the query; TADX handles provider pagination internally.
Schema defaults to 20 matching fields; use `--all` for complete matching metadata within 10,000 fields, without combining it with `--limit`.
Narrow large schema discovery by role or table when necessary.
Repeat `--field-id` to inspect several exact fields in one schema fetch; missing or ambiguous selections fail explicitly.
Use `--full` for their expanded details, and retain a sufficient limit or `--all` for the selected set.

An ordinary live `list` retrieves a bounded selection without collecting the complete resource scope or accessing SQLite.
Lists default to 25 rows and accept `--limit 1..10000`, or `--all` for all matching records within 10,000.
When `more_available` is true, use `--all` or narrow the filters.
`--all` requires complete coverage and cannot be combined with an explicit `--limit`.
Live `--all` renders the collected records and attempts to save them to the catalog.
An unfiltered complete collection atomically replaces that resource scope; filtered collections record only the selected observations.
A catalog write failure preserves the live answer with a warning.
`--full` changes presentation only.
Ordinary live lists and searches do not update the catalog.
`--catalog` is local-only and never falls back to Tableau, so a catalog miss does not prove remote absence.
Live schema reads write the retrieved schema through to the catalog, while `--catalog` reads only previously captured schema.
Catalog refresh defaults to inventory scopes without permissions; include `permissions` explicitly in the selected scopes for bulk permission reads.
Refresh replaces requested scopes and preserves unrelated observations with their original timestamps.
Catalog refresh can preserve accessible records after an item-level permission denial and report `status: partial` with `complete: false`.
Review warnings and skipped coverage through `catalog status --full`; a denied permission read is not an empty permission set.
Authentication failures still fail the refresh.

## Exact selectors

After discovery, use the returned LUID.
Name selection requires both exact `--name` and slash-delimited `--project`, and ambiguity fails.
List `--project-name` filters a leaf project name, not a project path.
Datasource `list --catalog --project-name` uses exact cached project names and indexed project LUIDs; refresh the projects and datasources scopes if required cache coverage is missing.
Older catalog schemas require an explicit refresh before local reads can use the new project identity index.
Project actions use `--id` or an exact slash-delimited `--project` path; `--project-id` remains a compatibility alias.
Literal slashes in project names remain visible alongside their LUIDs.
If a literal name and nested hierarchy produce the same display path, use the authoritative LUID; path selection reports ambiguity.
A sole configured environment is inferred; with multiple environments, remote writes require --env, including publish.

## Batches

Repeat `--id` on pull or `--artifact` on publish to process up to 100 resources of one type.
TADX validates the full selection first, preserves selection order, processes sequentially, continues after independent failures, and returns a nonzero aggregate result when any item fails.
Batch publish rejects `--name` because each artifact keeps its own name.
TADX does not infer dependency order or retry failed items.
Keep successful item results when a batch fails; resolve remaining prerequisites and retry only failed or skipped selections with known safe outcomes.
For migrations, publish datasource dependencies before workbooks that reference them.

## Managed artifacts

Pull writes native content and metadata into a registered workspace.
Workbook pull includes extracts by default.
`--include-pds` acquires only direct published datasource dependencies as sibling artifacts without recursion.
`--overwrite` is required to replace dirty local content.

Publish selects managed content with `--id`, unique `--artifact-name`, or an advanced `--artifact` directory path.
Use the source LUID returned by pull with the logical workspace; filesystem scans are unnecessary for managed publication.

```text
tadx content workbook publish --workspace '<workspace>' --id '<source-workbook-luid>' --env '<destination-alias>' --project-id '<destination-project-luid>' --preview
```

The pull result's source-update preview points back to the original site and project, not a migration destination.
Use `--file <native-file>` for an existing workbook, datasource, or flow file without a workspace.
These selectors are mutually exclusive; ambiguity fails.
Artifact selectors are workspace-relative and use forward slashes.
Artifact provenance never selects a publish target.
Publish requires an explicit destination project.
Datasource publish never infers create, overwrite, append, or replace mode.

## Mutations and uncertain outcomes

Add supported `--preview` to resolve and review a mutation without applying it, even when the mutation gate is off.
A preview does not authorize execution or changing `TADX_ENABLE_MUTATIONS`.
Before changing the flag by any mechanism, follow the explicit setting-change approval rule in the TADX root skill.
Run without `--preview` only when the requested mutation is authorized and the mutation gate is enabled.
Move operations are same-site operations, not cross-site migrations.
Content `delete` affects Tableau, while `workspace artifact delete` affects only local managed state.
`--as-job` submits a server-side job and waits for completion; it does not detach.
An asynchronous publish timeout is an unknown outcome, not proof of failure, so inspect authoritative live state before retrying.

## Lineage limits

Native pulls capture bounded lineage as metadata when available.
`lineage pull` creates a metadata-only artifact when the native package is not needed.
Lineage is evidence only within the requested direction, depth, permissions, and returned bounds.
Missing or inaccessible edges do not prove that a resource is independent.
