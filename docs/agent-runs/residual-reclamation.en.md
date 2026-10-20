# Agent Run residual reclamation: Cloud authority and evidence

English | [中文](residual-reclamation.md)

D4/D5 [bounded startup settlement](startup-settlement.en.md) can finish the business Run while retaining unresolved resources in
`agent_run_residuals`. Cloud continues collecting evidence on that original ledger and only grants system cleanup authority when the
evidence qualifies. Deletion follows the existing owned-delete path. It neither settles the [IssueRun](issue-runs.en.md) again nor reopens its Workspace.

This page describes the Cloud step 1 implementation boundary. It is not evidence of physical termination by the Controller or Sandbox Server,
deployment acceptance, or joint fault acceptance. The
[residual reclamation ADR](https://github.com/ora-space/specs/blob/main/decisions/cloud/issue-run/20261018-residual-agent-run-reclamation.md)
remains `approved`; this step does not declare the whole ADR implemented.

## Ledger states and one durable responsibility

| `reclaim_state` | Meaning | Authority fields |
|---|---|---|
| `reconciling` | Collect original-identity facts; deletion is not yet authorized | ID, mode and operation are NULL; authorization is `{}` |
| `reclaiming` | One durable responsibility has been granted; continue or await evidence | Fixed `reclaim_id`, mode and nonempty authorization; `owned_delete` binds exactly one operation |
| `cleared` | Disposition under the original responsibility is complete | Keep that responsibility, with both `cleared_at` and disposition |

Step 1 production authorization uses only `owned_delete` and completes only as `released`. `controlled` reserves the shared internal arbitration
point; it is neither a controlled management endpoint nor operator authority. The schema's `terminated_data_retained` and `externally_confirmed`
dispositions are reserved for later controlled paths.

Automatic responsibility stays bound to the same `delete_workspace` operation. Restarts, lease changes, retries and quiesce refusals do not
create another responsibility or operation, and do not revive a failed historical operation. Ordinary deletion, dispatch and permit paths retain
residual guards; an internal system responsibility is not a public deletion flag. The transaction rechecks the ledger, Run, Workspace, Project and
operation tenant, original bindings and generation; a UUID foreign key alone does not establish that scope.

The [migration](../../internal/core/migrations/README.en.md) only adds columns. Defaults initialize existing D4/D5 rows as `reconciling`, without
inferring completion, backfilling D8, or changing old `reason`, `responsibility` or business settlement. A legacy row with non-null `cleared_at`
causes an explicit error and rollback pending compatibility handling (A2).

## Original-identity evidence and deletion eligibility

`reclaim_targets` is a union of original plans and fact targets: original operations/effects, preallocated sandbox ensure IDs, sandboxes, Nodes,
executions, clones, work and data. Late facts may add original targets; they cannot replace an old target with the current generation or drop an
unknown target because a query did not find it.

New facts append to `reclaim_evidence` with logical event keys, facts, sources and database receipt times, instead of changing old
`responsibility.lateFacts`. An identical replay reuses its receipt; new evidence, additional targets or authority transitions increment
`reclaim_version`. Evidence intake, authorization, permits and completion use Cloud's existing serialized transaction boundary. Completion must
therefore reread the full target set and latest facts in its own transaction.

System authorization requires located original identities, closed admission, no conflict, no blocking Project operation or other activity, and
qualifying delivery and execution evidence. Execution evidence includes input closure and an explicit unfinished-execution list for the same
runtime control identity, together with original execution terminals. Per-target termination evidence can also establish physical closure.
A no-target proof applies only when original plans and execution relationships truly do not exist; missing rows, NULL results and elapsed time
do not establish that proof.

Step 1 accepts delivery eligibility only through a genuinely registered and verified Revision for the original `deliver_revision` execution;
every other actual delivery execution needs its own completed verification. There is currently no unified existing durable source for explicit
delivery abandonment. `failed`, `skipped`, business timeouts, absent object storage and missing results do not imply explicit abandonment (A1).
Entries with insufficient evidence remain open.

Authorized cleanup follows existing quiesce → terminate → cleanup. A quiesce refusal leaves the original operation in `retry_wait`, retaining
the original authority and guards for evidence collection and retry (A3). It neither restores new execution eligibility, escalates to force-stop,
nor creates another operation.
When the existing Controller requests `blocked` for unconfirmed termination, this system responsibility instead persists claimable `retry_wait`;
actual plans and permits still recheck evidence.

A failed `sandbox_terminate` / `workspace_data_delete` effect bound to this responsibility is a retryable observation, not a disposal conclusion.
After fresh evidence, the same effect may advance to success while retaining both observations. Historical provisioning/execution failures,
external identity bindings and successful conclusions remain protected against contradictions.
After completion, the existing effect-result intake only preserves original bound facts or audits conflicts; it cannot advance the operation,
change its version or grant execution authority again.

Every original sandbox/ensure ID requires a corresponding successful termination result containing both `terminated` and `lateEnsureFenced`.
`terminated_at`, a failed ensure, a 404 or elapsed time alone is insufficient. The current safe target can reuse owned delete; an additional
historical ID without both confirmations remains open, without replacing its identity or dispatching another termination effect (A5).
Data-deletion planning, permits and completion recheck target coverage. One transaction confirms successful data deletion, succeeds the operation,
soft-deletes the Workspace and writes `cleared/released`, without changing the Run's phase/status/result or repeating business settlement activity.

## Existing fact intake and conflicts

Cloud uses existing `runtime_ack`, creation-fact, execution-result and Revision intake. This step adds no RPC, proto or Controller discovery
mechanism. After checking the original control epoch/version, Node, sandbox and generation, `runtime_ack` records input closure and the full
`unfinishedExecutionIds` list on the ledger. Session terminals and delivery results are accepted under their original execution/work/resource
identity. A done Run or cleared ledger is not a reason to reject a consistent late fact. Output rescue uses existing Revision verification;
it does not open admission or start a new session or execution.

Consistent replay preserves the original conclusion. A contradictory terminal first appends a conflict audit and sets `reclaim_conflicted_at`;
the audit and block commit before the stable `409 residual_fact_conflict` is returned (A4). Canonical facts, existing `cleared_at` and disposition
remain intact. Further deletion or reuse is blocked; this step provides no conflict-resolution endpoint.

## Reconciliation, recovery and remaining scope

`ReconcileAgentRunResidualsOnce` scans `reconciling/reclaiming` ledger rows independently of whether the business Run is still `releasing`.
Bounded passes rotate by `reclaim_checked_at NULLS FIRST, created_at, run_id`; a checked time is scheduling metadata, not completion evidence.
Each entry rereads targets, eligibility and original bindings. An existing responsibility resumes its original operation, subject to the current
Controller lease and effect permits. Query failures, insufficient evidence and temporary refusals neither clear the ledger nor expand authority.

If a failed create's original effects have no existing active-discovery path, they continue waiting for verifiable original-identity facts (A6).
Cloud's evidence intake and responsibility scheduling do not establish new Controller discovery capability, or production validation of real
containers, late-ensure fencing or physical data deletion.

| Later scope | Step 1 boundary |
|---|---|
| Step 2: controlled operator endpoints, retained data, external confirmation | Not implemented; internal `controlled` arbitration does not supply authentication, intent or commands |
| Step 3: Controller / Sandbox Server physical contracts and per-target evidence | This page states Cloud evidence requirements, not completed physical acceptance |
| Step 4: cross-repository joint fault and recovery acceptance | Incomplete; single-repository implementation or database tests are not a substitute |
| Taking over D8 historical deletion abandonment and residuals without ledger rows | Not implemented; no ledger backfill, automatic retry or historical business-result rewrite |

See the [core code map](../../internal/core/README.en.md) for implementation entry points. Test results require their execution artifacts;
this page does not claim migration, concurrency or end-to-end tests have passed.
