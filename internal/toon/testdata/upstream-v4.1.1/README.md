# TOON v4.1.1 conformance fixtures

The `encode/` and `decode/` directories are exact copies of the official language-neutral TOON fixtures.
The fixture schema, upstream test documentation, and upstream license are also preserved here.

Source: `https://github.com/toon-format/spec`
Tag: `v4.1.1`
Commit: `62f16b369408180f1faf1cba7da1b46d1f336f12`
Specification version: `4.1`
License: `MIT`

The default Go test suite runs all 179 encoding cases, all 335 strict decoding cases, and all 24 explicit non-strict decoding cases.
Set `TOON_FIXTURES` to another upstream `tests/fixtures` directory to perform an optional external audit.
