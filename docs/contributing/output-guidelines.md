# Compact and full output guidelines

Every TADX capability returns structured TOON through the shared output layer.
The default response is an explicit compact projection designed to minimize model context while preserving every field needed for the next safe decision.
`--full` returns expanded, bounded details for the same operation.

These rules are part of the coding contract for every action slice.

## Standard compact output

Include a field in the compact response only when it serves at least one of these purposes:

- State the outcome or operation status.
- Identify the authoritative resource and include the exact selector context needed for the next action.
- Identify an actionable artifact path or mutation target.
- Expose safety state needed to authorize, retry, continue, or stop.
- Report aggregate counts, partial outcomes, warnings, and continuation metadata.
- Provide a concrete next-step command template in `help[]`.

Prefer aggregate counts over item detail.
Render actionable artifact paths relative to the resolved workspace with forward slashes.
Resolve absolute filesystem paths only at runtime and never emit a developer home or checkout path.
Do not include fingerprints, provenance internals, secondary identifiers, successful request IDs, or repeated records unless the next safe decision requires them.
Warnings appear once at the top level unless their location is necessary to understand an independently meaningful partial outcome.

When the compact projection omits fields available through `--full`, include this exact top-level field immediately before `help[]`:

```toon
details: "--full"
```

The marker means that the caller can add `--full` to the same command.
Do not place this disclosure marker in `help[]` because `help[]` is reserved for concrete next actions.
Omit the marker when compact and full output are identical.

## Expanded full output

Full output is a strict, bounded superset of the compact information for the same completed operation.
It may add relative canonical artifact paths, fingerprints, nonsecret provenance, secondary LUIDs, successful request IDs, validation diagnostics, and bounded item details.
It must not change the remote request, local mutation, result semantics, selector resolution, or safety behavior.
Every full-detail collection has an explicit capability-specific maximum and reports the total or omitted count when truncation occurs.
Full output retains compact aggregate counts even when it adds the underlying detail records.

`--full` never:

- Fetches another page or increases a page size.
- Expands an unbounded inventory.
- Returns a raw upstream payload.
- Weakens secret redaction.
- Hides or changes a mutation preview.

List and search commands expand only the current bounded page.
Use an explicit field selector when a list capability supports caller-selected fields.

## Fields that must remain compact

Never hide a field that is required to:

- Authorize or understand a consequential mutation.
- Identify an ambiguous or authoritative target.
- Determine whether an operation completely succeeded.
- Avoid an unsafe retry.
- Follow pagination or continuation state.
- Act on a warning or corrective action.

Successful Tableau request IDs normally belong in full output.
Request IDs and job IDs required to diagnose errors or uncertain outcomes remain in the standard structured error.

If a count is unknown, omit it or represent the unknown state explicitly.
Never render a zero that could be mistaken for a completed empty result.

## Implementation pattern

The action owns its complete typed result plus explicit compact and bounded full projections.
Implement `CompactOutput() any` and `FullOutput() any` on a detail-bearing action output so the shared renderer can select the requested presentation.
Do not use omission tags or reflection rules as the primary field-selection mechanism because newly added fields must not leak into compact output accidentally.

Keep `--full` as presentation state in the CLI and shared renderer.
Do not add it to action input or let it reach resource adapters.

Do not build either output shape in Cobra.
Do not modify the TOON codec to implement field projection.

## Required tests

Every detail-bearing capability must include:

1. An exact compact TOON golden fixture.
2. An exact full TOON golden fixture.
3. Assertions that compact output omits full-only fields.
4. An assertion that compact output includes `details: "--full"` and full output omits it.
5. A bounded-output test using many detail records.
6. Tests proving detail and warning bounds report omitted records without losing aggregate counts.
7. Tests proving warnings, unknown counts, continuation state, and safety fields are represented correctly.
8. A CLI test proving `--full` changes only presentation and not the action input or operation behavior.

Treat accidental compact-schema growth as a test failure that requires explicit review.
