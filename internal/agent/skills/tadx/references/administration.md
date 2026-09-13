# Tableau administration concepts

## Identity and membership

A site role is a licensing/access ceiling, not an authentication method or a content permission grant.
Authentication options depend on the site's identity configuration; do not guess a different identity provider after a rejection.
Usernames are exact site identities; full names and email metadata are not interchangeable identity selectors.
Removing a user from a site and deleting a group are different from removing a membership.

Membership replacement declares the entire desired direct set; an empty set removes every direct member.
Use additive/removal membership operations when unrelated members must remain.
Group synchronization and external directory policies can constrain supported changes.

## Permissions and project scope

Permission operations change explicit rules, not computed effective access.
Site roles, group membership, deny rules, inheritance, and project locks can affect what a user can actually do.
A successful rule change does not verify the user's complete access.
Use the resource-specific capability vocabulary in permission help, not an approximate UI role name.

Project metadata exposes available `content_permissions` and `controlling_permissions_project_luid` evidence.
An omitted controller is unknown, not proof of independent permissions.
Project defaults affect a broader scope than a rule on one workbook or datasource; do not substitute them without authorization.
Changing a rule requires separately authorized create/delete steps, not an atomic permission update.

Project deletion does not promise to preserve descendants or content.
Inspect relevant dependents before deleting a populated project; a preview is not a complete cascade inventory.

## Shared labels versus attachments

`admin label-value` and `admin label-category` manage shared vocabulary.
`catalog label` applies or removes labels on assets.
Deleting an attachment is not deleting its shared definition or the underlying asset.
See [Catalog metadata](catalog.md) for description inheritance and label identity.
