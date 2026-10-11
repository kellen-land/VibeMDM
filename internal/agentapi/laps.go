package agentapi

import (
	"errors"
	"net/http"

	"github.com/vibemdm/vibemdm/internal/laps"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
	"github.com/vibemdm/vibemdm/internal/webhook"
)

// lapsConfig tells the agent whether (and why) to rotate its local admin password.
func (h *Handler) lapsConfig(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	cfg, err := h.st.LAPSConfigForAgent(r.Context(), comp)
	if err != nil {
		h.log.Error("laps config", "err", err, "computer", comp.Name)
		writeErr(w, 500, "LAPS configuration unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, cfg)
}

// lapsStage stores the password the agent is about to apply. It must happen first:
// if the upload fails the agent leaves the account alone.
func (h *Handler) lapsStage(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var req model.AgentLAPSStage
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	cfg, err := h.st.LAPSConfigForAgent(r.Context(), comp)
	if err != nil || !cfg.Enabled {
		writeErr(w, 403, "LAPS is not enabled for this computer")
		return
	}
	if req.Account != cfg.Account {
		writeErr(w, 400, "account does not match the LAPS policy")
		return
	}
	if len(req.Password) < laps.MinLength || len(req.Password) > laps.MaxLength {
		writeErr(w, 400, "password length is outside the allowed range")
		return
	}
	fp, err := h.st.LAPSStage(r.Context(), comp.ID, req.Account, req.Password)
	if err != nil {
		h.log.Error("laps stage", "err", err, "computer", comp.Name)
		writeErr(w, 500, "could not store the password")
		return
	}
	writeJSON(w, 200, map[string]string{"fingerprint": fp})
}

// lapsCommit resolves a staged password. 409 means nothing matching is staged
// (the agent re-stages and retries).
func (h *Handler) lapsCommit(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var req model.AgentLAPSCommit
	if err := decode(r, &req); err != nil || req.Fingerprint == "" {
		writeErr(w, 400, "invalid request")
		return
	}
	if err := h.st.LAPSCommit(r.Context(), comp.ID, req.Fingerprint, req.Applied, req.Error); err != nil {
		if errors.Is(err, store.ErrLAPSNoPending) {
			writeErr(w, 409, "no matching staged password")
			return
		}
		h.log.Error("laps commit", "err", err, "computer", comp.Name)
		writeErr(w, 500, "could not record the result")
		return
	}
	if req.Applied {
		h.st.Audit(r.Context(), "agent:"+comp.Name, "laps.rotated", "computer", comp.ID, map[string]any{"fingerprint": req.Fingerprint})
		h.hooks.Fire(webhook.LAPSPasswordRotated, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "serial": comp.SerialNumber})
		h.log.Info("LAPS password rotated", "computer", comp.Name)
	} else {
		h.st.Audit(r.Context(), "agent:"+comp.Name, "laps.rotation_failed", "computer", comp.ID, map[string]any{"error": req.Error})
		h.log.Warn("LAPS rotation failed on the Mac", "computer", comp.Name, "error", req.Error)
	}
	w.WriteHeader(http.StatusNoContent)
}

// lapsError records a failure that happened before anything was staged.
func (h *Handler) lapsError(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var req struct {
		Error string `json:"error"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if err := h.st.LAPSReportError(r.Context(), comp.ID, req.Error); err != nil {
		writeErr(w, 500, "could not record the error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
