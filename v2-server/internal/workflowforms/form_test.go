package workflowforms

import "testing"

func TestFrozenReviewBoundary(t *testing.T) {
	fields := []Field{{Name: "severity", Label: "严重程度", Type: "select", Required: true, Options: []string{"high", "low"}}, {Name: "evidence", Label: "证据", Type: "readonly"}}
	rows := []Finding{{Key: "disk:root", Title: "磁盘占用过高"}}
	valid := func() []Answer {
		return []Answer{{Key: "disk:root", Decision: "confirmed", Assignee: &Assignee{Type: "human", ID: "00000000-0000-0000-0000-000000000001"}, Values: map[string]string{"severity": "high"}}}
	}
	for _, tc := range []struct {
		name      string
		edit      func([]Answer) []Answer
		final     bool
		wantError bool
	}{
		{"valid", func(a []Answer) []Answer { return a }, true, false},
		{"draft", func(a []Answer) []Answer { return nil }, false, false},
		{"missing row", func(a []Answer) []Answer { return nil }, true, true},
		{"duplicate row", func(a []Answer) []Answer { return append(a, a[0]) }, true, true},
		{"foreign row", func(a []Answer) []Answer { a[0].Key = "other"; return a }, true, true},
		{"evidence tampering", func(a []Answer) []Answer { a[0].Values["evidence"] = "changed"; return a }, true, true},
		{"unknown column", func(a []Answer) []Answer { a[0].Values["projectId"] = "other"; return a }, true, true},
		{"unknown option", func(a []Answer) []Answer { a[0].Values["severity"] = "critical"; return a }, true, true},
		{"missing assignee", func(a []Answer) []Answer { a[0].Assignee = nil; return a }, true, true},
		{"pending", func(a []Answer) []Answer { a[0].Decision = "pending"; return a }, true, true},
		{"rejected", func(a []Answer) []Answer {
			a[0].Decision = "rejected"
			a[0].Assignee = nil
			a[0].Values = nil
			return a
		}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAnswers(fields, rows, tc.edit(valid()), tc.final, nil)
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v", err)
			}
		})
	}
	answers := valid()
	answers[0].Assignee = nil
	if err := ValidateAnswers(fields, rows, answers, true, map[string]bool{"disk:root": true}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAnswers(fields, nil, nil, true, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRejectAmbiguousEvidence(t *testing.T) {
	for _, rows := range [][]Finding{
		{{Key: "same", Title: "first"}, {Key: "same", Title: "second"}},
		{{Key: "", Title: "missing key"}},
		{{Key: "disk", Title: ""}},
		make([]Finding, MaxRows+1),
	} {
		if err := ValidateDefinition(nil, rows); err == nil {
			t.Fatal("accepted ambiguous evidence")
		}
	}
	if err := ValidateDefinition([]Field{{Name: "decision", Label: "Override", Type: "text"}}, nil); err == nil {
		t.Fatal("accepted reserved column")
	}
}
