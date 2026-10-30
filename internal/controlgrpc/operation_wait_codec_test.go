package controlgrpc

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/wanglongan587/cloud/internal/controlpb"
	"github.com/wanglongan587/cloud/internal/core"
)

func TestOperationWaitSnapshotFieldsRoundTrip(t *testing.T) {
	snap := snapshot(core.Object{"operation": core.Object{"nodeUnavailableAttempts": int64(6)}, "clones": []core.Object{{"executionId": "original", "createdAt": "2026-10-29T00:00:00Z"}}})
	wire, err := proto.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var decoded controlpb.OperationSnapshot
	if err := proto.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetOperation().GetNodeUnavailableAttempts() != 6 || len(decoded.GetClones()) != 1 || decoded.GetClones()[0].GetCreatedAt() != "2026-10-29T00:00:00Z" {
		t.Fatalf("durable wait state lost: %v", &decoded)
	}
	for _, reason := range []string{"node_unavailable", "clone_result_pending", "clone_result_timeout", "clone_result_unknown"} {
		value := enum(controlpb.DeferReason_value, "DEFER_REASON_", reason)
		if value == 0 || name(controlpb.DeferReason(value).String(), "DEFER_REASON_") != reason {
			t.Fatalf("reason lost: %s", reason)
		}
	}
}
