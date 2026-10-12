package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

// ---- restricted software ----

const restrictedCols = `id, name, process_name, match_type, kill_process, notify_user, display_message, enabled, scope, created_at, updated_at`

func scanRestricted(row interface{ Scan(...any) error }) (*model.RestrictedSoftware, error) {
	var r model.RestrictedSoftware
	var sc db.JSON[model.Scope]
	if err := row.Scan(&r.ID, &r.Name, &r.ProcessName, &r.MatchType, &r.KillProcess, &r.NotifyUser, &r.DisplayMessage, &r.Enabled, &sc, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	r.Scope = sc.V
	r.Scope.Normalize()
	return &r, nil
}

// ListRestrictedSoftware returns all rules.
func (s *Store) ListRestrictedSoftware(ctx context.Context) ([]model.RestrictedSoftware, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+restrictedCols+` FROM restricted_software ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RestrictedSoftware{}
	for rows.Next() {
		r, err := scanRestricted(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// GetRestrictedSoftware returns one rule.
func (s *Store) GetRestrictedSoftware(ctx context.Context, id string) (*model.RestrictedSoftware, error) {
	return scanRestricted(s.DB.QueryRowContext(ctx, `SELECT `+restrictedCols+` FROM restricted_software WHERE id = $1`, id))
}

func normRestricted(r *model.RestrictedSoftware) {
	if r.MatchType != "contains" && r.MatchType != "regex" {
		r.MatchType = "exact"
	}
	r.Scope.Normalize()
}

// CreateRestrictedSoftware inserts a rule.
func (s *Store) CreateRestrictedSoftware(ctx context.Context, r model.RestrictedSoftware) (*model.RestrictedSoftware, error) {
	normRestricted(&r)
	return scanRestricted(s.DB.QueryRowContext(ctx, `INSERT INTO restricted_software (name, process_name, match_type, kill_process, notify_user, display_message, enabled, scope)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb) RETURNING `+restrictedCols,
		r.Name, r.ProcessName, r.MatchType, r.KillProcess, r.NotifyUser, r.DisplayMessage, r.Enabled, jsonArg(r.Scope)))
}

// UpdateRestrictedSoftware updates a rule.
func (s *Store) UpdateRestrictedSoftware(ctx context.Context, r model.RestrictedSoftware) (*model.RestrictedSoftware, error) {
	normRestricted(&r)
	return scanRestricted(s.DB.QueryRowContext(ctx, `UPDATE restricted_software SET name=$2, process_name=$3, match_type=$4, kill_process=$5, notify_user=$6,
		display_message=$7, enabled=$8, scope=$9::jsonb, updated_at=now() WHERE id=$1 RETURNING `+restrictedCols,
		r.ID, r.Name, r.ProcessName, r.MatchType, r.KillProcess, r.NotifyUser, r.DisplayMessage, r.Enabled, jsonArg(r.Scope)))
}

// DeleteRestrictedSoftware removes a rule.
func (s *Store) DeleteRestrictedSoftware(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM restricted_software WHERE id=$1`, id)
	return err
}

// RestrictedForComputer returns enabled rules in scope for a computer.
func (s *Store) RestrictedForComputer(ctx context.Context, c *model.Computer) ([]model.RestrictedSoftware, error) {
	rules, err := s.ListRestrictedSoftware(ctx)
	if err != nil {
		return nil, err
	}
	cache := NewMembershipCache()
	out := []model.RestrictedSoftware{}
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		in, err := s.InScopeComputer(ctx, c, r.Scope, cache)
		if err != nil {
			return nil, err
		}
		if in {
			out = append(out, r)
		}
	}
	return out, nil
}

// RecordRestrictedEvent stores a violation report.
func (s *Store) RecordRestrictedEvent(ctx context.Context, restrictedID, computerID, ruleName, process, username string, killed bool) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO restricted_software_events (restricted_id, computer_id, rule_name, process, username, killed)
		VALUES (NULLIF($1,'')::uuid, $2, $3, $4, $5, $6)`, restrictedID, computerID, ruleName, process, username, killed)
	return err
}

// ListRestrictedEvents returns recent violations.
func (s *Store) ListRestrictedEvents(ctx context.Context, computerID string, limit, offset int) ([]model.RestrictedEvent, int, error) {
	limit, offset = clampPage(limit, offset)
	where, args := "", []any{}
	if computerID != "" {
		where = " WHERE e.computer_id = $1"
		args = append(args, computerID)
	}
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM restricted_software_events e`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id, COALESCE(e.restricted_id::text,''), e.computer_id, c.name, e.rule_name, e.process, e.username, e.killed, e.at
		FROM restricted_software_events e JOIN computers c ON c.id = e.computer_id`+where+
		fmt.Sprintf(` ORDER BY e.at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.RestrictedEvent{}
	for rows.Next() {
		var e model.RestrictedEvent
		if err := rows.Scan(&e.ID, &e.RestrictedID, &e.ComputerID, &e.ComputerName, &e.RuleName, &e.Process, &e.Username, &e.Killed, &e.At); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// ---- advanced searches ----

const searchCols = `id, name, criteria, display_fields, created_at, updated_at`

func scanSearch(row interface{ Scan(...any) error }) (*model.AdvancedSearch, error) {
	var a model.AdvancedSearch
	var crit db.JSON[[]model.Criterion]
	var fields db.JSON[[]string]
	if err := row.Scan(&a.ID, &a.Name, &crit, &fields, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	a.Criteria, a.DisplayFields = crit.V, fields.V
	if a.Criteria == nil {
		a.Criteria = []model.Criterion{}
	}
	if a.DisplayFields == nil {
		a.DisplayFields = []string{}
	}
	return &a, nil
}

// ListAdvancedSearches returns saved searches.
func (s *Store) ListAdvancedSearches(ctx context.Context) ([]model.AdvancedSearch, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+searchCols+` FROM advanced_searches ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AdvancedSearch{}
	for rows.Next() {
		a, err := scanSearch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// GetAdvancedSearch returns one saved search.
func (s *Store) GetAdvancedSearch(ctx context.Context, id string) (*model.AdvancedSearch, error) {
	return scanSearch(s.DB.QueryRowContext(ctx, `SELECT `+searchCols+` FROM advanced_searches WHERE id = $1`, id))
}

// CreateAdvancedSearch inserts a saved search.
func (s *Store) CreateAdvancedSearch(ctx context.Context, a model.AdvancedSearch) (*model.AdvancedSearch, error) {
	return scanSearch(s.DB.QueryRowContext(ctx, `INSERT INTO advanced_searches (name, criteria, display_fields) VALUES ($1,$2::jsonb,$3::jsonb) RETURNING `+searchCols,
		a.Name, jsonArg(a.Criteria), jsonArg(a.DisplayFields)))
}

// UpdateAdvancedSearch updates a saved search.
func (s *Store) UpdateAdvancedSearch(ctx context.Context, a model.AdvancedSearch) (*model.AdvancedSearch, error) {
	return scanSearch(s.DB.QueryRowContext(ctx, `UPDATE advanced_searches SET name=$2, criteria=$3::jsonb, display_fields=$4::jsonb, updated_at=now() WHERE id=$1 RETURNING `+searchCols,
		a.ID, a.Name, jsonArg(a.Criteria), jsonArg(a.DisplayFields)))
}

// DeleteAdvancedSearch removes a saved search.
func (s *Store) DeleteAdvancedSearch(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM advanced_searches WHERE id=$1`, id)
	return err
}

// RunAdvancedSearch evaluates criteria and returns matching computers.
func (s *Store) RunAdvancedSearch(ctx context.Context, criteria []model.Criterion, limit int) ([]model.Computer, error) {
	ids, err := s.PreviewCriteria(ctx, criteria)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []model.Computer{}, nil
	}
	if limit <= 0 {
		limit = 500
	}
	comps, _, err := s.ListComputers(ctx, ComputerFilter{IDs: ids, Limit: limit})
	return comps, err
}

// ---- mac apps (Installomator catalog) ----

const appCols = `id, name, label, category, description, update_behavior, options, enabled, scope, self_service, created_at, updated_at, deployment`

func scanApp(row interface{ Scan(...any) error }) (*model.MacApp, error) {
	var a model.MacApp
	var sc db.JSON[model.Scope]
	var ss db.JSON[model.PolicySelfService]
	var opts db.JSON[model.InstallomatorOptions]
	if err := row.Scan(&a.ID, &a.Name, &a.Label, &a.Category, &a.Description, &a.UpdateBehavior, &opts, &a.Enabled, &sc, &ss, &a.CreatedAt, &a.UpdatedAt, &a.Deployment); err != nil {
		return nil, wrapNotFound(err)
	}
	a.Scope, a.SelfService, a.Options = sc.V, ss.V, opts.V
	a.Scope.Normalize()
	return &a, nil
}

// ListMacApps returns catalog entries.
func (s *Store) ListMacApps(ctx context.Context) ([]model.MacApp, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+appCols+` FROM mac_apps ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.MacApp{}
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// GetMacApp returns one catalog entry.
func (s *Store) GetMacApp(ctx context.Context, id string) (*model.MacApp, error) {
	return scanApp(s.DB.QueryRowContext(ctx, `SELECT `+appCols+` FROM mac_apps WHERE id = $1`, id))
}

func normApp(a *model.MacApp) {
	if a.UpdateBehavior != "install_once" {
		a.UpdateBehavior = "keep_updated"
	}
	if a.SelfService.ButtonText == "" {
		a.SelfService.ButtonText = "Install"
	}
	if a.Deployment != "self_service" {
		a.Deployment = "automatic"
	} else {
		a.SelfService.Enabled = true // Self Service is the only way it is installed
	}
	a.Scope.Normalize()
	a.Label = strings.TrimSpace(a.Label)
}

// CreateMacApp inserts a catalog entry.
func (s *Store) CreateMacApp(ctx context.Context, a model.MacApp) (*model.MacApp, error) {
	normApp(&a)
	return scanApp(s.DB.QueryRowContext(ctx, `INSERT INTO mac_apps (name, label, category, description, update_behavior, enabled, scope, self_service, options, deployment)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,$10) RETURNING `+appCols,
		a.Name, a.Label, a.Category, a.Description, a.UpdateBehavior, a.Enabled, jsonArg(a.Scope), jsonArg(a.SelfService), jsonArg(a.Options), a.Deployment))
}

// UpdateMacApp updates a catalog entry.
func (s *Store) UpdateMacApp(ctx context.Context, a model.MacApp) (*model.MacApp, error) {
	normApp(&a)
	return scanApp(s.DB.QueryRowContext(ctx, `UPDATE mac_apps SET name=$2, label=$3, category=$4, description=$5, update_behavior=$6, enabled=$7,
		scope=$8::jsonb, self_service=$9::jsonb, options=$10::jsonb, deployment=$11, updated_at=now() WHERE id=$1 RETURNING `+appCols,
		a.ID, a.Name, a.Label, a.Category, a.Description, a.UpdateBehavior, a.Enabled, jsonArg(a.Scope), jsonArg(a.SelfService), jsonArg(a.Options), a.Deployment))
}

// DeleteMacApp removes a catalog entry.
func (s *Store) DeleteMacApp(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM mac_apps WHERE id=$1`, id)
	return err
}

// AppInstalledOn reports whether a Mac has the app: VibeMDM installed it (a completed run), or
// the agent's inventory lists an app of that name.
func (s *Store) AppInstalledOn(ctx context.Context, appID, computerID, appName string) (bool, error) {
	var ok bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM app_install_logs WHERE app_id=$1 AND computer_id=$2 AND status='completed')
		OR EXISTS (SELECT 1 FROM computer_apps WHERE computer_id=$2 AND lower(name) IN (lower($3), lower($3) || '.app'))`, appID, computerID, appName).Scan(&ok)
	return ok, err
}

// SelfServiceVisible reports whether a Mac app appears in Self Service.
func SelfServiceVisible(a model.MacApp) bool {
	return a.Deployment == "self_service" || a.SelfService.Enabled
}

// AppInstallDue reports whether an app should run on a computer now,
// considering update behavior and recent/pending runs.
func (s *Store) AppInstallDue(ctx context.Context, appID, computerID, behavior string) (bool, error) {
	var window string
	switch behavior {
	case "install_once":
		window = "100 years"
	default: // keep_updated: re-run weekly
		window = "7 days"
	}
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM app_install_logs WHERE app_id=$1 AND computer_id=$2 AND (
		(status = 'completed' AND COALESCE(completed_at, created_at) > now() - $3::interval)
		OR (status = 'pending' AND created_at > now() - interval '2 hours'))`, appID, computerID, window).Scan(&n)
	return n == 0, err
}

// CreateAppInstallLog records a pending app install handed to the agent.
func (s *Store) CreateAppInstallLog(ctx context.Context, appID, computerID, trigger string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO app_install_logs (app_id, computer_id, trigger) VALUES ($1,$2,$3) RETURNING id`, appID, computerID, trigger).Scan(&id)
	return id, err
}

// CompleteAppInstallLog finalizes an app install result.
func (s *Store) CompleteAppInstallLog(ctx context.Context, logID, computerID, status, logText string) error {
	if status != "completed" {
		status = "failed"
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE app_install_logs SET status=$3, log=$4, completed_at=now() WHERE id=$1 AND computer_id=$2`, logID, computerID, status, cleanText(logText))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AppStatusSummary counts install outcomes per app.
func (s *Store) AppStatusSummary(ctx context.Context, appID string) (map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT status, count(DISTINCT computer_id) FROM app_install_logs WHERE app_id = $1 GROUP BY status`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

// ListAppInstallLogs returns recent app install runs.
func (s *Store) ListAppInstallLogs(ctx context.Context, appID, computerID string, limit, offset int) ([]model.AppInstallLog, int, error) {
	limit, offset = clampPage(limit, offset)
	var where []string
	var args []any
	if appID != "" {
		args = append(args, appID)
		where = append(where, fmt.Sprintf("l.app_id = $%d", len(args)))
	}
	if computerID != "" {
		args = append(args, computerID)
		where = append(where, fmt.Sprintf("l.computer_id = $%d", len(args)))
	}
	w := ""
	if len(where) > 0 {
		w = " WHERE " + strings.Join(where, " AND ")
	}
	base := ` FROM app_install_logs l JOIN mac_apps a ON a.id = l.app_id JOIN computers c ON c.id = l.computer_id` + w
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*)`+base, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.DB.QueryContext(ctx, `SELECT l.id, l.app_id, a.name, l.computer_id, c.name, l.status, l.trigger, l.log, l.created_at, l.completed_at`+base+
		fmt.Sprintf(` ORDER BY l.created_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.AppInstallLog{}
	for rows.Next() {
		var l model.AppInstallLog
		var done sql.NullTime
		if err := rows.Scan(&l.ID, &l.AppID, &l.AppName, &l.ComputerID, &l.ComputerName, &l.Status, &l.Trigger, &l.Log, &l.CreatedAt, &done); err != nil {
			return nil, 0, err
		}
		l.CompletedAt = db.TimePtr(done)
		out = append(out, l)
	}
	return out, total, rows.Err()
}

// ExpireStaleAppInstalls fails app installs that never reported back.
func (s *Store) ExpireStaleAppInstalls(ctx context.Context) {
	s.DB.ExecContext(ctx, `UPDATE app_install_logs SET status='failed', log = log || E'\nTimed out waiting for agent result.', completed_at=now()
		WHERE status = 'pending' AND created_at < now() - interval '2 hours'`)
}
