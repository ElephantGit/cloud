//go:build linux

package agentruntime

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// Stages of a session's first turn, in order. Each starts when the previous one ends, so together
// they add up to the wait before the page shows the Agent's first output.
const (
	stageRuntime   = "runtime"
	stageWorkspace = "workspace"
	stagePlugin    = "plugin"
	stageAgent     = "agent"
	stageResponse  = "response"
)

// preparation times one session's path to first output and publishes each change. It is
// diagnostic only: a failed publication is dropped and never affects the turn.
type preparation struct {
	mu     sync.Mutex
	emit   Emit
	ref    execution.Ref
	stages []execution.PreparationStage
	done   bool
}

func newPreparation(ref execution.Ref, emit Emit) *preparation {
	return &preparation{emit: emit, ref: ref}
}

// enter ends the running stage at the given time and starts id there.
func (p *preparation) enter(id string, at time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return
	}
	at = p.closeLocked(at)
	p.stages = append(p.stages, execution.PreparationStage{ID: id, StartedAt: at})
	p.publishLocked()
}

// reused marks the running stage as served by work prepared in advance.
func (p *preparation) reused() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.done && len(p.stages) > 0 {
		p.stages[len(p.stages)-1].Reused = true
		p.publishLocked()
	}
}

// finish ends the running stage. Later calls change nothing, so the first output wins.
func (p *preparation) finish(at time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done || len(p.stages) == 0 {
		return
	}
	p.closeLocked(at)
	p.done = true
	p.publishLocked()
}

// current names the running stage, empty once finished or before the first one starts.
func (p *preparation) current() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done || len(p.stages) == 0 {
		return ""
	}
	return p.stages[len(p.stages)-1].ID
}

// closeLocked ends the running stage, keeping stage times ordered when a time comes from Node.
func (p *preparation) closeLocked(at time.Time) time.Time {
	at = at.UTC()
	if len(p.stages) == 0 {
		return at
	}
	last := &p.stages[len(p.stages)-1]
	if at.Before(last.StartedAt) {
		at = last.StartedAt
	}
	last.EndedAt = &at
	return at
}

func (p *preparation) publishLocked() {
	stages := make([]execution.PreparationStage, len(p.stages))
	copy(stages, p.stages)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = p.emit(ctx, execution.Event{Ref: p.ref, OccurredAt: time.Now().UTC(), Kind: execution.EventPreparation, Preparation: &execution.Preparation{Stages: stages}})
}

// observeHistory advances the preparation from Node history: a recorded session means the Agent
// has started, and the first thing the page can show ends the wait.
func (p *preparation) observeHistory(snapshot *execution.HistorySnapshot) {
	if p.current() == "" {
		return
	}
	for _, record := range snapshot.Records {
		// A text chunk's content is an object but a tool call's is a list, so it is read per kind.
		var line struct {
			Type   string
			At     string
			Update struct {
				SessionUpdate string
				Content       json.RawMessage
			}
		}
		if json.Unmarshal(record.Data, &line) != nil {
			continue
		}
		if p.current() == stageAgent && (line.Type == "meta" || line.Update.SessionUpdate == "user_message_chunk") {
			at, err := time.Parse(time.RFC3339Nano, line.At)
			if err != nil {
				at = time.Now()
			}
			p.enter(stageResponse, at)
		}
		if visibleOutput(line.Update.SessionUpdate, line.Update.Content) {
			now := time.Now()
			if p.current() == stageAgent {
				p.enter(stageResponse, now)
			}
			p.finish(now)
			return
		}
	}
}

// visibleOutput reports whether the page renders a history update as Agent output.
func visibleOutput(update string, content json.RawMessage) bool {
	switch update {
	case "tool_call", "tool_call_update", "plan":
		return true
	case "agent_message_chunk", "agent_thought_chunk":
		var text struct{ Text string }
		return json.Unmarshal(content, &text) == nil && strings.TrimSpace(text.Text) != ""
	default:
		return false
	}
}
