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
Run `tadx --help` for the command index, then one category help call such as `tadx content --help` to learn its commands, flags, and examples, including nested actions.
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

Run `tadx` for a local overview of your configuration, or `tadx --help` for the command index.
Category help is complete for its scope: `tadx admin group --help` includes membership actions, while `tadx admin --help` includes all administration actions.
Leaf help such as `tadx admin group create --help` remains available for a focused lookup.

## Add skills to your agent

Install the bundled [TADX skill](internal/agent/skills/tadx/SKILL.md) and [Pulse authoring skill](internal/agent/skills/tadx-pulse/SKILL.md) so your agent knows the commands and how to use them safely:

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

## Safe defaults

Read operations query Tableau live unless you explicitly select the local cache.
Compact TOON output is the default; `--full` adds bounded detail for the same operation.
Use `--json` when you need JSON output.
Tableau LUIDs are authoritative, and ambiguous selectors fail instead of guessing.

Remote mutations are disabled by default.
Supported read-only previews remain available while mutations are disabled.
Enabling remote mutations is an optional, explicit opt-in that is separate from permission to perform a particular Tableau operation.
Agents must ask before changing the mutation setting or its scope.
See `tadx mutation status` and `tadx mutation set --help` when you are ready to configure that policy.

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

## License

TADX is licensed under [Apache 2.0](LICENSE).
See [third-party notices](THIRD_PARTY_NOTICES.md) for bundled dependencies.
