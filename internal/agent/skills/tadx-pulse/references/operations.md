# Resolve and manage existing Pulse objects

Read this for identity, portability, and subscription scope; resource help owns command syntax.

## Resolve a named metric

Search a business name, then inspect the returned exact metric LUID and its definition.
If discovery returns a definition, list its metrics and inspect the variant whose full period and population match the request.
A definition LUID is not a metric LUID, and a name alone does not choose a variant.

Exact-name and datasource-scoped definition discovery can traverse provider pages; bounded output is not proof of absence.
Reuse definitions with matching semantics, not merely matching names.
Use `tadx pulse metric list --definition-id <definition-luid> --full` for complete saved variant rows and capture `tadx last --full` immediately when a compact result points to more detail.
Use `--all` for all bounded rows or `--limit` for a bounded page; remove `--limit` from an all-rows request instead of combining the flags.
Metric and user selectors must be exact nonempty LUID tokens without embedded whitespace; datasource field IDs and captions follow their own schema rules.

## Pull and assess edits

Pull writes a local artifact; `--preview` checks bundle scope and local conflicts without writing it.
Execution retrieves the specifications again and rechecks conflicts; preview does not prove filesystem write access.
`--overwrite` can replace dirty local work; use it only when that replacement is intended.
Publication is a recreation, with an explicit source-to-destination datasource mapping even on the same site.
TADX Pulse exports are accepted as publication inputs, including saved variants and admitted configuration sections.
Known bookkeeping metadata is converted to the create document, while unsupported sections fail locally instead of being silently dropped.
Publish validates destination fields and creates new definition and metric identities, reporting their mapping and leaving originals untouched.
No followers, principals, values, or insights are transported.
After reviewing the complete plan, remove --preview only for an authorized recreation.
Changing local files does not change Tableau; identity-preserving updates remain unsupported.
For local definition edits, keep `resource.json` and the definition copy in `bundle.json` consistent; publish rejects disagreement before authentication.
Metric variant edits belong in `bundle.json`.

## Resolve follower identities and change subscriptions

Subscriptions belong to an exact metric variant.
Resolve users and groups in the same environment before follow or unfollow.
Usernames, emails, and external group IDs are not subscription LUID selectors.

For an authorized change, execute the reviewed flags without `--preview` and verify the exact subscription end state.
Creation and analytics requests do not automatically authorize subscriptions.
Do not switch identities or variants after access errors.

## Delete at the requested scope

Unfollow removes a subscription, whereas metric delete removes a non-default variant.
The default metric cannot be deleted independently.
Deleting the shared definition is not a substitute for a narrower request.
Inspect visible dependents; deletion previews do not enumerate every Tableau-managed cascade.
After authorized execution, verify the requested end state.
After uncertainty, inspect returned identities before another write.
Do not delete to add a slicer, repair a create error, or resolve a duplicate.
An exact metric 404 is a resource-level missing or inaccessible result in the selected environment; verify that metric and site before changing CLI configuration.
