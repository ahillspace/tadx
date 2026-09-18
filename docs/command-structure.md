# Command structure and help standard

Status: maintained current standard for the registered CLI command tree.
This is the single standard for public command structure and help construction.
Use this standard for new help work; operating skills explain discovery and Tableau concepts rather than duplicating syntax.

## Command structure

Resource operations use `tadx <category> <resource> <verb>`, with at most three command words after `tadx`.
Categories organize related work; resources identify the object; verbs identify the operation.
Direct-action categories such as `auth` and `env`, and standalone utilities such as `search` and `update`, remain shorter.
Do not introduce artificial levels to fill the pattern.
The depth limit applies to public syntax, not internal action package directories.

## Help at each level

Navigation help shows where to go; operational help provides everything needed to construct commands for one resource.
Use `-h`, `--help`, and `tadx help <path>` consistently.

| Request | Required output |
| --- | --- |
| `tadx -h` | All top-level categories and utilities, short descriptions, resource names under each category, and all global flags. |
| `tadx <category> -h` | That category's resources, short descriptions, and available verbs for each; no expanded per-action flag reference. |
| `tadx <category> <resource> -h` | Complete resource reference: verb meanings, selectors, flags, constraints, relevant notes, and examples. |
| `tadx <category> <resource> <verb> -h` | Focused action reference from the same definitions, including applicable shared flags, constraints, notes, and examples without sibling operations. |

Root help must expose resource paths so users can go directly to a resource reference without first requesting category help.
Category navigation does not promise complete operation syntax; resource help does.
Show shared verb names first, followed by resource descriptions and their remaining verbs.
Derive shared sets from the current command tree and name their resource scope unless every listed resource shares them.
Use a shared group only when it shortens the navigation; retain standalone actions and resources with different verbs.
Shared verb names do not imply shared flags or behavior.
Resource help must stand alone without assuming root or category help was already read.
Do not include sibling-resource operations or options in a resource reference.
Every verb under the same resource resolves to its focused subset from the same definitions, regardless of supplied argument values.
Complete resource help includes every action definition and constraint, so focused action help never hides a necessary flag or requires another help call.

Apply the same rule structurally across the CLI:

- `pulse -h` introduces definitions and metrics; each resource gets its own complete reference, with focused subsets at known verbs.
- `auth -h` and `env -h` already sit above executable actions, so they provide complete operational references with focused subsets at known verbs.
- `workspace -h` covers its direct actions and includes a navigation entry for `artifact`; `workspace artifact -h` covers artifact actions with focused subsets at known verbs.
- `mutation -h` covers per-site consent status and changes; `policy -h` covers candidate samples, validation, and fixed-path status recovery.
- Standalone commands such as `search` show their own complete help.

Bare `tadx` remains the session overview, not an alias for `tadx -h`.

## Compact presentation

Use the shared compact operational renderer rather than concatenating conventional leaf manuals.
The renderer builds command and flag inventory from registered commands, then adds scoped notes and examples for the current category, resource, or verb.
Complete resource help and focused verb help use the same registered definitions, with focused output limited to the selected action.
TADX additionally supplies short resource and verb descriptions where Tableau terminology does not explain the operation.
Use this order for operational references:

```text
Usage: tadx <category> <resource> <verb> [flags]
Shared:
  common invocation flags
Target:
  exact selectors
Options:
  shared option meanings
Source:
  local input alternatives, where applicable
Commands:
  verb: required inputs [optional flags] (brief explanation)
Rules:
  essential constraints not conveyed by syntax
Batch:
  eligible repeated selectors, per-item file syntax, and bounds
Examples:
  a few complete, useful commands
```

Use `tadx content datasource -h` to inspect the current compact presentation.
Preserve readable indentation and line breaks; omit sections that do not apply.
Use canonical names, showing only selected useful aliases for frequent or long compound flags, such as `--environment (--env,-e)` and `--workspace (--ws,-w)`.
Do not decorate every command or flag with aliases; supported shortcuts remain executable without appearing in every reference.
Put any displayed alias immediately beside its canonical name.
Explain a shared flag once per reference; identify the actions it applies to instead of implying universal support.
Keep actual choices, required inputs, meaningful defaults, omission behavior, repeatability, and conflicts inline with the relevant syntax.
Use concrete placeholders such as `<luid>`, `<path>`, and `<open|closed|all>`; define shared `target` or `source` syntax once before using it.
Do not imply that different resources accept identical flags or publish modes.

Retain line breaks between meaningful sections, but remove gratuitous blank lines, repeated headings, and boilerplate.
Compression means removing redundancy, not stripping necessary facts or hiding them behind another help call.
Measure characters and tokens, not line count alone; a single enormous line is not compact help.
Do not assume identical help appended twice to an agent conversation receives a prompt-cache hit.

## What help must explain

Describe user outcomes rather than internal implementation vocabulary.
For example, describe publish as "Publish local content to Tableau"; distinguish a file path from a previously pulled workspace item alongside its selector, not in every action summary.
Clearly distinguish inspect from pull, local changes from remote changes, expanded details from additional records, and previews from execution.
Explain contextual exceptions once where they apply, including environment omission, local-only cache reads, workspace selection, collisions, automatic accepted-job receipt recovery, and synchronous flow publication.
Mention mutation policy without suggesting that discovering or previewing an operation authorizes enabling mutations.

Document useful same-action batches in one compact section per operational reference.
Identify eligible selectors, repeatability, per-item file syntax, shared invocation controls, limits, and unsupported combinations accurately.
Do not advertise batch support on every verb merely because another verb supports it.

Resource-specific notes and examples may clarify concepts such as embedded versus published datasource metadata.
Keep them short and relevant to completing the operation.
Long qualitative workflows, including Pulse metric design, belong in skills and optional references, not help.
Users with resource help should not need skills or trial commands to discover required syntax.
Skills should complement help with judgment, not duplicate the flag manual.

## Learn-once conventions

Use this matrix for conventions shared across resource references, while keeping operation-specific flags and exceptions in their owning help.

| Convention | Public rule |
| --- | --- |
| Exact selectors and project scope | Workbook, datasource, and flow inspect accept `--id`, or `--name` with exactly one of `--project` and `--project-id`; their lists accept `--project-id` as an optional filter. |
| Environment, workspace, and cache | An environment selects the Tableau site and remote target; a workspace selects the local artifact root; `--cache` selects local observations only where supported and never falls back to Tableau. |
| Repetition and batches | Repeat one target-selector dimension for a same-action batch; use `--batch-file` for per-item selector combinations; batching does not create a workflow or infer dependencies. |
| Bounds and presentation | Use `--all` for a bounded complete collection where supported, `--limit` or `--cursor` for bounded pages where supported, `--full` for more detail on the same result, and `--json` to change encoding. |
| Preview, mutation, and waiting | Use `--preview` for a no-change plan; mutation policy still governs execution; use `--no-wait` only on supported publish or pull actions, which return local recovery status instead of silently replaying work. |
| Site mutation consent | Use `tadx mutation status --environment <alias>` to inspect consent for the canonical server and exact site; use `tadx mutation set --environment <alias> --enabled=<boolean>` with `true` or `false` only after explicit authorization for that persisted site setting. |
| Managed policy recovery | Use `tadx policy samples --output <directory>` for exclusive candidate files, `tadx policy validate <file>` for schema and ID checks, and `tadx policy status [--full]` for the fixed protected path. |
| Outcomes and recovery | Preserve confirmed identities and per-item results; inspect the exact job or operation status after an unknown outcome, and do not blindly repeat a consequential action. |
| Genuine exceptions | Catalog reads are live-only; Pulse follower reads use a fixed bounded result; `job --operation-id` identifies a local invocation while `job --id` identifies a Tableau job. |
| Local inventory | Environment and workspace inventories accept `--all` for every row within the 10,000-record bound; defaults remain bounded pages, and `--full` still controls detail only. |

## Implementation and acceptance

Derive executable command and flag inventory from the actual Cobra tree, with explicit navigation and operational-reference boundaries.
Keep factual syntax synchronized with validation through automated command and flag coverage checks.
Keep one maintained definitions source, rendering complete resource references and focused action subsets from it rather than maintaining copies at each level.
Help must work without configuration, authentication, credential access, Tableau calls, or side effects.

The approved content reference establishes the presentation pattern for every category.
Content contains workbook, datasource, flow, and project; lineage and asset labels belong to `catalog`.
Admin membership and shared label resources use one hyphenated resource word to keep paths within three words after `tadx`.
Verify:

- Root and category navigation expose every applicable resource and operation without expanding descendant manuals.
- Resource references contain every supported action and its required syntax, while excluding unrelated sibling resources.
- All verb-help paths render the focused subset of their owning operational reference, including aliases and all supported help spellings.
- Required inputs, choices, defaults, constraints, batch support, and shared-flag applicability match executable behavior.
- Direct-action categories, mixed workspace navigation, standalone utilities, and the admin roll-up follow the same standard.
- Help calls remain side-effect-free even with invalid configuration or supplied mutation arguments.
- Snapshot and size-regression tests protect approved output, including long categories and Pulse; inspect the built CLI output as well as test results.

Record line and character counts and the tokenizer used for token measurements; label estimates explicitly.
Choose output budgets from approved complete samples, not arbitrary truncation limits that omit capabilities.
When command structure changes, update CONTRIBUTING.md, the build-skill pointer, and affected operating Guidance, then regenerate affected command references through their generators.
Do not modify auto-generated documentation by hand.

## Canonical command-tree paths

These placement decisions are separate from the category-neutral help rules above.
`content` retains workbook, datasource, flow, and project operations.
`catalog lineage` owns lineage capture, and `catalog label` owns labels attached to assets.
Shared label value and category definitions remain under `admin`.
Keep `cache` for local cached observations and `catalog` for upstream Tableau metadata, tagging, lineage, and asset labels.

| Resource | Canonical path |
| --- | --- |
| Lineage capture | `catalog lineage` |
| Asset labels | `catalog label` |
| Group membership | `admin group-member` |
| Shared label values | `admin label-value` |
| Shared label categories | `admin label-category` |

Retain supported verbs and semantics for these paths.
Do not add commands, rename unrelated flags, or invent compatibility paths as a side effect of help work.
