package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// squadStore is the part of the database the squad routes need. It lives beside the handlers so
// the rest of the server can run against a store that does not implement squads yet.
type squadStore interface {
	TenantMemberRole(context.Context, string, string) (string, error)
	Squads(context.Context, string) ([]store.SquadSummary, error)
	Squad(context.Context, string, string) (store.SquadDetail, error)
	CreateSquad(context.Context, string, string, store.SquadInput) (store.SquadDetail, error)
	UpdateSquad(context.Context, string, string, string, store.SquadInput) (store.SquadDetail, error)
	ArchiveSquad(context.Context, string, string, string, int64) (store.SquadDetail, error)
	AddSquadMember(context.Context, string, string, string, store.SquadMemberInput) (store.SquadDetail, error)
	UpdateSquadMember(context.Context, string, string, string, string, store.SquadMemberUpdate) (store.SquadDetail, error)
	RemoveSquadMember(context.Context, string, string, string, string, int64) (store.SquadDetail, error)
}

var squadUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type squadRequest struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Charter       string          `json:"charter"`
	LeaderAgentID json.RawMessage `json:"leaderAgentId"`
	Version       int64           `json:"version"`
}

type squadMemberRequest struct {
	Type         string `json:"type"`
	UserID       string `json:"userId"`
	SpaceAgentID string `json:"spaceAgentId"`
	Role         string `json:"role"`
	Maintainer   bool   `json:"maintainer"`
	Version      int64  `json:"version"`
}

type squadMemberUpdateRequest struct {
	Role       *string `json:"role"`
	Maintainer *bool   `json:"maintainer"`
	Version    int64   `json:"version"`
}

func decodeSquad(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		fault(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		fault(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	return true
}

// squadLeader separates an absent leader (create only), an explicit null (clear) and an id.
func squadLeader(raw json.RawMessage, required bool) (*string, string) {
	if len(raw) == 0 {
		if required {
			return nil, "leader_required"
		}
		return nil, ""
	}
	if string(raw) == "null" {
		return nil, ""
	}
	var id string
	if json.Unmarshal(raw, &id) != nil || !squadUUID.MatchString(id) {
		return nil, "invalid_leader"
	}
	return &id, ""
}

func validSquadText(name, description, charter string) string {
	if !store.ValidSquadName(name) {
		return "invalid_name"
	}
	if utf8.RuneCountInString(description) > 2000 {
		return "invalid_description"
	}
	if utf8.RuneCountInString(charter) > 8000 {
		return "invalid_charter"
	}
	return ""
}

func squadMemberRole(role string) bool {
	return utf8.RuneCountInString(role) <= 500
}

// squadFault writes the stable error for a squad store error and reports whether it handled it.
func squadFault(w http.ResponseWriter, e error) bool {
	switch {
	case errors.Is(e, store.ErrNotFound):
		fault(w, http.StatusNotFound, "not_found")
	case errors.Is(e, store.ErrForbidden):
		fault(w, http.StatusForbidden, "forbidden")
	case errors.Is(e, store.ErrConflict):
		fault(w, http.StatusConflict, "version_conflict")
	case errors.Is(e, store.ErrArchived):
		fault(w, http.StatusConflict, "squad_archived")
	case errors.Is(e, store.ErrMemberExists):
		fault(w, http.StatusConflict, "member_exists")
	case errors.Is(e, store.ErrInvalidMember):
		fault(w, http.StatusBadRequest, "invalid_member")
	case errors.Is(e, store.ErrAgentUnavailable):
		fault(w, http.StatusBadRequest, "invalid_space_agent")
	case errors.Is(e, store.ErrUserUnavailable):
		fault(w, http.StatusBadRequest, "invalid_user")
	default:
		return false
	}
	return true
}

func (h *handler) squadStore(w http.ResponseWriter) (squadStore, bool) {
	s, ok := h.store.(squadStore)
	if !ok {
		fault(w, http.StatusServiceUnavailable, "squad_unavailable")
	}
	return s, ok
}

// squadMember keeps the tenant scoping and live-membership check in one place: every squad route
// answers 404 to a non-member so another tenant cannot be enumerated.
func (h *handler) squadMember(w http.ResponseWriter, r *http.Request, u store.User, tenant, squadID string) (squadStore, bool) {
	s, ok := h.squadStore(w)
	if !ok {
		return nil, false
	}
	if !squadUUID.MatchString(tenant) || (squadID != "" && !squadUUID.MatchString(squadID)) {
		fault(w, http.StatusNotFound, "not_found")
		return nil, false
	}
	if _, e := s.TenantMemberRole(r.Context(), tenant, u.ID); e != nil {
		if squadFault(w, e) {
			return nil, false
		}
		internal(w, "squad membership", e)
		return nil, false
	}
	return s, true
}

func (h *handler) listSquads(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant := r.PathValue("tid")
	s, ok := h.squadMember(w, r, u, tenant, "")
	if !ok {
		return
	}
	items, e := s.Squads(r.Context(), tenant)
	if e != nil {
		internal(w, "list squads", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": ""})
}

func (h *handler) createSquad(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant := r.PathValue("tid")
	s, ok := h.squadMember(w, r, u, tenant, "")
	if !ok {
		return
	}
	var req squadRequest
	if !decodeSquad(w, r, &req) {
		return
	}
	if req.Version != 0 {
		fault(w, http.StatusBadRequest, "invalid_version")
		return
	}
	leader, code := squadLeader(req.LeaderAgentID, false)
	if code != "" {
		fault(w, http.StatusBadRequest, code)
		return
	}
	in := store.SquadInput{Name: req.Name, Description: req.Description, Charter: req.Charter, LeaderAgentID: leader}
	if code := validSquadText(in.Name, in.Description, in.Charter); code != "" {
		fault(w, http.StatusBadRequest, code)
		return
	}
	out, e := s.CreateSquad(r.Context(), tenant, u.ID, in)
	if e != nil {
		if squadFault(w, e) {
			return
		}
		internal(w, "create squad", e)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *handler) getSquad(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, id := r.PathValue("tid"), r.PathValue("sid")
	s, ok := h.squadMember(w, r, u, tenant, id)
	if !ok {
		return
	}
	out, e := s.Squad(r.Context(), tenant, id)
	if e != nil {
		if squadFault(w, e) {
			return
		}
		internal(w, "get squad", e)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) updateSquad(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, id := r.PathValue("tid"), r.PathValue("sid")
	s, ok := h.squadMember(w, r, u, tenant, id)
	if !ok {
		return
	}
	var req squadRequest
	if !decodeSquad(w, r, &req) {
		return
	}
	if req.Version < 1 {
		fault(w, http.StatusPreconditionRequired, "version_required")
		return
	}
	leader, code := squadLeader(req.LeaderAgentID, true)
	if code != "" {
		fault(w, http.StatusBadRequest, code)
		return
	}
	in := store.SquadInput{Name: req.Name, Description: req.Description, Charter: req.Charter, LeaderAgentID: leader, Version: req.Version}
	if code := validSquadText(in.Name, in.Description, in.Charter); code != "" {
		fault(w, http.StatusBadRequest, code)
		return
	}
	out, e := s.UpdateSquad(r.Context(), tenant, u.ID, id, in)
	if e != nil {
		if squadFault(w, e) {
			return
		}
		internal(w, "update squad", e)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) archiveSquad(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, id := r.PathValue("tid"), r.PathValue("sid")
	s, ok := h.squadMember(w, r, u, tenant, id)
	if !ok {
		return
	}
	var req struct {
		Version int64 `json:"version"`
	}
	if !decodeSquad(w, r, &req) {
		return
	}
	if req.Version < 1 {
		fault(w, http.StatusPreconditionRequired, "version_required")
		return
	}
	out, e := s.ArchiveSquad(r.Context(), tenant, u.ID, id, req.Version)
	if e != nil {
		if squadFault(w, e) {
			return
		}
		internal(w, "archive squad", e)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) addSquadMember(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, id := r.PathValue("tid"), r.PathValue("sid")
	s, ok := h.squadMember(w, r, u, tenant, id)
	if !ok {
		return
	}
	var req squadMemberRequest
	if !decodeSquad(w, r, &req) {
		return
	}
	if req.Version < 1 {
		fault(w, http.StatusPreconditionRequired, "version_required")
		return
	}
	if !squadMemberRole(req.Role) {
		fault(w, http.StatusBadRequest, "invalid_role")
		return
	}
	switch req.Type {
	case store.MemberHuman:
		if req.SpaceAgentID != "" || !squadUUID.MatchString(req.UserID) {
			fault(w, http.StatusBadRequest, "invalid_member")
			return
		}
	case store.MemberAgent:
		if req.UserID != "" || req.Maintainer || !squadUUID.MatchString(req.SpaceAgentID) {
			fault(w, http.StatusBadRequest, "invalid_member")
			return
		}
	default:
		fault(w, http.StatusBadRequest, "invalid_member")
		return
	}
	in := store.SquadMemberInput{Type: req.Type, UserID: req.UserID, SpaceAgentID: req.SpaceAgentID, Role: req.Role, Maintainer: req.Maintainer, Version: req.Version}
	out, e := s.AddSquadMember(r.Context(), tenant, u.ID, id, in)
	if e != nil {
		if squadFault(w, e) {
			return
		}
		internal(w, "add squad member", e)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *handler) updateSquadMember(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, id, member := r.PathValue("tid"), r.PathValue("sid"), r.PathValue("mid")
	if !squadUUID.MatchString(member) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	s, ok := h.squadMember(w, r, u, tenant, id)
	if !ok {
		return
	}
	var req squadMemberUpdateRequest
	if !decodeSquad(w, r, &req) {
		return
	}
	if req.Version < 1 {
		fault(w, http.StatusPreconditionRequired, "version_required")
		return
	}
	if req.Role == nil || req.Maintainer == nil || !squadMemberRole(*req.Role) {
		fault(w, http.StatusBadRequest, "invalid_member")
		return
	}
	in := store.SquadMemberUpdate{Role: *req.Role, Maintainer: *req.Maintainer, Version: req.Version}
	out, e := s.UpdateSquadMember(r.Context(), tenant, u.ID, id, member, in)
	if e != nil {
		if squadFault(w, e) {
			return
		}
		internal(w, "update squad member", e)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) removeSquadMember(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, id, member := r.PathValue("tid"), r.PathValue("sid"), r.PathValue("mid")
	if !squadUUID.MatchString(member) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	s, ok := h.squadMember(w, r, u, tenant, id)
	if !ok {
		return
	}
	version, e := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("version")), 10, 64)
	if e != nil || version < 1 {
		fault(w, http.StatusPreconditionRequired, "version_required")
		return
	}
	out, e := s.RemoveSquadMember(r.Context(), tenant, u.ID, id, member, version)
	if e != nil {
		if squadFault(w, e) {
			return
		}
		internal(w, "remove squad member", e)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
