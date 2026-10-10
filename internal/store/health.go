package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

const healthCols = `id, check_id, object_type, object_id, object_name, severity, summary, detail, facts, remediations, state,
	snoozed_until, acknowledged_by, first_seen, last_seen, resolved_at, occurrences, misses, notified, heal_attempts, last_heal_at, last_heal_error`

func scanHealth(sc interface{ Scan(...any) error }) (model.HealthFinding, error) {
	var f model.HealthFinding
	var facts db.JSON[map[string]any]
	var rems db.JSON[[]model.HealthRemediation]
	err := sc.Scan(&f.ID, &f.CheckID, &f.ObjectType, &f.ObjectID, &f.ObjectName, &f.Severity, &f.Summary, &f.Detail, &facts, &rems, &f.State,
		&f.SnoozedUntil, &f.AcknowledgedBy, &f.FirstSeen, &f.LastSeen, &f.ResolvedAt, &f.Occurrences, &f.Misses, &f.Notified, &f.HealAttempts, &f.LastHealAt, &f.LastHealError)
	f.Facts = facts.V
	if f.Facts == nil {
		f.Facts = map[string]any{}
	}
	f.Remediations = rems.V
	if f.Remediations == nil {
		f.Remediations = []model.HealthRemediation{}
	}
	return f, err
}

func (s *Store) queryHealth(ctx context.Context, q string, args ...any) ([]model.HealthFinding, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.HealthFinding{}
	for rows.Next() {
		f, err := scanHealth(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// HealthFindingsForCheck returns every stored finding (any state) for a check.
func (s *Store) HealthFindingsForCheck(ctx context.Context, checkID string) ([]model.HealthFinding, error) {
	return s.queryHealth(ctx, `SELECT `+healthCols+` FROM health_findings WHERE check_id = $1`, checkID)
}

// GetHealthFinding loads one finding.
func (s *Store) GetHealthFinding(ctx context.Context, id string) (*model.HealthFinding, error) {
	f, err := scanHealth(s.DB.QueryRowContext(ctx, `SELECT `+healthCols+` FROM health_findings WHERE id = $1`, id))
	if err != nil {
		return nil, wrapNotFound(err)
	}
	return &f, nil
}

// HealthFilter selects findings for listing.
type HealthFilter struct {
	States     []string // default: pending excluded, resolved excluded
	Severity   string
	CheckID    string
	ObjectType string
	ObjectID   string
	Limit      int
}

// ListHealthFindings returns findings, most severe and most recent first.
func (s *Store) ListHealthFindings(ctx context.Context, f HealthFilter) ([]model.HealthFinding, error) {
	states := f.States
	if len(states) == 0 {
		states = []string{"open", "acknowledged", "snoozed"}
	}
	where := []string{"state = ANY($1)"}
	args := []any{"{" + strings.Join(states, ",") + "}"}
	add := func(col, v string) {
		if v != "" {
			args = append(args, v)
			where = append(where, col+" = $"+itoa(len(args)))
		}
	}
	add("severity", f.Severity)
	add("check_id", f.CheckID)
	add("object_type", f.ObjectType)
	add("object_id", f.ObjectID)
	limit := f.Limit
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	return s.queryHealth(ctx, `SELECT `+healthCols+` FROM health_findings WHERE `+strings.Join(where, " AND ")+`
		ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, last_seen DESC LIMIT `+itoa(limit), args...)
}

// InsertHealthFinding stores a new finding and returns its ID.
func (s *Store) InsertHealthFinding(ctx context.Context, f model.HealthFinding) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO health_findings
		(check_id, object_type, object_id, object_name, severity, summary, detail, facts, remediations, state, notified)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10,$11) RETURNING id`,
		f.CheckID, f.ObjectType, f.ObjectID, f.ObjectName, f.Severity, f.Summary, f.Detail, jsonArg(f.Facts), jsonArg(f.Remediations), f.State, f.Notified).Scan(&id)
	return id, err
}

// UpdateHealthFinding writes back the mutable fields of a finding.
func (s *Store) UpdateHealthFinding(ctx context.Context, f model.HealthFinding) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE health_findings SET object_name=$2, severity=$3, summary=$4, detail=$5, facts=$6::jsonb,
		remediations=$7::jsonb, state=$8, snoozed_until=$9, acknowledged_by=$10, first_seen=$11, last_seen=$12, resolved_at=$13,
		occurrences=$14, misses=$15, notified=$16, heal_attempts=$17, last_heal_at=$18, last_heal_error=$19 WHERE id=$1`,
		f.ID, f.ObjectName, f.Severity, f.Summary, f.Detail, jsonArg(f.Facts), jsonArg(f.Remediations), f.State, f.SnoozedUntil,
		f.AcknowledgedBy, f.FirstSeen, f.LastSeen, f.ResolvedAt, f.Occurrences, f.Misses, f.Notified, f.HealAttempts, f.LastHealAt, f.LastHealError)
	return err
}

// DeleteHealthFinding removes a finding (used for conditions that cleared during their grace period).
func (s *Store) DeleteHealthFinding(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM health_findings WHERE id = $1`, id)
	return err
}

// AddHealthEvent appends a lifecycle event.
func (s *Store) AddHealthEvent(ctx context.Context, findingID, checkID, event, actor, msg string) {
	var fid any
	if findingID != "" {
		fid = findingID
	}
	s.DB.ExecContext(ctx, `INSERT INTO health_events (finding_id, check_id, event, actor, message) VALUES ($1,$2,$3,$4,$5)`, fid, checkID, event, actor, msg)
}

// HealthEvents returns events for one finding, or the most recent events overall.
func (s *Store) HealthEvents(ctx context.Context, findingID string, limit int) ([]model.HealthEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, COALESCE(finding_id::text, ''), check_id, event, actor, message, at FROM health_events`
	args := []any{}
	if findingID != "" {
		q += ` WHERE finding_id = $1`
		args = append(args, findingID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY at DESC, id DESC LIMIT `+itoa(limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.HealthEvent{}
	for rows.Next() {
		var e model.HealthEvent
		if err := rows.Scan(&e.ID, &e.FindingID, &e.CheckID, &e.Event, &e.Actor, &e.Message, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// HealthCheckSettings returns admin overrides keyed by check ID.
func (s *Store) HealthCheckSettings(ctx context.Context) (map[string]model.HealthCheckSetting, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT check_id, enabled, auto_heal FROM health_check_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]model.HealthCheckSetting{}
	for rows.Next() {
		var id string
		var st model.HealthCheckSetting
		var ah sql.NullBool
		if err := rows.Scan(&id, &st.Enabled, &ah); err != nil {
			return nil, err
		}
		if ah.Valid {
			v := ah.Bool
			st.AutoHeal = &v
		}
		out[id] = st
	}
	return out, rows.Err()
}

// SaveHealthCheckSetting stores an override for a check.
func (s *Store) SaveHealthCheckSetting(ctx context.Context, checkID string, st model.HealthCheckSetting) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO health_check_settings (check_id, enabled, auto_heal) VALUES ($1,$2,$3)
		ON CONFLICT (check_id) DO UPDATE SET enabled = EXCLUDED.enabled, auto_heal = EXCLUDED.auto_heal, updated_at = now()`,
		checkID, st.Enabled, st.AutoHeal)
	return err
}

// PruneHealth deletes resolved findings and events older than the given age.
func (s *Store) PruneHealth(ctx context.Context, days int) {
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	s.DB.ExecContext(ctx, `DELETE FROM health_findings WHERE state = 'resolved' AND resolved_at < $1`, cutoff)
	s.DB.ExecContext(ctx, `DELETE FROM health_events WHERE at < $1`, cutoff)
}
