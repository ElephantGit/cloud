# Adding Features — for agents

How-to for the two most common changes. Read [architecture.md](architecture.md) first. The whole
point: stay **additive**, respect the invariants, and satisfy the OpenAPI contract test.

## A. Add an endpoint to an existing resource

Touch exactly these (in order):

1. **Route** — `internal/api/router/router.go` `Routes()`: add
   `{Method, "/api/v1/tenants/:tid/…", "", []string{...body fields…}}`. `Action==""` ⇒ public.
   New path params (`:xid`) get bound into `PublicRequest` in the same handler (search
   `c.Param(`).
2. **PublicRequest field** — `internal/core/public.go`: add the param field to the struct if new.
3. **Dispatch** — `public.go` `Public()` (write) and/or `readPublic()` (read): add a `case`. For a
   route nested under `/issues/:iid/…`, dispatch **inside** the issue case by
   `strings.Contains(r.Path, "/xxx")`, and in `readPublic` put the nested GET **before**
   `case r.IssueID != ""` (trap #1).
4. **Body type** — if a field isn't a string/integer, extend `validField` in `router.go` (closed
   type system, trap #3).
5. **Core logic** — new or existing `internal/core/*.go`: raw SQL via `t.exec`/`t.one`/`t.list`
   inside the dispatch; enforce tenant scope, version, idempotency as applicable.
6. **OpenAPI** — `internal/contract/openapi.go`: add/adjust the schema + `responseSchema` +
   `inputSchema` + `optionalField` (+ `isList` for list endpoints). Insert resource branches
   **before** the `Contains(path,"/issues")` fallthrough (trap #2).
7. **Regenerate** — `go run ./cmd/openapi` → rewrites `api/openapi.json` (required, trap #4).
   If the frontend client should consume the new endpoint too, run `task frontend:generate` and commit
   `api/openapi.json` together with `frontend/src/api` (the drift is checked by the CI `frontend` job
   and `TestPublishedOpenAPIIsValidAndCurrent`).
8. **Test** — `integration/…_test.go`: drive it through the router via `f.call`/`f.path` (see
   `issues_test.go` / `issue_extensions_test.go`). The validating transport checks every response.

## B. Add a new sub-resource (table + endpoints)

1. **Migration** — new `internal/core/migrations/NNNN_*.sql` (see [database.md](database.md)).
   Never edit an applied migration.
2. **Core file** — new `internal/core/<thing>.go` with `thing`, `thingList`, `createThing`,
   `updateThing`, `deleteThing` following the `issue_*.go` shape (structural FK to
   `tenant_memberships` for user refs, `validText`, `version` precondition, soft delete).
3. Then steps **A.1–A.8**.

## Invariant checklist (don't break these)

- [ ] All SQL inside `transact` (one advisory-locked tx). No new `*sql.DB` handle.
- [ ] Every query filters `tenant_id`. Business reads are tenant-wide unless owner-scoped by design.
- [ ] Mutating read-then-write calls `version(o, body.N("version"))` and bumps `version=version+1`.
- [ ] POST/DELETE require `Idempotency-Key` (router enforces); PUT uses version instead.
- [ ] Soft-delete business rows (`deleted_at=now()`); only join tables hard-delete.
- [ ] Set `updated_at=now()` explicitly in each UPDATE.
- [ ] New body fields are in the route `Fields` allowlist AND pass `validField`.
- [ ] Every response field exists in the OpenAPI schema (`additionalProperties:false`).
- [ ] `api/openapi.json` regenerated after any `openapi.go` change.

## Verify (run all before claiming done)

```bash
cd cloud
go build ./...
GOPROXY=https://goproxy.cn,direct go test ./internal/... ./cmd/...
go run ./cmd/openapi            # if openapi.go changed

# integration (Docker Postgres; App Control blocks go test temp exe → precompiled binary):
docker start cloud-postgres-1
GOPROXY=https://goproxy.cn,direct GOCACHE="$PWD/.local/gocache" GOTMPDIR="$PWD/.local/gotmp" \
  go test -c -o .local/gotmp/integration.test.exe ./integration
cd integration && TEST_DATABASE_URL='host=127.0.0.1 port=55432 user=ora password=ora-local dbname=ora sslmode=disable' \
  REQUIRE_POSTGRES=1 ../.local/gotmp/integration.test.exe
```

## Reference examples

| You want to… | Look at |
| --- | --- |
| Add a tenant-level CRUD resource | `issue_statuses.go` + `labels` routes |
| Add an issue-nested sub-resource | `issue_comments.go` (nested dispatch) |
| Add a many-to-many with embedding | `issue_labels.go` (`attachLabels`) |
| Add a query param | `?q=` in `issues.go` (`likePattern`) + `Query` in `PublicRequest` |
| Add a batch/bulk op | `batchUpdate` in `issues.go` |
