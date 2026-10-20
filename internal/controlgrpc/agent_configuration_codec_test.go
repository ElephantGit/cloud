package controlgrpc

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/wanglongan587/cloud/internal/core"
)

// Frozen settings cross both dispatch directions without float64 reinterpretation or credentials.
func TestAgentConfigurationRoundTrip(t *testing.T) {
	frozen := core.Object{
		"pluginId": "official/minimal", "pluginVersion": "1.0.0", "target": "x86_64-unknown-linux-gnu",
		"packageSha256": "a", "schemaVersion": int64(2), "declarationFingerprint": "b", "adapterVersion": int64(1),
		"configurationRevision": int64(4), "settings": core.Object{"budget": json.Number("9007199254740993"), "verbose": false, "style": "literal $(text)"},
	}
	got, err := agentConfigurationObject(agentConfigurationMessage(frozen))
	if err != nil || !reflect.DeepEqual(got, frozen) {
		t.Fatalf("non-secret frozen input changed: %v", err)
	}
	if agentConfigurationMessage(core.Object{}) != nil {
		t.Fatal("legacy input acquired a declaration")
	}
	p := agentConfigurationMessage(frozen)
	p.SettingsJson += " {}"
	if _, err := agentConfigurationObject(p); err == nil {
		t.Fatal("extra JSON value accepted")
	}
}
