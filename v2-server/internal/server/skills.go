package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/wanglongan587/cloud/v2-server/internal/skilldoc"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func (h *handler) listSkills(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant := r.PathValue("tid")
	if !squadUUID.MatchString(tenant) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	items, e := h.store.ListSkills(r.Context(), tenant, u.ID)
	if e != nil {
		skillError(w, "list skills", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *handler) getSkill(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, id := r.PathValue("tid"), r.PathValue("sid")
	if !squadUUID.MatchString(tenant) || !squadUUID.MatchString(id) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	item, e := h.store.GetSkill(r.Context(), tenant, u.ID, id)
	if e != nil {
		skillError(w, "get skill", e)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *handler) createSkill(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant := r.PathValue("tid")
	if !squadUUID.MatchString(tenant) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}
	if !decodeSkillJSON(w, r, &in) {
		return
	}
	item, e := h.store.CreateSkill(r.Context(), tenant, u.ID, store.SkillInput{
		Name: in.Name, Description: in.Description, Content: in.Content,
	})
	if e != nil {
		skillError(w, "create skill", e)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *handler) updateSkill(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, id := r.PathValue("tid"), r.PathValue("sid")
	if !squadUUID.MatchString(tenant) || !squadUUID.MatchString(id) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}
	if !decodeSkillJSON(w, r, &in) {
		return
	}
	item, e := h.store.UpdateSkill(r.Context(), tenant, u.ID, id, store.SkillInput{
		Name: in.Name, Description: in.Description, Content: in.Content,
	})
	if e != nil {
		skillError(w, "update skill", e)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *handler) deleteSkill(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, id := r.PathValue("tid"), r.PathValue("sid")
	if !squadUUID.MatchString(tenant) || !squadUUID.MatchString(id) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	if e := h.store.DeleteSkill(r.Context(), tenant, u.ID, id); e != nil {
		skillError(w, "delete skill", e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) importSkill(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant := r.PathValue("tid")
	if !squadUUID.MatchString(tenant) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	var in struct {
		FileName  string `json:"fileName"`
		Content   string `json:"content"`
		Overwrite bool   `json:"overwrite"`
	}
	if !decodeSkillJSON(w, r, &in) {
		return
	}
	if ext := strings.ToLower(filepath.Ext(in.FileName)); ext != ".md" && ext != ".markdown" {
		fault(w, http.StatusBadRequest, "skill_file_type")
		return
	}
	item, e := h.store.ImportSkill(r.Context(), tenant, u.ID, []byte(in.Content), in.Overwrite)
	if e != nil {
		skillError(w, "import skill", e)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// decodeSkillJSON keeps the existing strict JSON behavior but admits a full 1 MiB SKILL.md.
func decodeSkillJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		fault(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	return true
}

func skillError(w http.ResponseWriter, operation string, e error) {
	if code := skilldoc.Code(e); code != "" {
		fault(w, http.StatusBadRequest, code)
		return
	}
	switch {
	case errors.Is(e, store.ErrNotFound):
		fault(w, http.StatusForbidden, "forbidden")
	case errors.Is(e, store.ErrSkillNotFound):
		fault(w, http.StatusNotFound, "skill_not_found")
	case errors.Is(e, store.ErrSkillNameConflict):
		fault(w, http.StatusConflict, "skill_name_conflict")
	default:
		internal(w, operation, e)
	}
}
