# tadx CLI improvement proposal: agent ergonomics

## Why this document exists

An agent was asked to migrate one user's content (2 workbooks, 20 published datasources) from one Tableau site to another using nothing but `tadx`.
The run was done deliberately with no skill and no CLAUDE.md, so the only guidance the agent had was the CLI itself: its help text, its flags, and its error messages.
The migration succeeded for 19 of 20 datasources and 1 of 2 workbooks.
Everything that slowed it down or blocked it was a place where the CLI knew something the agent could not discover from the CLI.

This is a proposal, not a patch.
Every recommendation below is grounded in the current source (file and line references are given so each point can be verified before any change is made).
The three themes match the three friction areas the test surfaced:

1. Tool and capability discoverability and documentation.
2. Getting around shell timeouts on slow, extract-bound operations.
3. Making it obvious what to do when a workbook references a published datasource.

A short priority table is at the end.

---

## Theme 1: discoverability and documentation

### What happened during the test

The agent concluded, twice, that `tadx` was a read-only or pull-only tool.
It is not.
It has full write verbs (`project create/update`, `datasource publish/delete`, `workbook publish/delete`), but they are gated behind the environment variable `TADX_ENABLE_MUTATIONS=1`, and the way that gate is implemented makes the write side of the tool invisible to anyone who does not already know the variable exists.

### Ground truth

The gate is read once at startup and threaded down as a bool (`cmd/tadx/main.go:13-14`):

```go
exitCode := app.Run(ctx, os.Args[1:], os.Stdout, app.Options{
    MutationsEnabled: os.Getenv("TADX_ENABLE_MUTATIONS") == "1",
})
```

Mutation subcommands are registered but hidden, not removed.
They are always wired into the command tree; the gate only sets `Hidden`:

- `internal/cli/content/datasource_lifecycle.go:72` (publish), `:142` (delete)
- `internal/cli/content/project.go:48` (create), `:56` (update)
- `internal/cli/content/command.go:152` (workbook publish)
- `internal/cli/content/workbook_delete.go:24` (workbook delete)

The dependencies for those commands are wired unconditionally regardless of the flag (`internal/app/content_remote.go:46-47`), and each command's `RunE` calls the mutator directly with no per-command runtime check on `MutationsEnabled`.

Two consequences follow, and both are confusing:

1. With the flag unset, a hidden mutation command still resolves and still runs.
It does not print "unknown command"; it just executes while being absent from help.
2. A genuinely nonexistent command produces a Cobra usage error, "unknown command ... for ...", from `root.Find` (`internal/app/app.go:112-114`).

So a hidden-but-runnable verb and a typo look completely different from each other, and neither state tells the agent the truth: that the capability exists and how to reveal it.

The one surface that should have rescued the agent, the `capability` command, is itself gated for mutation discovery:

- `capability list` silently drops mutation rows when the flag is off (`internal/app/app.go:527-528`: `if definition.RemoteMutation && !includeMutations { continue }`).
- `capability list --mutation` refuses outright (`actions/capability/list/action.go:39-40`): `mutation discovery is disabled; set TADX_ENABLE_MUTATIONS=1`.

The registry itself already knows every mutation capability (`internal/capability/registry_gen.go`, each `RemoteMutation: true` row).
The CLI filters that knowledge out at the exact moment an agent would use it to learn what the tool can do.

This is a chicken-and-egg failure.
The only affordance that names the environment variable is an error you can only trigger by already knowing to pass `--mutation`, which you would only pass if you already knew mutations existed.

### A second, smaller discoverability trap: flag names

Project selection is spelled three different ways across verbs, which produced "unknown flag" errors during the test:

- `list` uses `--project-name` (`internal/cli/content/datasource_inventory.go:51`, `internal/cli/content/workbook_inventory.go:36`).
- `get` uses `--project` as a slash-delimited path (`datasource_inventory.go:79`, `workbook_inventory.go:59`).
- `publish` uses `--project` and `--project-id` (`datasource_lifecycle.go:61-62`, workbook publish `command.go:184-185`).

An agent that learns `--project-name` on `list` will guess wrong on `get` and `publish`, and vice versa.

### A third: the `--artifact` contract is learnable only by trial

`--artifact` requires a workspace-relative, slash-delimited managed directory of the exact shape `artifacts/<kind>/<name>`.
It rejects absolute paths, backslashes, non-clean paths, and it is not the `.tdsx`/`.twb` file (`internal/cli/content/command.go:192-201`).
During the test this took three failed attempts to learn, because the rule lives only in the error string, not in any example the agent could read up front.

### Recommendations

1. Separate discovery from execution.
Discovery is read-only and safe; it should never be gated.
Let `capability list` always include mutation rows, marked with a status such as `gated: true` or `enabled: false`, regardless of `TADX_ENABLE_MUTATIONS`.
Let `capability list --mutation` list them rather than refuse.
Keep the environment variable as the gate on actually applying a mutation, not on learning that mutations exist.

2. Make gated verbs visible but inert, instead of hidden and silently runnable.
Show them in help with a suffix like "(requires TADX_ENABLE_MUTATIONS=1 to apply)".
Have their `RunE` refuse with a clear, actionable error when the flag is off, rather than running while hidden.
This replaces two confusing states (invisible-but-runs, absent-errors) with one honest state (visible, and it tells you exactly how to enable it).

3. Put the operating model in root help.
`tadx --help` should state, in a few lines, that the tool is preview-first (mutations preview by default and require `--apply`), that write verbs require `TADX_ENABLE_MUTATIONS=1`, and that `tadx capability list` is the way to enumerate everything.
An agent reads root help first; that is the highest-leverage place for these three facts.

4. Normalize project-selection flags.
Accept `--project`, `--project-name`, and `--project-id` as aliases on every verb that selects a project, or at minimum document the matrix in each command's help.
Consistency here removes a whole class of "unknown flag" retries.

5. Ship an `AGENTS.md` (or equivalent) at the repo root.
A short, example-driven reference covering the mutation gate, the preview/apply model, the `--artifact` managed-directory contract with one worked example, and the project-flag matrix.
This is the documentation the test was missing, and it belongs with the tool, not in each caller's private config.

---

## Theme 2: shell timeouts on slow operations

### What happened during the test

Pulls and publishes that carry extracts routinely exceed the 120 second timeout of a typical agent shell tool.
The agent only completed the 20-datasource publish loop by moving it to a background job and polling an output file.
Nothing in the CLI told it to do that; it had to infer the workaround after hitting the wall.

### Ground truth

Extract-bound pulls and publishes are long by nature.
The workbook pull path downloads dependencies serially and aborts the whole pull if any single dependency download fails (`actions/workbook/pull/action.go:100-104`), so a slow multi-dependency pull is both long and all-or-nothing.
A `--as-job` flag exists on publish (`datasource_lifecycle.go` publish flag set), which is the right primitive, but it is not surfaced as the default guidance for extract-heavy work, so an agent does not know to reach for it.

### Recommendations

1. Document the background-execution pattern next to the slow commands.
The help for `pull` and `publish` should note that extract-bound runs can take minutes and show the recommended non-blocking invocation (background the process, write to an output file, poll it).
This is the single change that would have saved the most wall-clock time in the test.

2. Surface `--as-job` as the recommended path for extract-heavy artifacts.
Either default to job submission when the artifact carries an extract, or print a one-line hint pointing at `--as-job` when a synchronous publish is likely to be long.

3. Emit an early heartbeat or progress line.
A harness cannot distinguish "slow but working" from "hung" when a command is silent for two minutes.
A periodic progress line (bytes transferred, phase, dependency N of M) lets a caller wait intelligently instead of killing the process.

4. Make long operations resumable and partial-failure tolerant.
A workbook pull that aborts on one failed dependency (see Theme 3) forces a full restart.
A resumable pull, or one that records what it already fetched, turns a 401 on dependency 12 of 15 into a cheap retry rather than a full redo.

---

## Theme 3: workbooks that reference published datasources

### What happened during the test

One workbook was portable and published cleanly.
The other referenced a published datasource whose project the agent's credentials could not read, and it could not be moved.
Getting from "pull the workbook" to "understand why it will not publish on the target site" took several failed attempts, because the ordering rule (publish the datasources first, then the workbook) is real and is partly encoded in the tool, but is not presented as a single obvious plan.

### Ground truth (this theme is the closest to already being right)

Detection at pull time is solid.
A workbook is tagged `portability: source-site-bound` if and only if the Metadata API returns at least one published-datasource reference (`actions/workbook/pull/action.go:73-91`), and the count and dependency list are recorded.

`--include-pds` acquires the direct published-datasource dependencies as sibling artifacts in a bundle, without recursion (`internal/cli/content/command.go:134`; acquisition loop `actions/workbook/pull/action.go:98-116`).

A pre-publish warning already exists, and it is good.
When a source-site-bound workbook is published to a different environment or site, the plan carries a warning (`actions/workbook/publish/action.go:121-127`) that says, in effect, this workbook references N published datasources bound to the source site, publishing to the target will fail until those datasources exist there, acquire them with `--include-pds` and publish them to the target first.

The dependency list is persisted in the artifact manifest `metadata.json` under `published_datasources`, each entry carrying `luid`, `name`, `source_site`, and `local_artifact_path` (`internal/artifact/workbook.go:61-66`).

So the raw material for a clean workflow is all present.
The gaps are in how it is delivered to an agent:

1. The warning is prose in the plan output only.
It does not block the apply, it is not a machine-readable `corrective_action`, and it does not include the structured list of datasource names and luids the agent needs to act.
2. There is no Apply-side guard.
The real failure, Tableau 400011 / PublishingException "Datasource '<name>' not found", surfaces generically as `workbook.publish.failed` (`actions/workbook/publish/action.go:171-182`) with no special-casing of the missing-datasource cause.
3. There is no command to list a pulled workbook's dependencies without re-pulling.
`lineage` only offers `pull` (`internal/cli/content/lineage.go:16-20`); the names and luids exist only inside `metadata.json`, captured at pull time.
4. `--include-pds` is all-or-nothing.
One unreadable dependency project returns a 401 wrapped as `workbook.pull.dependency-download` (`actions/workbook/pull/action.go:100-104`) and aborts the entire pull, with a generic corrective action that does not name the datasource or say it is an access/project-scope problem.

### Recommendations

1. Promote the existing warning into structured guidance.
Attach the dependency list (names and luids) and a machine-readable `corrective_action` to the preview output, so an agent can act on it without parsing English.
The data is already in `metadata.json`; this is a delivery change, not a detection change.

2. Offer an ordered publish plan, or a single sequenced command.
When publishing a source-site-bound bundle, emit the explicit order ("publish these N datasources, then this workbook"), ideally behind a `workbook publish --with-deps` that sequences datasource-first automatically.
This turns the multi-step rule the agent had to discover into one call.

3. Map the cross-site failure to a specific error.
When apply fails with 400011, return a distinct id such as `workbook.publish.missing-datasource` that names the datasource, instead of the generic `workbook.publish.failed`.
An agent can recover from a named cause; it cannot recover from a generic one.

4. Add a read-only dependency-list command.
Something like `content workbook deps --artifact <dir>` that reads `published_datasources` from a pulled artifact's `metadata.json` and lists names, luids, and whether each was acquired.
This lets an agent plan the datasource-first sequence without a second pull.

5. Make `--include-pds` partial-failure tolerant.
When one dependency cannot be downloaded, name the datasource, say plainly that it is an access or project-scope issue, and offer to continue with the dependencies that were reachable rather than aborting the whole pull.

---

## Priority summary

| Priority | Change | Theme | Payoff |
|---|---|---|---|
| P0 | Ungate mutation discovery in `capability list` (mark gated, never hide) | 1 | Kills the chicken-and-egg; the tool stops looking read-only |
| P0 | Structured, machine-readable dependency list + corrective_action on the source-site-bound warning | 3 | Agent can act on the datasource-first rule without guessing |
| P1 | Gated verbs visible-but-inert with an actionable "set TADX_ENABLE_MUTATIONS=1" error | 1 | One honest state instead of two confusing ones |
| P1 | Document background execution and surface `--as-job` for extract-heavy work | 2 | Removes the biggest wall-clock cost |
| P1 | `workbook publish --with-deps` sequenced publish, or an explicit ordered plan | 3 | One call instead of a discovered multi-step dance |
| P2 | Normalize project-selection flags across verbs | 1 | Removes "unknown flag" retries |
| P2 | Map 400011 to `workbook.publish.missing-datasource` naming the datasource | 3 | Recoverable named cause |
| P2 | `content workbook deps` read command over a pulled artifact | 3 | Plan without re-pulling |
| P2 | Root-help operating-model summary and an `AGENTS.md` at repo root | 1 | The documentation this test was missing |
| P2 | Partial-failure-tolerant `--include-pds` with named dependency errors | 2, 3 | Turns a full restart into a cheap retry |

## Known bug found along the way (separate from the three themes)

The datasource collision check rejects any datasource whose name contains `&` or `,`, rather than escaping it.
The name is placed into a Tableau REST filter `name:eq:<name>` after an explicit guard (`internal/tableau/datasource/client.go:371-374`):

```go
if strings.ContainsAny(field.value, ",&") {
    return nil, fmt.Errorf("datasource filter %s cannot contain ampersand or comma", field.name)
}
```

All four collision modes (`--create`, `--overwrite`, `--append`, `--replace`) run this same check unconditionally (`actions/datasource/publish/action.go:169`, filter call `:190`), and there is no escape hatch flag (`--force`, `--skip-collision`) to bypass it.
During the test this permanently blocked one datasource ("Hubbell Global Sales & Pipeline Final") from being migrated, because the name could not be changed faithfully and there was no way around the check.
The fix is to URL-encode the filter value (the surrounding `url.Values` would encode it correctly if the guard did not fail first), and optionally to add a `--skip-collision` escape hatch for the cases where a lookup genuinely cannot be formed.
