package gateway

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// previewJoinLink is reached only after relay's same-origin check for the exact
// POST route. It may attach a resolved identity, never a browser-supplied token.
func (h *handler) previewJoinLink(c *gin.Context) {
	if !h.Limiter.Allow(clientKey(c.Request)) {
		h.fail(c, fault{"rate_limited", http.StatusTooManyRequests})
		return
	}
	credentials := Credentials{}
	if cookie, err := c.Request.Cookie(h.Cookies.SessionCookieName()); err == nil && cookie.Value != "" {
		session, err := h.Store.Resolve(c.Request.Context(), cookie.Value)
		if err != nil && !errors.Is(err, ErrNotFound) {
			h.fail(c, fault{"session_unavailable", http.StatusServiceUnavailable})
			return
		}
		if err == nil {
			credentials, err = h.Issuer.Issue(session.Identity)
			if err != nil {
				h.fail(c, fault{"credential_unavailable", http.StatusServiceUnavailable})
				return
			}
		} else {
			http.SetCookie(c.Writer, h.Cookies.ClearSessionCookie())
		}
	}
	if credentials.Service == "" {
		var err error
		credentials.Service, err = h.Issuer.issueService()
		if err != nil {
			h.fail(c, fault{"credential_unavailable", http.StatusServiceUnavailable})
			return
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.UpstreamTimeout)
	defer cancel()
	out := c.Request.Clone(ctx)
	out.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	for _, name := range []string{"Authorization", "X-Ora-User-Token", "Cookie", "Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip"} {
		out.Header.Del(name)
	}
	out.Header.Set("Authorization", "Bearer "+credentials.Service)
	if credentials.User != "" {
		out.Header.Set("X-Ora-User-Token", credentials.User)
	}
	c.Header("Cache-Control", "no-store")
	h.proxy.ServeHTTP(c.Writer, out, func(_ error) {
		h.fail(c, fault{"upstream_unavailable", http.StatusBadGateway})
	}, func() {})
}
