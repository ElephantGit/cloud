package execution

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Prompt preserves ACP ContentBlock values without changing the port when attachments arrive.
// The first runtime accepts text only. Each raw block must be independently valid JSON.
type Prompt []json.RawMessage

// TextPrompt constructs a text block without interpolating user content into JSON.
func TextPrompt(text string) Prompt {
	block, _ := json.Marshal(struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{Type: "text", Text: text})
	return Prompt{block}
}

// Text validates supported input and joins text blocks in input order. Non-text blocks fail
// explicitly instead of silently discarding user input. Unknown text metadata remains intact.
func (p Prompt) Text() (string, error) {
	if len(p) == 0 {
		return "", fmt.Errorf("%w: empty prompt", ErrInvalidRequest)
	}
	var text strings.Builder
	for _, raw := range p {
		var block struct {
			Type string  `json:"type"`
			Text *string `json:"text"`
		}
		if err := json.Unmarshal(raw, &block); err != nil || block.Type == "" {
			return "", fmt.Errorf("%w: malformed content block", ErrInvalidRequest)
		}
		if block.Type != "text" {
			return "", fmt.Errorf("%w: content type %q", ErrUnsupported, block.Type)
		}
		if block.Text == nil {
			return "", fmt.Errorf("%w: text block requires text", ErrInvalidRequest)
		}
		text.WriteString(*block.Text)
	}
	if strings.TrimSpace(text.String()) == "" {
		return "", fmt.Errorf("%w: empty prompt", ErrInvalidRequest)
	}
	return text.String(), nil
}
