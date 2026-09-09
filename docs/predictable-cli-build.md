# Predictable CLI build

Review base: `f091482`.
This branch includes all earlier unmerged work through `c230183` plus the changes below.
The maintainer performs the comprehensive review in ChatGPT; implementation agents run automated tests only.

## Locked scope

- Infer the sole configured environment for reads and writes; with multiple environments, require an explicit remote-write target and never infer it from artifact provenance.
- Adding the second environment explains the new write-target requirement; missing-target errors list configured names.
- Retain named workspaces under `<user-home>/TADX/workspaces/<name>` with custom roots supported.
- Managed artifacts can be selected by name or source LUID within the selected workspace; ambiguity fails and paths remain an advanced selector.
- Native files can be published directly without manually manufacturing managed metadata.
- Compact artifact output shows useful identity and state, not hashed directories or fingerprints; expanded output exposes locations.
- Pulse bundles contain definitions, variants, and datasource references; publish recreates them with explicit datasource mapping and validates the resulting plan.
- Pulse recreation leaves originals untouched and reports identity mappings; no identity-preserving updates, followers, users, values, or insights are transported.
- Standardize primary `--id`, rename `--new-name`, related-object selectors, public datasource terminology, detail flags, and record-count flags without losing meaningful distinctions.
- Accept bounded larger requested limits where full traversal already supports them, and explicitly report truncation.
- Logout reports remaining environment-variable credentials; mutation status identifies the effective value and source.
- Persistent user mutation policy is supported; an explicitly set environment value overrides it and absence of both means disabled.
- No operational mutation setting may be changed without the maintainer's explicit permission.
- User-facing external capabilities are labeled "Out of scope" and remain separate from executable commands.
- Server-side publish job help says it waits for completion.
- Bind every catalog access to the normalized server origin and site; never infer the origin of legacy unbound data.
- Cache complete follower snapshots per metric, including empty sets; unsuccessful/incomplete reads do not replace evidence.
- Skill installation receipts distinguish untouched upgrades from user modifications, with rollback and conservative handling of unknown installations.
- `tadx last` displays one global bounded, redacted full result and timestamp without replay, authentication, IDs, or history; it never overwrites itself.

## Deferred

Shorthand command aliases and short flags will be designed separately, including collision analysis.
No shorthand implementation, VS Code extension, live Tableau mutation, comprehensive agent review, or release is part of this build.

## Implementation lanes

- Catalog lane: target isolation and catalog read/write integration.
- Content lane: workspace/artifact selection, native-file publishing, compact output, and content flag consistency.
- Pulse lane: portable bundles and complete follower snapshots.
- Coordinator: target inference, saved policy, last-result output, installer receipts, registry, integration, and Guidance.

## Acceptance

Tests exercise CLI behavior through local HTTP fixtures and isolated local files.
Run focused regression tests, full tests, race checks, vet, documentation generation, and platform builds.
Push the complete branch and provide the exact review base and head with verification limits.
