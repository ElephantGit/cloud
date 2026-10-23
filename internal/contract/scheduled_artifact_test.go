package contract

import (
	"slices"
	"testing"
)

// Pin the actual response shapes: the generic /runs fallback must never turn artifact reads into
// IssueRun, which would make regenerated clients incompatible with the public API.
func TestScheduledArtifactRouteResponseSchemas(t *testing.T) {
	paths := Document()["paths"].(map[string]any)
	for _, tc := range []struct{ path, schema string }{
		{"/api/v1/tenants/{tid}/issues/{iid}/runs/{rid}/artifacts", "ScheduledArtifactList"},
		{"/api/v1/tenants/{tid}/issues/{iid}/runs/{rid}/artifacts/{aidx}", "ScheduledArtifactRead"},
	} {
		operation := paths[tc.path].(map[string]any)["get"].(map[string]any)
		response := operation["responses"].(map[string]any)["200"].(map[string]any)
		schema := response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		if schema["$ref"] != "#/components/schemas/"+tc.schema {
			t.Fatalf("%s response = %v, want %s", tc.path, schema, tc.schema)
		}
	}
}

func TestSpaceEventIncludesScheduledArtifactInvalidation(t *testing.T) {
	schemas := Document()["components"].(map[string]any)["schemas"].(map[string]any)
	event := schemas["SpaceEvent"].(map[string]any)["properties"].(map[string]any)
	types := event["type"].(map[string]any)["enum"].([]string)
	if !slices.Contains(types, "scheduled_task.artifacts_updated") {
		t.Fatal("artifact invalidation missing from the published SSE contract")
	}
}
