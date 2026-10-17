package core

import "testing"

// TestPostgresUnsupportedText pins the raw-text rules that keep one document from aborting the
// result transaction. Go's decoder accepts most of these inputs — it even replaces unpaired
// surrogate escapes — so the document's raw bytes are the only place the distinction survives.
func TestPostgresUnsupportedText(t *testing.T) {
	for _, tc := range []struct {
		name        string
		content     string
		reason      string
		unsupported bool
	}{
		{name: "plain text", content: `{"a":"plain"}`},
		{name: "escaped backslash", content: "{\"a\":\"\\\\u0000\"}"},
		{name: "nul escape", content: "{\"a\":\"\\u0000\"}", reason: "unsupported_unicode_escape", unsupported: true},
		{name: "lone low surrogate", content: "{\"a\":\"\\udc00\"}", reason: "unsupported_unicode_escape", unsupported: true},
		{name: "lone high surrogate", content: "{\"a\":\"\\ud800\"}", reason: "unsupported_unicode_escape", unsupported: true},
		{name: "valid surrogate pair", content: "{\"a\":\"\\ud83d\\ude00\"}"},
		{name: "invalid utf-8", content: "{\"a\":\"" + string([]byte{0xff}) + "\"}", reason: "invalid_utf8", unsupported: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason, unsupported := postgresUnsupportedText([]byte(tc.content))
			if unsupported != tc.unsupported || reason != tc.reason {
				t.Fatalf("postgresUnsupportedText(%q) = %q, %v; want %q, %v",
					tc.content, reason, unsupported, tc.reason, tc.unsupported)
			}
		})
	}
}
