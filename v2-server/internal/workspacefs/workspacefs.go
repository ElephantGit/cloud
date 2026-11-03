// Package workspacefs reads an Agent's checkout for the pages that let a person look at it: the
// directory tree, one file, and what changed against the commit the checkout started from. It is
// read-only and never touches the checkout's own Git index, so it cannot disturb a running Agent.
//
// Every path is relative to the checkout and is resolved through symlinks before use: a path that
// leaves the checkout, or names the .git directory, is refused rather than followed.
package workspacefs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Limits bound what one request reads, so a large file or diff cannot exhaust the server.
const (
	MaxFileBytes = 2 << 20
	MaxDiffBytes = 4 << 20
	MaxChanges   = 2000
	MaxEntries   = 5000
)

var (
	// ErrInvalidPath reports a path that is absolute, escapes the checkout or names .git.
	ErrInvalidPath = errors.New("invalid workspace path")
	// ErrNotFound reports a path that does not exist in the checkout.
	ErrNotFound = errors.New("workspace path not found")
	// ErrGone reports a checkout whose directory no longer exists.
	ErrGone = errors.New("workspace removed")
	// ErrNotDirectory and ErrNotFile report a path of the other kind.
	ErrNotDirectory = errors.New("not a directory")
	ErrNotFile      = errors.New("not a file")
	// ErrBinary reports content that is not UTF-8 text.
	ErrBinary = errors.New("binary file")
	// ErrTooLarge reports a file or diff beyond the limits above.
	ErrTooLarge = errors.New("too large")
)

// Workspace is one checkout and the commit it was cloned at.
type Workspace struct {
	Root       string
	BaseCommit string
}

// Kind is a directory entry's type.
type Kind string

// Entry kinds.
const (
	KindFile      Kind = "file"
	KindDirectory Kind = "directory"
)

// Entry is one child of a directory. Path is relative to the checkout, with forward slashes.
type Entry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind Kind   `json:"kind"`
}

// Directory lists one directory, directories first, then by name.
type Directory struct {
	Path      string  `json:"path"`
	Entries   []Entry `json:"entries"`
	Truncated bool    `json:"truncated"`
}

// File is one text file's content.
type File struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	SizeBytes int64  `json:"sizeBytes"`
}

// Status is how a file changed against the base commit.
type Status string

// Change statuses. Renames are reported as a deletion and an addition.
const (
	StatusAdded    Status = "added"
	StatusModified Status = "modified"
	StatusDeleted  Status = "deleted"
)

// Change is one changed file. Additions and Deletions are line counts, zero for binary files.
type Change struct {
	Path      string `json:"path"`
	Status    Status `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary"`
}

// Changes lists every file that differs from the base commit, including untracked files that are
// not ignored, sorted by path.
type Changes struct {
	BaseCommit string   `json:"baseCommit"`
	Files      []Change `json:"files"`
	Truncated  bool     `json:"truncated"`
}

// Diff is one file's unified diff against the base commit; Patch is empty when it is unchanged.
type Diff struct {
	Path  string `json:"path"`
	Patch string `json:"patch"`
}

// root returns the checkout's resolved root, or ErrGone.
func (w Workspace) root() (string, error) {
	root, e := filepath.EvalSymlinks(w.Root)
	if errors.Is(e, os.ErrNotExist) {
		return "", ErrGone
	}
	if e != nil {
		return "", fmt.Errorf("resolve workspace: %w", e)
	}
	return root, nil
}

// Clean validates a page-supplied relative path and returns it in canonical slash form; the
// checkout root is "". It never touches the disk.
func Clean(p string) (string, error) {
	if strings.ContainsRune(p, 0) || strings.Contains(p, "\\") || strings.HasPrefix(p, "/") {
		return "", ErrInvalidPath
	}
	p = path.Clean("/" + p)[1:]
	if p == "" {
		return "", nil
	}
	if first, _, _ := strings.Cut(p, "/"); first == ".git" {
		return "", ErrInvalidPath
	}
	return p, nil
}

// resolve maps a cleaned relative path to a location inside the resolved root, following symlinks
// and refusing any that lead outside it or into .git.
func resolve(root, rel string) (string, error) {
	full, e := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(e, os.ErrNotExist) {
		return "", ErrNotFound
	}
	if e != nil {
		return "", fmt.Errorf("resolve path: %w", e)
	}
	inside, e := filepath.Rel(root, full)
	if e != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", ErrInvalidPath
	}
	if inside != "." {
		if first, _, _ := strings.Cut(filepath.ToSlash(inside), "/"); first == ".git" {
			return "", ErrInvalidPath
		}
	}
	return full, nil
}

// List returns the children of the directory at rel. Symlinks are listed by what they point to;
// those leading outside the checkout, and .git, are left out.
func (w Workspace) List(rel string) (Directory, error) {
	rel, e := Clean(rel)
	if e != nil {
		return Directory{}, e
	}
	root, e := w.root()
	if e != nil {
		return Directory{}, e
	}
	dir, e := resolve(root, rel)
	if e != nil {
		return Directory{}, e
	}
	if info, e := os.Stat(dir); e != nil {
		return Directory{}, fmt.Errorf("stat directory: %w", e)
	} else if !info.IsDir() {
		return Directory{}, ErrNotDirectory
	}
	items, e := os.ReadDir(dir)
	if e != nil {
		return Directory{}, fmt.Errorf("read directory: %w", e)
	}
	out := Directory{Path: rel, Entries: []Entry{}}
	for _, item := range items {
		if rel == "" && item.Name() == ".git" {
			continue
		}
		child := path.Join(rel, item.Name())
		kind := KindFile
		if item.Type()&os.ModeSymlink != 0 {
			target, e := resolve(root, child)
			if e != nil {
				continue
			}
			info, e := os.Stat(target)
			if e != nil {
				continue
			}
			if info.IsDir() {
				kind = KindDirectory
			}
		} else if item.IsDir() {
			kind = KindDirectory
		} else if !item.Type().IsRegular() {
			continue
		}
		if len(out.Entries) == MaxEntries {
			out.Truncated = true
			break
		}
		out.Entries = append(out.Entries, Entry{Name: item.Name(), Path: child, Kind: kind})
	}
	slices.SortFunc(out.Entries, func(a, b Entry) int {
		if a.Kind != b.Kind {
			if a.Kind == KindDirectory {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

// Read returns the text file at rel. Files over MaxFileBytes report ErrTooLarge and content that
// is not UTF-8 or contains NUL reports ErrBinary.
func (w Workspace) Read(rel string) (File, error) {
	rel, e := Clean(rel)
	if e != nil {
		return File{}, e
	}
	if rel == "" {
		return File{}, ErrNotFile
	}
	root, e := w.root()
	if e != nil {
		return File{}, e
	}
	full, e := resolve(root, rel)
	if e != nil {
		return File{}, e
	}
	f, e := os.Open(full)
	if e != nil {
		return File{}, fmt.Errorf("open file: %w", e)
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return File{}, fmt.Errorf("stat file: %w", e)
	}
	if !info.Mode().IsRegular() {
		return File{}, ErrNotFile
	}
	if info.Size() > MaxFileBytes {
		return File{}, ErrTooLarge
	}
	data, e := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if e != nil {
		return File{}, fmt.Errorf("read file: %w", e)
	}
	if len(data) > MaxFileBytes {
		return File{}, ErrTooLarge
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return File{}, ErrBinary
	}
	return File{Path: rel, Content: string(data), SizeBytes: int64(len(data))}, nil
}

// Changes lists what differs from the base commit, untracked files included.
func (w Workspace) Changes(ctx context.Context) (Changes, error) {
	root, e := w.root()
	if e != nil {
		return Changes{}, e
	}
	out := Changes{BaseCommit: w.BaseCommit, Files: []Change{}}
	e = w.withIndex(ctx, root, func(git gitFunc) error {
		status, e := git(MaxDiffBytes, "diff", "--name-status", "-z", w.BaseCommit, "--")
		if e != nil {
			return e
		}
		numstat, e := git(MaxDiffBytes, "diff", "--numstat", "-z", w.BaseCommit, "--")
		if e != nil {
			return e
		}
		counts := parseNumstat(numstat)
		fields := strings.Split(strings.TrimSuffix(string(status), "\x00"), "\x00")
		for i := 0; i+1 < len(fields); i += 2 {
			c := Change{Path: fields[i+1]}
			switch fields[i] {
			case "A":
				c.Status = StatusAdded
			case "D":
				c.Status = StatusDeleted
			default:
				c.Status = StatusModified
			}
			n, ok := counts[c.Path]
			c.Binary = !ok || n.binary
			c.Additions, c.Deletions = n.additions, n.deletions
			if len(out.Files) == MaxChanges {
				out.Truncated = true
				break
			}
			out.Files = append(out.Files, c)
		}
		return nil
	})
	if e != nil {
		return Changes{}, e
	}
	slices.SortFunc(out.Files, func(a, b Change) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

// Diff returns one file's unified diff against the base commit. rel may name a deleted file.
func (w Workspace) Diff(ctx context.Context, rel string) (Diff, error) {
	rel, e := Clean(rel)
	if e != nil {
		return Diff{}, e
	}
	if rel == "" {
		return Diff{}, ErrNotFile
	}
	root, e := w.root()
	if e != nil {
		return Diff{}, e
	}
	var patch []byte
	e = w.withIndex(ctx, root, func(git gitFunc) error {
		var e error
		patch, e = git(MaxDiffBytes, "diff", w.BaseCommit, "--", ":(literal)"+rel)
		return e
	})
	if e != nil {
		return Diff{}, e
	}
	return Diff{Path: rel, Patch: string(patch)}, nil
}

type gitFunc func(limit int, args ...string) ([]byte, error)

// withIndex runs git against a private copy of the checkout's index in which untracked, unignored
// files are marked intent-to-add, so diffs show them as added without staging anything the Agent
// would see. The copy is removed afterwards.
func (w Workspace) withIndex(ctx context.Context, root string, run func(gitFunc) error) error {
	if !validCommit(w.BaseCommit) {
		return fmt.Errorf("workspace base commit %q is not a full object name", w.BaseCommit)
	}
	tmp, e := os.CreateTemp("", "workspace-index-")
	if e != nil {
		return fmt.Errorf("create index copy: %w", e)
	}
	index := tmp.Name()
	defer os.Remove(index)
	git := func(limit int, args ...string) ([]byte, error) {
		return runGit(ctx, root, index, limit, args...)
	}
	source, e := runGit(ctx, root, "", 4096, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if e == nil {
		e = copyIndex(strings.TrimSpace(string(source)), tmp)
	}
	if e = errors.Join(e, tmp.Close()); e != nil {
		return e
	}
	if _, e := git(4096, "add", "--intent-to-add", "--all", "--", "."); e != nil {
		return e
	}
	return run(git)
}

func copyIndex(source string, dst *os.File) error {
	src, e := os.Open(source)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return fmt.Errorf("open index: %w", e)
	}
	defer src.Close()
	if _, e := io.Copy(dst, src); e != nil {
		return fmt.Errorf("copy index: %w", e)
	}
	return nil
}

// runGit runs git in the checkout with the Agent's repository configuration neutralized where it
// could run programs or change the output format: hooks, fsmonitor, external diff and textconv.
func runGit(ctx context.Context, root, index string, limit int, args ...string) ([]byte, error) {
	base := []string{"-c", "core.quotepath=off", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null",
		"-c", "diff.noprefix=false", "-c", "diff.mnemonicPrefix=false", "-c", "color.ui=false"}
	if args[0] == "diff" {
		args = append([]string{"diff", "--no-renames", "--no-ext-diff", "--no-textconv", "--no-color"}, args[1:]...)
	}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	if index != "" {
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+index)
	}
	out := &limitedBuffer{limit: limit}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = out, &stderr
	e := cmd.Run()
	if out.over {
		return nil, ErrTooLarge
	}
	if e != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], e, strings.TrimSpace(stderr.String()))
	}
	return out.Bytes(), nil
}

// limitedBuffer keeps at most limit bytes and records whether more arrived.
type limitedBuffer struct {
	bytes.Buffer
	limit int
	over  bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		b.over = true
		return 0, ErrTooLarge
	}
	return b.Buffer.Write(p)
}

type lineCount struct {
	additions, deletions int
	binary               bool
}

// parseNumstat reads `git diff --numstat -z` without renames: "added\tdeleted\tpath\0" per file,
// with "-" counts for binary files.
func parseNumstat(out []byte) map[string]lineCount {
	counts := map[string]lineCount{}
	for _, record := range strings.Split(string(out), "\x00") {
		added, rest, ok := strings.Cut(record, "\t")
		if !ok {
			continue
		}
		deleted, name, ok := strings.Cut(rest, "\t")
		if !ok {
			continue
		}
		a, ae := strconv.Atoi(added)
		d, de := strconv.Atoi(deleted)
		counts[name] = lineCount{additions: a, deletions: d, binary: ae != nil || de != nil}
	}
	return counts
}

func validCommit(c string) bool {
	if len(c) != 40 && len(c) != 64 {
		return false
	}
	for _, r := range c {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}
