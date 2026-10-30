// Command v2-server is Cloud v2's temporary single-process backend; see README.md.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// defaultDatabase is the database v2-server uses on the repository's compose PostgreSQL.
const defaultDatabase = "postgres://ora:ora-local@127.0.0.1:55432/ora_v2?sslmode=disable"

func main() {
	addr := flag.String("addr", envOr("V2_ADDR", "127.0.0.1:8090"), "listen address")
	dbURL := flag.String("db", envOr("V2_DATABASE_URL", defaultDatabase), "PostgreSQL URL")
	flag.Parse()

	if e := run(*addr, *dbURL); e != nil {
		slog.Error("v2-server", "error", e)
		os.Exit(1)
	}
}

func run(addr, dbURL string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, e := store.Open(ctx, dbURL)
	if e != nil {
		return e
	}
	defer db.Close()

	// The explicit unavailable backend lets page adapters share one injection point
	// without reporting successful execution before the real runtime is installed.
	executions := &executiontest.Stub{}
	srv := &http.Server{
		Addr: addr,
		Handler: server.New(db, server.Options{
			SessionTTL: 7 * 24 * time.Hour,
			DemoUser:   store.Identity{Source: "dev", Subject: "demo", DisplayName: "演示用户"},
			Execution:  executions,
			Catalog:    executions,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()
	slog.Info("v2-server listening", "addr", addr)

	select {
	case e := <-errs:
		return e
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e := srv.Shutdown(shutdown); e != nil {
		return e
	}
	if e := <-errs; !errors.Is(e, http.ErrServerClosed) {
		return e
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
