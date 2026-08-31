# TOON output contract

TOON (Token-Oriented Object Notation) is the default TADX CLI output format.
It is a compact, human-readable, lossless encoding of the JSON data model that minimizes tokens for LLM consumption.
TADX conforms to the upstream specification; it does not create a private dialect.

## Upstream and sourcing

- Specification: the toon-format/spec repository. Pin the exact spec version per TADX release. At time of writing the current spec is v4.1 (Working Draft), MIT licensed.
- Reference implementation: toon-format/toon (TypeScript/JavaScript), MIT licensed.
- Go sourcing decision: prefer a maintained Go implementation that conforms to the pinned spec version. If none can be verified, implement a conforming Go codec (encoder plus decoder) inside the TADX repository. Do not substitute YAML or a pseudo-TOON format.
- TOON is a foundation deliverable. It must be built, tested, and frozen before any resource action fans out, because every action renders through it.

## Grammar essentials (from spec v4.1)

This section is a builder-facing summary. The pinned spec is authoritative; where this summary and the pinned spec differ, the spec wins.

Structure and indentation:
- Nested objects use indentation, not braces. Encoders use a consistent number of spaces per level (default 2). Tabs must not be used for indentation.
- Key-value lines are `key: value` with exactly one space after the colon. No trailing spaces; no trailing newline at document end.

Keys:
- A key may be unquoted only if it matches `^[A-Za-z_][A-Za-z0-9_.]*$`. Otherwise it must be quoted and escaped, including in headers.
- A dotted key like `user.name` is one literal key; the dot has no structural meaning.

Scalars:
- Numbers are canonical decimals: no exponent for the normal magnitude range, no leading zeros except a single `0`, no trailing fractional zeros, `1.0` renders `1`, `-0` renders `0`.
- Booleans are lowercase `true` and `false`. Null is lowercase `null`.

String quoting triggers (a string must be quoted when any is true):
- it is empty;
- it has leading or trailing whitespace;
- it equals `true`, `false`, or `null`;
- it is numeric-like (matches `^[+-]?[0-9]+(?:\.[0-9]+)?(?:e[+-]?[0-9]+)?$` case-insensitively);
- it contains a colon, double quote, or backslash;
- it contains brackets or braces;
- it contains a control character (U+0000 to U+001F);
- it contains the active delimiter;
- it equals or starts with `-`;
- it equals or starts with `#`.
Otherwise it may be unquoted; Unicode, emoji, and internal spaces are safe.

Escapes inside double quotes:
- Emit `\\`, `\"`, `\n`, `\r`, `\t`, and `\uXXXX` for other control characters. Decoders reject unknown escapes and unterminated strings.

Arrays and tables:
- Length marker `[N]`: N is a non-negative integer with no leading zeros. In strict mode a decoded count must equal N.
- Inline primitive array: `tags[3]: admin,ops,dev`.
- Tabular array `[N]{fields}`: declare fields once, then one row per object at depth +1, cells joined by the active delimiter.
- Keyed tabular `[N:]{fields}`: a colon after the length marks a keyed header; each entry row is `entrykey: cells`. At the root the entry key is omitted.
- Nested field groups: a uniform-object column collapses into a group such as `customer{name,country}`; rows stay flat.
- List form is the fallback for mixed or non-uniform elements: one `-` item per element; a bare `-` is an empty object.

Empty values:
- Empty array on a field: `key: []`. Empty root array: `[]` on its own line. A bare `key:` decodes as an empty or nested object, not an empty array.

Delimiters:
- Comma is default (symbol omitted). Tab and pipe are options that trade readability for fewer tokens; the symbol appears in the header, e.g. `tags[3|]: reading|gaming|coding`.
- The active delimiter is declared by the nearest header; absence means comma with no inheritance. Quoting is delimiter-aware.

Comments:
- A full-line comment begins, after leading spaces only, with `#`. Decoders strip comment lines in a pre-pass in all modes; a `#` elsewhere is ordinary content. Encoders must not emit comment lines.

## TADX rendering rules

- Compact TOON is the default. --full returns expanded, untruncated TOON where applicable. --raw returns the underlying raw payload only where the registry marks the capability raw-capable.
- Pagination continuation metadata, warnings, partial outcomes (where allowed), and structured errors are all rendered in TOON.
- Default output is bounded and deterministic. --full does not license dumping an unbounded inventory.
- Secret redaction runs before rendering and always takes precedence, including over --raw.

Compact output is an explicit field projection, not merely full output with long strings truncated.
Full output is a bounded superset for the same operation and never widens pagination or changes behavior.
When compact output hides fields, it includes `details: "--full"` immediately before `help[]`.
The marker is omitted from full output and from results whose compact and full shapes are identical.
Field classification follows `docs/contributing/output-guidelines.md`.

## JSON interoperability

- TOON is a lossless representation of the JSON data model; round-trips are deterministic and lossless.
- JSON interop uses the standard TOON codec. TADX does not add a separate format or conversion command domain.

## Testing and conformance (required before freeze)

- Pin the exact upstream spec version and record it in the repository.
- Run the upstream conformance test suite against the chosen encoder/decoder.
- Golden rendering fixtures for compact and --full output.
- Round-trip codec tests (encode then decode returns the original data model).
- Fuzz tests targeting the grammar, the quoting triggers, and the escape rules.
