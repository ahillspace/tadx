# Command structure and help standard

Status: approved design; content help pilot implemented for review, remaining categories pending.
This is the single standard for public command structure and help construction.
Use this standard for new help work; complete the rollout before updating the operating skills to promise it across all categories.

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
| `tadx <category> <resource> <verb> -h` | The same complete resource reference, not a separate verb-only document. |

Root help must expose resource paths so users can go directly to a resource reference without first requesting category help.
Category navigation does not promise complete operation syntax; resource help does.
Resource help must stand alone without assuming root or category help was already read.
Do not include sibling-resource operations or options in a resource reference.
Every verb under the same resource resolves to the same reference, regardless of supplied argument values.
Do not build progressively larger verb-specific help or require another help call to learn a necessary flag or constraint.

Apply the same rule structurally across the CLI:

- `pulse -h` introduces definitions and metrics; each resource gets its own complete reference, mirrored by its verbs.
- `auth -h` and `env -h` already sit above executable actions, so they provide complete operational references mirrored by those actions.
- `workspace -h` covers its direct actions and includes a navigation entry for `artifact`; `workspace artifact -h` covers artifact actions.
- Standalone commands such as `search` show their own complete help.

Bare `tadx` remains the session overview, not an alias for `tadx -h`.

## Compact presentation

Follow gh-axi's compact usage, command inventory, action flag groups, notes, and examples rather than concatenating conventional leaf manuals.
TADX additionally supplies short resource and verb descriptions where Tableau terminology does not explain the operation.
Use this order for operational references:

```text
usage: tadx <category> <resource> <verb> [flags]
actions:
  verb (alias): short explanation
flags{shared}:
  compact shared selectors and applicable options
flags{verb}:
  compact action-specific options and inline constraints
notes:
  essential behavior not conveyed by syntax
examples:
  a few complete, useful commands
```

Use comma-separated option groups rather than a paragraph per flag.
Put aliases immediately beside their canonical command or flag, such as `datasource (ds)` and `--environment (--env, -e)`.
Do not append repeated "alias:" explanations.
Explain a shared flag once per reference; identify the actions it applies to instead of implying universal support.
Keep actual choices, required inputs, meaningful defaults, omission behavior, repeatability, and conflicts inline with the relevant syntax.
Use concrete placeholders such as `<luid>`, `<path>`, and `<open|closed|all>`, not unexplained "target options" placeholders.
Do not imply that different resources accept identical flags or publish modes.

Retain line breaks between meaningful sections, but remove gratuitous blank lines, repeated headings, and boilerplate.
Compression means removing redundancy, not stripping necessary facts or hiding them behind another help call.
Measure characters and tokens, not line count alone; a single enormous line is not compact help.
Do not assume identical help appended twice to an agent conversation receives a prompt-cache hit.

## What help must explain

Describe user outcomes rather than internal implementation vocabulary.
For example, describe publish as "Publish local content to Tableau"; distinguish a file path from a previously pulled workspace item alongside its selector, not in every action summary.
Clearly distinguish inspect from pull, local changes from remote changes, expanded details from additional records, and previews from execution.
Explain contextual exceptions once where they apply, including environment omission, local-only cache reads, workspace selection, collisions, and server-side jobs that still wait for completion.
Mention mutation policy without suggesting that discovering or previewing an operation authorizes enabling mutations.

Document useful same-action batches in one compact section per operational reference.
Identify eligible selectors, repeatability, per-item file syntax, shared invocation controls, limits, and unsupported combinations accurately.
Do not advertise batch support on every verb merely because another verb supports it.

Resource-specific notes and examples may clarify concepts such as embedded versus published datasource metadata.
Keep them short and relevant to completing the operation.
Long qualitative workflows, including Pulse metric design, belong in skills and optional references, not help.
Users with resource help should not need skills or trial commands to discover required syntax.
Skills should complement help with judgment, not duplicate the flag manual.

## Implementation and acceptance

Derive executable command and flag inventory from the actual Cobra tree, with explicit navigation and operational-reference boundaries.
Keep factual syntax synchronized with validation; use shared presentation metadata for concise explanations rather than copying help strings between levels.
Help must work without configuration, authentication, credential access, Tableau calls, or side effects.

Implement and measure the content pilot first for user review before applying the pattern to other categories.
The pilot changes help for workbook, datasource, flow, and project without relocating other commands or rolling up admin paths.
Existing commands outside the pilot must remain discoverable and executable until their placement is settled.
Then verify:

- Root and category navigation expose every applicable resource and operation without expanding descendant manuals.
- Resource references contain every supported action and its required syntax, while excluding unrelated sibling resources.
- All verb-help paths mirror their owning operational reference, including aliases and all supported help spellings.
- Required inputs, choices, defaults, constraints, batch support, and shared-flag applicability match executable behavior.
- Direct-action categories, mixed workspace navigation, standalone utilities, and the admin roll-up follow the same standard.
- Help calls remain side-effect-free even with invalid configuration or supplied mutation arguments.
- Snapshot and size-regression tests protect approved output, including long categories and Pulse; inspect the built CLI output as well as test results.

Record line and character counts and the tokenizer used for token measurements; label estimates explicitly.
Choose output budgets from approved complete samples, not arbitrary truncation limits that omit capabilities.
Once implemented, replace the old descendant-expansion rules in CONTRIBUTING.md with a link here, update the build-skill pointer and affected operating Guidance, and regenerate affected command references through their generators.
Do not modify auto-generated documentation by hand.

## Agreed command-tree changes

These placement decisions are separate from the category-neutral help rules above.
`content` retains workbook, datasource, flow, and project operations.
Lineage, tagging, and label operations move outside `content` once their destinations are decided.
Keep `cache` for local cached observations and `catalog` for upstream Tableau metadata.

| Current resource path | New resource path |
| --- | --- |
| `admin group member` | `admin group-member` |
| `admin label value` | `admin label-value` |
| `admin label category` | `admin label-category` |

Retain supported verbs and semantics when changing these paths.
Do not add commands, rename unrelated flags, or invent compatibility paths as a side effect of help work.
