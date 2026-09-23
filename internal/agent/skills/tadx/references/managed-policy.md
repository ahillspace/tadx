# Managed policy ceilings

Read this reference when `tadx policy status` reports `active` or `error`.

An active policy allowlists canonical capability IDs and can set `remote_mutations` to `false`.
Every operation, including previews and local writes, still needs its own allowed capability ID.
A remote write also needs managed `remote_mutations: true` and saved consent for the selected server and exact site.
A denied ordinary capability cannot run with `--preview` or `--force`.
An `error` policy blocks operational commands and leaves help, samples, validation, status, and administrator installation available.

The policy location is platform-owned.
On Windows, `tadx policy install` requests UAC for a scoped installer child and keeps the caller unelevated.
On Linux and macOS, an unprivileged install uses `/usr/bin/sudo` through the controlling terminal; `sudo` handles its own prompt, and TADX does not capture passwords.
Its optional `--output` selects a dedicated local absolute directory, recorded in a protected machine-wide locator.
Omitting `--output` selects the platform default path, and the default template is `superuser`.
The defaults are `Program Files\TADX` on Windows, `/etc/tadx` on Linux, and `/Library/Application Support/TADX` on macOS.
Installation overwrites the selected policy, preserves old locations, and never changes saved site consent.
Ordinary commands cannot override the active location through flags, environment variables, or user configuration.
An absent default file is `unmanaged` only when there is no platform locator.
An invalid locator or missing located policy fails closed.
Windows verifies file and immediate-directory ownership/DACLs and path integrity; Unix requires root ownership and no group or world write access throughout the path.
Windows ancestor permissions are warning-only.
When `state: active`, `protected: true` with `path_protected: false` means the loaded policy is enforced but its path may permit substitution, including an older, more permissive policy.
Check `state` separately: the protection flags do not establish valid JSON or an active policy.
The warning does not disable the loaded policy's restrictions or authorize changes to machine permissions, the policy, or saved site consent.
Candidates are snapshots, not roles, and new IDs remain denied until an administrator updates the file.

All three standard templates allow every current read, including administrative inventory and the default cache refresh.
The `read-only` template blocks all Tableau mutations.
The `read-write-no-admin` template allows non-administrative mutations but excludes administrative remote mutations.
The `superuser` template includes every current capability ID and allows remote mutations, subject to saved site consent.
The accepted legacy template input `admin` is an alias for `superuser`, not a separate role.
Custom policies can still deny reads by omitting their capability IDs.
Some non-administrator writes require administrative preflight IDs.
Compare each denial with `tadx policy status --full` before changing a candidate.
Templates snapshot the current capability registry; explicitly reinstall a standard template with the current CLI to refresh its IDs.
An active policy is not updated automatically when TADX changes.

Read the repository's `docs/managed-policy.md` for schema, system locations, secure deployment, status checks, updates, and removal.
When the repository sources are not installed, use the [managed policy guide](https://github.com/ahillspace/tadx/blob/main/docs/managed-policy.md).
