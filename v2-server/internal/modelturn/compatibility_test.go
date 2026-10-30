package modelturn

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

func TestRemoteProviderCompatibilityMetadata(t *testing.T) {
	metadata := "data: " + `{"choices":[],"prompt_filter_results":[{"prompt_index":0,"content_filter_results":{}}]}` + "\n\n"
	for _, test := range []struct {
		name, format, body, usage string
	}{
		{
			name: "JSON reasoning extensions and inconsistent usage", format: "application/json",
			body: `{"choices":[{"index":0,"message":{"role":"assistant","content":"final answer","reasoning_content":null,"reasoning":"private thought","provider_extension":{"text":"not answer"}},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":8}}`,
		},
		{
			name: "SSE filtering metadata and finish-only chunk", format: "text/event-stream",
			body: metadata +
				"data: " + `{"choices":[{"index":0,"delta":{"reasoning_content":"private thought"},"finish_reason":""}]}` + "\n\n" +
				chunk("final ", "") + metadata + chunk("answer", nil) +
				"data: " + `{"choices":[{"index":0,"finish_reason":"stop"}]}` + "\n\n" + metadata +
				"data: " + `{"choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}` + "\n\n" +
				"data: " + `{"choices":[],"usage":{}}` + "\n\n" + done,
			usage: `{"inputTokens":2,"outputTokens":3,"totalTokens":5}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := remote(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", test.format)
				_, _ = io.WriteString(w, test.body)
			})
			var emitted strings.Builder
			result, err := Run(t.Context(), binding(provider), prompt, func(_ context.Context, text string) error {
				emitted.WriteString(text)
				return nil
			})
			if err != nil || result.Status != execution.StatusCompleted || !result.AnswerComplete || result.Answer != "final answer" || emitted.String() != result.Answer {
				t.Fatalf("result=%+v err=%v emitted=%q", result, err, emitted.String())
			}
			if test.usage == "" {
				if result.Usage != nil {
					t.Fatalf("inconsistent usage retained: %s", result.Usage)
				}
			} else {
				assertJSONEqual(t, result.Usage, test.usage)
			}
		})
	}
}

func TestRemoteProviderCompletionStopsOpenStreamAndJoins(t *testing.T) {
	serverStopped, providerReturned := make(chan struct{}), make(chan struct{})
	provider := remote(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, chunk("final answer", "stop")+done)
		w.(http.Flusher).Flush()
		// A compatible upstream may keep its connection open after the protocol ended.
		<-r.Context().Done()
		close(serverStopped)
	})
	joined := providerFunc(func(w http.ResponseWriter, r *http.Request, model nodeservices.Model) {
		defer close(providerReturned)
		provider.Complete(w, r, model)
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := Run(ctx, binding(joined), prompt, discard)
	if err != nil || result.Status != execution.StatusCompleted || !result.AnswerComplete || result.Answer != "final answer" || ctx.Err() != nil {
		t.Fatalf("result=%+v err=%v parent=%v", result, err, ctx.Err())
	}
	select {
	case <-providerReturned:
	default:
		t.Fatal("Run returned before Provider.Complete exited")
	}
	await(t, serverStopped, "upstream shutdown after DONE")
}

func TestSSEExplicitEmptyAnswerIsComplete(t *testing.T) {
	result, err := Run(t.Context(), binding(response("text/event-stream", chunk("", "stop")+done)), prompt, discard)
	if err != nil || result.Status != execution.StatusCompleted || !result.AnswerComplete || result.Answer != "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
