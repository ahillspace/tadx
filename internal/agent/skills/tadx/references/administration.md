# Administer users, groups, and permissions

Select the exact environment and site before administration.
Discover candidates with typed search or bounded resource lists, then inspect the authoritative user or group LUID live.
Resolve supported exact selectors through command help; do not assume display names uniquely identify principals.
Use `admin user` or `admin group` help for the requested mutation and its accepted fields.
Apply the root skill's mutation gate and preview behavior.
After changes, inspect the resulting identity and relevant state live.

Use `admin permission inspect --kind <kind> --id <resource-luid>` to inspect rules on an exact resource.
Narrow results with supported capability and principal filters.
To inspect project defaults, use a project target and `--default-for <content-kind>`.
Project defaults and resource rules answer different questions; do not present either as a computed effective-access decision.
The current permission category exposes inspection, not permission mutation.
For another administrative operation, check capability availability instead of inventing a command or bypassing TADX through an unrelated API.

For repeated offline inventory, refresh users and groups together with explicit scopes.
Permission inspection is live unless its command help explicitly offers `--catalog`.
Catalog collection of permissions does not imply every permission operation supports local reads.
