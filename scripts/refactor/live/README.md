# Validate live-test evidence

This offline G9 tool validates evidence accounting, not live execution or semantic correctness.
It does not launch models, read credentials, change consent, install software, or contact Tableau.
Synthetic unit tests do not complete G9.
The [approved gate specification](../../../docs/maintainability/regression-gates.md) remains the acceptance contract.

## Run the checks

Use Python 3.11 or later, with no third-party dependencies:

```text
python -B -m unittest discover -s scripts/refactor/live -p test_validate.py
python -B scripts/refactor/live/validate.py --candidate evidence/candidate.json --catalog evidence/capabilities.json --run evidence/run.json --results evidence/attempts.jsonl --evidence-root evidence --build linux/amd64=evidence/tadx
```

Repeat `--build PLATFORM=PATH` for every candidate build.
The validator reads all inputs without modifying them.
Success returns `evidence_accounting_passed`; rejection returns `blocked` and exit code 1.
Malformed inputs also fail closed without printing their contents.

## Input contract

Supply `candidate.json` independently from the accepted G0 build/provenance capture.
Do not derive expected identity from the run being checked.
It contains exactly these fields:

```json
{
  "source_revision": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "source_tree_sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "catalog_sha256": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
  "builds": {
    "linux/amd64": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
  }
}
```

These synthetic hashes illustrate the format and cannot qualify a real build.
Catalog and binary hashes cover actual supplied file bytes.
The source commit and source-tree digest must match throughout the run; this tool relies on G0 for their derivation and build provenance.
It does not reconstruct source snapshots or prove that a binary was compiled from the asserted source.

The catalog uses the generated capability JSON array shape from `docs/reference/capabilities.json`.
Every entry with `owner: "cli"`, `implementation: "implemented"`, and a nonempty `command_path` requires completed evidence.
There is no reduced selection list, implicit skip, or automatic waiver.
The default validates final sweeps; it never derives a reduced action selection from the run being checked.
Maintainer waivers are not automatically accepted.

### Checkpoint scope

For a bounded migration checkpoint, pass `--checkpoint evidence/checkpoint.json` from the independently reviewed scope definition.
The scope contains exactly `schema_version: 1`, `checkpoint_id`, `candidate`, `required_actions`, and `scope_review`.
The candidate identity must match the independently supplied candidate, and every required action must belong to its complete executable catalog.
The nonempty action list includes shared-dependency consumers identified during scope review, not only commands with renamed packages.
`scope_review` is a hashed evidence reference that explains that impact analysis.
The run must declare `scope: "checkpoint"` and include the identical scope object under `checkpoint`.
G0-G8, outcome evidence, retained attempts, and cleanup requirements still apply.
The summary labels checkpoint accounting explicitly and also reports the full catalog size.
A checkpoint cannot satisfy final acceptance, which still requires an independent full-catalog sweep without `--checkpoint`.
The validator checks the declared scope, not the semantic completeness of its dependency impact analysis.

`run.json` contains:

- `schema_version: 1`, `scope: "final"`, and a nonempty `run_id`.
- `candidate`: the complete expected candidate identity.
- `prior_gates`: keys `G0` through `G8`, each set to `"passed"`.
- `prior_gates_evidence`: a reference to the preceding gates' evidence.
- `attempt_ids`: every attempt ID in JSONL order, including exclusions and retries.

Each reference has exactly `path` and `sha256` fields.
Paths use forward slashes, remain relative to the evidence root, and resolve to nonempty files with matching hashes.
Do not place secrets, private site identifiers, or host paths in tracked examples.
Evidence must be sanitized before ingestion; this validator is not a credential scanner.

Each JSONL attempt contains these fields:

| Fields | Requirement |
| --- | --- |
| `attempt_id`, `case_id`, `action_id`, `run_id` | Unique attempt ID, task case, executable action ID, and matching run |
| `attempt_number`, `previous_attempt_id` | Start at 1 and null; retries increment by one and reference the previous attempt for that case |
| `candidate`, `platform`, `binary_sha256` | Exact expected identity and one candidate build |
| `requested_model`, `actual_model` | `gpt-6-luna` |
| `actual_provider` | `openai` |
| `requested_reasoning_effort`, `actual_reasoning_effort` | `medium` |
| `fixture_id`, `reason` | Nonsecret fixture identity and concise grading reason |
| `classification` | `completed`, `completed_with_workaround`, `failed`, or `excluded` |
| `confidence` | `high`, `medium`, or `low` |
| `harness_agreement` | `agreed`, `disagreed`, or `unavailable` |
| `telemetry` | Explicit `wall_seconds`, `model_tokens`, `model_turns`, and `tool_calls`; unavailable values are null |
| `evidence` | References named `task`, `transcript`, `commands`, `model_metadata`, `fixture_authority`, and `assessment` |
| `cleanup` | `status` and an `evidence` reference; status is `completed`, `not_needed`, `unresolved`, or `failed` |
| `intentional_refusal` | Boolean required for completed attempts |
| `proof` | Completed attempts need `kind` and an `evidence` reference |

Proof kinds are `native_effect`, `local_state`, and `native_refusal`.
Native effects also require `evidence.native_acknowledgement` and `evidence.request_response` references.
Remote mutations cannot claim success from local-state evidence alone.
A native refusal counts only when `intentional_refusal` is true.
For workarounds, require `workaround_review: "accepted"` and an `evidence.workaround_review` reference.
The assessment must judge requested targets, flags, values, and verified effects, not final-answer formatting.

Excluded attempts preserve known requested and actual model, provider, and effort strings, even when those settings differ from the required configuration.
Use null for unavailable metadata, never to conceal a known mismatch.
Eligible attempts require the exact model, provider, and effort settings in the table.
Excluded attempts can record null transcript, command, or model-metadata references when unavailable.
They never cover an action.
Their task, assessment, fixture-authority, and cleanup references remain required.
Successful retries preserve earlier exclusions; any correctness failure or unresolved cleanup still blocks this gate.
The manifest catches omitted attempts relative to its recorded schedule, but is not tamper-proof proof that every historical attempt was disclosed.

## Authority and reusable harness gaps

Architecture approval does not authorize enabling saved Tableau mutation consent.
Confirm the exact server, site, disposable fixture scope, and cleanup authority before live execution.
Obtain separate explicit permission before credential persistence or consent changes.
Keep ordinary host configuration, installed software, and credential stores unchanged.

The existing external benchmark harness supports Linux Docker isolation, local fixtures, disposable native credential storage, and Linux installer fixtures.
It needs exact-candidate build/catalog qualification and explicit Luna-medium configuration; its saved model selection is not the required model.
Its current authored mappings omit `job.cancel`, `job.inspect`, `job.wait`, `policy.install`, `policy.samples`, `policy.status`, `policy.validate`, and `pulse.subscription.list`.
Windows installer fixture, execution, grading, and cleanup stages remain unimplemented.
Native macOS checks require an appropriate host or hosted CI; cross-compilation is not runtime evidence.
The repository exploration launcher prohibits auth, policy, consent, and installed-tool changes, so it cannot cover those workflows unchanged.

These findings describe observed setup gaps, not permanent exclusions or live-test outcomes.
No executor or external harness modifications are included here.
Hashed files prove reference integrity, not truthful contents, correct model metadata, actual user effects, or valid authority.
Independent semantic grading and provenance checks remain required.
