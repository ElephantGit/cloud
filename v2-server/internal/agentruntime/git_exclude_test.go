//go:build linux

package agentruntime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// gitCheckout is a committed repository with a project-owned skill and an exclude rule of its own.
func gitCheckout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q", ".")
	if err := os.MkdirAll(filepath.Join(dir, ".claude", "skills", "owned"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude", "skills", "owned", "SKILL.md"), []byte("project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "base")
	if err := os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), []byte("# project rule\n*.tmp"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func gitStatus(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestMaterializedSkillsStayOutOfGit(t *testing.T) {
	checkout := gitCheckout(t)
	p := &projectRuntime{root: t.TempDir(), driver: &Driver{opts: Options{
		UserSkills: func(context.Context, execution.Scope) ([]UserSkill, error) {
			return []UserSkill{{Name: "review", Description: "检查改动", Content: "# Review"}, {Name: "owned", Description: "项目已有", Content: "# Owned"}}, nil
		},
	}}}
	for range 2 {
		if err := p.attachUserSkills(context.Background(), checkout, execution.Scope{}); err != nil {
			t.Fatal(err)
		}
	}
	market := skillLink{ID: "acme/tools", Version: "1.0.0", Name: "lint"}
	if err := p.projectSkills([]string{checkout}, nil, []skillLink{market}); err != nil {
		t.Fatal(err)
	}
	if status := gitStatus(t, checkout); status != "" {
		t.Fatalf("Ora's skills show up in git status:\n%s", status)
	}
	// The Agent's own work, including a change to the project's skill, stays visible.
	if err := os.WriteFile(filepath.Join(checkout, ".claude", "skills", "owned", "SKILL.md"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, ".claude", "skills", "new.md"), []byte("agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if status := gitStatus(t, checkout); status != " M .claude/skills/owned/SKILL.md\n?? .claude/skills/new.md\n" {
		t.Fatalf("status after the Agent's edits = %q", status)
	}
	exclude, err := os.ReadFile(filepath.Join(checkout, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# project rule\n*.tmp\n" + excludeHeader + "\n" +
		"/.opencode/skills/review\n/.opencode/skills/owned\n/.claude/skills/review\n/.claude/skills/lint\n/.opencode/skills/lint\n"
	if string(exclude) != want {
		t.Fatalf("exclude =\n%s\nwant\n%s", exclude, want)
	}
}

func TestExcludeLeavesCheckoutsWithoutGitDirectory(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := excludeFromGit(root, []string{".claude/skills/x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Fatalf("created .git in a checkout without one: %v", err)
	}
}

func TestEscapeGitPattern(t *testing.T) {
	if got := escapeGitPattern("a b/*[x]?!#"); got != `a\ b/\*\[x]\?\!\#` {
		t.Fatalf("escape = %s", got)
	}
	if strings.ContainsRune(escapeGitPattern("plain-name_1"), '\\') {
		t.Fatal("escaped a plain name")
	}
}
