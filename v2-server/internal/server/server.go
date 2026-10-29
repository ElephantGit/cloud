// Package server is v2-server's HTTP surface: the Gateway's browser login routes, simplified to a
// development login, and the subset of Cloud's api/openapi.json that v2 uses.
package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// SessionCookie is the Gateway's development cookie name, so the frontend sees the same session.
const SessionCookie = "ora_session"

// Store is what the handlers need from the database.
type Store interface {
	Ping(ctx context.Context) error
	Login(ctx context.Context, who store.Identity, ttl time.Duration) (string, error)
	Logout(ctx context.Context, token string) error
	SessionUser(ctx context.Context, token string) (store.User, error)
	Tenants(ctx context.Context, userID string) ([]store.Tenant, error)
}

// Options tune the server.
type Options struct {
	// SessionTTL is how long a login lasts.
	SessionTTL time.Duration
	// DemoUser is the identity the development login signs in as.
	DemoUser store.Identity
}

// New returns the handler for every route v2-server serves.
func New(s Store, o Options) http.Handler {
	h := &handler{store: s, opts: o}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /auth/providers", h.providers)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/logout", h.logout)
	mux.HandleFunc("GET /api/v1/me", h.authed(h.me))
	mux.HandleFunc("GET /api/v1/me/tenants", h.authed(h.tenants))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { fault(w, http.StatusNotFound, "not_found") })
	return mux
}

type handler struct {
	store Store
	opts  Options
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	if e := h.store.Ping(r.Context()); e != nil {
		slog.Error("health", "error", e)
		fault(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"dependencies": map[string]string{"objectStore": "unconfigured", "gitServer": "unconfigured"},
	})
}

// providers matches the Gateway: v2-server offers only the development login.
func (h *handler) providers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"providers": []string{"dev"}, "default": "dev"})
}

// login is the Gateway's POST /auth/login with the provider round trip skipped: the development
// login signs in at once and sends the browser straight back to returnTo.
func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Provider string `json:"provider"`
		ReturnTo string `json:"returnTo"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&in); e != nil {
		fault(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if in.Provider != "" && in.Provider != "dev" {
		fault(w, http.StatusBadRequest, "unknown_provider")
		return
	}
	token, e := h.store.Login(r.Context(), h.opts.DemoUser, h.opts.SessionTTL)
	if e != nil {
		internal(w, "login", e)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		MaxAge: int(h.opts.SessionTTL / time.Second),
	})
	writeJSON(w, http.StatusOK, map[string]string{"authorizationUrl": localPath(in.ReturnTo)})
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie(SessionCookie); e == nil {
		if e := h.store.Logout(r.Context(), c.Value); e != nil {
			internal(w, "logout", e)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// authed resolves the session cookie to a user, answering 401 like Cloud when there is none.
func (h *handler) authed(next func(http.ResponseWriter, *http.Request, store.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie(SessionCookie)
		if e != nil {
			fault(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		u, e := h.store.SessionUser(r.Context(), c.Value)
		if errors.Is(e, store.ErrNotFound) {
			fault(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		if e != nil {
			internal(w, "session", e)
			return
		}
		next(w, r, u)
	}
}

func (h *handler) me(w http.ResponseWriter, _ *http.Request, u store.User) {
	writeJSON(w, http.StatusOK, u)
}

// tenants returns every membership in one page; v2 users belong to a handful of tenants.
func (h *handler) tenants(w http.ResponseWriter, r *http.Request, u store.User) {
	items, e := h.store.Tenants(r.Context(), u.ID)
	if e != nil {
		internal(w, "tenants", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": ""})
}

// localPath keeps a post-login redirect on this origin.
func localPath(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") {
		return "/"
	}
	return p
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if e := json.NewEncoder(w).Encode(v); e != nil {
		slog.Error("write response", "error", e)
	}
}

// fault writes Cloud's Error shape.
func fault(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"code": code, "params": map[string]any{}, "requestId": requestID()})
}

func internal(w http.ResponseWriter, op string, e error) {
	slog.Error(op, "error", e)
	fault(w, http.StatusInternalServerError, "internal")
}

func requestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
