# Resolve and manage existing Pulse objects

| Action | Purpose | Required and useful flags |
| --- | --- | --- |
| `tadx search <term>` | Discover a named metric, definition, user, or group. | `--environment`, `--type metric\|definition\|user\|group\|pulse\|admin`, `--limit` |
| `tadx pulse definition list` | List definitions or find an exact name. | `--environment`, `--name`, `--datasource-id`, `--limit`, `--all`, `--catalog`, `--full` |
| `tadx pulse definition inspect` | Read one definition. | `--id`, `--environment`, `--catalog`, `--full` |
| `tadx pulse definition pull` | Save a definition artifact in a registered workspace. | `--id`, `--environment`, `--workspace`, `--overwrite` |
| `tadx pulse metric list` | List variants of one definition. | `--definition-id`, `--environment`, `--limit`, `--all`, `--catalog`, `--full` |
| `tadx pulse metric inspect` | Read one metric's saved settings. | `--id`, `--environment`, `--catalog`, `--full` |
| `tadx pulse metric followers` | List subscriptions for one metric. | `--id`, `--environment`, `--catalog`, `--full` |
| `tadx pulse metric follow` | Subscribe one exact user or group. | `--environment`, `--id`, exactly one of `--user-id` or `--group-id`, `--preview` |
| `tadx pulse metric unfollow` | Remove one exact subscription. | `--environment`, `--subscription-id`; or `--id` with one follower selector; `--preview` |
| `tadx pulse metric delete` | Delete a non-default variant. | `--environment`, `--id`, `--preview` |
| `tadx pulse definition delete` | Delete the shared definition and Tableau-managed dependents. | `--environment`, `--id`, `--preview` |

Use exact identities and explicit environments; `--env` aliases `--environment`.
Use live reads before consequential changes.
`--catalog` is local-only and can be stale or incomplete.

## Resolve a named metric

Search a business name, then inspect the returned exact metric LUID and its definition.
If discovery returns a definition, list its metrics and inspect the variant whose full period and population match the request.
A definition LUID is not a metric LUID, and a name alone does not choose a variant.

```text
tadx search --environment '<alias>' --type metric '<business-name>'
tadx pulse definition list --environment '<alias>' --name '<exact-definition-name>' --all --full
tadx pulse metric list --environment '<alias>' --definition-id '<definition-luid>' --all --full
tadx pulse metric inspect --environment '<alias>' --id '<metric-luid>' --full
```

Definition and metric lists default to 25 returned objects, with `--limit` up to 100.
`--all` scans up to 100 provider pages and 10,000 records, failing if the complete result exceeds that bound.
Do not combine `--all` with `--limit`.
`more_available` means the rendered result is bounded; use `--all` when completeness is required.
Exact definition `--name` searches across provider pages rather than filtering one page.
Use `--datasource-id <luid>` to restrict definitions to the selected datasource before the returned-result limit.
Filtering may still traverse provider pages; it reduces returned context, not necessarily upstream requests.
Search supports `--limit` up to 2,000; narrow the query if more results remain beyond that bound.

## Pull and assess edits

```text
tadx pulse definition pull --environment '<alias>' --id '<definition-luid>' --workspace '<workspace-name>'
```

Pull writes a local artifact and has no preview flag.
`--overwrite` can replace dirty local work; use it only when that replacement is intended.
There is no definition update or publish-from-file command.
Inspect the definition and requested edit, then report an unavailable capability without silently creating a replacement.
Changing a pulled artifact does not change Tableau.
A replacement requires explicit authorization and consideration of existing metrics and followers.

## Resolve follower identities and change subscriptions

Subscriptions belong to an exact metric variant.
Resolve users and groups in the same environment before follow or unfollow:

```text
tadx admin user inspect --environment '<alias>' --name '<exact-username-or-email>' --full
tadx admin group inspect --environment '<alias>' --name '<exact-group-name>' --full
tadx pulse metric followers --environment '<alias>' --id '<metric-luid>' --full
```

For approximate or conceptual names, use `tadx search --type user` or `--type group` first, then inspect the chosen exact LUID.
Usernames, emails, and external group IDs are not subscription LUID selectors.

```text
tadx pulse metric follow --environment '<alias>' --id '<metric-luid>' --user-id '<user-luid>' --preview
tadx pulse metric follow --environment '<alias>' --id '<metric-luid>' --group-id '<group-luid>' --preview
tadx pulse metric unfollow --environment '<alias>' --subscription-id '<subscription-luid>' --preview
tadx pulse metric unfollow --environment '<alias>' --id '<metric-luid>' --user-id '<user-luid>' --preview
```

Choose exactly one follower selector; `--group-id` can replace `--user-id` in the metric-based unfollow form.
Do not combine subscription-ID selection with metric or follower selectors.
For an authorized change, execute the reviewed flags without `--preview` and verify the exact subscription end state.
Creation and analytics requests do not automatically authorize subscriptions.
Do not switch identities or variants after access errors.

## Delete at the requested scope

```text
tadx pulse metric delete --environment '<alias>' --id '<non-default-metric-luid>' --preview
tadx pulse definition delete --environment '<alias>' --id '<definition-luid>' --preview
```

Unfollow removes a subscription, whereas metric delete removes a non-default variant.
The default metric cannot be deleted independently.
Deleting the shared definition is not a substitute for a narrower request.
Inspect visible dependents; deletion previews do not enumerate every Tableau-managed cascade.
After authorized execution, verify the requested end state.
After uncertainty, inspect returned identities before another write.
Do not delete to add a slicer, repair a create error, or resolve a duplicate.
