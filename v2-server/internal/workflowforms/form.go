// Package workflowforms validates answers against a frozen, server-owned form.
// It performs no persistence or task creation; those effects belong to the host.
package workflowforms

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxRows bounds one review and its eventual task batch.
const MaxRows = 100

var memberID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Field describes an author-configured column. Source values remain server-owned.
type Field struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

// Finding is immutable evidence captured when the form opens. Key identifies the
// check and object across days, not a model-generated title or a report date.
type Finding struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
	Severity    string `json:"severity"`
	Module      string `json:"module"`
}

// Assignee is a typed workspace reference; the host checks its current validity.
type Assignee struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Answer contains only editable values, never evidence or a caller-selected project.
type Answer struct {
	Key      string            `json:"key"`
	Decision string            `json:"decision"`
	Assignee *Assignee         `json:"assignee,omitempty"`
	Values   map[string]string `json:"values"`
}

// FieldError identifies the row and control that requires correction.
type FieldError struct{ Key, Field, Reason string }

func (e *FieldError) Error() string { return fmt.Sprintf("%s.%s: %s", e.Key, e.Field, e.Reason) }

func invalid(key, field, reason string) error { return &FieldError{key, field, reason} }

// ValidateDefinition rejects ambiguous identities and unsupported controls before
// any form can become a durable waiting point.
func ValidateDefinition(fields []Field, findings []Finding) error {
	if len(findings) > MaxRows {
		return invalid("", "findings", "too many rows")
	}
	names := map[string]bool{}
	for _, f := range fields {
		if strings.TrimSpace(f.Name) == "" || names[f.Name] || f.Name == "decision" || f.Name == "assignee" || f.Name == "key" {
			return invalid("", f.Name, "invalid or duplicate field name")
		}
		names[f.Name] = true
		if strings.TrimSpace(f.Label) == "" {
			return invalid("", f.Name, "label required")
		}
		switch f.Type {
		case "readonly", "text", "textarea":
			if len(f.Options) != 0 {
				return invalid("", f.Name, "options require a select field")
			}
		case "select":
			if len(f.Options) == 0 {
				return invalid("", f.Name, "options required")
			}
			seen := map[string]bool{}
			for _, option := range f.Options {
				if strings.TrimSpace(option) == "" || seen[option] {
					return invalid("", f.Name, "invalid or duplicate option")
				}
				seen[option] = true
			}
		default:
			return invalid("", f.Name, "unsupported field type")
		}
	}
	keys := map[string]bool{}
	for _, f := range findings {
		if strings.TrimSpace(f.Key) == "" || f.Key != strings.TrimSpace(f.Key) || utf8.RuneCountInString(f.Key) > 200 || keys[f.Key] {
			return invalid(f.Key, "key", "invalid or duplicate finding key")
		}
		keys[f.Key] = true
		if strings.TrimSpace(f.Title) == "" || utf8.RuneCountInString(f.Title) > 200 {
			return invalid(f.Key, "title", "title must contain 1 to 200 characters")
		}
	}
	return nil
}

// ValidateAnswers checks a draft or final submission against its frozen rows.
// An existing-task lookup is supplied by the host under its transaction: only
// those rows may omit an assignee because the existing task keeps its owner.
func ValidateAnswers(fields []Field, findings []Finding, answers []Answer, final bool, existing map[string]bool) error {
	if err := ValidateDefinition(fields, findings); err != nil {
		return err
	}
	rows := map[string]bool{}
	for _, row := range findings {
		rows[row.Key] = true
	}
	columns := map[string]Field{}
	for _, f := range fields {
		columns[f.Name] = f
	}
	seen := map[string]bool{}
	for _, a := range answers {
		if !rows[a.Key] || seen[a.Key] {
			return invalid(a.Key, "key", "unknown or duplicate row")
		}
		seen[a.Key] = true
		switch a.Decision {
		case "", "pending":
			if final {
				return invalid(a.Key, "decision", "confirmation required")
			}
		case "confirmed", "rejected":
		default:
			return invalid(a.Key, "decision", "unknown decision")
		}
		if a.Assignee != nil {
			if !memberID.MatchString(a.Assignee.ID) {
				return invalid(a.Key, "assignee", "identifier required")
			}
			switch a.Assignee.Type {
			case "human", "agent", "squad":
			default:
				return invalid(a.Key, "assignee", "unknown assignee type")
			}
		}
		if final && a.Decision == "confirmed" && a.Assignee == nil && !existing[a.Key] {
			return invalid(a.Key, "assignee", "assignee required")
		}
		for name, value := range a.Values {
			f, ok := columns[name]
			if !ok || f.Type == "readonly" {
				return invalid(a.Key, name, "field is not editable")
			}
			if utf8.RuneCountInString(value) > 4000 {
				return invalid(a.Key, name, "value too long")
			}
			if f.Type == "select" && value != "" {
				found := false
				for _, option := range f.Options {
					found = found || option == value
				}
				if !found {
					return invalid(a.Key, name, "unknown option")
				}
			}
		}
		if final && a.Decision == "confirmed" {
			for _, f := range fields {
				if f.Required && f.Type != "readonly" && strings.TrimSpace(a.Values[f.Name]) == "" {
					return invalid(a.Key, f.Name, "value required")
				}
			}
		}
	}
	if final && len(seen) != len(rows) {
		return invalid("", "answers", "every row must be confirmed")
	}
	return nil
}
