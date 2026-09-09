# TADX

TADX is a CLI authored specifically to work well with agents, but is also useful to operate by hand.
The main goal is to allow agents to work with both Tableau Cloud and Server with reduced confusion, token usage, and vastly increased speed.

Initial testing has shown drastic improvement over all other methods of equiping agents with the tools it needs to work with Tableau.

TADX handles Tableau lifecycle work, not natural-language data queries or analytical rendering.

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

Pulse creation accepts raw field IDs and unique display names from datasource metadata.
TADX resolves displayed field names to raw IDs before previewing or publishing, so a field caption such as `Regional Manager` can map to its underlying `People` field automatically.
Exact raw IDs take precedence, and ambiguous names fail before a remote write.
This does not translate aliases for categorical member values.

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
The root package includes optional references for content lifecycle, workspaces, catalogs, and administration.
Guidance applies when using TADX; it does not override your choice of other tools.
Claude uses `~/.claude/skills`, Codex uses `~/.codex/skills`, and Cursor uses `~/.cursor/skills`.
Existing packages under `~/.agents/skills` remain untouched.
No repository checkout or Tableau credentials are required.
The installer creates no `AGENTS.md`, `CLAUDE.md`, or Cursor rules.

Use `--preview` to inspect changes without writing files and `--full` to see home-relative paths and package fingerprints.
Identical packages remain unchanged.
Replacing a divergent package requires `--force`, which preserves the previous package under the agent directory's `.tadx-skill-backups` directory.
This local operation does not require `TADX_ENABLE_MUTATIONS`.

## Remote mutation permission

TADX reads `TADX_ENABLE_MUTATIONS` from its process environment; only the value `1` enables remote mutation execution.
TADX has no persistent mutation-toggle command.
A shell setting applies to that shell and its child processes; operating-system environment settings or shell startup files can affect future shells.
Supported read-only `--preview` operations work while the gate is off and do not authorize execution.

Agents must obtain explicit user permission before changing this flag through any mechanism, including enabling, disabling, or unsetting it.
Permission for a Tableau operation is separate from permission to change the flag.
An approval covers only its explicitly stated setting change and scope; session approval does not authorize a persistent change.
For example: "May I enable remote mutations for this session, allowing TADX to create, change, or delete Tableau resources?"
A request for persistent approval must identify its scope and effect on future shells.

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

Ordinary live `list` commands fetch a bounded result and do not require the catalog to answer.
Use `--limit` for a larger bounded result or `--all` for the full supported resource scope.
`--all` shares the catalog refresh collector and returns the live results directly, with a best-effort catalog update.
A cache-write failure is a warning, not a failed live list.
Filtered results never establish complete coverage of an unfiltered scope.
Output reports `more_available` instead of exposing opaque cursors.

Live searches with content terms use Tableau's native content search.
Administration and Pulse searches use their dedicated APIs, and a broad search returns native content before their results.
Use `--catalog` when local freshness is sufficient and no Tableau request should occur.

Refresh the local inventory when you need broad offline search:

```text
tadx catalog refresh --environment dev
tadx search revenue --environment dev --catalog
tadx catalog status --environment dev
```

Permissions are excluded from the default refresh because they require additional per-resource requests.
Include the `permissions` scope explicitly only when needed, requesting all desired scopes together.
An explicit refresh failure preserves the previous catalog generation and reports an error.
Refresh collects into private disk-backed staging before opening the active catalog's publication transaction.
Tableau response times and retries therefore do not hold the shared catalog write lock.
Staging enforces row and database-page limits; SQLite journal and temporary files add disk overhead.
Publication replaces only the requested inventory kinds and preserves independent datasource schema, Pulse, and unrequested inventory observations.
Preserved observations retain their original timestamps and freshness; refreshing inventory does not reverify them.
After a catalog schema upgrade, run an explicit refresh to rebuild the disposable cache; workspaces, artifacts, and credentials are not removed.

Catalog collection uses concurrent reads for speed, starting at up to four requests and adapting to a default ceiling of 32 per CLI process.
Tableau throttling responses reduce concurrency and pause all workers in that collection for the supplied retry delay.
This is not a server-wide traffic limit: concurrent CLI processes have separate budgets.
For a server needing lower traffic, set the environment's ceiling:

```text
tadx env update dev --catalog-max-concurrency 8
tadx env update dev --clear-catalog-max-concurrency
```

The configurable range is 1 to 256; clearing it restores the default of 32.

## Scope and other tools

TADX covers content artifacts, workspaces, administration, catalogs, and Pulse definition lifecycle.
It does not query datasource values, render view data or images, produce current Pulse insights, or semantically author workbooks.
Other connected tools remain independent: TADX does not configure, select, call, proxy, or report their connections.

## Contribute

Action builders should start with [CONTRIBUTING.md](CONTRIBUTING.md).
