# Managed policy

TADX can enforce an administrator-owned capability ceiling for every installed TADX process on a machine.
The ceiling is optional, machine-wide, and separate from Tableau permissions and site mutation consent.
Policies are designed for you to customize by editing their JSON files.
The supplied templates are starting points, not fixed access levels.
You can change the permitted operations and remote-mutation setting to match your requirements.

This document describes the installed TADX implementation and its protected operating-system policy file.
It does not describe privileged agent containment or controls for other clients on the machine.

## Understand the policy boundary

TADX loads the policy from its protected system location at each command and worker boundary.
The installer can select a directory through a protected machine-wide locator on each supported platform.
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
Removing only a located policy file fails closed until the administrator repairs the installation or removes its locator.

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
Linux checks `/etc/tadx-policy-location.json`, and macOS checks `/private/etc/tadx-policy-location.json` for a protected custom-location record.
An absent Unix locator selects the platform default path for compatibility with manually installed policies.
An invalid or insecure Unix locator is an error, and installation can repair invalid locator contents.

On Windows, ownership and DACL checks cover the policy file and its immediate installation directory.
Ancestor ownership and DACLs, including the drive root, are also inspected, but unsafe or unverifiable ancestor protection produces a warning rather than blocking use.
Creating unrelated children alone is not treated as permission to replace the policy path.
Ancestor path integrity remains mandatory, and ancestors remain open against replacement while the policy is read or installed.
Those handles do not prevent substitution between commands.
Someone with replacement rights above the installation directory can move the protected directory aside and substitute another valid administrator-owned policy, including an older policy that permits more operations.
The registry records a location, not the identity of the original directory or a fingerprint of approved policy contents.
Choosing a warning instead of rejection deliberately accepts this risk; use a fully protected path when this boundary matters.
TADX does not change ancestor permissions automatically.
On Linux and macOS, the parent directory and every ancestor must remain root-owned and not group- or world-writable.
The policy file must be a regular file with exactly one hard link.
Symbolic links, Windows reparse points, and invalid path types cause a policy error.

## Create a candidate

Start with a template, then edit its JSON before deployment.
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
The output reports the selected system path when known and platform deployment guidance.
An invalid or unreadable locator does not prevent candidate generation.
When the location is unknown, `system_path` is empty and the instructions direct an administrator to inspect status and repair the locator rather than presenting the default as the active location.
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

### Install on Windows

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
TADX accepts an owner of LocalSystem, built-in Administrators, or TrustedInstaller for each checked object.
TADX rejects non-administrator effective modification rights on the file, immediate installation directory, and registry locator.
TADX also rejects reparse points and files with more than one hard link.

The [Windows known-folder API](https://learn.microsoft.com/en-us/windows/win32/api/shlobj_core/nf-shlobj_core-shgetknownfolderpath) defines the operating-system path lookup.
The [Windows security descriptor API](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-getsecurityinfo) and [file security model](https://learn.microsoft.com/en-us/windows/win32/fileio/file-security-and-access-rights) explain the owner and DACL checks.
The [ShellExecuteW documentation](https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shellexecutew) describes the `runas` elevation verb.

### Install on Linux

Run the installer from an ordinary terminal or agent session:

```sh
tadx policy install
tadx policy install --template read-only
tadx policy install --output /etc/tadx-custom --template read-write-no-admin
tadx policy status --full
```

The default policy path is `/etc/tadx/managed-policy.json`.
The default template is `superuser`, and `--template` accepts `read-only`, `read-write-no-admin`, or `superuser`.
The legacy template input `admin` remains an alias for `superuser`.
The installer uses `/usr/bin/sudo` through the controlling terminal when the process is not root.
`sudo` owns its password prompt; TADX does not capture or log the password.
The existing sudo authorization can be reused, and root processes do not prompt.
If TADX reports that no controlling terminal is available, run `sudo tadx policy install` with the same options from a terminal, or run TADX as root.
The active policy file and every ancestor must be root-owned and not group- or world-writable.
The selected directory must be dedicated, local, absolute, and protected throughout its path.
The installer can create the selected leaf directory when its parent already exists.
It rejects symlinks, hard links, unsafe ACLs, unrelated files, and directories it cannot verify.
It does not change ancestor permissions or modify unrelated files.
Omitting `--output` selects the default path and resets the active locator to that path.
Each successful installation replaces the selected policy and preserves files in earlier locations.

The Linux check uses the group mode bits as the effective POSIX ACL mask.
Removing group write permission removes effective ACL write access.
Review every ancestor with `namei -l /etc/tadx/managed-policy.json`.

The [Linux `open(2)` reference](https://man7.org/linux/man-pages/man2/open.2.html) documents no-follow path handling.
The [Linux `acl(5)` reference](https://man7.org/linux/man-pages/man5/acl.5.html) documents effective ACL permissions.

### Install on macOS

Run the installer from an ordinary terminal or agent session:

```sh
tadx policy install
tadx policy install --template read-only
tadx policy install --output "/Library/Application Support/TADX-Custom" --template read-write-no-admin
tadx policy status --full
```

The default policy path is `/Library/Application Support/TADX/managed-policy.json`.
The default template is `superuser`, and `--template` accepts `read-only`, `read-write-no-admin`, or `superuser`.
The legacy template input `admin` remains an alias for `superuser`.
The installer uses `/usr/bin/sudo` through the controlling terminal when the process is not root.
`sudo` owns its password prompt; TADX does not capture or log the password.
The existing sudo authorization can be reused, and root processes do not prompt.
If TADX reports that no controlling terminal is available, run `sudo tadx policy install` with the same options from a terminal, or run TADX as root.
The active policy file and every ancestor must be root-owned and not group- or world-writable.
The selected directory must be dedicated, local, absolute, and protected throughout its path.
The installer can create the selected leaf directory when its parent already exists.
It rejects symlinks, hard links, unsafe ACLs, unrelated files, and directories it cannot verify.
It does not change ancestor permissions or modify unrelated files.
Omitting `--output` selects the default path and resets the active locator to that path.
Each successful installation replaces the selected policy and preserves files in earlier locations.

macOS extended ACLs are inspected through native security attributes.
Read-only, deny, and inherit-only entries can pass when the installer verifies that modification remains root-only.
The installer rejects an effective modification grant it cannot prove is root-only.
Inspect ACLs with `ls -ldeO` and remove unsafe entries before installation:

```sh
sudo chmod -N "/Library/Application Support/TADX"
```

Review the path with `ls -ldeO` and check every ancestor before running TADX.
The [Apple `getattrlist(2)` source](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/man/man2/getattrlist.2) and [Apple `kauth.h` source](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/sys/kauth.h) describe the native security attributes used by the check.

## Check effective state

Run status after deployment:

```text
tadx policy status
tadx policy status --full
```

The compact status reports the selected system path, state, protection results, warnings, candidate validity, remote ceiling, and allowed and denied counts.
The full status adds the allowlisted IDs and each protection check.

| State | Meaning | TADX operational behavior |
| --- | --- | --- |
| `active` | The selected file is protected and its strict schema is valid. | Enforce the listed IDs and the `remote_mutations` ceiling. |
| `unmanaged` | The default file is absent, with no platform locator installed. | Apply no managed ceiling, then apply normal site consent and Tableau authorization. |
| `error` | A required file, directory, locator, schema, or path-integrity check fails. | Block operational execution and expose recovery tools and help. |

On Windows, `protected` describes the policy file and its immediate directory; `path_protected` also requires the ancestor protection checks to pass.
An active policy can report `protected: true`, `path_protected: false`, and ancestor warnings without becoming an error or changing its capability and mutation restrictions.
Warnings appear in compact and full status and install receipts, including partial installation receipts after a confirmed destination write; ordinary commands also report them on stderr without altering JSON stdout.
Installation warnings describe the confirmed destination, which may differ from the currently active policy location.
This diagnostic readback does not establish activation or replace the original installation error.
On Unix, ancestor protection remains a requirement, not a warning-only check.
`candidate_valid` is `true` only for an active policy.
A protected file with invalid JSON reports `candidate_valid: false`, but its state remains `error`.

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
An absent default file produces `unmanaged` only when no platform locator is installed.
An unreadable, malformed, or insecure replacement produces `error` and blocks operations.

On Linux and macOS, an administrator removes the selected policy and its locator, then optionally removes its empty TADX directory.
The default paths are `/etc/tadx/managed-policy.json` and `/Library/Application Support/TADX/managed-policy.json`.
The locator paths are `/etc/tadx-policy-location.json` and `/private/etc/tadx-policy-location.json`.
The next status reports `unmanaged` when no platform locator or default policy remains.
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

Only `policy install` installs and activates a template.
It requests UAC approval on Windows or uses root privileges through `sudo` on Linux and macOS.
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
