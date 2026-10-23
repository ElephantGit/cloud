package controlgrpc

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/wanglongan587/cloud/internal/controlpb"
	"github.com/wanglongan587/cloud/internal/core"
)

// A scheduled session crosses the contract as the admission Cloud froze: the trigger belongs to the
// durable input, so a dispatch that drops or changes it conflicts with the work item instead of
// silently running a scheduled occurrence as a manual session.
func TestScheduledSessionTriggerRoundTrip(t *testing.T) {
	const scheduled = `{"agentPluginId":"official/a","agentPluginVersion":"1.0.0","checkoutExecutionId":"checkout",` +
		`"gitIdentity":{"email":"ada@example.invalid","name":"Ada"},` +
		`"initialTurn":{"content":[{"text":"review"}],"turnId":"turn-1"},` +
		`"kind":"agent_session","trigger":"scheduled"}`
	const manual = `{"agentPluginId":"official/a","agentPluginVersion":"1.0.0","checkoutExecutionId":"checkout",` +
		`"gitIdentity":{"email":"ada@example.invalid","name":"Ada"},` +
		`"initialTurn":{"content":[{"text":"review"}],"turnId":"turn-1"},"kind":"agent_session"}`

	for name, tc := range map[string]struct {
		frozen  string
		trigger controlpb.AgentSessionTrigger
	}{
		"scheduled": {frozen: scheduled, trigger: controlpb.AgentSessionTrigger_AGENT_SESSION_TRIGGER_SCHEDULED},
		"manual":    {frozen: manual, trigger: controlpb.AgentSessionTrigger_AGENT_SESSION_TRIGGER_UNSPECIFIED},
	} {
		t.Run(name, func(t *testing.T) {
			var frozen core.Object
			if err := json.Unmarshal([]byte(tc.frozen), &frozen); err != nil {
				t.Fatal(err)
			}
			message := sessionMessage(frozen)
			if message.GetTrigger() != tc.trigger {
				t.Fatalf("trigger = %v, want %v", message.GetTrigger(), tc.trigger)
			}
			back, err := sessionObject(message)
			if err != nil {
				t.Fatal(err)
			}
			text, err := json.Marshal(back)
			if err != nil {
				t.Fatal(err)
			}
			// The dispatch equality gate compares this exact encoding, so the round trip must
			// reproduce the frozen input byte for byte.
			if string(text) != tc.frozen {
				t.Fatalf("round trip must reproduce the frozen input:\n got %s\nwant %s", text, tc.frozen)
			}
		})
	}
}

// The business verdict crosses as a structured value Cloud validates and stores verbatim; an
// unspecified verdict or an over-long detail is a malformed result rather than "not reported".
func TestScheduledRunResultCrossesAsAStructuredValue(t *testing.T) {
	node := &controlpb.NodeIdentity{NodeId: "node-1", NodeIncarnationId: "incarnation-1"}
	endedResult := func(detail *string, verdict controlpb.ScheduledVerdict) *controlpb.ExecutionResult {
		return &controlpb.ExecutionResult{
			Node: node,
			Outcome: &controlpb.ExecutionResult_AgentSessionEnded{AgentSessionEnded: &controlpb.AgentSessionEnded{
				Reason:          controlpb.AgentSessionEndReason_AGENT_SESSION_END_REASON_IDLE_TIMEOUT,
				ScheduledResult: &controlpb.ScheduledRunResult{Verdict: verdict, VerdictDetail: detail},
			}},
		}
	}

	stored, err := resultObject(endedResult(proto.String("no comparison base"), controlpb.ScheduledVerdict_SCHEDULED_VERDICT_RANGE_UNRESOLVED))
	if err != nil {
		t.Fatal(err)
	}
	if want := (core.Object{"verdict": "range_unresolved", "verdictDetail": "no comparison base"}); !objectEqual(stored.O("scheduledResult"), want) {
		t.Fatalf("stored scheduled result = %+v, want %+v", stored.O("scheduledResult"), want)
	}
	// A stored result returns to the Controller with the business facts it carried, so a recovered
	// dispatch is queried exactly as it reported.
	back := result(stored).GetAgentSessionEnded().GetScheduledResult()
	if back.GetVerdict() != controlpb.ScheduledVerdict_SCHEDULED_VERDICT_RANGE_UNRESOLVED || back.GetVerdictDetail() != "no comparison base" {
		t.Fatalf("stored verdict did not survive the round trip: %v", back)
	}
	// A run that reported no verdict stores no field, which reads as "not reported".
	bare, err := resultObject(&controlpb.ExecutionResult{
		Node: node,
		Outcome: &controlpb.ExecutionResult_AgentSessionEnded{AgentSessionEnded: &controlpb.AgentSessionEnded{
			Reason: controlpb.AgentSessionEndReason_AGENT_SESSION_END_REASON_CANCELLED,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bare["scheduledResult"] != nil {
		t.Fatalf("an unreported verdict must store no field, got %+v", bare["scheduledResult"])
	}
	if _, err := resultObject(endedResult(nil, controlpb.ScheduledVerdict_SCHEDULED_VERDICT_UNSPECIFIED)); err == nil {
		t.Fatal("an unspecified verdict must be refused")
	}
	if _, err := resultObject(endedResult(proto.String(string(make([]byte, core.MaxScheduledRunVerdictDetailRunes+1))), controlpb.ScheduledVerdict_SCHEDULED_VERDICT_FAIL)); err == nil {
		t.Fatal("an over-long verdict detail must be refused")
	}
}

// objectEqual compares two durable objects by their canonical encoding.
func objectEqual(got, want core.Object) bool {
	gotText, gotErr := json.Marshal(got)
	wantText, wantErr := json.Marshal(want)
	return gotErr == nil && wantErr == nil && string(gotText) == string(wantText)
}
