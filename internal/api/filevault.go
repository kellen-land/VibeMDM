package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/commands"
	"github.com/vibemdm/vibemdm/internal/escrow"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
	"github.com/vibemdm/vibemdm/internal/webhook"
)

type builtinProfileView struct {
	Deployed bool                 `json:"deployed"`
	Profile  *model.ConfigProfile `json:"profile,omitempty"`
	Status   map[string]int       `json:"status"`
}

func (h *Handler) builtinProfile(ctx context.Context, identifier string) builtinProfileView {
	v := builtinProfileView{Status: map[string]int{}}
	p, err := h.st.GetProfileByIdentifier(ctx, identifier)
	if err != nil {
		return v
	}
	p.Payload = ""
	v.Profile = p
	v.Deployed = p.Scope.AllComputers || !p.Scope.Targets.IsEmpty()
	if st, err := h.st.DeploymentStatusCounts(ctx, p.ID); err == nil {
		v.Status = st
	}
	return v
}

// filevaultOverview reports fleet escrow state and the built-in profiles.
func (h *Handler) filevaultOverview(w http.ResponseWriter, r *http.Request) {
	rows, sum, err := h.st.FileVaultOverview(r.Context())
	if err != nil {
		h.fail(w, err, "filevault")
		return
	}
	settings, _ := h.st.GetSettings(r.Context())
	out := map[string]any{
		"summary": sum, "computers": rows, "auto_remediate": settings.FileVaultAutoRemediate,
		"escrow_profile":  h.builtinProfile(r.Context(), escrow.EscrowProfileIdentifier),
		"enforce_profile": h.builtinProfile(r.Context(), escrow.EnforceProfileIdentifier),
	}
	if h.escrow != nil {
		out["escrow_certificate"] = map[string]any{"subject": h.escrow.Cert.Subject.CommonName, "not_after": h.escrow.Cert.NotAfter}
	}
	writeJSON(w, 200, out)
}

// upsertBuiltin creates or updates a built-in profile and re-syncs its scope.
func (h *Handler) upsertBuiltin(r *http.Request, identifier, name, description string, payload []byte, enabled bool, scope model.Scope) (*model.ConfigProfile, error) {
	if !enabled {
		scope = model.Scope{} // keep the record, remove from every Mac
	}
	scope.Normalize()
	existing, err := h.st.GetProfileByIdentifier(r.Context(), identifier)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	p := model.ConfigProfile{Name: name, Description: description, Category: "Security", Identifier: identifier, Payload: string(payload), Scope: scope}
	if err := normalizeProfilePayload(&p); err != nil {
		return nil, err
	}
	var out *model.ConfigProfile
	reinstall := false
	if existing == nil {
		if !enabled {
			return nil, nil
		}
		out, err = h.st.CreateProfile(r.Context(), p)
	} else {
		p.ID = existing.ID
		reinstall = existing.Payload != p.Payload
		out, err = h.st.UpdateProfile(r.Context(), p)
	}
	if err != nil {
		return nil, err
	}
	h.syncProfileAsync(out, actor(r), reinstall)
	return out, nil
}

// filevaultProfiles turns the built-in escrow / enforcement profiles on or off and scopes them.
func (h *Handler) filevaultProfiles(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Escrow *struct {
			Enabled bool        `json:"enabled"`
			Scope   model.Scope `json:"scope"`
		} `json:"escrow"`
		Enforce *struct {
			Enabled bool                  `json:"enabled"`
			Scope   model.Scope           `json:"scope"`
			Options escrow.EnforceOptions `json:"options"`
		} `json:"enforce"`
		AutoRemediate *bool `json:"auto_remediate"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	settings, _ := h.st.GetSettings(r.Context())
	if req.Escrow != nil {
		if h.escrow == nil {
			writeErr(w, 500, "escrow identity not loaded")
			return
		}
		payload, err := h.escrow.EscrowProfile(settings.OrgName)
		if err != nil {
			h.fail(w, err, "escrow profile")
			return
		}
		if _, err := h.upsertBuiltin(r, escrow.EscrowProfileIdentifier, "FileVault Recovery Key Escrow",
			"Escrows each Mac's FileVault personal recovery key to VibeMDM (built-in).", payload, req.Escrow.Enabled, req.Escrow.Scope); err != nil {
			h.fail(w, err, "escrow profile")
			return
		}
		h.st.Audit(r.Context(), actor(r), "filevault.escrow_profile", "profile", escrow.EscrowProfileIdentifier, map[string]any{"enabled": req.Escrow.Enabled})
	}
	if req.Enforce != nil {
		payload, err := escrow.EnforceProfile(settings.OrgName, req.Enforce.Options)
		if err != nil {
			h.fail(w, err, "enforcement profile")
			return
		}
		if _, err := h.upsertBuiltin(r, escrow.EnforceProfileIdentifier, "FileVault Enforcement",
			"Requires FileVault with a personal recovery key (built-in).", payload, req.Enforce.Enabled, req.Enforce.Scope); err != nil {
			h.fail(w, err, "enforcement profile")
			return
		}
		h.st.Audit(r.Context(), actor(r), "filevault.enforce_profile", "profile", escrow.EnforceProfileIdentifier, map[string]any{"enabled": req.Enforce.Enabled})
	}
	if req.AutoRemediate != nil {
		settings.FileVaultAutoRemediate = *req.AutoRemediate
		if err := h.st.SaveSettings(r.Context(), settings); err != nil {
			h.fail(w, err, "settings")
			return
		}
		h.st.Audit(r.Context(), actor(r), "filevault.auto_remediate", "settings", "", map[string]any{"enabled": *req.AutoRemediate})
	}
	h.filevaultOverview(w, r)
}

// computerFileVault returns escrow status for one computer (never the key).
func (h *Handler) computerFileVault(w http.ResponseWriter, r *http.Request) {
	st, err := h.st.FileVaultStatusFor(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	writeJSON(w, 200, st)
}

// revealFileVaultKey returns the decrypted recovery key. Admin-only, audited, webhook-notified.
func (h *Handler) revealFileVaultKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	decode(r, &req)
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	prk, st, err := h.st.GetFileVaultKey(r.Context(), comp.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "no FileVault recovery key is escrowed for this computer")
			return
		}
		h.fail(w, err, "filevault key")
		return
	}
	p := auth.FromContext(r.Context())
	details := map[string]any{"reason": req.Reason, "ip": auth.ClientIP(r), "fingerprint": st.Fingerprint}
	h.st.Audit(r.Context(), p.Username, "filevault.key_viewed", "computer", comp.ID, details)
	h.hooks.Fire(webhook.FileVaultKeyViewed, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "serial": comp.SerialNumber, "actor": p.Username, "reason": req.Reason})
	h.log.Info("FileVault recovery key viewed", "computer", comp.Name, "actor", p.Username)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"key": prk, "status": st})
}

// remediateFileVault queues Escrow Buddy on a Mac to generate and escrow a new key at next login.
func (h *Handler) remediateFileVault(w http.ResponseWriter, r *http.Request) {
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	t, err := h.queueTask(r, comp, "filevault_regenerate", "", nil, "", "", "", "")
	if err != nil {
		h.fail(w, err, "task")
		return
	}
	writeJSON(w, 200, t)
}

// checkMissingKeys asks every Mac with FileVault on but no escrowed key for SecurityInfo now.
func (h *Handler) checkMissingKeys(w http.ResponseWriter, r *http.Request) {
	comps, err := h.st.ComputersNeedingKeyCheck(r.Context(), 0) // manual check: always re-query
	if err != nil {
		h.fail(w, err, "filevault")
		return
	}
	queued := 0
	for i := range comps {
		cmd, err := commands.SecurityInfo()
		if err != nil {
			continue
		}
		if _, err := h.cmdr.Enqueue(r.Context(), &comps[i], cmd, "filevault_check", "", actor(r)); err == nil {
			queued++
		}
	}
	writeJSON(w, 200, map[string]any{"queued": queued})
}
