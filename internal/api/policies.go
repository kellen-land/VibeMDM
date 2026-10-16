package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/localaccount"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

type policyWithStatus struct {
	model.Policy
	Status map[string]int `json:"status"`
}

func (h *Handler) listPolicies(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListPolicies(r.Context())
	if err != nil {
		h.fail(w, err, "policies")
		return
	}
	out := make([]policyWithStatus, 0, len(items))
	for _, p := range items {
		st, _ := h.st.PolicyStatusSummary(r.Context(), p.ID)
		if st == nil {
			st = map[string]int{}
		}
		redactPolicyFor(r, &p)
		out = append(out, policyWithStatus{Policy: p, Status: st})
	}
	writeJSON(w, 200, listResp[policyWithStatus]{Items: out, Total: len(out)})
}

func (h *Handler) getPolicy(w http.ResponseWriter, r *http.Request) {
	p, err := h.st.GetPolicy(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "policy")
		return
	}
	st, _ := h.st.PolicyStatusSummary(r.Context(), p.ID)
	if st == nil {
		st = map[string]int{}
	}
	redactPolicyFor(r, p)
	writeJSON(w, 200, policyWithStatus{Policy: *p, Status: st})
}

// validPolicyApps checks Installomator steps.
// validPolicyAccounts checks the Local Accounts payload. prev is the stored version, whose
// passwords are kept when an entry for the same action and user is sent without one.
func (h *Handler) validPolicyAccounts(ctx context.Context, accounts, prev []model.PolicyAccount) error {
	if len(accounts) > 20 {
		return errors.New("a policy can hold at most 20 local account actions")
	}
	lapsAccount := ""
	if c, err := h.st.GetLAPSConfig(ctx); err == nil && c.Enabled {
		lapsAccount = c.AccountName
	}
	for _, a := range accounts {
		stored := false
		for _, p := range prev {
			if p.Action == a.Action && p.Username == a.Username && p.PasswordSet {
				stored = true
			}
		}
		if err := localaccount.Validate(a, a.Password != "" || stored); err != nil {
			return err
		}
		if lapsAccount != "" && strings.EqualFold(a.Username, lapsAccount) {
			return fmt.Errorf("%s is the LAPS account; manage it under Security → LAPS", a.Username)
		}
	}
	return nil
}

func (h *Handler) validPolicyApps(apps []model.PolicyApp) error {
	for _, a := range apps {
		if a.AppID != "" {
			if _, err := h.st.GetMacApp(context.Background(), a.AppID); err != nil {
				return fmt.Errorf("the Mac App %q does not exist", a.AppID)
			}
			continue
		}
		if !h.validAppLabel(a.Label) {
			return fmt.Errorf("unknown Installomator label %q", a.Label)
		}
		if err := validOptions(a.Options); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) createPolicy(w http.ResponseWriter, r *http.Request) {
	var p model.Policy
	if err := decode(r, &p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := h.validPolicyApps(p.Apps); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := h.validPolicyAccounts(r.Context(), p.Accounts, nil); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(p.Name) == "" {
		writeErr(w, 400, "name required")
		return
	}
	out, err := h.st.CreatePolicy(r.Context(), p)
	if err != nil {
		h.fail(w, err, "policy")
		return
	}
	h.st.Audit(r.Context(), actor(r), "policy.create", "policy", out.ID, map[string]any{"name": out.Name})
	h.wakeScope(out.Scope, out.Enabled)
	writeJSON(w, 201, out)
}

func (h *Handler) updatePolicy(w http.ResponseWriter, r *http.Request) {
	var p model.Policy
	if err := decode(r, &p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p.ID = r.PathValue("id")
	if err := h.validPolicyApps(p.Apps); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	existing, err := h.st.GetPolicy(r.Context(), p.ID)
	if err != nil {
		h.fail(w, err, "policy")
		return
	}
	if err := h.validPolicyAccounts(r.Context(), p.Accounts, existing.Accounts); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	out, err := h.st.UpdatePolicy(r.Context(), p)
	if err != nil {
		h.fail(w, err, "policy")
		return
	}
	h.st.Audit(r.Context(), actor(r), "policy.update", "policy", out.ID, map[string]any{"name": out.Name})
	h.wakeScope(out.Scope, out.Enabled)
	writeJSON(w, 200, out)
}

func (h *Handler) deletePolicy(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeletePolicy(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "policy")
		return
	}
	h.st.Audit(r.Context(), actor(r), "policy.delete", "policy", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) policyLogs(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListPolicyLogs(r.Context(), store.PolicyLogFilter{PolicyID: r.PathValue("id"), Status: r.URL.Query().Get("status"), Limit: qInt(r, "limit", 50), Offset: qInt(r, "offset", 0)})
	if err != nil {
		h.fail(w, err, "policy logs")
		return
	}
	writeJSON(w, 200, listResp[model.PolicyLog]{Items: items, Total: total})
}

func (h *Handler) allPolicyLogs(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListPolicyLogs(r.Context(), store.PolicyLogFilter{Status: r.URL.Query().Get("status"), Limit: qInt(r, "limit", 50), Offset: qInt(r, "offset", 0)})
	if err != nil {
		h.fail(w, err, "policy logs")
		return
	}
	writeJSON(w, 200, listResp[model.PolicyLog]{Items: items, Total: total})
}

func (h *Handler) getPolicyLog(w http.ResponseWriter, r *http.Request) {
	l, err := h.st.GetPolicyLog(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "policy log")
		return
	}
	writeJSON(w, 200, l)
}

func (h *Handler) flushPolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ComputerID string `json:"computer_id"`
	}
	decode(r, &req)
	n, err := h.st.FlushPolicyLogs(r.Context(), r.PathValue("id"), req.ComputerID)
	if err != nil {
		h.fail(w, err, "flush")
		return
	}
	h.st.Audit(r.Context(), actor(r), "policy.flush", "policy", r.PathValue("id"), map[string]any{"computer_id": req.ComputerID, "deleted": n})
	if p, err := h.st.GetPolicy(r.Context(), r.PathValue("id")); err == nil {
		if req.ComputerID != "" {
			h.notifier.RequestCheckin(req.ComputerID)
		} else {
			h.wakeScope(p.Scope, p.Enabled)
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "deleted": n})
}

// runPolicy queues an immediate run on computers (ignores triggers and frequency).
func (h *Handler) runPolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ComputerIDs []string `json:"computer_ids"`
		GroupIDs    []string `json:"group_ids"`
		Scoped      bool     `json:"scoped"` // run on everything in the policy's scope
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p, err := h.st.GetPolicy(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "policy")
		return
	}
	var targets []string
	if req.Scoped {
		targets, err = h.st.ResolveScope(r.Context(), p.Scope)
	} else {
		targets, err = h.st.ResolveScope(r.Context(), model.Scope{Targets: model.ScopeSelection{ComputerIDs: req.ComputerIDs, GroupIDs: req.GroupIDs}})
	}
	if err != nil {
		h.fail(w, err, "scope")
		return
	}
	queued, skipped := 0, []string{}
	for _, id := range targets {
		c, err := h.st.GetComputer(r.Context(), id)
		if err != nil {
			continue
		}
		if _, err := h.queueTask(r, c, "run_policy", "", nil, "", p.ID, "", ""); err != nil {
			skipped = append(skipped, c.Name+": "+err.Error())
			continue
		}
		queued++
	}
	writeJSON(w, 200, map[string]any{"queued": queued, "skipped": skipped})
}

// wakeScope asks every agent in scope to check in now so changes apply on demand.
func (h *Handler) wakeScope(sc model.Scope, enabled bool) {
	if !enabled {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		ids, err := h.st.ResolveScope(ctx, sc)
		if err != nil {
			h.log.Warn("resolving scope for wake", "err", err)
			return
		}
		h.notifier.RequestCheckin(ids...)
	}()
}
