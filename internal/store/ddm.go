package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/ddm"
	"github.com/vibemdm/vibemdm/internal/model"
)

const planCols = `id, name, enabled, kind, target_os_version, target_build_version, target_local_datetime, details_url, settings, scope, created_at, updated_at`

func scanPlan(row interface{ Scan(...any) error }) (*model.UpdatePlan, error) {
	var p model.UpdatePlan
	var set db.JSON[map[string]any]
	var sc db.JSON[model.Scope]
	if err := row.Scan(&p.ID, &p.Name, &p.Enabled, &p.Kind, &p.TargetOSVersion, &p.TargetBuildVersion, &p.TargetLocalDateTime, &p.DetailsURL, &set, &sc, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	p.Settings, p.Scope = set.V, sc.V
	if p.Settings == nil {
		p.Settings = map[string]any{}
	}
	p.Scope.Normalize()
	return &p, nil
}

// ListUpdatePlans returns all plans.
func (s *Store) ListUpdatePlans(ctx context.Context) ([]model.UpdatePlan, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+planCols+` FROM update_plans ORDER BY kind, lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.UpdatePlan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetUpdatePlan returns a plan.
func (s *Store) GetUpdatePlan(ctx context.Context, id string) (*model.UpdatePlan, error) {
	return scanPlan(s.DB.QueryRowContext(ctx, `SELECT `+planCols+` FROM update_plans WHERE id = $1`, id))
}

// SaveUpdatePlan creates (empty ID) or updates a plan. Callers validate first.
func (s *Store) SaveUpdatePlan(ctx context.Context, p model.UpdatePlan) (*model.UpdatePlan, error) {
	p.Scope.Normalize()
	if p.Settings == nil {
		p.Settings = map[string]any{}
	}
	if p.ID == "" {
		return scanPlan(s.DB.QueryRowContext(ctx, `INSERT INTO update_plans (name, enabled, kind, target_os_version, target_build_version, target_local_datetime, details_url, settings, scope)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb) RETURNING `+planCols,
			p.Name, p.Enabled, p.Kind, p.TargetOSVersion, p.TargetBuildVersion, p.TargetLocalDateTime, p.DetailsURL, jsonArg(p.Settings), jsonArg(p.Scope)))
	}
	return scanPlan(s.DB.QueryRowContext(ctx, `UPDATE update_plans SET name=$2, enabled=$3, kind=$4, target_os_version=$5, target_build_version=$6,
		target_local_datetime=$7, details_url=$8, settings=$9::jsonb, scope=$10::jsonb, updated_at=now() WHERE id=$1 RETURNING `+planCols,
		p.ID, p.Name, p.Enabled, p.Kind, p.TargetOSVersion, p.TargetBuildVersion, p.TargetLocalDateTime, p.DetailsURL, jsonArg(p.Settings), jsonArg(p.Scope)))
}

// DeleteUpdatePlan removes a plan.
func (s *Store) DeleteUpdatePlan(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM update_plans WHERE id = $1`, id)
	return err
}

// SettingsFromMap converts a plan's stored settings into ddm.Settings.
func SettingsFromMap(m map[string]any) ddm.Settings {
	var st ddm.Settings
	b, _ := json.Marshal(m)
	json.Unmarshal(b, &st)
	return st
}

// EffectivePlans picks the plans that apply to a computer. With several
// enforcement plans in scope, the highest target version wins (ties: earliest
// deadline); with several settings plans, the most recently updated wins.
func (s *Store) EffectivePlans(ctx context.Context, c *model.Computer) (enf *model.UpdatePlan, set *model.UpdatePlan, err error) {
	plans, err := s.ListUpdatePlans(ctx)
	if err != nil {
		return nil, nil, err
	}
	cache := NewMembershipCache()
	var enfs, sets []model.UpdatePlan
	for _, p := range plans {
		if !p.Enabled {
			continue
		}
		in, err := s.InScopeComputer(ctx, c, p.Scope, cache)
		if err != nil {
			return nil, nil, err
		}
		if !in {
			continue
		}
		if p.Kind == "enforcement" {
			enfs = append(enfs, p)
		} else {
			sets = append(sets, p)
		}
	}
	sort.SliceStable(enfs, func(i, j int) bool {
		if c := ddm.CompareVersions(enfs[i].TargetOSVersion, enfs[j].TargetOSVersion); c != 0 {
			return c > 0
		}
		return enfs[i].TargetLocalDateTime < enfs[j].TargetLocalDateTime
	})
	sort.SliceStable(sets, func(i, j int) bool { return sets[i].UpdatedAt.After(sets[j].UpdatedAt) })
	if len(enfs) > 0 {
		enf = &enfs[0]
	}
	if len(sets) > 0 {
		set = &sets[0]
	}
	return enf, set, nil
}

// DeclarationsFor builds a computer's full declaration set.
func (s *Store) DeclarationsFor(ctx context.Context, c *model.Computer) ([]ddm.Declaration, error) {
	enfPlan, setPlan, err := s.EffectivePlans(ctx, c)
	if err != nil {
		return nil, err
	}
	var enf *ddm.Enforcement
	if enfPlan != nil {
		enf = &ddm.Enforcement{PlanID: enfPlan.ID, TargetOSVersion: enfPlan.TargetOSVersion, TargetBuildVersion: enfPlan.TargetBuildVersion,
			TargetLocalDateTime: enfPlan.TargetLocalDateTime, DetailsURL: enfPlan.DetailsURL}
	}
	var set *ddm.Settings
	if setPlan != nil {
		st := SettingsFromMap(setPlan.Settings)
		set = &st
	}
	return ddm.Build(enf, set), nil
}

// ---- device state ----

func (s *Store) ensureDDMState(ctx context.Context, computerID string) {
	s.DB.ExecContext(ctx, `INSERT INTO ddm_device_state (computer_id) VALUES ($1) ON CONFLICT DO NOTHING`, computerID)
}

// GetDDMState returns a computer's DDM state (zero state if none yet).
func (s *Store) GetDDMState(ctx context.Context, computerID string) (*model.DDMState, error) {
	st := &model.DDMState{ComputerID: computerID, Status: map[string]any{}, DeclarationsStatus: map[string]any{}, Errors: []any{}}
	var sent, sync, stat sql.NullTime
	var status, decl db.JSON[map[string]any]
	var errs db.JSON[[]any]
	err := s.DB.QueryRowContext(ctx, `SELECT served_token, sync_sent_token, sync_sent_at, last_sync_at, last_status_at, status, declarations_status, errors
		FROM ddm_device_state WHERE computer_id = $1`, computerID).Scan(&st.ServedToken, &st.SyncSentToken, &sent, &sync, &stat, &status, &decl, &errs)
	if err == sql.ErrNoRows {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	st.SyncSentAt, st.LastSyncAt, st.LastStatusAt = db.TimePtr(sent), db.TimePtr(sync), db.TimePtr(stat)
	if status.V != nil {
		st.Status = status.V
	}
	if decl.V != nil {
		st.DeclarationsStatus = decl.V
	}
	if errs.V != nil {
		st.Errors = errs.V
	}
	return st, nil
}

// MarkDDMServed records the token a device fetched.
func (s *Store) MarkDDMServed(ctx context.Context, computerID, token string) {
	s.ensureDDMState(ctx, computerID)
	s.DB.ExecContext(ctx, `UPDATE ddm_device_state SET served_token = $2, last_sync_at = now() WHERE computer_id = $1`, computerID, token)
}

// MarkDDMSyncSent records that we asked a device to sync to a token.
func (s *Store) MarkDDMSyncSent(ctx context.Context, computerID, token string) {
	s.ensureDDMState(ctx, computerID)
	s.DB.ExecContext(ctx, `UPDATE ddm_device_state SET sync_sent_token = $2, sync_sent_at = now() WHERE computer_id = $1`, computerID, token)
}

// ApplyDDMStatus merges a status report and keeps the last 50 raw reports.
func (s *Store) ApplyDDMStatus(ctx context.Context, computerID string, raw []byte) error {
	var rep ddm.StatusReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		return err
	}
	cur, err := s.GetDDMState(ctx, computerID)
	if err != nil {
		return err
	}
	merged := ddm.MergeStatus(cur.Status, rep.StatusItems)
	declStatus := cur.DeclarationsStatus
	if rep.Declarations != nil {
		declStatus = rep.Declarations
	}
	errs := cur.Errors
	if rep.Errors != nil {
		errs = rep.Errors
	}
	s.ensureDDMState(ctx, computerID)
	if _, err := s.DB.ExecContext(ctx, `UPDATE ddm_device_state SET status = $2::jsonb, declarations_status = $3::jsonb, errors = $4::jsonb, last_status_at = now()
		WHERE computer_id = $1`, computerID, jsonArg(merged), jsonArg(declStatus), jsonArg(errs)); err != nil {
		return err
	}
	s.DB.ExecContext(ctx, `INSERT INTO ddm_status_reports (computer_id, report) VALUES ($1, $2::jsonb)`, computerID, string(raw))
	s.DB.ExecContext(ctx, `DELETE FROM ddm_status_reports WHERE computer_id = $1 AND id NOT IN
		(SELECT id FROM ddm_status_reports WHERE computer_id = $1 ORDER BY at DESC LIMIT 50)`, computerID)
	// Reflect the OS version the device reports.
	if dev, ok := merged["device"].(map[string]any); ok {
		if osm, ok := dev["operating-system"].(map[string]any); ok {
			v, _ := osm["version"].(string)
			b, _ := osm["build-version"].(string)
			s.DB.ExecContext(ctx, `UPDATE computers SET os_version = CASE WHEN $2 <> '' THEN $2 ELSE os_version END,
				os_build = CASE WHEN $3 <> '' THEN $3 ELSE os_build END WHERE id = $1`, computerID, v, b)
		}
	}
	return nil
}

// RecentDDMReports returns a computer's latest raw status reports.
func (s *Store) RecentDDMReports(ctx context.Context, computerID string, limit int) ([]map[string]any, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT report, at FROM ddm_status_reports WHERE computer_id = $1 ORDER BY at DESC LIMIT $2`, computerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var rep db.JSON[map[string]any]
		var at sql.NullTime
		if err := rows.Scan(&rep, &at); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"at": at.Time, "report": rep.V})
	}
	return out, rows.Err()
}
