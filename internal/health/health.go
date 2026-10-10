// Package health detects misconfigurations and fleet problems, explains them,
// offers remediations, and self-heals where it is safe to do so.
//
// Design goals ("visibility without noise"):
//   - checks apply grace periods and thresholds so transient blips never alert
//   - findings are deduplicated per (check, object) and grouped per check
//   - findings resolve themselves when the condition clears; a finding that
//     reappears within an hour is reopened silently (flap protection)
//   - admins can acknowledge or snooze; only new warning/critical findings
//     fire webhooks
//   - only remediations marked Safe may run automatically, with backoff and a
//     cap on attempts; every attempt is recorded
package health

import (
	"context"
	"sort"
	"time"
)

// Severity levels.
const (
	Critical = "critical"
	Warning  = "warning"
	Info     = "info"
)

// Remediation is an action that can fix a finding.
type Remediation struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Safe remediations may run automatically (self-healing) when the check's
	// auto-heal setting is on. Unsafe ones are only run by an admin.
	Safe bool `json:"safe"`
	// URL points the admin to the place to fix it manually (console route).
	URL string                          `json:"url,omitempty"`
	Run func(ctx context.Context) error `json:"-"`
}

// Runnable reports whether the server can perform this remediation itself.
func (r Remediation) Runnable() bool { return r.Run != nil }

// Finding is one detected problem with one object.
type Finding struct {
	CheckID      string         `json:"check_id"`
	ObjectType   string         `json:"object_type"`
	ObjectID     string         `json:"object_id"`
	ObjectName   string         `json:"object_name"`
	Severity     string         `json:"severity"`
	Summary      string         `json:"summary"`
	Detail       string         `json:"detail"`
	Facts        map[string]any `json:"facts"`
	Remediations []Remediation  `json:"remediations"`
}

// Check is a detector.
type Check struct {
	ID          string
	Title       string
	Category    string // platform | fleet | configuration | security
	Description string
	// AutoHealDefault is whether Safe remediations run automatically unless an
	// admin changes the setting.
	AutoHealDefault bool
	// MaxHealAttempts caps automatic attempts per finding (default 3).
	MaxHealAttempts int
	// Grace hides a new finding until its condition has persisted this long,
	// so transient blips never surface. Zero shows findings immediately.
	Grace time.Duration
	// SupportsHeal: the check can emit Safe, runnable remediations.
	SupportsHeal bool
	Run          func(ctx context.Context) ([]Finding, error)
}

// CheckInfo describes a check for the console.
type CheckInfo struct {
	ID              string `json:"id"`
	GraceSeconds    int    `json:"grace_seconds"`
	Title           string `json:"title"`
	Category        string `json:"category"`
	Description     string `json:"description"`
	AutoHealDefault bool   `json:"auto_heal_default"`
	Enabled         bool   `json:"enabled"`
	AutoHeal        bool   `json:"auto_heal"`
	SupportsHeal    bool   `json:"supports_heal"`
	LastRunError    string `json:"last_run_error,omitempty"`
}

// Registry holds checks in display order.
type Registry struct {
	checks []Check
	byID   map[string]Check
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry { return &Registry{byID: map[string]Check{}} }

// Add registers a check.
func (r *Registry) Add(c Check) {
	if c.MaxHealAttempts == 0 {
		c.MaxHealAttempts = 3
	}
	r.checks = append(r.checks, c)
	r.byID[c.ID] = c
}

// Get returns a check by ID.
func (r *Registry) Get(id string) (Check, bool) {
	c, ok := r.byID[id]
	return c, ok
}

// All returns checks sorted by category then title.
func (r *Registry) All() []Check {
	out := append([]Check(nil), r.checks...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Title < out[j].Title
	})
	return out
}

// healBackoff is the minimum wait before automatic attempt n+1.
func healBackoff(attempts int) time.Duration {
	if attempts <= 0 {
		return 0
	}
	return time.Duration(attempts) * time.Hour
}

// SeverityRank orders severities for sorting.
func SeverityRank(s string) int {
	switch s {
	case Critical:
		return 0
	case Warning:
		return 1
	}
	return 2
}
