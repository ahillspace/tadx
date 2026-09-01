# Tableau API documentation routing

The repository-local Tableau API captures are the first search surface for upstream contracts.
They are large reference files and must never be read end to end.

## Routing order

1. Search the relevant local file for the exact REST operation heading or Metadata API field.
2. Read only the matching section and any shared concept it explicitly cites.
3. Record the source URL, API version, exact heading or bounded line range, and canonical Git blob digest in `docs/evidence/`.
4. Use official Tableau web documentation when the local capture is missing, unclear, contradictory, or version-sensitive.

Use only official Tableau sources for web verification.
Documentation evidence does not authorize live wiring by itself.
The exact request, response, error, pagination, and identity contract still needs the evidence level required by the capability registry.

## Local routing table

| Need | Local file |
|---|---|
| REST lifecycle operations | `Tableau API Documentation/tableau_rest_api.md` |
| Metadata API concepts and examples | `Tableau API Documentation/tableau_metadata_api.md` |
| Metadata API types and fields | `Tableau API Documentation/tableau_metadata_api_reference.md` |
| Migration SDK only for an admitted migration task | `Tableau API Documentation/tableau_migration_sdk_api_reference.md` |

Use `rg -n '^## <operation>$'` for REST headings.
Use an exact `rg` query with bounded output for Metadata API fields because parts of the schema capture are stored on very long physical lines.

Useful REST headings include `Delete Data Source`, `Delete Flow`, `Delete Workbook`, `Download Flow`, `Publish Flow`, `Query Flow`, `Query Flows for a Site`, and `Update Flow`.

## Live verification

The configured development Tableau Cloud site and PAT may be used for opt-in live verification.
Live tests stay outside the standard hermetic suite, use disposable content, and persist only sanitized evidence.
