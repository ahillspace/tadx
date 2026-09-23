# Managed policy

TADX can enforce an administrator-owned capability ceiling for every installed TADX process on a machine.
The ceiling is optional, machine-wide, and separate from Tableau permissions and site mutation consent.

This document describes the installed TADX implementation and its protected operating-system policy file.
It does not describe privileged agent containment or controls for other clients on the machine.

## Understand the policy boundary

TADX loads the policy from its protected system location at each command and worker boundary.
On Windows, administrator installation can select a directory through a protected machine-wide locator.
Ordinary commands cannot override that location with a flag, environment variable, user configuration value, or current directory.
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

All three standard templates include every current read capability, including administrative inventory and local operations.
The `read-only` template blocks every Tableau mutation through `remote_mutations: false`.
The `read-write-no-admin` template excludes administrative remote mutations while allowing the other current operations.
Custom policies can still deny reads by omitting their capability IDs.
Content label updates and deletes can require `admin.label.value.inspect` as a preflight, including for `--preview`.

An administrator can remove the policy manually using the platform-specific removal instructions below.
On Windows, removing only a located policy file fails closed until the administrator repairs the installation or removes its locator.

## Find the policy path

TADX resolves the path at runtime as follows:

| Platform | Default path |
| --- | --- |
| Windows | The native operating-system `Program Files` directory, followed by `TADX\managed-policy.json`. |
| macOS | `/Library/Application Support/TADX/managed-policy.json`. |
| Linux | `/etc/tadx/managed-policy.json`. |

Windows resolves the native `Program Files` directory through the Windows known-folder API.
The implementation checks the native 64-bit location first, then the platform fallback.
Do not substitute `%ProgramFiles%`, a user directory, or a process-specific path.

Windows also checks the native 64-bit registry key `HKEY_LOCAL_MACHINE\SOFTWARE\TADX\ManagedPolicy`.
Its protected `Directory` value is a `REG_SZ` containing the selected absolute directory, with no policy JSON or credentials in the registry.
Only an absent key uses the default path for compatibility with manually installed policies.
A present but unreadable, insecure, malformed, or incomplete locator is an error.
A locator pointing to a missing policy is also an error, including when it selects the default directory.

On Windows, ownership and DACL checks cover the policy file and its immediate installation directory.
Ancestor ownership and DACLs, including the drive root, do not produce failures or warnings.
Ancestor path integrity is still checked, and ancestors remain open against replacement while the policy is read or installed.
On Linux and macOS, the parent directory and every ancestor remain part of the ownership and permissions boundary.
The policy file must be a regular file with exactly one hard link.
Symbolic links, Windows reparse points, and invalid path types cause a policy error.

## Create a candidate

Use the recovery command to create three editable candidates in a user-selected directory:

```text
tadx policy samples --output ./tadx-policy-candidates
```

The command creates these files:

- `read-only.json` includes every current capability ID and sets `remote_mutations` to `false`, blocking Tableau mutations while allowing reads, previews, and local operations.
- `read-write-no-admin.json` includes every current read and local capability plus non-administrative remote mutations, and sets `remote_mutations` to `true`.
- `superuser.json` includes every current capability ID and sets `remote_mutations` to `true`.

The command creates new files only.
It never overwrites an existing candidate, installs a policy, activates a policy, or changes file protection.
The output reports the fixed system path and platform deployment guidance.
No elevation is required to create candidates in a user-owned directory.

Templates are snapshots of the current capability registry.
They are not roles, and a new capability remains denied until an administrator updates the allowlist.
The read-only template includes remote mutation IDs, so allowed previews remain available while remote writes remain denied.
It permits current local operations such as downloads, cache refreshes, workspace changes, and configuration changes.

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

An administrator deploys a policy to the protected system location.
Protect the file and its TADX directory before checking status; Unix platforms also require protected ancestors.
Keep the prior policy in a protected backup outside the fixed path until the replacement is confirmed.

### Deploy on Windows

Run the installer from an ordinary terminal or agent session:

```text
tadx policy install
tadx policy install --template read-only
tadx policy install --output C:\TADX-Policy --template read-write-no-admin
tadx policy status --full
```

The default template is `superuser`, which includes all current capabilities and sets `remote_mutations` to `true`.
The other templates are `read-only` and `read-write-no-admin`, with the same meanings as the candidate templates above.
The accepted legacy input `admin` is an alias for `superuser`, not a fourth template.
No template changes saved site consent, credentials, or Tableau permissions.
Omitting `--output` always selects the native `Program Files\TADX` directory, even after a custom installation.
The directory must be local and absolute, and its parent must already exist.
The installer creates the selected directory if necessary, or accepts an existing dedicated directory containing only `managed-policy.json`.
It rejects drive roots, user homes, shared system directories, unrelated contents, reparse points, and multiply linked policy files.

An unelevated invocation requests Windows UAC approval for one scoped installer child.
The calling terminal and agent remain unelevated.
Cancelling that approval before the child starts leaves the installation unchanged.
Once the child starts, the caller waits for its receipt instead of terminating an installer that may be committing changes.
If the caller or child is forcibly terminated, inspect `tadx policy status` and the selected destination before retrying.

The installer protects the file and immediate directory with Administrators ownership, full control for Administrators and SYSTEM, and read/traverse access for ordinary users.
It does not recursively reset ACLs or modify ancestors.
Concurrent installers serialize, stage a protected file in the destination directory, atomically replace `managed-policy.json`, verify it, then publish the protected locator.
Each successful install overwrites the selected policy and changes the active location.
Files in previous installation directories remain untouched.
The receipt records the failure phase and confirmed protection, policy-file, and locator changes.
A locator failure after installing a different destination normally leaves the prior location active and the new protected file inactive.
Replacing an already selected policy takes effect before locator publication, so a later failure does not imply rollback.
An incomplete locator created during publication fails closed and can be repaired by rerunning the installer.
`policy install` remains available when the current policy denies operations or has errors.
On Linux and macOS, the command reports that installation is unsupported; use the deployment instructions below.

TADX accepts an owner of LocalSystem, built-in Administrators, or TrustedInstaller for each checked object.
TADX rejects non-administrator effective modification rights on the file, immediate installation directory, and registry locator.
TADX also rejects reparse points and files with more than one hard link.

The [Windows known-folder API](https://learn.microsoft.com/en-us/windows/win32/api/shlobj_core/nf-shlobj_core-shgetknownfolderpath) defines the operating-system path lookup.
The [Windows security descriptor API](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-getsecurityinfo) and [file security model](https://learn.microsoft.com/en-us/windows/win32/fileio/file-security-and-access-rights) explain the owner and DACL checks.
The [ShellExecuteW documentation](https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shellexecutew) describes the `runas` elevation verb.

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

The compact status reports the selected system path, state, protection result, candidate validity, remote ceiling, and allowed and denied counts.
The full status adds the allowlisted IDs and each protection check.

| State | Meaning | TADX operational behavior |
| --- | --- | --- |
| `active` | The selected file is protected and its strict schema is valid. | Enforce the listed IDs and the `remote_mutations` ceiling. |
| `unmanaged` | The default file is absent, with no Windows locator installed. | Apply no managed ceiling, then apply normal site consent and Tableau authorization. |
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
Templates are snapshots of capability IDs in the CLI that creates them.
To refresh a standard template after the capability registry changes, explicitly install that template again with the current CLI.
This does not update the active policy automatically.

Do not edit the active JSON in place while TADX commands are running.
Use an administrator-controlled temporary file, protect it, and replace the active regular file as one deployment operation.
Keep a protected backup until `tadx policy status --full` confirms the intended IDs and protection checks.

If deployment fails, inspect the receipt and current status before deciding which protected file or locator needs repair.
An absent default file produces `unmanaged` only when no Windows locator is installed.
An unreadable, malformed, or insecure replacement produces `error` and blocks operations.

On Linux and macOS, an administrator deletes the fixed policy file and optionally removes its empty TADX directory.
The next status reports `unmanaged` on those platforms.
Removal does not change saved site consent, Tableau permissions, credentials, or other clients.

On Windows, first run `tadx policy status --full` and identify the exact selected file.
From a 64-bit elevated PowerShell session, inspect only the dedicated locator key:

```powershell
Get-ItemProperty -LiteralPath 'Registry::HKEY_LOCAL_MACHINE\SOFTWARE\TADX\ManagedPolicy' -Name Directory
```

Remove the exact selected `managed-policy.json` file, then remove only the dedicated locator key:

```powershell
Remove-Item -LiteralPath 'C:\TADX-Policy\managed-policy.json'
Remove-Item -LiteralPath 'Registry::HKEY_LOCAL_MACHINE\SOFTWARE\TADX\ManagedPolicy'
```

Replace the example file path with the exact path confirmed by status and the locator; do not delete a directory tree or the parent `SOFTWARE\TADX` key.
If the default native `Program Files\TADX\managed-policy.json` still exists from an earlier installation, removing the locator reactivates that legacy file.
Inspect and explicitly remove that exact file too if the intent is to return to `unmanaged`.
Previous custom directories remain untouched unless the administrator separately chooses to remove their exact policy files and empty directories.
Run `tadx policy status --full` afterward to confirm the intended state.
If the locator is already absent, remove only the confirmed default policy file.
If the locator is invalid, `tadx policy install` can repair it, or an administrator can inspect and remove this dedicated key before checking the default path.

## Recover from failures

These commands remain available when policy enforcement blocks other operations:

```text
tadx --help
tadx policy --help
tadx policy samples --output <directory>
tadx policy validate <candidate-file>
tadx policy status --full
tadx policy install
```

Only `policy install` installs and activates a template, with administrator authorization on Windows.
`policy samples` creates candidates only in the requested output directory.
`policy validate` checks candidate content only.
`policy status` reads the protected selected system location.

If an active policy denies a capability, inspect the canonical ID in the capability output and compare it with `allowed_capabilities`.
If an operation has an administrative preflight, that preflight can require its own canonical ID.
A custom policy can therefore allow a write while denying an inspect needed to prepare or verify it.
The superuser template includes all current IDs and avoids that class of omission.
Reuse known canonical IDs from capability output or help instead of running broad discovery only to identify a policy prerequisite.

Administrative IDs cover user, group, membership, permission, and shared label definition operations.
Project operations, content ownership operations, and job cancellation remain non-administrative IDs.
All three standard templates include administrative read IDs.
Only `read-write-no-admin` excludes administrative remote mutation IDs.

## References

Read the [TADX command reference](command-structure.md) for help conventions and the [getting started guide](getting-started.md) for site consent.
Read the [capability reference](reference/capabilities.md) for the generated current operation inventory.
