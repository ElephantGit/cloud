//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/projectrepos"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestPrepareDemoKeepsOneRealProjectAcrossRestarts(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := pgx.Identifier{"demo_" + hex.EncodeToString(nonce[:])}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	db, err := store.OpenConfig(ctx, cfg.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	root := filepath.Join(t.TempDir(), "repositories")
	repositories, err := projectrepos.New(db, projectrepos.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	scope, first, err := prepareDemo(ctx, db, repositories)
	if err != nil {
		t.Fatal(err)
	}
	if first.RepositoryURL != "fixture://demo" || first.OwnerUserID != scope.UserID {
		t.Fatalf("unexpected demo: %+v", first)
	}
	commit, err := exec.CommandContext(ctx, "git", "--git-dir="+first.MirrorPath, "rev-parse", first.SnapshotRef).Output()
	if err != nil || strings.TrimSpace(string(commit)) != first.Commit {
		t.Fatalf("project lacks its real snapshot: %q %v", commit, err)
	}

	inspect, err := pgx.ConnectConfig(ctx, cfg.ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close(context.Background())
	// The development tenant must be selected by identity, not its position in a sorted list.
	var other string
	if err := inspect.QueryRow(ctx, "INSERT INTO tenants(name) VALUES ('000 other tenant') RETURNING id").Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := inspect.Exec(ctx, "INSERT INTO memberships(tenant_id,user_id,role) VALUES ($1,$2,'owner')", other, scope.UserID); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = store.OpenConfig(ctx, cfg.Copy())
	if err != nil {
		t.Fatal(err)
	}
	repositories, err = projectrepos.New(db, projectrepos.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	secondScope, second, err := prepareDemo(ctx, db, repositories)
	if err != nil || secondScope != scope || second.ID != first.ID || second.Commit != first.Commit {
		t.Fatalf("restart changed demo: %+v %+v %v", secondScope, second, err)
	}
	items, err := repositories.List(ctx, scope)
	if err != nil || len(items) != 1 {
		t.Fatalf("duplicate projects: %+v %v", items, err)
	}
	var sessions int
	if err := inspect.QueryRow(ctx, "SELECT count(*) FROM sessions").Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("bootstrap retained a login token: %d %v", sessions, err)
	}
	if _, err := inspect.Exec(ctx, "UPDATE tenants SET status='suspended' WHERE id=$1", scope.TenantID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareDemo(ctx, db, repositories); err == nil {
		t.Fatal("startup silently bypassed suspended demo tenant")
	}
}
