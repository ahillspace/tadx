# Managed policy ceilings

Read this reference when `tadx policy status` reports `active` or `error`.

An active policy allowlists canonical capability IDs and can set `remote_mutations` to `false`.
Every operation, including previews and local writes, still needs its own allowed capability ID.
A remote write also needs managed `remote_mutations: true` and saved consent for the selected server and exact site.
A denied ordinary capability cannot run with `--preview` or `--force`.
An `error` policy blocks operational commands and leaves help, samples, validation, and status available.

The fixed policy path is platform-owned.
No flag, environment variable, configuration value, or current directory changes its location.
An absent file is `unmanaged` and adds no ceiling.
Candidates are snapshots, not roles, and new IDs remain denied until an administrator updates the file.

Non-administrator templates exclude user, group, membership, permission, and shared label definition IDs.
The default cache refresh includes users and groups.
Use explicit `projects`, `workbooks`, `datasources`, `flows`, and `views` scopes when those IDs are denied.
Some non-administrator writes require administrative preflight IDs.
Compare each denial with `tadx policy status --full` before changing a candidate.

Read the repository's `docs/managed-policy.md` for schema, fixed paths, secure deployment, status checks, updates, and removal.
When the repository sources are not installed, use the [managed policy guide](https://github.com/ahillspace/tadx/blob/main/docs/managed-policy.md).
