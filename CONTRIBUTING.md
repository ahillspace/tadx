# Build an action for TADX

TADX contributions are small, isolated actions built on shared Tableau adapters and CLI infrastructure.
The maintained engineering standards now live in the repository-local [tadx-build skill](.agents/skills/tadx-build/SKILL.md), rather than a second copy here.
It covers package boundaries, adapters, output, identity, authentication, mutation policy, registry integration, and verification.
It requires no OpenSpec, Superpowers, or agent-review plugin.

## Start with your agent

Open a checkout of this repository in your coding agent and invoke:

```text
$tadx-build Build [the action you need]. The user outcome is [outcome].
Include [scope], exclude [scope], and stop after implementation and automated tests.
```

If your agent does not discover repo-local skills, give it the explicit entry point:

```text
Read .agents/skills/tadx-build/SKILL.md and use it to build [the action you need] in this repository.
```

The complete skill directory is tracked in Git, with its references alongside it.
All implementation paths in the skill are relative to the repository root.
It is opt-in and does not replace instructions for unrelated tasks.
`tadx agent install` installs end-user operating Guidance, not this development skill.

## Human contributors

Read the [build standard](.agents/skills/tadx-build/SKILL.md) and choose one relevant example from its [implementation map](.agents/skills/tadx-build/references/implementation-map.md).
Only inventory and cache changes need the [inventory contracts](.agents/skills/tadx-build/references/inventory-and-cache.md).
Start from updated `main` on a feature branch; shared agents use disjoint files in one checkout.
Keep tests and bounded evidence with the implementation, and use the skill's verification checklist before handoff.
Live tests require separately authorized targets and remain outside the standard suite.
The maintainer handles agent-based review; a build request does not automatically authorize a push, PR, merge, or release.
