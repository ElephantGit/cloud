// Package workflow executes frozen workflow graphs using the existing Cloud semantics.
package workflow

import "encoding/json"

// Object is a JSON object at the workflow document boundary.
type Object map[string]any

func (o Object) S(k string) string { v, _ := o[k].(string); return v }
func (o Object) B(k string) bool   { v, _ := o[k].(bool); return v }
func (o Object) N(k string) int64 {
	switch v := o[k].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	}
	return 0
}
func (o Object) O(k string) Object {
	switch v := o[k].(type) {
	case map[string]any:
		return Object(v)
	case Object:
		return v
	}
	return Object{}
}

const fieldKeySnapshotID = "snapshotId"

func canonicalJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func jsonText(v any) string      { return canonicalJSON(v) }

// Validate returns every executable graph violation in document order.
// Drafts remain editable without this check; publication and run admission require it.
func Validate(graph Object) []Object {
	problems := validateWorkflowGraph(graph)
	out := make([]Object, 0, len(problems))
	for _, problem := range problems {
		out = append(out, problem.Object())
	}
	return out
}

// Normalize canonicalizes file globals after successful validation.
func Normalize(graph Object) Object { return normalizeWorkflowGlobals(graph) }
