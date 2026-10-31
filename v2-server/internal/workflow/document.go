package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
)

// Document and Snapshot use Cloud's existing public resource shapes. Keep the graph
// as JSON: editor annotations and large JSON numbers must survive a save unchanged.
type Document struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenantId"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Graph       json.RawMessage `json:"graph"`
	Version     int64           `json:"version"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	DeletedAt   *time.Time      `json:"deletedAt"`
}

type Snapshot struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenantId"`
	WorkflowID string          `json:"workflowId"`
	Version    int64           `json:"version"`
	Name       string          `json:"name"`
	Graph      json.RawMessage `json:"graph"`
	CreatedAt  time.Time       `json:"createdAt"`
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor"`
}

type CreateDocument struct {
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Graph       json.RawMessage `json:"graph,omitempty"`
}

type UpdateDocument struct {
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Graph       json.RawMessage `json:"graph,omitempty"`
	Version     *int64          `json:"version,omitempty"`
}

type PublishDocument struct {
	Name *string `json:"name,omitempty"`
}

type DocumentVersion struct {
	Version *int64 `json:"version,omitempty"`
}

var ErrVersionRequired = errors.New("workflow version required")

// GraphError retains the same node/field problems as the original publish gate.
type GraphError struct{ Problems []Object }

func (*GraphError) Error() string { return "workflow graph invalid" }

const MaxDocumentBytes = 8 << 20

// FreezeGraph reuses the executable graph gate and file-global normalization.
// The validation copy uses the original engine's numeric representation; the
// stored copy retains JSON numbers exactly, including opaque editor annotations.
func FreezeGraph(raw json.RawMessage) (json.RawMessage, error) {
	var graph Object
	if len(raw) > workflowStateLimit-workflowFailureReserve || json.Unmarshal(raw, &graph) != nil || graph == nil {
		return nil, &GraphError{Problems: []Object{{"code": "graph_invalid"}}}
	}
	if nodes, _ := graph["nodes"].([]any); len(nodes) > 256 {
		return nil, &GraphError{Problems: []Object{{"code": "graph_execution_limit", "field": "nodes"}}}
	}
	if edges, _ := graph["edges"].([]any); len(edges) > 2048 {
		return nil, &GraphError{Problems: []Object{{"code": "graph_execution_limit", "field": "edges"}}}
	}
	if problems := Validate(graph); len(problems) != 0 {
		return nil, &GraphError{Problems: problems}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var preserved Object
	if err := decoder.Decode(&preserved); err != nil {
		return nil, err
	}
	return json.Marshal(Normalize(preserved))
}
