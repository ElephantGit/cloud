// Package storetest opens an isolated v2-server store for tests.
package storetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// Open gives the test its own PostgreSQL schema on TEST_DATABASE_URL, dropped afterwards. The
// test is skipped when TEST_DATABASE_URL is unset.
func Open(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, e := pgx.Connect(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { admin.Close(ctx) })
	var b [6]byte
	_, _ = rand.Read(b[:])
	schema := "v2test_" + hex.EncodeToString(b[:])
	if _, e := admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	config, e := pgxpool.ParseConfig(url)
	if e != nil {
		t.Fatal(e)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	s, e := store.OpenConfig(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	return s
}

// Login signs in a development user and returns their ID and the demo tenant's ID.
func Login(t *testing.T, s *store.Store, subject string) (user, tenant string) {
	t.Helper()
	ctx := context.Background()
	token, e := s.Login(ctx, store.Identity{Source: "dev", Subject: subject, DisplayName: subject}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	u, e := s.SessionUser(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	tenants, e := s.Tenants(ctx, u.ID)
	if e != nil || len(tenants) == 0 {
		t.Fatalf("tenants: %v %v", tenants, e)
	}
	return u.ID, tenants[0].ID
}
