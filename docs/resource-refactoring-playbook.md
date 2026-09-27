# Refactor a resource without changing CLI behavior

Use this playbook to replace unnecessary per-verb scaffolding with cohesive resource operations.
Assess related resources together before choosing shared code.
Migrate one resource at a time, and revise this playbook from tested results.

## Scope and ownership

For content, assess workbook, datasource, and flow together.
Group each resource's operations in its own action package.
Keep substantial pull and publish workflows explicit rather than creating one configurable lifecycle engine.
Other categories retain their existing package layout until separately assessed.
Project also receives a bounded cleanup audit without requiring package regrouping.
Admin membership and permission mutations share cohesive packages after assessment; distinct inspection contracts remain separate.
Admin user, group, label category, and label value lifecycle operations use one cohesive package per resource after a complete caller assessment.
Pulse definition and metric operations use resource packages; environment profiles share a profile package.
Auth and capability operations retain verb packages, with small contracts and projections beside the operation that owns them.
Catalog database, table, and column updates share metadata rules in one package while retaining separate mutation sequences.
Their lists and inspections share a focused read package; search and audit retain separate traversal contracts.
Workspace operations share one action package and one app-owned runtime adapter, with narrow dependencies for each operation.
The workspace manager and artifact packages retain their distinct filesystem responsibilities.

Resource operations own input rules, planning, resource-specific decisions, error context, and output projections.
The composition root owns environment, authentication, policy integration, cache selection, workspace selection, and receipt persistence.
Resource adapters own normalized Tableau access; Tableau clients own HTTP and API representations.
Artifact code owns package handling, safe replacement, and filesystem containment.
Existing paging and identity packages own matching shared mechanisms.
Do not move responsibilities merely to match the CLI command hierarchy.

## Establish the baseline

1. Record the source revision, existing changes, and a recoverable source snapshot or patch.
2. Build a pinned baseline executable without replacing the installed CLI.
3. Record every implemented verb, callers, shared consumers, and meaningful output or mutation differences.
4. Run existing affected tests and save baseline CLI task evidence.
5. Count production files, packages, and physical lines separately from tests, generated files, and documentation.

## Preserve the external contract

Preserve flags, defaults, exact selection, command identifiers, output fields, omission rules, ordering, errors, and recovery commands.
Preserve cached payloads, stored receipts, and artifact metadata as well as terminal output when consolidating record types.
Preserve HTTP request sequencing, completeness bounds, preview behavior, policy enforcement, partial results, and uncertain mutation outcomes.
Do not standardize differences between resources during a behavior-preserving refactor.
For example, workbook and flow publication place revalidation on different sides of preparation.
Potential defects discovered during comparison require a separate finding rather than an incidental behavior change.

## Refactor and remove unnecessary code

1. Assign one owner to each fact and responsibility before moving code.
2. Consolidate verb packages into the resource package, retaining explicit operation names and focused files.
3. Reuse an internal resource record and resolver where the meanings match.
4. Remove wrappers that only translate between equivalent verb-owned representations.
5. Remove duplicate checks only when the same unchanged fact is already established.
6. Remove unused helpers, forwarding interfaces, duplicate structures, and obsolete test scaffolding.
7. Consolidate small files when their contents serve the same responsibility.
8. Review the resulting call path and remove any indirection introduced solely by the migration.

Different external projections can coexist with one internal resource record.
Do not expose fields accidentally by serializing a newly shared internal record directly.
Keep interfaces that isolate real external dependencies or provide useful test seams.
Stateless operations can accept those interfaces directly instead of storing them in constructor-only action objects.
A single consumer is a reason to examine an interface, not automatic evidence that it is unnecessary.
Recheck fresh API responses and mutable files at their actual trust boundaries.
Do not remove a reread merely because it resembles an earlier read.

For every proposed removal, identify the callers and the responsibility that remains elsewhere.
For repeated checks, establish whether the checked fact can change between them.
Existing patterns and passing tests do not by themselves justify an abstraction.
Trace production callers before treating direct calls from unit tests as a separate trust boundary.
An internal composition method reached only after CLI validation need not revalidate unchanged input fields.
Preserve error ordering on real command and batch paths rather than adding tests for unsupported entrypoints to justify repeated checks.
Argument counts do not establish nonempty positional values; trace what each parser check actually guarantees before removing action validation.
Treat simplification evidence separately from correctness review; both are needed to assess the result.

## Share mechanisms deliberately

Share code when at least two real consumers have the same invariant and execution contract.
An adapter can embed a normalized client when its forwarded methods add no behavior; retain explicit methods that translate resource-specific scope.
Prefer existing focused packages over a new generic utility layer.
Keep resource-specific fingerprint inputs, errors, outputs, and mutation sequences with their operations.
Avoid resource-kind switches, configuration matrices, or callback-heavy frameworks that conceal different workflows.
If resource boundaries require artificial shared APIs and forwarding layers, reconsider the boundary before extending it.

## Keep tests useful

Move and adapt existing tests with the implementation.
Preserve behavioral assertions, failure cases, fixture coverage, and golden expectations.
Rename conflicting test helpers rather than deleting their tests.
Add focused characterization assertions only for uncovered behavior at risk.
Record any removed test and why its implementation-only contract no longer exists.
Do not keep compatibility packages solely to avoid updating tests or internal imports.

Run affected operation, adapter, CLI, and composition tests after each resource migration.
Test shared consumers when a shared representation or mechanism changes.
Use the exploration skill with fresh Luna agents and pinned baseline and candidate executables.
Repeat actual user tasks, record exact commands and outcomes, and distinguish reads, previews, and completed writes.
Live fixture writes require explicit authorization; never treat previews as evidence of mutation execution.

## Review and advance

The first resource requires a code review before migrating the next resource.
Fix actionable findings, repeat affected tests, and update this playbook with the lessons.
Subsequent resources follow the validated approach, not a mechanical directory-flattening script.
Finish the integrated change with the complete existing suite, vet, formatting, and risk-focused race tests.
Use a clean source snapshot when unrelated local experiments interfere with repository scans; do not weaken the scans.

Completion requires fewer unnecessary concepts and handoffs, not merely fewer files.
Report actual file, package, and line changes, separating relocation and boilerplate removal from removed logic.
Record preserved boundaries, remaining duplication, test results, review findings, and verification limits.
Do not push, merge, release, or replace installed binaries without separate authorization.

## Lessons from the workbook pilot

Review new, untracked package files against the recoverable baseline; ordinary `git diff` does not include them.
Verify cached record JSON separately from each operation's external projection, including field order, empty values, and diagnostic omission.
Preserve explicit project-resolution phases around preparation and commit rather than sharing a snapshot across those phases.
When replacing constructors with functions, preserve dependency evaluation order as well as the operation body.
Mechanical migration tools can update AST symbols and imports, but output contracts and test-helper collisions still require deliberate inspection.

## Lessons from datasource integration

A richer shared record must not broaden an existing mutation's identity comparison.
Compare the same fields as before, and test changes to unrelated metadata between reads.
Keep optional enrichment separate from base records when operations expose different observations.
Exercise shared consumers, including cache readers and Pulse schema consumers, after changing datasource representations.

## Lessons from flow integration

Treat cache serialization as a separate contract, even when terminal output remains unchanged.
An enriched record can change a later cached list by adding a field that the original detail writer never stored.
Reuse the original unbounded detail projection for cache writes; do not cache the truncated full-output projection.
Test absent fields, required empty fields, and collections larger than display limits.

## Lessons from project cleanup

Remove a forwarding adapter only after tracing its callers and confirming that the remaining client owns equivalent request checks.
Retain adapters that normalize mutation results or preserve path and index-delay behavior.
Distinguish pre-connection input checks from checks of the resolved environment and site; these inspect different facts.
Within one action, reuse parsed cursor state rather than validating it and then parsing the same input again.
Move small validation and phase helpers beside their operation without removing fresh-state boundaries.
Retire tests that only exercise a deleted forwarding hop, and identify where their meaningful assertions remain covered.

## Lessons from the next categories

Do not replace a partial-result traversal with a collector that discards observations on error.
Keep resource selectors, operation errors, and output projections outside a shared paging mechanism.
Declare any new shared-value dependency in the architecture allowlist and retain tests rejecting higher-layer imports.
Removing a JSON round trip can also remove a copy boundary; verify serialized payloads and ownership of nested values.
Shared cursor encoding must preserve existing tokens while resource-specific fingerprints and resolved-target checks remain explicit.
Preserve explicit add/remove and create/delete operations even when their records and validation share an owner.
An assessed category can retain its package layout when regrouping would only rename types and move different projections.

## Lessons from operation file consolidation

Different output contracts do not require different packages or separate files for every type and validator.
Colocate small contracts with their operation without removing narrow dependency interfaces or hiding the operation sequence.
Use shared observations only where field meanings match; preserve unknown values and operation-specific output omissions.
Sharing a profile type must not make listing apply effective defaults that only inspection previously resolved.
Do not make a delete depend on configuration parsing that only inspection previously required.
Keep raw bundle construction and cache observation capture separate from terminal projections.
When small files can be combined within an existing package, avoid an unnecessary package migration.
Report file consolidation separately from eliminated logic and duplicate representations.
Capture stdout, stderr, exit codes, and exact arguments from the first baseline call.
Use matching cache preconditions in before/after tests, and record fixture-specific differences explicitly.

## Lessons from catalog updates

Shared metadata rules do not require a shared mutation runner.
Keep resource-specific identity checks, write ordering, partial receipts, and recovery commands beside each operation.
Tag planning must preserve observation state, stable deduplication, request order, and non-null empty change arrays.
Test those shared rules before extracting them, and preserve each resource's clearing and parent-identity requirements.

## Lessons from workspace workflows

A cohesive action service can remove per-verb constructors and composition-root forwarding methods without combining operation sequences.
Keep narrow operation dependencies even when one runtime implements them all.
Shared workspace identity records must not broaden create, register, list, or deletion output projections.
Private plan and apply helpers do not require public misuse contracts, but fresh filesystem checks still protect mutable state.
Distinguish lexical containment of fixed path segments from symlink and filesystem containment checks.
Test local lifecycle changes through the CLI with an isolated configuration and disposable workspaces; remote API calls provide no additional evidence for those operations.

## Lessons from Admin workflows

Trace direct search callers as well as resource commands before removing action-level input checks.
Preserve pre-authentication validation where CLI argument handlers establish only part of the input contract.
Retain parsed cursors and normalized desired members instead of discarding those results and computing them again during execution.
Resolved-target cursor binding remains a separate check; reuse the parsed token without skipping that binding.
A normalized client can own per-response identity and pagination checks, while its resource adapter retains cross-page conflicts and capability authorization.
Keep separate remote rereads and readback verification even when their comparison helpers are shared.
Do not broaden mutation equality or cache payloads when replacing per-verb records with a shared observation.
Attach the selected binary, configuration, fixture authority, and replay task to the exploration manifest before starting a worker.
