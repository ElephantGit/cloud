# internal/core: authoritative domain and persistence

[中文](README.md) | [English](README.en.md)

This package owns business state, authorization, transactions and recovery decisions. HTTP and gRPC adapters translate requests into Store calls; see the [document map](../../docs/README.en.md) for cross-module contracts.

## Code map

| File group | Package responsibility | Cross-module documentation |
|---|---|---|
| `store.go`, `public.go`, `control.go` | Injected SQL pool, transaction wrapper, dispatch, idempotency and Fault | [Persistence and transactions](../../docs/core/persistence.en.md) |
| `auth.go`, `tenant.go`, `join.go`, `space*.go` | Verified identity resolution, tenant and membership scope | [Identity and tenancy](../../docs/identity/tenancy.en.md) |
| `node*.go`, `runtime_*.go`, `run_workspace.go` | Node facts, control sessions, fencing and run Workspaces | [Runtime control](../../docs/runtime/runtime-control.en.md) |
| `repository_*.go`, `git_*.go` | Bindings, sync/publish Operations, GitGrant and credential metadata | [External Git line](../../docs/repositories/external-git.en.md) |
| `issue*.go`, `collaboration*.go`, `form_descriptor.go` | Issue aggregates, timeline, interactions and consuming collaboration ports | [Issue collaboration](../../docs/collaboration/interactions.en.md) |
| `agent_run_*.go`, `execution*.go`, `thread*.go`, `business_hooks.go` | Run lifecycle, execution registry, Thread and transaction-scoped settlement hooks | [IssueRun](../../docs/agent-runs/issue-runs.en.md), [Controller](../../docs/runtime/controller-integration.en.md) |
| `agent_run_residual.go`, `agent_run_residual_reclaim.go` | Original identity/target coverage and evidence eligibility, one reclamation responsibility and one owned-delete operation, effect/permit and completion checks | [Residual reclamation](../../docs/agent-runs/residual-reclamation.en.md) |
| `agent_run_residual_fact.go`, `agent_run_residual_loop.go` | Appended facts without rewriting old responsibility, replay and durable conflict blocking; ledger scans independent of Run phase and original-operation recovery | [Residual reclamation](../../docs/agent-runs/residual-reclamation.en.md) |
| `agent_run_controlled_reclaim*.go` | Deployment-operator intents and command receipts, one root responsibility and dependent data deletion, per-target termination and external confirmation, and D8 guards without ledger backfill | [Residual reclamation](../../docs/agent-runs/residual-reclamation.en.md) |
| `task_conversation_*.go`, `thread_preview.go` | Ordered task messages, serial same-task Agent handoff and pause barriers, follow-up prompts frozen from Node visible text, configuration-gap hints and a bounded in-process live preview; initial Agent prompts read only the frozen run input | [Thread](../../docs/agent-runs/threads.en.md), [IssueRun](../../docs/agent-runs/issue-runs.en.md) |
| `revision*.go`, `session_*.go` | Revision registration, archives, session listing and statistics | [Revision](../../docs/agent-runs/revisions.en.md), [Session reads](../../docs/agent-runs/session-reads.en.md) |
| `model_*.go` | Personal configuration, frozen bindings, grant digests and request observations; no API Key decryption | [Model access](../../docs/models/model-access.en.md) |
| `plugins.go`, `plugin_*.go`, `workflow_asset_imports.go` | Catalog snapshots with Agent declaration copies, space selection, Node install plans with frozen declaration associations and asset import ledger | [Plugin marketplace](../../docs/plugins/marketplace.en.md) |
| `workflow*.go` | Graphs, published snapshots, admission, run identity and scheduler; WorkflowRun creation shared by Issue confirm/manual and direct starts, the `workflow_invocation_ref` link, and the scheduler's projection of WorkflowRun status onto the linked IssueRun | [Workflows](../../docs/workflows/workflows.en.md), [Issue collaboration](../../docs/collaboration/interactions.en.md) |
| `hub.go`, `signals.go` | Post-commit invalidation and control hints; no authoritative state | [HTTP API](../../docs/core/http-api.en.md) |

## Store and transaction boundaries

- `NewStore` obtains `*sql.DB` from injected GORM and wires the real Workflow Directory/Forms; it neither connects nor migrates nor creates a global database.
- `transact` takes `pg_advisory_xact_lock(67420911)` inside PostgreSQL, handles domain Faults and post-commit notifications. Hooks must use the supplied transaction, never start a nested Store transaction.
- Network, Git, filesystem and object-store calls happen outside transactions; callers recheck identity, version and authorization afterwards. Archive reads, publish-source checks and credential confirmation have dedicated entrypoints.
- `Object` is the boundary JSON value. `Fault` and `ErrorCode` convert expected failures and database errors into safe public errors. Domain panics are converted only at the transaction control boundary; unexpected panics must not be swallowed.
- Commands wire Store ports and policy durations. See [collab](../collab/README.en.md) for development ports; simulated results do not replace physical execution evidence.

See [migrations](migrations/README.en.md) for complete migration filenames. Package `*_db_test.go` tests need isolated real PostgreSQL schemas; [integration](../../integration/README.en.md) covers HTTP, Git and storage boundaries.

Residual facts retain their original intake: `agent_run_provisioning_fact.go` handles creation facts, `node_execution.go` and `revision.go` handle
original execution terminals and delivery, and `runtime_control_delivery.go` accepts full runtime evidence through existing `runtime_ack`.
`run_workspace.go`, `control.go` and effect/permit paths enforce the original responsibility binding. No new RPC/proto or public system-delete
flag is introduced. Step 2 uses `cloudctl residual` with deployment database credentials and authenticated `session_user`; an operator label is
self-reported. No HTTP route or tenant privilege is added. One root terminate and at most one dependent delete_data intent each follow three
states. Root completion never regresses; deletion binds one operation and preserves the first `cleared_at`. External confirmation requires an
existing unfinished intent and per-target fact references, without fabricating physical success.
Controlled targets use existing `ListForceStops` / `ConfirmForceStop` / `GetEffectPermit` fields, independently of ordinary force-stop history
and business completion. D8 takeover is explicit and uses original identity without ledger backfill. See the dedicated page for automatic
A1–A6 eligibility, two-stage disposition and recovery guards. This describes the Cloud implementation contract, not passing tests;
Controller/Sandbox physical proof and cross-repository fault acceptance remain steps 3 and 4 respectively.
