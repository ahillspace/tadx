# Managed policy

TADX can enforce an administrator-owned capability ceiling for every installed TADX process on a machine.
The ceiling is optional, machine-wide, and separate from Tableau permissions and site mutation consent.

This document describes the installed TADX implementation and its fixed operating-system policy file.
It does not describe privileged agent containment or controls for other clients on the machine.

## Understand the policy boundary

TADX loads the policy from one fixed operating-system path at each command and worker boundary.
TADX does not discover that path from a flag, environment variable, configuration value, or current directory.
TADX does not ship an active policy, enroll a machine, or provide a policy removal command.

The policy can narrow TADX operations, but it cannot grant Tableau permissions.
Remote writes require all of the following conditions:

- No managed policy is installed, or the canonical capability ID is in the active policy allowlist.
- The active policy sets `remote_mutations` to `true`, or no policy is installed.
- Saved consent is enabled for the selected environment's canonical server and exact site.

An allowed capability can still fail because Tableau rejects the request, the credential lacks access, or a required prerequisite is unavailable.
When a managed policy is active, an unlisted ordinary capability cannot run, even with `--preview` or `--force`.
Preview remains read-only when its capability ID is allowed and the policy sets `remote_mutations` to `false`.
When a managed policy is active, read-only operations can write local downloads, caches, workspaces, or configuration only when their own IDs are allowed.

The non-administrator templates intentionally exclude administrative inventory.
The default `tadx cache refresh` scope includes users and groups, so it is not a non-administrator cache command.
Choose explicit non-administrator scopes when using those templates:

```text
tadx cache refresh --scope projects --scope workbooks --scope datasources --scope flows --scope views --environment dev
```

`tadx search --type user` requires `admin.user.list` even with `--cache`.
An untyped broad search can include users and groups, so choose a resource type when the policy excludes administrative IDs.
Content label updates and deletes can require `admin.label.value.inspect` as a preflight, including for `--preview`.

An administrator can remove the fixed policy file manually.
After removal, TADX reports an unmanaged state and applies no machine-wide ceiling.
No enrollment or active-policy record remains after removal.

## Find the fixed policy path

TADX resolves the path at runtime as follows:

| Platform | Fixed path |
| --- | --- |
| Windows | The native operating-system `Program Files` directory, followed by `TADX\managed-policy.json`. |
| macOS | `/Library/Application Support/TADX/managed-policy.json`. |
| Linux | `/etc/tadx/managed-policy.json`. |

Windows resolves the native `Program Files` directory through the Windows known-folder API.
The implementation checks the native 64-bit location first, then the platform fallback.
Do not substitute `%ProgramFiles%`, a user directory, or a process-specific path.

The parent directory and every ancestor are part of the protection boundary.
The policy file must be a regular file with exactly one hard link.
Symbolic links, Windows reparse points, and insecure ancestors cause a policy error.

## Create a candidate

Use the recovery command to create three editable candidates in a user-selected directory:

```text
tadx policy samples --output ./tadx-policy-candidates
```

The command creates these files:

- `read-only.json` includes all current non-administrative capability IDs and sets `remote_mutations` to `false`.
- `read-write-no-admin.json` includes all current non-administrative capability IDs and sets `remote_mutations` to `true`.
- `admin.json` includes every current capability ID, including administrative IDs, and sets `remote_mutations` to `true`.

The command creates new files only.
It never overwrites an existing candidate, installs a policy, activates a policy, or changes file protection.
The output reports the fixed system path and platform deployment guidance.
No elevation is required to create candidates in a user-owned directory.

Templates are snapshots of the current capability registry.
They are not roles, and a new capability remains denied until an administrator updates the allowlist.
The read-only template includes remote mutation IDs, so allowed previews remain available while remote writes remain denied.
It permits allowed ordinary local operations such as downloads, cache refreshes, workspace changes, and configuration changes.

## Edit the JSON schema

Every candidate must contain exactly these three fields:

```json
{
  "version": 1,
  "allowed_capabilities": [
    "workbook.inspect",
    "workbook.pull"
  ],
  "remote_mutations": false
}
```

The schema has these requirements:

- `version` is the integer `1`.
- `allowed_capabilities` is a non-null array of canonical capability ID strings.
- Every ID exists in the current registry.
- Every ID occurs at most once.
- `remote_mutations` is a non-null Boolean.
- Unknown fields, duplicate fields, duplicate IDs, malformed JSON, trailing data, and unknown IDs are rejected.

Use `tadx capability list --all --json` when the policy allows `capability.list` and you need current registry IDs.
If the policy denies that capability, use the generated [capability reference](reference/capabilities.md).
Use the exact `id` values from that output.
Do not copy display labels or invent IDs from command paths.

The registry can gain IDs in a later TADX release.
An older policy does not grant those new IDs automatically.
Update the policy deliberately when the installed registry changes.

## Validate a candidate

Validate a candidate before deployment:

```text
tadx policy validate ./tadx-policy-candidates/read-only.json
```

Validation checks the JSON schema and the current capability IDs.
Validation does not check ownership, permissions, directory protection, activation, or machine-wide policy state.
Validation does not modify the candidate or the fixed policy path.

The command remains available when the active policy is invalid or blocks other operations.
Treat a valid candidate as input for administrator deployment, not as evidence of activation.

## Deploy a policy

An administrator deploys the selected candidate to the fixed path.
Protect the file, its TADX parent directory, and every ancestor before checking status.
Keep the prior policy in a protected backup outside the fixed path until the replacement is confirmed.

### Deploy on Windows

Run the following outline from an elevated PowerShell session.
The `ProgramFiles` value comes from the operating system, not an environment variable.
Run the outline in 64-bit PowerShell when both 32-bit and 64-bit runtimes are installed.

```powershell
$programFiles = [Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFiles)
$policyDir = Join-Path $programFiles 'TADX'
$policyPath = Join-Path $policyDir 'managed-policy.json'
$candidate = (Resolve-Path '.\tadx-policy-candidates\read-write-no-admin.json').Path

New-Item -ItemType Directory -Force -Path $policyDir | Out-Null
# Inspect an existing target first. Do not copy through a link or reparse point.
Copy-Item -LiteralPath $candidate -Destination $policyPath -Force

icacls.exe $policyDir /setowner '*S-1-5-32-544'
icacls.exe $policyPath /setowner '*S-1-5-32-544'
icacls.exe $policyDir /inheritance:r
icacls.exe $policyDir /grant:r '*S-1-5-18:(OI)(CI)(F)' '*S-1-5-32-544:(OI)(CI)(F)' '*S-1-5-11:(OI)(CI)(RX)'
icacls.exe $policyPath /inheritance:r
icacls.exe $policyPath /grant:r '*S-1-5-18:(F)' '*S-1-5-32-544:(F)' '*S-1-5-11:(R)'

icacls.exe $policyDir
icacls.exe $policyPath
```

The example grants full control to LocalSystem and built-in Administrators.
It grants read and traverse access to Authenticated Users.
Review and remove any additional non-administrator grants that include write, delete, ownership, or DACL rights.
Review the ACLs on all ancestors, including the native `Program Files` directory.

TADX accepts an owner of LocalSystem, built-in Administrators, or TrustedInstaller for each checked object.
TADX rejects non-administrator effective modification rights on the file or its protected path.
TADX also rejects reparse points and files with more than one hard link.

The [Windows known-folder API](https://learn.microsoft.com/en-us/windows/win32/api/shlobj_core/nf-shlobj_core-shgetknownfolderpath) defines the operating-system path lookup.
The [Windows security descriptor API](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-getsecurityinfo) and [file security model](https://learn.microsoft.com/en-us/windows/win32/fileio/file-security-and-access-rights) explain the owner and DACL checks.
Use the [icacls reference](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/icacls) for the command syntax.

### Deploy on Linux

Run the following outline with `sudo`:

```sh
sudo install -d -o root -g root -m 0755 /etc/tadx
sudo install -o root -g root -m 0644 \
  ./tadx-policy-candidates/read-write-no-admin.json \
  /etc/tadx/managed-policy.json
sudo chown root:root /etc/tadx /etc/tadx/managed-policy.json
sudo chmod 0755 /etc/tadx
sudo chmod 0644 /etc/tadx/managed-policy.json
```

The file and every ancestor must be owned by `root`.
The file and every ancestor must deny group and other write permission.
The file must be a regular file with one hard link.
Do not deploy through a symbolic link.

If the path uses extended POSIX ACLs, inspect their effective permissions with `getfacl`.
Remove extended entries that grant write access, or remove the ACL before applying the root-owned modes:

```sh
sudo getfacl /etc/tadx /etc/tadx/managed-policy.json
sudo setfacl -b /etc/tadx /etc/tadx/managed-policy.json
sudo chmod 0755 /etc/tadx
sudo chmod 0644 /etc/tadx/managed-policy.json
```

The Linux check uses the group mode bits as the effective POSIX ACL mask.
Removing group write permission therefore removes effective ACL write access.
Review every ancestor with `namei -l /etc/tadx/managed-policy.json`.

The [Linux `open(2)` reference](https://man7.org/linux/man-pages/man2/open.2.html) documents no-follow path handling.
The [Linux `acl(5)` reference](https://man7.org/linux/man-pages/man5/acl.5.html) documents effective ACL permissions.

### Deploy on macOS

Run the following outline with `sudo`:

```sh
sudo install -d -o root -g wheel -m 0755 "/Library/Application Support/TADX"
sudo install -o root -g wheel -m 0644 \
  ./tadx-policy-candidates/read-write-no-admin.json \
  "/Library/Application Support/TADX/managed-policy.json"
sudo chown root:wheel \
  "/Library/Application Support/TADX" \
  "/Library/Application Support/TADX/managed-policy.json"
sudo chmod 0755 "/Library/Application Support/TADX"
sudo chmod 0644 "/Library/Application Support/TADX/managed-policy.json"
```

The file and every ancestor must be owned by `root`.
The file and every ancestor must deny group and other write permission.
The file must be a regular file with one hard link.
Do not deploy through a symbolic link.

macOS extended ACLs are inspected through the native security attributes.
Read-only, deny, and inherit-only entries can pass.
An effective modification grant that TADX cannot prove is root-only fails closed.
If an administrator intends to remove extended ACLs, run `chmod -N` on the policy directory and file before the final mode check:

```sh
sudo chmod -N "/Library/Application Support/TADX"
sudo chmod -N "/Library/Application Support/TADX/managed-policy.json"
sudo chmod 0755 "/Library/Application Support/TADX"
sudo chmod 0644 "/Library/Application Support/TADX/managed-policy.json"
```

Review the path with `ls -ldeO` and check every ancestor before running TADX.
The [Apple `getattrlist(2)` source](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/man/man2/getattrlist.2) and [Apple `kauth.h` source](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/sys/kauth.h) describe the native security attributes used by the check.

## Check effective state

Run status after deployment:

```text
tadx policy status
tadx policy status --full
```

The compact status reports the fixed path, state, protection result, candidate validity, remote ceiling, and allowed and denied counts.
The full status adds the allowlisted IDs and each protection check.

| State | Meaning | TADX operational behavior |
| --- | --- | --- |
| `active` | The fixed file is protected and its strict schema is valid. | Enforce the listed IDs and the `remote_mutations` ceiling. |
| `unmanaged` | The fixed file is absent. | Apply no managed ceiling, then apply normal site consent and Tableau authorization. |
| `error` | The file or path is unreadable, insecure, malformed, or unavailable. | Block operational execution and expose recovery tools and help. |

`protected` is `true` only after the secure path and file checks pass.
`candidate_valid` is `true` only for an active policy.
A protected file with invalid JSON reports a protected path and `candidate_valid: false`, but its state remains `error`.

The `checks` array identifies each inspected path and reports its check kind, pass result, and failure reason.
Use those reasons to repair ownership, modes, ACLs, links, or file type.
Run status again after every repair.

## Update or remove a policy

To update a policy, validate a new candidate, preserve the current policy, deploy the replacement with protected ownership, and run status.
The policy is read again at each command and worker boundary.
An update does not require an enrollment step or a TADX restart.

Do not edit the active JSON in place while TADX commands are running.
Use an administrator-controlled temporary file, protect it, and replace the active regular file as one deployment operation.
Keep a protected backup until `tadx policy status --full` confirms the intended IDs and protection checks.

If deployment fails, restore the prior protected file before retrying an operation.
An absent file produces `unmanaged`.
An unreadable, malformed, or insecure replacement produces `error` and blocks operations.

To remove the policy, an administrator deletes the fixed file and optionally removes its empty TADX policy directory.
The next status reports `unmanaged`.
Removal does not change saved site consent, Tableau permissions, credentials, or other clients.

## Recover from failures

These commands remain available when policy enforcement blocks other operations:

```text
tadx --help
tadx policy --help
tadx policy samples --output <directory>
tadx policy validate <candidate-file>
tadx policy status --full
```

Recovery commands do not install, activate, or remove the managed policy.
`policy samples` creates candidates only in the requested output directory.
`policy validate` checks candidate content only.
`policy status` reads only the fixed system path.

If an active policy denies a capability, inspect the canonical ID in the capability output and compare it with `allowed_capabilities`.
If an operation has an administrative preflight, that preflight can require its own canonical ID.
A custom policy can therefore allow a write while denying an inspect needed to prepare or verify it.
The administrator template includes all current IDs and avoids that class of omission.
Reuse known canonical IDs from capability output or help instead of running broad discovery only to identify a policy prerequisite.

Administrative IDs cover user, group, membership, permission, and shared label definition operations.
Project operations, content ownership operations, and job cancellation remain non-administrative IDs.
The `read-only` and `read-write-no-admin` templates exclude administrative IDs.

## References

Read the [TADX command reference](command-structure.md) for help conventions and the [getting started guide](getting-started.md) for site consent.
Read the [capability reference](reference/capabilities.md) for the generated current operation inventory.
