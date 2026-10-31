//go:build linux

package main

import (
	"context"
	"net"
	"net/http"
	"time"
)

// HTTP observations follow the process lifetime. Shutdown alone waits for active SSE
// handlers without cancelling them; the base context lets those handlers close their
// streams and join their readers before the execution service and database are closed.
// A browser disconnect only cancels that request, never the process or execution workers.
func newHTTPServer(ctx context.Context, addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
}
