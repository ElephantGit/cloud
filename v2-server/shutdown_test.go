//go:build linux

package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// These tests use the command's server constructor and a real listener. The event handler
// has the same request-lifetime contract as chatEvents: flush the SSE headers, wait for a
// request cancellation, then finish. Shutdown alone does not cancel that active request.
func startShutdownHTTP(t *testing.T, lifetime context.Context, handler http.Handler) (*http.Server, string, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := newHTTPServer(lifetime, listener.Addr().String(), handler)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = listener.Close()
	})
	return srv, "http://" + listener.Addr().String(), served
}

func shutdownHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	transport := &http.Transport{DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport}
}

func awaitShutdownResult[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP lifecycle worker did not finish")
		var zero T
		return zero
	}
}

func TestHTTPShutdownCancelsActiveEventStream(t *testing.T) {
	lifetime, stop := context.WithCancel(context.Background())
	defer stop()
	observerEnded := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		observerEnded <- r.Context().Err()
	})
	srv, base, served := startShutdownHTTP(t, lifetime, handler)
	request, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(request, http.MethodGet, base+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := shutdownHTTPClient(t).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("event stream = %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	// Receiving the flushed headers proves Shutdown will meet an active, idle SSE request.
	readerEnded := make(chan error, 1)
	go func() { _, err := io.Copy(io.Discard, response.Body); readerEnded <- err }()
	stop()
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdown); err != nil {
		t.Fatalf("Shutdown with an open event stream = %v", err)
	}
	if err := awaitShutdownResult(t, observerEnded); !errors.Is(err, context.Canceled) {
		t.Fatalf("event observer did not receive process cancellation: %v", err)
	}
	if err := awaitShutdownResult(t, readerEnded); err != nil {
		t.Fatalf("event stream was forcibly broken instead of ending cleanly: %v", err)
	}
	if err := awaitShutdownResult(t, served); !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Serve after Shutdown = %v", err)
	}
}

func TestHTTPClientDisconnectDoesNotCancelServerLifetime(t *testing.T) {
	lifetime, stop := context.WithCancel(context.Background())
	defer stop()
	observerEnded := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		observerEnded <- r.Context().Err()
	})
	srv, base, served := startShutdownHTTP(t, lifetime, handler)
	client := shutdownHTTPClient(t)
	observation, disconnect := context.WithCancel(context.Background())
	defer disconnect()
	req, err := http.NewRequestWithContext(observation, http.MethodGet, base+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	disconnect()
	if err := awaitShutdownResult(t, observerEnded); !errors.Is(err, context.Canceled) {
		t.Fatalf("disconnected observer = %v", err)
	}
	if err := lifetime.Err(); err != nil {
		t.Fatalf("browser disconnection canceled the process/worker owner: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	check, err := client.Do(req)
	if err != nil {
		t.Fatalf("server did not survive observer disconnection: %v", err)
	}
	check.Body.Close()
	if check.StatusCode != http.StatusOK {
		t.Fatalf("health after observer disconnection = %d", check.StatusCode)
	}
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := awaitShutdownResult(t, served); !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Serve after Shutdown = %v", err)
	}
}
