package contract

func joinManagementSchemas(s obj) {
	methods := func() obj { return enumeration("workspace_created", "invitation", "application", "directory") }
	for _, name := range []string{"Invitation", "JoinLink"} {
		p := properties(s, name)
		p["note"], p["createdByName"] = str(), str()
	}
	properties(s, "Invitation")["consumedByName"] = optional(str())
	properties(s, "JoinLink")["pendingCount"] = number()
	properties(s, "JoinLink")["approvedCount"] = number()
	properties(s, "JoinRequest")["linkNote"] = str()
	properties(s, "JoinRequest")["decidedByName"] = optional(str())
	properties(s, "MemberListItem")["joinMethod"] = optional(methods())
	properties(s, "MemberListItem")["joinedAt"] = optional(timestamp())
	properties(s, "MemberListItem")["actorDisplayName"] = optional(str())
	s["JoinLinkPreview"] = object(obj{
		"kind": enumeration("invite", "apply"), "state": enumeration("active", "used", "expired", "revoked", "unavailable"),
		"name": str(), "slug": str(), "alreadyMember": boolean(), "applicationStatus": enumeration("pending"),
	}, "kind", "state")
	s["MemberJoinEvent"] = object(obj{
		"id": uuid(), "tenantId": uuid(), "userId": uuid(), "method": methods(), "actorUserId": uuid(),
		"actorDisplayName": str(), "joinedAt": timestamp(), "invitationId": optional(uuid()), "requestId": optional(uuid()),
		"invitationNote": optional(str()), "linkNote": optional(str()),
	}, "id", "tenantId", "userId", "method", "actorUserId", "actorDisplayName", "joinedAt")
}
