# Named workspaces

TADX separates a portable logical workspace name from its machine-local root.
Create and clone use `<home>/TADX/workspaces/<name>` by default.
Use `--path` to override that location.
Register still requires the exact existing root.
Lifecycle commands use the logical name and never accept that root as the `--workspace` value.

## Create a workspace

```text
tadx workspace create development
```

The command creates `tadx.yaml`, `artifacts/`, and `.tadx/` at the new root and records a stable workspace ID in the non-secret user configuration.
The default root is `<home>/TADX/workspaces/development` on Windows, macOS, and Linux.
Pass `--path <machine-local-workspace-root>` to create or clone at another location.
Workspace names are unique under case-insensitive comparison.
Names must remain portable across supported operating systems, so path separators, Windows-invalid characters, reserved device names, and trailing dots or spaces are rejected.
Canonical roots are also unique, so one directory cannot be registered under two names.
Use `--full` with workspace create, clone, list, or status to discover registered machine-local roots.

## Select a workspace

Commands resolve a workspace in this order:

1. The exact logical name supplied with `--workspace`.
2. The registered workspace containing the current directory.
3. The selected environment's configured default workspace.
4. The general configured default workspace.
5. A structured failure when no workspace can be selected.

Pull and publish never create a workspace implicitly.
Use `tadx workspace list` to discover registered names and `tadx workspace status --workspace <name>` to inspect one bounded artifact page.

## Select an artifact

Artifact selectors are relative to the resolved workspace and use slash-delimited managed paths.
Managed artifact roots include workbooks, datasources, flows, Pulse definitions, and standalone lineage captures.

```text
tadx content workbook publish --workspace development --artifact "artifacts/workbook/<artifact-directory>" --environment production --project "Department/Ops"
```

TADX rejects absolute artifact selectors, parent traversal, backslash-delimited persisted selectors, and paths outside a managed artifact root.
Artifact fields remain portable and workspace-relative in compact and full output.
Workspace create, clone, list, and status omit the registered root from compact output and expose it only under `--full`.

## Move or delete local artifacts

`tadx workspace artifact move` transfers one exact managed artifact between two registered workspaces without changing its Tableau identity.
Move requires explicit source and destination workspace names.
The move fails on a destination collision or any concurrent payload, metadata, or sidecar change.

A move or deletion can select an artifact by its workspace-relative path, or by kind and Tableau LUID.
Use `--preview` to inspect the exact local change before applying it.

`tadx workspace artifact delete` removes one exact local artifact.
A dirty artifact also requires `--force`, and the artifact is revalidated atomically before removal.
Remote content deletion remains a separate resource action.

## Unregister, delete, and clean

`tadx workspace unregister <name>` removes a registration while preserving the workspace directory and its contents.
Use it when the registry entry is obsolete or the workspace root is unavailable.

`tadx workspace delete <name>` removes the registered workspace and its root.
It protects unrecognized or changed files as dirty state; inspect the preview and use `--force` only when you intend to remove that state.
Deletion is blocked when the workspace is still configured as a default.

`tadx workspace clean --workspace <name> --class <class>` removes disposable data under `.tadx/` while preserving managed artifacts.
Classes are `temporary` (`tmp` and `staging`), `cache`, `logs`, and `all`.
Use `--preview` to inspect the selected cleanup.
Cleaning temporary data fails while a download is using it.

## Breaking changes

Named workspaces change two previously accepted invocations.
Update existing scripts before upgrading.

`--workspace` now takes a logical registry name, not a filesystem path.
A directory path passed to `--workspace` no longer resolves to that directory; create or register the root under a logical name and pass that name thereafter.
Use `tadx workspace register <name> --path <root>` to adopt an existing workspace directory.

`content workbook publish --artifact` now requires a workspace-relative managed path such as `artifacts/workbook/<artifact-directory>`.
It no longer accepts a bare directory or a canonical payload path; absolute selectors, parent traversal, and backslash-delimited selectors are rejected.
