# TADX agent guide

TADX is a deterministic Tableau lifecycle and development CLI for coding agents and humans.
Its purpose is higher accuracy, lower tokens, and lower latency when operating Tableau.

Read this first, then go to the executable capability registry.

## Ownership in one line

Tableau MCP is the data analyst: it owns querying actual data (VDS/query-datasource), view data and images, and Pulse metric values and insights.
TADX is the development and lifecycle harness: content and artifacts, workspaces, administration, and Pulse definition and configuration lifecycle.
TADX never calls or proxies MCP inside a command. CLI and MCP are peer surfaces; the agent chooses.

## How to work here

- Start with the capability registry: run capability list to discover, capability get to inspect one capability's ownership, selectors, safety, evidence, and blockers.
- Every executable command is one isolated action package. To understand a command, read its package.
- Cobra is thin plumbing. It parses arguments, invokes an action, renders the output, and maps the exit code. It contains no Tableau behavior.
- Resource adapters isolate Tableau API complexity. Actions call adapters through narrow interfaces the action owns; actions never call raw HTTP.
- Tests define externally visible behavior before implementation.
- Blocked capabilities are registry metadata only. Do not guess a blocked or docs-only capability into existence; a capability needs its upstream contract captured before any live API code is written.

## Non-negotiables

- PAT authentication only.
- Default output is TOON.
- Consequential mutations preview by default and require --apply. --force never means --apply.
- Mutation discovery gating is not authorization.
- Tableau LUIDs are authoritative; ambiguous selectors fail; no fuzzy or interactive resolution.
- Secrets TADX handles are never persisted or printed.

## Read deeper

- scope-v1: what V1 builds and does not build.
- axi: the CLI behavioral contract.
- toon: the output format contract.
- contributing/adding-a-capability: the end-to-end build guide for one capability, including the contract-row-to-manifest-to-CLI wiring chain and the dependency and import rules the architecture test enforces.
- contributing/adding-an-adapter: the end-to-end build guide for one Tableau resource seam, including the client family, resource adapter, pagination envelope, redaction, and contract test.
- build-order: the order slices are built and what must be frozen first.
- The arc42 and the V1 capability contract: the authoritative product and capability sources.

## Owned repos for reference

- projects/tableau/tableau-tooling = tabget - core go CLI action for indexing an entire Tableau CLoud or Server instances all the way down to permissions. tabporter - app to get items from Tableau, unpackage them, convert them to csv, modify them with agents, publish them (incomplete).
- projects/tableau/juju = Legacy monolithic application for creating Tableau Pulse and Tableau Next Metrics. 
- projects/tableau/juju-local = Two more recent applications that mirror Juju capability, but are much smaller and easier to understand (incomplete, but metrics creation and forking works for Pulse)

Assets in these repositories can be used as long as they align with all tadx guidelines. All code should be reviewed and optimized prior to implementation. 

## Archived material

Files under `archived/` are obsolete historical inputs.
Do not read them for current implementation guidance.
