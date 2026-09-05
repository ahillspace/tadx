# Permission rule mutations

Contract checked against current official Tableau documentation on 2026-09-05.
Verification uses hermetic TLS HTTP tests; no live Tableau mutation was performed.

## Upstream contract

The [permission methods reference](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_permissions.htm) documents workbook, datasource, and project rule writes.
The [flow methods reference](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_flow.htm#add_flow_permissions) documents flow rule writes.
These released REST paths apply to Tableau Cloud and Server.
Flow methods require REST API 3.3 or later.

Create uses PUT on the resource permissions collection with one user or group LUID and one capability/mode pair.
Project defaults use the project default-permissions collection with an explicit workbooks, datasources, or flows suffix.
The XML envelope contains permissions and granteeCapabilities elements.
Default and project requests omit resource qualifier elements.
An exact HTTP 200 response must confirm the requested principal, capability, and mode.
Delete uses the corresponding collection path followed by users or groups, principal LUID, capability, and case-sensitive Allow or Deny.
Deletion requires an empty HTTP 204 response.

## Command semantics

An existing capability can cause Tableau to ignore an add request.
Create therefore rejects an opposite existing mode and treats an identical mode as unchanged.
Changing modes requires an explicit delete followed by create.
No atomic individual-rule update operation is exposed.
Delete treats an absent capability as unchanged and rejects a mismatched mode.

Preview validates exact principal identity and reads the live resource permissions without writing.
Execution re-reads the target rule before the write.
Inherited, unknown, conflicting, and changed state fail before mutation.
Revalidation cannot eliminate changes between the final read and the upstream write.
The commands do not calculate effective access or follow a controlling project automatically.

The shared mutation policy gates both commands.
Uncertain successful responses preserve request IDs and prohibit automatic retry.
Hermetic tests cover escaped paths, supported resource/default combinations, principal validation, malformed responses, unknown outcomes, preview, and revalidation.
