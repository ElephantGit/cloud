package requestmetrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

type memoryRecorder struct {
	beginErr, finishErr error
	finished            []Attempt
	begins              int
}

func (r *memoryRecorder) BeginModelRequest(_ context.Context, _ execution.Ref, a Attempt) (Attempt, error) {
	r.begins++
	a.ID = "attempt"
	return a, r.beginErr
}
func (r *memoryRecorder) FinishModelRequest(_ context.Context, a Attempt) error {
	r.finished = append(r.finished, a)
	return r.finishErr
}

type chunksProvider struct {
	calls  int
	chunks []string
}

func (p *chunksProvider) Complete(w http.ResponseWriter, _ *http.Request, _ nodeservices.Model) {
	p.calls++
	w.Header().Set("Content-Type", "text/event-stream")
	for _, chunk := range p.chunks {
		_, _ = w.Write([]byte(chunk))
	}
}

func TestRecorderRefusesDispatchAndRetriesOnlyPersistence(t *testing.T) {
	p := &chunksProvider{chunks: []string{"data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\r", "\n\r\n", "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":0,\"completion_tokens\":2,\"total_tokens\":2}}\n\n", "data: [DONE]\n\n"}}
	recorder := &memoryRecorder{beginErr: errors.New("database down")}
	binding := nodeservices.Binding{Protocol: nodeservices.OpenAICompletions, Provider: p}
	wrapped := Wrap(recorder, execution.Ref{SessionID: "trusted", TurnID: "turn"}, binding)
	request := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
	response := httptest.NewRecorder()
	wrapped.Complete(response, request, nodeservices.Model{ID: "fixture"})
	if response.Code != 503 || p.calls != 0 {
		t.Fatalf("unrecorded dispatch: %d %d", response.Code, p.calls)
	}
	recorder.beginErr = nil
	recorder.finishErr = errors.New("ambiguous commit")
	response = httptest.NewRecorder()
	wrapped.Complete(response, request, nodeservices.Model{ID: "fixture"})
	if p.calls != 1 || len(recorder.finished) != 3 {
		t.Fatalf("provider=%d finish=%d", p.calls, len(recorder.finished))
	}
	a := recorder.finished[0]
	if a.Outcome != "succeeded" || !a.Streamed || !a.TerminalObserved || a.FirstOutputMS == nil || a.Usage.Availability != "complete" || a.Usage.Input == nil || *a.Usage.Input != 0 || a.Usage.Output == nil || *a.Usage.Output != 2 {
		t.Fatalf("observation=%+v", a)
	}
	if response.Body.String() != strings.Join(p.chunks, "") {
		t.Fatal("forwarded payload changed")
	}
}

func TestRecorderRemoteDispatchBoundary(t *testing.T) {
	var calls int
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer upstream.Close()
	remote, err := nodeservices.NewRemoteProvider(upstream.URL, "test-key", upstream.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &memoryRecorder{}
	p := Wrap(recorder, execution.Ref{SessionID: "trusted", TurnID: "turn"}, nodeservices.Binding{Protocol: nodeservices.OpenAICompletions, Provider: remote})
	send := func(body string) int {
		w := httptest.NewRecorder()
		p.Complete(w, httptest.NewRequest("POST", "/", strings.NewReader(body)), nodeservices.Model{ID: "test"})
		return w.Code
	}
	if send("broken") != 400 || recorder.begins != 0 || calls != 0 {
		t.Fatal("invalid local request counted")
	}
	recorder.beginErr = errors.New("offline")
	if send("{}") != 503 || calls != 0 || len(recorder.finished) != 0 {
		t.Fatal("admission failure dispatched or finished")
	}
	recorder.beginErr = nil
	if send("{}") != 502 || calls != 1 || len(recorder.finished) != 1 {
		t.Fatal("missing upstream failure")
	}
	a := recorder.finished[0]
	if a.Outcome != "failed" || a.HTTPStatus == nil || *a.HTTPStatus != 429 {
		t.Fatalf("upstream status lost: %+v", a)
	}
}

func TestRecorderRejectsHTTP200Diagnostics(t *testing.T) {
	for _, body := range []string{`{"error":{"message":"diagnostic"}}`, `{}`} {
		w := observerWriter{ResponseWriter: httptest.NewRecorder(), observer: usageObserver{protocol: "openai-completions"}, start: time.Now()}
		_, _ = w.Write([]byte(body))
		w.finish()
		if !w.failed || w.observer.terminal {
			t.Fatal("non-model response reported as complete")
		}
	}
	o := usageObserver{protocol: "openai-completions"}
	o.observeEvent([]byte("data: {\"error\":{\"type\":\"upstream_error\"}}"))
	o.observeEvent([]byte("data: [DONE]"))
	if !o.providerError {
		t.Fatal("terminal marker hid provider error")
	}
}
