//go:build linux

package agentruntime

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/skilldoc"
)

// workflowSkillState records only files owned by this projection. Project-authored files
// cannot be replaced, and edits to previously projected documents cause a conflict.
type workflowSkillState struct {
	Links     []skillLink
	Documents []execution.SkillDocument
}

// applyWorkflowSkills runs after exclusive checkout admission and before Agent startup.
// Workflow checkouts are excluded from the background market-wide projection, so one
// node's imported skills cannot leak into the next node through the shared run directory.
func (p *projectRuntime) applyWorkflowSkills(checkout, workspace string, selection execution.Skills) error {
	p.skillsMu.Lock()
	defer p.skillsMu.Unlock()
	statePath := filepath.Join(p.root, "workflow-skills-"+digest(workspace)+".json")
	var before workflowSkillState
	raw, err := os.ReadFile(statePath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &before); err != nil {
			return err
		}
	}
	installed, err := p.links()
	if err != nil {
		return err
	}
	next := workflowSkillState{Documents: selection.Documents}
	names := map[string]bool{}
	for _, plugin := range selection.Plugins {
		found := false
		for _, link := range installed {
			if link.ID != plugin.ID || link.Version != plugin.Version {
				continue
			}
			found = true
			if names[link.Name] {
				return errors.New("workflow_skill_name_conflict")
			}
			names[link.Name] = true
			if data, err := os.ReadFile(filepath.Join(p.assetPath(link), "SKILL.md")); err != nil || len(data) == 0 {
				return errors.New("workflow_skill_package_unavailable")
			}
			next.Links = append(next.Links, link)
		}
		if !found {
			return errors.New("workflow_skill_package_unavailable")
		}
	}
	for _, doc := range next.Documents {
		if _, err := skilldoc.Validate(skilldoc.Document{Name: doc.Name, Description: doc.Description, Content: doc.Content}); err != nil {
			return errors.New("workflow_skill_invalid")
		}
		if names[doc.Name] {
			return errors.New("workflow_skill_name_conflict")
		}
		names[doc.Name] = true
	}
	root, err := os.OpenRoot(checkout)
	if err != nil {
		return err
	}
	defer root.Close()
	oldDocs := map[string]string{}
	oldLinks := map[string]string{}
	for _, link := range before.Links {
		oldLinks[link.Name] = p.assetPath(link)
	}
	for _, doc := range before.Documents {
		oldDocs[doc.Name] = renderWorkflowSkill(doc)
	}
	// Check all owned documents before deleting any. Never traverse project symlinks.
	for _, base := range []string{".opencode", ".claude"} {
		if !ensureSkillDir(root, base) || !ensureSkillDir(root, base+"/skills") {
			return errors.New("workflow_skill_directory_conflict")
		}
		for name, content := range oldDocs {
			dir := base + "/skills/" + name
			info, err := root.Lstat(dir)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil || !info.IsDir() {
				return errors.New("workflow_skill_path_conflict")
			}
			info, err = root.Lstat(dir + "/SKILL.md")
			if err != nil || !info.Mode().IsRegular() {
				return errors.New("workflow_skill_content_conflict")
			}
			data, err := root.ReadFile(dir + "/SKILL.md")
			if err != nil || string(data) != content {
				return errors.New("workflow_skill_content_conflict")
			}
			entries, err := os.ReadDir(filepath.Join(checkout, dir))
			if err != nil || len(entries) != 1 {
				return errors.New("workflow_skill_content_conflict")
			}
		}
		for _, doc := range next.Documents {
			if _, owned := oldDocs[doc.Name]; owned {
				continue
			}
			path := base + "/skills/" + doc.Name
			if target, err := root.Readlink(path); err == nil && target == oldLinks[doc.Name] {
				continue // projectSkills removes the owned package link before writing the document.
			}
			if _, err := root.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
				return errors.New("workflow_skill_path_conflict")
			}
		}
	}
	for _, base := range []string{".opencode", ".claude"} {
		for name := range oldDocs {
			dir := base + "/skills/" + name
			if err := root.Remove(dir + "/SKILL.md"); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err := root.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	if err := p.projectSkills([]string{checkout}, before.Links, next.Links); err != nil {
		return errors.New("workflow_skill_path_conflict")
	}
	var created []string
	for _, base := range []string{".opencode", ".claude"} {
		for _, doc := range next.Documents {
			dir := base + "/skills/" + doc.Name
			wrote, err := writeUserSkill(root, dir, UserSkill{Name: doc.Name, Description: doc.Description, Content: doc.Content})
			if err != nil || !wrote {
				return errors.New("workflow_skill_write_failed")
			}
			created = append(created, dir)
		}
	}
	if err := excludeFromGit(root, created); err != nil {
		return err
	}
	return writeSkillIndex(statePath, next)
}

func renderWorkflowSkill(doc execution.SkillDocument) string {
	return skilldoc.Render(skilldoc.Document{Name: doc.Name, Description: doc.Description, Content: doc.Content})
}

func workflowSkillError(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "workflow_skill_")
}
