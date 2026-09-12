# Batch actions and script results

## Available options

| Option | Use |
| --- | --- |
| `--json` | Emit the normal structured result as JSON for shell parsing. |
| `--full` | Expand bounded detail without changing the operation or output encoding. |
| Repeated selector | Vary one supported target dimension for up to 100 selections in order. |
| `--batch-file <file.json>` | Repeat one supported action with different per-item flags. |
| `args: ["value"]` in a file item | Supply positional inputs where that action supports them. |
| `--preview` | Preview the whole selection on actions that support preview. |

Batch support covers content and project lifecycle, datasource schema, lineage, administration, Pulse, workspace/profile actions, guidance installation, and selected scoped lists and utilities.
Category and leaf help identify each supported selector, positional input, and batch-file option.
Vary only one selector dimension: for group membership, repeat users for one group or groups for one user.
Use explicit file rows for different user/group pairs; TADX does not infer their pairing.
Interactive login, global policy/default changes, and release installation remain single operations.
Capability details expose `supports_batch` for feature inventory.

## Supply different settings

The file contains one `items` array of objects whose keys are canonical long flag names without `--`.
Item values override shared action flags; arrays supply repeatable flags.
An explicit empty array clears an inherited repeatable value; it does not mean "keep the shared value."
An `args` array replaces inherited positional arguments on commands that support positional batch rows.
Use 1-100 items in a regular UTF-8 JSON file no larger than 1 MiB; expanded selections also stay within 100.
Duplicate object keys and identical item selections are rejected.
Set config, preview, force, and output mode on the command, never inside an item.
Environment also stays on the command, except for utilities whose help explicitly permits per-item environments.
File items cannot select a different command or refer to earlier results.

For different permission sets on one project, a file can contain:

```json
{"items":[
  {"principal-username":"analyst1@tadx.net","capability":["Read","Write"],"mode":"Allow"},
  {"principal-username":"analyst2@tadx.net","capability":["Read"],"mode":"Allow"}
]}
```

```text
tadx admin permission create --env dev --kind project --id <project-luid> --principal-type user --batch-file permissions.json --preview
```

After approval, omit preview to perform those same operations.
Preview does not grant mutation authorization or enable the mutation gate.

## Use results without redundant turns

Batch items run sequentially and share the command's authenticated session for the same target and credential.
Flag syntax is checked before dispatch; each action retains its normal validation and drift checks.
Independent item failures do not stop later items; cancellation skips unfinished items.
A partial failure returns a nonzero exit code and preserves per-item results and errors.
The operation is not atomic; inspect unknown write outcomes and retry only the unfinished items.

For steps that depend on newly created objects, use a short script to parse `--json` results and pass their IDs to later commands.
Check every exit code before using its result; retain confirmed identities if a follow-up fails.
PowerShell supports `ConvertFrom-Json`; Python's JSON library and jq support other shells.
No TOON conversion utility is required.
Use TOON when the agent needs to read and reason about the answer, and stop scripting when a result requires a new decision or authorization.
