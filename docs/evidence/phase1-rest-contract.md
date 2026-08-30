# Phase 1 REST contract evidence

This record freezes the upstream contract used by the Phase 1 remote capabilities.
The captured official Tableau REST API help is `Tableau API Documentation/tableau_rest_api.md`.
Its SHA-256 digest is `89bf3a33175ddcfee15e45b13facfa1833b4a06218580c66ef48e04691f1842b`.

The implementation uses these captured sections:

- PAT sign-in request, response, and errors at lines 40901 through 41166.
- Classic pagination at lines 1699 through 1751.
- Workbook listing at lines 38650 through 38925.
- Project listing at lines 36768 through 37060.
- Workbook download at lines 20889 through 20955.
- Upload initiation at lines 28079 through 28146.
- Upload append at lines 10432 through 10506.
- Workbook publish at lines 34005 through 34425.
- Asynchronous publish jobs at lines 3475 through 3504.
- Job queries at lines 36548 through 36643.

Contract tests use local HTTP servers and parse the multipart publish and append bodies.
They assert methods, paths, headers, payload identities, exact uploaded bytes, ordered multi-block sequence IDs, pagination, response parsing, errors, terminal jobs, and in-flight polling timeouts.
No live Tableau deployment verification is claimed.

The Query Job endpoint is documented as administrator-only while workbook publish can be available to non-administrator publishers.
Synchronous publish is the default, and asynchronous polling requires the explicit `--as-job` option.
When Tableau accepts an asynchronous publish but job polling is forbidden, cancelled, or times out, the mutation outcome is unknown rather than failed.
The error retains the job and request IDs and does not advise an automatic retry.
The capture disagrees on a 1,000-block versus 10,000-block upload limit.
TADX applies the conservative 1,000-block limit.
