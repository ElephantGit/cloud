package nodehistory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/node"
)

func record(seq uint32, kind string, extra map[string]any) json.RawMessage {
	m := map[string]any{"at": "2026-10-29T12:00:00Z", "seq": seq, "type": kind}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return b
}
func meta() json.RawMessage {
	return record(0, "meta", map[string]any{"sessionId": "agent", "schemaVersion": 2})
}
func update(seq uint32, kind, text string) json.RawMessage {
	return record(seq, "update", map[string]any{"update": map[string]any{"sessionUpdate": kind, "content": map[string]any{"type": "text", "text": text}}})
}
func ended(seq uint32, reason string) json.RawMessage {
	return record(seq, "turnEnded", map[string]any{"stop_reason": reason})
}

func setupReader(t *testing.T, lines ...json.RawMessage) (*Reader, string) {
	t.Helper()
	home := t.TempDir()
	path, err := historyPath("agent")
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(home, path)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	var data strings.Builder
	for _, line := range lines {
		data.Write(line)
		data.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(data.String()), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r, path
}

func TestReadTurnUsesFullOrderedCorrectedRecordsAndExactBoundary(t *testing.T) {
	long := strings.Repeat("长回答🙂\n", 40000)
	r, _ := setupReader(t, meta(), update(1, "user_message_chunk", "first"), update(3, "agent_message_chunk", "old"), update(2, "agent_thought_chunk", "secret thought"), update(3, "agent_message_chunk", long), ended(4, "end_turn"), update(5, "user_message_chunk", "second"), update(6, "agent_message_chunk", "partial"), ended(7, "cancelled"))
	first, err := r.ReadTurn(context.Background(), "agent", 4, "end_turn")
	if err != nil {
		t.Fatal(err)
	}
	if first.Answer != long || len(first.Records) != 4 {
		t.Fatalf("wrong first result: len=%d records=%d", len(first.Answer), len(first.Records))
	}
	second, err := r.ReadTurn(context.Background(), "agent", 7, "cancelled")
	if err != nil || second.Answer != "partial" || second.StopReason != "cancelled" {
		t.Fatalf("second=%+v %v", second, err)
	}
	if _, err := r.ReadTurn(context.Background(), "agent", 8, "end_turn"); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("missing turn returned old answer: %v", err)
	}
	if _, err := r.ReadTurn(context.Background(), "agent", 7, "end_turn"); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("wrong reason=%v", err)
	}
}

func TestChildThoughtToolAndPreviousTurnCannotBecomeAnswer(t *testing.T) {
	child := record(3, "update", map[string]any{"update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "child answer"}, "_meta": map[string]any{"opencode/child-session": map[string]any{"id": "child", "parentID": "parent", "depth": 1}}}})
	r, _ := setupReader(t, meta(), update(1, "user_message_chunk", "prompt"), update(2, "agent_message_chunk", "parent"), child, update(4, "agent_thought_chunk", "thought"), record(5, "update", map[string]any{"update": map[string]any{"sessionUpdate": "tool_call", "rawOutput": "tool output", "content": []any{map[string]any{"type": "content", "content": map[string]string{"type": "text", "text": "tool log"}}}}}), ended(6, "end_turn"))
	got, err := r.ReadTurn(context.Background(), "agent", 6, "end_turn")
	if err != nil || got.Answer != "parent" {
		t.Fatalf("answer=%q err=%v", got.Answer, err)
	}
}

func TestReaderFailsClosedForMissingCorruptGappedAndOversizedHistory(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []json.RawMessage
		want  error
	}{
		{"corrupt", []json.RawMessage{meta(), []byte("broken"), ended(2, "end_turn")}, ErrDamaged},
		{"gap", []json.RawMessage{meta(), ended(2, "end_turn")}, ErrIncomplete},
		{"gap_marker", []json.RawMessage{meta(), record(1, "gap", nil), ended(2, "end_turn")}, ErrIncomplete},
		{"unknown_record", []json.RawMessage{meta(), record(1, "unknown", nil), ended(2, "end_turn")}, ErrDamaged},
		{"identity", []json.RawMessage{record(0, "meta", map[string]any{"sessionId": "other", "schemaVersion": 2}), update(1, "agent_message_chunk", "wrong"), ended(2, "end_turn")}, ErrIdentity},
		{"oversized", []json.RawMessage{meta(), update(1, "agent_message_chunk", strings.Repeat("x", maxLineBytes)), ended(2, "end_turn")}, ErrLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := setupReader(t, tc.lines...)
			_, err := r.ReadTurn(context.Background(), "agent", 2, "end_turn")
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	r, path := setupReader(t, meta(), update(1, "agent_message_chunk", "answer"), ended(2, "end_turn"))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(`{"seq":3`)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.ReadTurn(context.Background(), "agent", 2, "end_turn")
	if err != nil || got.Answer != "answer" {
		t.Fatalf("closed turn before torn tail=%+v %v", got, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadTurn(context.Background(), "agent", 2, "end_turn"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing history=%v", err)
	}
	for _, id := range []string{"../agent", "/etc/passwd", "a/b", "", "a\\b"} {
		if _, err := r.Record(context.Background(), id, 1); !errors.Is(err, ErrIdentity) {
			t.Fatalf("unsafe id %q: %v", id, err)
		}
	}
}

func TestRootRejectsHistorySymlinkEscape(t *testing.T) {
	r, path := setupReader(t, meta(), update(1, "agent_message_chunk", "answer"), ended(2, "end_turn"))
	external := filepath.Join(t.TempDir(), "other.jsonl")
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(external, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := r.ReadTurn(context.Background(), "agent", 2, "end_turn"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("escaped root: %v", err)
	}
}

func TestPreviewReplacementAndSettleDoNotDuplicateText(t *testing.T) {
	r, _ := setupReader(t, meta(), update(1, "agent_message_chunk", "full answer"), ended(2, "end_turn"))
	p := NewProjection(r, "agent")
	preview := func(version int, text string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"version": version, "blocks": []any{map[string]any{"history_sequence": 1, "record": map[string]any{"type": "update", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": text}}}}}})
		return b
	}
	if err := p.Preview(preview(1, "full")); err != nil {
		t.Fatal(err)
	}
	if err := p.Preview(preview(2, "full answer")); err != nil {
		t.Fatal(err)
	}
	if err := p.Preview(preview(1, "stale")); err != nil {
		t.Fatal(err)
	}
	settled, _ := json.Marshal(map[string]any{"history_sequence": 1, "truncated": true, "record": map[string]any{"seq": 1, "type": "update"}})
	if err := p.Settle(context.Background(), settled); err != nil {
		t.Fatal(err)
	}
	snapshot := p.Snapshot()
	if len(snapshot.Records) != 1 || snapshot.Records[0].Preview {
		t.Fatalf("duplicate or pending: %+v", snapshot)
	}
	if err := p.Preview(preview(3, "late")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, p.Snapshot()) {
		t.Fatal("late preview replaced settled evidence")
	}
	snapshot.Records[0].Data[0] = 'x'
	if !json.Valid(p.Snapshot().Records[0].Data) {
		t.Fatal("consumer mutated projection")
	}
	if err := p.Preview([]byte(`{"version":4,"blocks":[],"incomplete":true}`)); err != nil {
		t.Fatal(err)
	}
	if !p.Snapshot().Incomplete {
		t.Fatal("lost incomplete preview status")
	}
}

func TestConsumeRequiresMatchingDurableBoundaryAndFullHistory(t *testing.T) {
	r, path := setupReader(t, meta(), update(1, "agent_message_chunk", "complete answer"), ended(2, "end_turn"))
	p := NewProjection(r, "agent")
	payload, _ := json.Marshal(map[string]any{"turn_id": "turn", "history_sequence": 2, "record": ended(2, "end_turn")})
	sequence := uint64(42)
	m := node.Message{Type: "thread_event", OperationID: "operation", ExecutionID: "agent", Sequence: &sequence, Payload: payload}
	got, err := p.Consume(context.Background(), m)
	if err != nil || got.TurnID != "turn" || got.Turn == nil || got.Turn.Answer != "complete answer" {
		t.Fatalf("observation=%+v err=%v", got, err)
	}
	wrong := m
	wrong.ExecutionID = "other"
	if _, err := p.Consume(context.Background(), wrong); !errors.Is(err, ErrIdentity) {
		t.Fatalf("wrong execution=%v", err)
	}
	wrong = m
	wrong.Sequence = nil
	if _, err := p.Consume(context.Background(), wrong); !errors.Is(err, ErrIdentity) {
		t.Fatalf("unsequenced settle=%v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	got, err = p.Consume(context.Background(), m)
	if !errors.Is(err, ErrUnavailable) || got.Turn != nil {
		t.Fatalf("missing history returned success: %+v %v", got, err)
	}
}

func TestInvalidPreviewDoesNotReplaceLastGoodSnapshot(t *testing.T) {
	p := NewProjection(nil, "agent")
	if err := p.Preview([]byte(`{"version":1,"blocks":[{"history_sequence":1,"record":{"type":"update"}}]}`)); err != nil {
		t.Fatal(err)
	}
	before := p.Snapshot()
	for _, payload := range []string{
		`{"version":2,"blocks":[{"record":{"type":"update"}}]}`,
		`{"version":2,"blocks":[{"history_sequence":1,"record":{}},{"history_sequence":1,"record":{}}]}`,
	} {
		if err := p.Preview([]byte(payload)); !errors.Is(err, ErrDamaged) {
			t.Fatalf("invalid preview=%v", err)
		}
		if !reflect.DeepEqual(before, p.Snapshot()) {
			t.Fatal("invalid preview replaced good state")
		}
	}
}
