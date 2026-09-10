# TOON output contract

TOON is TADX's default structured output format.
The in-repository [codec](../internal/toon/toon.go) implements specification version `4.1` and pins upstream fixture revision `v4.1.1`.
The [fixture provenance](../internal/toon/testdata/upstream-v4.1.1/README.md) records the upstream commit and MIT license.

## Codec behavior

The codec encodes and decodes the JSON data model without introducing a TADX-specific dialect.
Decoding uses strict mode by default and returns numbers as `json.Number`.
Decimal values outside Go's finite `float64` range remain decimal `json.Number` values.
Encoding normalizes decimal spelling while preserving mathematical value.
Objects use indentation, and uniform arrays can use tabular headers with declared lengths and fields.
Quoting preserves strings that would otherwise be ambiguous with keys, delimiters, or scalar values.

The maintained [conformance tests](../internal/toon/conformance_test.go) exercise the pinned upstream encoding, strict decoding, and non-strict decoding fixtures.
[Codec tests](../internal/toon/toon_test.go) and [fuzz targets](../internal/toon/fuzz_test.go) cover local regressions and round trips.

## TADX rendering

Compact output is an explicit field projection for the operation.
`--full` adds bounded details for that same operation without increasing record limits or changing its behavior.
Results with hidden compact fields expose a `details: "--full"` hint; identical compact and full shapes omit that hint.
Warnings, partial outcomes, and structured errors survive rendering.
Paginated discovery output reports `more_available` without opaque continuation cursors.
Supported `--limit` and `--all` options control record acquisition separately from presentation.

Raw payload output exists only where the capability supports it.
Secret redaction applies before output, including raw output.
The [output tests](../internal/output/output_test.go) cover compact and full projections, golden output, and redaction.
TOON supports JSON interoperability through the codec; TADX does not expose a separate output-format or conversion command domain.

The repository [build skill](../.agents/skills/tadx-build/SKILL.md) contains field-selection procedures and required verification for output changes.
