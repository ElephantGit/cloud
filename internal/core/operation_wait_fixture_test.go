package core

import (
	"database/sql"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// operationWaitDB applies SQL directly to an isolated schema. This deliberately avoids the
// migration receipt/checksum harness; it tests schema behavior without file digest validation.
func operationWaitDB(t *testing.T) *Store {
	return operationWaitDBThrough(t, "")
}

func operationWaitDBThrough(t *testing.T, through string) *Store {
	t.Helper()
	config := coreTestDatabaseConfig(t)
	admin := stdlib.OpenDB(*config)
	schema := "operation_wait_" + newID()[:8]
	if _, err := admin.Exec("CREATE SCHEMA " + pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	config.RuntimeParams["search_path"] = schema
	pool := stdlib.OpenDB(*config)
	t.Cleanup(func() {
		_ = pool.Close()
		if _, err := admin.Exec("DROP SCHEMA " + pgx.Identifier{schema}.Sanitize() + " CASCADE"); err != nil {
			t.Error(err)
		}
		_ = admin.Close()
	})
	applyOperationWaitMigrations(t, pool, through)
	store := &Store{Pool: pool, Signals: NewControlHub()}
	BindBusinessHooks(store)
	return store
}

func applyOperationWaitMigrations(t *testing.T, pool *sql.DB, through string) {
	t.Helper()
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if through != "" && entry.Name() > through {
			break
		}
		body, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(string(body)); err != nil {
			t.Fatalf("apply %s: %v", entry.Name(), err)
		}
	}
}
