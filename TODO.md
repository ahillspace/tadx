# Open work

This backlog contains unresolved investigations and deferred proposals.
An investigation does not establish a product defect or authorize a live experiment.
The [remediation verification record](docs/evidence/guidance-remediation-verification.md) preserves completed work and its limits.

## Product investigations

| Item | Remaining question | Existing evidence |
| --- | --- | --- |
| PULSE-05 | Establish provider support for numeric-only MIN/MAX restrictions. | Adjustable dimensions and existing running-total safeguards are implemented; this additional restriction still needs provider evidence. |
| READ-03 | Reconcile historical skipped workbook and flow inventory totals against raw upstream responses. | The slash-project cause is fixed; remaining historical dropped-record totals are unresolved. |
| READ-07 | Explain reported duplicate native-search identities using raw upstream evidence. | Existing code rejects duplicates within and across pages; the reports remain unconfirmed. |
| Workspace removal | Evaluate removal of the sole default workspace and distinguish disposable-state cleanup from downloaded-artifact deletion. | A cleanup result of zero removed entries does not establish an empty workspace. |
| Concurrent authentication | Determine whether recovered authentication failures reflect agent concurrency, expired credentials, or a CLI defect. | Earlier observations include agents ignoring sequential-PAT Guidance; current command coordination requires a separately assessed rerun. |

## Verification gaps

- Native Zsh and Fish installer execution remains unverified in the recorded remediation run.
- Live permission-denial hydration and nonempty Pulse dimension-filter forks remain unexercised in that run.
- Pulse creation, a timeframe fork, and follower readbacks succeeded, but the complete workflow exceeded its time budget.
  The [workflow performance evidence](docs/evidence/agent-workflow-performance.md) and remediation timings inform any separately scoped follow-up.

## Evaluation questions

- Distinguish agent-requested cleanup from evaluator-only cleanup obligations while retaining original failed outcomes after later cleanup succeeds.
- Assess whether recommendations distinguish schema evidence from actual date coverage or values and describe catalog coverage using the requested scopes.
- Resolve the temporary-user role prerequisite where an access or following exercise conflicts with harness restrictions on licensed roles.
  A role assignment alone does not establish a purchase or billing event.

## Deferred proposals

- A local Tableau MCP installer and connection management remain proposals outside current TADX ownership.
- Connected-app direct trust remains separate from current PAT-only authentication.
- Datasource composition authoring remains indefinitely deferred; ordinary and existing composed package pull and publish remain supported.
- Long-operation progress, resumability, and automatic background execution remain separate proposals.
