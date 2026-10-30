package modelturn

import "encoding/json"

// normalizeUsage uses the ACP Usage field names. Counts are for this one provider invocation;
// aggregating a conversation remains the caller's job. Missing basic counters mean unavailable,
// not zero. Optional reasoning/cache counters are emitted only when explicitly reported.
// The boolean reports a complete basic counter set, even when inconsistent counts must clear
// earlier usage. Incomplete metadata leaves an earlier valid measurement intact.
func normalizeUsage(raw json.RawMessage) (json.RawMessage, bool, error) {
	if !nonNull(raw) {
		return nil, false, nil
	}
	var usage struct {
		Prompt        *uint64 `json:"prompt_tokens"`
		Completion    *uint64 `json:"completion_tokens"`
		Total         *uint64 `json:"total_tokens"`
		PromptDetails struct {
			Cached *uint64 `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionDetails struct {
			Reasoning *uint64 `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	}
	if json.Unmarshal(raw, &usage) != nil {
		return nil, false, ErrInvalidResponse
	}
	for _, count := range []*uint64{usage.Prompt, usage.Completion, usage.Total, usage.PromptDetails.Cached, usage.CompletionDetails.Reasoning} {
		if count != nil && *count > (1<<53)-1 {
			return nil, false, ErrInvalidResponse
		}
	}
	if usage.Prompt == nil || usage.Completion == nil || usage.Total == nil {
		return nil, false, nil
	}
	if *usage.Prompt+*usage.Completion != *usage.Total {
		return nil, true, nil
	}
	normalized := struct {
		Total     uint64  `json:"totalTokens"`
		Input     uint64  `json:"inputTokens"`
		Output    uint64  `json:"outputTokens"`
		Thought   *uint64 `json:"thoughtTokens,omitempty"`
		CacheRead *uint64 `json:"cachedReadTokens,omitempty"`
	}{*usage.Total, *usage.Prompt, *usage.Completion, usage.CompletionDetails.Reasoning, usage.PromptDetails.Cached}
	data, _ := json.Marshal(normalized)
	return data, true, nil
}
