# Capability build task template

Copy this template per capability, fill the placeholders, and hand it to one build agent.
It assumes Phase 0 is frozen and the capability already has its registry row.
The template does not restate the contracts; it points the agent at them and defines done.

---

## Task: build capability `<domain>.<verb>`

You are building exactly one TADX capability in its own action package. Build only this capability.

### Read before you start (binding contracts, in priority order)
1. AGENTS.md - routing and non-negotiables.
2. docs/contributing/adding-a-capability.md - the coding contract you must follow exactly (folder layout, required artifacts, action shape, identity, output/errors, evidence gate, registry rules, completion checklist).
3. docs/axi.md - the behavioral contract and the concrete AXI conventions C1-C10.
4. docs/toon.md - the output format you render through.
5. docs/contributing/output-guidelines.md - the compact versus --full field-selection and test contract.
6. docs/scope-v1.md - confirm this capability is in scope for V1 and not blocked.

If any two sources conflict, the arc42 and the V1 capability contract win, then AGENTS.md, then these docs. Stop and flag the conflict rather than guessing.

### This capability
- Registry ID: `<id>`
- Domain / verb: `<domain>` / `<verb>`
- Product disposition: `<ship | delegated>`
- Read-only or mutation: `<read | consequential-mutation>`
- Evidence level: `<architecture-locked | local-contract | docs-only | live-verified>`
- Selectors: `<luid, exact name, project path, ...>`
- Upstream operation (if remote): `<endpoint + API version, or "none">`

### Evidence gate (decides how far you may build)
- If evidence is docs-only: build only to the adapter seam - types, validation, local orchestration, fixtures, tests. Do not write live API-calling code. Do not make the command executable. Stop at the seam and report what upstream contract is still needed.
- If evidence is live-verified or the upstream contract is captured with a passing contract test: build the full slice.

### Method
- Write tests first (behavior, selector ambiguity, output shape, error/exit, and where applicable redaction, mutation discovery, preview/apply, artifact/provenance).
- Then implement to pass them.
- Render only through the output layer in TOON. Never build output in Cobra.
- Classify every output field as compact or full before implementation.
- Add separate compact and full goldens for detail-bearing output, plus a bounded compact-output test.
- Keep safety-critical and next-decision fields compact, and use the exact `details: "--full"` marker when expanded fields exist.
- Declare your own narrow dependency interfaces in this package. Do not import Cobra, net/http, or another action.

### Done means (all must hold)
- Every item on the adding-a-capability completion checklist is satisfied.
- The architecture test, registry validation, golden fixtures, TOON conformance, and full CI (format, vet, test, race, cross-compile, tidy) pass.
- Generated capability docs were regenerated as the final step and committed.
- For a docs-only capability: the package stops at the adapter seam, the command is not executable, and the missing upstream contract is stated in the report.

### Do not
- Do not build any other capability, invent a blocked or docs-only capability into existence, or add a second confirmation prompt.
- Do not let `--force` mean `--apply`, let discovery visibility imply permission, or persist or print secrets.
- Do not create utils/helpers/common/services packages or empty future packages.
- Do not weaken, edit, or route around an enforcement gate to make your work pass. If a gate blocks correct work, stop and flag it.

### Report back
- The registry ID built, the files added, test results, and which enforcement gates passed.
- Any contract conflict, missing upstream evidence, or gate you believe is wrong. Do not silently work around it.
