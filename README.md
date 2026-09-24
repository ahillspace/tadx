# TADX

TADX lets coding agents and humans find, inspect, download, publish, and manage Tableau content from the terminal.
Give your agent an outcome, such as finding a workbook's dependencies or preparing a project for a new analyst, instead of walking it through Tableau API calls.
Discoverable commands, compact results, and bundled agent skills reduce API plumbing and repeated setup, while previews and explicit site permissions keep changes under your control.

TADX works with Tableau Cloud and Tableau Server.
It is an independent, pre-1.0 project and is not supported by or associated with Tableau or Salesforce.

## What you can do

- Find workbooks, datasources, and flows, inspect their dependencies, and pull a useful working set into a local workspace.
- Publish and organize Tableau content with exact identities, previews, and ordered batch outcomes where supported.
- Inspect projects, users, groups, group membership, and permissions, then make authorized administrative changes.
- Explore lineage and upstream catalog metadata, audit metadata quality, and update supported descriptions, contacts, tags, and labels.
- Discover suitable datasource fields and manage Pulse definitions, metric variants, and followers.
- Build and refresh local caches for bounded offline discovery and preserve portable workspace state.

Availability still depends on the Tableau product, version, licensed features, site configuration, and permissions of the authenticated user.
Use `tadx capability list` and `tadx capability get` to inspect TADX's supported operations and their availability notes.

## TADX and Tableau MCP

TADX and Tableau MCP solve different parts of an agent's Tableau workflow.

TADX owns lifecycle operations, artifacts, workspaces, administration, upstream metadata, configuration, and Pulse definition lifecycle.
Tableau MCP is the complementary choice for analytical data queries, rendered view data and images, and current Pulse metric values and insights.
TADX does not configure, call, or proxy Tableau MCP, so the user or host agent remains responsible for that connection.

## Install

The installers select the correct published binary, verify its checksum, add shell completion and PATH configuration, and install the bundled TADX agent Guidance in detected agent directories.
If no supported agent directory is detected, Guidance is installed in the shared `~/.agents/skills` directory.

Windows PowerShell:

```powershell
irm https://tadx.net/install.ps1 | iex
```

macOS or Linux:

```sh
curl -fsSL https://tadx.net/install.sh | sh
```

If `tadx.net` is unavailable, use the installer assets published with the latest GitHub release:

```powershell
irm https://github.com/ahillspace/tadx/releases/latest/download/install.ps1 | iex
```

```sh
curl -fsSL https://github.com/ahillspace/tadx/releases/latest/download/install.sh | sh
```

Published releases provide Windows, macOS, and Linux binaries for amd64 and arm64.
See [GitHub releases](https://github.com/ahillspace/tadx/releases/latest) for the current version and downloads.

Run `tadx update --check` to check for a newer release without changing the installation.
Run `tadx update` to update the CLI and refresh its bundled Guidance.
Released builds also show occasional update notices during interactive use; set `TADX_NO_UPDATE_NOTIFIER=1` to disable automatic checks.
Updates replace TADX-owned skill packages, so keep personal extensions in a separate skill.
See [uninstall instructions](docs/getting-started.md#uninstall-tadx) to remove the CLI or bundled Guidance.
See [installation maintenance](docs/getting-started.md#maintain-the-installation) for explicit agent targets, custom installation directories, profile controls, and updates.

## First steps

Start with the local overview and command index:

```text
tadx
tadx --help
```

The overview reports local environment, credential, workspace, and mutation-policy readiness without contacting Tableau or reading a stored PAT.
Create an environment profile for the exact Tableau Cloud or Tableau Server site, then authenticate with a Tableau personal access token.
Use the built-in references for the accepted URL, site, and credential options:

```text
tadx env add --help
tadx auth login --help
```

TADX uses PAT authentication only.
Interactive login validates the PAT before saving it, and persistence requires the native Windows Credential Manager, macOS Keychain, or Linux Secret Service.
PATs and session tokens are not written to TADX configuration, output, logs, artifacts, or caches.
Automation can instead reference PAT name and secret environment variables from an environment profile.

Once connected, search for content and inspect a result.
Replace `ENVIRONMENT_ALIAS` with the profile you created and `WORKBOOK_LUID` with an ID returned by search before running these examples:

```text
tadx search revenue --environment ENVIRONMENT_ALIAS --type workbook
tadx content workbook inspect --environment ENVIRONMENT_ALIAS --id WORKBOOK_LUID
```

For the inputs and examples for a particular action, go directly to its verb help:

```text
tadx content workbook pull --help
```

Read [Getting started](docs/getting-started.md) for authentication checks, workspaces, caches, previews, Pulse authoring, and maintenance.

## Agent Guidance

The installer and `tadx update` automatically deploy the bundled `tadx` and `tadx-pulse` Guidance for detected agents.
The Guidance teaches agents how to discover commands, respect safety boundaries, and apply Tableau-specific judgment without duplicating the CLI reference.
It does not create agent instruction files, configure Tableau MCP, grant mutation permission, or prove that an agent host has loaded the skills.

The installer currently recognizes Claude, Codex, Cursor, OpenCode, Pi, Hermes, GitHub Copilot, Gemini CLI, and Cline default skill locations.
No separate skill-install step is needed for a normal installation.
See [Agent Guidance](docs/getting-started.md#agent-guidance) to add a newly installed agent, select an explicit target, or inspect installation locations and ownership rules.

## Safety and output

TADX defaults to compact TOON output for agent efficiency.
Use `--full` for expanded bounded detail and `--json` when a JSON encoding is required.
Tableau LUIDs are authoritative, and ambiguous selectors fail instead of guessing.

Remote mutations start disabled for each Tableau site.
Writes require saved consent for the exact server and site, any applicable managed-policy permission, and Tableau authorization.
Agents must ask before changing the saved site setting; `--preview` and `--force` do not grant permission.
Use `tadx mutation status` to inspect consent and [preview supported changes](docs/getting-started.md#preview-remote-changes) before applying them.

Supported mutation commands expose `--preview` so you can resolve targets and inspect proposed changes without applying them.
Acquisition and local-state operations also provide previews where they can write files or configuration.
Optional [managed policies](docs/managed-policy.md) let administrators restrict TADX operations through an OS-protected, machine-wide policy.
On Unix, the policy file and every ancestor must be root-owned and not group- or world-writable.
On Windows, unsafe ancestor permissions produce a warning rather than blocking use; such a path can permit policy substitution despite a protected policy file and containing folder.
They do not sandbox an agent or stop a process that already has administrator/root authority from changing the policy.
Read the [security guide](docs/security.md) for credential storage, mutation controls, local data, policy installation, and the limits of these protections.

## Documentation

- [Getting started](docs/getting-started.md) covers setup, credentials, content, caches, previews, Pulse, and updates.
- [Capability reference](docs/reference/capabilities.md) and the [interactive capability map](docs/reference/capability-map.html) describe the supported action inventory.
- [Workspaces](docs/workspaces.md) covers local artifacts, paths, status, and cloning.
- [Security](docs/security.md) explains authentication, credential storage, permission boundaries, and local data handling.
- [Managed policies](docs/managed-policy.md) covers optional machine-wide controls, secure deployment, and recovery.
- [Command shorthand](docs/reference/shorthand.md) lists supported aliases.
- [Architecture](docs/architecture/README.md) explains the internal boundaries and design.

Contributions and feedback are welcome.
Start with [CONTRIBUTING.md](CONTRIBUTING.md), which points coding agents and human contributors to the maintained build standards.

## License

TADX is licensed under [Apache 2.0](LICENSE).
See [third-party notices](THIRD_PARTY_NOTICES.md) for bundled dependencies.
