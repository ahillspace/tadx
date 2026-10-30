# TADX documentation

Use these guides for current behavior and implementation boundaries.
Executable help owns command syntax; capability metadata owns availability, safety classification, and supported previews.
Start with [CONTRIBUTING.md](../CONTRIBUTING.md) when changing the repository.

## Operate TADX

| Guide | Use it for |
| --- | --- |
| [Getting started](getting-started.md) | Installation, authentication, environment profiles, discovery, previews, and upgrades. |
| [Workspaces](workspaces.md) | Managed artifacts, paths, status, cloning, moves, deletion, and cleanup. |
| [Security](security.md) | Credentials, site consent, local data, trust boundaries, and protection limits. |
| [Managed policy](managed-policy.md) | Machine-wide restrictions, templates, deployment, and recovery. |
| [Capability reference](reference/capabilities.md) | Generated operation inventory and contracts. |
| [Capability map](https://tadx.net/capabilities.html) | Interactive command discovery, filters, ownership, and safety requirements. |
| [Command shorthand](reference/shorthand.md) | Supported command and flag aliases. |
| [VS Code extension](../extensions/vscode/README.md) | Editor setup, inventory, artifact operations, and extension development. |

## Build and maintain TADX

Read the [architecture guide](architecture/README.md) before selecting an implementation boundary.
It connects runtime, resource and state ownership, delivery, editor integration, and source evidence.

| Reference | Use it for |
| --- | --- |
| [Repository structure](repository-structure.md) | Package ownership, dependency direction, capability generation, and source locations. |
| [Refactoring playbook](resource-refactoring-playbook.md) | Behavior-preserving resource changes and verification. |
| [Command structure](command-structure.md) | Public command organization and help design. |
| [Output contract](toon.md) | Compact TOON, JSON encoding, expansion, and partial failures. |
| [Search source map](architecture/search.md) | Native search, bounded inventories, cache selection, and query ownership. |
| [API evidence](evidence/README.md) | Upstream contracts, observations, unsupported assumptions, and evidence limits. |
| [Website publishing](website.md) | Static allowlist, capability map generation, browser checks, and explicit Pages publication. |
| [Extension releases](vscode-extension-release.md) | Separate versioning, CLI compatibility, packaging, verification, and explicit publication. |

The capability [Markdown](reference/capabilities.md), [JSON](reference/capabilities.json), and embedded data in the [map source](reference/capability-map.html) share one registry.
Refresh them with `go generate ./internal/capability`; do not edit generated metadata manually.
Bundled operating Guidance lives in `internal/agent/skills`; repository build instructions live in the explicitly invoked `tadx-build` skill.

## Historical material

Completed plans, dated validation logs, superseded diagrams, and speculative discussions belong under `archived/`.
The [2026-10-30 sweep record](../archived/docs/2026-10-30/README.md) accounts for every document examined and preserves the original archived files.
Their links and observations describe their original snapshots; use the guides above for current work.
