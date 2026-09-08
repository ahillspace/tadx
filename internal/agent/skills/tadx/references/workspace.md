# Manage workspaces and local artifacts

## Available actions

Workspace commands are local and do not accept `--environment`.
Add `--full` when machine-local roots, fingerprints, provenance, or expanded artifact state are needed.

| Action | What it does | Required and useful optional flags |
|---|---|---|
| `tadx workspace create <name>` | Create and register a workspace with a new identity. | `--path` overrides the default root. |
| `tadx workspace register [name]` | Register an existing workspace without changing its files or identity. | `--path` is required; the name is optional when the manifest supplies it. |
| `tadx workspace clone <source>` | Copy a registered workspace under a new identity. | `--name` is required; `--path` overrides the default root. |
| `tadx workspace list` | List registered workspaces. | `--limit 1..10000` |
| `tadx workspace status` | Report resolved workspace identity and managed artifact state. | `--workspace`, `--limit 1..10000` |
| `tadx workspace set-default <name>` | Set the general default to one available registered workspace. | None. |
| `tadx workspace unregister <name>` | Remove a registration while preserving every file. | None. |
| `tadx workspace delete <name>` | Remove an exact registration and its managed root. | `--preview`, `--force` |
| `tadx workspace artifact move` | Move one managed artifact between workspaces without changing Tableau identity. | `--source`, `--destination`; `--artifact`, or both `--kind` and `--id` |
| `tadx workspace artifact delete` | Delete one exact local managed artifact. | `--workspace`; `--artifact`, or both `--kind` and `--id`; `--preview`, `--force` |
| `tadx workspace clean` | Remove one class of disposable local state while preserving canonical artifacts. | `--workspace`; `--class temporary\|cache\|logs\|all` |

## Workspace model

A workspace is a named, registered local directory containing managed Tableau artifacts, metadata, and TADX state.
Creation establishes `tadx.yaml`, `artifacts/`, and `.tadx/`.
Without `--path`, create and clone use `<home>/TADX/workspaces/<name>`.
Use `--path` only to choose another machine-local root.
Workspace names are portable, case-insensitively unique, and used by `--workspace`; a filesystem path is not a workspace selector.
TADX never creates a workspace implicitly during pull.
List and status report `more_available` when output is bounded; increase `--limit` up to 10,000 to inspect more from the beginning.

Workspace selection follows this order:

1. Explicit logical `--workspace` name.
2. The registered workspace containing the current directory.
3. The selected environment's default workspace.
4. The general configured default workspace.
5. A structured failure when none resolves.

## Artifact selectors and state

Keep artifact paths workspace-relative and slash-delimited, such as `artifacts/datasource/<artifact-directory>`.
Absolute paths, parent traversal, and backslash-delimited artifact selectors are invalid.
An artifact can instead be selected by both `--kind` and authoritative Tableau `--id`.
Preserve provenance and sidecars when editing native payloads.
`workspace status` reports clean, dirty, missing, and invalid managed artifacts.

Artifact move requires different source and destination workspaces and fails on destination collision.
It preserves the artifact's Tableau identity and local dirty state.
It does not move remote Tableau content.

## Removal and cleanup

`unregister` preserves the workspace root and all files.
It refuses to remove a workspace that remains a general or environment default.
`workspace delete` removes both the registration and managed root, and rejects broad roots, identity drift, nested registered workspaces, unsafe links, and unapproved dirty or invalid state.
Use `--force` only when discarding dirty, invalid, or unmanaged local files is authorized.

`workspace artifact delete` removes only one exact local artifact.
A dirty artifact requires `--force` in addition to any mutation policy authorization.
`workspace clean` removes only the selected disposable state class and preserves canonical managed artifacts.
None of these local operations deletes Tableau content.
