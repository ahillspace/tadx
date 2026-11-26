# Roadmap

This roadmap tracks planned TADX enhancements.
Use [GitHub Issues](https://github.com/ahillspace/tadx/issues) to report bugs.

## Planned

### Publish workbooks to Personal Space

Support Personal Space as a workbook publication destination on Tableau sites that support it.
Respect site permissions and verify the destination after publication.

### TDS Edit API support

Tableau documents work-copy checkout, reads, element edits, and publication in REST API 3.30.
TADX validates typed changes files locally and exercises the remote workflow against simulated Tableau responses.
Remote commands await live verification on a supported Tableau site.
Save as new requires verified project placement and create-only behavior.
Save overwrite requires a documented way to preserve the original datasource LUID and a revision check that covers its full definition.
See the [TDS Edit evidence record](docs/evidence/tds-edit-contract.md#evidence-needed-for-activation) for the remaining qualification steps.

### Tableau Knowledge Graph support

Support Tableau Knowledge Graph capabilities outside Tableau MCP's current capabilities.
