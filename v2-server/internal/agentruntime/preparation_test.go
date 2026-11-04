//go:build linux

package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/node"
)

func recordingPreparation(fail error) (*preparation, *[]execution.Preparation) {
	var published []execution.Preparation
	return newPreparation(execution.Ref{SessionID: "s", TurnID: "t"}, func(_ context.Context, event execution.Event) error {
		if event.Kind != execution.EventPreparation || event.Preparation == nil {
			return errors.New("unexpected event")
		}
		published = append(published, *event.Preparation)
		return fail
	}), &published
}

func historyRecords(t *testing.T, lines ...map[string]any) *execution.HistorySnapshot {
	t.Helper()
	snapshot := &execution.HistorySnapshot{}
	for i, line := range lines {
		data, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		snapshot.Records = append(snapshot.Records, execution.HistoryRecord{Sequence: uint32(i), Data: data})
	}
	return snapshot
}

func update(kind, text string) map[string]any {
	return map[string]any{"type": "update", "update": map[string]any{"sessionUpdate": kind, "content": map[string]any{"text": text}}}
}

func TestPreparationStagesAreContiguousAndFinishOnFirstOutput(t *testing.T) {
	timing, published := recordingPreparation(nil)
	start := time.Now().Add(-time.Minute).UTC()
	timing.enter(stageRuntime, start)
	timing.enter(stageWorkspace, start.Add(time.Second))
	timing.reused()
	timing.enter(stageAgent, start.Add(2*time.Second))
	meta := start.Add(5 * time.Second)
	timing.observeHistory(historyRecords(t, map[string]any{"type": "meta", "at": meta}, update("user_message_chunk", "hi")))
	if got := timing.current(); got != stageResponse {
		t.Fatalf("stage after session record = %q", got)
	}
	timing.observeHistory(historyRecords(t, update("agent_thought_chunk", "  ")))
	if timing.current() == "" {
		t.Fatal("a blank thought ended the wait")
	}
	toolCall := map[string]any{"type": "update", "update": map[string]any{"sessionUpdate": "tool_call", "content": []any{map[string]any{"type": "content"}}}}
	timing.observeHistory(historyRecords(t, toolCall))
	if timing.current() != "" {
		t.Fatal("first output did not end the wait")
	}
	count := len(*published)
	timing.finish(time.Now())
	timing.enter(stageAgent, time.Now())
	if len(*published) != count {
		t.Fatal("a finished preparation published again")
	}
	stages := (*published)[count-1].Stages
	if len(stages) != 4 || !stages[1].Reused {
		t.Fatalf("stages = %+v", stages)
	}
	for i, stage := range stages {
		if stage.EndedAt == nil {
			t.Fatalf("stage %s never ended", stage.ID)
		}
		if i > 0 && !stage.StartedAt.Equal(*stages[i-1].EndedAt) {
			t.Fatalf("gap before %s: %+v", stage.ID, stages)
		}
	}
	if !stages[2].EndedAt.Equal(meta) {
		t.Fatalf("Agent stage ended at %v, want the session record's %v", stages[2].EndedAt, meta)
	}
}

func TestPreparationOutputWithoutSessionRecordStillCountsAsResponse(t *testing.T) {
	timing, published := recordingPreparation(nil)
	timing.enter(stageAgent, time.Now())
	timing.observeHistory(historyRecords(t, update("agent_message_chunk", "OK")))
	stages := (*published)[len(*published)-1].Stages
	if len(stages) != 2 || stages[1].ID != stageResponse || stages[1].EndedAt == nil {
		t.Fatalf("stages = %+v", stages)
	}
}

func TestPreparationClampsANodeTimeBeforeTheStage(t *testing.T) {
	timing, published := recordingPreparation(nil)
	start := time.Now().UTC()
	timing.enter(stageAgent, start)
	timing.enter(stageResponse, start.Add(-time.Second))
	stages := (*published)[len(*published)-1].Stages
	if stages[0].EndedAt.Before(stages[0].StartedAt) || stages[1].StartedAt.Before(*stages[0].EndedAt) {
		t.Fatalf("stages out of order: %+v", stages)
	}
}

func TestPreparationIgnoresPublicationFailure(t *testing.T) {
	timing, _ := recordingPreparation(errors.New("store down"))
	timing.enter(stageRuntime, time.Now())
	timing.finish(time.Now())
	if timing.current() != "" {
		t.Fatal("failed publication changed preparation state")
	}
}

func TestPreparationWithoutStagesPublishesNothing(t *testing.T) {
	timing, published := recordingPreparation(nil)
	timing.finish(time.Now())
	timing.observeHistory(historyRecords(t, update("agent_message_chunk", "OK")))
	if len(*published) != 0 {
		t.Fatalf("published %+v", *published)
	}
}

func TestDriverPublishesPreparationOnlyForTheFirstTurn(t *testing.T) {
	h := newIPCHarness(t, nil)
	ctx := ipcContext(t)
	session := h.session("session", "workspace")
	collect := func(events chan execution.Event) Emit {
		return func(_ context.Context, event execution.Event) error {
			if event.Kind == execution.EventPreparation {
				events <- event
			}
			return nil
		}
	}
	first := make(chan execution.Event, 32)
	done := startIPCRun(h, ctx, session, "first", collect(first))
	f := receiveIPC(t, h.created)
	start := nextIPCCommand(t, f, "start_agent_session")
	block := map[string]any{"history_sequence": 2, "turn_id": "first", "record": update("agent_thought_chunk", "想一想")}
	payload, _ := json.Marshal(map[string]any{"version": 1, "incomplete": false, "blocks": []any{block}})
	f.in <- node.Message{Type: "thread_preview", Version: 1, OperationID: start.OperationID, ExecutionID: start.ExecutionID, Payload: payload}
	f.finishTurn(t, start, "first", "OK")
	if outcome := receiveIPC(t, done); outcome.err != nil {
		t.Fatal(outcome.err)
	}
	var last execution.Event
	for len(first) > 0 {
		last = <-first
	}
	if last.Preparation == nil {
		t.Fatal("no preparation event")
	}
	var ids []string
	for _, stage := range last.Preparation.Stages {
		ids = append(ids, stage.ID)
		if stage.EndedAt == nil {
			t.Fatalf("stage %s never ended", stage.ID)
		}
	}
	want := []string{stageRuntime, stageWorkspace, stagePlugin, stageAgent, stageResponse}
	if len(ids) != len(want) {
		t.Fatalf("stages = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("stages = %v, want %v", ids, want)
		}
	}
	second := make(chan execution.Event, 32)
	done = startIPCRun(h, ctx, session, "second", collect(second))
	f.finishTurn(t, nextIPCCommand(t, f, "submit_user_turn"), "second", "OK")
	if outcome := receiveIPC(t, done); outcome.err != nil {
		t.Fatal(outcome.err)
	}
	if len(second) != 0 {
		t.Fatalf("later turn published preparation: %+v", <-second)
	}
}
