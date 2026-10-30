package executionruntime

import (
	"errors"
	"strings"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

// ModelsFromEnvironment always includes a credential-free fixture. Exactly one optional
// OpenAI-compatible provider is selected by the three environment values; partial config fails.
func ModelsFromEnvironment(getenv func(string) string) (map[string]ModelChoice, error) {
	model := nodeservices.Model{ID: "fixture", Name: "Fixture", ContextWindow: 128000, MaxTokens: 16384}
	choices := map[string]ModelChoice{"fixture": {Model: execution.Model{ID: "fixture", Name: "Fixture", BindingVersion: "fixture-v1"}, Binding: nodeservices.Binding{Model: model, Provider: nodeservices.FixtureProvider{}}}}
	base, key, name := strings.TrimSpace(getenv("V2_MODEL_BASE_URL")), strings.TrimSpace(getenv("V2_MODEL_API_KEY")), strings.TrimSpace(getenv("V2_MODEL_NAME"))
	if base == "" && key == "" && name == "" {
		return choices, nil
	}
	if base == "" || key == "" || name == "" {
		return nil, errors.New("V2_MODEL_BASE_URL, V2_MODEL_API_KEY and V2_MODEL_NAME must be configured together")
	}
	provider, err := nodeservices.NewRemoteProvider(base, key, nil)
	if err != nil {
		return nil, err
	}
	model.ID = name
	model.Name = name
	choices["configured"] = ModelChoice{Model: execution.Model{ID: "configured", Name: name, BindingVersion: "configured-v1"}, Binding: nodeservices.Binding{Model: model, Provider: provider}}
	return choices, nil
}
