# TADX Linux handoff

The original archive captures the VS Code extension work and related repository state on 2026-10-24.
These working handoff documents also record the subsequent scope decisions from 2026-10-25.
Start with [the agent prompt](START-HERE.md), [the context](CONTEXT.md), [requested capabilities](REQUESTED-CAPABILITIES.md), and [the validation record](VALIDATION.md).
The current build covers repository integration, versioning, CLI compatibility, release packaging, release automation, upgrade guidance, and guided CLI installation and update actions.
The working source prepares extension `0.2.0`; the [release guide](../../vscode-extension-release.md) owns its versioning, setup, and publication procedures.
The configuration-page backlog remains deferred to Linux development.

## Continue through Git

The maintainer now prefers committing and pushing the intended source and handoff files, then pulling that branch on Linux.
Use the reviewed branch and commit identified in the final handoff or PR.
Verify that `extensions/vscode` and this handoff directory exist in that commit before relying on a pull to transfer them.
Untracked files and ignored archives do not transfer through Git.

After cloning or pulling the reviewed branch, read [START-HERE.md](START-HERE.md) from that checkout.
The source and handoff documents in the reviewed commit take precedence over the older offline archive.
Do not apply the historical archive patch over a newer or dirty checkout.

## Transfer the original archive

The portable archive is `dist/handoffs/tadx-linux-handoff-2026-10-24.zip`, relative to the repository root.
It is an optional offline backup of the original capture, not the current Git delivery artifact.
It does not include subsequent source or handoff edits unless explicitly regenerated and reverified.
Copy that archive to the Linux machine and extract it into a new directory.
Keep the archive outside the destination checkout.

The archive contains:

| File | Purpose |
| --- | --- |
| `repository.bundle` | Offline Git history for the captured feature branch. |
| `working-tree.patch` | Every tracked working-tree change against the captured commit. |
| `untracked-source.tar.gz` | Allowlisted new extension files, documentation, and Go tests. |
| `tadx.vsix` | The last verified extension package for optional manual installation. |
| `manifest.json` | Base revision, included files, excluded paths, and content hashes. |
| `working-tree-status.txt` | The captured status of included tracked and untracked paths. |
| `SHA256SUMS` | Integrity hashes for the bundle payload and handoff documents. |
| `README.md`, `START-HERE.md`, `CONTEXT.md`, `VALIDATION.md` | Instructions and development context. |

The working-tree patch includes concurrent edits outside the extension task.
Preserve those changes and inspect their ownership before committing, reverting, or expanding their scope.
The manifest identifies the captured paths; inclusion does not establish task ownership or validation.

The bundle excludes unrelated untracked `.sf/`, `jev-evals/`, `tadx-policy-check/`, `CONSOLIDATION-PROPOSAL.md`, `docs/se-fork-build-plan.md`, and `docs/se-package-coding-prompt.md`.
It excludes ignored local configuration, credentials, caches, downloaded artifacts, workspaces, dependencies, test runtimes, and generated outputs, except the explicitly included VSIX.
This is a development handoff, not a complete backup of the Windows machine or every untracked project.

## Restore the original archive on Linux

Run the following commands inside the extracted bundle directory.
Use a new destination; do not apply the capture over an existing dirty checkout.
Stop if any command fails.

```bash
sha256sum --check SHA256SUMS
git clone --branch feat/vscode-artifact-explorer ./repository.bundle ./tadx
git -C tadx bundle verify ../repository.bundle
test "$(git -C tadx rev-parse HEAD)" = "8976a2b42f71c6ec586b64f7dbcd760bb2a27b0f"
git -C tadx apply --check ../working-tree.patch
git -C tadx apply ../working-tree.patch
tar -xzf untracked-source.tar.gz -C tadx
git -C tadx remote set-url origin https://github.com/ahillspace/tadx.git
git -C tadx status --short
```

Git restores tracked shell executable modes and repository symlinks from its history.
The patch preserves tracked changes without copying Windows checkout representations of those symlinks.
The new source archive contains only relative regular-file paths.

The restored branch remains at the captured commit with the captured edits uncommitted.
Continue this branch while reviewing the handoff; do not replace it with a fresh `main` checkout and discard the work.
The remote URL change configures future Git operations and does not fetch or push anything.

## Resume work

Give the Linux agent [START-HERE.md](START-HERE.md) from the restored repository.
Use [VALIDATION.md](VALIDATION.md) to build the Linux CLI and validate the extension.
Rebuild dependencies and native test fixtures on Linux instead of copying Windows outputs.
Configure Tableau access locally only if the next authorized task requires it.
No saved credentials, machine configuration, or mutation consent transfer with this handoff.

The original bundle is an immutable capture of its date's state.
These repository documents can advance beyond that capture.
Later archive deliveries require a new capture and integrity verification.
