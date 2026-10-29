# v2-server

Cloud v2's **temporary** single-process backend (see the v2 section of the root `AGENTS.md`). It is
deleted once `frontend-v2` runs against Cloud, so favor the simplest code that keeps the demo honest
over Cloud's durability machinery. It does not follow the specs process.

- **Contract first.** Every `/api` route implements a path of `api/openapi.json` with the same
  request and response shapes, so `frontend-v2` can switch to Cloud unchanged. Tests validate each
  such response against that document; a new route needs the same check. Do not edit
  `api/openapi.json` for v2. A route the contract lacks lives here only, is listed under "契约之外" in
  `README.md` as a candidate for Cloud, and is exempt from the check.
- **Login mirrors the Gateway's browser routes** (`/auth/providers`, `/auth/login`, `/auth/logout`,
  the `ora_session` cookie) with only a development provider.
- **Simplified on purpose.** One process, one PostgreSQL database, schema applied idempotently at
  startup from `internal/store/schema.sql` (no migrations), no tenant isolation beyond membership
  checks. Write down every simplification in `README.md` so nobody mistakes it for Cloud behavior.
- **Layout.** `main.go` wires configuration and lifecycle; `internal/store` owns SQL;
  `internal/server` owns HTTP. Keep handlers thin and context-aware.
- **Gate.** `gofmt`, `go vet` and `go test -race ./...` with `TEST_DATABASE_URL` set; the v2
  workflow runs the same. Tests skip without a database, so run them against one before pushing.
