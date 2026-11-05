package server

import (
	"errors"
	"net/http"
	"os"

	"github.com/wanglongan587/cloud/v2-server/internal/pluginmarket"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func (h *handler) pluginCatalog(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, space := r.PathValue("tid"), r.PathValue("sid")
	if !squadUUID.MatchString(tenant) || !squadUUID.MatchString(space) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	items, syncedAt, e := h.store.PluginCatalog(r.Context(), tenant, space, u.ID)
	if e != nil {
		pluginError(w, "plugin catalog", e)
		return
	}
	body := map[string]any{"items": items}
	if !syncedAt.IsZero() {
		body["syncedAt"] = syncedAt
	} else {
		body["syncedAt"] = nil
	}
	writeJSON(w, http.StatusOK, body)
}

func (h *handler) spacePlugins(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, space := r.PathValue("tid"), r.PathValue("sid")
	if !squadUUID.MatchString(tenant) || !squadUUID.MatchString(space) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	items, e := h.store.SpacePlugins(r.Context(), tenant, space, u.ID)
	if e != nil {
		pluginError(w, "space plugins", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *handler) installSpacePlugin(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, space := r.PathValue("tid"), r.PathValue("sid")
	if !squadUUID.MatchString(tenant) || !squadUUID.MatchString(space) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var body struct {
		Identifier    string `json:"identifier"`
		PluginVersion string `json:"pluginVersion"`
		Version       *int64 `json:"version"`
	}
	if !decodeStrict(w, r, &body) {
		return
	}
	if body.Identifier == "" {
		fault(w, http.StatusBadRequest, "invalid_plugin_id")
		return
	}
	var version int64
	if body.Version != nil {
		if *body.Version < 0 {
			fault(w, http.StatusBadRequest, "invalid_version")
			return
		}
		version = *body.Version
	}
	in := store.PluginInstallInput{
		Identifier: body.Identifier, PluginVersion: body.PluginVersion, Version: version,
	}
	source, e := h.store.PluginPackageSource(r.Context(), tenant, space, u.ID, in.Identifier)
	if e != nil {
		pluginError(w, "install plugin", e)
		return
	}
	if in.PluginVersion != "" && in.PluginVersion != source.Entry.Version {
		pluginError(w, "install plugin", store.ErrPluginVersionUnavailable)
		return
	}
	var resource store.SpacePlugin
	if source.Entry.Kind == "workflow" {
		if h.opts.ValidateWorkflow == nil {
			fault(w, http.StatusServiceUnavailable, "workflow_validation_unavailable")
			return
		}
		download := h.opts.DownloadPluginPackage
		if download == nil {
			download = pluginmarket.Download
		}
		file, e := download(r.Context(), source.Locator, source.Digest, source.Retrieval, source.Credentials)
		if e != nil {
			pluginPackageError(w, e)
			return
		}
		defer os.Remove(file)
		documents, e := pluginmarket.VerifyWorkflowArchive(file, in.Identifier, source.Entry.Version)
		if e != nil {
			fault(w, http.StatusBadRequest, "plugin_package_invalid")
			return
		}
		resource, e = h.store.InstallWorkflowPlugin(
			r.Context(), tenant, space, u.ID, r.URL.Path, key, in, documents, h.opts.ValidateWorkflow,
		)
	} else {
		resource, e = h.store.InstallSpacePlugin(
			r.Context(), tenant, space, u.ID, r.URL.Path, key, in,
		)
	}
	if e != nil {
		pluginError(w, "install plugin", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resource": resource})
}

func (h *handler) removeSpacePlugin(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, space := r.PathValue("tid"), r.PathValue("sid")
	if !squadUUID.MatchString(tenant) || !squadUUID.MatchString(space) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var body struct {
		Identifier string `json:"identifier"`
		Version    *int64 `json:"version"`
	}
	if !decodeStrict(w, r, &body) {
		return
	}
	if body.Identifier == "" {
		fault(w, http.StatusBadRequest, "invalid_plugin_id")
		return
	}
	if body.Version == nil {
		fault(w, http.StatusPreconditionRequired, "version_required")
		return
	}
	if *body.Version < 0 {
		fault(w, http.StatusBadRequest, "invalid_version")
		return
	}
	resource, e := h.store.RemoveSpacePlugin(r.Context(), tenant, space, u.ID, r.URL.Path, key, store.PluginRemoveInput{
		Identifier: body.Identifier, Version: *body.Version,
	})
	if e != nil {
		pluginError(w, "remove plugin", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resource": resource})
}

func pluginError(w http.ResponseWriter, operation string, e error) {
	switch {
	case errors.Is(e, store.ErrForbidden):
		fault(w, http.StatusForbidden, "forbidden")
	case errors.Is(e, store.ErrInvalidPluginID):
		fault(w, http.StatusBadRequest, "invalid_plugin_id")
	case errors.Is(e, store.ErrPluginNotFound):
		fault(w, http.StatusNotFound, "plugin_not_found")
	case errors.Is(e, store.ErrPluginVersionUnavailable):
		fault(w, http.StatusBadRequest, "plugin_version_unavailable")
	case errors.Is(e, store.ErrPluginNotInstalled):
		fault(w, http.StatusNotFound, "plugin_not_installed")
	case errors.Is(e, store.ErrPluginKindNotInstallable):
		fault(w, http.StatusBadRequest, "plugin_kind_not_installable")
	case errors.Is(e, store.ErrPluginRuntimeManaged):
		fault(w, http.StatusConflict, "plugin_runtime_managed")
	case errors.Is(e, store.ErrPluginWorkflowInvalid):
		fault(w, http.StatusBadRequest, "plugin_workflow_invalid")
	case errors.Is(e, store.ErrPluginPackageCredentials):
		fault(w, http.StatusServiceUnavailable, "market_credentials_unavailable")
	case errors.Is(e, store.ErrConflict):
		fault(w, http.StatusConflict, "version_conflict")
	case errors.Is(e, store.ErrIdempotencyConflict):
		fault(w, http.StatusConflict, "idempotency_conflict")
	default:
		internal(w, operation, e)
	}
}

func pluginPackageError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, pluginmarket.ErrSource):
		fault(w, http.StatusBadRequest, "plugin_package_source_invalid")
	case errors.Is(e, pluginmarket.ErrCredentials):
		fault(w, http.StatusServiceUnavailable, "market_credentials_unavailable")
	case errors.Is(e, pluginmarket.ErrPackage):
		fault(w, http.StatusBadRequest, "plugin_package_invalid")
	case errors.Is(e, pluginmarket.ErrTransient):
		fault(w, http.StatusServiceUnavailable, "plugin_package_unavailable")
	default:
		fault(w, http.StatusServiceUnavailable, "plugin_package_unavailable")
	}
}
