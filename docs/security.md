# Security model

This guide describes behavior in the current repository source.
It does not establish which changes are available in a published release.

TADX is a command line client, not a sandbox for an agent or other program.
Its security boundaries depend on the operating system, local account, Tableau authorization, and the user's choices about credentials and local files.
An administrator or root user can change local protections, and a compromised host can expose process memory, files, or credentials.

## Authentication and credentials

TADX supports Tableau personal access token (PAT) authentication.
Sign-in requires an HTTPS server URL.
The interactive `tadx auth login` command asks for the PAT name and secret, validates them with Tableau, and stores the validated pair in the operating system credential store.
An agent must have the user's explicit authorization before invoking this persisting login action.
The command does not display a separate save-confirmation prompt.

The credential record contains the PAT name, PAT secret, and a fingerprint for the configured server and exact site.
TADX configuration stores an opaque credential reference, not the PAT values.
When credentials are loaded, TADX checks that the record matches the configured server and site.
A complete PAT environment-variable pair takes precedence over the stored record; TADX does not combine a partial pair with stored credentials.

The native backend depends on the operating system: Windows uses Windows Credential Manager, macOS uses the `security` generic-password service, and Linux uses Secret Service over D-Bus and its login collection.
Linux therefore requires an available Secret Service and an unlockable collection.
These integrations do not promise hardware-backed storage or protection from a compromised account or host.
`tadx auth logout` removes the locally stored TADX record; it does not revoke the PAT in Tableau.

Session tokens and credential reuse are scoped to a command's in-memory session state.
TADX redacts PATs and session tokens from its controlled configuration, ordinary output, and diagnostics.
This does not control terminal recording, process memory inspection, operating-system backups, or third-party agent behavior.

## Local data

The user configuration contains non-secret environment and site settings, opaque credential references, saved mutation consent, and registered workspace roots.
Workspaces can contain native workbook, datasource, flow, Pulse-definition, and lineage artifacts.
Inventory cache databases are stored under the configuration directory.

TADX also retains a bounded `last-result.json` beside configuration, job receipts in the user cache's `tadx/jobs` directory, and publication or download operation records in `tadx/operations`.
Operation records can include command arguments, target and configuration paths, Tableau names, IDs, metadata, and result payloads.
Raw PATs and session tokens are excluded from these records.
Job monitoring receipts can retain an opaque credential and target-derived coordination hash; it cannot authenticate to Tableau, but it is credential-derived identity data.
The remaining local data may still be sensitive.

TADX does not encrypt these local files, control operating-system backup retention, or guarantee secure deletion.
Choose workspace locations and local retention policies appropriate to the data, and protect access to the machine and user account.

## Remote mutation controls

Saved mutation consent is disabled by default and belongs to the canonical server URL and exact site content URL.
Aliases that resolve to the same server and site share that setting; another server or site has a separate setting.
Changing consent persists the selected site's setting.
An agent must obtain explicit authorization naming the exact server, site, and persisted setting before changing it.

For a remote write, TADX also checks the managed policy when installed and Tableau's own authorization.
Managed policy can restrict TADX capability IDs and disable remote mutations, but cannot grant Tableau permissions.
The commands `--preview`, discovery, and `--force` do not grant mutation consent or bypass a policy denial.
A preview remains read-only when its capability is permitted.

## Administrator managed policy

Managed policy is an optional machine-wide capability ceiling for ordinary TADX processes.
It does not contain credentials and is not a sandbox for agents, other programs, or administrator/root users.
All standard templates include current read capabilities, including administrative reads.
The `read-only` template sets `remote_mutations` to false; `read-write-no-admin` excludes administrative remote writes; `superuser` includes every current capability and is the default.
The legacy template input `admin` is an alias for `superuser`.
Templates snapshot the current capability registry, so install the desired template again from a current CLI to refresh its snapshot.

Windows installation requests UAC for a scoped installer child.
The default destination is the native `Program Files\TADX` directory on each invocation that omits `--output`; an explicit local absolute `--output` directory may be selected when its parent exists.
Installation replaces the selected policy file and leaves files in previous locations in place.
Windows checks ownership and modification rights on the policy file, immediate TADX directory, and registry locator; path integrity and reparse checks remain in force.
Unix policy installation through the CLI is unsupported; administrators deploy manually, with ownership and mode checks covering the path ancestors.

See [Managed policy](managed-policy.md) for schema, installation, status, recovery, and removal procedures.

## Network and release integrity

Requests carrying Tableau session authorization require HTTPS.
The default CLI client uses Go's standard certificate verification and environment proxy behavior; proxy configuration is outside TADX's own trust guarantee.
Embedded clients supplied by other programs can have different settings.
TADX rejects cross-origin redirects and redirects from HTTPS to a less secure scheme.

The platform installers download release assets over HTTPS and compare each asset's SHA-256 checksum with `checksums.txt` from that release.
This detects a mismatch between the downloaded asset and fetched manifest.
The inspected release workflow does not establish a separate artifact signature or an independent checksum trust root, so this check does not authenticate a release independently of GitHub, its release account, or its workflow.
The updater installs the selected release and refreshes TADX-owned Guidance with recovery behavior described in the [website and installer guide](website.md).

## Operating guidance

Use exact Tableau LUIDs for consequential targets and inspect previews before execution.
Keep PATs out of command arguments and shared logs; use secure prompts or configured environment-variable references.
Review local operation records and workspace artifacts as potentially sensitive data.
Treat managed policy as one layer alongside site consent, Tableau permissions, and operating-system access controls.
