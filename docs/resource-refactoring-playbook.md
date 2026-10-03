# Refactor action boundaries without changing CLI behavior

Use this playbook when an action package has redundant representations, checks, or handoffs.
Assess a whole command category and its shared callers before choosing a package boundary.
A cohesive resource package is useful when operations share records, validation, or mechanisms.
Keep separate packages when regrouping would only relocate code or merge distinct contracts.
Do not treat the command tree as the required package tree.

## Identify ownership

Assign one owner to each fact and responsibility before moving code.
Choose one owner for each validation, normalization, error, preview, and output rule; add only the layers an operation needs.
Keep operation entry points, projections, plans, mutation sequences, errors, and recovery context explicit.
Resource actions own input rules and resource decisions.
The composition root wires environment, authentication, policy, cache, workspace, and receipt dependencies.
Each operation keeps its cache-versus-live decision at its current owner.
Adapters implement narrow action-owned ports and normalize Tableau access; clients own HTTP and API representations.
Artifact and workspace packages own their filesystem and locking boundaries.
Existing identity and paging packages own matching shared mechanisms.
Keep narrow dependency interfaces where they isolate external behavior or support meaningful failure tests.
Stateless operations can receive those interfaces directly when an action object adds no responsibility.

Group related operations when a shared owner removes redundant handoffs.
Keep distinct operations even when they share a record, validator, or helper.
For example, a shared metadata rule does not imply a shared mutation runner.
Keep substantial preparation, commit, and verification phases visible rather than hiding them in a configurable lifecycle engine.
Share a mechanism only when real consumers have the same invariant and execution contract.
If a shared API needs resource-kind switches, forwarding layers, or callback-heavy configuration, reassess the boundary.
Colocate small contracts and helpers with their operation when a package move adds no value.

## Establish comparable evidence

1. Record the source revision, existing changes, and a recoverable source snapshot or patch.
2. Build a pinned baseline executable without replacing the installed CLI.
3. Inventory implemented commands, direct callers, shared consumers, and their external contracts.
4. Run affected tests and capture baseline CLI tasks, including exact arguments, stdout, stderr, and exit codes.
5. Count production packages, files, and physical lines separately from tests, generated files, and documentation.

Use matching configuration, cache state, and fixture preconditions for baseline and candidate comparisons.
Record fixture-specific differences instead of treating them as behavior changes.
When live comparison is appropriate, use the exploration workflow with fresh Luna medium agents, pinned executables, and authorized fixtures.
Attach the selected executable, configuration, fixture authority, and replay task to the exploration manifest.
Distinguish reads, previews, and completed writes; previews do not prove mutation execution.
Use isolated configuration and disposable workspaces for local lifecycle tasks.

## Preserve observable contracts

Preserve flags, defaults, selectors, command identifiers, output fields, omission rules, ordering, errors, and follow-up commands.
Preserve stdout, stderr, exit codes, JSON field order where it is part of serialized output, and nil-versus-empty distinctions at exposed boundaries.
Check terminal output, cache payloads, saved results, receipts, artifacts, and durable operation records separately.
An internal record can serve several operations, but each operation retains its own external projection.
Do not serialize a richer shared record directly when it broadens a cache or output contract.
Keep optional enrichment apart from base observations when operations previously exposed different fields.
Preserve unknown values and confirmed partial results.
Do not replace a partial-result traversal with a collector that discards observations on error.

Preserve HTTP request order, completeness bounds, preview behavior, policy checks, authorization prerequisites, and uncertain mutation outcomes.
Keep resource-specific identity comparisons at their original scope, even if a new shared record contains more fields.
Keep list, detail, search, and audit traversal contracts distinct when their errors, paging, or partial results differ.
Preserve cursor bytes and continuation fingerprints, including inputs that appear redundant in other code paths.
Keep resource-specific fingerprints, resolved-target binding, and upstream cursor extraction explicit.
Preserve cache write projections independently of bounded terminal projections.
For example, a detail cache can require unbounded collections even when `--full` truncates display.
Check exact generated artifact bytes, stable deduplication, request order, and empty collection encoding where consumers depend on them.
Removing a JSON round trip can remove a copy boundary; protect mutable nested values where ownership requires it.

A behavior-preserving refactor does not standardize differences between resources or repair unrelated defects incidentally.
Record a discovered defect separately, then make any correction as a deliberate change with its own evidence.

## Simplify with boundary evidence

Trace production and shared callers before removing an interface, adapter, field, validation, or copy.
For each removal, identify the surviving owner and the contract it preserves.
Remove wrappers that only translate equivalent representations or forward calls without behavior.
Retain adapters that normalize responses, enforce request checks, or preserve resource-specific scope and timing.
Remove duplicate validation only when the same fact remains unchanged between checks.
Argument count checks do not establish nonempty positional values.
Internal construction does not replace strict parsing at external-file or privileged-process boundaries.
Use validated and parsed results instead of recomputing them at later handoffs.

Treat independent observations as separate trust boundaries.
Recheck mutable files, remote state, resolved environment, and site identity when the operation reaches them.
Keep post-prompt resolution, locked rereads, pre-replacement checks, and readback verification where their timing matters.
A check before connection can protect a different fact from a check after environment resolution.
Keep authorization and plan agreement checks where independently prepared requests meet.
Removing a repeated permission check requires evidence that the surviving check records the complete actual scope before requests begin.
Do not merge different default scope policies or move a denial across environment resolution to share preparation.
Preserve failure order on real CLI and batch paths.

Keep filesystem lexical containment separate from symlink and actual filesystem containment.
Keep fresh schema, configuration, cache, credential, and remote-state checks where state can change.
Distinguish local readiness from authenticated access or native credential availability.
Discovery metadata describes permission state but does not authorize execution.

## Migrate tests and verify

Move and adapt existing tests with the implementation.
Preserve behavioral assertions, failure cases, fixtures, and golden expectations.
Rename conflicting test helpers rather than deleting their tests.
Retire tests of removed internal wiring only after identifying where their external assertions remain covered.
Do not retain compatibility packages or parallel duplicate suites solely to avoid updating tests.
Add focused characterization tests for uncovered contracts at risk, including caches, partial errors, mutable copies, and fresh-state checks.
Test shared consumers when a representation changes, including cache readers and other resource actions.
Use actual CLI failure paths for command contracts rather than unsupported direct entry points.

Run affected action, adapter, CLI, and composition tests after each bounded migration.
Finish the integrated change with the existing suite, vet, formatting, and risk-focused race tests.
Use a clean source snapshot when unrelated local experiments interfere with repository scans; keep enforcement gates intact.
Review new untracked files against the source snapshot because ordinary `git diff` excludes them.
The maintainer runs agent-based code reviews separately; do not add a mandatory review-agent gate to this workflow.

## Report the result

Completion means fewer unnecessary concepts and handoffs, with observable behavior and independent boundaries preserved.
Report actual package, file, and line changes without an arbitrary line-count target.
Separate relocation and file consolidation from eliminated logic or duplicate representations.
Record remaining duplication, behavioral findings, tests, CLI comparisons, and verification limits.
Do not push, merge, release, or replace installed binaries without separate authorization.
