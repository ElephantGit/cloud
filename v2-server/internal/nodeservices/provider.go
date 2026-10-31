package nodeservices

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RemoteProvider forwards one protocol's model calls to one configured HTTPS provider. The API
// key stays in backend memory, redirects are forbidden, and upstream error bodies are discarded.
type RemoteProvider struct {
	endpoint, key string
	protocol      Protocol
	client        *http.Client
}

// NewRemoteProvider forwards OpenAI chat completions. baseURL is a trusted deployment URL
// ending in the provider's API prefix (usually /v1), never a URL supplied by a page.
// transport may provide a private trusted CA.
func NewRemoteProvider(baseURL, apiKey string, transport http.RoundTripper) (*RemoteProvider, error) {
	return newRemoteProvider(OpenAICompletions, baseURL, "/chat/completions", apiKey, transport)
}

// NewRemoteMessagesProvider forwards Anthropic Messages calls; baseURL is as for
// NewRemoteProvider, and the key is sent as x-api-key.
func NewRemoteMessagesProvider(baseURL, apiKey string, transport http.RoundTripper) (*RemoteProvider, error) {
	return newRemoteProvider(AnthropicMessages, baseURL, "/messages", apiKey, transport)
}

func newRemoteProvider(protocol Protocol, baseURL, path, apiKey string, transport http.RoundTripper) (*RemoteProvider, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("invalid model provider configuration")
	}
	if transport == nil {
		transport = &http.Transport{Proxy: nil, ForceAttemptHTTP2: true, MaxIdleConns: 16, MaxIdleConnsPerHost: 8, IdleConnTimeout: time.Minute, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second}
	}
	return &RemoteProvider{endpoint: strings.TrimRight(baseURL, "/") + path, key: apiKey, protocol: protocol, client: &http.Client{Transport: transport, Timeout: 5 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Complete preserves successful JSON/SSE payloads and usage fields. Only selected content and
// protocol headers cross the boundary, and the Agent's token is never forwarded to the provider.
func (p *RemoteProvider) Complete(w http.ResponseWriter, r *http.Request, model Model) {
	var body map[string]json.RawMessage
	d := json.NewDecoder(r.Body)
	if d.Decode(&body) != nil || body == nil || d.Decode(new(any)) != io.EOF {
		fault(w, 400, "invalid_completion")
		return
	}
	body["model"], _ = json.Marshal(model.ID)
	data, err := json.Marshal(body)
	if err != nil {
		fault(w, 400, "invalid_completion")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), "POST", p.endpoint, bytes.NewReader(data))
	if err != nil {
		fault(w, 502, "provider_unavailable")
		return
	}
	if p.protocol == AnthropicMessages {
		req.Header.Set("X-Api-Key", p.key)
		// The API version and beta features are part of the request's meaning, never a credential.
		req.Header.Set("Anthropic-Version", cmp.Or(r.Header.Get("Anthropic-Version"), "2023-06-01"))
		if beta := r.Header.Get("Anthropic-Beta"); beta != "" {
			req.Header.Set("Anthropic-Beta", beta)
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+p.key)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		fault(w, 502, "provider_unavailable")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.WarnContext(r.Context(), "model provider rejected request", "status", resp.StatusCode)
		fault(w, 502, "provider_rejected")
		return
	}
	content := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(content, "application/json") && !strings.HasPrefix(content, "text/event-stream") {
		fault(w, 502, "provider_response_invalid")
		return
	}
	w.Header().Set("Content-Type", content)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(flushWriter{w}, resp.Body)
}

type flushWriter struct{ http.ResponseWriter }

func (w flushWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}

// FixtureProvider is deterministic model behavior, not a simulated Agent. If a shell tool is
// advertised it asks the real Agent to read README and write a marker, then answers on tool return.
type FixtureProvider struct{}

// FixtureAnswer and FixtureMarker are stable assertions for the real Agent boundary test.
const FixtureAnswer = "V2 fixture complete. Real Agent executed the shell tool."
const FixtureMarker = "v2 fixture tool executed\n"

func (FixtureProvider) Complete(w http.ResponseWriter, r *http.Request, model Model) {
	var req struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	d := json.NewDecoder(r.Body)
	if d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF {
		fault(w, 400, "invalid_completion")
		return
	}
	tool := ""
	if len(req.Messages) > 0 && req.Messages[len(req.Messages)-1].Role != "tool" {
		for _, t := range req.Tools {
			if t.Function.Name == "bash" || t.Function.Name == "shell" {
				tool = t.Function.Name
				break
			}
		}
	}
	finish := "stop"
	answer := "V2 fixture response."
	if len(req.Messages) > 0 && req.Messages[len(req.Messages)-1].Role == "tool" {
		answer = FixtureAnswer
	}
	message := map[string]any{"role": "assistant", "content": answer}
	if tool != "" {
		args, _ := json.Marshal(fixtureCommand())
		message["content"] = nil
		message["tool_calls"] = []any{map[string]any{"index": 0, "id": "call_v2_fixture", "type": "function", "function": map[string]string{"name": tool, "arguments": string(args)}}}
		finish = "tool_calls"
	}
	if !req.Stream {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "v2-fixture", "object": "chat.completion", "model": model.ID, "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 32, "completion_tokens": 32, "total_tokens": 64}})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	for _, choice := range []any{map[string]any{"index": 0, "delta": message, "finish_reason": nil}, map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}} {
		data, _ := json.Marshal(map[string]any{"id": "v2-fixture", "object": "chat.completion.chunk", "model": model.ID, "choices": []any{choice}})
		if _, err := (flushWriter{w}).Write(append(append([]byte("data: "), data...), []byte("\n\n")...)); err != nil {
			return
		}
	}
	_, _ = flushWriter{w}.Write([]byte("data: [DONE]\n\n"))
}
