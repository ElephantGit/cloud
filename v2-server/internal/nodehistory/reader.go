// Package nodehistory reads the fixed local Node's complete history and projects replaceable
// snapshots. It never modifies Node state or interprets an IPC preview as a complete answer.
package nodehistory

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Stable, sanitized errors describe why a result cannot be claimed complete.
var (
	ErrUnavailable = errors.New("history_unavailable")
	ErrDamaged     = errors.New("history_damaged")
	ErrIncomplete  = errors.New("history_incomplete")
	ErrLimit       = errors.New("history_limit_exceeded")
	ErrIdentity    = errors.New("history_identity_mismatch")
)

const maxFileBytes = 64 * 1024 * 1024
const maxLineBytes = 8 * 1024 * 1024
const maxAnswerBytes = 16 * 1024 * 1024

// Record is one settled HistoryLine, or a pending preview record with the same stable position.
// Data is an opaque Node history object, not an ACP delta. It must replace the previous position.
type Record struct {
	Sequence uint32          `json:"sequence"`
	Data     json.RawMessage `json:"data"`
	Preview  bool            `json:"preview,omitempty"`
}

// Turn is derived from full, validated local history ending at an observed Node turn boundary.
// Its Answer excludes thought, tool output and valid child-session projections. Separate settled
// assistant messages are joined with a newline, matching desktop's final_assistant_message.
type Turn struct {
	EndSequence uint32   `json:"endSequence"`
	StopReason  string   `json:"stopReason"`
	Answer      string   `json:"answer"`
	Records     []Record `json:"records"`
}

// Reader is anchored in a trusted Workspace's Node home, outside the checkout. Execution IDs
// come from admission, never an arbitrary user path. The fixed layout is desktop cab59a3c.
type Reader struct{ root *os.Root }

// Open pins the Node home directory. OpenRoot prevents symlink/path escapes while reading.
func Open(nodeHome string) (*Reader, error) {
	if !filepath.IsAbs(nodeHome) {
		return nil, ErrIdentity
	}
	root, err := os.OpenRoot(nodeHome)
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Reader{root: root}, nil
}

// Close releases the directory handle; callers join reads before closing the reader.
func (r *Reader) Close() error { return r.root.Close() }

func historyPath(execution string) (string, error) {
	if len(execution) == 0 || len(execution) > 128 {
		return "", ErrIdentity
	}
	for _, c := range execution {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return "", ErrIdentity
		}
	}
	first := execution[:min(2, len(execution))]
	rest := execution[len(first):]
	second := rest[:min(2, len(rest))]
	return filepath.Join("sessions", first, second, execution+".jsonl"), nil
}

type line struct {
	At            string          `json:"at"`
	Sequence      *uint32         `json:"seq"`
	Type          string          `json:"type"`
	SessionID     string          `json:"sessionId"`
	SchemaVersion int             `json:"schemaVersion"`
	StopReason    string          `json:"stop_reason"`
	Update        json.RawMessage `json:"update"`
}

func decodeLine(data []byte) (line, error) {
	var l line
	if !utf8.Valid(data) || json.Unmarshal(data, &l) != nil || l.Sequence == nil || l.Type == "" {
		return l, ErrDamaged
	}
	if _, err := time.Parse(time.RFC3339Nano, l.At); err != nil {
		return l, ErrDamaged
	}
	if l.Type == "gap" {
		return l, ErrIncomplete
	}
	switch l.Type {
	case "meta", "agentSwitched", "handoffDelivered":
	case "update":
		var update struct {
			Kind string `json:"sessionUpdate"`
		}
		if json.Unmarshal(l.Update, &update) != nil || update.Kind == "" {
			return l, ErrDamaged
		}
	case "turnEnded":
		if l.StopReason == "" {
			return l, ErrDamaged
		}
	default:
		return l, ErrDamaged
	}
	return l, nil
}

// read resolves duplicate positions by last write. Node appends settled records out of order.
// A partial tail is ignored only while fetching a previously observed completed boundary; it
// cannot invent that boundary. Complete corrupt lines always fail closed.
func (r *Reader) read(ctx context.Context, execution string) (map[uint32]Record, error) {
	path, err := historyPath(execution)
	if err != nil {
		return nil, err
	}
	f, err := r.root.Open(path)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		return nil, ErrUnavailable
	}
	if stat.Size() > maxFileBytes {
		return nil, ErrLimit
	}
	scanner := bufio.NewScanner(io.NewSectionReader(f, 0, stat.Size()))
	scanner.Buffer(make([]byte, 64*1024), maxLineBytes+1)
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if end := bytes.IndexByte(data, '\n'); end >= 0 {
			return end + 1, data[:end], nil
		}
		if atEOF {
			return len(data), nil, nil
		}
		return 0, nil, nil
	})
	records := make(map[uint32]Record)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data := scanner.Bytes()
		if len(bytes.TrimSpace(data)) == 0 {
			continue
		}
		l, err := decodeLine(data)
		if err != nil {
			return nil, err
		}
		records[*l.Sequence] = Record{Sequence: *l.Sequence, Data: bytes.Clone(data)}
	}
	if scanner.Err() != nil {
		return nil, ErrLimit
	}
	meta, ok := records[0]
	if !ok {
		return nil, ErrIncomplete
	}
	l, err := decodeLine(meta.Data)
	if err != nil || l.Type != "meta" || l.SessionID != execution || l.SchemaVersion < 1 || l.SchemaVersion > 2 {
		return nil, ErrIdentity
	}
	return records, nil
}

// Record retrieves the full local counterpart of a truncated IPC record. No content is taken
// from visible_text; that projection has its own smaller truncation boundary.
func (r *Reader) Record(ctx context.Context, execution string, sequence uint32) (Record, error) {
	records, err := r.read(ctx, execution)
	if err != nil {
		return Record{}, err
	}
	record, ok := records[sequence]
	if !ok {
		return Record{}, ErrIncomplete
	}
	return record, nil
}

// ReadTurn requires a specific turnEnded history position and the matching wire stop reason.
// It never returns the preceding turn when the requested turn is missing or still running.
func (r *Reader) ReadTurn(ctx context.Context, execution string, endSequence uint32, stopReason string) (Turn, error) {
	records, err := r.read(ctx, execution)
	if err != nil {
		return Turn{}, err
	}
	if endSequence == 0 || stopReason == "" {
		return Turn{}, ErrIncomplete
	}
	end, ok := records[endSequence]
	if !ok {
		return Turn{}, ErrIncomplete
	}
	l, err := decodeLine(end.Data)
	if err != nil || l.Type != "turnEnded" || l.StopReason != stopReason {
		return Turn{}, ErrIncomplete
	}
	// Validate a complete prefix rather than silently losing a missing record. Use the map
	// size to reject adversarial sequence values without iterating billions of positions.
	if uint64(endSequence)+1 > uint64(len(records)) {
		return Turn{}, ErrIncomplete
	}
	previous := uint32(0)
	for seq := uint32(0); seq < endSequence; seq++ {
		record, ok := records[seq]
		if !ok {
			return Turn{}, ErrIncomplete
		}
		line, err := decodeLine(record.Data)
		if err != nil {
			return Turn{}, err
		}
		if line.Type == "turnEnded" {
			previous = seq
		}
	}
	turn := Turn{EndSequence: endSequence, StopReason: stopReason, Records: []Record{}}
	var parts []string
	size := 0
	for seq := previous + 1; seq <= endSequence; seq++ {
		record := records[seq]
		l, err := decodeLine(record.Data)
		if err != nil {
			return Turn{}, err
		}
		turn.Records = append(turn.Records, record)
		if l.Type != "update" {
			continue
		}
		text, err := assistantText(l.Update)
		if err != nil {
			return Turn{}, err
		}
		if text == nil {
			continue
		}
		size += len(*text)
		if len(parts) > 0 {
			size++
		}
		if size > maxAnswerBytes {
			return Turn{}, ErrLimit
		}
		parts = append(parts, *text)
	}
	turn.Answer = strings.Join(parts, "\n")
	return turn, nil
}

func assistantText(raw json.RawMessage) (*string, error) {
	var update struct {
		Kind    string                     `json:"sessionUpdate"`
		Content json.RawMessage            `json:"content"`
		Meta    map[string]json.RawMessage `json:"_meta"`
	}
	if json.Unmarshal(raw, &update) != nil || update.Kind == "" {
		return nil, ErrDamaged
	}
	if update.Kind != "agent_message_chunk" {
		return nil, nil
	}
	var content struct {
		Type string  `json:"type"`
		Text *string `json:"text"`
	}
	if json.Unmarshal(update.Content, &content) != nil {
		return nil, ErrDamaged
	}
	if content.Type != "text" {
		return nil, nil
	}
	if content.Text == nil {
		return nil, ErrDamaged
	}
	var child struct {
		ID       string  `json:"id"`
		ParentID string  `json:"parentID"`
		Depth    float64 `json:"depth"`
	}
	if json.Unmarshal(update.Meta["opencode/child-session"], &child) == nil && strings.TrimSpace(child.ID) != "" && strings.TrimSpace(child.ParentID) != "" && child.Depth >= 1 && child.Depth <= 9007199254740991 && math.Trunc(child.Depth) == child.Depth {
		return nil, nil
	}
	return content.Text, nil
}

func ordered(records map[uint32]Record) []Record {
	out := make([]Record, 0, len(records))
	for _, r := range records {
		r.Data = bytes.Clone(r.Data)
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}
