# Getting started with TADX

The [README](../README.md) covers installation and the shortest path to a downloaded workbook.

## What you can do next

Common next outcomes include:

- Find and pull workbook, datasource, flow, and lineage artifacts into a portable workspace.
- Inspect projects, users, groups, and permission rules, then preview an exact access or content change.
- Discover published datasource fields and create or reuse Pulse definitions and metric variants.

Run `tadx capability list` for the exact feature set in your installed release.
Run `tadx <command> --help` before a consequential operation to confirm its selectors and safety flags.

## Verify authentication

Inspect nonsecret credential readiness without contacting Tableau:

```text
tadx auth status --environment dev
```

Validate the active credential against the configured site:

```text
tadx auth check --environment dev
```

To replace an OS-stored PAT, run login again and enter the replacement at the secure prompts:

```text
tadx auth login --environment dev
```

To remove only the PAT saved by TADX:

```text
tadx auth logout --environment dev
```

Logout does not revoke or delete the PAT in Tableau.
If configured PAT environment variables are still present, they remain available to TADX.
An OS-stored PAT stays available until you remove or replace it locally, or Tableau rejects it because it expired, was revoked, or was disabled.

For automation, configure both PAT environment-variable references on the environment profile:

```text
tadx env update dev --pat-name-env TADX_DEV_PAT_NAME --pat-secret-env TADX_DEV_PAT_SECRET
```

A complete variable pair overrides an OS-stored PAT for that process, and TADX does not combine partial credentials from different sources.

## Pull other project assets

Search first, select the intended returned LUID, and then pull with that exact identity:

```text
tadx search customer --environment dev --type datasource
tadx content datasource pull --environment dev --id DATASOURCE_LUID

tadx search forecast --environment dev --type flow
tadx content flow pull --environment dev --id FLOW_LUID
```

Inspect exact locations and local state afterward:

```text
tadx workspace status --workspace development --full
```

See [Workspaces](workspaces.md) for workspace paths, cloning, registration, and local artifact operations.

## Use the local catalog

Refresh the default inventory scopes when you need broad offline discovery:

```text
tadx catalog refresh --environment dev
tadx catalog status --environment dev --full
tadx search revenue --environment dev --type workbook --catalog
```

`--catalog` is local-only and never falls back to Tableau.
Review freshness, coverage, warnings, and partial results before treating a catalog result as complete.
Permissions require an explicitly selected refresh scope because they add per-resource requests.
Catalog collection adapts up to 32 concurrent requests per CLI process by default.
Use `tadx env update dev --catalog-max-concurrency 8` to set a lower ceiling for a server that needs less traffic.

## Preview remote changes

Remote mutations are disabled unless an authorized user explicitly opts in to a session or saved policy.
Permission to perform a Tableau operation does not itself authorize changing that policy.
Check the effective policy and source with:

```text
tadx mutation status
```

If you choose to enable remote writes persistently, run `tadx mutation set --enabled=true`.
This affects future sessions until changed; `tadx mutation set --enabled=false` disables the saved policy.
An explicit `TADX_ENABLE_MUTATIONS=0` or `1` in the process overrides the saved setting.
These settings do not grant Tableau permissions or authorize an agent to perform unrelated work.

Supported mutation commands accept `--preview` while execution is disabled.
A preview resolves the exact target and proposed settings without authorizing or applying the change.

For example, this prepares a new site user without creating it:

```text
tadx admin user create --environment dev --name USERNAME --site-role SITE_ROLE --auth-setting AUTH_SETTING --preview --full
```

Choose a supported Tableau site role and use the site's identity configuration for the authentication setting.

## Explore Pulse authoring

Start from an exact published datasource and inspect its field metadata:

```text
tadx search sales --environment dev --type datasource
tadx content datasource schema --environment dev --id DATASOURCE_LUID --query sales --limit 20 --full
```

Use the installed Pulse Guidance for complete measure, date, dimension, and slicer discovery.
Inspect existing definitions before creating a duplicate:

```text
tadx pulse definition list --environment dev --datasource-id DATASOURCE_LUID --all --full
```

## Maintain the installation

### Agent Guidance

Current source builds support these global installation locations:

| Target | Default skill directory |
| --- | --- |
| `claude` | `~/.claude/skills/` |
| `codex` | `~/.codex/skills/` |
| `cursor` | `~/.cursor/skills/` |
| `opencode` | `~/.config/opencode/skills/` |
| `pi` | `~/.pi/agent/skills/` |
| `hermes` | `~/.hermes/skills/` |
| `copilot` | `~/.copilot/skills/` |
| `gemini` | `~/.gemini/skills/` |
| `cline` | `~/.cline/skills/` |

`~` represents your user home directory, including on Windows.
Each target receives the same `tadx` and `tadx-pulse` packages and their references.
Both `tadx agent install` and `tadx agent uninstall` accept these targets, with `--preview` available before changing files.
Installation does not create agent instruction files, alter MCP configuration, or prove that an agent has loaded the skills.
The installer uses these default directories, not custom agent installation roots.

The added locations follow the official [OpenCode](https://opencode.ai/docs/skills/), [Pi](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/skills.md), and [Hermes](https://hermes-agent.nousresearch.com/docs/user-guide/features/skills) discovery conventions.
The other added targets use the documented personal skill directories for [GitHub Copilot](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills), [Gemini CLI](https://geminicli.com/docs/cli/skills/), and [Cline](https://docs.cline.bot/customization/skills).
These are local installations; installing for Copilot does not configure a hosted Copilot cloud agent.

### Guidance startup notice

When TADX does not detect its root skill in a supported location, it prints a short installation notice to stderr on the first invocation in a shell session.
Structured command output on stdout remains unchanged.
Detection checks local files, not whether the agent has actually read them.
Completion requests do not emit the notice.
If session identification or the local notice cache is unavailable, the notice is skipped without failing your command.

Set `TADX_GUIDANCE_NOTICE=0` to suppress the notice for human-only or automated use.
To keep that preference across terminals, save the variable in your shell profile or user environment settings.
No credentials, mutation policy, or agent configuration are changed by the notice.

Session detection normally uses the parent process identity.
A host that launches a new shell for every tool call can set `TADX_GUIDANCE_SESSION` to a stable, unique session identifier.
Reusing that identifier suppresses repeated notices across those shells; TADX does not automatically identify every agent host.

### Upgrade or remove

Run the platform installer again to upgrade or repair the CLI.
Then reinstall Guidance separately for each agent that should receive the new bundled packages, such as `tadx agent install --target codex`.

Preview Guidance removal before uninstalling it:

```text
tadx agent uninstall --target codex --preview
tadx agent uninstall --target codex
```

Removing the CLI does not automatically remove configuration, workspaces, catalogs, Guidance, or OS-stored credentials.
Use `tadx auth logout` and `tadx agent uninstall` first for any local state you also want removed.
Download the platform installer again as shown in the README, then invoke its uninstall action instead of its default installation action:

```powershell
& $installer -Action Uninstall
```

```sh
sh "$installer_dir/install.sh" uninstall
```
