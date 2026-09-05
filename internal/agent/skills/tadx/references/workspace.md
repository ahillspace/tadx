# Manage workspaces and local artifacts

Create a named workspace with `tadx workspace create <name>`.
Use `--path "<root>"` only when choosing a different filesystem location.
To adopt an existing workspace, use `tadx workspace register <name> --path "<root>"`.
Workspace commands are local and do not accept `--environment`.
Creation establishes `tadx.yaml`, `artifacts/`, and `.tadx/`; registration uses the existing workspace identity.
Workspace names are portable and unique without case sensitivity.

Workspace selection follows this order:

1. Explicit logical `--workspace` name.
2. Registered workspace containing the current directory.
3. Selected environment's default workspace.
4. General configured default workspace.
5. Structured failure when no workspace resolves.

Use workspace `list` or `status --full` to discover the registered root when needed.
Keep artifact selectors workspace-relative and slash-delimited, such as `artifacts/datasource/<artifact-directory>`.
Absolute paths, parent traversal, and backslash-delimited artifact selectors are invalid.
Preserve provenance and sidecars when editing native payloads; workspace status reports dirty and missing artifacts.

Use `workspace clone` to copy a managed workspace under a new identity.
Use `workspace move` with explicit source and destination names to transfer one artifact without changing its Tableau identity.
Destination collisions fail; choose another destination rather than overwriting unrelated artifacts.

`workspace clean --class <class>` removes the selected disposable state, preserving canonical artifacts.
Inspect help for supported classes.
`workspace artifact delete` removes one exact artifact by default and supports `--preview`.
A dirty artifact additionally requires `--force`; use it only when discarding those edits is authorized.
Local cleanup does not remove remote Tableau content.
