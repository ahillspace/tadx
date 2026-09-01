# Tableau API documentation captures

These files were scraped from official Tableau documentation for targeted local contract research.
They are reference captures, not first-party product requirements.
Do not read any large capture end to end.

## Search workflow

1. Choose the exact API family from the table below.
2. Use `rg` to find the exact operation heading, type, or field.
3. Read only the matching section and any shared concept it cites.
4. Record the source URL, API version, bounded section, and canonical Git blob digest in `docs/evidence/`.
5. Verify against current official Tableau web documentation when the capture is missing, unclear, contradictory, or version-sensitive.

Use `git hash-object -- '<path>'` to calculate the canonical Git blob digest.
Do not use a raw filesystem digest because checkout line endings can differ by platform.

## Routing table

| Need | Capture |
|---|---|
| REST lifecycle operations | `tableau_rest_api.md` |
| Metadata API concepts and examples | `tableau_metadata_api.md` |
| Metadata API types and fields | `tableau_metadata_api_reference.md` |
| Migration SDK concepts | `tableau_migration_sdk.md` |
| Migration SDK API reference | `tableau_migration_sdk_api_reference.md` |
| Tableau MCP tool contracts | `tableau_mcp_tools.md` |
| VizQL Data Service contracts | `tableau_vizql_data_service.md` |
| Pulse metric definitions | `pulse_metric_definition.md` |

## Search examples

```powershell
rg -n '^## Delete Workbook$' 'Tableau API Documentation/tableau_rest_api.md'
rg -n '^## Publish Flow$' 'Tableau API Documentation/tableau_rest_api.md'
rg -n -o '.{0,120}parentPublishedDatasourcesConnection.{0,240}' 'Tableau API Documentation/tableau_metadata_api_reference.md'
```

Some Metadata API schema definitions occupy very long physical lines.
Use exact searches with bounded match output so one result cannot consume the agent context.

## Evidence boundary

Captured documentation can establish a docs-only seam.
It does not authorize executable live API code when the capability registry requires captured contract behavior or live verification.
