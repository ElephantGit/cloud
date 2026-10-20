package core

// taskPersonalModelGap reuses the authoritative default-executor policy without returning private
// connection or credential material. It is advisory at read time and rechecked on dispatch.
func taskPersonalModelGap(t *transaction, uid string) string {
	reason, _, _ := defaultExecutorGap(t, uid)
	switch reason {
	case "":
		return ""
	case slotReasonConnectionDown:
		return "model_connection_unavailable"
	case slotReasonCredentialMissing:
		return "model_credential_required"
	default:
		return "model_default_required"
	}
}

func taskAgentReadiness(t *transaction, task Object, uid string) []Object {
	agents := t.list("SELECT id,plugin_id,space_id FROM space_agents WHERE tenant_id=$1 AND status='active' ORDER BY id LIMIT 500", task.S("tenantId"))
	project := t.one("SELECT space_id FROM projects WHERE id=$1 AND tenant_id=$2 AND deleted_at IS NULL", nullable(task.S("projectRef")), task.S("tenantId"))
	personalGap := taskPersonalModelGap(t, uid)
	readiness := make([]Object, 0, len(agents))
	for _, agent := range agents {
		personal := agent.S("pluginId") == "official/ora-space.opencode"
		gap := ""
		if project == nil || project.S("spaceId") != agent.S("spaceId") {
			gap = "issue_project_required"
		} else if personal {
			gap = personalGap
		}
		readiness = append(readiness, Object{"agentId": agent.S("id"), "requiresPersonalModel": personal, "errorCode": nullable(gap)})
	}
	return readiness
}
