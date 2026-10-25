# Bind portable live fixtures

Target profiles hold identity bindings and authored truth for live tests.
Keep completed profiles in ignored local storage because they contain target-specific identifiers.
Keep PAT values in environment variables or approved native credential references.
A target change replaces the profile and runs setup again.
Test IDs, assertions, and scenario code stay the same.

The suite treats missing bindings as blocked prerequisites.
An empty result, missing expected manifest, denied API, and unsupported target never become passes.
`BindingRequirements()` and `ValidateAssignedBindings()` in [admin_requirements.go](suite/admin_requirements.go) declare dependencies for the 62 administration, catalog, labels, lineage, and Pulse scenarios.
Setup checks the selected cases before creating remote fixtures.
Fresh API reads still verify identities, access, and mutation baselines when each case runs.
`AssignedFixtureCapabilities()` declares the exact independent fixture writes and cleanup capabilities for these cases.
Native publish and delete capabilities derive from the declared native bindings.

## Understand binding fields

Every binding uses this shape:

```json
{
  "id": "authoritative-rest-luid",
  "name": "Exact fixture name",
  "path": "fixtures/native-package.tflx",
  "params": {
    "metadata_id": "authoritative-metadata-id"
  },
  "expected": {}
}
```

`id` identifies the exact REST resource.
`metadata_id` identifies the distinct Metadata API resource.
Use the actual identifier for each API rather than inventing a conversion.
`path` resolves relative to the profile file.
`params` contains strings, including serialized JSON when a query needs variables.
`expected` contains authored JSON values.
Map comparisons require every authored field, and array comparisons require the complete expected set with no duplicates or extra rows.
Object field order and array order do not affect these comparisons.

Create expected truth from fixture source files and independent API observations.
Do not copy candidate CLI output into expected values to make a failing test pass.

## Bind administration fixtures

The following bindings cover administration scenarios:

| Binding | Required fields | Use |
| --- | --- | --- |
| `user_a` | `id` | Read a known user, preserve a sentinel, and seed direct membership. |
| `user_b` | `id`, distinct from `user_a` | Add or remove a member, change catalog contact, and test follower isolation. |
| `controlled_user` | `name` containing `{prefix}`, `params.auth_setting` | Create an owned disposable user for user lifecycle tests. |
| `flow-native` | `path` and authored native `expected` facts | Independently publish a valid flow for flow permission CRUD. |

Example profile fragment:

```json
{
  "bindings": {
    "user_a": { "id": "existing-fixture-user-a" },
    "user_b": { "id": "existing-fixture-user-b" },
    "controlled_user": {
      "name": "{prefix}@owned-example.test",
      "params": {
        "auth_setting": "ServerDefault",
        "siteRole": "Unlicensed"
      }
    },
    "flow-native": { "path": "fixtures/approved-flow.tflx", "expected": { "flow": { "nodes": { "authored-node-id": { "name": "Live fixture output" } } } } }
  }
}
```

Replace the example authentication setting with one supported by the selected target.
The suite replaces `{prefix}` with its generated ownership namespace.
Configure a login domain that you control before selecting user creation tests.
Setup does not discover users to invite or invent user addresses.
User creation can cause target-controlled invitations or licensing effects, so this pack requires deliberate selection and a disposable account policy.
Role update starts from a supported role other than `Viewer` and verifies that `Viewer` persists.
The actor and verifier retain permission to delete the disposable account after the role change.
Existing `user_a` and `user_b` accounts remain read-only fixtures.

Groups, category definitions, and label value definitions use per-case names and exact cleanup journal entries.
Group membership tests change only their newly created group.
Permission tests publish an owned workbook, datasource, or flow, or create an owned project.
Workbook and datasource permission cases explicitly require their `workbook-native` or `datasource-native` binding, including authored expectations.
Attachment label cases explicitly require `datasource-native`.
Setup generates these core native bindings when absent.
The selected binding closure and artifact hashes include these fixture dependencies.
They add a sentinel permission rule through the independent API before testing the selected principal's rule.
Project fixtures must use `ManagedByOwner` content permissions so direct permission tests do not inherit locked parent permissions.

## Bind catalog assets

Catalog tests use existing indexed assets because native publication alone does not guarantee every required physical asset and metadata relationship.
Provision these fixtures in the selected target, then record their exact identities.
The setup profile does not depend on a specific Tableau Cloud pod or Tableau Server hostname.

| Binding | Required fields |
| --- | --- |
| `catalog_database` | `id`, `name`, `params.metadata_id`, and independent oracle. |
| `catalog_table` | Database fields plus `params.parent_id` for the database REST LUID. |
| `catalog_column` | Database fields plus `params.parent_id` for the table REST LUID. |
| `catalog_database_sibling` | Different database `id` for mutation isolation. |
| `catalog_table_sibling` | Different table `id` for mutation isolation. |
| `catalog_column_sibling` | Different column `id` and its table `params.parent_id`. |

Read tests compare CLI identities and descriptions with independent REST reads.
They also run a separately authored Metadata query against the known source truth.
Include the exact parent identity in `expected.read` to protect parent scoping.

Example database binding:

```json
{
  "id": "database-fixture-luid",
  "name": "Live fixture database",
  "params": {
    "metadata_id": "database-metadata-id",
    "oracle_query": "query FixtureDatabase($luid:String!){databasesConnection(filter:{luid:$luid},first:2){totalCount pageInfo{hasNextPage} nodes{id luid name description}}}",
    "oracle_variables": "{\"luid\":\"database-fixture-luid\"}"
  },
  "expected": {
    "oracle": {
      "databasesConnection": {
        "totalCount": 1,
        "pageInfo": { "hasNextPage": false },
        "nodes": [{
          "id": "database-metadata-id",
          "luid": "database-fixture-luid",
          "name": "Live fixture database",
          "description": "Authored baseline description"
        }]
      }
    },
    "read": {
      "name": "Live fixture database",
      "description": "Authored baseline description"
    }
  }
}
```

The connection and filter names follow the selected target's Metadata schema.
Keep independent queries bounded and read-only.
Record terminal coverage such as `hasNextPage: false` in authored oracle expectations when completeness matters.
A GraphQL error, missing row, changed description, or changed identity fails the oracle check.

Before selecting database or table update tests, assign an existing nonempty description and contact to the subject fixture.
The contact must differ from `user_b`.
Clearing database or table descriptions and contacts is outside the currently verified contract, so tests block before changing an unrestorable baseline.
Column description restoration supports an empty string.
Subject and sibling column bindings each record their exact table parent.

Update tests journal the previous description and contact before mutation.
They independently add two unique tags, then ask the CLI to add a third tag and remove one seeded tag.
REST reads verify the changed description and contact, the exact tag changes, preservation of unrelated tags, and the sibling's unchanged state.
Cleanup restores the original writable fields and removes only the generated tags.
It never deletes external catalog assets.

## Author audit truth

Use `audit_database`, `audit_table`, `audit_datasource`, and `audit_workbook` bindings.
Each binding needs the selected scope's REST `id`, an independent oracle query, `expected.oracle`, and `expected.output`.
The output expectation requires positive `scanned`, explicit `complete`, nonnegative `summary.present`, `summary.missing`, and `summary.unknown`, and a nonempty complete `findings` array.
Each finding records `metadata_id`, `type`, `name`, `check`, `state`, and `source`.
Include `luid`, `parent_table_luid`, and `datasource_metadata_id` where those identities exist.
Metadata-only fields do not need invented REST LUIDs.

Example field audit output expectation:

```json
{
  "scanned": 3,
  "complete": true,
  "summary": { "present": 2, "missing": 1, "unknown": 0 },
  "findings": [
    { "metadata_id": "field-direct", "type": "CalculatedField", "name": "Direct", "check": "descriptions", "state": "present", "source": "direct" },
    { "metadata_id": "field-inherited", "type": "ColumnField", "name": "Inherited", "check": "descriptions", "state": "present", "source": "inherited" },
    { "metadata_id": "field-missing", "type": "ColumnField", "name": "Missing", "check": "descriptions", "state": "missing", "source": "direct" }
  ]
}
```

Datasource and workbook audit fixtures require direct, inherited, and missing descriptions.
Workbook expectations also include exact `groups` entries when assessments include embedded and published datasource groups.
Database audit runs both supported checks and includes the exact seeded table and column findings.
Table, datasource, and workbook audit cases request description checks.
The suite reconciles summary counts against the returned findings independently of the authored totals.
An incomplete upstream observation stays incomplete or unknown according to the fixture truth.

## Author catalog search truth

Use `catalog_search` for database and table search.
Set `params.query`, an independent oracle, and a nonempty exact `expected.output.items` set.
Include expected `complete`, `scanned`, and `page` facts when those facts define the fixture contract.
Matching items record `metadata_id`, `luid`, `type`, `name`, and `parent` where applicable.
Provision a contrasting asset without the search token.

Use `catalog_column_search` for scoped column search.
Add `params.parent_id` with the selected table REST LUID.
Provision matching column names in two different tables.
The authored output includes only the column in the selected table, with its exact parent.
The independent oracle records both source fixtures so the expected exclusion has an observable basis.

## Author lineage truth

Use `lineage_workbook`, `lineage_datasource`, and `lineage_flow` bindings.
Each binding needs the root REST `id`, `params.metadata_id`, an independent oracle, and `expected.graph`.
The graph expectation needs at least two nodes and one directed edge.
Workbook and datasource fixtures include known field relationships.

```json
{
  "id": "workbook-fixture-luid",
  "params": {
    "metadata_id": "workbook-metadata-id",
    "oracle_query": "query FixtureWorkbook($luid:String!){workbooksConnection(filter:{luid:$luid},first:2){nodes{id luid name}}}",
    "oracle_variables": "{\"luid\":\"workbook-fixture-luid\"}"
  },
  "expected": {
    "oracle": {
      "workbooksConnection": {
        "nodes": [{ "id": "workbook-metadata-id", "luid": "workbook-fixture-luid", "name": "Live fixture workbook" }]
      }
    },
    "graph": {
      "nodes": [
        { "metadata_id": "workbook-metadata-id", "kind": "workbook", "rest_luid": "workbook-fixture-luid" },
        { "metadata_id": "embedded-datasource-id", "kind": "embedded_datasource" },
        { "metadata_id": "field-metadata-id", "kind": "field" }
      ],
      "edges": [
        { "from_metadata_id": "workbook-metadata-id", "to_metadata_id": "embedded-datasource-id", "relationship": "contains" },
        { "from_metadata_id": "embedded-datasource-id", "to_metadata_id": "field-metadata-id", "relationship": "contains" }
      ]
    }
  }
}
```

Replace the illustrative graph with the actual authored fixture graph.
Expected graph nodes and edges are required members, so unrelated server enrichment can coexist with the fixture contract.
Every actual graph edge must have existing endpoints.
The saved root must map the expected REST LUID to the expected Metadata identity.
The suite reads the saved workspace-relative graph file rather than trusting the CLI receipt alone.

## Bind Pulse eligibility and fields

Pulse scenarios require an eligible datasource and working released Pulse APIs on the selected target.
Use target readiness evidence to distinguish available, denied, and unsupported capabilities.
A target without Pulse support cannot pass Pulse tests through an empty inventory.
Do not assume a Server target supports every Cloud capability.

```json
{
  "bindings": {
    "pulse_datasource": {
      "id": "eligible-datasource-luid",
      "params": {
        "measure_field": "[Revenue]",
        "date_field": "[Order Date]",
        "dimension_field": "[Region]",
        "category": "West"
      }
    },
    "pulse_destination_datasource": { "id": "compatible-destination-datasource-luid" }
  }
}
```

Field values are exact provider field IDs, not guessed captions.
The datasource contains a numeric measure, a date field, and a dimension with the authored category value.
Both actor and verifier can read the datasource and the definitions created from it.
Configure existing eligible follower users through `user_a` and `user_b`.
The actor and verifier can use separate PATs, or the runner can use its guarded serial handoff with one PAT.
Subscription listing uses the independently authenticated actor identity even when the verifier uses another account.

Fixture creation supplies the admitted business sections: specification, extension options, representation options, insights options, comparisons, datasource goals, related links, certification, name, and description.
Pull compares every section present in the independent saved response, including provider defaults.
Publish compares every admitted saved business section rather than requiring optional empty sections that the provider omits.
You can override the native create document through `pulse_datasource.expected` when the fixture needs nondefault formatting or supported business options.
Treat that object as a create document rather than an output expectation wrapper.
The suite owns the generated definition name and preserves the configured datasource field IDs.

Portable bundle cases seed included and excluded filter variants in addition to the default metric.
They compare full saved definition configuration, exact metric inventories, periods, filters, and default markers with independent Pulse API observations.
Pull also seeds a source follower to prove that subscriptions stay outside portable bundles.
Publish creates new objects and compares every source-to-destination mapping with independent reads.
It verifies that source bytes, source remote configuration, and source followers remain unchanged.
Destination metrics have no copied source subscriptions.

`pulse_destination_datasource` is optional.
Without it, publish uses an explicit source-to-source datasource mapping and still creates new definitions and metrics.
A distinct destination datasource must expose compatible exact field IDs because publish does not remap field identities.
Both source and destination remain selected-target resources; cross-target credentials are not embedded in bundles.

## Restore and retain evidence

The candidate CLI performs the behavior under test.
The independent fixture client provisions prerequisites, reads postconditions, and cleans up exact owned identities.
Cleanup intents persist before remote writes, and confirmed identities persist before later verification.
An uncertain unnamed metric or subscription creation keeps its owned parent and journal for inspection.
Cleanup does not guess that an unobserved mutation failed.
Accepted jobs must reach an observed terminal outcome before related resources can be removed.

Fixture accounts, external catalog assets, and reusable eligible datasources stay outside deletion ownership.
Generated groups, projects, published content, label definitions, attachments, definitions, metric variants, and subscriptions have per-case cleanup entries.
Use the runner's explicit cleanup command to resume an interrupted journal against the same guarded target.

The maintained API contracts are visible in [independent REST fixtures](harness/api_resources.go), [fixture creation](harness/fixtures.go), [catalog scenarios](suite/catalog.go), and [Pulse scenarios](suite/pulse.go).
The CLI's corresponding provider constraints are defined in [catalog metadata writes](../../internal/tableau/metadataassets/client.go), [catalog query contracts](../../internal/tableau/metadataassets/graphql.go), and [Pulse saved configuration contracts](../../internal/tableau/pulse/client.go).
