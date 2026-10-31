//go:build linux

package agentruntime

import (
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

// agentPlugin is one pinned Agent release every project Node installs. An Executor names it by
// AgentID and PluginVersion; its bindings must speak the plugin's protocol.
type agentPlugin struct {
	id, version string
	// name is the package's route on the local package listener; file is its prepared asset.
	name, file string
	pkg        func() (nodeservices.AgentPackage, error)
	protocol   nodeservices.Protocol
	// declaration is the digest of a declared plugin's Agent declaration. A declared plugin
	// applies a frozen model configuration itself; an undeclared one is configured by Node.
	declaration string
}

const (
	pluginID      = "official/ora-space.opencode"
	pluginVersion = "0.6.11"
)

var agentPlugins = []agentPlugin{
	{id: pluginID, version: pluginVersion, name: "opencode", file: "opencode-0.6.11.orax", pkg: nodeservices.HostOpenCodePackage, protocol: nodeservices.OpenAICompletions},
	{id: "official/ora-space.claude", version: "0.4.5", name: "claude", file: "claude-0.4.5.orax", pkg: nodeservices.HostClaudePackage, protocol: nodeservices.AnthropicMessages, declaration: nodeservices.ClaudeDeclarationSHA256},
}

func findPlugin(id, version string) (agentPlugin, bool) {
	for _, p := range agentPlugins {
		if p.id == id && p.version == version {
			return p, true
		}
	}
	return agentPlugin{}, false
}

// installSpec is the plugin's install_plugins entry; a declared plugin also binds its
// declaration digest so Node proves the archive carries exactly that declaration.
func (p agentPlugin) installSpec(pkg nodeservices.AgentPackage, url string) map[string]any {
	download := map[string]string{"url": url, "sha256": pkg.SHA256}
	release := map[string]any{"kind": "targets", "targets": []any{map[string]any{"target": pkg.Target, "download": download}}}
	spec := map[string]any{"plugin_id": p.id, "version": p.version, "release": release}
	if p.declaration != "" {
		spec["agent_configuration"] = map[string]any{"schema_version": 2, "declaration_sha256": p.declaration}
	}
	return spec
}

// agentConfiguration is the frozen configuration a declared plugin's session starts with. v2
// has no per-user Settings, so the revision is zero and Settings are empty.
func (p agentPlugin) agentConfiguration(model nodeservices.Model) (map[string]any, error) {
	if p.declaration == "" {
		return nil, nil
	}
	pkg, err := p.pkg()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"plugin_id": p.id, "plugin_version": p.version, "target": pkg.Target, "package_sha256": pkg.SHA256,
		"schema_version": 2, "declaration_fingerprint": p.declaration, "adapter_version": 1, "configuration_revision": 0,
		"settings_json": "{}", "protocol": p.protocol,
		"model": map[string]any{"id": model.ID, "name": model.Name, "context_window": model.ContextWindow, "max_tokens": model.MaxTokens},
	}, nil
}
