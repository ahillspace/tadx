---
name: tadx-explore
description: Explore installed TADX correctness and workflow efficiency with Luna, using bounded scopes, fixtures, evidence, cleanup, and handoff.
---

# Explore TADX with Luna

State and reports use ignored `.tadx-explore/`; setup only prepares payloads.
Assess both correctness and avoidable work, including successful workflows.
Choose a user outcome within the authorized scope, not only a set of commands to stress.
Use category or resource scope when that outcome spans verbs; keep focused regressions narrowly scoped.

Choose a scope, set selectors, and prepare:

```text
node .agents/skills/tadx-explore/scripts/explore.mjs init --environment <alias>
node .agents/skills/tadx-explore/scripts/explore.mjs prepare --scope "content workbook move"
```

`init` writes selectors and budgets to `.tadx-explore/settings.json` when absent; `prepare` accepts `--environment`, `--workspace <registered-name>`, and `--project-id` overrides.
Fixture writes need a named environment, while local read-only scopes can use null.
Pass `--allow-fixture-writes` to `prepare`, never to `init`, when explicit authorization covers the selected environment and disposable fixtures.
The flag records authority on that run only, and existing authorization can cover later runs in the same scope.
Pass the returned `spawn` object unchanged to `spawn_agent` with a fresh Luna call, preserving `model: gpt-6-luna`, `reasoning_effort: medium`, and `fork_turns: none`.
Do not paste custom prompts.

After each worker, run `next --run <run-id>` and pass returned spawns unchanged until a terminal summary or user stop.
If `next` reports pending without a spawn, wait for the worker and do not reissue the pass.
Use `summarize [--run <run-id>]` for aggregate counts, and omit `--run` after terminal state to refresh the cross-run summary.
Do not read full reports unless the user requests details.
Keep handoff to 120 words or fewer with the summary path and counts.
Only the worker reads [references/worker.md](references/worker.md); the parent must not load it.

Treat runs as remote-read-only unless their manifests record covered fixture-write authorization.
Never change authentication, consent, policy, installed software, or ordinary user configuration.
If consent is missing, stay read-only and report the exact server and site permission needed.
Delete or move only unique, run-owned copies, and stop after an uncertain write or unresolved cleanup.
