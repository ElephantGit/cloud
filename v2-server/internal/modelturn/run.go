// Package modelturn executes one admitted, text-only model turn through a configured provider.
// It owns no admission, session store, Node, Workspace, HTTP listener, or automatic retry.
package modelturn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

const (
	maxMessages     = 256
	maxRequestBytes = 1 << 20
	maxWireBytes    = 64 << 20
	maxEventBytes   = 8 << 20
	maxAnswerBytes  = 16 << 20
	maxDuration     = 5 * time.Minute
)

// Errors are bounded public reasons, never upstream bodies or a persistence callback's error.
var (
	ErrInvalidResponse = errors.New("model_response_invalid")
	ErrIncomplete      = errors.New("model_response_incomplete")
	ErrLimit           = errors.New("model_limit_exceeded")
	ErrProvider        = errors.New("model_provider_failed")
	ErrUnsupported     = errors.New("model_response_unsupported")
	ErrLength          = errors.New("model_output_truncated")
	ErrRefused         = errors.New("model_refused")
	ErrEmit            = errors.New("model_event_write_failed")
)

// Message is a text item from the caller's already-authorized conversation history. Only user
// and assistant roles are supported; credentials, system messages and tool definitions are not.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Emit retains or publishes one ordered text fragment. It is called synchronously, must honor
// ctx, and may apply backpressure. An error cancels the provider and makes the turn fail; Run
// never reports that deltas or its returned Result have already been persisted.
type Emit func(ctx context.Context, delta string) error

// Run executes a single turn after the owner has committed admission. Binding comes from trusted
// backend configuration; messages end in the current user's prompt. JSON fallback and SSE are
// accepted, but only a complete plain-text response with a normal stop succeeds. No retries or
// continuation are performed. A failed Result may retain partial text and always has
// AnswerComplete=false. The caller must durably publish the returned terminal Result.
//
// Run invokes Provider.Complete synchronously with a cancellable, bounded context and returns
// only after that call has exited. Providers must honor request cancellation and use their writer
// from one goroutine. Closing an observer must not cancel this execution context.
// A complete SSE stop plus DONE cancels upstream I/O without changing the completed result.
func Run(ctx context.Context, binding nodeservices.Binding, messages []Message, emit Emit) (execution.Result, error) {
	if err := ctx.Err(); err != nil {
		return failed("", "", err), err
	}
	if binding.Provider == nil || binding.Model.ID == "" || binding.Model.MaxTokens == 0 {
		return failed("", "", execution.ErrUnavailable), execution.ErrUnavailable
	}
	if emit == nil || len(messages) == 0 || len(messages) > maxMessages || messages[0].Role != "user" || messages[len(messages)-1].Role != "user" {
		return failed("", "", execution.ErrInvalidRequest), execution.ErrInvalidRequest
	}
	size := 0
	for _, message := range messages {
		if (message.Role != "user" && message.Role != "assistant") || !utf8.ValidString(message.Content) || (message.Role == "user" && strings.TrimSpace(message.Content) == "") {
			return failed("", "", execution.ErrInvalidRequest), execution.ErrInvalidRequest
		}
		size += len(message.Content)
		if size > maxRequestBytes {
			return failed("", "", ErrLimit), ErrLimit
		}
	}
	body, err := json.Marshal(struct {
		Model         string    `json:"model"`
		Messages      []Message `json:"messages"`
		Stream        bool      `json:"stream"`
		N             int       `json:"n"`
		MaxTokens     uint64    `json:"max_tokens"`
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}{Model: binding.Model.ID, Messages: messages, Stream: true, N: 1, MaxTokens: binding.Model.MaxTokens,
		StreamOptions: struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true}})
	if err != nil || len(body) > maxRequestBytes {
		return failed("", "", ErrLimit), ErrLimit
	}
	providerContext, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()
	req, err := http.NewRequestWithContext(providerContext, http.MethodPost, "http://modelturn.invalid/chat/completions", bytes.NewReader(body))
	if err != nil {
		return failed("", "", execution.ErrInvalidRequest), execution.ErrInvalidRequest
	}
	req.Header.Set("Content-Type", "application/json")
	writer := &responseWriter{ctx: providerContext, cancel: cancel, header: make(http.Header), emit: emit}
	binding.Provider.Complete(writer, req, binding.Model)
	err = writer.finish()
	if err == nil {
		return execution.Result{Status: execution.StatusCompleted, Answer: writer.answer.String(), AnswerComplete: true, StopReason: "stop", Usage: writer.usage}, nil
	}
	// A local parser/sink failure cancels only the derived context and keeps its own reason.
	// Parent cancellation and the execution timeout supersede transport/parser incompleteness.
	if writer.err == nil || errors.Is(writer.err, context.Canceled) || errors.Is(writer.err, context.DeadlineExceeded) {
		if contextErr := providerContext.Err(); contextErr != nil {
			err = contextErr
		}
	}
	return failed(writer.answer.String(), writer.stopReason, err), err
}

func failed(answer, stopReason string, err error) execution.Result {
	result := execution.Result{Status: execution.StatusFailed, Answer: answer, StopReason: stopReason}
	switch {
	case errors.Is(err, context.Canceled):
		result.Status, result.StopReason = execution.StatusCancelled, "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		result.FailureCode = "model_timeout"
	case errors.Is(err, execution.ErrInvalidRequest):
		result.FailureCode = "model_request_invalid"
	case errors.Is(err, execution.ErrUnavailable):
		result.FailureCode = "model_unavailable"
	default:
		result.FailureCode = err.Error()
	}
	return result
}
