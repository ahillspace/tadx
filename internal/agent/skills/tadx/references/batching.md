# Batch outcomes and dependent work

Batching repeats one supported action, not an entire workflow.
Use it for already-decided targets; resource help supplies repeated selectors and per-item file syntax.
One repeated dimension is not a Cartesian product or positional pairing of several lists.
Different user/group pairs or permission settings need explicit item rows.

Items run in parallel and share authentication for the same target and credential; results are listed in selection order.
Each publication item reports Tableau's outcome when its synchronous publish request completes.
Independent failures do not stop other items; cancellation can leave unfinished items.
The batch is not atomic, and a nonzero exit can contain confirmed successes.
Preserve those results and retry only unfinished items with known safe outcomes.
An unknown write outcome needs inspection, not automatic replay.

File items cannot change commands or consume earlier results.
When later steps depend on newly created identities, a short script can parse structured JSON results and pass the confirmed IDs onward.
Check each exit code, preserve partial results, and pause when a new decision or authorization is needed.
Do not scrape TOON or add a conversion tool for this purpose; TADX already emits JSON.
For workbook publish, a `--batch-file` row can provide its own `datasource-binding` array.
Repeated `--artifact` selectors share binding flags across items.
Use per-item rows when workbooks need different mappings.
Batching does not infer dependency order or datasource mappings, or grant mutation permission.
