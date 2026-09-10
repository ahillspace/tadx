# Content update REST contract evidence

This record freezes the captured Tableau REST contracts used by content move, rename, ownership, and project hierarchy actions.
The historical local capture is `Tableau API Documentation/tableau_rest_api.md`.
The official source is [Tableau REST API help](https://help.tableau.com/current/api/rest_api/en-us/REST/).
The local capture and its line references are historical provenance; see [capture availability](README.md).

The implementation uses these captured sections:

- Update Data Source at lines 42682 through 42830.
- Update Flow at lines 44046 through 44176.
- Update Flow Owner at lines 44460 through 44540.
- Update Project at lines 45703 through 45767.
- Update Workbook at lines 48313 through 48442.

Workbook update supports an explicit name, project LUID, or owner LUID.
Datasource update supports an explicit name, project LUID, or owner LUID.
Project update supports an explicit parent project LUID, including an empty parent value for a top-level project.
Flow move uses Update Flow with an explicit project LUID.
Flow ownership uses Tableau's recommended Update Flow Owner endpoint with exact flow and user LUIDs.
The captured Update Flow request has no name field, so TADX does not claim or simulate flow rename.

Workbook, datasource, project, and flow move requests use `PUT /api/{version}/sites/{site-luid}/{resource}/{resource-luid}` with an XML body containing only requested changes.
Flow ownership uses `PUT /api/{version}/sites/{site-luid}/flows/{flow-luid}/owner/{user-luid}` with no request or response body.
Each client requires HTTP 200 and verifies the returned resource LUID plus every requested identity field.
An incomplete or contradictory response produces an unknown mutation outcome.
Hermetic HTTP tests freeze each supported request body, response identity, and missing-input rejection.

Actions resolve exact identities, produce a read-only preview, and revalidate authoritative LUIDs before the final request.
Move and rename actions reject exact destination name collisions before mutation.
Project moves reject self-parent and descendant-parent cycles before mutation.
