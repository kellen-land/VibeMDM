package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/health"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

// healthGroup aggregates the findings of one check for the console.
type healthGroup struct {
	CheckID   string                `json:"check_id"`
	Title     string                `json:"title"`
	Category  string                `json:"category"`
	Severity  string                `json:"severity"` // worst open severity
	Count     int                   `json:"count"`
	Open      int                   `json:"open"`
	Samples   []model.HealthFinding `json:"samples"`
	FirstSeen time.Time             `json:"first_seen"`
}

func (h *Handler) decorate(fs []model.HealthFinding) []model.HealthFinding {
	for i := range fs {
		if c, ok := h.health.Registry().Get(fs[i].CheckID); ok {
			fs[i].CheckTitle, fs[i].Category = c.Title, c.Category
		}
	}
	return fs
}

// healthSummary returns counts and the grouped issue list (open + acknowledged;
// snoozed and info-level items are counted but kept out of the headline).
func (h *Handler) healthSummary(w http.ResponseWriter, r *http.Request) {
	fs, err := h.st.ListHealthFindings(r.Context(), store.HealthFilter{})
	if err != nil {
		h.fail(w, err, "health")
		return
	}
	h.decorate(fs)
	counts := map[string]int{"critical": 0, "warning": 0, "info": 0, "acknowledged": 0, "snoozed": 0}
	groups := map[string]*healthGroup{}
	for _, f := range fs {
		switch f.State {
		case "snoozed":
			counts["snoozed"]++
			continue
		case "acknowledged":
			counts["acknowledged"]++
		default:
			counts[f.Severity]++
		}
		g := groups[f.CheckID]
		if g == nil {
			g = &healthGroup{CheckID: f.CheckID, Title: f.CheckTitle, Category: f.Category, Severity: health.Info, FirstSeen: f.FirstSeen, Samples: []model.HealthFinding{}}
			groups[f.CheckID] = g
		}
		g.Count++
		if f.State == "open" {
			g.Open++
			if health.SeverityRank(f.Severity) < health.SeverityRank(g.Severity) {
				g.Severity = f.Severity
			}
		}
		if f.FirstSeen.Before(g.FirstSeen) {
			g.FirstSeen = f.FirstSeen
		}
		if len(g.Samples) < 3 {
			g.Samples = append(g.Samples, f)
		}
	}
	out := make([]healthGroup, 0, len(groups))
	for _, g := range groups {
		if g.Open == 0 {
			g.Severity = "acknowledged"
		}
		out = append(out, *g)
	}
	rank := func(s string) int {
		if s == "acknowledged" {
			return 3
		}
		return health.SeverityRank(s)
	}
	sort.Slice(out, func(i, j int) bool {
		if rank(out[i].Severity) != rank(out[j].Severity) {
			return rank(out[i].Severity) < rank(out[j].Severity)
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Title < out[j].Title
	})
	checks, err := h.health.Checks(r.Context())
	if err != nil {
		h.fail(w, err, "health")
		return
	}
	enabled, failing := 0, []map[string]string{}
	for _, c := range checks {
		if c.Enabled {
			enabled++
		}
		if c.LastRunError != "" {
			failing = append(failing, map[string]string{"check_id": c.ID, "title": c.Title, "error": c.LastRunError})
		}
	}
	var lastRun *time.Time
	if t := h.health.LastRun(); !t.IsZero() {
		lastRun = &t
	}
	writeJSON(w, 200, map[string]any{"counts": counts, "groups": out, "last_run": lastRun,
		"checks_enabled": enabled, "checks_total": len(checks), "check_errors": failing})
}

func (h *Handler) listHealthFindings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.HealthFilter{Severity: q.Get("severity"), CheckID: q.Get("check"), ObjectType: q.Get("object_type"), ObjectID: q.Get("object_id"), Limit: qInt(r, "limit", 1000)}
	if s := q.Get("state"); s != "" {
		f.States = strings.Split(s, ",")
	}
	fs, err := h.st.ListHealthFindings(r.Context(), f)
	if err != nil {
		h.fail(w, err, "health")
		return
	}
	writeJSON(w, 200, listResp[model.HealthFinding]{Items: h.decorate(fs), Total: len(fs)})
}

func (h *Handler) getHealthFinding(w http.ResponseWriter, r *http.Request) {
	f, err := h.st.GetHealthFinding(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "finding")
		return
	}
	events, err := h.st.HealthEvents(r.Context(), f.ID, 100)
	if err != nil {
		h.fail(w, err, "finding")
		return
	}
	var desc string
	if c, ok := h.health.Registry().Get(f.CheckID); ok {
		desc = c.Description
	}
	writeJSON(w, 200, map[string]any{"finding": h.decorate([]model.HealthFinding{*f})[0], "events": events, "check_description": desc})
}

type healthStateReq struct {
	State       string  `json:"state"` // acknowledged | snoozed | open
	SnoozeHours float64 `json:"snooze_hours"`
}

func (h *Handler) setHealthState(ctx context.Context, id, who string, req healthStateReq) (*model.HealthFinding, error) {
	return h.health.SetState(ctx, id, req.State, who, time.Duration(req.SnoozeHours*float64(time.Hour)))
}

func (h *Handler) healthFindingState(w http.ResponseWriter, r *http.Request) {
	var req healthStateReq
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	f, err := h.setHealthState(r.Context(), r.PathValue("id"), actor(r), req)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			h.fail(w, err, "finding")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	h.st.Audit(r.Context(), actor(r), "health."+req.State, "health_finding", f.ID, map[string]any{"check": f.CheckID, "object": f.ObjectName})
	writeJSON(w, 200, f)
}

func (h *Handler) remediateHealthFinding(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RemediationID string `json:"remediation_id"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id := r.PathValue("id")
	label, err := h.health.Remediate(r.Context(), id, req.RemediationID, actor(r))
	switch {
	case errors.Is(err, health.ErrAlreadyResolved):
		writeJSON(w, 200, map[string]any{"resolved": true, "message": "The condition has already cleared; the finding was resolved."})
		return
	case errors.Is(err, store.ErrNotFound):
		h.fail(w, err, "finding")
		return
	case err != nil:
		writeErr(w, 422, err.Error())
		return
	}
	h.st.Audit(r.Context(), actor(r), "health.remediate", "health_finding", id, map[string]any{"remediation": req.RemediationID})
	writeJSON(w, 200, map[string]any{"resolved": false, "message": label + " — done."})
}

// bulkHealth applies a state change or a remediation to many findings.
func (h *Handler) bulkHealth(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs           []string `json:"ids"`
		Action        string   `json:"action"` // acknowledge | snooze | reopen | remediate
		SnoozeHours   float64  `json:"snooze_hours"`
		RemediationID string   `json:"remediation_id"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > 1000 {
		writeErr(w, 400, "between 1 and 1000 ids required")
		return
	}
	ok, failed := 0, map[string]string{}
	for _, id := range req.IDs {
		var err error
		switch req.Action {
		case "acknowledge":
			_, err = h.setHealthState(r.Context(), id, actor(r), healthStateReq{State: "acknowledged"})
		case "snooze":
			_, err = h.setHealthState(r.Context(), id, actor(r), healthStateReq{State: "snoozed", SnoozeHours: req.SnoozeHours})
		case "reopen":
			_, err = h.setHealthState(r.Context(), id, actor(r), healthStateReq{State: "open"})
		case "remediate":
			_, err = h.health.Remediate(r.Context(), id, req.RemediationID, actor(r))
			if errors.Is(err, health.ErrAlreadyResolved) {
				err = nil
			}
		default:
			writeErr(w, 400, "unknown action")
			return
		}
		if err != nil {
			failed[id] = err.Error()
		} else {
			ok++
		}
	}
	h.st.Audit(r.Context(), actor(r), "health.bulk_"+req.Action, "health_finding", "", map[string]any{"count": len(req.IDs), "succeeded": ok})
	writeJSON(w, 200, map[string]any{"succeeded": ok, "failed": failed})
}

func (h *Handler) listHealthChecks(w http.ResponseWriter, r *http.Request) {
	checks, err := h.health.Checks(r.Context())
	if err != nil {
		h.fail(w, err, "health")
		return
	}
	writeJSON(w, 200, checks)
}

func (h *Handler) updateHealthCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, ok := h.health.Registry().Get(id)
	if !ok {
		writeErr(w, 404, "check not found")
		return
	}
	var req struct {
		Enabled  *bool           `json:"enabled"`
		AutoHeal json.RawMessage `json:"auto_heal"` // true | false | null (reset to default)
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cur, err := h.st.HealthCheckSettings(r.Context())
	if err != nil {
		h.fail(w, err, "health")
		return
	}
	st, has := cur[id]
	if !has {
		st = model.HealthCheckSetting{Enabled: true}
	}
	if req.Enabled != nil {
		st.Enabled = *req.Enabled
	}
	if len(req.AutoHeal) > 0 {
		var v *bool
		if err := json.Unmarshal(req.AutoHeal, &v); err != nil {
			writeErr(w, 400, "auto_heal must be true, false or null")
			return
		}
		if v != nil && *v && !c.SupportsHeal {
			writeErr(w, 400, "this check has no automatic remediation")
			return
		}
		st.AutoHeal = v
	}
	if err := h.st.SaveHealthCheckSetting(r.Context(), id, st); err != nil {
		h.fail(w, err, "health")
		return
	}
	h.st.Audit(r.Context(), actor(r), "health.check_settings", "health_check", id, map[string]any{"enabled": st.Enabled, "auto_heal": st.AutoHeal})
	// Apply right away (disabling closes findings; enabling evaluates).
	h.health.RunCheck(r.Context(), id)
	h.listHealthChecks(w, r)
}

func (h *Handler) runHealthCheck(w http.ResponseWriter, r *http.Request) {
	if err := h.health.RunCheck(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "check not found")
			return
		}
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (h *Handler) runHealthAll(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	h.health.RunAll(ctx)
	h.healthSummary(w, r)
}

func (h *Handler) listHealthEvents(w http.ResponseWriter, r *http.Request) {
	ev, err := h.st.HealthEvents(r.Context(), "", qInt(r, "limit", 100))
	if err != nil {
		h.fail(w, err, "health")
		return
	}
	writeJSON(w, 200, ev)
}
