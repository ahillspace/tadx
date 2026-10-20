# Tableau GraphQL test schema

The schema fixture derives complete type definitions from Tableau's official current Metadata API [schema reference](https://help.tableau.com/current/api/metadata_api/en-us/reference/).
Metadata asset and lineage tests use this fixture to validate generated GraphQL queries.
The fixture validates the documented current contract; it does not establish deployment compatibility with every Tableau version.

`tableau-schema-provenance.json` records the source URLs, retrieval date, source hashes, output hash, and mechanical transformations.
The generator removes description comments and presentation whitespace, changes legacy interface separators to standard GraphQL syntax, and omits built-in scalars supplied by the parser.
It preserves Tableau fields, arguments, types, enums, and directives.
No hand-authored replacement schema or invented fallback fields appear in the fixture.

To update the fixture from the official reference, run:

```powershell
python internal\tableau\metadataassets\testdata\generate-schema.py
```

The optional `--source-cache` argument reuses official HTML captures containing `index.html` and the named reference pages.
Review upstream schema differences and run both consuming packages after regeneration.
Standard tests read the checked-in fixture and need no network or Python installation.
