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
| `task_conversation_*.go`, `thread_preview.go` | Ordered task messages, serial same-task Agent handoff and pause barriers, follow-up prompts frozen from Node visible text, configuration-gap hints and a bounded in-process live preview; initial Agent prompts read only the frozen run input | [Thread](../../docs/agent-runs/threads.en.md), [IssueRun](../../docs/agent-runs/issue-runs.en.md) |
| `revision*.go`, `session_*.go` | Revision registration, archives, session listing and statistics | [Revision](../../docs/agent-runs/revisions.en.md), [Session reads](../../docs/agent-runs/session-reads.en.md) |
| `model_*.go` | Personal configuration, frozen bindings, grant digests and request observations; no API Key decryption | [Model access](../../docs/models/model-access.en.md) |
| `plugins.go`, `plugin_*.go`, `workflow_asset_imports.go` | Catalog snapshots with Agent declaration copies, space selection, Node install plans with frozen declaration associations and asset import ledger | [Plugin marketplace](../../docs/plugins/marketplace.en.md) |
| `workflow*.go` | Graphs, published snapshots, admission, run identity and scheduler | [Workflows](../../docs/workflows/workflows.en.md) |
| `hub.go`, `signals.go` | Post-commit invalidation and control hints; no authoritative state | [HTTP API](../../docs/core/http-api.en.md) |

## Store and transaction boundaries

- `NewStore` obtains `*sql.DB` from injected GORM and wires the real Workflow Directory/Forms; it neither connects nor migrates nor creates a global database. `NewDevelopmentStore` additionally permits the retired clone fixture.
- `transact` takes `pg_advisory_xact_lock(67420911)` inside PostgreSQL, handles domain Faults and post-commit notifications. Hooks must use the supplied transaction, never start a nested Store transaction.
- Network, Git, filesystem and object-store calls happen outside transactions; callers recheck identity, version and authorization afterwards. Archive reads, publish-source checks and credential confirmation have dedicated entrypoints.
- `Object` is the boundary JSON value. `Fault` and `ErrorCode` convert expected failures and database errors into safe public errors. Domain panics are converted only at the transaction control boundary; unexpected panics must not be swallowed.
- Commands wire Store ports and policy durations. See [collab](../collab/README.en.md) for development ports; simulated results do not replace physical execution evidence.

See [migrations](migrations/README.en.md) for complete migration filenames. Package `*_db_test.go` tests need isolated real PostgreSQL schemas; [integration](../../integration/README.en.md) covers HTTP, Git and storage boundaries.
