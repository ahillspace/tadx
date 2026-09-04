# tadx build TODO

Running backlog of things to build.
Add new items under Backlog; move to Done when shipped.

## Backlog

### 1. Workspace location handling

Problem.
`tadx workspace create` and `workspace register` both require `--path` (hard usage error if omitted).
There is no default location - not home, not XDG, not the current project.
Worse, the staging dir is created in `filepath.Dir(root)`, so a shallow path like `/tableau` aims the mkdir at `/` and fails with a cryptic `mkdir /.tadx-workspace-stage-...: read-only file system` on macOS.
The corrective_action does not point at the real cause.

Recommended change.
- Make `--path` optional. When omitted, default to a platform data dir: `<data-dir>/tadx/workspaces/<name>` (macOS `~/Library/Application Support/tadx/workspaces/`). This mirrors the pattern tadx already uses for config via `os.UserConfigDir()`.
- Keep `--path` as an explicit override.
- Validate the resolved root: reject a path whose parent resolves to `/` (or any non-writable / too-shallow root) with a real usage error, instead of letting the downstream mkdir blow up.
- Offer a project-local mode (`--path .` or a `--here` flag) for the case where the workspace should sit beside the code.
- VS Code extension: the extension supplies the workspace root automatically (the open folder); the human never types a path. The `containingWorkspace` cwd-matching already exists as the resolution seam - reuse it.

Recommendation.
Default to the data dir, not the current project folder, because workspaces hold pulled artifacts and should not spray into whatever directory you happen to be in.
Provide the project-local mode as opt-in.

Code seams.
`internal/cli/workspace/command.go:103` and `:130` (the empty-path rejection), and `internal/workspace/manager.go:632` (canonicalRoot empty-root guard) plus `createRoot` (`manager.go:478`) for the root validation.

### 2. Auth secret management

Problem.
tadx today resolves PAT secrets only from environment variables referenced by name in the env profile (`--pat-name-env` / `--pat-secret-env`).
This is a hardened variant of the plain-env-var pattern - great for CI and agents, but rough for a human at a laptop: secrets evaporate per shell and users must hand-roll `~/.zshrc` or a wrapper.
Most CLIs offer a `login` subcommand (token cached to a dotfile) or an OS keychain / credential helper for interactive use.

Recommended change.
Add a layered secret source, resolved in precedence:
1. Explicit env var (CI / agents) - already built, keep as the automation contract.
2. OS keychain (macOS Keychain, Windows wincred, Linux secret-service / pass) for interactive humans - add this as the default human path, ideally via a `tadx auth login` that stores the PAT in the keychain.
3. Never a plaintext file as the default.

Recommendation.
This keeps the existing "never persist plaintext" invariant intact while giving humans the frictionless experience they expect (docker and gh already prove this model).
It is additive - the env-var-reference path stays; the keychain becomes the default interactive path.

### 3. Decide how to expose the CLI to an LLM (empirical, not theoretical)

Problem.
Open question: how does an agent best learn to drive tadx - just `--help`, a repo skill, an AGENTS.md, or improved help strings / command renames?
Known friction candidates already: broken `help` subcommand and missing `version`, `--path` required on workspace create with a cryptic read-only-root error, the env-var secret indirection, and the catalog-vs-content split.
Rather than pick blind, run fresh agents against the CLI with logging and watch where they get confused.

Planned experiment.
- Logging shim: a `tadx` wrapper on PATH that appends every invocation (argv + exit code + stderr) to a log file, then calls the real binary - objective command trace, independent of agent self-report.
- Fresh agents that know only that a `tadx` binary exists, explicitly forbidden from reading the tadx source tree (source access would fake the confusion signal). One realistic, human-phrased task each.
- Tasks: (1) connect to my site and confirm it works; (2) find all workbooks owned by an email; (3) download a named workbook locally; (4) list every user; (5) "what can this tool do?" (pure discovery - tests help / --help / capability list).
- Optional broader pass: mutation attempts, `--preview`, ambiguous selectors, and invalid inputs to exercise mutation safety and error messages.
- Output: collect command traces + confusion notes, synthesize a ranked friction report, map each point to a fix (rename, AGENTS.md, help-string, defect). That report decides skill vs AGENTS.md vs better help empirically.

Status / notes.
- A live Tableau site + PAT is available for an end-to-end run (auth -> refresh -> search -> pull), which gives the richest signal.
- Not running yet - queued for when we pick this up.

## Done

(nothing yet)
