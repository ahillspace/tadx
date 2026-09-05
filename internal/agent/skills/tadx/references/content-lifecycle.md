# Manage shared content lifecycle

Workbooks, published datasources, and flows share exact discovery, inspect, pull, publish, and delete operations.
Check the resource's operation help for differences; flags and publish modes are not interchangeable.

## Inspect and pull

Resolve the remote LUID and source environment, then inspect current state before acquiring or changing content.
Use a registered workspace and keep the returned managed artifact path for subsequent operations.
Pull writes local artifacts; `--overwrite` replaces dirty local content only when the task authorizes that replacement.
Workbook pull includes extracts by default.
Use `--include-extract=false` when extracts are unnecessary, and `--include-pds` only when direct published datasource dependencies are needed.
Dependency acquisition creates sibling artifacts without recursion.

Datasource and flow pull preserve native artifacts.
For field metadata, use `content datasource schema --id <luid> --query <term>` with bounded filters instead of downloading content unnecessarily.
For dependencies without a native download, use `content lineage pull` with a bounded direction and depth.
Lineage is evidence within returned bounds; missing or inaccessible edges do not prove independence.

## Publish and verify

Inspect workspace status for artifact provenance and dirty or missing state.
Publish takes a managed artifact directory, such as `artifacts/workbook/<artifact-directory>`, not a payload file or absolute path.
Set the intended environment and destination project explicitly when promoting content; publish can otherwise use the artifact's recorded source environment.
Use authoritative destination project IDs when available.
Choose collision behavior intentionally, and inspect command help for resource-specific overwrite, append, replace, or create modes.

When review is needed, add `--preview` to the intended publish command.
To perform the authorized operation, run it without `--preview` with mutations enabled.
Inspect the returned authoritative identity and bounded outcome; use a live read to resolve uncertain results before retrying.
The `--as-job` option polls supported publishes to a bounded result; do not interpret a timeout as proof of failure.

## Distinguish remote and local changes

Content `delete` removes remote Tableau content.
`workspace artifact delete` removes a local managed artifact.
`workspace move` transfers a local artifact between workspaces and preserves Tableau identity.
Content `move` changes the remote project without downloading or republishing the item.
Inspect the exact target and applicable help before destructive changes.

## Move and update remote content

Use authoritative LUIDs after discovery, and use `--preview` when the requested destination or field change needs review.

```text
tadx content workbook move --environment <alias> --id <workbook-luid> --destination-project-id <project-luid> --preview
tadx content datasource move --environment <alias> --id <datasource-luid> --destination-project-id <project-luid> --preview
tadx content workbook update --environment <alias> --id <workbook-luid> --new-name "<name>" --owner-id <user-luid> --preview
tadx content datasource update --environment <alias> --id <datasource-luid> --new-name "<name>" --owner-id <user-luid> --preview
tadx content flow update --environment <alias> --id <flow-luid> --owner-id <user-luid> --preview
```

For move, use exactly one destination selector: `--destination-project-id` or `--destination-project`.
Workbook and datasource update accept `--new-name`, `--owner-id`, or both.
Flow update changes only the owner because Tableau has no supported flow rename operation.
Every content update or move requires an explicit environment and revalidates the exact remote identity.
