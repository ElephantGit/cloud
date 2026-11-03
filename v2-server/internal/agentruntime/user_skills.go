//go:build linux

package agentruntime

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/skilldoc"
)

// attachUserSkills projects the session owner's catalog into the two discovery roots Agents read.
// Existing project-owned or market-linked paths win; only a missing path is created.
func (p *projectRuntime) attachUserSkills(ctx context.Context, checkout string, scope execution.Scope) error {
	if p.driver.opts.UserSkills == nil {
		return nil
	}
	skills, err := p.driver.opts.UserSkills(ctx, scope)
	if err != nil {
		return execution.ErrUnavailable
	}
	if len(skills) == 0 {
		return nil
	}
	root, err := os.OpenRoot(checkout)
	if err != nil {
		return execution.ErrUnavailable
	}
	defer root.Close()
	for _, base := range []string{".opencode", ".claude"} {
		if !ensureSkillDir(root, base) || !ensureSkillDir(root, base+"/skills") {
			continue
		}
		for _, skill := range skills {
			if err := writeUserSkill(root, base+"/skills/"+skill.Name, skill); err != nil {
				return execution.ErrUnavailable
			}
		}
	}
	return nil
}

// ensureSkillDir reports whether path is a directory after creating missing components.
func ensureSkillDir(root *os.Root, path string) bool {
	info, err := root.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := root.Mkdir(path, 0700); err != nil {
			return false
		}
		return true
	}
	return err == nil && info.IsDir()
}

func writeUserSkill(root *os.Root, dir string, skill UserSkill) error {
	if err := skilldoc.ValidateName(skill.Name); err != nil {
		return err
	}
	_, err := root.Lstat(dir)
	switch {
	case err == nil:
		// Project-authored, market-linked, or duplicate catalog names are never overwritten.
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	if !strings.HasPrefix(dir, ".opencode/skills/") && !strings.HasPrefix(dir, ".claude/skills/") {
		return errors.New("invalid skill projection path")
	}
	if err := root.Mkdir(dir, 0700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return err
	}
	content := skilldoc.Render(skilldoc.Document{
		Name: skill.Name, Description: skill.Description, Content: skill.Content,
	})
	file, err := root.OpenFile(dir+"/SKILL.md", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if removeErr := root.Remove(dir); removeErr != nil {
			return errors.Join(err, removeErr)
		}
		return err
	}
	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = root.Remove(dir + "/SKILL.md")
		_ = root.Remove(dir)
		return err
	}
	return nil
}
