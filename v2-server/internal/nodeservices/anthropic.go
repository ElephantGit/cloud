package nodeservices

import (
	"encoding/json"
	"io"
	"net/http"
)

// MessagesFixtureProvider is the Anthropic Messages counterpart of FixtureProvider: the same
// deterministic model behavior for an Agent that speaks Messages. Offered a Bash tool, it asks
// the real Agent to read README and write the marker, then answers once the tool result returns.
type MessagesFixtureProvider struct{}

func (MessagesFixtureProvider) Complete(w http.ResponseWriter, r *http.Request, model Model) {
	var req struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	d := json.NewDecoder(r.Body)
	if d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF || len(req.Messages) == 0 {
		fault(w, 400, "invalid_completion")
		return
	}
	// Content is either a string or a block list; only a block list can carry a tool result.
	var blocks []struct {
		Type string `json:"type"`
	}
	returned := false
	if json.Unmarshal(req.Messages[len(req.Messages)-1].Content, &blocks) == nil {
		for _, b := range blocks {
			returned = returned || b.Type == "tool_result"
		}
	}
	block := map[string]any{"type": "text", "text": "V2 fixture response."}
	stop := "end_turn"
	if returned {
		block["text"] = FixtureAnswer
	} else {
		for _, t := range req.Tools {
			if t.Name == "Bash" {
				block = map[string]any{"type": "tool_use", "id": "toolu_v2_fixture", "name": "Bash", "input": fixtureCommand()}
				stop = "tool_use"
				break
			}
		}
	}
	usage := map[string]int{"input_tokens": 32, "output_tokens": 32}
	if !req.Stream {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "msg_v2_fixture", "type": "message", "role": "assistant", "model": model.ID, "content": []any{block}, "stop_reason": stop, "stop_sequence": nil, "usage": usage})
		return
	}
	// A streamed block starts empty and arrives as one delta, as the Messages API sends it.
	start, delta := map[string]any{"type": "text", "text": ""}, map[string]any{"type": "text_delta", "text": block["text"]}
	if block["type"] == "tool_use" {
		input, _ := json.Marshal(block["input"])
		start = map[string]any{"type": "tool_use", "id": block["id"], "name": block["name"], "input": map[string]any{}}
		delta = map[string]any{"type": "input_json_delta", "partial_json": string(input)}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	for _, event := range []map[string]any{
		{"type": "message_start", "message": map[string]any{"id": "msg_v2_fixture", "type": "message", "role": "assistant", "model": model.ID, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]int{"input_tokens": 32, "output_tokens": 1}}},
		{"type": "content_block_start", "index": 0, "content_block": start},
		{"type": "content_block_delta", "index": 0, "delta": delta},
		{"type": "content_block_stop", "index": 0},
		{"type": "message_delta", "delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": map[string]int{"output_tokens": 32}},
		{"type": "message_stop"},
	} {
		data, _ := json.Marshal(event)
		if _, err := (flushWriter{w}).Write([]byte("event: " + event["type"].(string) + "\ndata: " + string(data) + "\n\n")); err != nil {
			return
		}
	}
}

// fixtureCommand is the shell work both fixtures request; the marker proves the Agent ran it.
func fixtureCommand() map[string]string {
	return map[string]string{"command": "(cat README.md 2>/dev/null || cat README) >/dev/null && printf '%s\\n' 'v2 fixture tool executed' > ora-v2-result.txt", "description": "Read README and write the v2 fixture marker"}
}
