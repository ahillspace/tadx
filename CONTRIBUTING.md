# Contribute to TADX

Start with the [repository architecture](docs/repository-structure.md) to find the owner of a change.
The [contributor walkthrough](docs/contributor-walkthrough.md) traces a read, a mutation, and a local-state change from command to evidence.
The [implementation map](.agents/skills/tadx-build/references/implementation-map.md) points to focused examples and engineering references.

## Run the current source

Install the Go version specified in `go.mod`, then build a local executable:

```sh
go build -trimpath -o ./bin/ ./cmd/tadx
```

The build creates `bin/tadx` on macOS/Linux or `bin/tadx.exe` on Windows.
Run that executable to use unreleased changes from this checkout.
Release installers and `tadx update` use published releases, not this source tree.

## Make a change

Start from updated `main`, unless continuing an assigned branch or authorized stack.
Preserve unrelated local changes and coordinate shared files when multiple contributors use the checkout.

1. Identify the resource or service that owns the requested outcome.
2. Trace its command binding, workflow, provider, and shared mechanisms.
3. Reproduce a bug through the public command when practical, then add a regression test for the observed failure.
4. Implement the behavior in its owner, preserving distinct operation contracts and safety checks.
5. Update maintained documentation and regenerate marked references through their documented tools.
6. Run affected tests and the required integrated checks before handoff.

Cobra owns syntax and presentation binding; actions own outcomes; adapters own substantive provider translation; app constructs dependencies.
Do not add a forwarding layer solely to complete that sequence.
Share mechanisms when consumers have matching invariants, not merely similar function names.
Keep credentials, private configuration, downloaded Tableau content, and local test output out of commits.

## Verify the change

Start with focused tests for the changed package and its consumers.
Use the tests beside each owner to identify the concrete behavior at risk.
For example, after changing a workbook workflow, run its action and adapter tests:

```sh
go test ./actions/workbook ./internal/resources/workbook
```

Include CLI and app tests when command mapping, wiring, persistence, or output changes.
Run the complete automated suite and architecture checks before submitting the change:

```sh
go test ./...
go vet ./...
```

Required CI also checks platform behavior, race safety, builds, and generated references.
A local pass does not replace required platform jobs or authorized live verification.
For structural migrations, follow the [regression gates](docs/maintainability/regression-gates.md), including independent navigation review and exact-build live evidence.
Record exclusions and unavailable evidence; do not count them as passes.

Tests must protect a concrete observable behavior.
Keep workflow tests beside actions, protocol tests beside providers, and command integration tests at the CLI or app boundary.
Name tests for the behavior they protect, not the review phase that introduced them.
Do not retain obsolete production wrappers solely to satisfy internal tests.

## Work with a coding agent

The repository-local [tadx-build skill](.agents/skills/tadx-build/SKILL.md) contains maintained implementation standards and verification references.
Explicitly request it for a named task and state the outcome, scope, and stopping point:

```text
$tadx-build Build [the action you need]. The user outcome is [outcome].
Include [scope], exclude [scope], and stop after implementation and automated tests.
```

If the agent does not discover repository-local skills, provide the skill path and request its use for that task.
The maintainer controls agent-based reviews; a build request does not authorize a push, pull request, merge, or release.

## Explore the installed CLI

Use the repository-local [tadx-explore skill](.agents/skills/tadx-explore/SKILL.md) for bounded exploration of an installed CLI category, resource, or verb.
Follow its fixture, evidence, and cleanup requirements.
Exploration does not authorize changing saved mutation consent or persisting credentials.
