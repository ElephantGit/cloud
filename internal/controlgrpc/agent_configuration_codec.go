package controlgrpc

import (
	"encoding/json"
	"io"
	"math"
	"strings"

	"github.com/wanglongan587/cloud/internal/controlpb"
	"github.com/wanglongan587/cloud/internal/core"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// agentConfigurationMessage transports only the immutable private configuration snapshot.
func agentConfigurationMessage(o core.Object) *controlpb.AgentModelConfiguration {
	if len(o) == 0 {
		return nil
	}
	settings, _ := json.Marshal(o.O("settings"))
	return &controlpb.AgentModelConfiguration{
		PluginId: o.S("pluginId"), PluginVersion: o.S("pluginVersion"), Target: o.S("target"), PackageSha256: o.S("packageSha256"),
		SchemaVersion: uint32(o.N("schemaVersion")), DeclarationFingerprint: o.S("declarationFingerprint"), AdapterVersion: uint32(o.N("adapterVersion")),
		ConfigurationRevision: uint64(o.N("configurationRevision")), SettingsJson: string(settings),
	}
}

// agentConfigurationObject retains exact JSON numbers when registering a dispatched execution.
func agentConfigurationObject(p *controlpb.AgentModelConfiguration) (core.Object, error) {
	var settings core.Object
	decoder := json.NewDecoder(strings.NewReader(p.GetSettingsJson()))
	decoder.UseNumber()
	if p.GetConfigurationRevision() > math.MaxInt64 || len(p.GetSettingsJson()) > 256*1024 || decoder.Decode(&settings) != nil || settings == nil || decoder.Decode(new(any)) != io.EOF {
		return nil, status.Error(codes.InvalidArgument, "invalid_dispatch")
	}
	return core.Object{
		"pluginId": p.GetPluginId(), "pluginVersion": p.GetPluginVersion(), "target": p.GetTarget(), "packageSha256": p.GetPackageSha256(),
		"schemaVersion": int64(p.GetSchemaVersion()), "declarationFingerprint": p.GetDeclarationFingerprint(), "adapterVersion": int64(p.GetAdapterVersion()),
		"configurationRevision": int64(p.GetConfigurationRevision()), "settings": settings,
	}, nil
}
