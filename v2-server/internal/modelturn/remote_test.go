package modelturn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

func remote(t *testing.T, handler http.HandlerFunc) *nodeservices.RemoteProvider {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	provider, err := nodeservices.NewRemoteProvider(server.URL+"/v1", "backend-private-key", server.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestRemoteProviderJSONAndSSE(t *testing.T) {
	for _, format := range []string{"application/json", "text/event-stream"} {
		t.Run(format, func(t *testing.T) {
			var calls atomic.Int32
			provider := remote(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v1/chat/completions" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer backend-private-key" {
					t.Errorf("unexpected request path, method or authorization")
				}
				var request map[string]json.RawMessage
				if json.NewDecoder(r.Body).Decode(&request) != nil || string(request["model"]) != `"configured-model"` || string(request["stream"]) != "true" || len(request["tools"]) != 0 || len(request["tool_choice"]) != 0 {
					t.Errorf("unexpected model-only request")
				}
				w.Header().Set("Content-Type", format)
				if format == "application/json" {
					_, _ = io.WriteString(w, strings.TrimSuffix(jsonAnswer("真实 TLS 🙂", "stop"), "}")+`,"usage":{"prompt_tokens":4,"completion_tokens":6,"total_tokens":10}}`)
					return
				}
				_, _ = io.WriteString(w, chunk("真实 TLS ", nil))
				w.(http.Flusher).Flush()
				_, _ = io.WriteString(w, chunk("🙂", "stop"))
				_, _ = io.WriteString(w, `data: {"choices":[],"usage":{"prompt_tokens":4,"completion_tokens":6,"total_tokens":10}}`+"\n\n"+done)
			})
			var emitted strings.Builder
			result, err := Run(t.Context(), binding(provider), prompt, func(_ context.Context, delta string) error {
				emitted.WriteString(delta)
				return nil
			})
			if err != nil || result.Status != execution.StatusCompleted || !result.AnswerComplete || result.Answer != "真实 TLS 🙂" || emitted.String() != result.Answer || calls.Load() != 1 {
				t.Fatalf("result=%+v err=%v emitted=%q calls=%d", result, err, emitted.String(), calls.Load())
			}
			assertJSONEqual(t, result.Usage, `{"inputTokens":4,"outputTokens":6,"totalTokens":10}`)
		})
	}
}

func TestRemoteProviderCancellationAndSinkFailure(t *testing.T) {
	for _, sinkFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("sink_failure_%v", sinkFailure), func(t *testing.T) {
			serverStopped, firstEmitted, providerReturned := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			provider := remote(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, chunk("kept", nil))
				w.(http.Flusher).Flush()
				if sinkFailure {
					_, _ = io.WriteString(w, chunk("rejected", nil))
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
				close(serverStopped)
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			joined := providerFunc(func(w http.ResponseWriter, r *http.Request, model nodeservices.Model) {
				defer close(providerReturned)
				provider.Complete(w, r, model)
			})
			type outcome struct {
				result execution.Result
				err    error
			}
			finished := make(chan outcome, 1)
			go func() {
				result, err := Run(ctx, binding(joined), prompt, func(_ context.Context, delta string) error {
					if delta == "kept" {
						close(firstEmitted)
						return nil
					}
					return errors.New("private SQL connection credential")
				})
				finished <- outcome{result, err}
			}()
			await(t, firstEmitted, "first model delta")
			if !sinkFailure {
				cancel()
			}
			await(t, serverStopped, "upstream request cancellation")
			select {
			case got := <-finished:
				if got.result.Answer != "kept" || got.result.AnswerComplete || calls.Load() != 1 {
					t.Fatalf("result=%+v err=%v calls=%d", got.result, got.err, calls.Load())
				}
				if sinkFailure {
					if !errors.Is(got.err, ErrEmit) || got.result.Status != execution.StatusFailed || got.result.FailureCode != "model_event_write_failed" {
						t.Fatalf("sink failure result=%+v err=%v", got.result, got.err)
					}
				} else if !errors.Is(got.err, context.Canceled) || got.result.Status != execution.StatusCancelled {
					t.Fatalf("cancellation result=%+v err=%v", got.result, got.err)
				}
				if strings.Contains(fmt.Sprint(got.result, got.err), "credential") {
					t.Fatal("sink error leaked")
				}
				select {
				case <-providerReturned:
				default:
					t.Fatal("Run returned before Provider.Complete exited")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Run did not join cancelled provider")
			}
		})
	}
}

func TestRemoteProviderCancellationBeforeHeaders(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	provider := remote(t, func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(stopped)
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		result, err := Run(ctx, binding(provider), prompt, discard)
		if result.Status != execution.StatusCancelled || result.AnswerComplete {
			t.Errorf("result=%+v err=%v", result, err)
		}
		finished <- err
	}()
	await(t, started, "upstream request start")
	cancel()
	await(t, stopped, "upstream cancellation before headers")
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestRemoteProviderFailureAndTruncatedStream(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(fmt.Sprintf("http_rejected_%v", rejected), func(t *testing.T) {
			var calls atomic.Int32
			provider := remote(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if rejected {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = io.WriteString(w, "backend-private-key: provider private error")
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, chunk("partial", nil))
			})
			result, err := Run(t.Context(), binding(provider), prompt, discard)
			want := ErrIncomplete
			if rejected {
				want = ErrProvider
			}
			if !errors.Is(err, want) || result.Status != execution.StatusFailed || result.AnswerComplete || calls.Load() != 1 {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, calls.Load())
			}
			if strings.Contains(fmt.Sprint(result, err), "private") {
				t.Fatal("provider credential/error body leaked")
			}
		})
	}
}

func await(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}
