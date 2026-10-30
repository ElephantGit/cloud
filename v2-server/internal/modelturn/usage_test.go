package modelturn

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestUsageNormalization(t *testing.T) {
	tests := []struct {
		name, usage, want string
		invalid           bool
	}{
		{"absent", "", "", false},
		{"null", "null", "", false},
		{"partial", `{"prompt_tokens":12,"completion_tokens":3}`, "", false},
		{"empty", `{}`, "", false},
		{"complete", `{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}`, `{"inputTokens":12,"outputTokens":3,"totalTokens":15}`, false},
		{"reported zero", `{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}`, `{"inputTokens":0,"outputTokens":0,"totalTokens":0}`, false},
		{"optional", `{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":5},"completion_tokens_details":{"reasoning_tokens":2}}`, `{"inputTokens":12,"outputTokens":3,"totalTokens":15,"cachedReadTokens":5,"thoughtTokens":2}`, false},
		{"optional zero", `{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15,"completion_tokens_details":{"reasoning_tokens":0}}`, `{"inputTokens":12,"outputTokens":3,"totalTokens":15,"thoughtTokens":0}`, false},
		{"inconsistent smaller total", `{"prompt_tokens":12,"completion_tokens":3,"total_tokens":14}`, "", false},
		{"inconsistent larger total", `{"prompt_tokens":12,"completion_tokens":3,"total_tokens":20}`, "", false},
		{"negative", `{"prompt_tokens":-1,"completion_tokens":3,"total_tokens":2}`, "", true},
		{"fractional", `{"prompt_tokens":0.5}`, "", true},
		{"string", `{"prompt_tokens":"12"}`, "", true},
		{"array", `[]`, "", true},
		{"unsafe number", `{"prompt_tokens":9007199254740992}`, "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := jsonAnswer("answer", "stop")
			if test.usage != "" {
				body = strings.TrimSuffix(body, "}") + `,"usage":` + test.usage + "}"
			}
			result, err := Run(t.Context(), binding(response("application/json", body)), prompt, discard)
			if test.invalid {
				if !errors.Is(err, ErrInvalidResponse) || result.AnswerComplete {
					t.Fatalf("result=%+v err=%v", result, err)
				}
				return
			}
			if err != nil || !result.AnswerComplete {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if test.want == "" {
				if result.Usage != nil {
					t.Fatalf("unreported counters invented: %s", result.Usage)
				}
				return
			}
			assertJSONEqual(t, result.Usage, test.want)
		})
	}
}

func TestSSEFinalUsage(t *testing.T) {
	full := `data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}` + "\n\n"
	partial := `data: {"choices":[],"usage":{"prompt_tokens":12}}` + "\n\n"
	empty := `data: {"choices":[],"usage":{}}` + "\n\n"
	result, err := Run(t.Context(), binding(response("text/event-stream", chunk("answer", "stop")+full+done)), prompt, discard)
	if err != nil || !result.AnswerComplete {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	assertJSONEqual(t, result.Usage, `{"inputTokens":12,"outputTokens":3,"totalTokens":15}`)
	result, err = Run(t.Context(), binding(response("text/event-stream", chunk("answer", "stop")+full+partial+empty+done)), prompt, discard)
	if err != nil || !result.AnswerComplete {
		t.Fatalf("incomplete final metadata: result=%+v err=%v", result, err)
	}
	assertJSONEqual(t, result.Usage, `{"inputTokens":12,"outputTokens":3,"totalTokens":15}`)
	result, err = Run(t.Context(), binding(response("text/event-stream", full+chunk("answer", "stop")+done)), prompt, discard)
	if err != nil || !result.AnswerComplete {
		t.Fatalf("metadata before choice finish: result=%+v err=%v", result, err)
	}
	assertJSONEqual(t, result.Usage, `{"inputTokens":12,"outputTokens":3,"totalTokens":15}`)
	inconsistent := `data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":20}}` + "\n\n"
	result, err = Run(t.Context(), binding(response("text/event-stream", chunk("answer", "stop")+full+inconsistent+done)), prompt, discard)
	if err != nil || !result.AnswerComplete || result.Usage != nil {
		t.Fatalf("inconsistent counters must only clear usage: result=%+v err=%v", result, err)
	}
}

func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotValue, wantValue any
	if json.Unmarshal(got, &gotValue) != nil || json.Unmarshal([]byte(want), &wantValue) != nil {
		t.Fatalf("invalid JSON got=%s want=%s", got, want)
	}
	gotJSON, _ := json.Marshal(gotValue)
	wantJSON, _ := json.Marshal(wantValue)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("got=%s want=%s", got, want)
	}
}
