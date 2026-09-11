# Inventory and cache contracts

Read only for list, search, cache, schema caching, or related discovery changes.
These specialize the build skill's output, identity, and resource-bound contracts.

## Live reads and pagination

Live reads are the default; explicit `--cache` is local-only and never falls back to Tableau.
Ordinary limited lists fetch bounded provider results without collecting a whole inventory or requiring SQLite.
Explicit `--all` and scoped cache refresh share the collector; list renders the normalized live result rather than reading it back from SQLite.
Share typed filter builders between limited and full reads so filters retain identical meaning.
Keep provider pagination loops typed and private, not recursive action calls or public cursor tokens.
Reuse one invocation-scoped project hierarchy rather than fetching projects once per item.
Complete, filtered, capped, partial, and failed collections must remain distinguishable.
Store project identity as an indexed LUID, not by extracting it from JSON payload text.

## Cache publication and freshness

Bind cache identity to normalized server endpoint and site, not just an environment alias.
Unbound legacy data requires explicit refresh; do not guess its source or rebuild it on an ordinary read.
An unfiltered, complete full collection may replace that resource scope.
Filtered collections record observations without claiming full inventory coverage.
Best-effort cache failure must not discard a successful live answer; retain a warning.
Explicit refresh failure preserves the previous generation and returns an error.
Collect into bounded disk staging, then publish in a short atomic transaction; do not hold the active SQLite write transaction over network calls.
Replace only requested inventory kinds, preserving independent schemas, Pulse observations, and other scopes with their original timestamps.
Refreshing inventory does not reverify preserved observations.
Persist complete sets, including empty sets, where omission means removal, such as metric follower snapshots.
Failed reads must not replace prior verified observations.

## Traffic and completeness

Default refresh excludes permissions; require an explicit scope for per-resource permission reads.
An item-level permission denial may yield a partial usable inventory with warnings, never false complete coverage.
Use the shared adaptive collector and cancelable Retry-After cooldown, not independent worker retry loops.
The default per-process concurrency ceiling is 32 and is configurable by environment; it is not a server-wide traffic limit.
Test paging/truncation, filter parity, target isolation, empty results, preserved observations, failed publication, and throttling as relevant to the change.
