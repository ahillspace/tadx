# TADX architecture

TADX is one CLI binary, not a resident service.
The diagram shows command handling, operation rules, remote integrations, local state, and rendering.

![The main TADX components and their source references](tadx-architecture.svg)

Open [the editable Excalidraw diagram](tadx-architecture.excalidraw) in Excalidraw to zoom, edit, or follow the numbered source links.
The side legend points to concrete files rather than listing every command inside the boxes.
Solid arrows summarize runtime flow; dashed arrows show registry and dependency wiring.
Action packages group related operations where their ownership and behavior match.
Their dependencies use focused interfaces, direct calls, or shared services according to each package's contract.

The [package reference](../repository-structure.md) describes the current source layout.
The [search source map](search.md) traces native search, additional REST/Pulse inventories, cache search, and catalog metadata search.
The [capability map](../reference/capability-map.html) answers a different question: which operations TADX provides.
The [preserved HTML overview](index.html) also includes a workbook-pull walkthrough.
Open either HTML file in a browser; GitHub displays its source rather than running the page.
