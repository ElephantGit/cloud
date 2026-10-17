# Ora Cloud — Documentation Index

Entry point for anyone (human or agent) picking up this codebase. Two reading paths:

| I am… | Start here | Then |
| --- | --- | --- |
| **An AI agent** continuing development | [`development/agent/architecture.md`](development/agent/architecture.md) | [`development/agent/adding-features.md`](development/agent/adding-features.md) when adding an endpoint |
| **A new teammate** getting oriented | [`development/onboarding/overview.md`](development/onboarding/overview.md) | the [Cloud ADRs](https://github.com/ora-space/specs/tree/main/decisions/cloud) for why the system is shaped this way |

## Doc map

```
docs/
  INDEX.md                      ← you are here (map + pointers)
  plugins.md                    ← 插件市场:选择状态权威、catalog 快照、同步、effect 链路

  development/
    agent/                      ← for AI agents: dense, precise, convention-focused
      architecture.md           ← request path, core conventions, invariants, traps
      api-reference.md          ← every endpoint (public + internal), fields, error codes
      database.md               ← all tables & migrations, add/alter rules
      adding-features.md        ← how-to: add an endpoint / sub-resource safely
    onboarding/                 ← for new teammates: readable, tables & diagrams
      overview.md               ← what Ora Cloud is, the moving parts, glossary
  acceptance.md                 ← product acceptance criteria
  authentication.md             ← auth model
  core-contract.md              ← core behavioural contract
  execution-contract.md         ← execution/operations contract
```

Tenancy today: every tenant has exactly one space; tenant membership decides access and the
`admin/member` role; projects in a space are shared with its active members and only admins delete
them. See [core contract](core-contract.md) and the [migration directory](../internal/core/migrations/README.md).

## Source of truth (one doc per topic)

| Topic | Where | Notes |
| --- | --- | --- |
| **Architecture decisions** | [specs `decisions/cloud/`](https://github.com/ora-space/specs/tree/main/decisions/cloud) | ADRs and their core test cases; the code follows them |
| **Issue collaboration** | [Issue collaboration ADR](https://github.com/ora-space/specs/blob/main/decisions/cloud/issue-collaboration/0-typed-targets-and-explicit-execution-intent.md) | `@` target modes, Comment ≠ Interaction ≠ IssueRun, ports (D1), Timeline, Workflow forms and the single Confirm boundary |
| **Issue architecture in code** | [agent/architecture.md](development/agent/architecture.md) | how the collaboration ports and fixtures are wired |
| **API** | [agent/api-reference.md](development/agent/api-reference.md) | live endpoint/field reference; `api/openapi.json` is the machine truth |
| **Database** | [agent/database.md](development/agent/database.md) + [migration README](../internal/core/migrations/README.md) | Current table inventory and immutable migration history |
| **How to add code** | [agent/adding-features.md](development/agent/adding-features.md) | endpoint / sub-resource how-to + verify |

## Repo layout (one glance)

| Path | What lives there |
| --- | --- |
| `cmd/` | Entry points: `server`, `cloudctl`, `openapi`, `simulator`, demos. |
| `internal/api/router/` | HTTP layer — route allowlist + dual-JWT auth + strict JSON. |
| `internal/core/` | Business logic — `Store.Public`/`Control`, raw SQL in advisory-locked tx, migrations. |
| `internal/contract/` | OpenAPI generator (`openapi.go`) → `api/openapi.json`. |
| `internal/simulator/` | Test/dev substrate (in-memory node + credential signing). |
| `integration/` | End-to-end tests against real PostgreSQL (isolated schema per test). |
| `api/openapi.json` | **Generated** — do not hand-edit; run `go run ./cmd/openapi`. |
| `scripts/` | Dev/demo shell wrappers. |

Work-in-progress notes (plans, progress, implementation logs) do not belong in this repository: they
live in the issue that tracks the work and are never committed, so `docs/` only holds documents that
are maintained together with the code.

## Golden rules (both audiences)

1. **Additive first** — new files/routes/migrations; never refactor core to add a feature.
2. **Never edit an applied migration** — add a new `NNNN_*.sql`; alter tables there.
3. **`api/openapi.json` is generated** — after changing `internal/contract/openapi.go`, run
   `go run ./cmd/openapi` or the contract test fails.
4. **Verify before claiming done** — `go build ./...`, `go test ./internal/... ./cmd/...`, and the
   integration suite against Docker PostgreSQL (see [adding-features.md](development/agent/adding-features.md) §Verify).
