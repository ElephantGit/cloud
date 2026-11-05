package workflowforms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Config is frozen alongside the workflow snapshot. Selectors reference upstream
// variables; a reviewer is a workspace member ID, or empty for the run starter.
type Config struct {
	Title          string   `json:"title"`
	ReportSelector []string `json:"reportSelector"`
	ItemsSelector  []string `json:"itemsSelector"`
	ReviewerID     string   `json:"reviewerId,omitempty"`
	SubmitLabel    string   `json:"submitLabel"`
	Fields         []Field  `json:"fields"`
}

// DecodeConfig rejects misspelled settings instead of silently changing an
// author's intended review boundary. It also bounds rendered form dimensions.
func DecodeConfig(raw []byte) (Config, error) {
	var c Config
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return c, fmt.Errorf("expected one configuration object")
	}
	if strings.TrimSpace(c.Title) == "" || strings.TrimSpace(c.SubmitLabel) == "" {
		return c, fmt.Errorf("title and submitLabel are required")
	}
	for _, s := range [][]string{c.ReportSelector, c.ItemsSelector} {
		if len(s) < 2 {
			return c, fmt.Errorf("selector requires a node and variable")
		}
		for _, p := range s {
			if strings.TrimSpace(p) == "" {
				return c, fmt.Errorf("empty selector segment")
			}
		}
	}
	if c.ReviewerID != "" && !memberID.MatchString(c.ReviewerID) {
		return c, fmt.Errorf("invalid reviewer ID")
	}
	if len(c.Fields) > 20 {
		return c, fmt.Errorf("too many form columns")
	}
	return c, ValidateDefinition(c.Fields, nil)
}

// DecodeFindings admits only complete structured evidence, never a JSON string
// containing Markdown or silently ignored metadata. The host supplies the value
// resolved from the frozen graph's variable pool.
func DecodeFindings(value any) ([]Finding, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(raw, []byte("null")) {
		return nil, fmt.Errorf("findings must be an array")
	}
	var rows []Finding
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&rows); err != nil {
		return nil, err
	}
	if err = ValidateDefinition(nil, rows); err != nil {
		return nil, err
	}
	return rows, nil
}
