package workflowforms

import (
	"strings"
	"testing"
)

func TestConfigBoundary(t *testing.T) {
	valid := `{"title":"确认","submitLabel":"提交","reportSelector":["report","structured_output","reportMarkdown"],"itemsSelector":["report","structured_output","findings"],"fields":[]}`
	if _, err := DecodeConfig([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{valid + ` {}`, strings.Replace(valid, `"fields":[]`, `"fileds":[]`, 1), strings.Replace(valid, `"itemsSelector":["report","structured_output","findings"]`, `"itemsSelector":["report",""]`, 1), strings.Replace(valid, `"title":"确认"`, `"title":" "`, 1)} {
		if _, err := DecodeConfig([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestFindingsAreStructuredEvidence(t *testing.T) {
	for _, value := range []any{nil, `[]`, map[string]any{"key": "disk"}, []any{map[string]any{"key": "disk", "title": "磁盘", "projectId": "foreign"}}} {
		if _, err := DecodeFindings(value); err == nil {
			t.Fatalf("accepted %#v", value)
		}
	}
	rows, err := DecodeFindings([]any{map[string]any{"key": "disk", "title": "磁盘"}})
	if err != nil || len(rows) != 1 {
		t.Fatalf("%v %v", rows, err)
	}
	if rows, err := DecodeFindings([]any{}); err != nil || rows == nil {
		t.Fatalf("empty evidence: %v %v", rows, err)
	}
}
