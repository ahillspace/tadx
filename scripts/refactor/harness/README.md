# Prepare isolated project-harness integration

This offline preparer produces a **blocked integration snapshot**, not G9 evidence.
It copies the selected six-case runtime source closure and its worker-image inputs under exact source hashes.
It never runs a binary, builds an image, launches a model, contacts Tableau, reads credentials, or changes consent.
The original external harness remains unchanged.

Run the synthetic tests with Python 3.11 or later and Node.js 22 or later:

```text
python -B -m unittest discover -s scripts/refactor/harness -p "test_*.py"
```

## Prepare reviewed inputs

Supply the current external suite directory explicitly.
The output must be a new directory with an existing parent, outside that suite.
There is no discovery of earlier runs, automatic rebuild, resume, or in-place update.
The checked-in lock selects the static Python and broker source closure, the worker Dockerfile, and exactly six project exercise/profile pairs.
The copied index contains only those six cases.
The lock omits runtime configuration, credentials, historical evidence, and unrelated case definitions.
Changed source hashes or patch anchors require review and prevent preparation.

```text
python -B scripts/refactor/harness/prepare.py --harness external/current-suite --output ignored/new-preparation --candidate accepted/candidate.json --catalog accepted/capabilities.json --build linux/amd64=accepted/linux/tadx --build windows/amd64=accepted/windows/tadx.exe --capture accepted/current-cli-capture/manifest.json --authority private/project-authority.json --consent-evidence private/reviewed-consent-evidence.json
```

The candidate uses the [live validator identity schema](../live/README.md).
Supply the accepted `linux/amd64` and `windows/amd64` builds with repeated `--build` arguments.
The prebuilt capture manifest must name the same source commit and tree digest as the accepted candidate.
It must bind both exact binaries and contain the matching registry, complete `tadx` and `tadx-pulse` Guidance trees, help, and native build metadata.
The preparer checks every declared byte hash and copies the supplied binaries without running or rebuilding them.
It rebases a sanitized capture manifest and current CLI pointer to the copied files.
No installed host CLI substitutes for an accepted build.
Independent G0 evidence must establish source-to-build provenance; matching supplied hashes cannot establish it.
Exit code 2 means the snapshot was prepared but live execution remains blocked.
Exit code 1 means preparation failed.
Partial filesystem output stays blocked and must not be reused.

## Preserve existing authority

The separate authority input has exactly this schema, shown with synthetic values:

```json
{
  "schema_version": 1,
  "environment": "fixture",
  "server_url": "https://tableau.example.test",
  "site_content_url": "fixture-site",
  "fixture_scope": "unique-run-owned-projects",
  "consent_changes_authorized": false,
  "credential_persistence_authorized": false,
  "saved_consent": {
    "server_url": "https://tableau.example.test",
    "site_content_url": "fixture-site",
    "enabled": true,
    "source": "saved_site_setting"
  },
  "consent_evidence_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}
```

An operator independently verifies the saved consent and records the hash of sanitized evidence.
The preparer checks evidence integrity, not its semantic truth or freshness.
Fresh exact-site verification remains required before eventual dispatch.
Authority and consent evidence are hashed but not copied into the snapshot.
Do not provide credential files as either input.
Do not track real authority files, private target identifiers, or generated snapshots.

The copied bridge's consent helper accepts only the authorized canonical server, exact site, and single environment.
It copies the existing enabled saved consent into disposable configuration without changing host consent.
It rejects disabled requests, conflicting saved entries, additional environments, consent-change authority, and credential-persistence authority.
Legacy mutation-change flags are neither copied nor fabricated.
Fixture permission does not authorize new consent settings.

## Applied safeguards and remaining work

The copied broker rejects `disposable_native` bypass mode and retains the existing strict project guard and native CLI invocation.
The copied launcher and bridge block at their entry points before task dispatch.
The copied launcher's configuration path also blocks until accepted-source provenance has independent G0 qualification.
These edits are recorded with source and prepared hashes in `preparation.json`.
They are preparation safeguards, not a complete sandbox against manually running arbitrary copied source.

The copied positive-project preflight checks independently verified saved consent for the exact environment, server, and site.
It rejects legacy consent-change flags.
Fixture journals bind cleanup to the exact run, case, and site LUID.
Setup creates a new top-level parent and descendants without requiring an existing configured project as a sentinel.
Provisioner writes and restoration target only confirmed owned IDs, and creation requires an owned parent after the initial root.
Task-created projects enter ownership only after successful native command evidence and independent exact-ID readback agree.
Missing acknowledgment quarantines cleanup instead of adopting a project by name.

The copied list and inspect adapter reuses that fresh fixture setup and exact-ID cleanup.
It checks native results against independent baseline records and unchanged scoped projects, content inventories, permissions, and sentinel state.
Its separate read guard permits bounded child listing under the owned parent and inspection of recorded owned projects.
It rejects outside projects, changed configuration, missing baselines, and every remote mutation.
The copied read prompts expose the scoped parent and target IDs; the list case therefore qualifies a bounded owned-parent list.
It does not claim coverage of unfiltered sitewide listing.
Read discovery in the existing mutation guard remains separate from mutation authorization.
Offline tests cover the policy and adapter with synthetic bindings and an in-memory oracle, not live Tableau behavior.

Live integration remains blocked on these qualifications:

- Qualify the accepted-source reference used by the copied launcher's configuration path.
  The rebased capture manifest intentionally omits the original host repository path.
- Inspect the immutable base worker image ID and installed Codex runtime version.
  Verify that the worker image uses the accepted Linux binary bytes, copied broker inputs, and matching Guidance.
  A mutable image tag or the preparer's copied hashes alone cannot establish this.
- Qualify the copied runtime composition, including the consent preflight, owned read adapter, original mutation guards, and cleanup.
  Offline source closure and synthetic tests do not establish live behavior or authorize dispatch.
- Provision a new run-owned parent and descendants with exact ownership, independent credential roles, and verified cleanup.
  Do not reuse existing projects or persist PATs.
- Capture trustworthy actual provider, model, and reasoning-effort metadata.
  Requested `gpt-6-luna` and `medium` arguments or ordinary `exec --json` events alone are insufficient.
- Complete G0-G8 and independently reviewed checkpoint coverage for the exact candidate.

Do not remove the launcher block until those qualifications have reviewed evidence.
Missing actual model metadata remains blocked or excluded, never an inferred live pass.
The existing [G9 evidence validator](../live/README.md) remains the acceptance accounting layer.
