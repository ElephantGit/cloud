package nodeservices

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func messages(t *testing.T, c *http.Client, url, key, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		req.Header.Set("X-Api-Key", key)
	}
	// Claude Code always sends this placeholder bearer next to the real key.
	req.Header.Set("Authorization", "Bearer acp-proxy")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(data)
}

// A grant names its protocol's route, and its token works only there: a Messages token cannot
// reach an OpenAI provider and an OpenAI token cannot reach a Messages provider.
func TestGrantTokenWorksOnlyOnItsProtocolRoute(t *testing.T) {
	s := testServiceWith(t, map[string]Binding{
		"openai":   {Model: testModel, Protocol: OpenAICompletions, Provider: FixtureProvider{}},
		"messages": {Model: testModel, Protocol: AnthropicMessages, Provider: MessagesFixtureProvider{}},
	})
	c, public := serviceClient(t, s, true), serviceClient(t, s, false)
	grant := func(execution, binding string) (token string, protocol string) {
		t.Helper()
		if err := s.Permit(execution, binding); err != nil {
			t.Fatal(err)
		}
		status, data := request(t, c, "POST", s.URL+"/internal/v1/model-grants", "", `{"bindingId":"`+binding+`","executionId":"`+execution+`"}`)
		var body struct{ Token, Protocol, ProxyBaseURL string }
		if status != 200 || json.Unmarshal(data, &body) != nil {
			t.Fatalf("grant %s=%d %s", binding, status, data)
		}
		if want := s.URL + proxyPath[Protocol(body.Protocol)]; body.ProxyBaseURL != want {
			t.Fatalf("grant %s proxy=%q want %q", binding, body.ProxyBaseURL, want)
		}
		return body.Token, body.Protocol
	}
	messagesToken, protocol := grant("claude", "messages")
	if protocol != "anthropic-messages" {
		t.Fatalf("Messages grant protocol=%q", protocol)
	}
	openAIToken, protocol := grant("opencode", "openai")
	if protocol != "openai-completions" {
		t.Fatalf("OpenAI grant protocol=%q", protocol)
	}
	route := s.URL + "/runtime/anthropic/v1/messages"
	if status, body := messages(t, public, route, messagesToken, `{"messages":[{"role":"user","content":"hi"}]}`); status != 200 || !strings.Contains(body, "V2 fixture response.") {
		t.Fatalf("Messages call=%d %s", status, body)
	}
	for name, key := range map[string]string{"no key": "", "OpenAI token": openAIToken, "unknown": "forged"} {
		if status, _ := messages(t, public, route, key, `{"messages":[{"role":"user","content":"hi"}]}`); status != 401 {
			t.Errorf("Messages route with %s=%d", name, status)
		}
	}
	if status, _ := request(t, public, "POST", s.URL+"/runtime/openai/v1/chat/completions", messagesToken, `{"messages":[]}`); status != 401 {
		t.Errorf("OpenAI route with Messages token=%d", status)
	}
	s.End("claude")
	if status, _ := messages(t, public, route, messagesToken, `{"messages":[{"role":"user","content":"hi"}]}`); status != 401 {
		t.Errorf("Messages route after End=%d", status)
	}
}

func TestMessagesFixtureAsksForBashThenAnswers(t *testing.T) {
	call := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		MessagesFixtureProvider{}.Complete(w, httptest.NewRequest("POST", "/", strings.NewReader(body)), testModel)
		return w
	}
	tools := `"tools":[{"name":"Read"},{"name":"Bash"}]`
	var reply struct {
		Model      string
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type, Text, Name string
			Input            map[string]string
		}
	}
	w := call(`{"messages":[{"role":"user","content":[{"type":"text","text":"go"}]}],` + tools + `}`)
	if json.Unmarshal(w.Body.Bytes(), &reply) != nil || reply.StopReason != "tool_use" || len(reply.Content) != 1 || reply.Content[0].Name != "Bash" || reply.Content[0].Input["command"] != fixtureCommand()["command"] || reply.Model != testModel.ID {
		t.Fatalf("first turn=%s", w.Body)
	}
	w = call(`{"messages":[{"role":"user","content":"go"},{"role":"assistant","content":[]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_v2_fixture","content":"ok"}]}],` + tools + `}`)
	if json.Unmarshal(w.Body.Bytes(), &reply) != nil || reply.StopReason != "end_turn" || reply.Content[0].Text != FixtureAnswer {
		t.Fatalf("after tool result=%s", w.Body)
	}
	w = call(`{"stream":true,"messages":[{"role":"user","content":"go"}],` + tools + `}`)
	var kinds []string
	var partial string
	for _, line := range strings.Split(w.Body.String(), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var event struct {
			Type  string
			Delta struct {
				PartialJSON string `json:"partial_json"`
			}
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatalf("stream event %q: %v", data, err)
		}
		kinds = append(kinds, event.Type)
		partial += event.Delta.PartialJSON
	}
	want := "message_start content_block_start content_block_delta content_block_stop message_delta message_stop"
	var input map[string]string
	if w.Header().Get("Content-Type") != "text/event-stream" || strings.Join(kinds, " ") != want || json.Unmarshal([]byte(partial), &input) != nil || input["command"] != fixtureCommand()["command"] {
		t.Fatalf("stream events=%v partial=%q", kinds, partial)
	}
	if w = call(`{"messages":[]}`); w.Code != 400 {
		t.Fatalf("empty request=%d", w.Code)
	}
}

func TestRemoteMessagesProviderUsesItsKeyAndProtocolHeaders(t *testing.T) {
	type seen struct{ path, key, auth, version, beta, caller, model string }
	observed := make(chan seen, 1)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data map[string]any
		_ = json.NewDecoder(r.Body).Decode(&data)
		model, _ := data["model"].(string)
		observed <- seen{r.URL.Path, r.Header.Get("X-Api-Key"), r.Header.Get("Authorization"), r.Header.Get("Anthropic-Version"), r.Header.Get("Anthropic-Beta"), r.Header.Get("X-Caller-Secret"), model}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"type":"message","usage":{"output_tokens":7}}`)
	}))
	defer upstream.Close()
	p, err := NewRemoteMessagesProvider(upstream.URL+"/v1", "backend-key", upstream.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"model":"attacker","messages":[]}`))
	r.Header.Set("X-Api-Key", "agent-token")
	r.Header.Set("Authorization", "Bearer acp-proxy")
	r.Header.Set("Anthropic-Version", "2023-06-01")
	r.Header.Set("Anthropic-Beta", "fine-grained-tool-streaming-2025-05-14")
	r.Header.Set("X-Caller-Secret", "do-not-forward")
	w := httptest.NewRecorder()
	p.Complete(w, r, testModel)
	want := seen{"/v1/messages", "backend-key", "", "2023-06-01", "fine-grained-tool-streaming-2025-05-14", "", testModel.ID}
	if got := <-observed; w.Code != 200 || got != want || !strings.Contains(w.Body.String(), `"output_tokens":7`) {
		t.Fatalf("upstream saw %+v (want %+v), response %d %s", got, want, w.Code, w.Body)
	}
	if _, err := NewRemoteMessagesProvider("http://example.test/v1", "key", nil); err == nil {
		t.Fatal("accepted a plain HTTP provider")
	}
}
