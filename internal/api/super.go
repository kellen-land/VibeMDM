package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/schema"
	"github.com/vibemdm/vibemdm/internal/store"
	"github.com/vibemdm/vibemdm/internal/superwf"
)

func (h *Handler) superView(r *http.Request) (map[string]any, error) {
	cfg, _, err := h.st.GetSuperConfig(r.Context())
	if err != nil {
		return nil, err
	}
	cfg.Version = superwf.Version
	values := map[string]any{}
	if cfg.ProfileID != nil {
		if p, err := h.st.GetProfile(r.Context(), *cfg.ProfileID); err == nil && p.SchemaValues != nil {
			values = p.SchemaValues
		}
	}
	return map[string]any{"config": cfg, "values": values, "schema_id": superwf.SchemaRef, "version": superwf.Version,
		"script_url": superwf.ScriptURL, "sha256": superwf.SHA256, "license": "Apache-2.0", "project": "https://github.com/Macjutsu/super"}, nil
}

func (h *Handler) getSuper(w http.ResponseWriter, r *http.Request) {
	v, err := h.superView(r)
	if err != nil {
		h.fail(w, err, "super")
		return
	}
	writeJSON(w, 200, v)
}

// saveSuper stores the deployment and (re)generates the preferences profile.
func (h *Handler) saveSuper(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled      bool           `json:"enabled"`
		Scope        model.Scope    `json:"scope"`
		AuthMode     string         `json:"auth_mode"`
		AuthAccount  string         `json:"auth_account"`
		AuthPassword *string        `json:"auth_password"` // omit to keep, "" to clear
		Values       map[string]any `json:"values"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	switch req.AuthMode {
	case "", "none":
		req.AuthMode, req.AuthAccount = "none", ""
		empty := ""
		req.AuthPassword = &empty
	case "local", "service":
		if strings.TrimSpace(req.AuthAccount) == "" {
			writeErr(w, 400, "an account name is required for "+req.AuthMode+" authentication")
			return
		}
	default:
		writeErr(w, 400, "auth_mode must be none, local or service")
		return
	}
	b, ok := schema.BuiltinByDomain("com.macjutsu.super")
	if !ok {
		writeErr(w, 500, "super schema missing")
		return
	}
	clean, errs := b.Schema.Validate(req.Values)
	if len(errs) > 0 {
		writeJSON(w, 400, map[string]any{"error": "some settings are invalid", "errors": errs})
		return
	}
	cur, _, err := h.st.GetSuperConfig(r.Context())
	if err != nil {
		h.fail(w, err, "super")
		return
	}
	settings, _ := h.st.GetSettings(r.Context())
	payload, err := schema.BuildProfile(schema.ProfileInput{Identifier: superwf.ProfileIdentifier, DisplayName: "super (S.U.P.E.R.M.A.N.) settings",
		Description: "Managed by Software Updates → super in VibeMDM.", Organization: settings.OrgName, Domain: "com.macjutsu.super", Values: clean})
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	prof, err := h.upsertBuiltin(r, superwf.ProfileIdentifier, "super (S.U.P.E.R.M.A.N.) settings",
		"Managed by Software Updates → super. Edit it there.", payload, req.Enabled, req.Scope)
	if err != nil {
		h.fail(w, err, "profile")
		return
	}
	cfg := model.SuperConfig{Enabled: req.Enabled, Scope: req.Scope, AuthMode: req.AuthMode, AuthAccount: strings.TrimSpace(req.AuthAccount), ProfileID: cur.ProfileID}
	if prof != nil {
		cfg.ProfileID = &prof.ID
		h.st.SetProfileSchema(r.Context(), prof.ID, superwf.SchemaRef, clean)
	}
	if err := h.st.SaveSuperConfig(r.Context(), cfg, req.AuthPassword); err != nil {
		h.fail(w, err, "super")
		return
	}
	h.st.Audit(r.Context(), actor(r), "super.save", "super", "", map[string]any{"enabled": req.Enabled, "auth_mode": req.AuthMode})
	if req.Enabled {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			h.superSvc.Reconcile(ctx)
		}()
	}
	h.getSuper(w, r)
}

// superStatus lists in-scope Macs with their super state.
func (h *Handler) superStatus(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := h.st.GetSuperConfig(r.Context())
	if err != nil {
		h.fail(w, err, "super")
		return
	}
	ids, err := h.st.ResolveScope(r.Context(), cfg.Scope)
	if err != nil {
		h.fail(w, err, "scope")
		return
	}
	rows, err := h.st.SuperStatus(r.Context(), ids)
	if err != nil {
		h.fail(w, err, "super")
		return
	}
	summary := map[string]int{"targeted": len(rows), "current": 0, "outdated": 0, "missing": 0}
	type row struct {
		store.SuperStatusRow
		State string `json:"state"` // current | outdated | missing
	}
	out := make([]row, 0, len(rows))
	for _, x := range rows {
		st := "missing"
		switch v := superwf.InstalledVersion(x.Super); {
		case v == superwf.Version:
			st = "current"
		case v != "":
			st = "outdated"
		}
		summary[st]++
		out = append(out, row{SuperStatusRow: x, State: st})
	}
	writeJSON(w, 200, map[string]any{"summary": summary, "computers": out, "version": superwf.Version, "enabled": cfg.Enabled})
}

// installSuper queues installs now for specific computers (or all missing/outdated in scope).
func (h *Handler) installSuper(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ComputerIDs []string `json:"computer_ids"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(req.ComputerIDs) == 0 {
		writeJSON(w, 200, map[string]any{"queued": h.superSvc.Reconcile(r.Context())})
		return
	}
	queued, skipped := 0, map[string]string{}
	for _, id := range req.ComputerIDs {
		comp, err := h.st.GetComputer(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			skipped[id] = "not found"
			continue
		} else if err != nil {
			h.fail(w, err, "computer")
			return
		}
		if err := h.superSvc.QueueInstall(r.Context(), comp, actor(r)); err != nil {
			skipped[comp.Name] = err.Error()
			continue
		}
		queued++
	}
	h.st.Audit(r.Context(), actor(r), "super.install", "super", "", map[string]any{"queued": queued})
	writeJSON(w, 200, map[string]any{"queued": queued, "skipped": skipped})
}
