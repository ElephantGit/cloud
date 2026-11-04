// Package requestmetrics observes HTTP attempts without retaining prompts, responses or credentials.
package requestmetrics

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

// Attempt is an immutable completed observation; nil counters mean unavailable, never zero.
type Attempt struct {
	ProviderSource   string     `json:"providerSource"`
	ID               string     `json:"id"`
	StartedAt        time.Time  `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt"`
	ModelID          string     `json:"modelId"`
	Protocol         string     `json:"protocol"`
	Streamed         bool       `json:"streamed"`
	Outcome          string     `json:"outcome"`
	ErrorCode        string     `json:"errorCode"`
	HTTPStatus       *int       `json:"httpStatus"`
	DurationMS       *int64     `json:"durationMs"`
	FirstOutputMS    *int64     `json:"firstOutputMs"`
	TerminalObserved bool       `json:"terminalObserved"`
	Usage            Usage      `json:"usage"`
}

// Usage preserves protocol-specific independent counters and their availability.
type Usage struct {
	Input          *int64 `json:"inputTokens"`
	Output         *int64 `json:"outputTokens"`
	Total          *int64 `json:"totalTokens"`
	CacheRead      *int64 `json:"cacheReadTokens"`
	CacheWrite     *int64 `json:"cacheWriteTokens"`
	Reasoning      *int64 `json:"reasoningTokens"`
	InputSemantics string `json:"inputSemantics"`
	Availability   string `json:"availability"`
}

// Recorder admits attempts against trusted persisted turn identities and finishes them idempotently.
type Recorder interface {
	BeginModelRequest(context.Context, execution.Ref, Attempt) (Attempt, error)
	FinishModelRequest(context.Context, Attempt) error
}

// Wrap observes one admitted turn's provider calls. A failed begin never sends upstream.
func Wrap(db Recorder, ref execution.Ref, binding nodeservices.Binding) nodeservices.Provider {
	return &provider{db: db, ref: ref, binding: binding}
}

type provider struct {
	db      Recorder
	ref     execution.Ref
	binding nodeservices.Binding
}

func (p *provider) Complete(w http.ResponseWriter, r *http.Request, m nodeservices.Model) {
	source := "upstream"
	switch p.binding.Provider.(type) {
	case nodeservices.FixtureProvider, nodeservices.MessagesFixtureProvider:
		source = "fixture"
	}
	var a Attempt
	var err error
	out := &observerWriter{ResponseWriter: w, observer: usageObserver{protocol: string(p.binding.Protocol)}}
	begin := func() bool {
		a, err = p.db.BeginModelRequest(r.Context(), p.ref, Attempt{ProviderSource: source, ModelID: m.ID, Protocol: string(p.binding.Protocol), Outcome: "started", Usage: Usage{InputSemantics: "unknown", Availability: "unavailable"}})
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"code":"model_metrics_unavailable"}`))
			return false
		}
		out.start = time.Now()
		return true
	}

	var upstreamStatus *int
	if remote, ok := p.binding.Provider.(*nodeservices.RemoteProvider); ok {
		status := remote.CompleteObserved(out, r, m, begin)
		if a.ID == "" || err != nil {
			return
		}
		if status != 0 {
			upstreamStatus = &status
		}
	} else {
		if !begin() {
			return
		}
		p.binding.Provider.Complete(out, r, m)
	}
	out.finish()
	now := time.Now().UTC()
	duration := now.Sub(out.start).Milliseconds()
	a.FinishedAt = &now
	a.DurationMS = &duration
	a.FirstOutputMS = out.first
	a.Streamed = out.streamed
	a.TerminalObserved = out.observer.terminal && !out.observer.providerError
	status := out.status
	if status == 0 {
		status = 200
	}
	a.HTTPStatus = upstreamStatus
	if source == "fixture" {
		a.HTTPStatus = &status
	}
	a.Outcome = "incomplete"
	switch {
	case a.TerminalObserved && status < 400 && !out.failed:
		a.Outcome = "succeeded"
	case r.Context().Err() != nil:
		a.Outcome = "cancelled"
		a.ErrorCode = "model_request_cancelled"
	case status >= 400 || out.observer.providerError:
		a.Outcome = "failed"
		a.ErrorCode = "model_provider_failed"
	case out.failed:
		a.Outcome = "interrupted"
		a.ErrorCode = "model_response_interrupted"
	case a.TerminalObserved:
		a.Outcome = "succeeded"
	}
	a.Usage = out.observer.observedUsage()
	// Retrying persistence never retries the provider. An unresolved started row remains honest.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
	defer cancel()
	for range 3 {
		if p.db.FinishModelRequest(ctx, a) == nil {
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
}

// A bounded incremental decoder forwards bytes unchanged and never persists payloads.
type observerWriter struct {
	http.ResponseWriter
	observer                   usageObserver
	start                      time.Time
	first                      *int64
	status                     int
	streamed, failed, overflow bool
	buffer                     []byte
}

func (w *observerWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.streamed = strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream")
	w.ResponseWriter.WriteHeader(status)
}
func (w *observerWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	n, err := w.ResponseWriter.Write(b)
	if err != nil {
		w.failed = true
	}
	if w.status < 400 && !w.overflow {
		if len(w.buffer)+n > 8<<20 {
			w.buffer = nil
			w.overflow = true
			w.failed = true
		} else {
			w.buffer = append(w.buffer, b[:n]...)
			if w.streamed {
				w.events()
			}
		}
	}
	return n, err
}
func (w *observerWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (w *observerWriter) output(found bool) {
	if found && w.first == nil {
		v := time.Since(w.start).Milliseconds()
		w.first = &v
	}
}
func (w *observerWriter) events() {
	// Normalize line endings only in the observation copy, including CRLF split across writes.
	for {
		i := bytes.Index(w.buffer, []byte("\n\n"))
		end := 2
		if j := bytes.Index(w.buffer, []byte("\r\n\r\n")); j >= 0 && (i < 0 || j < i) {
			i = j
			end = 4
		}
		if i < 0 {
			return
		}
		w.output(w.observer.observeEvent(w.buffer[:i]))
		w.buffer = w.buffer[i+end:]
	}
}
func (w *observerWriter) finish() {
	if w.overflow || w.status >= 400 {
		return
	}
	if !w.streamed {
		if validProtocolJSON(w.buffer, w.observer.protocol) {
			w.output(w.observer.observeJSON(w.buffer))
		} else {
			w.failed = true
		}
	}
	w.buffer = nil
}
