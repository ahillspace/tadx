# Command integration checklist

Use this checklist for a new command or a change to its user-visible capability, policy, or discovery behavior.

- Add or update canonical facts and implementation metadata in `internal/capability/definitions.go` or `metadata_definitions.go`, including the stable capability ID, command path, disposition, safety, selectors, availability, and evidence.
- Register the actual Cobra command and composition wiring, annotate it with the canonical capability ID, and retain command-tree binding coverage.
- Add collision-free shorthands in `internal/cli/shorthand.go` for new names longer than three characters, unless a documented reason exempts them.
- Put syntax, flags, constraints, defaults, and examples in the owning help reference and command structure documentation; update affected root, category, resource, or verb help and `CONTRIBUTING.md` as appropriate.
- Update installed Guidance under `internal/agent/skills/tadx/` or `tadx-pulse/` only when operating guidance changes; the bundle is embedded from `internal/agent/skills`.
- Run `go generate ./internal/capability` after registry changes; this generates the Markdown reference, JSON inventory, and capability map data block.
- When the public capability map or website content changes, inspect `scripts/build-site.mjs` and its public-file allowlist; the build copies the map as `capabilities.html`, the generated JSON, and authored `site/index.html` and `site/security.html`.
- Review `internal/managedpolicy/policy.go` and its tests when capability IDs or classifications change; all standard templates include reads, while mutation coverage depends on the template.
- Generate policy samples with the candidate build and check the new ID's intended inclusion or exclusion in each template; sample files are generated snapshots, not a second source of truth.
- Do not migrate or broaden an existing customer policy during an upgrade; new IDs remain denied until an administrator explicitly adds them.
- Keep any change to an active local managed policy within existing explicit authorization for that exact test target; prefer isolated fixtures and never disable a real policy or consent gate to make tests pass.
- Before authorized live testing, resolve the active `managed-policy.json`, preserve custom rules, and add only the required IDs; preserve the original file for restoration when the change is temporary, and verify the resulting policy status.
- Add end-to-end acceptance coverage through the command entrypoint for visible syntax, binding, compact and full output, policy denial and recovery, and no-write preview behavior where applicable; assess whether agent operating Guidance or examples need before-and-after workflow coverage.
- For paged commands, keep raw provider continuation tokens internal; preserve supported user-facing `--cursor` continuation and contextual follow-up commands when part of the command contract.

Useful checks include `internal/capability/generate_test.go`, `internal/capability/map_inventory_test.go`, `internal/cli` binding and help tests, `internal/managedpolicy/policy_test.go`, the managed-policy app E2E tests, and `scripts/tests/site_test.mjs`.
Local test-policy files are not maintained repository inputs; do not make this checklist depend on an untracked developer directory.

For changes intended to reduce agent effort, define the real user task and success criteria before implementation, then compare baseline and candidate runs with fresh Luna agents at medium reasoning when live testing is authorized.
Record the binary revision, task outcome, command count, redundant discovery or verification calls, partial results, and cleanup; passing code tests alone does not establish a workflow improvement.
Use the repository's `tadx-explore` workflow for assigned free-form exploration, keeping its ignored run evidence separate from maintained guidance.
