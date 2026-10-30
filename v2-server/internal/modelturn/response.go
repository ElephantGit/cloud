package modelturn

import (
	"bytes"
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"strings"

	"github.com/wanglongan587/cloud/v2-server/internal/strictjson"
)

// responseWriter keeps one synchronous owner: the provider's response-writing call stack.
// Rejecting a response cancels upstream I/O before returning the bounded error to that provider.
type responseWriter struct {
	ctx         context.Context
	cancel      context.CancelFunc
	header      http.Header
	emit        Emit
	started     bool
	format      string
	wireBytes   int
	jsonBody    bytes.Buffer
	answer      strings.Builder
	usage       json.RawMessage
	stopReason  string
	finished    bool
	sawContent  bool
	done        bool
	completed   bool
	err         error
	line, event []byte
	hasData     bool
	skipLF      bool
	sseStarted  bool
}

func (w *responseWriter) Header() http.Header { return w.header }

func (w *responseWriter) WriteHeader(status int) {
	if w.started {
		return
	}
	w.started = true
	if err := w.ctx.Err(); err != nil {
		w.fail(err)
		return
	}
	if status < 200 || status >= 300 {
		w.fail(ErrProvider)
		return
	}
	format, params, err := mime.ParseMediaType(w.header.Get("Content-Type"))
	if err != nil || (format != "application/json" && format != "text/event-stream") || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		w.fail(ErrInvalidResponse)
		return
	}
	w.format = format
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.completed {
		// The protocol terminator already completed this turn. Stop provider writes
		// without turning our own transport cancellation into a failed answer.
		return 0, context.Canceled
	}
	if err := w.ctx.Err(); err != nil {
		return 0, w.fail(err)
	}
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}
	if w.err != nil {
		return 0, w.err
	}
	if len(data) > maxWireBytes-w.wireBytes {
		return 0, w.fail(ErrLimit)
	}
	w.wireBytes += len(data)
	if w.format == "application/json" {
		_, _ = w.jsonBody.Write(data)
	} else if err := w.stream(data); err != nil {
		return 0, w.fail(err)
	}
	if w.done && len(w.line) == 0 && !w.hasData {
		if err := w.ctx.Err(); err != nil {
			return 0, w.fail(err)
		}
		// Validate the whole received write before stopping I/O, including any data
		// after DONE. Run still joins the provider before publishing completion.
		w.completed = true
		w.cancel()
	}
	return len(data), nil
}

func (w *responseWriter) Flush() {
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}
}

func (w *responseWriter) fail(err error) error {
	if w.err == nil {
		w.err = err
		w.cancel()
	}
	return w.err
}

func (w *responseWriter) finish() error {
	if w.err != nil {
		return w.err
	}
	if w.completed {
		return nil
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if !w.started {
		return ErrIncomplete
	}
	if w.format == "application/json" {
		if err := w.consume(w.jsonBody.Bytes(), false); err != nil {
			return err
		}
	} else if len(w.line) != 0 || w.hasData || !w.done {
		return ErrIncomplete
	}
	if !w.finished {
		return ErrIncomplete
	}
	return w.ctx.Err()
}

type completion struct {
	Choices []struct {
		Index        *int            `json:"index"`
		Message      json.RawMessage `json:"message"`
		Delta        json.RawMessage `json:"delta"`
		FinishReason *string         `json:"finish_reason"`
	} `json:"choices"`
	Usage json.RawMessage `json:"usage"`
	Error json.RawMessage `json:"error"`
}

type assistant struct {
	Role         *string           `json:"role"`
	Content      *string           `json:"content"`
	ToolCalls    json.RawMessage   `json:"tool_calls"`
	FunctionCall json.RawMessage   `json:"function_call"`
	Refusal      *string           `json:"refusal"`
	Annotations  []json.RawMessage `json:"annotations"`
	Audio        json.RawMessage   `json:"audio"`
}

func nonNull(raw json.RawMessage) bool {
	return len(raw) != 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func (w *responseWriter) consume(data []byte, stream bool) error {
	if !strictjson.Valid(data) {
		return ErrInvalidResponse
	}
	var response completion
	if json.Unmarshal(data, &response) != nil || nonNull(response.Error) {
		return ErrInvalidResponse
	}
	usage, hasCounters, err := normalizeUsage(response.Usage)
	if err != nil {
		return err
	}
	if hasCounters {
		w.usage = usage
	}
	if len(response.Choices) == 0 {
		// Compatible providers may send filtering or usage metadata before, between,
		// or after text chunks. Such metadata never proves answer completion.
		if stream {
			return nil
		}
		return ErrInvalidResponse
	}
	if len(response.Choices) != 1 || response.Choices[0].Index == nil || *response.Choices[0].Index != 0 || w.finished {
		return ErrInvalidResponse
	}
	choice := response.Choices[0]
	content := choice.Message
	if stream {
		content = choice.Delta
		if nonNull(choice.Message) {
			return ErrInvalidResponse
		}
	} else if nonNull(choice.Delta) {
		return ErrInvalidResponse
	}
	if !nonNull(content) && !(stream && choice.FinishReason != nil && *choice.FinishReason != "") {
		return ErrInvalidResponse
	}
	var message assistant
	// Ignore provider extension fields (including reasoning); only content is answer
	// text. Known fields retain their type and unsupported-content checks below.
	if nonNull(content) && json.Unmarshal(content, &message) != nil {
		return ErrInvalidResponse
	}
	if (message.Role != nil && *message.Role != "assistant") || (!stream && (message.Role == nil || message.Content == nil)) {
		return ErrInvalidResponse
	}
	if len(message.Annotations) != 0 || nonNull(message.Audio) {
		return ErrUnsupported
	}
	if nonNull(message.FunctionCall) || (nonNull(message.ToolCalls) && !bytes.Equal(bytes.TrimSpace(message.ToolCalls), []byte("[]"))) {
		w.stopReason = "tool_calls"
		return ErrUnsupported
	}
	if message.Refusal != nil && *message.Refusal != "" {
		w.stopReason = "refusal"
		return ErrRefused
	}
	if message.Content != nil {
		w.sawContent = true
	}
	if message.Content != nil && *message.Content != "" {
		if len(*message.Content) > maxAnswerBytes-w.answer.Len() {
			return ErrLimit
		}
		if err := w.emit(w.ctx, *message.Content); err != nil {
			if contextErr := w.ctx.Err(); contextErr != nil {
				return w.fail(contextErr)
			}
			return w.fail(ErrEmit)
		}
		w.answer.WriteString(*message.Content)
	}
	if choice.FinishReason == nil || *choice.FinishReason == "" {
		if !stream {
			return ErrIncomplete
		}
		return nil
	}
	switch *choice.FinishReason {
	case "stop":
		if !w.sawContent {
			return ErrIncomplete
		}
		w.stopReason, w.finished = "stop", true
		return nil
	case "length":
		w.stopReason = "length"
		return ErrLength
	case "content_filter":
		w.stopReason = "content_filter"
		return ErrRefused
	case "tool_calls", "function_call":
		w.stopReason = *choice.FinishReason
		return ErrUnsupported
	default:
		return ErrInvalidResponse
	}
}
