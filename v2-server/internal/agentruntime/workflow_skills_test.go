//go:build linux

package agentruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

func TestWorkflowSkillsReplacePreviousNodeAndPreserveProjectFiles(t *testing.T) {
	p := &projectRuntime{root: t.TempDir()}
	checkout := t.TempDir()
	owned := filepath.Join(checkout, ".opencode/skills/project/SKILL.md")
	if err := os.MkdirAll(filepath.Dir(owned), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(owned, []byte("project-owned"), 0600); err != nil {
		t.Fatal(err)
	}
	a := execution.Skills{Documents: []execution.SkillDocument{{Name: "first", Description: "first skill", Content: "FIRST"}}}
	b := execution.Skills{Documents: []execution.SkillDocument{{Name: "second", Description: "second skill", Content: "SECOND"}}}
	for _, selection := range []execution.Skills{a, b, {}} {
		if err := p.applyWorkflowSkills(checkout, "run", selection); err != nil {
			t.Fatal(err)
		}
		for _, base := range []string{".opencode", ".claude"} {
			for _, name := range []string{"first", "second"} {
				data, err := os.ReadFile(filepath.Join(checkout, base, "skills", name, "SKILL.md"))
				want := len(selection.Documents) > 0 && selection.Documents[0].Name == name
				if want && (err != nil || !strings.Contains(string(data), selection.Documents[0].Content)) {
					t.Fatalf("missing %s: %s %v", name, data, err)
				}
				if !want && !os.IsNotExist(err) {
					t.Fatalf("previous node skill remains: %s", name)
				}
			}
		}
	}
	if data, err := os.ReadFile(owned); err != nil || string(data) != "project-owned" {
		t.Fatalf("changed project skill: %s %v", data, err)
	}
}

func TestWorkflowSkillsRefuseMissingPackageAndEditedDocument(t *testing.T) {
	p := &projectRuntime{root: t.TempDir()}
	checkout := t.TempDir()
	if err := p.applyWorkflowSkills(checkout, "run", execution.Skills{Plugins: []execution.SkillPlugin{{ID: "official/missing", Version: "1.0.0"}}}); err == nil {
		t.Fatal("missing package accepted")
	}
	selection := execution.Skills{Documents: []execution.SkillDocument{{Name: "review", Description: "review", Content: "original"}}}
	if err := p.applyWorkflowSkills(checkout, "run", selection); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(checkout, ".opencode/skills/review/SKILL.md")
	if err := os.WriteFile(path, []byte("user edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.applyWorkflowSkills(checkout, "run", execution.Skills{}); err == nil {
		t.Fatal("edited file removed")
	}
	if data, _ := os.ReadFile(path); string(data) != "user edit" {
		t.Fatal("user edit lost")
	}
}

func TestWorkflowSkillsSelectOnlyRequestedInstalledPackage(t *testing.T) {
	p := &projectRuntime{root: t.TempDir()}
	checkout := t.TempDir()
	links := []skillLink{{"official/one", "1.0.0", "one"}, {"official/two", "1.0.0", "two"}}
	for _, l := range links {
		if err := os.MkdirAll(p.assetPath(l), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p.assetPath(l), "SKILL.md"), []byte(l.Name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeSkillIndex(p.linksPath(), links); err != nil {
		t.Fatal(err)
	}
	if err := p.applyWorkflowSkills(checkout, "run", execution.Skills{Plugins: []execution.SkillPlugin{{ID: "official/one", Version: "1.0.0"}}}); err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{".opencode", ".claude"} {
		if data, err := os.ReadFile(filepath.Join(checkout, base, "skills/one/SKILL.md")); err != nil || string(data) != "one" {
			t.Fatalf("selected package: %s %v", data, err)
		}
		if _, err := os.Stat(filepath.Join(checkout, base, "skills/two")); !os.IsNotExist(err) {
			t.Fatal("unselected package exposed")
		}
	}
	// A subsequent node may select a personal Skill with the same name as the old package.
	if err := p.applyWorkflowSkills(checkout, "run", execution.Skills{Documents: []execution.SkillDocument{{Name: "one", Description: "Personal", Content: "PERSONAL"}}}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(checkout, ".opencode/skills/one/SKILL.md")); err != nil || !strings.Contains(string(data), "PERSONAL") {
		t.Fatal("package was not replaced by selected personal skill", err)
	}
	if err := p.applyWorkflowSkills(checkout, "run", execution.Skills{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(checkout, ".opencode/skills/one")); !os.IsNotExist(err) {
		t.Fatal("disabled package remains")
	}
}
