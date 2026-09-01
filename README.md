# TADX

TADX is a deterministic Tableau lifecycle CLI for coding agents and humans.
It reduces token use and ambiguity by combining exact selectors, safe previews, compact TOON output, portable workspaces, and bounded lineage.

Commands return only the fields needed for the next decision by default.
When more bounded detail is available, the response advertises `--full` instead of making the caller search for another command.
Tableau LUIDs remain authoritative, ambiguous selectors fail, and consequential mutations preview before `--apply`.

## Current status

TADX is usable and actively developed.
The executable registry marks roughly half of the core V1 action set as implemented.

The current build supports:

- Managing named environment profiles that reference PAT environment variables.
- Checking local authentication configuration and signing in to verify a Tableau site.
- Discovering capability ownership, availability, selectors, safety rules, and blockers.
- Searching the local catalog.
- Creating, registering, cloning, listing, inspecting, and moving named workspaces, plus deleting one local artifact safely.
- Listing and inspecting projects.
- Listing, inspecting, and pulling flows, plus previewed publishing, moves, and deletions.
- Pulling workbooks with optional direct published datasource acquisition, plus previewed publishing.
- Capturing bounded lineage for workbooks, published datasources, and flows.

Remaining V1 work includes broader catalog and content coverage, direct datasource lifecycle, administration, Pulse definitions and configuration, and diagnostics.
Some datasource composition and project pull work remains blocked until the required upstream evidence exists.

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

## TADX and Tableau MCP

TADX owns Tableau development and lifecycle work, including content artifacts, workspaces, administration, and configuration lifecycle.
Tableau MCP owns analytical work, including datasource queries, view data and images, and Pulse metric values and insights.
They are peer tools, and TADX does not call or proxy Tableau MCP.

## Contribute

Action builders should start with [CONTRIBUTING.md](CONTRIBUTING.md).
