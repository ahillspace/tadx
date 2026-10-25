# Qualify selected CLI contracts against Tableau

This suite runs the selected TADX executable against a real Tableau Cloud site or Tableau Server site.
It qualifies 130 named core command contracts through process-level commands, independent API observations, native artifact inspection, and exact cleanup ownership.
It does not replace the offline suite or enumerate every flag combination.

Use a small explicit selection after a change.
The runner has no default full-suite run.
Ordinary `go test ./...` tests the harness and scenario verifiers locally; it does not contact Tableau.
Live execution requires the dedicated runner phases or the `livecli` build tag with an explicit test selector.

## Find a test

[The catalog](catalog.json) records each test's stable ID, Go test name, group, capability IDs, owners, commands, assertions, and cleanup contract.
[The exclusions ledger](exclusions.json) accounts for capabilities that remain offline, unsupported, or delegated outside TADX.
Neither file contains target names or credentials.

Stable IDs use `live.<resource>.<behavior>`.
For example, `live.workbook.inspect` maps to `TestLiveWorkbookInspect`.
Hyphenated behavior names identify a distinct contract, such as `live.workbook.pull-dependencies`.
An ID remains stable when its implementation or fixture target changes.

Inspect the complete catalog without network access:

```powershell
go run ./tests/live/runner catalog --out .tadx-live\catalog.json
```

Create a surgical selection using exact IDs, capabilities, groups, or changed owner paths:

```powershell
go run ./tests/live/runner plan --id live.workbook.inspect --out .tadx-live\selection.json
go run ./tests/live/runner plan --capability workbook.update --out .tadx-live\update-selection.json
go run ./tests/live/runner plan --group admin-group --out .tadx-live\group-selection.json
go run ./tests/live/runner plan --changed-path internal/tableau/workbook/client.go --out .tadx-live\affected.json
go run ./tests/live/runner plan --changed-from main --out .tadx-live\branch-selection.json
```

Repeat a selector flag to select several values.
Selectors form a union, and the plan records the reason for every selected test.
A saved `--selection` cannot be combined with other selectors.
An unknown ID, capability, group, or unmapped source owner fails selection.

Changed-path selection follows the catalog's declared owners and shared consumers.
Shared runtime, authentication, configuration, output, transport, and live-harness changes can produce a broad selection.
Documentation changes and ordinary offline test-only changes do not select live cases.
`--changed-from` includes tracked Git differences and untracked files.
Inspect the resulting plan before setup; a path-based selection does not claim that unselected behavior passed.

## Prepare the local tools

Build the candidate from the source you intend to qualify:

```powershell
New-Item -ItemType Directory -Force .tadx-live\candidate | Out-Null
go build -trimpath -o .tadx-live\candidate\tadx.exe ./cmd/tadx
```

Use the platform's executable name when running on macOS or Linux.
The explicit `--binary` identifies the candidate; the runner does not substitute an installed release.

Core workbook and datasource fixtures use Python with the Tableau Hyper API:

```powershell
python -m venv .tadx-live\python
.tadx-live\python\Scripts\python.exe -m pip install tableauhyperapi
```

Setup generates deterministic native fixtures when the selected cases need them.
It never installs Python packages automatically.
Use `--python` to select the same environment for fixture generation and downloaded extract verification.
The [native fixture guide](fixtures/README.md) describes the authored rows, overwrite variants, append and replace expectations, and advanced packs.
Flow, dependency, composed-source, and large-upload cases require their declared additional fixtures.

## Create a target profile

Import an existing configured environment into ignored local storage:

```powershell
go run ./tests/live/runner init --environment test --out .tadx-live\target.json
```

Replace `test` with the environment alias already configured for the selected target.
Add `--config` when the source TADX configuration is not the default user configuration.
Import copies nonsecret selectors and credential references.
It does not save PAT values, change mutation consent, or modify administrator policy.

A profile has this shape:

```json
{
  "schema_version": 1,
  "alias": "qualification-target",
  "server_url": "https://tableau.example.test",
  "site_content_url": "qualification",
  "expected_site_luid": "selected-site-luid",
  "config_path": "source-config.yaml",
  "environment": "test",
  "actor_pat": {
    "name_env": "TADX_TEST_PAT_NAME",
    "secret_env": "TADX_TEST_PAT_SECRET"
  },
  "budgets": {
    "command_seconds": 120,
    "observation_seconds": 300,
    "operation_seconds": 1200
  },
  "bindings": {},
  "artifacts": {},
  "features": {}
}
```

`server_url` is an HTTPS origin or supported server base URL without embedded credentials.
`site_content_url` is explicit, including an empty string for a Server default site.
`expected_site_luid` guards against selecting a different site with a similar name.
`config_path`, artifact paths, and binding paths resolve relative to the profile file.
The optional `python` profile path also resolves relative to the profile file.
The `--python` flag resolves from your current working directory.
Use `ca_file` for a private Server certificate authority instead of disabling TLS verification.

The actor PAT authenticates candidate commands.
An optional `fixture_pat` uses the same credential-reference shape for the independent fixture client.
When supplied, its PAT name and secret must differ from the actor PAT.
The verifier can use another account, provided it retains permission to inspect and clean up all owned fixtures.
Subscription tests use the independently authenticated actor's user identity.

A single PAT is supported through serial handoffs.
The runner waits for the candidate process, then checks that known local transfer workers stopped before refreshing the independent verifier session.
It does not run two sessions using that PAT concurrently.
An active or unobserved worker prevents verifier authentication and cleanup, preserving its operation and resource journal.
Same-target phases use a target lock, including phases started from another checkout.

Configure only the inputs required by your selection:

- [Content bindings](content-bindings.md) cover native payloads, schema expectations, ownership transfers, fresh jobs, and workspace cases.
- [Administration, catalog, lineage, and Pulse bindings](bindings.md) cover exact users, metadata assets, authored audit and graph truth, labels, and eligible Pulse fields.
- Shared contracts declare additional prerequisites in `SharedBindingRequirements()` in [shared.go](suite/shared.go), including restricted authentication and operator-assisted native login.

Empty authored expectations cannot produce a native, audit, graph, or schema pass.
Accounts, licensed roles, catalog indexing, and optional product capabilities require actual target readiness.
The profile's `features` object supplies only the explicitly consumed feature choices; it does not authorize a write or override an API refusal.

## Check authorization

Every live phase checks the candidate's actual managed-policy status and the selected server and site's persisted mutation consent.
Fixture writes and cleanup writes receive the same authorization checks as subject operations.
The required capability set includes subject commands and the fixture operations they need.
An invalid policy, a denied capability, disabled remote mutations, or absent saved consent blocks execution.

Setup never repairs policy, edits an administrator allowlist, or enables site consent.
An agent needs explicit permission for the selected canonical server, exact site, and persisted setting before changing consent through TADX.
Approval to test against a site does not itself change that setting.
A read-only subject test can still need consent because its isolated prerequisites require remote fixture creation.

Native-store login qualification additionally requires an operator, a real interactive terminal, and explicit PAT persistence consent.
It is not an unattended substitute for the ordinary environment-reference authentication tests.

## Set up and run a selection

Use the same selection, profile, binary, and state directory for the following phases:

```powershell
go run ./tests/live/runner setup --target-file .tadx-live\target.json --binary .tadx-live\candidate\tadx.exe --state-dir .tadx-live\target --selection .tadx-live\selection.json --python .tadx-live\python\Scripts\python.exe
go run ./tests/live/runner verify-setup --target-file .tadx-live\target.json --binary .tadx-live\candidate\tadx.exe --state-dir .tadx-live\target --selection .tadx-live\selection.json
go run ./tests/live/runner run --target-file .tadx-live\target.json --binary .tadx-live\candidate\tadx.exe --state-dir .tadx-live\target --selection .tadx-live\selection.json
```

Setup validates the selected inputs, negotiates the target's REST version, checks independent fixture access, generates needed native files, and records an owned root project.
For workbook inspection, setup retains a dedicated workbook and a same-name decoy in separate projects, then waits for exact Metadata indexing.
The manifest checkpoints their identities before indexing completes, so repeating setup resumes the same fixture pack.
Inspection cases verify this pair without publishing or deleting it.
Other content scenarios create isolated fixtures.
Every case receives an isolated local configuration, workspace, cache, and state directory.
The candidate configuration includes only the selected environment and its existing site consent.
User credential stores remain outside the isolated test home except for the explicitly selected native-login contract.

Setup is reusable for the same target and fixture specification.
Running setup with another selection replaces the prepared selection after its prerequisites pass, while preserving the owned root.
`verify-setup` checks the selected setup coverage and current fixture observations without creating a replacement root.
A changed fixture hash, catalog fingerprint, or fixture specification requires setup again.
When the fixture specification changes, clean up the previous setup root before preparing its replacement.
If target indexing exceeds the observation budget, preserve the checkpoint and rerun setup or increase `budgets.observation_seconds` in your profile.
The runner refuses to start cases until setup reports readiness.
The candidate executable can change between runs; every report records its hash.

The runner runs selected cases serially.
A failed case still attempts bounded cleanup and records the result.
It preserves confirmed identities when a later assertion or API observation fails.
It never resubmits a consequential mutation to make an observation pass.

## Run a named Go entry point

After setup covers the chosen ID, the generated Go test wrapper provides another surgical entry point:

```powershell
$env:TADX_LIVECLI_TARGET = (Resolve-Path .tadx-live\target.json).Path
$env:TADX_LIVECLI_BINARY = (Resolve-Path .tadx-live\candidate\tadx.exe).Path
$env:TADX_LIVECLI_STATE = (Resolve-Path .tadx-live\target).Path
go test -tags=livecli ./tests/live -run '^TestLiveWorkbookInspect$' -count=1 -timeout=30m
```

Use the catalog's exact `go_name` and an anchored `-run` expression.
The `livecli` package rejects an absent `-run` selector.
It also requires explicit target and candidate environment variables.
These wrappers use the same scenarios and setup manifest as the runner.
Run one contract or a deliberate small set; a broad regular expression is an explicit broad live run.

The wrapper file is generated.
After changing catalog definitions, regenerate it with `go generate ./tests/live` rather than editing it manually.

## Read outcomes and evidence

The runner returns a nonzero exit code when selected contracts do not all pass.
A blocked, unsupported, or inconclusive result is never a qualification pass.

| Outcome | Meaning |
| --- | --- |
| `passed` | Subject assertions and required cleanup succeeded. |
| `failed` | A command, postcondition, evidence write, or cleanup failed. |
| `blocked-prerequisite` | Required bindings, access, policy, consent, tools, or setup are missing. |
| `unsupported-target` | The selected target explicitly lacks a required supported API or feature. |
| `inconclusive-observation` | A deadline, active operation, cancellation race, or uncertain acceptance prevents a supported conclusion. |
| `not_selected` report entries | No qualification claim exists for those IDs. |

Reports include the selected candidate hash, target fingerprint, catalog hash, fixture specification, per-case timings, cleanup status, and evidence paths.
Reports also record embedded Go and VCS build metadata when available.
The state tree contains the setup manifest and one directory per run:

```text
.tadx-live/
  target/
    manifest.json
    runs/
      <run-id>/
        report.json
        junit.xml
        <resource.behavior>/
          journal.json
          commands.json
          config.yaml
          workspace/
```

The runner writes JUnit output for selected results.
Successful cleanup removes each case's workspace and test home while retaining its journal, command evidence, and nonsecret configuration.
Nonpassing results remain visible as nonpassing entries, including prerequisite and observation limits.
Command evidence is bounded and redacts resolved PAT and session material.
A candidate credential leak fails the case even after evidence redaction.
Local configuration and fixture manifests still contain target-specific identities, so keep the complete state directory ignored and review evidence before sharing it.

Default command, observation, and operation budgets are 120 seconds, 300 seconds, and 1200 seconds.
Native publication, pull, and wait commands receive an operation budget plus a two-minute harness margin.
Cleanup has its own bounded observation window.
Read visibility checks retry only observation, retaining the accepted resource identity.
Cancellation races and already-terminal job observations report the coverage limit explicitly.
Large-upload cases use genuinely large native packages above the production threshold, without changing uploader thresholds.

## Recover an interrupted run

Preserve the state directory and inspect its report, accepted operation IDs, job IDs, and cleanup journal.
Resume cleanup using the exact generated `run_id` from the report:

```powershell
go run ./tests/live/runner cleanup --target-file .tadx-live\target.json --binary .tadx-live\candidate\tadx.exe --state-dir .tadx-live\target --run 0123456789abcdef0123
```

Replace the illustrative run ID with the recorded value.
The setup root's `setup_run` can also be cleaned after all dependent case runs have completed and cleaned up.
Create a fresh setup before running more cases after removing the root.

Cleanup requires the same guarded target and current authorization.
It reads required capabilities from each durable journal, so it needs no new test selection.
Catalog and artifact hash drift do not block exact-target journal recovery.
It deletes only exact journal-owned identities, verifies absence independently, and restores saved external catalog properties.
A generated name by itself does not authorize deleting an unrelated existing resource.
Ambiguous names, unknown mutation acceptance, active jobs, and unobserved workers retain state for inspection.
Cleanup does not guess that an unseen mutation failed or clear a lock whose owner might still be active.

Do not remove local state while a worker or remote job can still reference it.
If authorization or setup validation blocks recovery, preserve the journal and resolve that blocker before retrying cleanup.
No broad site reset, name-prefix sweep, or production-data deletion command is part of this workflow.

## Change the target

Create a separate profile and state directory for each Server site or Cloud site.
Point the profile at its source configuration and credential references, then provide the selected advanced fixture bindings for that site.
Run setup using the same stable selection.
REST versions and concrete object identities come from that target rather than constants in scenario code.

Clean the old target's owned runs before retiring its profile.
Never repoint an existing state directory at another site to reuse its journal.
Target fingerprints and site guards prevent cross-target cleanup.
Optional capabilities can differ between Server and Cloud; missing support stays explicit rather than weakening assertions.

## Maintain the suite

Keep command behavior in scenarios and target details in profiles and authored fixture bindings.
Add an independent postcondition for every meaningful subject mutation.
Keep fixture provisioning independent of the candidate command under test.
When adding a case, update the catalog, executable registry, binding requirements, fixture capability declarations, ownership mapping, and generated Go entry point together.
A capability outside the live scope needs an explicit reason in the exclusions ledger.

Run the local harness checks without a live target:

```powershell
go test ./tests/live/...
go vet ./tests/live/...
```

These checks verify selection, ownership, isolation, native parsing, and discriminating assertion behavior through local fixtures.
They do not certify a Tableau deployment.
Follow [the repository contribution checks](../../CONTRIBUTING.md#validate-locally) before pushing or merging.
Hosted CI remains manual-only and does not acquire permission to run a live suite from a source change.
