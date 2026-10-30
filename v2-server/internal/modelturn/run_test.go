package modelturn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

type providerFunc func(http.ResponseWriter, *http.Request, nodeservices.Model)

func (f providerFunc) Complete(w http.ResponseWriter, r *http.Request, model nodeservices.Model) {
	f(w, r, model)
}

func binding(provider nodeservices.Provider) nodeservices.Binding {
	return nodeservices.Binding{Model: nodeservices.Model{ID: "configured-model", Name: "Configured model", MaxTokens: 1024, ContextWindow: 8192}, Provider: provider}
}

var prompt = []Message{{Role: "user", Content: "你好"}}

func discard(context.Context, string) error { return nil }

func response(format, body string) nodeservices.Provider {
	return providerFunc(func(w http.ResponseWriter, _ *http.Request, _ nodeservices.Model) {
		w.Header().Set("Content-Type", format)
		_, _ = io.WriteString(w, body)
	})
}

func jsonAnswer(text, reason string) string {
	content, _ := json.Marshal(text)
	finish, _ := json.Marshal(reason)
	return fmt.Sprintf(`{"choices":[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":%s}]}`, content, finish)
}

func chunk(text string, reason any) string {
	data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": text}, "finish_reason": reason}}})
	return "data: " + string(data) + "\n\n"
}

const done = "data: [DONE]\n\n"

func TestFixtureAndTrustedRequest(t *testing.T) {
	messages := []Message{{"user", "第一问"}, {"assistant", "第一答"}, {"user", "下一问"}}
	called, returned := false, false
	provider := providerFunc(func(w http.ResponseWriter, r *http.Request, model nodeservices.Model) {
		called = true
		defer func() { returned = true }()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var request map[string]json.RawMessage
		if err := json.Unmarshal(data, &request); err != nil {
			t.Fatal(err)
		}
		if len(request) != 6 || string(request["model"]) != `"configured-model"` || string(request["n"]) != "1" || string(request["stream"]) != "true" || string(request["max_tokens"]) != "1024" || string(request["stream_options"]) != `{"include_usage":true}` {
			t.Fatalf("unexpected model-only request: %s", data)
		}
		var history []Message
		if json.Unmarshal(request["messages"], &history) != nil || fmt.Sprint(history) != fmt.Sprint(messages) {
			t.Fatalf("history changed: %s", request["messages"])
		}
		if r.Header.Get("Authorization") != "" || model.ID != "configured-model" {
			t.Fatal("request crossed credential/model boundary")
		}
		r.Body = io.NopCloser(strings.NewReader(string(data)))
		nodeservices.FixtureProvider{}.Complete(w, r, model)
	})
	var emitted strings.Builder
	result, err := Run(t.Context(), binding(provider), messages, func(_ context.Context, delta string) error {
		emitted.WriteString(delta)
		return nil
	})
	if err != nil || result.Status != execution.StatusCompleted || !result.AnswerComplete || result.StopReason != "stop" || result.Answer != "V2 fixture response." || result.Answer != emitted.String() || result.Usage != nil || !called || !returned {
		t.Fatalf("fixture result=%+v, err=%v, emitted=%q, called=%v, returned=%v", result, err, emitted.String(), called, returned)
	}
}

func TestInputRejectedBeforeProvider(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		want     error
	}{
		{"empty", nil, execution.ErrInvalidRequest},
		{"empty prompt", []Message{{"user", " \n\t"}}, execution.ErrInvalidRequest},
		{"assistant first", []Message{{"assistant", "answer"}, {"user", "next"}}, execution.ErrInvalidRequest},
		{"assistant last", []Message{{"user", "question"}, {"assistant", "answer"}}, execution.ErrInvalidRequest},
		{"system", []Message{{"user", "question"}, {"system", "inject"}, {"user", "next"}}, execution.ErrInvalidRequest},
		{"tool", []Message{{"user", "question"}, {"tool", "inject"}, {"user", "next"}}, execution.ErrInvalidRequest},
		{"developer", []Message{{"user", "question"}, {"developer", "inject"}, {"user", "next"}}, execution.ErrInvalidRequest},
		{"invalid utf8", []Message{{"user", string([]byte{0xff})}}, execution.ErrInvalidRequest},
		{"too large", []Message{{"user", strings.Repeat("a", maxRequestBytes+1)}}, ErrLimit},
		{"encoded too large", []Message{{"user", "a" + strings.Repeat("\n", maxRequestBytes/2)}}, ErrLimit},
		{"too many", make([]Message, maxMessages+1), execution.ErrInvalidRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := providerFunc(func(http.ResponseWriter, *http.Request, nodeservices.Model) {
				t.Fatal("invalid request reached provider")
			})
			result, err := Run(t.Context(), binding(provider), test.messages, discard)
			if !errors.Is(err, test.want) || result.Status != execution.StatusFailed || result.AnswerComplete || result.Answer != "" {
				t.Fatalf("result=%+v err=%v, want %v", result, err, test.want)
			}
		})
	}
	provider := providerFunc(func(http.ResponseWriter, *http.Request, nodeservices.Model) {
		t.Fatal("invalid binding reached provider")
	})
	for _, invalid := range []nodeservices.Binding{{}, {Provider: provider}, {Model: nodeservices.Model{ID: "model"}, Provider: provider}} {
		if result, err := Run(t.Context(), invalid, prompt, discard); !errors.Is(err, execution.ErrUnavailable) || result.FailureCode != "model_unavailable" {
			t.Fatalf("invalid binding result=%+v err=%v", result, err)
		}
	}
	if _, err := Run(t.Context(), binding(provider), prompt, nil); !errors.Is(err, execution.ErrInvalidRequest) {
		t.Fatalf("nil sink: %v", err)
	}
}

func TestSSEFragmentationAndUnicode(t *testing.T) {
	// CR/LF/CRLF and every UTF-8 byte are divided between writes; data lines form one JSON value.
	body := ": heartbeat\r\nid: ignored\r\nevent: message\r\ndata: {\r\ndata: \"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"你🙂\\uD83D\\uDE80�\"},\"finish_reason\":null}]}\r\n\r\n" + chunk("\n世界", "stop") + strings.ReplaceAll(done, "\n", "\r")
	provider := providerFunc(func(w http.ResponseWriter, _ *http.Request, _ nodeservices.Model) {
		w.Header().Set("Content-Type", "text/event-stream; charset=UTF-8")
		for i := range len(body) {
			if _, err := w.Write([]byte(body[i : i+1])); err != nil {
				t.Errorf("byte %d rejected: %v", i, err)
				return
			}
		}
	})
	var deltas []string
	result, err := Run(t.Context(), binding(provider), prompt, func(_ context.Context, delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil || !result.AnswerComplete || result.Answer != "你🙂🚀�\n世界" || strings.Join(deltas, "") != result.Answer || len(deltas) != 2 {
		t.Fatalf("result=%+v err=%v deltas=%q", result, err, deltas)
	}
}

func TestSSELeadingBOMPreservesFirstDelta(t *testing.T) {
	for _, byteSplit := range []bool{false, true} {
		t.Run(fmt.Sprintf("byte_split_%v", byteSplit), func(t *testing.T) {
			body := "\ufeff" + chunk("first", nil) + chunk("second\ufefftext", "stop") + done
			provider := providerFunc(func(w http.ResponseWriter, _ *http.Request, _ nodeservices.Model) {
				w.Header().Set("Content-Type", "text/event-stream")
				if !byteSplit {
					_, _ = io.WriteString(w, body)
					return
				}
				for i := range len(body) {
					if _, err := io.WriteString(w, body[i:i+1]); err != nil {
						t.Errorf("byte %d rejected: %v", i, err)
						return
					}
				}
			})
			var deltas []string
			result, err := Run(t.Context(), binding(provider), prompt, func(_ context.Context, delta string) error {
				deltas = append(deltas, delta)
				return nil
			})
			if err != nil || !result.AnswerComplete || result.Answer != "firstsecond\ufefftext" || len(deltas) != 2 || deltas[0] != "first" || strings.Join(deltas, "") != result.Answer {
				t.Fatalf("result=%+v err=%v deltas=%q", result, err, deltas)
			}
		})
	}
}

func TestSSERepeatedOrDamagedBOMCannotHideText(t *testing.T) {
	tests := []struct{ name, body string }{
		{"twice at start", "\ufeff\ufeff" + chunk("lost", nil)},
		{"after first event", chunk("first", nil) + "\ufeff" + chunk("lost", nil)},
		{"inside field", strings.Replace(chunk("lost", nil), "data:", "da\ufeffta:", 1)},
		{"invalid utf8 bom", "\xef\xbb" + chunk("lost", nil)},
		{"valid utf8 damaged bom", "\xef\xbb\xae" + chunk("lost", nil)},
		{"invalid utf8 after event", chunk("first", nil) + "\xef\xbb" + chunk("lost", nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := test.body + chunk("second", "stop") + done
			provider := providerFunc(func(w http.ResponseWriter, _ *http.Request, _ nodeservices.Model) {
				w.Header().Set("Content-Type", "text/event-stream")
				for i := range len(body) {
					if _, err := io.WriteString(w, body[i:i+1]); err != nil {
						return
					}
				}
			})
			result, err := Run(t.Context(), binding(provider), prompt, discard)
			if !errors.Is(err, ErrInvalidResponse) || result.AnswerComplete || result.Status != execution.StatusFailed {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestInvalidAndIncompleteResponses(t *testing.T) {
	tests := []struct {
		name, format, body string
		want               error
		stop               string
	}{
		{"missing type", "", jsonAnswer("text", "stop"), ErrInvalidResponse, ""},
		{"html", "text/html", "secret provider error", ErrInvalidResponse, ""},
		{"wrong charset", "application/json; charset=latin1", jsonAnswer("text", "stop"), ErrInvalidResponse, ""},
		{"malformed json", "application/json", `{"choices":`, ErrInvalidResponse, ""},
		{"trailing json", "application/json", jsonAnswer("text", "stop") + `{}`, ErrInvalidResponse, ""},
		{"error envelope", "application/json", `{"error":{"message":"secret provider error"}}`, ErrInvalidResponse, ""},
		{"no index", "application/json", `{"choices":[{"message":{"role":"assistant","content":"text"},"finish_reason":"stop"}]}`, ErrInvalidResponse, ""},
		{"wrong index", "application/json", strings.Replace(jsonAnswer("text", "stop"), `"index":0`, `"index":1`, 1), ErrInvalidResponse, ""},
		{"two choices", "application/json", `{"choices":[{"index":0},{"index":1}]}`, ErrInvalidResponse, ""},
		{"missing role", "application/json", strings.Replace(jsonAnswer("text", "stop"), `"role":"assistant",`, "", 1), ErrInvalidResponse, ""},
		{"wrong role", "application/json", strings.Replace(jsonAnswer("text", "stop"), `"assistant"`, `"system"`, 1), ErrInvalidResponse, ""},
		{"content array", "application/json", strings.Replace(jsonAnswer("text", "stop"), `"content":"text"`, `"content":[]`, 1), ErrInvalidResponse, ""},
		{"missing finish", "application/json", strings.Replace(jsonAnswer("text", "stop"), `,"finish_reason":"stop"`, "", 1), ErrIncomplete, ""},
		{"length", "application/json", jsonAnswer("partial", "length"), ErrLength, "length"},
		{"content filter", "application/json", jsonAnswer("partial", "content_filter"), ErrRefused, "content_filter"},
		{"unknown finish", "application/json", jsonAnswer("partial", "secret provider error"), ErrInvalidResponse, ""},
		{"tool finish", "application/json", jsonAnswer("partial", "tool_calls"), ErrUnsupported, "tool_calls"},
		{"function finish", "application/json", jsonAnswer("partial", "function_call"), ErrUnsupported, "function_call"},
		{"tool payload", "application/json", strings.Replace(jsonAnswer("text", "stop"), `"role":"assistant"`, `"role":"assistant","tool_calls":[{"function":{"name":"bash"}}]`, 1), ErrUnsupported, "tool_calls"},
		{"function payload", "application/json", strings.Replace(jsonAnswer("text", "stop"), `"role":"assistant"`, `"role":"assistant","function_call":{"name":"bash"}`, 1), ErrUnsupported, "tool_calls"},
		{"refusal", "application/json", strings.Replace(jsonAnswer("text", "stop"), `"role":"assistant"`, `"role":"assistant","refusal":"private reason"`, 1), ErrRefused, "refusal"},
		{"invalid utf8", "application/json", strings.Replace(jsonAnswer("text", "stop"), "text", string([]byte{0xff}), 1), ErrInvalidResponse, ""},
		{"unpaired surrogate", "application/json", strings.Replace(jsonAnswer("text", "stop"), "text", `\ud800`, 1), ErrInvalidResponse, ""},
		{"missing done", "text/event-stream", chunk("partial", "stop"), ErrIncomplete, "stop"},
		{"missing finish sse", "text/event-stream", chunk("partial", nil) + done, ErrIncomplete, ""},
		{"unframed done", "text/event-stream", chunk("partial", "stop") + "data: [DONE]\n", ErrIncomplete, "stop"},
		{"done first", "text/event-stream", done, ErrIncomplete, ""},
		{"duplicate done", "text/event-stream", chunk("text", "stop") + done + done, ErrInvalidResponse, "stop"},
		{"data after done", "text/event-stream", chunk("text", "stop") + done + chunk("extra", nil), ErrInvalidResponse, "stop"},
		{"choice after finish", "text/event-stream", chunk("text", "stop") + chunk("extra", nil) + done, ErrInvalidResponse, "stop"},
		{"truncated event", "text/event-stream", chunk("partial", nil) + `data: {"choices":`, ErrIncomplete, ""},
		{"damaged event", "text/event-stream", "data: {broken}\n\n", ErrInvalidResponse, ""},
		{"sse unpaired surrogate", "text/event-stream", strings.Replace(chunk("text", "stop"), "text", `\udfff`, 1) + done, ErrInvalidResponse, ""},
		{"sse missing delta and content", "text/event-stream", `data: {"choices":[{"index":0,"finish_reason":"stop"}]}` + "\n\n" + done, ErrIncomplete, ""},
		{"sse missing content", "text/event-stream", `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n" + done, ErrIncomplete, ""},
		{"sse empty finish", "text/event-stream", chunk("partial", "") + done, ErrIncomplete, ""},
		{"sse only metadata", "text/event-stream", `data: {"choices":[],"prompt_filter_results":[]}` + "\n\n" + done, ErrIncomplete, ""},
		{"sse metadata error", "text/event-stream", `data: {"choices":[],"error":{"message":"secret provider error"}}` + "\n\n" + chunk("text", "stop") + done, ErrInvalidResponse, ""},
		{"sse reasoning without answer", "text/event-stream", `data: {"choices":[{"index":0,"delta":{"reasoning_content":"private thought"},"finish_reason":"stop"}]}` + "\n\n" + done, ErrIncomplete, ""},
		{"empty stream", "text/event-stream", "", ErrIncomplete, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Run(t.Context(), binding(response(test.format, test.body)), prompt, discard)
			if !errors.Is(err, test.want) || result.Status != execution.StatusFailed || result.AnswerComplete || result.StopReason != test.stop || result.FailureCode != test.want.Error() {
				t.Fatalf("result=%+v err=%v, want err=%v stop=%s", result, err, test.want, test.stop)
			}
			if strings.Contains(fmt.Sprint(result, err), "secret provider error") || strings.Contains(fmt.Sprint(result, err), "private reason") {
				t.Fatal("provider error leaked")
			}
		})
	}
}

func TestValidEmptyAnswerAndLiteralSurrogateEscape(t *testing.T) {
	for _, answer := range []string{"", `literal \ud800`} {
		result, err := Run(t.Context(), binding(response("application/json", jsonAnswer(answer, "stop"))), prompt, discard)
		if err != nil || !result.AnswerComplete || result.Answer != answer {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
}

func TestPlainTextMetadataJSONAndSSE(t *testing.T) {
	tests := []struct {
		name, fields string
		want         error
	}{
		{"absent", "", nil},
		{"standard empty metadata", `,"annotations":[],"audio":null`, nil},
		{"null metadata", `,"annotations":null,"audio":null`, nil},
		{"empty annotations with whitespace", ",\"annotations\":[ \t ]", nil},
		{"null audio", `,"audio":null`, nil},
		{"actual annotations", `,"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com","title":"citation","start_index":0,"end_index":2}}]`, ErrUnsupported},
		{"actual audio", `,"audio":{"id":"audio-id","data":"encoded audio","transcript":"spoken text"}`, ErrUnsupported},
		{"empty audio object", `,"audio":{}`, ErrUnsupported},
		{"malformed annotations", `,"annotations":{}`, ErrInvalidResponse},
		{"unknown null metadata", `,"unknown_block":null`, nil},
		{"unknown object metadata", `,"unknown_block":{"text":"not answer"}`, nil},
		{"null reasoning", `,"reasoning_content":null,"reasoning":null`, nil},
		{"reasoning text", `,"reasoning_content":"private thought","reasoning":"more private thought"`, nil},
		{"invalid known content type", `,"content":123`, ErrInvalidResponse},
		{"invalid known role type", `,"role":123`, ErrInvalidResponse},
		{"invalid known refusal type", `,"refusal":123`, ErrInvalidResponse},
	}
	for _, stream := range []bool{false, true} {
		for _, test := range tests {
			t.Run(fmt.Sprintf("stream_%v/%s", stream, test.name), func(t *testing.T) {
				message := `{"role":"assistant","content":"ok"` + test.fields + "}"
				body := `{"choices":[{"index":0,"message":` + message + `,"finish_reason":"stop"}]}`
				format := "application/json"
				if stream {
					format = "text/event-stream"
					body = "data: " + strings.Replace(body, `"message":`, `"delta":`, 1) + "\n\n" + done
				}
				var emitted strings.Builder
				result, err := Run(t.Context(), binding(response(format, body)), prompt, func(_ context.Context, delta string) error {
					emitted.WriteString(delta)
					return nil
				})
				if test.want != nil {
					if !errors.Is(err, test.want) || result.AnswerComplete || result.Status != execution.StatusFailed || result.FailureCode != test.want.Error() || emitted.Len() != 0 {
						t.Fatalf("result=%+v err=%v emitted=%q", result, err, emitted.String())
					}
					return
				}
				if err != nil || !result.AnswerComplete || result.Status != execution.StatusCompleted || result.Answer != "ok" || emitted.String() != "ok" {
					t.Fatalf("result=%+v err=%v emitted=%q", result, err, emitted.String())
				}
			})
		}
	}
}

func TestLimitsCancelProvider(t *testing.T) {
	tests := []struct {
		name, format string
		write        func(http.ResponseWriter)
	}{
		{"event line", "text/event-stream", func(w http.ResponseWriter) { _, _ = io.WriteString(w, strings.Repeat("x", maxEventBytes+1)) }},
		{"event data lines", "text/event-stream", func(w http.ResponseWriter) {
			line := "data: " + strings.Repeat("x", maxEventBytes/2) + "\n"
			_, _ = io.WriteString(w, line+line)
		}},
		{"answer", "text/event-stream", func(w http.ResponseWriter) {
			text := strings.Repeat("x", maxAnswerBytes/4)
			for range 5 {
				_, _ = io.WriteString(w, chunk(text, nil))
			}
		}},
		{"json answer", "application/json", func(w http.ResponseWriter) {
			_, _ = io.WriteString(w, jsonAnswer(strings.Repeat("x", maxAnswerBytes+1), "stop"))
		}},
		{"wire", "application/json", func(w http.ResponseWriter) {
			block := []byte(strings.Repeat(" ", 1<<20))
			for range 65 {
				if _, err := w.Write(block); err != nil {
					return
				}
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := providerFunc(func(w http.ResponseWriter, r *http.Request, _ nodeservices.Model) {
				w.Header().Set("Content-Type", test.format)
				test.write(w)
				// JSON answer validation happens only once the synchronous provider returns.
				if test.name != "json answer" && r.Context().Err() != context.Canceled {
					t.Fatal("parser rejection did not cancel provider")
				}
			})
			result, err := Run(t.Context(), binding(provider), prompt, discard)
			if !errors.Is(err, ErrLimit) || result.AnswerComplete || len(result.Answer) > maxAnswerBytes {
				t.Fatalf("result status=%s complete=%v bytes=%d err=%v", result.Status, result.AnswerComplete, len(result.Answer), err)
			}
		})
	}
}

func TestPreCancelledAndDeadline(t *testing.T) {
	provider := providerFunc(func(http.ResponseWriter, *http.Request, nodeservices.Model) {
		t.Fatal("already cancelled request reached provider")
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := Run(ctx, binding(provider), prompt, discard)
	if !errors.Is(err, context.Canceled) || result.Status != execution.StatusCancelled || result.AnswerComplete || result.FailureCode != "" || result.StopReason != "cancelled" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	result, err = Run(ctx, binding(provider), prompt, discard)
	if !errors.Is(err, context.DeadlineExceeded) || result.Status != execution.StatusFailed || result.AnswerComplete || result.FailureCode != "model_timeout" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestCancelledTransportFaultIsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	provider := providerFunc(func(w http.ResponseWriter, _ *http.Request, _ nodeservices.Model) {
		cancel()
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "private transport error")
	})
	result, err := Run(ctx, binding(provider), prompt, discard)
	if !errors.Is(err, context.Canceled) || result.Status != execution.StatusCancelled || result.AnswerComplete {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestCancellationWhileJSONBufferedOrEmitting(t *testing.T) {
	for _, cancelDuringEmit := range []bool{false, true} {
		t.Run(fmt.Sprintf("during_emit_%v", cancelDuringEmit), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			provider := providerFunc(func(w http.ResponseWriter, _ *http.Request, _ nodeservices.Model) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, jsonAnswer("buffered text", "stop"))
				if !cancelDuringEmit {
					cancel()
				}
			})
			emitted := false
			result, err := Run(ctx, binding(provider), prompt, func(context.Context, string) error {
				emitted = true
				cancel()
				return nil
			})
			if !errors.Is(err, context.Canceled) || result.Status != execution.StatusCancelled || result.AnswerComplete || emitted != cancelDuringEmit {
				t.Fatalf("result=%+v err=%v emitted=%v", result, err, emitted)
			}
		})
	}
}

func TestCancellationWhileEmitAppliesBackpressure(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		result, err := Run(ctx, binding(response("text/event-stream", chunk("pending", nil))), prompt, func(ctx context.Context, _ string) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
		if result.Status != execution.StatusCancelled || result.AnswerComplete || result.Answer != "" {
			t.Errorf("result=%+v err=%v", result, err)
		}
		finished <- err
	}()
	await(t, started, "blocked Emit")
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after blocked Emit was cancelled")
	}
}
