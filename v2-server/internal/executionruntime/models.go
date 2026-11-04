package executionruntime

import (
	"errors"
	"strings"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

// messagesPrefix marks the Anthropic Messages bindings. Their executors drop it, so a model
// slot reads opencode:<slot> or claude:<slot>.
const messagesPrefix = "messages-"

// ModelsFromEnvironment returns only real provider choices configured by the deployment. Each
// protocol may add one provider, selected by three environment values together: V2_MODEL_* for
// OpenAI chat completions and V2_ANTHROPIC_MODEL_* for Anthropic Messages. An empty result means
// no real model is available; callers must expose an empty catalog instead of falling back to
// deterministic fixtures.
func ModelsFromEnvironment(getenv func(string) string) (map[string]ModelChoice, error) {
	choices := make(map[string]ModelChoice, 2)
	for _, c := range []struct {
		env, id  string
		protocol nodeservices.Protocol
		provider func(baseURL, apiKey string) (*nodeservices.RemoteProvider, error)
	}{
		{"V2_MODEL", "configured", nodeservices.OpenAICompletions, func(b, k string) (*nodeservices.RemoteProvider, error) {
			return nodeservices.NewRemoteProvider(b, k, nil)
		}},
		{"V2_ANTHROPIC_MODEL", messagesPrefix + "configured", nodeservices.AnthropicMessages, func(b, k string) (*nodeservices.RemoteProvider, error) {
			return nodeservices.NewRemoteMessagesProvider(b, k, nil)
		}},
	} {
		base, key, name := strings.TrimSpace(getenv(c.env+"_BASE_URL")), strings.TrimSpace(getenv(c.env+"_API_KEY")), strings.TrimSpace(getenv(c.env+"_NAME"))
		if c.protocol == nodeservices.AnthropicMessages {
			if token := strings.TrimSpace(getenv("V2_ANTHROPIC_MODEL_AUTH_TOKEN")); token != "" {
				if key != "" {
					return nil, errors.New("configure either V2_ANTHROPIC_MODEL_API_KEY or V2_ANTHROPIC_MODEL_AUTH_TOKEN, not both")
				}
				key = token
				c.provider = func(b, k string) (*nodeservices.RemoteProvider, error) {
					return nodeservices.NewBearerMessagesProvider(b, k, nil)
				}
			}
		}
		if base == "" && key == "" && name == "" {
			continue
		}
		if base == "" || key == "" || name == "" {
			return nil, errors.New(c.env + " requires a base URL, credential and model name")
		}
		provider, err := c.provider(base, key)
		if err != nil {
			return nil, err
		}
		model := nodeservices.Model{ID: name, Name: name, ContextWindow: 128000, MaxTokens: 16384}
		choices[c.id] = ModelChoice{Model: execution.Model{ID: c.id, Name: name, BindingVersion: c.id + "-v1"}, Binding: nodeservices.Binding{Model: model, Protocol: c.protocol, Provider: provider}}
	}
	return choices, nil
}

// WithFixtureModels adds the deterministic credential-free bindings used by fixture-check and
// end-to-end tests. It returns a copy so callers cannot mutate the configured model map. Normal
// server startup must not call this helper because fixtures are not real user-selectable models.
func WithFixtureModels(models map[string]ModelChoice) map[string]ModelChoice {
	choices := make(map[string]ModelChoice, len(models)+2)
	for id, choice := range models {
		choices[id] = choice
	}
	fixture := nodeservices.Model{ID: "fixture", Name: "Fixture", ContextWindow: 128000, MaxTokens: 16384}
	choices["fixture"] = ModelChoice{
		Model:   execution.Model{ID: "fixture", Name: "Fixture", BindingVersion: "fixture-v1"},
		Binding: nodeservices.Binding{Model: fixture, Protocol: nodeservices.OpenAICompletions, Provider: nodeservices.FixtureProvider{}},
	}
	choices[messagesPrefix+"fixture"] = ModelChoice{
		Model:   execution.Model{ID: messagesPrefix + "fixture", Name: "Fixture", BindingVersion: "messages-fixture-v1"},
		Binding: nodeservices.Binding{Model: fixture, Protocol: nodeservices.AnthropicMessages, Provider: nodeservices.MessagesFixtureProvider{}},
	}
	return choices
}

// agentExecutor is one installed Agent plugin; it runs every model binding of its protocol.
type agentExecutor struct {
	prefix, agentID, agentName, version string
	protocol                            nodeservices.Protocol
}

var agentExecutors = []agentExecutor{
	{"opencode", "official/ora-space.opencode", "OpenCode", "0.6.11", nodeservices.OpenAICompletions},
	{"claude", "official/ora-space.claude", "Claude Code", "0.4.5", nodeservices.AnthropicMessages},
}

// executorFor returns the executor of choice on a, or false when they speak different protocols.
func executorFor(a agentExecutor, choice ModelChoice) (execution.Executor, bool) {
	if choice.Binding.Protocol != a.protocol {
		return execution.Executor{}, false
	}
	slot := strings.TrimPrefix(choice.Model.ID, messagesPrefix)
	return execution.Executor{ID: a.prefix + ":" + slot, Name: a.agentName + " / " + choice.Model.Name, AgentID: a.agentID, AgentName: a.agentName, PluginVersion: a.version, Model: choice.Model}, true
}
