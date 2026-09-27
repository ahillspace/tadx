# Contribute to TADX

Use the repository-local [tadx-build skill](.agents/skills/tadx-build/SKILL.md) for implementation standards, architecture guidance, examples, and verification.
Select the skill's guidance that matches your change.

## Start with your agent

Ask your coding agent to use the build skill for a named task and state the outcome, scope, and stopping point.
For example:

```text
$tadx-build Build [the action you need]. The user outcome is [outcome].
Include [scope], exclude [scope], and stop after implementation and automated tests.
```

If your agent does not discover repository-local skills, give it the explicit entry point:

```text
Read .agents/skills/tadx-build/SKILL.md and use it to build [the action you need] in this repository.
```

## Run the current source

With the Go version specified in `go.mod` installed, build a local executable:

```sh
go build -trimpath -o ./bin/ ./cmd/tadx
```

Run the resulting executable to use unreleased changes from this checkout.
The build creates `bin/tadx` on macOS/Linux or `bin/tadx.exe` on Windows.
Release installers and `tadx update` use published releases, not this source tree.

## Keep changes maintainable

- Keep command plumbing, action behavior, Tableau adapters, and composition wiring within their established responsibilities.
- Preserve distinct operation contracts and mutation sequences, and share mechanisms only when their behavior matches.
- Keep user-facing behavior documented alongside the source of truth, and regenerate generated references with their documented tools.
- Keep credentials, private configuration, downloaded Tableau content, and local test output out of commits.

Use the build skill's [implementation map](.agents/skills/tadx-build/references/implementation-map.md) to find focused references and examples.
Start new work from updated `main`, unless continuing an assigned branch or authorized stack.
Coordinate shared files when multiple contributors work in the checkout.
Use the build skill's verification checklist before handoff.
The maintainer runs agent-based reviews; a build request does not authorize a push, pull request, merge, or release.

## Explore the installed CLI

Use the repository-local [tadx-explore skill](.agents/skills/tadx-explore/SKILL.md) for bounded exploration of a selected installed CLI category, resource, or verb.
