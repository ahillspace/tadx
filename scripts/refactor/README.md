# Initial refactor contract gates

This bounded offline runner checks source provenance, selected G1-G4 contracts, and nine isolated seeded defects.
It does not establish complete G0-G9 acceptance or authorize live operations.
The approved regression-gate specification remains authoritative.

## Run the checks

Use a clean source snapshot extracted from the approved Git revision, not the working checkout.
The snapshot must contain the tracked files, without `.git`, symlinks, credentials, or unrelated files.
Materialize tracked symlinks as regular files containing their exact link text; never follow them.
The runner compares its file contents with Git objects from `--repo`.
Use the gate manifest from the same checkpoint as the source.
Current manifests follow relocated owners and do not necessarily run against the original baseline.
It rejects undeclared additions, removals, and modifications.

```powershell
python -B scripts/refactor/run.py --source <snapshot> --repo . --revision <approved-revision>
```

For a candidate snapshot, repeat `--allow-change <relative-path>` for each approved changed file or directory.
An allowed path does not waive behavioral checks.
Changed snapshots have no claimed candidate commit; their complete source manifest and SHA-256 identify the tested content.
The runner rejects source changes detected during execution.
Create isolated snapshots before execution; this stability check is not a filesystem lock.

Evidence defaults to a new `tadx-refactor-gates-*` directory in the operating system's temporary directory.
Use `--output <new-directory>` to choose another destination outside the snapshot.
The runner never overwrites an existing evidence directory.
`summary.json` records source and fixture hashes, toolchain, candidate binary hash, commands, durations, and results.
`source-manifest.json` records repository-relative file hashes.
Separate command logs retain test evidence with local roots redacted.
No inherited API keys, PAT variables, user configuration, Go flags, or proxy configuration reach test subprocesses.
Go uses existing module caches with network module lookup disabled.
Only fake local services and test-owned persistence are involved in these selected tests.

## Interpret the result

`initial_checks_passed` means every configured test passed and every configured defect triggered its expected assertion.
It is not checkpoint acceptance.
Missing tests, skipped tests or subtests, timeouts, compilation failures, malformed evidence, and unexpected failures block the run.
Seeded runs require successful unmodified controls, a compiled test execution, and the designated assertion failure.
An unrelated panic or build failure cannot count as successful detection.
Exact replacement anchors fail closed when source changes; update anchors deliberately as operations move.
Seeds use temporary Go overlays and never modify the snapshot or call a live service.

The initial demonstrations cover missing and extra output fields, wrong success exit, preview writes, identity drift, false-success reporting, skipped restoration, incorrect restored-reference outcome, and incorrect restore-installation classification.
The manifest names each contract, existing test, mutation location, and expected assertion.
The optional `--manifest scripts/refactor/reporting-gates.json` checks the separately approved publication reporting correction and demonstrates detection of both reproduced defects.
That manifest requires the corrected source and its new tests; it intentionally does not pass against the original baseline.
The optional `--manifest scripts/refactor/baseline-fixes-gates.json` checks the separately approved flow request identity, project-create identity, and interrupted-publication receipt recovery corrections with three isolated defect seeds.
Existing tests retain their limitations: JSON parseability is not an exact key-set assertion, and fake credential storage is not a native keyring test.
The portable G4 restore tests distinguish configuration states and detect incorrect reporting; a separate Windows-only fake-store CLI test exercises the pre-replacement orphan path.
Unix durability scenarios, architecture conformance, full CI/platform checks, navigation acceptance, and live coverage remain outstanding requirements.
This runner does not replace those checks or the existing benchmark harness.

## Test the runner

The `Refactor gate tooling` workflow runs these offline unit tests, the live evidence validator tests, the accepted CLI capture tests, and the harness preparer tests on Linux and Windows.
It does not contact Tableau or a model provider and does not replace candidate contract, native platform, or live gates.

```powershell
python -B -m unittest discover -s scripts/refactor -p "test_*.py" -v
```
