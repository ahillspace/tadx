# TADX

TADX is a CLI authored specifically to work well with agents, but is also useful to operate by hand.
The main goal is to allow agents to work with both Tableau Cloud and Server with reduced confusion, token usage, and vastly increased speed.

Initial testing has shown drastic improvement over all other methods of equipping agents with the tools they need to work with Tableau.

TADX handles Tableau lifecycle work, not natural-language data queries or analytical rendering.

Feedback and collaboration are openly welcomed, with dedicated documentation for anyone looking to add actions to TADX using coding agents (see the [tadx-build skill](.agents/skills/tadx-build/SKILL.md)).

## What you can do

TADX is an independent, pre-1.0 project and is not supported by or associated with Tableau or Salesforce.
It is usable and actively developed.

Give your agent an outcome, not a list of API calls:

- Find sales workbooks, inspect their dependencies, and download a useful working set.
- Prepare a project and access for a temporary analyst.
- Discover suitable datasource fields and create a meaningful Pulse metric.
- Find missing descriptions in an upstream table and enrich the metadata without changing the underlying data.

Or use the same CLI directly to inspect, download, organize, and publish Tableau content.
TADX supports workbook, datasource, flow, and project lifecycle operations, local workspaces and caches, lineage, administration, upstream catalog metadata, and Pulse definition workflows.
Run `tadx --help` for the command roadmap and `tadx content --help` for its four resources and available actions.
Use a resource reference such as `tadx content workbook --help` for all its verbs, required inputs, options, accepted values, defaults, and constraints.
Every verb shows its owning operational reference; `catalog lineage` captures lineage and `catalog label` manages asset labels.
Use `tadx capability list` and `tadx capability get` for feature inventory and availability diagnostics.

## Install TADX and agent guidance

One-line installation from [tadx.net](https://tadx.net) is **coming soon**.

For now, [build TADX from source](CONTRIBUTING.md#run-the-current-source) on Windows, macOS, or Linux.
Add the resulting `bin` directory to your `PATH` to run the `tadx` commands below, or use the executable's path directly.

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
TADX validates the PAT and uses Windows Credential Manager, macOS Keychain, or Linux Secret Service to store it securely.
If the credential store is unavailable or locked, login fails instead of falling back to plaintext.

For CI or temporary use, environment profiles can instead reference a PAT name variable and a PAT secret variable.
See `tadx env --help` for those options.

Run `tadx` for a local overview of your configuration, or `tadx --help` for the command roadmap.
Category help is a short roadmap; resource help contains the complete syntax for that resource.
For example, `tadx admin --help` lists resources, `tadx admin group --help` covers group operations, and `tadx admin group-member --help` covers membership changes.
Verb help, such as `tadx admin group create --help`, repeats its resource reference; there is no need to request both.
Direct-action categories such as `auth` and `env` provide their complete reference at that level.
Selected useful aliases appear beside canonical flags; the [shorthand reference](docs/reference/shorthand.md) lists all shortcuts.
Both `-h` and `--help` show help without running the operation or reading credentials.

## Add skills to your agent

Install the bundled [TADX skill](internal/agent/skills/tadx/SKILL.md) and [Pulse authoring skill](internal/agent/skills/tadx-pulse/SKILL.md) for effective discovery and Tableau-specific judgment:

```text
tadx agent install --target auto
```

This detects supported agent directories and installs standard `SKILL.md` packages for each.
If none are detected, it uses the shared `~/.agents/skills` directory.
You can also choose a target explicitly:

```text
tadx agent install --target claude
tadx agent install --target codex
tadx agent install --target cursor
```

OpenCode, Pi, Hermes, GitHub Copilot, Gemini CLI, and Cline are also supported.
Use `--preview` to inspect the local file changes first.
Updates replace TADX's skill packages, including local edits, so keep personal additions in a separate skill.
See [agent guidance](docs/getting-started.md#agent-guidance) for all targets and installation locations.

### Installer switches

The one-line installers accept the same independent controls.

- `--version VERSION` or `-Version VERSION` selects an exact release; the default is `latest`.
- `--target TARGET` or `-Target TARGET` selects bundled agent Guidance; repeat it on Unix or pass a comma-separated list on Windows.
- `--install-dir DIRECTORY` or `-InstallDir DIRECTORY` selects the binary directory.
- The Unix directory precedence is `--install-dir`, `TADX_INSTALL_DIR`, then `$HOME/.local/bin`.
- The Windows directory precedence is `-InstallDir`, `TADX_INSTALL_DIR`, then `%LOCALAPPDATA%\Programs\tadx\bin`.
- `--no-modify-path` or `-NoModifyPath` leaves PATH and shell profiles unchanged.
- `--no-completion` or `-NoCompletion` leaves shell completion profiles unchanged.

For an installation without profile edits, use `install.sh --no-modify-path --no-completion` or `install.ps1 -NoModifyPath -NoCompletion`.

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

The full status output shows where the downloaded artifacts are stored.
Use pull's `--preview` to inspect acquisition scope and local conflicts before writing artifacts.

## Safe defaults

Read operations query Tableau live unless you explicitly select the local cache.
Compact TOON output is the default; `--full` adds bounded detail for the same operation.
Use `--json` when you need JSON output.
Tableau LUIDs are authoritative, and ambiguous selectors fail instead of guessing.

Remote mutations are disabled by default.
Supported read-only previews remain available while mutations are disabled.
Acquisition and local-state commands also expose previews for their planned file or configuration changes.
Interactive `auth login` and policy-changing `mutation set` remain explicit operations; `update --check` checks releases without installing them.
Enabling remote mutations is an optional, explicit opt-in that is separate from permission to perform a particular Tableau operation.
Agents must ask before changing the mutation setting or its scope.
See `tadx mutation status` and `tadx mutation set --help` when you are ready to configure that policy.
For repeated work, the operational reference identifies supported selectors and `--batch-file` inputs, including positional `args` arrays.
Batches vary one selector dimension at a time or use explicit item rows, with at most 100 expanded selections.

## Learn more

- [Getting started](docs/getting-started.md): credentials, content, caches, previews, Pulse, and updates.
- [Workspaces](docs/workspaces.md): local artifacts, paths, status, and cloning.
- [Catalog metadata](internal/agent/skills/tadx/references/catalog.md): inspection, descriptions, tags, audits, and labels.
- [Capabilities](docs/reference/capabilities.md) and the [interactive capability map](docs/reference/capability-map.html).
- [Command aliases](docs/reference/shorthand.md).
- [Contributing](CONTRIBUTING.md) and [architecture](docs/architecture/README.md).

## Future Vision

The current goal is to get TADX running quickly and smoothly against the simple content lifecycle you see with Tableau Cloud and Server.
This is to get it ready to augment the new experiences coming in Tableau (Tableau Authoring API, Tableau Knowledge Graph, Tableau MCP, TDS API, Composable Datasources, etc.).
Augmenting semantics, modifying published datasources, cleaning and composing data sources, and managing access with agents is all in scope as these new features become available and TADX is meant to act as the platform that allows agents to assist with these activities cleanly, quickly, cheaply, and at scale.

### Optional native search categories

Proposed: 2026-09-14; not implemented.
Extend `tadx search` with opt-in categories so agents can use improvements to Tableau's native relevance search beyond the current content, administration, and Pulse results.
Candidates include views, databases/files, tables/objects, virtual connections, collections, and Prep data roles; include lenses only where the deployed Tableau API still supports them.
Syntax such as `--include view,database,table` is illustrative and requires a CLI design decision.
Preserve current defaults, retain each category's native identity and parent relationship, and never represent a connection, table, or database as a published datasource merely because its name or URL resembles one.
Preserve Tableau's mixed relevance order across native categories, including semantic or AI ranking when supplied by the service.
Before implementation, define the category names, combination with `--type`, per-category result fields and follow-up commands, identity namespaces, and pagination after filtering or grouping.
Also decide explicit cache behavior for categories without local coverage and how this complements `catalog search` rather than silently substituting its different Metadata API matching behavior.
See the [current search architecture](docs/architecture/search.md) for the source split and the connection-identity defect that motivated this proposal.

## License

TADX is licensed under [Apache 2.0](LICENSE).
See [third-party notices](THIRD_PARTY_NOTICES.md) for bundled dependencies.
