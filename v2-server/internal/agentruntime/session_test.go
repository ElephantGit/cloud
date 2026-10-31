//go:build linux

package agentruntime

import (
	"encoding/json"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/node"
	"github.com/wanglongan587/cloud/v2-server/internal/nodehistory"
)

func TestIncompleteOrForeignTerminalCannotConfirmStop(t *testing.T) {
	for _, body := range []string{
		`{"kind":"agent_session_ended","result":{}}`,
		`{"kind":"agent_session_ended","result":{"reason":"unknown"}}`,
		`{"kind":"agent_session_ended","result":{"reason":"user_ended"}}`,
		`{"kind":"agent_session_ended","result":{"reason":"user_ended","node":{"node_id":"foreign","incarnation_id":"old"}}}`,
	} {
		a := &agentSession{execution: "agent", stopped: make(chan struct{}), active: &activeTurn{id: "turn", done: make(chan turnOutcome, 1)}, project: &projectRuntime{runtime: &node.Runtime{Client: &node.Client{}}}}
		_, ack := a.observe(node.Message{Type: "agent_session_ended", OperationID: "op-agent", ExecutionID: "agent", Payload: json.RawMessage(body)})
		if ack {
			t.Fatal("acknowledged unconfirmed stop")
		}
		select {
		case <-a.stopped:
			t.Fatal("invalid terminal confirmed process stop")
		default:
		}
	}
}

func TestOnlyNormalTurnEndCanComplete(t *testing.T) {
	for _, reason := range []string{"cancelled", "max_tokens", "max_turn_requests", "refusal", "unknown private provider error"} {
		result := turnResult(&nodehistory.Turn{Answer: "partial", StopReason: reason})
		if result.Status == execution.StatusCompleted || result.AnswerComplete {
			t.Fatalf("incomplete turn reported complete: %+v", result)
		}
		if reason == "unknown private provider error" && result.StopReason != "" {
			t.Fatal("unknown reason leaked")
		}
	}
	result := turnResult(&nodehistory.Turn{Answer: "complete", StopReason: "end_turn"})
	if result.Status != execution.StatusCompleted || !result.AnswerComplete || result.Usage != nil {
		t.Fatalf("normal result: %+v", result)
	}
}
