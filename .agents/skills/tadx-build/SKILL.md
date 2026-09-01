---
name: tadx-build
description: Apply selected repository-local Superpowers practices to one explicitly named TADX task. Use only when the user invokes $tadx-build.
---

# TADX build

Use this skill only for the task named in the invocation.
Read the current `AGENTS.md` and follow it.
Do not infer scope, authority, project state, or required source files from this skill.

Read `docs/contributing/build-context.md`, then read only the applicable vendored Superpowers instructions:

- For a feature or bug fix, read [test-driven development](references/superpowers/test-driven-development/instructions.md) before implementation.
- For a bug, test failure, or unexpected behavior, read [systematic debugging](references/superpowers/systematic-debugging/instructions.md).
- For two or more independent tasks without shared state, read [dispatching parallel agents](references/superpowers/dispatching-parallel-agents/instructions.md).
- After implementation, read [requesting code review](references/superpowers/requesting-code-review/instructions.md).
- When review feedback arrives, read [receiving code review](references/superpowers/receiving-code-review/instructions.md).
- Before claiming completion, read [verification before completion](references/superpowers/verification-before-completion/instructions.md).
- Only when the user explicitly requests an isolated worktree, read [using Git worktrees](references/superpowers/using-git-worktrees/instructions.md).

Read each selected file completely and follow it through the current repository instructions.
Use the current harness equivalent when a vendored file names a tool from another harness.
Do not load unlisted Superpowers material.
No Superpowers plugin or user-level skill installation is required.
