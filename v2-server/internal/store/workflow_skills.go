package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// ResolveWorkflowSkills resolves names against the run owner's catalog and plugin IDs
// against the target project's space. A selection never grants access to another space.
func (s *Store) ResolveWorkflowSkills(ctx context.Context, scope execution.Scope, project string, names []string) (*execution.Skills, error) {
	result := &execution.Skills{}
	if len(names) == 0 {
		return result, nil
	}
	if _, err := s.GetProject(ctx, scope, project); err != nil {
		return nil, fmt.Errorf("could not load the project for selected Skills")
	}
	documents, err := s.ListSkills(ctx, scope.TenantID, scope.UserID)
	if err != nil {
		return nil, fmt.Errorf("could not load your imported Skills")
	}
	byName := make(map[string]Skill, len(documents))
	for _, doc := range documents {
		byName[doc.Name] = doc
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		if strings.Contains(name, "/") {
			namespace, identifier, ok := pluginIdentity(name)
			if !ok {
				return nil, fmt.Errorf("invalid Skill plugin identity: %s", name)
			}
			var version string
			err := s.pool.QueryRow(ctx, `SELECT a.desired_version FROM v2_projects p
			 JOIN v2_space_agents a ON a.space_id=p.plugin_space_id AND a.tenant_id=p.tenant_id
			 JOIN v2_skill_results r ON r.selection_id=a.id AND r.project_id=p.id
			 WHERE p.id=$1 AND p.tenant_id=$2 AND a.source_namespace=$3 AND a.identifier=$4
			 AND a.kind='skill' AND a.desired_state='installed' AND a.status='active'
			 AND r.revision=a.version AND r.state='installed' AND r.installed_version=a.desired_version`,
				project, scope.TenantID, namespace, identifier).Scan(&version)
			if err != nil {
				return nil, fmt.Errorf("Skill plugin %s is not installed in this project; install it and wait for project confirmation", name)
			}
			result.Plugins = append(result.Plugins, execution.SkillPlugin{ID: name, Version: version})
		} else {
			doc, ok := byName[name]
			if !ok {
				return nil, fmt.Errorf("Skill %s is missing from your imported skills", name)
			}
			result.Documents = append(result.Documents, execution.SkillDocument{Name: doc.Name, Description: doc.Description, Content: doc.Content})
		}
	}
	return result, nil
}
