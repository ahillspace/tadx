# AXI: the TADX CLI behavioral contract

AXI is the Agent eXperience Interface: a set of design principles for building CLI tools that autonomous agents operate well.
The upstream source is the AXI specification at axi.md (author Kun Chen), with reference implementations gh-axi and chrome-devtools-axi.
This document is the TADX-binding form of AXI: every TADX command obeys it, and TADX conforms to this document rather than to any external repository.
Cobra is plumbing; AXI is the behavior.

The purpose is the product outcome the upstream benchmarks target: higher accuracy, lower tokens, lower latency, while staying understandable to humans.

The document has two parts.
The seven principles are the TADX behavioral contract, each a rule with a concrete do, a concrete do-not, and how it is enforced.
The concrete AXI conventions that follow are the upstream spec's ten code-level guidelines, mapped to exact TADX obligations.
A command that violates any rule in either part is not AXI-conformant and must not merge.

## 1. Progressive disclosure

Rule: the agent discovers only the detail it needs, when it needs it.
Do: expose a compact capability inventory (capability list) and focused per-capability detail (capability get) that includes ownership, selectors, safety, evidence, and blockers.
Do not: require an agent to load the full API surface, the whole site inventory, or an entire spec into context to use one command.
Enforced by: capability discovery tests; bounded default output; golden fixtures that fail on accidental output growth.

## 2. Bounded context

Rule: agent-facing output is bounded and deterministic in size.
Do: return a bounded slice with explicit continuation metadata; let catalog refresh exhaust upstream pages internally because it writes to cache, not to model context.
Do not: dump an unbounded list or full payload into the response. --full expands permitted output but never justifies dumping a whole site inventory.
Enforced by: pagination normalization tests; golden output tests; a default-output size assertion.

## 3. Deterministic behavior

Rule: the same inputs against the same Tableau state produce the same result, or the same explicit error.
Do: fail loudly and predictably; resolve exact targets; treat zero matches and multiple matches as errors.
Do not: fuzzy-match, prompt interactively, silently redirect after a remote rename or move, or add generic automatic retries. A bounded retry is allowed only where it is provably safe and deterministic.
Enforced by: identity tests (explicit LUID, exact name/path, zero and duplicate matches); deterministic error rendering tests.

## 4. Composability

Rule: commands are small primitives an agent or skill composes; a command does not hide a workflow.
Do: keep one public operation per command; allow a deterministic pseudo-primitive only when every step always belongs together and there is no meaningful mid-flow decision (for example, pull may resolve, download, write provenance, fingerprint, and render the view).
Do not: embed branching, judgment, mapping, or review inside a CLI verb. If a meaningful decision exists in the middle, it belongs in a skill or the agent.
Enforced by: capability-admission review; one-operation-per-row registry validation.

## 5. Structured output

Rule: output is structured, compact TOON by default, with a stable field contract.
Do: render through the output layer; support --full for expanded output and --raw only where the registry marks the capability as raw-capable; keep error, warning, pagination, and partial-outcome shapes stable.
Do not: build output inside Cobra; invent a private TOON dialect; emit YAML; let --raw bypass secret redaction.
Enforced by: golden fixtures; TOON conformance tests; the output layer being the only renderer.

## 6. Machine discovery

Rule: the executable capability registry is the single runtime source of truth for what exists, who owns it, and how to invoke it.
Do: derive generated help, reference docs, ownership views, and command wiring from the registry; give an agent a clear answer for MCP-owned or MCP-preferred capabilities, including which surface to use instead.
Do not: maintain a second hand-authored capability table; register fake or not-implemented commands; place Cobra factories inside registry metadata.
Enforced by: registry validation (every implemented command has exactly one registry entry and vice versa); generated-doc clean-diff check.

## 7. Agent ergonomics and safety

Rule: the interface is safe for an autonomous caller and legible to a human.
Do: keep mutation commands discoverable; block their execution when mutation policy is disabled; run enabled mutations by default; support --preview; require an explicit write target; redact secrets everywhere; use exit codes 0, 1, and 2; include Tableau request and job IDs in errors where available; emit a required human-readable view for artifacts.
Do not: add a second confirmation prompt or a production-only prompt; let --force bypass mutation policy; let discovery visibility imply permission; persist secrets TADX handles.
Enforced by: mutation and preview tests; mutation-discovery tests; secret-redaction tests; exit-code mapping tests.

For artifact publish, an explicit write target means the preview contains the fully resolved environment, site, project, and collision decision.
The caller does not need to repeat `--environment` when trusted artifact provenance supplies the source environment, site, and project defaults.
An explicit environment override selects a different target and requires the exact target project.
An artifact without source provenance requires an explicit target environment and project.
Source LUIDs remain provenance on cross-site publish and never identify the target resource.

## Concrete AXI conventions (upstream code guidelines)

These are the ten AXI spec conventions, restated as exact TADX code obligations.
Upstream groups them under Efficiency, Robustness, and Discoverability.
Each names the TADX principle it serves and how it is enforced.

### Efficiency

C1. Token-efficient output (serves principle 5).
Do: render every command in compact TOON by default; do not emit JSON as the default.
Enforced by: the output layer as sole renderer; golden fixtures; the toon contract.

C2. Minimal default schemas (serves principles 1, 2).
Do: return only the few fields an agent needs to act on a list item (identity, name, and the one or two fields that drive the next decision), not the full record. Expose the full field set through an explicit `--fields` selector.
Do not: return ten-plus fields per list item by default; add fields silently as a resource grows.
Enforced by: golden fixtures that fail on default-schema growth; a documented default field set per list capability.

For non-list results, the default is an explicit compact allowlist projection and `--full` returns expanded bounded detail for the same operation.
When compact output hides available detail, it includes the exact top-level marker `details: "--full"` so an agent never needs to hunt through help to discover the option.
The current field-selection and testing workflow is in [`CONTRIBUTING.md`](../CONTRIBUTING.md#preserve-output-and-safety-contracts).

C3. Content truncation with a size hint (serves principle 2).
Do: truncate a large text value and state what was cut, in the form `(truncated, N chars total - use --full to see complete body)`. `--full` returns the untruncated value.
Do not: emit an unbounded text body by default; truncate silently with no hint or escape hatch.
Enforced by: golden fixtures for truncated and `--full` output; a bounded-default assertion.

### Robustness

C4. Pre-computed aggregates (serves principles 2, 3).
Do: include the totals an agent would otherwise round-trip for. A paged list always reports the full total, not just the page size (for example a `count` of returned versus total). Where a command summarizes many items, include the summary counts inline.
Do not: force an agent to page the whole set only to learn the total.
Enforced by: pagination envelope tests that assert a total distinct from page size.

C5. Definitive empty states (serves principle 3).
Do: emit an explicit, structured zero-result result when nothing matched, so the caller can tell "no results" apart from "failed silently." A clean empty result exits 0.
Do not: return bare empty output for an empty result.
Enforced by: empty-result golden fixtures per read capability.

C6. Structured errors and exit codes (serves principle 7).
Do: return errors as structured data on stdout using the structured error type; reserve stderr for logs and diagnostics; use exit code 0 for success or a clean no-op, 1 for an operation or runtime failure, 2 for a usage error such as an unknown flag; make mutations idempotent where the upstream operation allows it; fail loud on an unknown flag rather than ignoring it.
Do not: prompt for interactive input; write structured results to stderr; swallow an unknown flag; retry non-idempotently.
Enforced by: exit-code mapping tests; structured-error rendering tests; a no-interactive-prompt assertion.

### Discoverability

C7. Ambient context (serves principle 6).
Do: provide the agent's operating context before it acts through the capability registry and the workspace and status commands. TADX may ship an optional Agent Skill generated from the same guidance; that skill is delegated surface, not a CLI verb.
Do not: require an agent to reconstruct environment, site, or auth state by trial and error.
Enforced by: capability discovery tests; workspace and auth-status behavior tests.

C8. Content first (serves principles 1, 6).
Do: where a command has a sensible default view, running it with no arguments shows live, actionable data plus a one-line description, not help text. `capability list` is the canonical content-first entry point.
Do not: make a bare command print only usage text when live data is the more useful default.
Enforced by: no-argument behavior tests for content-first commands.

C9. Contextual disclosure with next-step templates (serves principles 1, 4).
Do: append a `help[]` block of concrete next-step command templates to a result, carrying forward the fixed disambiguating flags already in play and leaving runtime values as explicit placeholders such as `<luid>` (never a guessed value). For example, a list result suggests the get command for one item as a template.
Do not: guess concrete IDs into a suggested command; suggest a next step that skips required apply or discovery gating.
Enforced by: golden fixtures for `help[]` blocks; a placeholder-not-value assertion.

The `details: "--full"` disclosure marker is not a next action and remains separate from `help[]`.

C10. Consistent way to get help (serves principle 6).
Do: give every subcommand a concise `--help` fallback whose content derives from the registry, for when contextual hints are not enough.
Do not: hand-author per-command help that can drift from the registry.
Enforced by: the generated-doc clean-diff check; registry-derived help.

## Where TADX intentionally diverges

TADX adds obligations the general AXI spec does not, because Tableau is a stateful system of record with consequential writes:
- consequential mutations run by default when enabled and support `--preview`; `--force` does not bypass mutation policy;
- mutation discovery remains available when mutation execution is disabled;
- identity is LUID-authoritative and ambiguity is a hard error, with no fuzzy or interactive resolution;
- secret redaction runs before rendering and takes precedence over `--raw`;
- a docs-only capability is built only to the adapter seam until its upstream contract is captured.

Where an upstream convention and a TADX safety rule conflict, the TADX safety rule wins.
