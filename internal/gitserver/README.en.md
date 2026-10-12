# gitserver: sandbox Git access

[中文](README.md) | [English](README.en.md)

Serves Project repositories (`/{tenantId}/{projectId}.git`) to sandboxes over smart HTTP only
(proposed ADR `cloud/git-server/0`). `internal/core` and PostgreSQL decide every permission; this
package holds no authorization state.

| Files | Responsibility |
|---|---|
| `config.go` | Deployment references, listener TLS: the repository listener requires a client certificate from the sandbox CA; the ref listener pins Cloud's exact leaf |
| `scope.go` | `RuntimeScope`: only the `git-access` SPIFFE purpose is accepted; WSS and model-access certificates from the same CA are not |
| `repository.go` | The four smart HTTP endpoints, canonical IDs, repository creation on first authorized write (staged then renamed), exact delivery-ref lookup |
| `service.go` | Per-request authorization with `Ora-Execution-Id`, `git http-backend` via CGI, the ref query and health |
| `hook.go` | One-time push tokens, the private Unix-socket hook channel and the `RunPreReceive` hook body |

Every repository request authenticates the runtime target from the certificate and asks Core whether
the declared execution may read (a session restoring a prior Revision) or write (a delivery). Any
denial is the same `403`, including an absent repository; a database failure is `503` and never
allows. A delivery may only create `refs/ora/deliveries/<workId>`: `pre-receive` repeats the full
Core decision immediately before any ref is written and rejects the whole push unless every update
creates exactly that ref with a commit. Replaying the same attempt is up to date and does not fail.

Repository policy and the hook path are applied to every git process through `GIT_CONFIG_*`
environment variables rather than stored in repositories, so a restart (new hook directory) or an
older repository cannot weaken them. Object transfer and hooks never hold a database transaction.

Tests use the real git client, `git http-backend` and TLS with a policy double for the boundary
matrix; Core's PostgreSQL tests cover the decision, and `integration/git_server_test.go` drives a
real delivery from Cloud state through the shipped command as the hook. Long fetches are not
re-authorized mid-transfer; writes are, in pre-receive.
