# TADX

TADX is a CLI authored specifically to work well with agents, but is also useful to operate by hand.
The main goal is to allow agents to work with both Tableau Cloud and Server with reduced confusion, token usage, and vastly increased speed.

Initial testing has shown drastic improvement over all other methods of equiping agents with the tools it needs to work with Tableau.

TADX handles Tableau lifecycle work, not natural-language data queries or analytical rendering.

Feedback and collaboration is openly welcomed and there is specific and deliberate documentation for anyone looking to add actions to TADX using coding agents (see tadx-build skill)

## Current status

TADX is an independent, pre-1.0 project and is not supported by or associated with Tableau or Salesforce.
It is usable and actively developed.

The current release supports named Tableau environments, PAT authentication, content discovery, local workspaces and catalogs, workbook, datasource, flow, and project lifecycle operations, bounded lineage, Tableau administration, and initial Pulse definition and metric workflows.
Some capabilities are intentionally deferred where the upstream contract is not yet supported or proven.
Check the exact capabilities in your installed version with:

```text
tadx capability list
```

## Put it to work

Give your agent an outcome, not a list of API calls:

- Find sales workbooks, inspect their dependencies, and download a useful working set.
- Prepare a project and access for a temporary analyst.
- Discover suitable datasource fields and create a meaningful Pulse metric.

Or use the same CLI directly to inspect, download, organize, and publish Tableau content.

## Install the CLI

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

The canonical commands below work with the latest release, v0.1.3.
Running the installer again upgrades or repairs the CLI.
Installing or upgrading the CLI does not automatically upgrade agent Guidance that you previously installed.

## Connect to Tableau

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

## Install agent Guidance

The CLI and agent Guidance are separate installations.
After installing the CLI, choose the one command for your coding agent:

```text
tadx agent install --target claude
tadx agent install --target codex
tadx agent install --target cursor
```

Guidance teaches the selected agent how to use TADX safely and installs standard `SKILL.md` packages in that agent's skill directory.
Run the command again after a CLI upgrade to update installed Guidance.
Use `--preview` first if you want to inspect the local file changes.

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

Read operations query Tableau live unless you explicitly select the local catalog.
Compact TOON output is the default; `--full` adds bounded detail for the same operation.
Tableau LUIDs are authoritative, and ambiguous selectors fail instead of guessing.

Remote mutations are disabled by default.
Supported read-only previews remain available while mutations are disabled.
Enabling remote mutations is an optional, explicit opt-in that is separate from permission to perform a particular Tableau operation.
Agents must ask before changing the mutation setting or its scope.
See `tadx mutation status` and `tadx mutation set --help` when you are ready to configure that policy.

## Learn more

- [`docs/getting-started.md`](docs/getting-started.md) covers credentials, other artifact types, catalogs, previews, Pulse discovery, and maintenance.
- [`docs/workspaces.md`](docs/workspaces.md) explains workspace identity, paths, status, cloning, and local artifact operations.
- [`docs/reference/capabilities.md`](docs/reference/capabilities.md) documents the capability registry generated from the current source tree.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) is the day-to-day guide for building one bounded TADX action.

The source tree includes CLI shorthand planned for the next release, but v0.1.3 accepts the canonical commands used in this README.
See [`docs/reference/shorthand.md`](docs/reference/shorthand.md) only when using a build that includes that feature.

## Future Vision

The current goal is to get TADX running quickly and smoothly against the simple content lifecycle you see with Tableau Cloud and Server. This is to get it ready to augment the new experiences coming in Tableau (Tableau Authoring API, Tableau Knowledge Graph, Tableau MCP, TDS API, Composable Datasources, etc.). Augmenting semantics, modifying published datasources, cleaning and composing data sources, and managing access with agents is all in scope as these new features become available and TADX is meant to act as the platform that allows agents to assist with these activites cleanly, quickly, cheaply, and at scale.
