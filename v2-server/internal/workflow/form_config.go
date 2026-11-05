package workflow

import (
	"encoding/json"
	"github.com/wanglongan587/cloud/v2-server/internal/workflowforms"
)

// FormConfig returns the frozen authoring configuration without exposing the
// graph's internal mutable map to callers.
func (n Node) FormConfig() json.RawMessage {
	raw, _ := json.Marshal(n.data["formConfig"])
	return raw
}

// TaskFormNode names the upstream form whose accepted answers createTasks uses.
func (n Node) TaskFormNode() string {
	c, _ := n.data["createTasksConfig"].(object)
	id, _ := c["formNodeId"].(string)
	return id
}

// validateFormPlacement keeps durable review recovery limited to a single root
// form with a deterministic tail. No Agent or container can run after review.
func validateFormPlacement(d *document, scopes map[string]*scope) error {
	root := scopes[""]
	var form *node
	for _, n := range d.nodes {
		if !d.active[n.id] || (n.kind != "form" && n.kind != "createTasks") {
			continue
		}
		if n.owner != "" {
			return invalid("invalid_region", n.id, "parentId", "review nodes must be in the root graph")
		}
		if n.kind == "form" {
			if form != nil {
				return invalid("invalid_form", n.id, "data.kind", "only one form is supported")
			}
			form = n
		}
	}
	for _, n := range root.nodes {
		if n.owner != "" {
			continue
		}
		if form != nil && n.id != form.id && !root.ancestors(form.id)[n.id] && !root.ancestors(n.id)[form.id] {
			return invalid("invalid_form", n.id, "edges", "all root work must precede or follow the form")
		}
		if n.kind == "createTasks" {
			id := rootNodeView(n).TaskFormNode()
			if form == nil || id != form.id || !root.ancestors(n.id)[form.id] {
				return invalid("invalid_form", n.id, "createTasksConfig.formNodeId", "task creation requires its upstream form")
			}
		}
		if form != nil && root.ancestors(n.id)[form.id] && n.kind != "createTasks" && n.kind != "output" {
			return invalid("invalid_form", n.id, "data.kind", "form successors must create tasks or output results")
		}
	}
	return nil
}

func rootNodeView(n *node) Node { return Node{data: n.data} }

func validateFormNode(n *node) error {
	if n.kind == "form" {
		raw, _ := json.Marshal(n.data["formConfig"])
		c, err := workflowforms.DecodeConfig(raw)
		if err != nil {
			return invalid("invalid_form", n.id, "formConfig", err.Error())
		}
		n.selectors = append(n.selectors, c.ReportSelector, c.ItemsSelector)
		n.bindings = append(n.bindings, c.ReportSelector, c.ItemsSelector)
	}
	if n.kind == "createTasks" {
		r := newReader(n.data, "invalid_form", n.id, "data").child("createTasksConfig")
		r.only("formNodeId")
		if r.requiredString("formNodeId") == "" {
			r.fail("formNodeId", "upstream form required")
		}
		return *r.err
	}
	return nil
}
