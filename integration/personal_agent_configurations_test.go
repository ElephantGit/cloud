package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wanglongan587/cloud/internal/core"
	"github.com/wanglongan587/cloud/internal/pluginmarket"
)

func TestPersonalAgentConfigurationHTTP(t *testing.T) {
	f := setup(t)
	space := f.defaultSpaceID()
	declaration := `{"schemaVersion":2,"agent":{"modelAccess":{"contractVersion":1,"protocols":["anthropic-messages"],"modelCardinality":"single"}},"settings":{"style":{"type":"string","title":"Style","description":"Style","required":true}}}`
	compiled, err := pluginmarket.CompileAgentConfiguration([]byte(declaration))
	must(t, err)
	release := core.Object{
		"id": "official/ora-space.claude", "kind": "agent", "version": "1.0.0",
		"url": "https://packages.example.invalid/claude.orax", "sha256": strings.Repeat("b", 64),
		"agentConfiguration": core.Object{"schemaVersion": 2, "declarationSha256": compiled.Fingerprint, "declaration": declaration},
	}
	raw, err := json.Marshal(release)
	must(t, err)
	_, err = f.store.Pool.Exec(`INSERT INTO space_plugins(space_id,tenant_id,source_namespace,identifier,desired_state,desired_version,selected_release)
		SELECT id,tenant_id,'official','ora-space.claude','installed','1.0.0',$2 FROM collab_workspaces WHERE id=$1`, space, string(raw))
	must(t, err)
	connection := f.call("POST", "/api/v1/me/model-connections", core.Object{
		"name": "Personal", "protocol": "anthropic-messages", "baseUrl": "https://models.example.invalid/v1", "authMode": "bearer",
		"models": []any{core.Object{"id": "model", "name": "Model", "contextWindow": 128000, "maxTokens": 8192}},
	}, "agent-connection", 201).O("resource")
	path := "/api/v1/me/agent-configurations/" + space + "/official/ora-space.claude"
	initial := f.call("GET", path, nil, "", 200)
	if initial.N("revision") != 0 || !initial.B("cleared") {
		t.Fatal(initial)
	}
	body := core.Object{
		"revision": 0, "selectionVersion": 1, "declarationFingerprint": compiled.Fingerprint,
		"settings": core.Object{"style": "brief"}, "connectionId": connection.S("id"), "modelId": "model",
	}
	f.call("PUT", path, body, "", 400)
	saved := f.call("PUT", path, body, "agent-save", 200)
	replayed := f.call("PUT", path, body, "agent-save", 200)
	if saved.O("resource").N("revision") != 1 || replayed.O("resource").N("revision") != 1 {
		t.Fatal(saved, replayed)
	}
	f.call("PUT", path, body, "agent-stale", 409)
	body["revision"] = 1
	body["settings"] = core.Object{"style": false}
	f.call("PUT", path, body, "agent-wrong-type", 400)
	body["settings"] = core.Object{"style": "brief", "apiKey": "must-not-be-a-setting"}
	f.call("PUT", path, body, "agent-extra-setting", 400)
	body["settings"] = core.Object{"style": "brief"}
	body["userId"] = "caller-selected-owner"
	f.call("PUT", path, body, "agent-owner-field", 400)
	delete(body, "userId")
	encoded, err := json.Marshal(f.call("GET", path, nil, "", 200))
	must(t, err)
	for _, forbidden := range []string{"credentialId", "baseUrl", "apiKey", "token"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("private connection field %s in response", forbidden)
		}
	}
	f.call("DELETE", path, core.Object{"revision": 1}, "agent-clear", 200)
	body["revision"] = 0
	f.call("PUT", path, body, "agent-old-create", 409)
	body["revision"] = 2
	f.call("PUT", path, body, "agent-refill", 200)
}
