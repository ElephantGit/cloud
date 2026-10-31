package nodeservices

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const grantTTL = 10 * time.Minute

func (s *Service) issue(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		fault(w, 405, "method_not_allowed")
		return
	}
	var req struct {
		BindingID   string `json:"bindingId"`
		ExecutionID string `json:"executionId"`
	}
	if !decode(w, r, &req) {
		return
	}
	id, err := randomID()
	if err != nil {
		fault(w, 500, "grant_unavailable")
		return
	}
	token, err := randomID()
	if err != nil {
		fault(w, 500, "grant_unavailable")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || req.ExecutionID == "" || req.BindingID == "" || s.permits[req.ExecutionID] != req.BindingID {
		fault(w, 403, "model_access_denied")
		return
	}
	// Duplicate grant requests invalidate the previous token, and expired entries never grow
	// without bound. Only the active execution's binding decides the model/provider.
	for existing, g := range s.grants {
		if g.execution == req.ExecutionID || !time.Now().Before(g.expires) {
			g.cancel()
			delete(s.grants, existing)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	g := &grant{execution: req.ExecutionID, binding: req.BindingID, token: token, expires: time.Now().Add(grantTTL), ctx: ctx, cancel: cancel}
	s.grants[id] = g
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"grantId": id, "token": token, "expiresAt": g.expires, "protocol": s.opts.Bindings[g.binding].Protocol, "proxyBaseUrl": s.URL + proxyPath[s.opts.Bindings[g.binding].Protocol], "model": s.opts.Bindings[g.binding].Model})
}

func (s *Service) changeGrant(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/internal/v1/model-grants/")
	if path == r.URL.Path {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	renew := len(parts) == 2 && parts[1] == "renew" && r.Method == "POST"
	revoke := len(parts) == 1 && r.Method == "DELETE"
	if (!renew && !revoke) || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	var req struct {
		ExecutionID string `json:"executionId"`
	}
	if !decode(w, r, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.grants[parts[0]]
	if s.closed || g == nil || g.execution != req.ExecutionID {
		fault(w, 403, "model_access_revoked")
		return
	}
	if revoke {
		g.cancel()
		delete(s.grants, parts[0])
		_ = json.NewEncoder(w).Encode(map[string]any{})
		return
	}
	if s.permits[g.execution] != g.binding || !time.Now().Before(g.expires) {
		g.cancel()
		delete(s.grants, parts[0])
		fault(w, 403, "model_access_revoked")
		return
	}
	g.expires = time.Now().Add(grantTTL)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"expiresAt": g.expires})
}

// model runs one Agent model call on its grant's binding. A token is accepted only on the
// route of its binding's protocol, so a Messages grant cannot reach an OpenAI provider.
func (s *Service) model(w http.ResponseWriter, r *http.Request, protocol Protocol, token string) {
	if r.Method != "POST" {
		fault(w, 405, "method_not_allowed")
		return
	}
	if token == "" {
		fault(w, 401, "invalid_model_token")
		return
	}
	s.mu.Lock()
	var selected *grant
	for _, g := range s.grants {
		if subtle.ConstantTimeCompare([]byte(g.token), []byte(token)) == 1 && time.Now().Before(g.expires) && s.permits[g.execution] == g.binding {
			selected = g
			break
		}
	}
	if selected == nil || s.closed || s.opts.Bindings[selected.binding].Protocol != protocol {
		s.mu.Unlock()
		fault(w, 401, "invalid_model_token")
		return
	}
	binding := s.opts.Bindings[selected.binding]
	// Request disconnect, revoke/End, and the grant's fixed expiry all stop provider work.
	// A later renewal extends future calls only, never an already unbounded stream.
	ctx, cancel := context.WithDeadline(r.Context(), selected.expires)
	stop := context.AfterFunc(selected.ctx, cancel)
	s.mu.Unlock()
	defer cancel()
	defer stop()
	r = r.Clone(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024*1024)
	binding.Provider.Complete(w, r, binding.Model)
}
