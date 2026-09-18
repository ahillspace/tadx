# Workspaces and managed artifacts

A workspace is a named, registered local directory containing `tadx.yaml`, `artifacts/`, and `.tadx/`.
Names are case-insensitively unique; a workspace selector is a name, not a filesystem path.
Default creation uses `<home>/TADX/workspaces/<name>`; a custom root is optional.
TADX does not silently invent a workspace during a pull.

Explicit selection wins, then the registered workspace containing the current directory, then the environment default, then the general default.
Changing directories can therefore change the resolved workspace; the session overview reports selection.
Workspace location does not select a remote publish target.

## Files and identity

Register adopts an existing managed workspace, not an ordinary folder.
Clone copies it under a new workspace identity; moving an artifact between workspaces preserves its Tableau identity and dirty state.
Neither operation moves remote content.

Managed artifacts retain native files, provenance, and sidecars.
Source LUIDs help select local items but do not identify the destination of a publish.
Keep managed paths workspace-relative with forward slashes; reuse returned paths rather than reconstructing hashed directory names or rereading expanded status.
An existing native file can be published directly without manufacturing managed metadata.

Clean means payload and baseline agree; dirty means local changes, while missing or invalid state needs attention before replacement.
Preserve sidecars during native editing and do not overwrite dirty work merely to make a command pass.

## Removal

Unregister removes only registration; workspace deletion removes the managed root as well.
Reassign defaults before removing a workspace they reference when other workspaces remain.
Removing the sole workspace can clear those defaults without creating a replacement.
Cleanup of temporary state preserves canonical artifacts; artifact deletion removes selected local content only.
Discarding dirty, invalid, or unmanaged files requires deliberate authorization, even when force is available.
None of these operations deletes Tableau content.

Guidance install/uninstall receipts report actual package and preserved-backup paths.
An install backup is an older local package retained for recovery, not a failed new installation.
Previews describe planned preservation without promising a not-yet-generated backup name.
