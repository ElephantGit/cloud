package executionruntime

import (
	"errors"
	"strings"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

// messagesPrefix marks the Anthropic Messages bindings. Their executors drop it, so the same
// slot reads opencode:fixture for OpenCode and claude:fixture for Claude Code.
const messagesPrefix = "messages-"

// ModelsFromEnvironment always includes a credential-free fixture for each protocol. Each
// protocol may add one configured provider, selected by three environment values together:
// V2_MODEL_* for OpenAI chat completions and V2_ANTHROPIC_MODEL_* for Anthropic Messages.
func ModelsFromEnvironment(getenv func(string) string) (map[string]ModelChoice, error) {
	fixture := nodeservices.Model{ID: "fixture", Name: "Fixture", ContextWindow: 128000, MaxTokens: 16384}
	choices := map[string]ModelChoice{
		"fixture":                  {Model: execution.Model{ID: "fixture", Name: "Fixture", BindingVersion: "fixture-v1"}, Binding: nodeservices.Binding{Model: fixture, Protocol: nodeservices.OpenAICompletions, Provider: nodeservices.FixtureProvider{}}},
		messagesPrefix + "fixture": {Model: execution.Model{ID: messagesPrefix + "fixture", Name: "Fixture", BindingVersion: "messages-fixture-v1"}, Binding: nodeservices.Binding{Model: fixture, Protocol: nodeservices.AnthropicMessages, Provider: nodeservices.MessagesFixtureProvider{}}},
	}
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
		if base == "" && key == "" && name == "" {
			continue
		}
		if base == "" || key == "" || name == "" {
			return nil, errors.New(c.env + "_BASE_URL, " + c.env + "_API_KEY and " + c.env + "_NAME must be configured together")
		}
		provider, err := c.provider(base, key)
		if err != nil {
			return nil, err
		}
		model := fixture
		model.ID, model.Name = name, name
		choices[c.id] = ModelChoice{Model: execution.Model{ID: c.id, Name: name, BindingVersion: c.id + "-v1"}, Binding: nodeservices.Binding{Model: model, Protocol: c.protocol, Provider: provider}}
	}
	return choices, nil
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
