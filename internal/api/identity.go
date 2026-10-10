package api

import (
	"net/http"
	"strings"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/schema"
)

// XCreds (Twocanoes, BSD-3-Clause) integration: cloud identity at the login
// window. VibeMDM installs XCreds with the Installomator "xcreds" label and
// configures it with a managed-preferences profile built from the bundled schema.
const (
	xcredsProfileID = "com.vibemdm.xcreds"
	xcredsSchemaRef = "builtin:com.twocanoes.xcreds"
	xcredsLabel     = "xcreds"
)

func (h *Handler) xcredsApp(r *http.Request) (*model.MacApp, error) {
	apps, err := h.st.ListMacApps(r.Context())
	if err != nil {
		return nil, err
	}
	for i := range apps {
		if apps[i].Label == xcredsLabel {
			return &apps[i], nil
		}
	}
	return nil, nil
}

func (h *Handler) getXCreds(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"enabled": false, "scope": model.Scope{}, "values": map[string]any{}, "schema_id": xcredsSchemaRef,
		"profile_id": nil, "app": nil, "license": "BSD-3-Clause", "project": "https://github.com/twocanoes/xcreds"}
	if p, err := h.st.GetProfileByIdentifier(r.Context(), xcredsProfileID); err == nil {
		redactProfileFor(r, p)
		vals := p.SchemaValues
		if lic, ok := vals["LicenseFile"]; ok && lic != nil {
			vals["LicenseFile"] = "" // never echo the license; report presence instead
			out["has_license"] = true
		}
		out["values"], out["profile_id"], out["scope"] = vals, p.ID, p.Scope
		out["enabled"] = p.Scope.AllComputers || !p.Scope.Targets.IsEmpty()
	}
	if app, err := h.xcredsApp(r); err == nil && app != nil {
		st, _ := h.st.AppStatusSummary(r.Context(), app.ID)
		out["app"] = map[string]any{"id": app.ID, "enabled": app.Enabled, "status": st}
	}
	writeJSON(w, 200, out)
}

// saveXCreds turns the integration on or off for a scope: the XCreds app and its
// settings profile always share the same scope.
func (h *Handler) saveXCreds(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled    bool           `json:"enabled"`
		Scope      model.Scope    `json:"scope"`
		Values     map[string]any `json:"values"`
		InstallApp *bool          `json:"install_app"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	install := req.InstallApp == nil || *req.InstallApp
	b, _ := schema.BuiltinByDomain("com.twocanoes.xcreds")
	// An empty LicenseFile means "keep the stored license".
	if lic, _ := req.Values["LicenseFile"].(string); strings.TrimSpace(lic) == "" {
		delete(req.Values, "LicenseFile")
		if p, err := h.st.GetProfileByIdentifier(r.Context(), xcredsProfileID); err == nil {
			if old, ok := p.SchemaValues["LicenseFile"]; ok && old != nil {
				req.Values["LicenseFile"] = old
			}
		}
	}
	clean, errs := b.Schema.Validate(req.Values)
	if len(errs) > 0 {
		writeJSON(w, 400, map[string]any{"error": "some settings are invalid", "errors": errs})
		return
	}
	if req.Enabled {
		if _, ok := clean["discoveryURL"]; !ok {
			writeJSON(w, 400, map[string]any{"error": "the OpenID Connect discovery URL is required", "errors": []schema.FieldError{{Path: "discoveryURL", Message: "is required"}}})
			return
		}
		if _, ok := clean["clientID"]; !ok {
			writeJSON(w, 400, map[string]any{"error": "the client ID is required", "errors": []schema.FieldError{{Path: "clientID", Message: "is required"}}})
			return
		}
	}
	settings, _ := h.st.GetSettings(r.Context())
	payload, err := schema.BuildProfile(schema.ProfileInput{Identifier: xcredsProfileID, DisplayName: "XCreds cloud login",
		Description: "Managed by Identity → XCreds in VibeMDM.", Organization: settings.OrgName, Domain: "com.twocanoes.xcreds", Values: clean})
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	prof, err := h.upsertBuiltin(r, xcredsProfileID, "XCreds cloud login", "Managed by Identity → XCreds. Edit it there.", payload, req.Enabled, req.Scope)
	if err != nil {
		h.fail(w, err, "profile")
		return
	}
	if prof != nil {
		// Store values as JSON (license kept as base64 so it round-trips).
		stored := map[string]any{}
		for k, v := range clean {
			stored[k] = v
		}
		h.st.SetProfileSchema(r.Context(), prof.ID, xcredsSchemaRef, stored)
	}
	app, err := h.xcredsApp(r)
	if err != nil {
		h.fail(w, err, "app")
		return
	}
	appScope := req.Scope
	appEnabled := req.Enabled && install
	switch {
	case app == nil && appEnabled:
		if _, err := h.st.CreateMacApp(r.Context(), model.MacApp{Name: "XCreds", Label: xcredsLabel, Category: "Identity",
			Description: "Cloud identity login window by Twocanoes (installed by Identity → XCreds).", UpdateBehavior: "keep_updated",
			Enabled: true, Scope: appScope}); err != nil {
			h.fail(w, err, "app")
			return
		}
	case app != nil:
		app.Enabled, app.Scope = appEnabled, appScope
		if _, err := h.st.UpdateMacApp(r.Context(), *app); err != nil {
			h.fail(w, err, "app")
			return
		}
	}
	h.wakeScope(appScope, appEnabled)
	h.st.Audit(r.Context(), actor(r), "identity.xcreds_save", "identity", "xcreds", map[string]any{"enabled": req.Enabled, "install_app": install})
	h.getXCreds(w, r)
}
