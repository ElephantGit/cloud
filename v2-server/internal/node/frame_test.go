package node

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

func TestFramePreservesSequenceAndOpaquePayload(t *testing.T) {
	sequence := uint64(0)
	m := Message{Type: "thread_event", Version: 1, OperationID: "op", ExecutionID: "run", Sequence: &sequence, Payload: json.RawMessage(`{"record":{"_meta":{"opencode/child-session":"child"}},"text":"你好"}`)}
	frame, err := encode(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readFrame(bytes.NewReader(frame))
	if err != nil || got.Type != m.Type || got.Sequence == nil || *got.Sequence != 0 || !bytes.Equal(got.Payload, m.Payload) || got.ExecutionID != m.ExecutionID || got.OperationID != m.OperationID {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
}

func TestFrameRejectsCorruptionBeforeAllocation(t *testing.T) {
	valid, _ := encode(Message{Type: "heartbeat", Version: 1, Payload: json.RawMessage(`{}`)})
	oversize := make([]byte, 4)
	binary.BigEndian.PutUint32(oversize, maxFrameLength+1)
	for name, input := range map[string][]byte{
		"partial header": {0, 0}, "zero": {0, 0, 0, 0}, "oversize": oversize,
		"tag": {0, 0, 0, 1, 2}, "missing payload": valid[:len(valid)-1], "invalid json": {0, 0, 0, 2, 1, '{'},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readFrame(bytes.NewReader(input)); err == nil {
				t.Fatal("accepted corrupt frame")
			}
		})
	}
	if _, err := readFrame(bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
		t.Fatalf("clean EOF = %v", err)
	}
}

type shortWriter struct{ bytes.Buffer }

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > 2 {
		p = p[:2]
	}
	return w.Buffer.Write(p)
}

func TestWriteAllHandlesShortWrites(t *testing.T) {
	w := &shortWriter{}
	if err := writeAll(w, []byte("complete frame")); err != nil || w.String() != "complete frame" {
		t.Fatalf("write = %q, %v", w.String(), err)
	}
}
