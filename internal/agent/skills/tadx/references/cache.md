# Cache freshness and coverage

Live content search uses Tableau's site-native search and ranking; cache search is local lexical matching.
Use live search for concepts or approximate names and the cache for repeated known-term discovery when freshness is acceptable.
A cache miss never proves remote absence, and local reads never silently fall back to Tableau.

Ordinary bounded live lists/searches do not access SQLite or collect a complete inventory.
Complete inventory collection can save cache data best-effort: unfiltered collection replaces that resource scope; filtered collection records observations without claiming full coverage.
Schema reads also retain observations, but targeted reads do not establish complete site inventory.
A failed cache write does not invalidate the live answer.

Refresh replaces requested inventory scopes atomically while retaining unrelated schemas, Pulse observations, and their original timestamps.
Refreshing inventory does not make retained schema evidence freshly verified.
Permissions are not in the default refresh because their collection adds per-resource requests.
Explicit permission collection can retain accessible records with partial coverage; an inaccessible permission record is not an empty rule set.
Use reported freshness, coverage, and warnings before treating a scope as exhaustive.

Cache identity is bound to the actual server and site, not an editable alias.
Legacy or incompatible cache data requires explicit refresh; ordinary reads do not guess its origin or rebuild it.

Collection starts with up to four concurrent reads and ramps toward the environment ceiling, default 32.
Throttled reads share a cooldown within the run.
Lower the configured cache concurrency for sensitive servers; the environment reference documents the setting and bounds.
