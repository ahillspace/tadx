# TADX maintainability assessment

Status: baseline, pilot, and repository-wide structural proposal available; architecture approval, detailed migration verification, and implementation remain pending.
Read the [architecture proposal](maintainability/architecture-proposal.md) for the concrete target and [package map](maintainability/architecture-decision-map.json) for exact dispositions.
The [pilot report](maintainability/pilot-report.md) remains the original bounded evidence, not the final design decision.
Reference selection and repository-wide consistency are agreed priorities.
The scope permits substantial internal redesign while preserving supported CLI behavior.
Do not preserve an unnecessary boundary merely because earlier refactoring established it.

## Objective

An unfamiliar Go maintainer can locate a command, explain its behavior and dependencies, change it safely, and identify tests that detect relevant failures.
The repository must teach one coherent architecture rather than require category-specific architectural knowledge.
These criteria support a potential maintainer handoff; they do not establish Tableau endorsement or adoption requirements.

## Consistency is an acceptance criterion

Choose one repository-wide convention for each of these concerns before production migration:

| Concern | Required consistent convention |
| --- | --- |
| Navigation | A documented mapping from command name to implementation, tests, and shared dependencies |
| Command boundary | Where parsing, help, defaults, and command invocation belong |
| Workflow ownership | Where resource rules, operation sequencing, and outcome verification belong |
| Dependencies | How dependencies are constructed, passed, and replaced in tests |
| Representations | Who owns internal records and external projections, and when conversion is necessary |
| Outcomes | How success, refusal, partial completion, and uncertain effects are represented |
| Tests | Where each test level belongs, how it is named, and which contract it protects |
| Documentation | Where command facts, contributor instructions, and architectural decisions are maintained |

Categories can differ in behavior without using unrelated architectural patterns.
For example, different resource identity checks do not inherently require different wiring conventions.
An existing layout, different author, or completed historical refactor is not justification for an exception.
Exceptions require concrete behavioral evidence, explicit maintainer approval, and a recorded explanation at the relevant boundary.
Exceptions must remain narrow rather than exempt entire categories from the common model.
Temporary old/new structures require a migration entry and an exit condition; they are not the final architecture.
Completion requires every category to conform or have an approved exception.

## Pinned reference baseline

| Project | Revision | Role |
| --- | --- | --- |
| TADX | `dd22c33bd1d123066f25ec09e2d612cd95717400` | Assessed inventory and pilot baseline |
| GitHub CLI | `6fc1c29d5477bfe71da7af290eb481c0df7811f1` | Contributor navigation, command structure, and behavioral tests |
| Helm | `53dfa521e019f7b832497066032406b9cda9d5d4` | Stateful action boundaries and failure-to-state tests |

The TADX baseline is the fetched upstream revision, not the older working checkout or its local experiments.
Refresh the pin deliberately if assessment starts from a newer revision; do not mix findings from unlabelled snapshots.
Reference observations below come from source inspection, not executed tests.
Neither reference is a flawless template or a mandate to reproduce its directory tree.

## Reference examples

### GitHub CLI: read a remote resource

`gh issue list` places options, Cobra construction, and the execution function in a discoverable command package.
`ListOptions` makes dependencies visible; `NewCmdList` binds flags; `listRun` handles execution and rendering.
The tests distinguish terminal and nonterminal output and assert query values, not only successful execution.
See the pinned [implementation](https://github.com/cli/cli/blob/6fc1c29d5477bfe71da7af290eb481c0df7811f1/pkg/cmd/issue/list/list.go#L25) and [query assertions](https://github.com/cli/cli/blob/6fc1c29d5477bfe71da7af290eb481c0df7811f1/pkg/cmd/issue/list/list_test.go#L167).

Adopt predictable navigation, explicit dependencies, and contract-specific assertions.
Do not infer that TADX must place all workflow logic inside Cobra packages.

### GitHub CLI: mutate a remote resource

`gh issue close` has a traceable path from command construction through target lookup, optional comment creation, mutation, and success output.
Tests assert target IDs, comment payloads, closure reasons, and duplicate-target validation.
See the pinned [workflow](https://github.com/cli/cli/blob/6fc1c29d5477bfe71da7af290eb481c0df7811f1/pkg/cmd/issue/close/close.go#L91) and [tests](https://github.com/cli/cli/blob/6fc1c29d5477bfe71da7af290eb481c0df7811f1/pkg/cmd/issue/close/close_test.go).

Adopt explicit operation order and assertions about the actual mutation request.
Do not copy its verification strength blindly: this mutation requests an ID and does not independently read back the final issue state.
Comment creation also precedes closure, so a later close failure can leave a partial effect.
The inspected test file does not demonstrate recovery from that combination.
TADX must retain the affected-state evidence required by its own operation contracts.

### Helm: execute a stateful lifecycle action

The install command constructs an install action, binds flags, invokes execution, and renders the result.
Action tests use in-memory release storage and controllable Kubernetes behavior.
Dry-run tests assert that no release is stored; wait-failure tests assert failed release state.
See the pinned [command](https://github.com/helm/helm/blob/53dfa521e019f7b832497066032406b9cda9d5d4/pkg/cmd/install.go#L132), [test dependencies](https://github.com/helm/helm/blob/53dfa521e019f7b832497066032406b9cda9d5d4/pkg/action/action_test.go#L52), and [state assertions](https://github.com/helm/helm/blob/53dfa521e019f7b832497066032406b9cda9d5d4/pkg/action/install_test.go#L612).

Adopt tests that connect failure injection to an observable outcome.
Inspect fake fidelity: an [ownership test limitation](https://github.com/helm/helm/blob/53dfa521e019f7b832497066032406b9cda9d5d4/pkg/action/install_test.go#L221) explicitly notes that its fake cannot prove the ownership change.
Passing a test is not proof of effects the fake never models.

### Contributor guidance

GitHub CLI documents command-to-file mapping and traces an example execution in its [project guide](https://github.com/cli/cli/blob/6fc1c29d5477bfe71da7af290eb481c0df7811f1/docs/project-layout.md).
Its [command guide](https://github.com/cli/cli/blob/6fc1c29d5477bfe71da7af290eb481c0df7811f1/docs/command-development.md) specifies a repeatable development pattern.
Its [testing guide](https://github.com/cli/cli/blob/6fc1c29d5477bfe71da7af290eb481c0df7811f1/docs/testing.md) distinguishes test boundaries and isolates ordinary tests from live resources.

GitHub CLI uses per-command packages; Helm separates commands from lifecycle actions.
Select a coherent TADX model after comparing complete workflows, rather than importing different reference patterns into different categories.
Document the chosen model with a read, a mutation, and a local-state example.

## Assessment evidence

Inventory production code, tests, fixtures, tooling, generated files, and maintained documentation separately.
Review complete workflows and shared consumers rather than judging files independently.
Use a coverage ledger with stable category and test identifiers so every area is accounted for.
Track review status separately from whether changes are needed.

Each production finding records its revision, source locations, observed behavior, maintainer impact, proposed owner, alternatives, and verification requirements.
Distinguish correctness defects, unnecessary complexity, structural inconsistency, and optional preferences.
Record a specific reference pattern when it supports the recommendation.
Counts of files, lines, interfaces, or coverage are supporting measurements, not quality targets.

Each test or coherent test group records:

- The protected contract and realistic failure mechanism.
- The assertion that detects the failure and the boundary exercised.
- The evidence modeled by fixtures or fakes, including limitations.
- Distinct coverage, dependencies, and relevant platform requirements.
- A disposition: keep, strengthen, consolidate, relocate, or remove.
- Replacement coverage before removal, or retain pending evidence when uncertain.

Rare security and data-loss failures remain important even when difficult to trigger.
Names based on development phases do not establish redundancy or justify removing coverage.
For selected critical contracts, controlled local fault injection can verify that assertions detect the intended defect.
Keep fault injection isolated from production source, operator state, and external services.

## Execution and decision gates

1. Establish the pinned inventory and existing verification state without including unrelated local experiments.
2. Pilot Project mutation, Workbook mutation, and authentication/configuration persistence using the same assessment rubric.
3. Review the pilot with the maintainer and agree on the target conventions and exception policy.
4. Assess all remaining categories and shared systems, reconciling recommendations against the same conventions.
5. Approve a repository-wide target map and ordered migration plan before production changes.
6. Implement the required regression gates and demonstrate that controlled seeded defects fail them before production migration.
7. Implement bounded migrations with characterization tests, preserved contracts, focused checks, and broader integration verification.
8. Verify consistency across all categories and repeat independent maintainer-navigation exercises.
9. After the preceding gates pass, require Luna at medium reasoning effort to live-test every executable action before final acceptance.

Each migration checkpoint also requires live tests for every affected action after its preceding gates pass, including actions affected through shared dependencies.
The final candidate requires a complete command/action sweep, not only tests from earlier checkpoints.
Use the bounded fixtures, authorization, evidence, and exclusion rules in the regression-gate specification.

The [regression-gate specification](maintainability/regression-gates.md) defines the proposed evidence and approval requirements.
Its additional gates remain unimplemented; no finite test suite guarantees zero regressions.

Assign independent assessment areas in parallel, with one coordinator owning cross-cutting decisions.
Do not let parallel work create category-specific architectures or competing shared types.
Track unrelated defects separately from behavior-preserving migrations.
Treat instruction or architecture-checker changes as explicit design decisions, not obstacles to bypass.

Each migration specifies included paths, external contracts, necessary test changes, required checks, recovery approach, and non-goals.
Preserve authorization, target identity, fresh-state checks, secret protection, and confirmed partial outcomes.
Preserve CLI flags, output contracts, persistence formats, and compatibility unless a separate change is approved.
No live Tableau mutations, model-task reruns, pushes, releases, or installed-binary replacement are part of this assessment.

## Completion evidence

A fresh reviewer must locate representative commands, explain their safety boundaries, identify failure tests, and describe the files and checks required for a change.
Use equivalent tasks across categories to detect inconsistent conventions.
Record wrong turns, unexplained handoffs, unnecessary edits, and verification gaps rather than relying on a subjective simplicity score.
Document remaining limitations and approved exceptions.

The durable outputs are an assessment report, a coverage/decision ledger, and an approved target structure with migration acceptance criteria.
Do not mark the work complete while category coverage, structural decisions, or required verification remains unresolved.

## Current progress

- Reference projects selected and revisions pinned.
- Read, mutation, and stateful-action examples inspected without running their tests.
- Repository-wide consistency recorded as an acceptance criterion.
- Committed-tree inventory complete with reproducible [file and test declaration evidence](maintainability/inventory-dd22c33.json).
- Project mutation, Workbook mutation, and authentication/configuration pilots complete as bounded source reviews, not executed tests.
- [Pilot report](maintainability/pilot-report.md), [coverage ledger](maintainability/coverage-ledger.json), and [draft regression gates](maintainability/regression-gates.md) available for approval.
- Concrete architecture proposal covers all 123 current non-test Go package directories and ownership splits for all 42 app source files.
- [Content](maintainability/content-domains-assessment.json), [remote domains](maintainability/remote-domains-assessment.json), [local services](maintainability/local-services-assessment.json), and [shared architecture](maintainability/shared-architecture-assessment.json) provide scoped supporting evidence.
- Structural accounting is not a completed function-by-function or test-by-test audit; each migration slice still requires its contract and shared-consumer verification.
- Target approval, detailed migration acceptance evidence, gate implementation, and production migration remain pending.
