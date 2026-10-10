package health

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Webhook events fired by the engine.
const (
	EventIssueOpened   = "HealthIssueOpened"
	EventIssueResolved = "HealthIssueResolved"
)

// Firer delivers webhook events.
type Firer interface {
	Fire(event string, payload any)
}

const (
	// flapWindow: a finding that comes back this soon after resolving is
	// reopened silently and keeps its history.
	flapWindow = time.Hour
	// resolveAfterMisses: consecutive clean runs required before a finding
	// resolves on its own (one missed scan never closes an issue).
	resolveAfterMisses = 2
	checkTimeout       = 45 * time.Second
)

// ErrAlreadyResolved is returned when a remediation targets a finding whose
// condition has already cleared.
var ErrAlreadyResolved = errors.New("the condition has already cleared")

// Engine runs checks and maintains findings.
type Engine struct {
	reg   *Registry
	st    *store.Store
	hooks Firer
	log   *slog.Logger

	mu      sync.Mutex // serializes runs
	stateMu sync.RWMutex
	lastRun time.Time
	errs    map[string]string
}

// NewEngine creates an engine.
func NewEngine(reg *Registry, st *store.Store, hooks Firer, logger *slog.Logger) *Engine {
	return &Engine{reg: reg, st: st, hooks: hooks, log: logger, errs: map[string]string{}}
}

// Registry returns the engine's checks.
func (e *Engine) Registry() *Registry { return e.reg }

// LastRun is when the last full run finished.
func (e *Engine) LastRun() time.Time {
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	return e.lastRun
}

// Checks describes every check with its effective settings.
func (e *Engine) Checks(ctx context.Context) ([]CheckInfo, error) {
	settings, err := e.st.HealthCheckSettings(ctx)
	if err != nil {
		return nil, err
	}
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	var out []CheckInfo
	for _, c := range e.reg.All() {
		enabled, heal := effective(c, settings)
		out = append(out, CheckInfo{
			ID: c.ID, Title: c.Title, Category: c.Category, Description: c.Description, GraceSeconds: int(c.Grace.Seconds()),
			AutoHealDefault: c.AutoHealDefault, Enabled: enabled, AutoHeal: heal, SupportsHeal: c.SupportsHeal,
			LastRunError: e.errs[c.ID],
		})
	}
	return out, nil
}

func effective(c Check, settings map[string]model.HealthCheckSetting) (enabled, autoHeal bool) {
	enabled, autoHeal = true, c.AutoHealDefault
	if s, ok := settings[c.ID]; ok {
		enabled = s.Enabled
		if s.AutoHeal != nil {
			autoHeal = *s.AutoHeal
		}
	}
	return enabled, autoHeal && c.SupportsHeal
}

// RunAll runs every enabled check.
func (e *Engine) RunAll(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	settings, err := e.st.HealthCheckSettings(ctx)
	if err != nil {
		e.log.Warn("health: loading check settings", "err", err)
		return
	}
	for _, c := range e.reg.All() {
		e.runLocked(ctx, c, settings, false)
	}
	e.stateMu.Lock()
	e.lastRun = time.Now()
	e.stateMu.Unlock()
}

// RunCheck runs one check now. Findings that are no longer detected resolve
// immediately (an admin asked for a fresh answer).
func (e *Engine) RunCheck(ctx context.Context, id string) error {
	c, ok := e.reg.Get(id)
	if !ok {
		return store.ErrNotFound
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	settings, err := e.st.HealthCheckSettings(ctx)
	if err != nil {
		return err
	}
	e.runLocked(ctx, c, settings, true)
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	if msg := e.errs[id]; msg != "" {
		return errors.New(msg)
	}
	return nil
}

func (e *Engine) setErr(id, msg string) {
	e.stateMu.Lock()
	if msg == "" {
		delete(e.errs, id)
	} else {
		e.errs[id] = msg
	}
	e.stateMu.Unlock()
}

func (e *Engine) runLocked(ctx context.Context, c Check, settings map[string]model.HealthCheckSetting, immediate bool) {
	enabled, autoHeal := effective(c, settings)
	existing, err := e.st.HealthFindingsForCheck(ctx, c.ID)
	if err != nil {
		e.log.Warn("health: loading findings", "check", c.ID, "err", err)
		return
	}
	if !enabled {
		// A disabled check shows nothing: quietly close what it had raised.
		for _, f := range existing {
			e.closeQuietly(ctx, f, "check disabled")
		}
		e.setErr(c.ID, "")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, checkTimeout)
	found, err := c.Run(cctx)
	cancel()
	if err != nil {
		// Never resolve findings because a check failed to run.
		e.log.Warn("health check failed", "check", c.ID, "err", err)
		e.setErr(c.ID, err.Error())
		return
	}
	e.setErr(c.ID, "")
	e.reconcile(ctx, c, existing, found, immediate)
	if autoHeal {
		e.autoHeal(ctx, c, found)
	}
}

func key(objectType, objectID string) string { return objectType + "\x00" + objectID }

func remediationMeta(rs []Remediation) []model.HealthRemediation {
	out := make([]model.HealthRemediation, 0, len(rs))
	for _, r := range rs {
		out = append(out, model.HealthRemediation{ID: r.ID, Label: r.Label, Description: r.Description, Safe: r.Safe, Runnable: r.Runnable(), URL: r.URL})
	}
	return out
}

// reconcile applies the noise rules described in the package comment.
func (e *Engine) reconcile(ctx context.Context, c Check, existing []model.HealthFinding, found []Finding, immediate bool) {
	now := time.Now()
	byKey := map[string]model.HealthFinding{}
	for _, f := range existing {
		byKey[key(f.ObjectType, f.ObjectID)] = f
	}
	seen := map[string]bool{}
	for _, nf := range found {
		k := key(nf.ObjectType, nf.ObjectID)
		if seen[k] {
			continue // checks should not emit duplicates; keep the first
		}
		seen[k] = true
		if nf.Facts == nil {
			nf.Facts = map[string]any{}
		}
		cur, ok := byKey[k]
		if !ok {
			f := model.HealthFinding{CheckID: c.ID, ObjectType: nf.ObjectType, ObjectID: nf.ObjectID, ObjectName: nf.ObjectName,
				Severity: nf.Severity, Summary: nf.Summary, Detail: nf.Detail, Facts: nf.Facts, Remediations: remediationMeta(nf.Remediations), State: "open"}
			if c.Grace > 0 {
				f.State = "pending"
			}
			f.Notified = f.State == "open" && nf.Severity != Info
			id, err := e.st.InsertHealthFinding(ctx, f)
			if err != nil {
				e.log.Warn("health: storing finding", "check", c.ID, "err", err)
				continue
			}
			f.ID = id
			if f.State == "open" {
				e.opened(ctx, c, f, "opened")
			}
			continue
		}
		prevSeverity := cur.Severity
		cur.ObjectName, cur.Summary, cur.Detail, cur.Facts = nf.ObjectName, nf.Summary, nf.Detail, nf.Facts
		cur.Severity = nf.Severity
		cur.Remediations = remediationMeta(nf.Remediations)
		cur.LastSeen = now
		cur.Misses = 0
		switch cur.State {
		case "resolved":
			if cur.ResolvedAt != nil && now.Sub(*cur.ResolvedAt) < flapWindow {
				// Flapping: reopen without another alert.
				cur.State = "open"
				cur.ResolvedAt = nil
				cur.Occurrences++
				e.st.AddHealthEvent(ctx, cur.ID, c.ID, "reopened", "system", "condition returned shortly after resolving")
			} else {
				cur.State, cur.ResolvedAt, cur.FirstSeen = "open", nil, now
				cur.Occurrences++
				cur.HealAttempts, cur.LastHealAt, cur.LastHealError = 0, nil, ""
				cur.AcknowledgedBy = ""
				if c.Grace > 0 {
					cur.State = "pending"
				} else {
					cur.Notified = cur.Severity != Info
					e.opened(ctx, c, cur, "opened")
				}
			}
		case "pending":
			if now.Sub(cur.FirstSeen) >= c.Grace {
				cur.State = "open"
				cur.Notified = cur.Severity != Info
				e.opened(ctx, c, cur, "opened")
			}
		case "snoozed":
			if cur.SnoozedUntil != nil && now.After(*cur.SnoozedUntil) {
				cur.State, cur.SnoozedUntil = "open", nil
				e.st.AddHealthEvent(ctx, cur.ID, c.ID, "unsnoozed", "system", "snooze expired and the condition is still present")
			}
		case "acknowledged":
			if SeverityRank(cur.Severity) < SeverityRank(prevSeverity) {
				cur.State, cur.AcknowledgedBy = "open", ""
			}
		}
		if cur.State != "pending" && SeverityRank(cur.Severity) < SeverityRank(prevSeverity) && cur.State != "snoozed" {
			cur.Notified = true
			e.st.AddHealthEvent(ctx, cur.ID, c.ID, "escalated", "system", fmt.Sprintf("severity %s → %s", prevSeverity, cur.Severity))
			e.fire(EventIssueOpened, c, cur)
		}
		if err := e.st.UpdateHealthFinding(ctx, cur); err != nil {
			e.log.Warn("health: updating finding", "check", c.ID, "err", err)
		}
	}
	// Anything not detected this run is on its way to resolving.
	for _, cur := range existing {
		if seen[key(cur.ObjectType, cur.ObjectID)] || cur.State == "resolved" {
			continue
		}
		if cur.State == "pending" {
			// Cleared during the grace period: it was never shown, leave no trace.
			e.st.DeleteHealthFinding(ctx, cur.ID)
			continue
		}
		cur.Misses++
		if !immediate && cur.Misses < resolveAfterMisses {
			e.st.UpdateHealthFinding(ctx, cur)
			continue
		}
		cur.State, cur.ResolvedAt, cur.SnoozedUntil = "resolved", &now, nil
		e.st.UpdateHealthFinding(ctx, cur)
		e.st.AddHealthEvent(ctx, cur.ID, c.ID, "resolved", "system", "condition no longer detected")
		if cur.Notified {
			e.fire(EventIssueResolved, c, cur)
		}
	}
}

func (e *Engine) closeQuietly(ctx context.Context, f model.HealthFinding, why string) {
	if f.State == "resolved" {
		return
	}
	if f.State == "pending" {
		e.st.DeleteHealthFinding(ctx, f.ID)
		return
	}
	now := time.Now()
	f.State, f.ResolvedAt = "resolved", &now
	e.st.UpdateHealthFinding(ctx, f)
	e.st.AddHealthEvent(ctx, f.ID, f.CheckID, "resolved", "system", why)
}

func (e *Engine) opened(ctx context.Context, c Check, f model.HealthFinding, event string) {
	e.st.AddHealthEvent(ctx, f.ID, c.ID, event, "system", f.Summary)
	lvl := slog.LevelInfo
	if f.Severity == Critical {
		lvl = slog.LevelWarn
	}
	e.log.Log(ctx, lvl, "health issue detected", "check", c.ID, "severity", f.Severity, "object", f.ObjectName, "summary", f.Summary)
	if f.Severity != Info {
		e.fire(EventIssueOpened, c, f)
	}
}

func (e *Engine) fire(event string, c Check, f model.HealthFinding) {
	if e.hooks == nil {
		return
	}
	e.hooks.Fire(event, map[string]any{
		"finding_id": f.ID, "check_id": c.ID, "check": c.Title, "severity": f.Severity, "summary": f.Summary,
		"object_type": f.ObjectType, "object_id": f.ObjectID, "object_name": f.ObjectName,
	})
}

// autoHeal runs Safe remediations for current findings within the attempt cap and backoff.
func (e *Engine) autoHeal(ctx context.Context, c Check, found []Finding) {
	stored, err := e.st.HealthFindingsForCheck(ctx, c.ID)
	if err != nil {
		return
	}
	byKey := map[string]model.HealthFinding{}
	for _, f := range stored {
		byKey[key(f.ObjectType, f.ObjectID)] = f
	}
	now := time.Now()
	for _, nf := range found {
		cur, ok := byKey[key(nf.ObjectType, nf.ObjectID)]
		if !ok || cur.State == "snoozed" || cur.State == "resolved" {
			continue
		}
		if cur.HealAttempts >= c.MaxHealAttempts {
			continue
		}
		if cur.LastHealAt != nil && now.Sub(*cur.LastHealAt) < healBackoff(cur.HealAttempts) {
			continue
		}
		var rem *Remediation
		for i := range nf.Remediations {
			if nf.Remediations[i].Safe && nf.Remediations[i].Runnable() {
				rem = &nf.Remediations[i]
				break
			}
		}
		if rem == nil {
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, checkTimeout)
		err := rem.Run(rctx)
		cancel()
		cur.HealAttempts++
		cur.LastHealAt = &now
		if err != nil {
			cur.LastHealError = err.Error()
			e.st.AddHealthEvent(ctx, cur.ID, c.ID, "heal_failed", "system", rem.Label+": "+err.Error())
			e.log.Warn("health: self-heal failed", "check", c.ID, "object", cur.ObjectName, "action", rem.ID, "err", err)
		} else {
			cur.LastHealError = ""
			e.st.AddHealthEvent(ctx, cur.ID, c.ID, "auto_heal", "system", fmt.Sprintf("%s (attempt %d of %d)", rem.Label, cur.HealAttempts, c.MaxHealAttempts))
			e.log.Info("health: self-heal applied", "check", c.ID, "object", cur.ObjectName, "action", rem.ID, "attempt", cur.HealAttempts)
		}
		e.st.UpdateHealthFinding(ctx, cur)
	}
}

// Remediate runs a remediation an admin chose for a finding. Remediations are
// rebuilt from a fresh run of the check so they always act on current state.
func (e *Engine) Remediate(ctx context.Context, findingID, remediationID, actor string) (string, error) {
	f, err := e.st.GetHealthFinding(ctx, findingID)
	if err != nil {
		return "", err
	}
	c, ok := e.reg.Get(f.CheckID)
	if !ok {
		return "", fmt.Errorf("unknown check %q", f.CheckID)
	}
	cctx, cancel := context.WithTimeout(ctx, checkTimeout)
	found, err := c.Run(cctx)
	cancel()
	if err != nil {
		return "", fmt.Errorf("re-running check: %w", err)
	}
	var live *Finding
	for i := range found {
		if found[i].ObjectType == f.ObjectType && found[i].ObjectID == f.ObjectID {
			live = &found[i]
			break
		}
	}
	if live == nil {
		e.mu.Lock()
		existing, _ := e.st.HealthFindingsForCheck(ctx, c.ID)
		e.reconcile(ctx, c, existing, found, true)
		e.mu.Unlock()
		return "", ErrAlreadyResolved
	}
	var rem *Remediation
	for i := range live.Remediations {
		if live.Remediations[i].ID == remediationID {
			rem = &live.Remediations[i]
		}
	}
	if rem == nil || !rem.Runnable() {
		return "", fmt.Errorf("remediation %q is not available for this finding", remediationID)
	}
	rctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	err = rem.Run(rctx)
	cancel()
	if err != nil {
		e.st.AddHealthEvent(ctx, f.ID, c.ID, "remediation_failed", actor, rem.Label+": "+err.Error())
		return "", err
	}
	e.st.AddHealthEvent(ctx, f.ID, c.ID, "remediated", actor, rem.Label)
	// Re-evaluate right away so server-side fixes show as resolved immediately.
	e.RunCheck(ctx, c.ID)
	return rem.Label, nil
}

// SetState acknowledges, snoozes or reopens a finding.
func (e *Engine) SetState(ctx context.Context, findingID, state, actor string, snoozeFor time.Duration) (*model.HealthFinding, error) {
	f, err := e.st.GetHealthFinding(ctx, findingID)
	if err != nil {
		return nil, err
	}
	if f.State == "resolved" || f.State == "pending" {
		return nil, fmt.Errorf("finding is %s", f.State)
	}
	msg := ""
	switch state {
	case "acknowledged":
		f.State, f.AcknowledgedBy, f.SnoozedUntil = "acknowledged", actor, nil
	case "snoozed":
		if snoozeFor <= 0 || snoozeFor > 90*24*time.Hour {
			return nil, errors.New("snooze duration must be between 1 minute and 90 days")
		}
		until := time.Now().Add(snoozeFor)
		f.State, f.SnoozedUntil = "snoozed", &until
		msg = "until " + until.UTC().Format(time.RFC3339)
	case "open":
		f.State, f.AcknowledgedBy, f.SnoozedUntil = "open", "", nil
	default:
		return nil, fmt.Errorf("unknown state %q", state)
	}
	if err := e.st.UpdateHealthFinding(ctx, *f); err != nil {
		return nil, err
	}
	e.st.AddHealthEvent(ctx, f.ID, f.CheckID, state, actor, msg)
	return f, nil
}
