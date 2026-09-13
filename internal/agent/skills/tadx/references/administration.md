# Administration

## Available actions

All actions accept `--full` for expanded bounded output and `--json` for scripts.
For repeated actions or different settings per item, use [batching](batching.md).
Flags in brackets are optional.
`--env` aliases `--environment`; the environment selects the site for user mutations.
A sole configured environment can be omitted; multiple environments require an explicit target for remote writes.

| Action | What it does | Key and optional flags |
| --- | --- | --- |
| `tadx admin user list` | List users, or collect the selected inventory with `--all`. | `--environment <alias>` [`--name <exact>`] [`--site-role <role>`] [`--limit 1..10000` or `--all`] [`--cache`] |
| `tadx admin user inspect` | Inspect one exact user. | `--id <luid>` or `--name <exact-username>` [`--environment <alias>`] [`--cache`] |
| `tadx admin user create` | Add one user to a site. | `--environment <alias> --name <username> --site-role <role>` and exactly one of `--auth-setting <value>` or `--idp-configuration-id <luid>` [`--identity-pool <name>`] [`--email <address>`] [`--language <code>`] [`--locale <code>`] [`--preview`] |
| `tadx admin user update` | Update exact users. | `--environment <alias>` and `--id <luid>` or `--username <exact>`; one or more of `--full-name`, `--email`, `--site-role`, `--auth-setting`, `--identity-pool`, `--idp-configuration-id`, `--language`, `--locale` [`--preview`] |
| `tadx admin user delete` | Remove exact users from a site. | `--environment <alias>` and repeated `--id <luid>` or `--username <exact>` [`--preview`] |
| `tadx admin group list` | List groups, or collect the selected inventory with `--all`. | `--environment <alias>` [`--name <exact>`] [`--domain <exact>`] [`--limit 1..10000` or `--all`] [`--cache`] |
| `tadx admin group inspect` | Inspect one exact group. | `--id <luid>` or `--name <exact>` [`--environment <alias>`] [`--members`] [`--cache`] |
| `tadx admin group create` | Create one group. | `--environment <alias> --name <name>` [`--minimum-site-role <role>`] [`--external-user-enabled`] [`--preview`] |
| `tadx admin group update` | Update group attributes or replace direct membership. | `--environment <alias> --id <luid>` [`--new-name <name>`] [`--minimum-site-role <role>`] [`--external-user-enabled`] [`--set-members --member-id <user-luid>` repeated] [`--preview`] |
| `tadx admin group delete` | Delete one group without deleting its users. | `--environment <alias> --id <luid>` [`--preview`] |
| `tadx admin group member add` | Add users without replacing other group members. | `--environment <alias> --group-id <luid>` and `--user-id <luid>` or `--username <exact>`; repeat one selector dimension [`--preview`] |
| `tadx admin group member remove` | Remove users without replacing other group members. | `--environment <alias> --group-id <luid>` and `--user-id <luid>` or `--username <exact>`; repeat one selector dimension [`--preview`] |
| `tadx admin permission inspect` | Inspect explicit or project-default permission rules. | `--kind <workbook\|datasource\|flow\|project> --id <resource-luid>` [`--environment <alias>`] [`--default-for <workbooks\|datasources\|flows>`] [`--principal-type <user\|group>`] [`--principal-id <luid>`] [`--capability <name>`] |
| `tadx admin permission create` | Add one exact permission rule. | `--environment <alias> --kind <kind> --id <resource-luid> --principal-type <user\|group> --principal-id <luid> --capability <name> --mode <Allow\|Deny>` [`--default-for <kind>`] [`--preview`] |
| `tadx admin permission delete` | Remove one exact permission rule. | Same selectors as permission create, including exact `--mode` [`--default-for <kind>`] [`--preview`] |
| `tadx content project list` | List projects, or collect the selected inventory with `--all`. | `--environment <alias>` [`--name <exact>`] [`--owner <exact>`] [`--parent-id <luid>`] [`--top-level`] [`--limit 1..10000` or `--all`] [`--cache`] |
| `tadx content project inspect` | Inspect one exact project. | `--id <luid>` or `--project <exact/path>` [`--environment <alias>`] [`--cache`] |
| `tadx content project create` | Create one project. | `--environment <alias> --name <name>` [`--description <text>`] [`--content-permissions <mode>`] [`--parent-id <luid>` or `--parent <exact/path>`] [`--preview`] |
| `tadx content project update` | Update one exact project's name, description, or permission mode. | `--environment <alias>` and one source selector, plus at least one of `--new-name`, `--description`, or `--content-permissions` [`--preview`] |
| `tadx content project move` | Move a project in the hierarchy. | `--environment <alias>` and one source selector, plus exactly one of `--parent-id <luid>`, `--parent <exact/path>`, or `--top-level` [`--preview`] |
| `tadx content project delete` | Delete one exact project. | `--environment <alias> --id <luid>` [`--preview`] |

## Operating rules

Shared label vocabulary is managed with `admin label value list/inspect/update/delete` and `admin label category list/inspect/create/update/delete`.
Applying or removing an asset label belongs to `catalog label`.
Read [Catalog metadata](catalog.md) for these action tables, exact-name selectors, and optional flags.

Reuse known user, group, project, and content LUIDs.
Usernames resolve exactly within the selected site; full/display names and email metadata are not identity selectors.
Permission actions accept `--principal-username` instead of `--principal-id` only with `--principal-type user`.
Repeat `--capability` on permission create/delete to apply several explicit rules with one shared mode.
Results confirm the individual rule changes, not the user's effective access through all groups and inherited policies.
Use `tadx search --type admin <term>` for intent or text discovery, then inspect the exact result.
Use lists for bounded filtered lookup or complete inventory, not as a substitute for search.

Ordinary live user, group, and project lists retrieve a bounded selection without collecting the complete scope or accessing SQLite.
Lists default to 25 rows and accept `--limit 1..10000`, or `--all` for all matching records within 10,000.
When `more_available` is true, use `--all` or narrow the filters.
`--all` requires complete coverage and cannot be combined with an explicit `--limit`.
Live `--all` renders the collection and attempts a cache update; an unfiltered complete collection replaces the resource scope.
A cache write failure preserves the live answer with a warning.
`--full` changes presentation only.
Filtered `--all` collections record observations without claiming complete site coverage.
`--cache` is local-only and never falls back to Tableau.

`admin group update --set-members` replaces all direct membership with the repeated `--member-id` values.
An empty set removes every direct member.
Use `group member add` or `group member remove` for a single relationship so unrelated members remain unchanged.

Permission actions manage explicit rules, not computed effective access.
Choose a resource-specific capability from `tadx admin permission --help`; focused create help contains the same accepted values.
Project capabilities are `ProjectLeader`, `Read`, and `Write`; do not substitute UI labels such as View or Editor.
For project defaults, `--default-for` selects the content kind's capability set.
Project discovery and inspection expose available `content_permissions` and `controlling_permissions_project_luid` metadata, including cache reads.
An omitted controller is unknown, not proof of independent permissions.
Do not automatically replace a content permission change with a project-default change; that expands the affected scope.
Project defaults and resource rules answer different questions.
Project locks, inherited defaults, and conflicting rules can still affect effective access.
There is no atomic permission update, so changing a rule requires separately authorized create and delete operations.

User `--auth-setting` accepts `ServerDefault`, `SAML`, `OpenID`, or `TableauIDWithMFA`.
Availability depends on the site's configured authentication methods.
Use exactly one of that flag or an exact `--idp-configuration-id` for creation; a site role is not an authentication setting.
Preserve provider authentication and synchronization diagnostics instead of guessing another identity configuration.

Project deletion does not promise to preserve descendants or content.
Inspect a nonempty project before deletion and do not infer Tableau's cascade behavior from the preview.
Report when a TADX command does not support the requested operation.
