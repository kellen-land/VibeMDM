package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
	"github.com/vibemdm/vibemdm/internal/webhook"
)

// lapsOverview returns the policy and every in-scope Mac's status (no passwords).
func (h *Handler) lapsOverview(w http.ResponseWriter, r *http.Request) {
	cfg, rows, sum, err := h.st.LAPSOverview(r.Context())
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	writeJSON(w, 200, map[string]any{"config": cfg, "computers": rows, "summary": sum})
}

// saveLAPS validates and stores the LAPS policy.
func (h *Handler) saveLAPS(w http.ResponseWriter, r *http.Request) {
	var req model.LAPSConfig
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cfg, err := h.st.SaveLAPSConfig(r.Context(), req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.st.Audit(r.Context(), actor(r), "laps.config", "settings", "", map[string]any{"enabled": cfg.Enabled, "account": cfg.AccountName,
		"rotation_days": cfg.RotationDays, "rotate_after_use": cfg.RotateAfterUse})
	h.lapsOverview(w, r)
}

// computerLAPS returns one computer's LAPS status (never a password).
func (h *Handler) computerLAPS(w http.ResponseWriter, r *http.Request) {
	c, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	st, err := h.st.LAPSStatusFor(r.Context(), c)
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	writeJSON(w, 200, st)
}

// revealLAPS returns the decrypted password. Admin-only; a reason is required;
// audited and announced by webhook. The policy may schedule a rotation after use.
func (h *Handler) revealLAPS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	decode(r, &req)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		writeErr(w, 400, "a reason is required")
		return
	}
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	p := auth.FromContext(r.Context())
	pw, st, err := h.st.RevealLAPS(r.Context(), comp.ID, p.Username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "no local administrator password is escrowed for this computer")
			return
		}
		h.fail(w, err, "laps password")
		return
	}
	h.st.Audit(r.Context(), p.Username, "laps.password_viewed", "computer", comp.ID,
		map[string]any{"reason": req.Reason, "ip": auth.ClientIP(r), "fingerprint": st.Fingerprint, "rotate_after": st.RotateAfter})
	h.hooks.Fire(webhook.LAPSPasswordViewed, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "serial": comp.SerialNumber, "actor": p.Username, "reason": req.Reason})
	h.log.Info("LAPS password viewed", "computer", comp.Name, "actor", p.Username)
	w.Header().Set("Cache-Control", "no-store")
	out := map[string]any{"account": st.Account, "password": pw.Current, "status": st}
	if pw.Pending != "" {
		// The agent staged this but never confirmed it: it may be the live password.
		out["unconfirmed_password"] = pw.Pending
	}
	writeJSON(w, 200, out)
}

// rotateLAPS asks the agent to rotate now.
func (h *Handler) rotateLAPS(w http.ResponseWriter, r *http.Request) {
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	if err := h.st.LAPSRequestRotation(r.Context(), comp.ID); err != nil {
		h.fail(w, err, "laps")
		return
	}
	if _, err := h.queueTask(r, comp, "laps_rotate", "", nil, "", "", "", ""); err != nil {
		h.fail(w, err, "task")
		return
	}
	h.st.Audit(r.Context(), actor(r), "laps.rotate_requested", "computer", comp.ID, nil)
	h.computerLAPS(w, r)
}
