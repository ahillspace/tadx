# Batch actions and script results

## Available options

| Option | Use |
| --- | --- |
| `--json` | Emit the normal structured result as JSON for shell parsing. |
| `--full` | Expand bounded detail without changing the operation or output encoding. |
| Repeated `--id` | Apply a supported action to up to 100 exact identities in order. |
| `--batch-file <file.json>` | Repeat one supported action with different per-item flags. |
| `--preview` | Preview the whole selection on actions that support preview. |

Batch support covers content and project lifecycle, datasource schema, lineage, admin users/groups/permissions, and Pulse lifecycle actions.
Group member add/remove repeat `--user-id` instead of the shared `--group-id`.
Existing repeated pull IDs and publish artifact selectors remain supported.
Configuration, authentication, installation, global state, and inventory-list commands do not accept batch files.
Capability details expose `supports_batch`; leaf help lists the repeatable selector and batch-file option.

## Supply different settings

The file contains one `items` array of objects whose keys are canonical long flag names without `--`.
Item values override shared action flags; arrays supply repeatable flags.
Use 1-100 items in a regular UTF-8 JSON file no larger than 1 MiB; expanded selections also stay within 100.
Duplicate object keys and identical item selections are rejected.
Set environment, config, preview, force, and output mode on the command, never inside an item.
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
