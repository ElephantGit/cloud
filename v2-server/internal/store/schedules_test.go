package store_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"os"
	"testing"
	"time"
)

func TestScheduleReopenAndConcurrentEdit(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, e := pgx.Connect(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close(ctx)
	var b [8]byte
	_, _ = rand.Read(b[:])
	schema := "schedule_" + hex.EncodeToString(b[:])
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	config, e := pgxpool.ParseConfig(url)
	if e != nil {
		t.Fatal(e)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	s, e := store.OpenConfig(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	token, e := s.Login(ctx, store.Identity{Source: "test", Subject: "owner", DisplayName: "Owner"}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	u, e := s.SessionUser(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	ts, e := s.Tenants(ctx, u.ID)
	if e != nil {
		t.Fatal(e)
	}
	in := store.ScheduleInput{Name: "original", Target: json.RawMessage(`{"kind":"chat","prompt":"hello","agent":"","model":""}`), Rule: json.RawMessage(`{"kind":"daily","time":"09:00","days":[],"hours":6,"timezone":"Asia/Shanghai"}`)}
	created, e := s.CreateScheduleConfig(ctx, ts[0].ID, u.ID, in)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = store.OpenConfig(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	items, e := s.ScheduleConfigs(ctx, ts[0].ID, u.ID)
	if e != nil || len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("reopen %v %v", items, e)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, name := range []string{"first", "second"} {
		go func(name string) {
			<-start
			change := in
			change.Name = name
			change.Version = 1
			_, err := s.UpdateScheduleConfig(ctx, ts[0].ID, u.ID, created.ID, change, nil)
			results <- err
		}(name)
	}
	close(start)
	wins, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			wins++
		} else if errors.Is(err, store.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins %d conflicts %d", wins, conflicts)
	}
	items, e = s.ScheduleConfigs(ctx, ts[0].ID, u.ID)
	if e != nil || items[0].Version != 2 {
		t.Fatalf("version %v %v", items, e)
	}
}
