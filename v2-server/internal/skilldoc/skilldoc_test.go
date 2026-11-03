package skilldoc

import (
	"strings"
	"testing"
)

func TestParseReadsFrontMatterAndBody(t *testing.T) {
	got, err := Parse([]byte("---\nname: review\ndescription: 检查改动\n---\n\n# Review\n\nRead the diff.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "review" || got.Description != "检查改动" || got.Content != "# Review\n\nRead the diff.\n" {
		t.Fatalf("parsed document = %+v", got)
	}
}

func TestParseRejectsUnsafeOrIncompleteDocuments(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		code string
	}{
		{"missing front matter", "# Review", "skill_front_matter_missing"},
		{"invalid yaml", "---\nname: [\n---\nbody", "skill_front_matter_invalid"},
		{"missing name", "---\ndescription: review\n---\nbody", "skill_name_missing"},
		{"dot name", "---\nname: .review\ndescription: review\n---\nbody", "skill_name_invalid"},
		{"path name", "---\nname: a/b\ndescription: review\n---\nbody", "skill_name_invalid"},
		{"missing description", "---\nname: review\n---\nbody", "skill_description_missing"},
		{"nul content", "---\nname: review\ndescription: review\n---\na\x00b", "skill_content_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse([]byte(test.raw))
			if code := Code(err); code != test.code {
				t.Fatalf("code = %q, want %q (%v)", code, test.code, err)
			}
		})
	}
}

func TestParseEnforcesManifestSize(t *testing.T) {
	raw := "---\nname: review\ndescription: review\n---\n" + strings.Repeat("x", MaxManifestBytes)
	_, err := Parse([]byte(raw))
	if code := Code(err); code != "skill_file_too_large" {
		t.Fatalf("code = %q, want skill_file_too_large", code)
	}
}

func TestRenderProducesDiscoverableSkillFile(t *testing.T) {
	raw := Render(Document{Name: "review", Description: `检查 "改动"`, Content: "# Review"})
	if !strings.HasPrefix(raw, "---\nname: \"review\"\ndescription: \"检查 \\\"改动\\\"\"\n---\n\n# Review\n") {
		t.Fatalf("rendered SKILL.md = %q", raw)
	}
	parsed, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Name != "review" || parsed.Description != `检查 "改动"` || parsed.Content != "# Review\n" {
		t.Fatalf("round trip = %+v", parsed)
	}
}
