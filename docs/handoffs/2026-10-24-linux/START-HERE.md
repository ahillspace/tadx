# Agent entry prompt

Continue the TADX VS Code extension work from this Linux handoff.
Read this directory's [README](README.md), [context](CONTEXT.md), [requested capabilities](REQUESTED-CAPABILITIES.md), and [validation record](VALIDATION.md).
Then read the repository [agent guide](../../../AGENTS.md), [architecture entry point](../../architecture/README.md), and [extension architecture](../../architecture/editor-extension.md).

The original offline capture uses `feat/vscode-artifact-explorer` at `8976a2b42f71c6ec586b64f7dbcd760bb2a27b0f` with uncommitted changes.
The entire extension and several supporting files were untracked in that capture.
The archive also preserves concurrent tracked changes outside this task.
The maintainer now plans to transfer the reviewed source and handoff through Git.
Use the actual reviewed commit when continuing through Git; do not reset it to the historical archive base.
Inspect status and preserve all captured work.
Do not reset, clean, or overwrite these files to start from a clean checkout.

The user requested this handoff to move development to Linux.
The completed extension features require Linux verification, not a rebuild from scratch.
On 2026-10-25, the user authorized repository integration, versioning, compatibility checks, release packaging, release automation, and upgrade guidance here.
The user also explicitly included guided CLI installation and update buttons in this build.
Check the current source and validation record for that task's final result.
The working source prepares extension `0.2.1`, with capability checks, guided CLI setup, and separately dispatched release packaging.
Read the [release and upgrade guide](../../vscode-extension-release.md) before changing versions, release pins, installation behavior, or publication.
The configuration page remains a deferred feature, with requested actions and additional suggestions separated in [REQUESTED-CAPABILITIES.md](REQUESTED-CAPABILITIES.md).
Keep individual **Delete Local** actions; bulk deletion is out of scope, and workspace removal or cleanup remains undecided.
After orientation, report the restored state and obtain the next implementation instruction before starting those deferred features.
The backlog does not authorize Marketplace publication, automatic CLI replacement, or changes to saved Tableau consent.

Apply these session requirements alongside the repository guide:

- Prefer correctness, simple ownership, and maintainability over small diffs or development cost.
- Reproduce bugs through the closest practical end-user flow before changing behavior.
- Use subagents for multistep or context-heavy tasks, with disjoint file ownership and one shared feature branch.
- Suggest regression-first tests for substantial builds and a review pass afterward.
- Do not launch agent-based reviews without an explicit review request.
- Do not use Astra models for tests or validation.
- Do not invoke `no-mistakes` unless explicitly requested.
- Invoke `tadx-build` only when explicitly requested for a named build task.
- Run automated checks locally; dispatch hosted CI only on explicit request.
- Preserve required gates and report unavailable native or privileged checks as limitations.
- Ask before a narrow task expands into prolonged unrelated remediation.
- Do not manually edit changelogs or generated files.
- Use plain dashes in human-facing writing and one sentence per line in long Markdown.
- Do not add an agent co-author to commit messages.
- Keep usernames, machine paths, private site names, tokens, and session data out of tracked content.
- Keep persistence, Tableau access, credentials, and mutation enforcement in the CLI.
- Obtain explicit approval before persisting PAT credentials or changing exact-site mutation consent.
- Do not send Slack messages, push, publish, or release as part of this handoff.

The source package uses Go 1.26 and Node.js 22.15 or later.
The extension uses native VS Code trees and editor webviews, with the public CLI as its backend.
The native credential store and Tableau setup belong to the Linux workspace host.
The Windows verification record does not establish Linux, remote-host, or live Tableau compatibility.
