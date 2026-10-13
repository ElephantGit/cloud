# Agent startup settlement

[中文](agent-startup-settlement.md) | [English](agent-startup-settlement.en.md)

The first prompt and queued work prove Cloud declared intent. Dispatch registration means Node may already have executed, even without an acknowledgement or Thread record. Startup reconciliation uses this boundary to decide whether withdrawal is safe.

This implementation supports review of the specs proposal `20261012-bounded-agent-startup-and-undispatched-withdrawal`. The new terminal boundary and default budget require owner approval and synchronized core cases before merging for deployment. No migration or rewriting of historical Node results is involved.

## Before registration

Under the same transaction lock as dispatch, withdrawal verifies there is no Agent execution, bound work or delivered command. Retiring unregistered work and commands, ending the Thread, discarding queued turns, settling the business outcome and declaring owned deletion commit together. Any failure rolls everything back; late registration is rejected.

| Condition | Business outcome | Safe reason |
|---|---|---|
| User end or cancellation | `cancelled`, delivery `skipped` | No failure |
| Frozen input requires model proxy, but the current initialized, connected Node with a heartbeat within 30 seconds explicitly lacks it | `failed`, delivery `skipped` | `model_proxy_unavailable` |
| Startup budget expired | `failed`, delivery `skipped` | `agent_start_failed` |

The end endpoint still returns `202/ending` with its existing request replay behavior. Periodic reconciliation settles the intent. A busy Project operation slot leaves the run `releasing` for the existing deletion redeclaration loop.

Capability evidence shares dispatch's database-clock freshness boundary. A Node whose connected flag outlives its heartbeat is temporarily unknown: it cannot trigger capability refusal or retire the frozen intent of an existing pending Thread. Disconnected and uninitialized Nodes cannot supply definitive capability absence either.

## Registered execution without a startup record

Expiry leaves the Thread `ending`, exposes `failureCode=agent_start_failed` on GET Thread and closes further input. Work, registration, queued turns, commands and the Workspace remain. An existing user end reason is retained; otherwise Cloud declares one policy cancellation. Repeated scans do not add termination commands.

Late records retain their original takeover order, and a first record cannot reopen an ending Thread. A real terminal Node event still enters delivery; cleanup is declared only when delivery settles. A policy timeout cancellation ends as `failed`; a prior user end retains its business outcome. Node failures retain bounded safe classifications. Cloud fabricates no Node result, receipt, history or Revision.

## Budget and validation

`issue_runs.agent_start_timeout` defaults to `30m`; `CLOUD_ISSUE_RUNS_AGENT_START_TIMEOUT` accepts a positive deployment override. The database clock compares the stable stage origin: successful Workspace creation before declaration, work creation after declaration, and execution creation after registration. User input, request replay and connection recovery do not renew it.

Database regressions cover withdrawal, explicit capability refusal, expiry, dispatch races, late registration, repeats, rollback and cleanup after Project contention. HTTP/gRPC tests cover request replay, closed input, safe reads, late records, terminal takeover and delivery. Real provider conversations and administrative force-stop require their own acceptance evidence; a timeout never proves that registered execution stopped.
