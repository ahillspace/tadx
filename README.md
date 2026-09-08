# TADX

TADX is a CLI authored specifically to work well with agents, but is also useful to operate by hand.
The main goal is to allow agents to work with both Tableau Cloud and Server with reduced confusion, token usage, and vastly increased speed.

Initial testing has shown drastic improvement over all other methods of equiping agents with the tools it needs to work with Tableau.

For agentic analytics, TADX defers to the official Tableau MCP and does not attempt to recreate any of the tooling necessary for NLQ.

Feedback and collaboration is openly welcomed and there is specific and deliberate documentation for anyone looking to add actions to TADX using coding agents (see tadx-build skill)

## Current status

TADX is not supported or associated with Tableau or Salesforce directly.

TADX is usable and actively developed.

The executable registry now covers the core content, workspace, catalog, administration, and initial Pulse workflows planned for V1.

The current build supports:

- Managing named environment profiles that reference PAT environment variables or an optional native OS credential.
- Checking local authentication configuration, storing a validated PAT, removing a stored PAT, and signing in to verify a Tableau site.
- Discovering capability ownership, availability, selectors, safety rules, and blockers.
- Querying Tableau live by default, with explicit local catalog reads through `--catalog`.
- Refreshing and inspecting the status of the local SQLite catalog.
- Searching workbooks, published datasources, flows, and projects through Tableau native search, while administration and Pulse use their dedicated APIs.
- Creating, registering, cloning, listing, inspecting, and moving named workspaces, plus deleting one local artifact safely.
- Listing, inspecting, creating, and updating projects.
- Listing, inspecting, and pulling flows, plus publishing, moves, deletions, and optional previews.
- Listing and inspecting workbooks, plus pulls, publishing, deletions, and optional previews.
- Listing and inspecting published datasources and bounded field metadata, plus pulls, publishing, deletions, and optional previews.
- Capturing bounded lineage for workbooks, published datasources, and flows.
- Listing, inspecting, pulling, creating, and deleting Pulse definitions.
- Listing, inspecting, forking, and deleting Pulse metrics, plus managing exact user and group followers.
- Managing Tableau site users, groups, memberships, and permission inspection.

Remaining work focuses on release hardening and capabilities that still lack a supported or proven upstream contract.
Datasource composition, datasource field-description updates, project pull and publish, and Pulse updates are deferred.

Use the registry in your installed build as the source of truth:

```text
tadx capability list
tadx capability get workbook.pull --full
```

## Future Vision

The current goal is to get TADX running quickly and smoothly against the simple content lifecycle you see with Tableau Cloud and Server. This is to get it ready to augment the new experiences coming in Tableau (Tableau Authoring API, Tableau Knowledge Graph, Tableau MCP, TDS API, Composable Datasources, etc.). Augmenting semantics, modifying published datasources, cleaning and composing data sources, and managing access with agents is all in scope as these new features become available and TADX is meant to act as the platform that allows agents to assist with these activites cleanly, quickly, cheaply, and at scale.

## Install TADX

Install or upgrade the latest release from a terminal.
The installers select the correct Windows, macOS, or Linux binary, verify its SHA-256 checksum, and install it without administrator access.
Anonymous installation requires public access to the repository and its release assets.
For private access, authenticate GitHub CLI before downloading the installer:

```text
gh auth login
```

### Windows

Download the PowerShell installer, inspect it, and run it:

```powershell
$installer = Join-Path $env:TEMP 'tadx-install.ps1'
Invoke-WebRequest 'https://github.com/ahillspace/tadx/releases/latest/download/install.ps1' -OutFile $installer
Get-Content $installer
& $installer
Remove-Item $installer
```

For a private repository, replace the `Invoke-WebRequest` command with:

```powershell
gh release download --repo ahillspace/tadx --pattern install.ps1 --output $installer
```

### macOS and Linux

Download the shell installer, inspect it, and run it:

```shell
installer="$(mktemp)"
curl -fsSL https://github.com/ahillspace/tadx/releases/latest/download/install.sh -o "$installer"
cat "$installer"
sh "$installer"
rm -f "$installer"
```

For a private repository, replace the `curl` command with:

```shell
gh release download --repo ahillspace/tadx --pattern install.sh --output "$installer"
```

Open a new terminal if `tadx` is not immediately available, then verify the installation:

```text
tadx version
tadx --help
```

Pass an explicit release with `-Version v1.2.3` on Windows or `--version v1.2.3` on macOS and Linux.
Running the installer again upgrades or repairs the installed binary.

To remove only the CLI binary and its installer-managed `PATH` entry, run one of these commands:

```powershell
$installer = Join-Path $env:TEMP 'tadx-install.ps1'
Invoke-WebRequest 'https://github.com/ahillspace/tadx/releases/latest/download/install.ps1' -OutFile $installer
& $installer -Action Uninstall
Remove-Item $installer
```

```shell
installer="$(mktemp)"
curl -fsSL https://github.com/ahillspace/tadx/releases/latest/download/install.sh -o "$installer"
sh "$installer" uninstall
rm -f "$installer"
```

CLI removal preserves TADX configuration, workspaces, catalogs, Guidance, and OS-stored credentials.
Before removing the CLI, use `tadx auth logout` and `tadx agent uninstall` for any state that you also want removed.

### Install from source

Install Git and Go 1.26 or later, clone the repository, and run:

```shell
go install github.com/ahillspace/tadx/cmd/tadx@latest
```

The Go binary directory must be present in `PATH`.

## Install agent Guidance

Choose the command for your coding agent:

```text
tadx agent install --target claude
tadx agent install --target codex
tadx agent install --target cursor
```

Each command installs the root TADX Guidance and the complete Pulse authoring Guidance as standard `SKILL.md` packages.
The root package includes optional references for content lifecycle, workspaces, administration, and Tableau MCP routing.
Claude uses `~/.claude/skills`, Codex uses `~/.codex/skills`, and Cursor uses `~/.cursor/skills`.
Existing packages under `~/.agents/skills` remain untouched.
No repository checkout or Tableau credentials are required.
The installer creates no `AGENTS.md`, `CLAUDE.md`, or Cursor rules.

Use `--preview` to inspect changes without writing files and `--full` to see home-relative paths and package fingerprints.
Identical packages remain unchanged.
Replacing a divergent package requires `--force`, which preserves the previous package under the agent directory's `.tadx-skill-backups` directory.
This local operation does not require `TADX_ENABLE_MUTATIONS`.

## Configure a Tableau environment

TADX uses Tableau personal access tokens.
Register an environment profile with environment-variable names for CI or temporary credential overrides:

```text
tadx env add dev --url https://example.tableau.com --site example-site --pat-name-env TADX_DEV_PAT_NAME --pat-secret-env TADX_DEV_PAT_SECRET
```

Choose one credential source.

For an interactive setup, enter the PAT name and secret at secure terminal prompts:

```text
tadx auth login --environment dev
```

TADX validates the PAT against the configured Tableau site before storing it in the native OS credential store.
The secret does not echo in the terminal.
TADX stores only an opaque credential reference in its configuration and never falls back to plaintext storage.
The native store uses Windows Credential Manager, macOS Keychain, or Linux Secret Service.
If the native store is unavailable or locked, login fails without saving the PAT elsewhere.
The login command requires an interactive terminal and does not accept credential flags or redirected input.

For CI or a temporary override, set both configured environment variables before running TADX.
A complete environment-variable pair takes precedence over the stored PAT.
If either variable is missing or empty, TADX reports an incomplete credential instead of combining credential sources.

Inspect the resolved nonsecret configuration, then verify the active credentials against Tableau:

```text
tadx auth status --environment dev
tadx auth check --environment dev
```

The stored PAT remains available until you remove it locally or Tableau rejects it because it was revoked, expired, or disabled.
To replace the stored PAT, run `tadx auth login --environment dev` again and enter the replacement values.
To stop using the stored PAT, run:

```text
tadx auth logout --environment dev
```

Logout removes only the local TADX credential.
It does not revoke or delete the PAT in Tableau.
Remove a temporary environment-variable override from the process to return to the stored PAT.
PATs and session tokens never appear in configuration values, output, logs, artifacts, catalogs, fixtures, or diagnostics.

## Create a named workspace

Create a unique logical workspace at the default human-accessible location:

```text
tadx workspace create development
tadx env update dev --default-workspace development
tadx workspace status --workspace development --full
```

The default root is `<home>/TADX/workspaces/development` on Windows, macOS, and Linux.
Use `--path <workspace-root>` with `workspace create` or `workspace clone` to override the default.
The `--full` output for workspace creation, cloning, listing, and status includes the registered machine-local root.
Workspace registration still requires an explicit root path.
Artifact moves still require explicit source and destination workspace names plus an artifact selector.
Workspace names are portable across supported operating systems and cannot contain path separators, Windows-invalid characters, reserved device names, or trailing dots or spaces.
Lifecycle commands use the logical workspace name, while artifact paths remain relative and portable.

## Pull a workbook

Pull one workbook by its authoritative LUID:

```text
tadx content workbook pull --environment dev --workspace development --id <workbook-luid>
```

Add `--include-pds` to acquire direct published datasource dependencies as sibling artifacts.
Add `--full` to the same command when you need expanded, bounded details.

## Choose live or catalog reads

Supported read commands query Tableau by default.
Pass `--catalog` to read only from the local SQLite catalog without authenticating or contacting Tableau:

```text
tadx content workbook list --environment dev
tadx content workbook list --environment dev --catalog
```

TADX reports the selected source, freshness, and coverage in the same output shape.
Catalog reads never fall back to Tableau.

An unfiltered workbook, datasource, flow, project, user, or group `list` performs a complete live inventory of that resource scope and atomically refreshes the scope in the catalog.
`--limit` bounds the rows rendered to the terminal, not the live inventory work.
Use the returned cursor to read the same catalog snapshot without repeating the remote traversal.
Adding a resource filter changes the operation to a bounded live query and records only the observed rows as a partial cache update.

Live searches with content terms use Tableau's native content search.
Administration and Pulse searches use their dedicated APIs, and a broad search returns native content before their results.
Use `--catalog` when local freshness is sufficient and no Tableau request should occur.

Refresh the complete local inventory when you need broad offline search:

```text
tadx catalog refresh --environment dev
tadx search revenue --environment dev --catalog
tadx catalog status --environment dev
```

## TADX and Tableau MCP

TADX owns Tableau development and lifecycle work, including content artifacts, workspaces, administration, and configuration lifecycle.
Tableau MCP owns analytical work, including datasource queries, view and custom-view results, and Pulse values and insights.
Use `get-datasource-metadata` before `query-datasource` for published datasource questions.
Use Tableau MCP view tools for data or images, and use its Pulse tools for current values, insight bundles, and briefs.
The user and host agent own Tableau MCP configuration and connection selection.
TADX does not configure, select, call, proxy, or report the connection state of Tableau MCP.

## Contribute

Action builders should start with [CONTRIBUTING.md](CONTRIBUTING.md).
