# Tableau document schemas

These files are the Tableau workbook (TWB) XML schemas, one per Tableau release, gzip-compressed and embedded into the binary.
The schemas from 2018.1 to 2026.2 are copied from the Tableau plugin's workbook authoring skill.
The 2026.3 schema comes from github.com/tableau/tableau-document-schemas at commit b01c07d0192c2d5ad67e8c48dd3b29b1be5e52f0.
Both sources are Apache-2.0 licensed; see THIRD_PARTY_NOTICES.md at the repository root.

The files are unmodified apart from compression.
To add a release, compress the upstream `twb_<release>.0.xsd` with `gzip -9n` into this directory and run `go test ./internal/twbschema`.
`twbschema.Versions` lists every file here, so no code change is needed.
To check a change against workbooks kept outside the repository, list their paths in a file and run `TWBSCHEMA_CORPUS=<list> go test ./internal/twbschema -run TestLocalCorpus -v`.
