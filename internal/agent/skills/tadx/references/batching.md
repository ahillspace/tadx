# Batch outcomes and dependent work

Batching repeats one supported action, not an entire workflow.
Use it for already-decided targets; resource help supplies repeated selectors and per-item file syntax.
One repeated dimension is not a Cartesian product or positional pairing of several lists.
Different user/group pairs or permission settings need explicit item rows.

Items run sequentially and share authentication for the same target and credential.
Publication batches submit accepted items before waiting, then use pooled monitoring with ordered per-item outcomes.
Independent failures do not stop later items; cancellation can leave unfinished items.
The batch is not atomic, and a nonzero exit can contain confirmed successes.
Preserve those results and retry only unfinished items with known safe outcomes.
An unknown write outcome needs inspection, not automatic replay.

File items cannot change commands or consume earlier results.
When later steps depend on newly created identities, a short script can parse structured JSON results and pass the confirmed IDs onward.
Check each exit code, preserve partial results, and pause when a new decision or authorization is needed.
Do not scrape TOON or add a conversion tool for this purpose; TADX already emits JSON.
Batching does not infer dependency order, rebind published datasource references, or grant mutation permission.
