package nodehistory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/wanglongan587/cloud/v2-server/internal/node"
)

// Snapshot replaces the complete previous UI projection. Its history positions are not the
// public event cursor. Incomplete describes omitted preview/process detail, never answer quality.
type Snapshot struct {
	Records    []Record `json:"records"`
	Incomplete bool     `json:"incomplete"`
}

// Observation is ready to retain atomically with the source envelope and caller's session/turn
// coordinates. Only after that durable write may the consumer ACK a sequenced Node message.
// A read error must become an explicit failed/incomplete result; never store a preview as success.
type Observation struct {
	Snapshot Snapshot
	TurnID   string
	Turn     *Turn
}

// Consume converts one thread_event/preview into a complete replacement projection and, for
// turnEnded, the full authoritative result. It leaves event cursors, persistence and ACK to its caller.
func (p *Projection) Consume(ctx context.Context, message node.Message) (Observation, error) {
	if message.ExecutionID != p.execution {
		return Observation{}, ErrIdentity
	}
	switch message.Type {
	case "thread_preview":
		if message.Sequence != nil {
			return Observation{}, ErrIdentity
		}
		if err := p.Preview(message.Payload); err != nil {
			return Observation{}, err
		}
		return Observation{Snapshot: p.Snapshot()}, nil
	case "thread_event":
		if message.Sequence == nil || message.OperationID == "" {
			return Observation{}, ErrIdentity
		}
		if err := p.Settle(ctx, message.Payload); err != nil {
			return Observation{}, err
		}
		var event struct {
			TurnID   string `json:"turn_id"`
			Sequence uint32 `json:"history_sequence"`
		}
		if json.Unmarshal(message.Payload, &event) != nil {
			return Observation{}, ErrDamaged
		}
		observation := Observation{Snapshot: p.Snapshot(), TurnID: event.TurnID}
		line, err := decodeLine(p.settled[event.Sequence].Data)
		if err != nil {
			return Observation{}, err
		}
		if line.Type == "turnEnded" {
			if p.reader == nil {
				return Observation{}, ErrUnavailable
			}
			if event.TurnID == "" {
				return Observation{}, ErrIdentity
			}
			turn, err := p.reader.ReadTurn(ctx, p.execution, event.Sequence, line.StopReason)
			if err != nil {
				return Observation{}, err
			}
			observation.Turn = &turn
		}
		return observation, nil
	default:
		return Observation{}, ErrIdentity
	}
}

// Projection has one owner per execution. Settled records take precedence over previews;
// a newer preview replaces the entire pending set, including removing vanished pending items.
type Projection struct {
	reader           *Reader
	execution        string
	settled, pending map[uint32]Record
	version          uint64
	incomplete       bool
}

// NewProjection uses an already-open reader. It does not own or close that reader.
func NewProjection(reader *Reader, execution string) *Projection {
	return &Projection{reader: reader, execution: execution, settled: map[uint32]Record{}, pending: map[uint32]Record{}}
}

// Preview applies a Node thread_preview payload. Replays and out-of-order versions are ignored.
// On a transport reconnect construct a fresh projection from retained records; versions are local
// to one Node incarnation. Never ACK previews: they have no durable execution sequence.
func (p *Projection) Preview(payload json.RawMessage) error {
	var preview struct {
		Version    uint64 `json:"version"`
		Incomplete bool   `json:"incomplete"`
		Blocks     []struct {
			Sequence *uint32         `json:"history_sequence"`
			Record   json.RawMessage `json:"record"`
		} `json:"blocks"`
	}
	if json.Unmarshal(payload, &preview) != nil || preview.Version == 0 {
		return ErrDamaged
	}
	if preview.Version <= p.version {
		return nil
	}
	next := map[uint32]Record{}
	seen := map[uint32]bool{}
	for _, b := range preview.Blocks {
		if b.Sequence == nil || !json.Valid(b.Record) || len(bytes.TrimSpace(b.Record)) == 0 || bytes.TrimSpace(b.Record)[0] != '{' {
			return ErrDamaged
		}
		if seen[*b.Sequence] {
			return ErrDamaged
		}
		seen[*b.Sequence] = true
		if _, settled := p.settled[*b.Sequence]; !settled {
			next[*b.Sequence] = Record{*b.Sequence, bytes.Clone(b.Record), true}
		}
	}
	p.pending = next
	p.version = preview.Version
	p.incomplete = preview.Incomplete
	return nil
}

// Settle upserts a thread_event. A truncated record must be recovered in full before it can
// replace a preview. Callers persist the returned projection/evidence before ACKing Node.
func (p *Projection) Settle(ctx context.Context, payload json.RawMessage) error {
	var event struct {
		Sequence  *uint32         `json:"history_sequence"`
		Truncated bool            `json:"truncated"`
		Record    json.RawMessage `json:"record"`
	}
	if json.Unmarshal(payload, &event) != nil || event.Sequence == nil {
		return ErrDamaged
	}
	record := Record{Sequence: *event.Sequence, Data: bytes.Clone(event.Record)}
	if event.Truncated {
		if p.reader == nil {
			return ErrUnavailable
		}
		var err error
		record, err = p.reader.Record(ctx, p.execution, *event.Sequence)
		if err != nil {
			return err
		}
	}
	l, err := decodeLine(record.Data)
	if err != nil {
		return err
	}
	if *l.Sequence != *event.Sequence {
		return ErrIdentity
	}
	p.settled[record.Sequence] = record
	delete(p.pending, record.Sequence)
	if l.Type == "turnEnded" {
		clear(p.pending)
		p.incomplete = false
	}
	return nil
}

// Snapshot returns independent copies, so UI/SQL consumers cannot mutate future projections.
func (p *Projection) Snapshot() Snapshot {
	all := make(map[uint32]Record, len(p.settled)+len(p.pending))
	for k, v := range p.pending {
		all[k] = v
	}
	for k, v := range p.settled {
		all[k] = v
	}
	return Snapshot{Records: ordered(all), Incomplete: p.incomplete}
}

// FailureCode maps a failed authoritative read to a bounded public reason. A caller must store
// an incomplete/failed result rather than treating the partial preview as a successful answer.
func FailureCode(err error) string {
	for _, known := range []error{ErrUnavailable, ErrDamaged, ErrIncomplete, ErrLimit, ErrIdentity} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return "history_read_failed"
}
