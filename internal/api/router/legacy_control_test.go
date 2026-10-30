package router

import (
	"encoding/json"
	"testing"

	"github.com/wanglongan587/cloud/internal/core"
)

func TestLegacyOperationResponseOmitsGRPCSchedulingCounter(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "advance_defer", true: "claim_snapshot_effect"}[nested], func(t *testing.T) {
			op := core.Object{"id": "original", "version": int64(4), "state": "retry_wait", "errorCode": "node_unavailable", "nodeUnavailableAttempts": int64(3)}
			out := op
			if nested {
				out = core.Object{"operation": op, "clones": []core.Object{{"executionId": "original-execution"}}}
			}
			legacyOperationResponse(out)
			wire, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			var decoded core.Object
			if err := json.Unmarshal(wire, &decoded); err != nil {
				t.Fatal(err)
			}
			if nested {
				decoded = decoded.O("operation")
			}
			if _, leaked := decoded["nodeUnavailableAttempts"]; leaked {
				t.Fatalf("gRPC-only scheduling counter leaked into legacy HTTP response: %s", wire)
			}
			if decoded.S("id") != "original" || decoded.N("version") != 4 || decoded.S("state") != "retry_wait" || decoded.S("errorCode") != "node_unavailable" {
				t.Fatalf("legacy operation fields changed: %s", wire)
			}
		})
	}
}
