# Administer users, groups, and permissions

Select the exact environment and site before administration.
Discover candidates with typed search or bounded resource lists, then inspect the authoritative user or group LUID live.
Resolve supported exact selectors through command help; do not assume display names uniquely identify principals.
Use the root Guidance's direct recipes; consult the requested leaf help only when its accepted fields remain unclear.
Apply the root Guidance's mutation gate and preview behavior.
After changes, inspect the resulting identity and relevant state live.

Use `admin permission inspect --kind <kind> --id <resource-luid>` to inspect rules on an exact resource.
Narrow results with supported capability and principal filters.
To inspect project defaults, use a project target and `--default-for <content-kind>`.
Project defaults and resource rules answer different questions; do not present either as a computed effective-access decision.
Use `admin permission create` to add exact rules and `admin permission delete` to remove exact rules when shipped by the installed build.
There is no atomic permission update; changing a rule requires separately authorized create/delete operations with intermediate state.
Select one `--principal-type user|group`, `--principal-id <luid>`, exact `--capability`, and case-sensitive `--mode Allow|Deny`.
These are notation alternatives, not literal pipe-delimited values.
For project defaults, combine `--kind project --id <project-luid>` with `--default-for workbooks`, `datasources`, or `flows`.
Do not assume create/delete overwrites all existing rules; verify the same resource with `--principal-id <principal-luid> --full` in one inspection.
Changing permissions does not itself prove effective access; project locks, defaults, and other rules can affect access.

`admin group update --set-members` replaces direct membership with the repeated `--member-id` values; an empty set removes all direct members.
Do not use it to add one person without preserving existing authorized membership.
Use incremental membership commands for one exact relationship:

```text
tadx admin group member add --environment <alias> --group-id <group-luid> --user-id <user-luid> --preview
tadx admin group member remove --environment <alias> --group-id <group-luid> --user-id <user-luid> --preview
```

Incremental add and remove are idempotent and preserve unrelated direct members.
Run the command without `--preview` after the requested relationship is confirmed.

Use `content project update --project-id <luid>` for supported field edits.
Use one project move form for the requested hierarchy change:

```text
tadx content project move --environment <alias> --project-id <project-luid> --parent-id <parent-project-luid> --preview
tadx content project move --environment <alias> --project-id <project-luid> --top-level --preview
```

Use exactly one destination: `--parent-id`, `--parent`, or `--top-level`.
Project move rejects hierarchy cycles and revalidates both project identities before mutation.
Use `content project delete --project-id <luid>` only for explicitly authorized remote deletion.
Its preview resolves project identity without enumerating descendant deletion effects.
Inspect nonempty-project and descendant behavior before applying; never infer that removing a project preserves its contents.
Unavailable commands remain unavailable; report the exact gap rather than bypassing TADX through an unrelated API.

For repeated offline inventory, refresh users and groups together with explicit scopes.
Permission inspection is live unless its command help explicitly offers `--catalog`.
Catalog collection of permissions does not imply every permission operation supports local reads.
