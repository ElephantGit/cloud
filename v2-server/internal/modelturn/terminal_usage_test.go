package modelturn

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// This is the redacted shape observed from a compatible provider: a normal stop,
// a second empty terminal choice with complete usage, then DONE. No live key is used.
const terminalUsage = `{"prompt_tokens":33,"completion_tokens":37,"total_tokens":70,"cost":0,"is_byok":false,"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":0,"audio_tokens":0,"video_tokens":0},"cost_details":{"upstream_inference_cost":0},"completion_tokens_details":{"reasoning_tokens":34,"image_tokens":0,"audio_tokens":0}}`
const terminalChoice = `{"index":0,"delta":{"content":"","role":"assistant"},"finish_reason":"stop","native_finish_reason":"stop"}`
const terminalAnswer = "Provider response."

func terminalPrefix() string {
	return chunk(terminalAnswer, nil) + "data: " + `{"choices":[{"index":0,"delta":{"content":"","role":"assistant","reasoning":null},"finish_reason":"stop","native_finish_reason":"stop"}]}` + "\n\n"
}

func terminalMetadata(choices, usage string) string {
	payload := `{"choices":[` + choices + `]`
	if usage != "" {
		payload += `,"usage":` + usage
	}
	return payload + "}"
}

func TestRemoteProviderRepeatedTerminalUsage(t *testing.T) {
	for _, delta := range []string{
		`"delta":{"content":"","role":"assistant"},`,
		``,
		`"delta":null,`,
		`"delta":{},`,
		`"delta":{"content":null,"role":null,"reasoning":null,"reasoning_details":null},`,
		`"delta":{"content":"","role":"","reasoning_content":"","reasoning_details":[ ]},`,
	} {
		t.Run(delta, func(t *testing.T) {
			choice := `{"index":0,` + delta + `"finish_reason":"stop","native_finish_reason":"stop"}`
			provider := remote(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, terminalPrefix()+"data: "+terminalMetadata(choice, terminalUsage)+"\n\n"+done)
			})
			var emitted strings.Builder
			result, err := Run(t.Context(), binding(provider), prompt, func(_ context.Context, text string) error {
				emitted.WriteString(text)
				return nil
			})
			if err != nil || result.Status != execution.StatusCompleted || !result.AnswerComplete || result.StopReason != "stop" || result.Answer != terminalAnswer || emitted.String() != terminalAnswer {
				t.Fatalf("result=%+v err=%v emitted=%q", result, err, emitted.String())
			}
			assertJSONEqual(t, result.Usage, `{"inputTokens":33,"outputTokens":37,"totalTokens":70,"thoughtTokens":34,"cachedReadTokens":0}`)
		})
	}
}

func TestRemoteProviderRejectsNonMetadataAfterTerminal(t *testing.T) {
	for _, test := range []struct{ name, choice, usage string }{
		{"new text", strings.Replace(terminalChoice, `"content":""`, `"content":"secret extra text"`, 1), terminalUsage},
		{"changed stop", strings.Replace(terminalChoice, `"finish_reason":"stop"`, `"finish_reason":"length"`, 1), terminalUsage},
		{"empty stop", strings.Replace(terminalChoice, `"finish_reason":"stop"`, `"finish_reason":""`, 1), terminalUsage},
		{"missing stop", strings.Replace(terminalChoice, `,"finish_reason":"stop"`, ``, 1), terminalUsage},
		{"foreign index", strings.Replace(terminalChoice, `"index":0`, `"index":1`, 1), terminalUsage},
		{"multiple choices", terminalChoice + "," + terminalChoice, terminalUsage},
		{"missing usage", terminalChoice, ""},
		{"null usage", terminalChoice, "null"},
		{"empty usage", terminalChoice, `{}`},
		{"partial usage", terminalChoice, `{"prompt_tokens":33}`},
		{"tool output", strings.Replace(terminalChoice, `"content":""`, `"content":"","tool_calls":[{"function":{"name":"secret tool"}}]`, 1), terminalUsage},
		{"function output", strings.Replace(terminalChoice, `"content":""`, `"content":"","function_call":{"name":"secret function"}`, 1), terminalUsage},
		{"refusal", strings.Replace(terminalChoice, `"content":""`, `"content":"","refusal":"secret refusal"`, 1), terminalUsage},
		{"reasoning", strings.Replace(terminalChoice, `"content":""`, `"content":"","reasoning":"secret reasoning"`, 1), terminalUsage},
		{"reasoning details", strings.Replace(terminalChoice, `"content":""`, `"content":"","reasoning_details":[{"type":"reasoning.text","text":"secret reasoning"}]`, 1), terminalUsage},
		{"reasoning details type", strings.Replace(terminalChoice, `"content":""`, `"content":"","reasoning_details":{}`, 1), terminalUsage},
		{"wrong role", strings.Replace(terminalChoice, `"role":"assistant"`, `"role":"user"`, 1), terminalUsage},
		{"role type", strings.Replace(terminalChoice, `"role":"assistant"`, `"role":42`, 1), terminalUsage},
		{"content type", strings.Replace(terminalChoice, `"content":""`, `"content":[]`, 1), terminalUsage},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := remote(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, terminalPrefix()+"data: "+terminalMetadata(test.choice, test.usage)+"\n\n"+done)
			})
			var emitted strings.Builder
			result, err := Run(t.Context(), binding(provider), prompt, func(_ context.Context, text string) error {
				emitted.WriteString(text)
				return nil
			})
			if err == nil || result.Status != execution.StatusFailed || result.AnswerComplete || result.Answer != terminalAnswer || emitted.String() != terminalAnswer || strings.Contains(fmt.Sprint(result, err), "secret") {
				t.Fatalf("result=%+v err=%v emitted=%q", result, err, emitted.String())
			}
		})
	}
}

func TestReasoningDetailsBeforeTerminalAreNotAnswer(t *testing.T) {
	for _, details := range []string{`[{"type":"reasoning.text","text":"private thought"}]`, `{"provider_extension":"private thought"}`} {
		body := "data: " + `{"choices":[{"index":0,"delta":{"reasoning_details":` + details + `},"finish_reason":null}]}` + "\n\n" + chunk(terminalAnswer, "stop") + done
		var emitted strings.Builder
		result, err := Run(t.Context(), binding(response("text/event-stream", body)), prompt, func(_ context.Context, text string) error {
			emitted.WriteString(text)
			return nil
		})
		if err != nil || !result.AnswerComplete || result.Answer != terminalAnswer || emitted.String() != terminalAnswer {
			t.Fatalf("result=%+v err=%v emitted=%q", result, err, emitted.String())
		}
	}
}

func TestTerminalUsageStillRequiresUnbrokenEnvelopeAndDone(t *testing.T) {
	metadata := "data: " + terminalMetadata(terminalChoice, terminalUsage) + "\n\n"
	withError := "data: " + strings.TrimSuffix(terminalMetadata(terminalChoice, terminalUsage), "}") + `,"error":{"message":"secret upstream error"}}` + "\n\n"
	for _, body := range []string{terminalPrefix() + metadata, terminalPrefix() + withError + done, terminalPrefix() + done + metadata} {
		result, err := Run(t.Context(), binding(response("text/event-stream", body)), prompt, discard)
		if err == nil || result.AnswerComplete || result.Status != execution.StatusFailed || strings.Contains(fmt.Sprint(result, err), "secret") {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
}
