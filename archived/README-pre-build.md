# TADX pre-build document set

This is the control layer that must exist before the TADX repository is scaffolded, so that every agent building a CLI capability builds it the same way, and the work stays in scope and under control.

These documents operationalize the two authoritative sources for builders; they do not replace them:
- the arc42 (product behavior, architecture boundaries, safety, scope authority);
- the V1 capability contract (the public capability inventory and per-capability status).

When a document here disagrees with a source, the source wins and the document is corrected.

## Read in this order

| Document | Purpose | Suggested repository placement |
| --- | --- | --- |
| scope-v1 | Token-light, definitive in/out of scope for V1. | docs/scope-v1.md |
| axi | The CLI behavioral contract: seven principles as checkable rules. | docs/axi.md |
| toon | The output format contract: grammar, rendering, codec, conformance. | docs/toon.md |
| AGENTS | Short agent routing and start-here guide. | AGENTS.md (repository root) |
| action-guidelines | How to add one capability: layout, artifacts, action shape, checklist. | docs/contributing/adding-a-capability.md |
| build-order | Foundation freeze and the dependency-ordered slice sequence. | docs/build-order.md |
| task-template | Per-capability task you fill in and hand to one build agent. | docs/contributing/task-template.md |
| runbook | The ordered steps and copy-paste prompts to scaffold and build. | docs/runbook.md |

## Companion docs the scaffold still produces

These are not in this set because they describe the repository as built, not the pre-build contract:
- repository-structure (the package layout and import diagram, as built); the dependency and import rules themselves live in action-guidelines;
- the generated capability reference, derived from the executable registry.

## Notes

These documents are intentionally pathless: they name artifacts rather than referencing any machine path, because they leave this machine.

Decisions recorded here that resolve prior contradictions:
- TOON conforms to the upstream toon-format specification (pin the version; prefer a Go implementation, else a conforming in-repo codec). It is a frozen foundation, not a per-action concern.
- Published-datasource-field description write-back is a deferred fast-follow pending the near-release TDS datasource-field API; the Metadata API's upstream-table description write is a separate capability at a different granularity.
- The configuration selector is the site content URL, not a site ID; the authoritative site LUID is resolved after authentication.
- Registry tests assert invariants, not a hardcoded capability count.
