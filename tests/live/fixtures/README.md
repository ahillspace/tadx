# Native fixture pack

The generator builds native Tableau files from public, deterministic data.
It does not download remote content or use Tableau credentials.
Generated files belong in ignored run storage, outside this directory.

## Generate the core pack

Use a Python environment with the Tableau Hyper API installed.

```powershell
python -m venv .tadx-live\python
.tadx-live\python\Scripts\python.exe -m pip install tableauhyperapi
.tadx-live\python\Scripts\python.exe tests\live\fixtures\generate.py .tadx-live\native
```

The Go setup helper runs `python` from the process search path.
Select the same Python environment when generating fixtures and verifying downloaded Hyper data.
The setup helper reports a blocked prerequisite if the Hyper API is unavailable.
It never installs packages automatically.

Each variant contains a Hyper extract, a packaged datasource, a packaged workbook, and an authored JSON manifest.
The workbook contains the **Sales by region** worksheet.
The datasource contains **ID**, **Order Date**, **Region**, and **Sales** fields.

| Variant | ID | Order Date | Region | Sales |
| --- | --- | --- | --- | --- |
| A | 1 | 2026-01-01 | East | 10 |
| A | 2 | 2026-01-02 | West | 25 |
| B | 3 | 2026-01-03 | North | 47 |

The variants have different row identities, categories, dates, totals, and row counts.
The append oracle expects the exact three-row multiset from A and B.
The replace oracle expects the exact B multiset.

## Bind native artifacts

`GenerateFixtures` returns profile artifact paths and bindings for these keys:

- `datasource-native` and `datasource-native-b`.
- `workbook-native` and `workbook-native-b`.
- `datasource-hyper` and `datasource-hyper-b`.
- `datasource-native-hyper` and `datasource-native-hyper-b`, as canonical content-seeding aliases.
- `datasource-append-result`, which contains the expected union manifest.

Keep target-specific artifact paths and private payloads in ignored profiles and run storage.
The source tree contains only the public generator.

## Verify native semantics

The independent verifier checks ZIP structure, member paths, XML syntax, authored field markers, worksheet markers, and extract counts.
When the binding includes `expected.rows`, it opens the downloaded Hyper extract with the Hyper API.
It compares exact row multisets, including duplicate multiplicity.
Row order does not affect the result.

`params.hyper_table` selects the extract table and defaults to `"Extract"."Extract"`.
Native artifact checks require an authored semantic manifest.
An empty manifest cannot produce a pass.

For flow artifacts, provide `expected.document` for an exact JSON comparison or `expected.flow` for a recursive object subset.
Choose stable authored nodes, relationships, and output configuration for a subset.
Do not use a generic nonempty-file check as the flow oracle.

## Supply advanced packs

Dependency graphs, composed sources, large multipart uploads, and executable Prep flows require explicit artifact bindings.
The core generator does not claim to prove these cases.
Missing advanced bindings produce blocked prerequisites.
Advanced packs need authored manifests that distinguish the intended payload from another valid native file.
