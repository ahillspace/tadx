# Getting started with TADX

The [README](../README.md) covers installation and the shortest path to a downloaded workbook.

## Start with your local setup

Run `tadx` once to see configured environments, credential sources, workspace selection, and effective mutation policy.
This is a local overview, not an authentication check: it does not sign in, retrieve OS-stored PATs, create workspaces, or overwrite `tadx last`.
`--full` expands bounded setup details; `--json` changes their encoding.
Use `tadx --help` for the command index instead.

## What you can do next

Common next outcomes include:

- Find and pull workbook, datasource, flow, and lineage artifacts into a portable workspace.
- Inspect projects, users, groups, and permission rules, then preview an exact access or content change.
- Discover published datasource fields and create or reuse Pulse definitions and metric variants.

Choose a category from `tadx --help`.
Navigate with category help, or go directly to a resource reference for its verbs, flags, and examples:

```text
tadx content workbook --help
tadx admin group --help
tadx pulse definition --help
```

Resource and verb help are identical: `tadx content workbook pull -h` shows the same reference as `tadx content workbook -h`.
Root and category navigation stay short; `tadx admin group-member -h` documents membership separately from group lifecycle.
For direct-action categories such as `auth` and `env`, category help is the complete operational reference.
Skills and their optional references explain Tableau concepts and judgment, while help owns command syntax.
Use `tadx capability list` and `tadx capability get` for feature inventory and availability diagnostics; they are not required to discover command syntax.
Confirm consequential selectors and safety flags from the relevant help, and obtain the required authorization before execution.

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

## Use the local cache

Refresh the default inventory scopes when you need broad offline discovery:

```text
tadx cache refresh --environment dev
tadx cache status --environment dev --full
tadx search revenue --environment dev --type workbook --cache
```

`--cache` is local-only and never falls back to Tableau.
Review freshness, coverage, warnings, and partial results before treating a cache result as complete.
When a requested type needs unavailable cache scopes, the error reports available and missing types when local evidence permits.
When a safe narrowed search is available, its recovery command searches a cached type without refreshing or switching to live Tableau.
Narrowing the type changes coverage and cannot establish complete content inventory.
Permissions require an explicitly selected refresh scope because they add per-resource requests.
Cache collection adapts up to 32 concurrent requests per CLI process by default.
Use `tadx env update dev --cache-max-concurrency 8` to set a lower ceiling for a server that needs less traffic.

The local inventory commands are named `cache`; the former `catalog` commands and `--catalog` flag are not retained as aliases.
Existing SQLite files remain under the configuration root's `catalog/` directory, with unchanged database names and table signatures, so the rename does not discard saved observations.
Those storage names are an internal compatibility detail; use `cache` in commands and `cache_max_concurrency` in environment configuration.
If an older configuration contains `catalog_max_concurrency`, rename that key to `cache_max_concurrency` without changing its value.

## Preview remote changes

### Publish workbook dependencies

A workbook can reference independently published datasources, even when its file is a packaged `.twbx`.
Before publishing, confirm those datasources exist at the destination and the workbook references identify them correctly.
Publish missing referenced datasources first, then the workbook.
Workbook publication does not publish dependencies automatically or rebind references to a different site.
Pulling with `--include-pds` saves direct datasource dependencies as separate artifacts; it does not make the workbook self-contained.

### Check mutation consent

Remote mutations are disabled unless saved consent is enabled for the selected server and exact site.
Permission to perform a Tableau operation does not itself authorize changing site consent.
Check saved consent for all configured environments with:

```text
tadx mutation status
```

Pass `--environment <alias>` to restrict the result to one environment, or use `--full` for canonical server and exact site details.
The setting is keyed by the canonical server URL and exact site content URL.
Aliases for the same server and site share consent.
Different servers or sites require separate settings.

To enable or disable consent for one selected site, run one of these commands:

```text
tadx mutation set --environment dev --enabled=true
tadx mutation set --environment dev --enabled=false
```

Agents must ask before changing site consent.
The request must name the selected server, exact site, and persistent scope.
An operation request does not authorize a consent change.
The legacy `mutations_enabled` configuration field and `TADX_ENABLE_MUTATIONS=0` or `1` values do not authorize remote writes or inherit into site settings.
Site consent does not grant Tableau permissions or authorize unrelated work.

An optional administrator-managed policy can add a machine-wide capability ceiling.
An administrator can install a template and check the protected policy state:

```text
tadx policy install --template read-only
tadx policy status --full
```

Omitting `--template` installs the `superuser` template, which allows remote mutations subject to saved site consent and Tableau permissions.
Installing or updating a policy does not change site consent.
Standard templates include current administrative reads, while `read-write-no-admin` excludes administrative remote writes.
Reinstall a standard template from a current CLI to refresh its capability snapshot.

Use `tadx policy install` for a standard template; it does not accept candidate file paths.
For a custom policy, generate a candidate, edit it, and validate it before manual administrator deployment:

```text
tadx policy samples --output ./tadx-policy-candidates
tadx policy validate ./tadx-policy-candidates/read-only.json
```

Read [Managed policy](managed-policy.md) for policy locations, schemas, protected deployment, status states, and recovery.
Candidate validation does not activate a policy.
After an administrator deploys the validated candidate following that guide, use `tadx policy status --full` to inspect the protected policy state.
On Linux and macOS, the installer uses `/usr/bin/sudo` through the controlling terminal when the process is not root.
`sudo` handles its own password prompt; TADX does not capture or log the password.
See [Managed policy](managed-policy.md) for the platform defaults, custom directories, and path protection requirements.

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

### Find your Pulse subscriptions

List Pulse subscriptions for the user authenticated to the selected Tableau environment:

```text
tadx pulse subscription list --environment <alias>
```

The user is the owner of the configured PAT, which can differ from the account signed into Tableau in your browser.
The command retrieves that user's subscription records and enriches only their metric IDs with available names and saved configuration.
It does not scan every metric on the site or retrieve metric values and insights.
The default page contains up to 25 subscriptions; use `--limit`, follow the returned `--cursor` command, or use `--all` for a bounded complete listing.
Review any reported partial results before treating the output as complete.
Tableau's user-filtered subscription API does not explicitly document expansion through group membership; the command does not perform that expansion itself.
A controlled live test returned a group-derived subscription for a user who belonged to that group, but this does not establish complete coverage for every group arrangement.

## Maintain the installation

### Agent Guidance

The combined installer and `tadx update` refresh Guidance for detected agent directories.
`tadx agent install --target auto` performs that refresh independently; when none are detected it uses the shared `generic` location.
Explicit targets remain available:

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
| `generic` | `~/.agents/skills/` |

`~` represents your user home directory, including on Windows.
Each target receives the same `tadx` and `tadx-pulse` packages and their references.
Both `tadx agent install` and `tadx agent uninstall` accept these targets, with `--preview` available before changing files.
Installation does not create agent instruction files, alter MCP configuration, or prove that an agent has loaded the skills.
The installer uses these default directories, not custom agent installation roots.
TADX owns the `tadx` and `tadx-pulse` directories: installation and updates replace local edits without `--force`.
Keep personal extensions in a separate skill; unrelated packages are never replaced.

The added locations follow the official [OpenCode](https://opencode.ai/docs/skills/), [Pi](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/skills.md), and [Hermes](https://hermes-agent.nousresearch.com/docs/user-guide/features/skills) discovery conventions.
The other added targets use the documented personal skill directories for [GitHub Copilot](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills), [Gemini CLI](https://geminicli.com/docs/cli/skills/), and [Cline](https://docs.cline.bot/customization/skills).
These are local installations; installing for Copilot does not configure a hosted Copilot cloud agent.

### Guidance startup notice

When TADX does not detect its root skill in a supported location, it prints a short installation notice to stderr on the first invocation in a shell session.
Structured command output on stdout remains unchanged.
Detection checks local files, not whether the agent has actually read them.
Completion requests and the bare `tadx` overview do not emit the notice or write its session marker.
If session identification or the local notice cache is unavailable, the notice is skipped without failing your command.

Set `TADX_GUIDANCE_NOTICE=0` to suppress the notice for human-only or automated use.
To keep that preference across terminals, save the variable in your shell profile or user environment settings.
No credentials, mutation policy, or agent configuration are changed by the notice.

Session detection normally uses the parent process identity.
A host that launches a new shell for every tool call can set `TADX_GUIDANCE_SESSION` to a stable, unique session identifier.
Reusing that identifier suppresses repeated notices across those shells; TADX does not automatically identify every agent host.

### Upgrade

Run `tadx update --check` to check the release without installing anything.
Run `tadx update` to upgrade or repair the CLI and refresh bundled Guidance for detected agents.
Use repeated `--target` flags to choose specific agents, for example `tadx update --target codex --target claude`.
The platform installer also performs a combined CLI-and-Guidance installation.
Release installation uses published binaries, not the current source branch.

Released builds also check GitHub for a newer published release alongside eligible interactive commands.
A completed check is cached for 24 hours, and a short notice appears on stderr after a successful command when an update is available.
The check does not install anything, use Tableau credentials, or send Tableau information.
It has a short network timeout and does not wait for the network when your command finishes.
An unfinished check can retry on a later command; check failures do not change command results.

Automatic checks are skipped in CI, when stdout or stderr is not a terminal, and for JSON output, help, completion, previews, updates, cache commands, and the local overview.
Commands using `--cache` also skip the check.
Development builds do not show update notices.
Set `TADX_NO_UPDATE_NOTIFIER=1` to disable automatic checks and notices.
The explicit `tadx update --check` command remains available regardless of this setting.
Version comparison detects newer release versions, not replaced assets published under the same version.

### Uninstall TADX

The platform installer removes the TADX executable and its managed PATH and completion entries.
It preserves configuration, workspaces, caches, Guidance skills, managed policy, and credentials stored in the operating system.
Run the installer action from any directory; you do not need a TADX source checkout.

On Windows PowerShell, run:

```powershell
& ([scriptblock]::Create((Invoke-RestMethod https://tadx.net/install.ps1))) -Action Uninstall
```

On macOS or Linux, run:

```sh
curl -fsSL https://tadx.net/install.sh | sh -s -- uninstall
```

If you installed TADX in a custom directory, pass that same directory with `-InstallDir` on Windows or `--install-dir` on macOS or Linux.
An explicit directory takes precedence over `TADX_INSTALL_DIR`, which takes precedence over the platform default for both installation and removal.
The installer defaults to `%LOCALAPPDATA%\Programs\tadx\bin` on Windows and `$HOME/.local/bin` on macOS or Linux.
To preserve existing PATH and completion setup during removal, add both `-NoModifyPath -NoCompletion` on Windows or `--no-modify-path --no-completion` on macOS or Linux.

Removing stored credentials or Guidance is optional and must happen before removing the CLI.
Run only the commands for the items you want removed; replace `ENVIRONMENT_ALIAS` with an environment name and repeat Guidance removal for each installed target:

```text
tadx auth logout --environment ENVIRONMENT_ALIAS
tadx agent uninstall --target codex --preview
tadx agent uninstall --target codex
```

Preview each Guidance removal before running it.
Do not run the CLI removal until you finish these commands.
