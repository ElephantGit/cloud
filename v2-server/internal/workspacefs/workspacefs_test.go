package workspacefs_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/workspacefs"
)

// checkout makes a Git repository with one commit, then changes it the way an Agent would:
// edit, delete, add, add binary, add ignored, all without staging.
func checkout(t *testing.T) workspacefs.Workspace {
	t.Helper()
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "init.defaultBranch=main"}, args...)...)
		cmd.Dir = root
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("git %v: %v: %s", args, e, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(name))
		if e := os.MkdirAll(filepath.Dir(full), 0o755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(full, []byte(content), 0o644); e != nil {
			t.Fatal(e)
		}
	}
	git("init", "-q", ".")
	write("README.md", "hello\n")
	write("src/old.go", "package src\n")
	write("src/keep.go", "package src\n")
	write(".gitignore", "*.log\n")
	git("add", ".")
	git("commit", "-qm", "base")
	base := git("rev-parse", "HEAD")
	write("README.md", "hello\nworld\n")
	if e := os.Remove(filepath.Join(root, "src", "old.go")); e != nil {
		t.Fatal(e)
	}
	write("src/new file.go", "package src\n\nfunc New() {}\n")
	write("logo.bin", "a\x00b")
	write("debug.log", "ignored\n")
	return workspacefs.Workspace{Root: root, BaseCommit: base}
}

func TestChangesIncludeUntrackedWithoutStaging(t *testing.T) {
	w := checkout(t)
	got, e := w.Changes(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	want := []workspacefs.Change{
		{Path: "README.md", Status: workspacefs.StatusModified, Additions: 1},
		{Path: "logo.bin", Status: workspacefs.StatusAdded, Binary: true},
		{Path: "src/new file.go", Status: workspacefs.StatusAdded, Additions: 3},
		{Path: "src/old.go", Status: workspacefs.StatusDeleted, Deletions: 1},
	}
	if got.BaseCommit != w.BaseCommit || got.Truncated || !reflect.DeepEqual(got.Files, want) {
		t.Fatalf("changes = %+v, want files %+v", got, want)
	}
	status, e := exec.Command("git", "-C", w.Root, "status", "--porcelain").Output()
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(status), "?? \"src/new file.go\"") && !strings.Contains(string(status), "?? src/") {
		t.Fatalf("reading changes staged the Agent's untracked files:\n%s", status)
	}
}

func TestDiffShowsOneFile(t *testing.T) {
	w := checkout(t)
	for _, tc := range []struct{ path, want string }{
		{"src/new file.go", "+func New() {}"},
		{"src/old.go", "-package src"},
		{"README.md", "+world"},
	} {
		d, e := w.Diff(context.Background(), tc.path)
		if e != nil {
			t.Fatalf("%s: %v", tc.path, e)
		}
		if !strings.Contains(d.Patch, tc.want) || strings.Count(d.Patch, "diff --git") != 1 {
			t.Fatalf("%s patch lacks %q or covers other files:\n%s", tc.path, tc.want, d.Patch)
		}
	}
	d, e := w.Diff(context.Background(), "src/keep.go")
	if e != nil || d.Patch != "" {
		t.Fatalf("unchanged file diff = %q, %v; want empty", d.Patch, e)
	}
}

func TestListHidesGitAndOutsideLinks(t *testing.T) {
	w := checkout(t)
	outside := t.TempDir()
	if e := os.Symlink(outside, filepath.Join(w.Root, "escape")); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink("src", filepath.Join(w.Root, "inner")); e != nil {
		t.Fatal(e)
	}
	got, e := w.List("")
	if e != nil {
		t.Fatal(e)
	}
	var names []string
	for _, x := range got.Entries {
		names = append(names, string(x.Kind)+":"+x.Path)
	}
	want := []string{"directory:inner", "directory:src", "file:.gitignore", "file:README.md", "file:debug.log", "file:logo.bin"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("entries = %v, want %v", names, want)
	}
	sub, e := w.List("src")
	if e != nil || len(sub.Entries) != 2 || sub.Entries[0].Path != "src/keep.go" {
		t.Fatalf("src = %+v, %v", sub, e)
	}
}

func TestReadText(t *testing.T) {
	w := checkout(t)
	f, e := w.Read("./src/../README.md")
	if e != nil || f.Path != "README.md" || f.Content != "hello\nworld\n" || f.SizeBytes != 12 {
		t.Fatalf("read = %+v, %v", f, e)
	}
}

func TestRefusals(t *testing.T) {
	w := checkout(t)
	if e := os.Symlink("/etc/hostname", filepath.Join(w.Root, "host")); e != nil {
		t.Fatal(e)
	}
	big := filepath.Join(w.Root, "big.txt")
	if e := os.WriteFile(big, []byte(strings.Repeat("x", workspacefs.MaxFileBytes+1)), 0o644); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name string
		call func() error
		want error
	}{
		{"absolute", func() error { _, e := w.Read("/etc/passwd"); return e }, workspacefs.ErrInvalidPath},
		{"backslash", func() error { _, e := w.Read(`..\x`); return e }, workspacefs.ErrInvalidPath},
		{"git dir", func() error { _, e := w.List(".git"); return e }, workspacefs.ErrInvalidPath},
		{"git file", func() error { _, e := w.Read("a/../.git/config"); return e }, workspacefs.ErrInvalidPath},
		{"git diff", func() error { _, e := w.Diff(context.Background(), ".git/HEAD"); return e }, workspacefs.ErrInvalidPath},
		{"symlink out", func() error { _, e := w.Read("host"); return e }, workspacefs.ErrInvalidPath},
		{"missing", func() error { _, e := w.Read("nope.txt"); return e }, workspacefs.ErrNotFound},
		{"binary", func() error { _, e := w.Read("logo.bin"); return e }, workspacefs.ErrBinary},
		{"too large", func() error { _, e := w.Read("big.txt"); return e }, workspacefs.ErrTooLarge},
		{"dir as file", func() error { _, e := w.Read("src"); return e }, workspacefs.ErrNotFile},
		{"file as dir", func() error { _, e := w.List("README.md"); return e }, workspacefs.ErrNotDirectory},
		{"gone", func() error {
			_, e := workspacefs.Workspace{Root: filepath.Join(w.Root, "missing"), BaseCommit: w.BaseCommit}.List("")
			return e
		}, workspacefs.ErrGone},
	} {
		if e := tc.call(); !errors.Is(e, tc.want) {
			t.Errorf("%s: error = %v, want %v", tc.name, e, tc.want)
		}
	}
}

func TestCleanStaysInside(t *testing.T) {
	for in, want := range map[string]string{"": "", ".": "", "a/./b/": "a/b", "../../a": "a", "a/../..": ""} {
		if got, e := workspacefs.Clean(in); e != nil || got != want {
			t.Errorf("Clean(%q) = %q, %v; want %q", in, got, e, want)
		}
	}
}
