# Capability build task template

The integration owner fills this bounded task card and gives it to one slice agent.
The integration owner reconciles product-wide sources before assignment.
The slice agent does not read product-wide documents unless this card cites one exact section.

---

## Task: build capability `<domain>.<verb>`

You are building exactly one TADX capability in its own action package. Build only this capability.

### Read before you start

1. `AGENTS.md`.
2. The focused registry result included below.
3. `docs/contributing/output-guidelines.md`.
4. The named closest code example.
5. The named task-specific evidence excerpt.

Do not read the arc42, the full capability contract, `docs/axi.md`, `docs/toon.md`, `docs/scope-v1.md`, or `docs/build-order.md` unless this card cites an exact section.
Stop when this accepted task contract conflicts with code or tests.

### This capability
- Registry ID: `<id>`
- Domain / verb: `<domain>` / `<verb>`
- Product disposition: `<ship | delegated>`
- Read-only or mutation: `<read | consequential-mutation>`
- Evidence level: `<architecture-locked | local-contract | docs-only | contract-verified | live-verified>`
- Selectors: `<luid, exact name, project path, ...>`
- Upstream operation (if remote): `<endpoint + API version, or "none">`

### Focused registry result

```toon
<paste `tadx capability get <id> --full` output here>
```

### Accepted behavior and exclusions

- Required behavior: `<exact bounded behavior>`
- Explicit exclusions: `<what this slice must not do>`
- Owned files: `<disjoint paths assigned to this agent>`
- Forbidden shared files: `tadx-v1-capability-contract-final.md`, `internal/capability/registry_gen.go`, `internal/capability/implementation.go`, `internal/cli/root.go`, `internal/app/app.go`, and `docs/reference/capabilities.md`, unless this card assigns integration ownership.
- Closest code example: `<one exact package or file>`

### Output contract

- Compact fields: `<exact fields>`
- Full-only fields: `<exact fields>`
- Bounds and omitted counts: `<exact limits>`
- Help commands: `<exact templates>`

### API evidence

- Local file and heading: `<path, heading, bounded line range, or none>`
- Captured evidence record: `<one exact file and section>`
- Official web verification required: `<yes or no, and why>`

### Evidence gate (decides how far you may build)
- If evidence is docs-only: build only to the adapter seam - types, validation, local orchestration, fixtures, tests. Do not write live API-calling code. Do not make the command executable. Stop at the seam and report what upstream contract is still needed.
- If evidence is contract-verified or live-verified: build the full slice.

### Method
- Write tests first (behavior, selector ambiguity, output shape, error/exit, and where applicable redaction, mutation discovery, preview/apply, artifact/provenance).
- Then implement to pass them.
- Render only through the output layer in TOON. Never build output in Cobra.
- Classify every output field as compact or full before implementation.
- Add separate compact and full goldens for detail-bearing output, plus a bounded compact-output test.
- Keep safety-critical and next-decision fields compact, and use the exact `details: "--full"` marker when expanded fields exist.
- Declare your own narrow dependency interfaces in this package. Do not import Cobra, net/http, or another action.

### Done means (all must hold)
- Assigned action, adapter, fixtures, and tests satisfy this card.
- Focused tests and the verification commands below pass.
- Integration requirements are returned to the integration owner instead of changing forbidden shared files.
- For a docs-only capability: the package stops at the adapter seam, the command is not executable, and the missing upstream contract is stated in the report.

### Verification commands

- Focused: `<commands>`
- Contract: `<commands>`
- Live opt-in: `<commands or none>`
- Final integration owner: `<integration test commands only>`

These commands do not authorize a comprehensive branch review, a review board, or the no-mistakes pipeline.
After the assigned checks pass, report the results and stop.

### Do not
- Do not build any other capability, invent a blocked or docs-only capability into existence, or add a second confirmation prompt.
- Do not let `--force` mean `--apply`, let discovery visibility imply permission, or persist or print secrets.
- Do not create utils/helpers/common/services packages or empty future packages.
- Do not weaken, edit, or route around an enforcement gate to make your work pass. If a gate blocks correct work, stop and flag it.

### Report back
- The registry ID built, files changed, test results, and required integration changes.
- Any contract conflict, missing upstream evidence, or gate you believe is wrong. Do not silently work around it.
