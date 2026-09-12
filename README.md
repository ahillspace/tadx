# TADX

TADX is a CLI authored specifically to work well with agents, but is also useful to operate by hand.
The main goal is to allow agents to work with both Tableau Cloud and Server with reduced confusion, token usage, and vastly increased speed.

Initial testing has shown drastic improvement over all other methods of equiping agents with the tools it needs to work with Tableau.

TADX handles Tableau lifecycle work, not natural-language data queries or analytical rendering.

Feedback and collaboration is openly welcomed and there is specific and deliberate documentation for anyone looking to add actions to TADX using coding agents (see [tadx-build skill](.agents/skills/tadx-build/SKILL.md))

## Current status

TADX is an independent, pre-1.0 project and is not supported by or associated with Tableau or Salesforce.
It is usable and actively developed.

The current release supports named Tableau environments, PAT authentication, content discovery, local workspaces and caches, workbook, datasource, flow, and project lifecycle operations, bounded lineage, Tableau administration, and initial Pulse definition and metric workflows.
Some capabilities are intentionally deferred where the upstream contract is not yet supported or proven.
Current source builds also support upstream database, table, and column metadata inspection and enrichment, scoped metadata audits, and supported content labels.
`catalog` means upstream Tableau metadata; `cache` is TADX's optional local inventory store.
Check the exact capabilities in your installed version with:

```text
tadx capability list
```

## Put it to work

Give your agent an outcome, not a list of API calls:

- Find sales workbooks, inspect their dependencies, and download a useful working set.
- Prepare a project and access for a temporary analyst.
- Discover suitable datasource fields and create a meaningful Pulse metric.
- Find missing descriptions in an upstream table and enrich the metadata without changing the underlying data.

Or use the same CLI directly to inspect, download, organize, and publish Tableau content.

## Install TADX and agent Guidance

The next release installs the CLI and its bundled skills together, detects your agent installations, and refreshes TADX-owned skills on every installation or update.
The prepared public entry points are:

```powershell
irm https://tadx.net/install.ps1 | iex
```

```sh
curl -fsSL https://tadx.net/install.sh | sh
```

These URLs require the website and release launch described in [Website publishing](docs/website.md); they are not the private-repository installation path yet.
Installers never change Tableau credentials or mutation policy.

### Install while the repository is private

The repository is currently private.
Install [GitHub CLI](https://cli.github.com/) and authenticate an account with repository access before continuing:

```text
gh auth login
gh auth status
```

You do not need to clone the repository.
The installers select the Windows, macOS, or Linux binary for your machine, verify its SHA-256 checksum, and install without administrator access.

### Windows

Download the PowerShell installer into a new temporary directory, inspect it, and run it:

```powershell
$installerDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $installerDir | Out-Null
gh release download --repo ahillspace/tadx --pattern install.ps1 --dir $installerDir
$installer = Join-Path $installerDir 'install.ps1'
Get-Content $installer
& $installer
Remove-Item -LiteralPath $installerDir -Recurse
```

### macOS and Linux

Download the shell installer into a new temporary directory, inspect it, and run it:

```sh
installer_dir="$(mktemp -d)"
gh release download --repo ahillspace/tadx --pattern install.sh --dir "$installer_dir"
cat "$installer_dir/install.sh"
sh "$installer_dir/install.sh"
rm -rf "$installer_dir"
```

The release assets are built from [`scripts/install.ps1`](scripts/install.ps1) and [`scripts/install.sh`](scripts/install.sh).
Open a new terminal if `tadx` is not immediately available, then confirm the installed release:

```text
tadx version
```

Release installers install the published release, not unreleased commits on `main`.
The combined installation, overview, and updater described here require the next release containing these changes.
For a source build, follow [CONTRIBUTING.md](CONTRIBUTING.md).

Once running that release, check or apply updates with:

```text
tadx update --check
tadx update
```

Updates refresh the CLI and bundled Guidance together, even if the binary version is unchanged.
Running the installer again also upgrades or repairs the installation.

## Connect to Tableau

Run `tadx` for a compact overview of environments, credential configuration, workspaces, and mutation policy.
This overview is local and read-only; it does not authenticate against Tableau.
Run `tadx --help` for the command index.

Create an environment profile for a Tableau Cloud or Tableau Server site:

```text
tadx env add dev --url https://example.tableau.com --site example-site
```

Use the server base URL for `--url` and the site's content URL slug for `--site`.
For Tableau's default site, omit `--site`.

TADX authenticates with Tableau personal access tokens only.
Create a PAT in Tableau, then run the interactive login:

```text
tadx auth login --environment dev
```

Enter the PAT name and secret at the secure prompts.
TADX validates the PAT before saving it, and the secret does not echo.
The credential is stored only in the native OS credential store: Windows Credential Manager, macOS Keychain, or Linux Secret Service.
If that store is unavailable or locked, login fails instead of falling back to plaintext.

For CI or temporary use, environment profiles can instead reference a PAT name variable and a PAT secret variable.
See `tadx env add --help` for those options.

## Agent Guidance

The combined installer detects supported agent directories and installs the same `tadx` and `tadx-pulse` packages for each.
If none are detected, it installs shared skills under `~/.agents/skills`.
To add another agent later or explicitly choose a target:

```text
tadx agent install --target claude
tadx agent install --target codex
tadx agent install --target cursor
```

Guidance teaches the selected agent how to use TADX safely and installs standard `SKILL.md` packages in that agent's skill directory.
Use `tadx agent install --target auto` to refresh all detected targets manually.
Use `--preview` first if you want to inspect the local file changes.
TADX owns these package directories and replaces them during upgrades, including local edits, without requiring `--force`.
Put personal additions in a separate skill; unrelated skills are preserved.

Supported targets also include OpenCode, Pi, Hermes, GitHub Copilot, Gemini CLI, Cline, and the shared `generic` location:

```text
tadx agent install --target opencode
tadx agent install --target pi
tadx agent install --target hermes
tadx agent install --target copilot
tadx agent install --target gemini
tadx agent install --target cline
```

Cloning the repository includes both the [TADX skill](internal/agent/skills/tadx/SKILL.md) and the [Pulse authoring skill](internal/agent/skills/tadx-pulse/SKILL.md), with their reference files.
These are the same packages embedded in the CLI, not separate copies.
Install them for your agent; cloning alone does not make the embedded source directory discoverable to your agent.
See [Guidance locations and the startup notice](docs/getting-started.md#agent-guidance) for current source-build behavior.

## Get your first workbook

Create a named workspace and make it the default for the environment:

```text
tadx workspace create development
tadx env update dev --default-workspace development
```

Search Tableau for the workbook you want:

```text
tadx search revenue --environment dev --type workbook
```

Choose the result you intend to use and copy its authoritative LUID.
Replace `WORKBOOK_LUID` below with that returned ID, then pull the workbook into the default workspace:

```text
tadx content workbook pull --environment dev --id WORKBOOK_LUID
```

Find the downloaded files and inspect their local state:

```text
tadx workspace status --workspace development --full
```

TADX writes managed artifacts under the workspace while keeping paths portable across operating systems.
By default, the workspace is created under `<home>/TADX/workspaces/development`.
The full status output reports the exact machine-local file locations.

## Safe defaults

Read operations query Tableau live unless you explicitly select the local cache.
Compact TOON output is the default; `--full` adds bounded detail for the same operation.
Tableau LUIDs are authoritative, and ambiguous selectors fail instead of guessing.

Remote mutations are disabled by default.
Supported read-only previews remain available while mutations are disabled.
Enabling remote mutations is an optional, explicit opt-in that is separate from permission to perform a particular Tableau operation.
Agents must ask before changing the mutation setting or its scope.
See `tadx mutation status` and `tadx mutation set --help` when you are ready to configure that policy.

## Learn more

- [`docs/getting-started.md`](docs/getting-started.md) covers credentials, other artifact types, caches, previews, Pulse discovery, and maintenance.
- [`docs/workspaces.md`](docs/workspaces.md) explains workspace identity, paths, status, cloning, and local artifact operations.
- [Catalog metadata Guidance](internal/agent/skills/tadx/references/catalog.md) covers source-build metadata inspection, descriptions, tags, audits, and labels.
- [`docs/reference/capabilities.md`](docs/reference/capabilities.md) documents the capability registry generated from the current source tree.
- [Capability map](docs/reference/capability-map.html) presents the current registry as an interactive visual inventory.
- [Architecture](docs/architecture/README.md) shows the main parts and links them to their source files.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) is the day-to-day guide for building one bounded TADX action.

This README describes the current source tree; older published releases may not include every command shown here.
See [`docs/reference/shorthand.md`](docs/reference/shorthand.md) for supported command and flag aliases.

## Future Vision

The current goal is to get TADX running quickly and smoothly against the simple content lifecycle you see with Tableau Cloud and Server.
This is to get it ready to augment the new experiences coming in Tableau (Tableau Authoring API, Tableau Knowledge Graph, Tableau MCP, TDS API, Composable Datasources, etc.).
Augmenting semantics, modifying published datasources, cleaning and composing data sources, and managing access with agents is all in scope as these new features become available and TADX is meant to act as the platform that allows agents to assist with these activites cleanly, quickly, cheaply, and at scale.
