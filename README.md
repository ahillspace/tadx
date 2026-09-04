# TADX

TADX is a deterministic Tableau lifecycle CLI for coding agents and humans.
It reduces token use and ambiguity by combining exact selectors, safe previews, compact TOON output, portable workspaces, and bounded lineage.

Commands return only the fields needed for the next decision by default.
When more bounded detail is available, the response advertises `--full` instead of making the caller search for another command.
Tableau LUIDs remain authoritative, ambiguous selectors fail, and consequential mutations support `--preview` before you run them.

## Current status

TADX is usable and actively developed.
The executable registry now covers the core content, workspace, catalog, administration, and initial Pulse workflows planned for V1.

The current build supports:

- Managing named environment profiles that reference PAT environment variables.
- Checking local authentication configuration and signing in to verify a Tableau site.
- Discovering capability ownership, availability, selectors, safety rules, and blockers.
- Querying Tableau live by default, with explicit local catalog reads through `--catalog`.
- Refreshing and inspecting the status of the local SQLite catalog.
- Searching supported content, administration, and Pulse resources live or through `--catalog`.
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

## Install from source

Install Git and Go 1.26 or later before building TADX.
Clone the repository on Windows, macOS, or Linux:

```shell
git clone https://github.com/ahillspace/tadx.git
cd tadx
```

### Windows

Build the binary from PowerShell:

```powershell
New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\Programs\tadx" | Out-Null
go build -o "$env:LOCALAPPDATA\Programs\tadx\tadx.exe" ./cmd/tadx
```

Add `%LOCALAPPDATA%\Programs\tadx` to your user `Path` in Windows Environment Variables, then open a new shell.

### macOS

Build the binary into a user-owned directory:

```shell
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/tadx" ./cmd/tadx
```

Add `export PATH="$HOME/.local/bin:$PATH"` to `~/.zshrc`, then open a new shell.

### Linux

Build the binary into a user-owned directory:

```shell
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/tadx" ./cmd/tadx
```

Add `export PATH="$HOME/.local/bin:$PATH"` to your shell profile, then open a new shell.

Verify the installed command:

```text
tadx --help
```

## Configure a Tableau environment

TADX uses Tableau personal access tokens and never stores their values in its configuration.
Use your shell, CI secret store, or credential manager to expose `TADX_DEV_PAT_NAME` and `TADX_DEV_PAT_SECRET` to the `tadx` process.

Register an environment profile that references those variable names:

```text
tadx env add dev --url https://example.tableau.com --site example-site --pat-name-env TADX_DEV_PAT_NAME --pat-secret-env TADX_DEV_PAT_SECRET
```

Inspect the resolved nonsecret configuration, then verify the credentials against Tableau:

```text
tadx auth status --environment dev
tadx auth check --environment dev
```

## Create a named workspace

Register a unique logical name for one machine-local workspace root:

```text
tadx workspace create development --path "<workspace-root>"
tadx env update dev --default-workspace development
tadx workspace status --workspace development
```

Lifecycle commands use the logical workspace name, while artifact paths remain relative and portable.

## Pull a workbook

Pull one workbook by its authoritative LUID:

```text
tadx content workbook pull --environment dev --workspace development --id <workbook-luid>
```

Add `--include-pds` to acquire direct published datasource dependencies as sibling artifacts.
Add `--full` to the same command when you need expanded, bounded details.

## Choose live or catalog reads

Supported read commands query Tableau by default and update their local catalog projection after a successful response.
Pass `--catalog` to read only from the local SQLite catalog without authenticating or contacting Tableau:

```text
tadx content workbook list --environment dev
tadx content workbook list --environment dev --catalog
```

TADX reports the selected source, freshness, and coverage in the same output shape.
Catalog reads never fall back to Tableau.

Refresh the complete local inventory when you need broad offline search:

```text
tadx catalog refresh --environment dev
tadx search revenue --environment dev --catalog
tadx catalog status --environment dev
```

## TADX and Tableau MCP

TADX owns Tableau development and lifecycle work, including content artifacts, workspaces, administration, and configuration lifecycle.
Tableau MCP owns analytical work, including datasource queries, view data and images, and Pulse metric values and insights.
They are peer tools, and TADX does not call or proxy Tableau MCP.

## Contribute

Action builders should start with [CONTRIBUTING.md](CONTRIBUTING.md).
