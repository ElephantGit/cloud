# cmd/server: Cloud service composition

[中文](README.md) | [English](README.en.md)

The production entrypoint composes configuration, databases, HTTP/gRPC adapters and lifecycle loops around one Store. Core owns business state and policy.

## Startup and serving

- Load YAML / `CLOUD_*` configuration, logging and the PostgreSQL pool. `CheckSchema` rejects missing, unknown or checksum-mismatched migrations without running DDL.
- Read deployment trust material and construct the Authenticator, object store and external-service clients.
- `router.New` serves public HTTP; `controlgrpc.NewSecure` serves mutually authenticated Controller gRPC. Retired management JSON paths stay retired.
- `configureCollaboration` composes real Space Agent and Workflow directories in production; collab fixtures require the explicit development switch.
- `wireRepository` connects Git ref queries, Credential Store and the Repo Executor worker according to configuration; network I/O stays outside Core transactions.

## Background tasks and shutdown

`main.go` owns loops with one cancellation context and WaitGroup: optional catalog sync and workflow asset imports, Agent dispatch/startup, blocked windows, Thread end/cancellation, delivery/release recovery and Workflow scheduling. Core methods still decide state; loops do not make process-local state authoritative.

On SIGINT/SIGTERM or a serving failure, cancel work, drain HTTP/gRPC, join loops, then close the database and flush logs. HTTP shutdown is bounded to 10 seconds; startup failure also reclaims already-started tasks.

See [Deployment](../../docs/operations/deployment.en.md) and [Configuration](../../docs/operations/configuration.en.md) for operations, and [IssueRun](../../docs/agent-runs/issue-runs.en.md), [Workflows](../../docs/workflows/workflows.en.md) and [External Git line](../../docs/repositories/external-git.en.md) for execution semantics. Local tests verify composition boundaries.
