# Named workspaces

TADX separates a portable logical workspace name from its machine-local root.
The root path is supplied once during registration.
Lifecycle commands use the logical name and never accept that root as the `--workspace` value.

## Register a workspace

```text
tadx workspace create development --path "<machine-local-workspace-root>"
```

The command creates `tadx.yaml`, `artifacts/`, and `.tadx/` at the new root and records a stable workspace ID in the non-secret user configuration.
Workspace names are unique under case-insensitive comparison.
Canonical roots are also unique, so one directory cannot be registered under two names.

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

```text
tadx content workbook publish --workspace development --artifact "artifacts/workbook/<artifact-directory>" --environment production --project "Department/Ops"
```

TADX rejects absolute artifact selectors, parent traversal, backslash-delimited persisted selectors, and paths outside a managed artifact root.
Compact and full output contain portable relative artifact paths and never reveal the registered machine-local root.

## Move or delete local artifacts

`workspace.move` transfers one exact managed artifact between two registered workspaces without changing its Tableau identity.
The move fails on a destination collision or any concurrent payload, metadata, or sidecar change.

`workspace.artifact.delete` previews one exact local deletion and requires `--apply` to perform it.
A dirty artifact also requires `--force`, and the artifact is revalidated atomically before removal.
Remote content deletion remains a separate resource action.
