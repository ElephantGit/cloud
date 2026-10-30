package execution

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestTextPromptPreservesUserContent(t *testing.T) {
	want := "你好 \"agent\"\n$PATH <script>"
	got, err := TextPrompt(want).Text()
	if err != nil || got != want {
		t.Fatalf("Text = %q, %v", got, err)
	}
	p := append(TextPrompt("first"), TextPrompt(" second")...)
	if got, err := p.Text(); err != nil || got != "first second" {
		t.Fatalf("blocks = %q, %v", got, err)
	}
}

func TestPromptRejectsUnusableInput(t *testing.T) {
	for _, tc := range []struct {
		name, block string
		want        error
	}{
		{"non-text", `{"type":"image","data":"..."}`, ErrUnsupported},
		{"missing text", `{"type":"text"}`, ErrInvalidRequest},
		{"null text", `{"type":"text","text":null}`, ErrInvalidRequest},
		{"wrong text type", `{"type":"text","text":10}`, ErrInvalidRequest},
		{"invalid json", `{`, ErrInvalidRequest},
		{"null", `null`, ErrInvalidRequest},
		{"empty", `{"type":"text","text":"  "}`, ErrInvalidRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := (Prompt{json.RawMessage(tc.block)}).Text(); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
	if _, err := (Prompt{}).Text(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty prompt = %v", err)
	}
}
