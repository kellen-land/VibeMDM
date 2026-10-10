package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

const policyCols = `id, name, enabled, category, triggers, frequency, scope, scripts, packages, apps, run_command, update_inventory, reboot, user_interaction, self_service, created_at, updated_at`

func scanPolicy(row interface{ Scan(...any) error }) (*model.Policy, error) {
	var p model.Policy
	var trig db.JSON[model.PolicyTriggers]
	var scope db.JSON[model.Scope]
	var scripts db.JSON[[]model.PolicyScript]
	var pkgs db.JSON[[]model.PolicyPackage]
	var apps db.JSON[[]model.PolicyApp]
	var reboot db.JSON[model.PolicyReboot]
	var ui db.JSON[model.PolicyUserInteraction]
	var ss db.JSON[model.PolicySelfService]
	if err := row.Scan(&p.ID, &p.Name, &p.Enabled, &p.Category, &trig, &p.Frequency, &scope, &scripts, &pkgs, &apps, &p.RunCommand, &p.UpdateInventory, &reboot, &ui, &ss, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	p.Triggers, p.Scope, p.Scripts, p.Packages, p.Reboot, p.UserInteraction, p.SelfService = trig.V, scope.V, scripts.V, pkgs.V, reboot.V, ui.V, ss.V
	p.Scope.Normalize()
	if p.Scripts == nil {
		p.Scripts = []model.PolicyScript{}
	}
	if p.Packages == nil {
		p.Packages = []model.PolicyPackage{}
	}
	p.Apps = apps.V
	if p.Apps == nil {
		p.Apps = []model.PolicyApp{}
	}
	return &p, nil
}

func normPolicy(p *model.Policy) {
	p.Scope.Normalize()
	switch p.Frequency {
	case "ongoing", "once_per_computer", "once_per_day", "once_per_week", "once_per_month":
	default:
		p.Frequency = "once_per_computer"
	}
	if p.Scripts == nil {
		p.Scripts = []model.PolicyScript{}
	}
	for i := range p.Scripts {
		if p.Scripts[i].Priority != "before" {
			p.Scripts[i].Priority = "after"
		}
		params := make([]string, 8)
		copy(params, p.Scripts[i].Parameters)
		p.Scripts[i].Parameters = params
	}
	if p.Packages == nil {
		p.Packages = []model.PolicyPackage{}
	}
	for i := range p.Packages {
		if p.Packages[i].Action == "" {
			p.Packages[i].Action = "install"
		}
	}
	if p.SelfService.ButtonText == "" {
		p.SelfService.ButtonText = "Install"
	}
	if p.Apps == nil {
		p.Apps = []model.PolicyApp{}
	}
}

// ListPolicies returns all policies.
func (s *Store) ListPolicies(ctx context.Context) ([]model.Policy, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+policyCols+` FROM policies ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Policy{}
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetPolicy returns a policy.
func (s *Store) GetPolicy(ctx context.Context, id string) (*model.Policy, error) {
	return scanPolicy(s.DB.QueryRowContext(ctx, `SELECT `+policyCols+` FROM policies WHERE id = $1`, id))
}

// CreatePolicy inserts a policy.
func (s *Store) CreatePolicy(ctx context.Context, p model.Policy) (*model.Policy, error) {
	normPolicy(&p)
	return scanPolicy(s.DB.QueryRowContext(ctx, `INSERT INTO policies (name, enabled, category, triggers, frequency, scope, scripts, packages, run_command, update_inventory, reboot, user_interaction, self_service, apps)
		VALUES ($1,$2,$3,$4::jsonb,$5,$6::jsonb,$7::jsonb,$8::jsonb,$9,$10,$11::jsonb,$12::jsonb,$13::jsonb,$14::jsonb) RETURNING `+policyCols,
		p.Name, p.Enabled, p.Category, jsonArg(p.Triggers), p.Frequency, jsonArg(p.Scope), jsonArg(p.Scripts), jsonArg(p.Packages), p.RunCommand, p.UpdateInventory,
		jsonArg(p.Reboot), jsonArg(p.UserInteraction), jsonArg(p.SelfService), jsonArg(p.Apps)))
}

// UpdatePolicy updates a policy.
func (s *Store) UpdatePolicy(ctx context.Context, p model.Policy) (*model.Policy, error) {
	normPolicy(&p)
	return scanPolicy(s.DB.QueryRowContext(ctx, `UPDATE policies SET name=$2, enabled=$3, category=$4, triggers=$5::jsonb, frequency=$6, scope=$7::jsonb, scripts=$8::jsonb, packages=$9::jsonb,
		run_command=$10, update_inventory=$11, reboot=$12::jsonb, user_interaction=$13::jsonb, self_service=$14::jsonb, apps=$15::jsonb, updated_at=now() WHERE id=$1 RETURNING `+policyCols,
		p.ID, p.Name, p.Enabled, p.Category, jsonArg(p.Triggers), p.Frequency, jsonArg(p.Scope), jsonArg(p.Scripts), jsonArg(p.Packages), p.RunCommand, p.UpdateInventory,
		jsonArg(p.Reboot), jsonArg(p.UserInteraction), jsonArg(p.SelfService), jsonArg(p.Apps)))
}

// DeletePolicy removes a policy.
func (s *Store) DeletePolicy(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM policies WHERE id=$1`, id)
	return err
}

// ---- policy logs ----

const policyLogCols = `l.id, l.policy_id, p.name, l.computer_id, c.name, l.trigger, l.status, l.started_at, l.completed_at, l.log, l.steps, l.created_at`

func scanPolicyLog(row interface{ Scan(...any) error }) (*model.PolicyLog, error) {
	var l model.PolicyLog
	var st, ct sql.NullTime
	var steps db.JSON[[]model.PolicyStep]
	if err := row.Scan(&l.ID, &l.PolicyID, &l.PolicyName, &l.ComputerID, &l.ComputerName, &l.Trigger, &l.Status, &st, &ct, &l.Log, &steps, &l.CreatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	l.StartedAt, l.CompletedAt = db.TimePtr(st), db.TimePtr(ct)
	l.Steps = steps.V
	if l.Steps == nil {
		l.Steps = []model.PolicyStep{}
	}
	return &l, nil
}

// CreatePendingPolicyLog creates a pending log row when a policy is handed to the agent.
func (s *Store) CreatePendingPolicyLog(ctx context.Context, policyID, computerID, trigger string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO policy_logs (policy_id, computer_id, trigger, status) VALUES ($1,$2,$3,'pending') RETURNING id`, policyID, computerID, trigger).Scan(&id)
	return id, err
}

// CompletePolicyLog records the result of a policy run.
func (s *Store) CompletePolicyLog(ctx context.Context, logID, computerID string, sub model.AgentPolicyLogSubmit) error {
	status := sub.Status
	if status != "completed" && status != "failed" {
		status = "failed"
	}
	if sub.Steps == nil {
		sub.Steps = []model.PolicyStep{}
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE policy_logs SET status=$3, started_at=$4, completed_at=$5, log=$6, steps=$7::jsonb, trigger = CASE WHEN $8 <> '' THEN $8 ELSE trigger END
		WHERE id=$1 AND computer_id=$2`, logID, computerID, status, sub.StartedAt, sub.CompletedAt, sub.Log, jsonArg(sub.Steps), sub.Trigger)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// InsertPolicyLog inserts a complete log row directly (ad-hoc runs).
func (s *Store) InsertPolicyLog(ctx context.Context, policyID, computerID string, sub model.AgentPolicyLogSubmit) error {
	status := sub.Status
	if status != "completed" && status != "failed" {
		status = "failed"
	}
	if sub.Steps == nil {
		sub.Steps = []model.PolicyStep{}
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO policy_logs (policy_id, computer_id, trigger, status, started_at, completed_at, log, steps) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb)`,
		policyID, computerID, sub.Trigger, status, sub.StartedAt, sub.CompletedAt, sub.Log, jsonArg(sub.Steps))
	return err
}

// PolicyLogFilter filters logs.
type PolicyLogFilter struct {
	PolicyID   string
	ComputerID string
	Status     string
	Limit      int
	Offset     int
}

// ListPolicyLogs returns logs.
func (s *Store) ListPolicyLogs(ctx context.Context, f PolicyLogFilter) ([]model.PolicyLog, int, error) {
	f.Limit, f.Offset = clampPage(f.Limit, f.Offset)
	var where []string
	var args []any
	if f.PolicyID != "" {
		args = append(args, f.PolicyID)
		where = append(where, fmt.Sprintf("l.policy_id = $%d", len(args)))
	}
	if f.ComputerID != "" {
		args = append(args, f.ComputerID)
		where = append(where, fmt.Sprintf("l.computer_id = $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("l.status = $%d", len(args)))
	}
	w := ""
	if len(where) > 0 {
		w = " WHERE " + strings.Join(where, " AND ")
	}
	base := ` FROM policy_logs l JOIN policies p ON p.id = l.policy_id JOIN computers c ON c.id = l.computer_id` + w
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*)`+base, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+policyLogCols+base+fmt.Sprintf(` ORDER BY l.created_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.PolicyLog{}
	for rows.Next() {
		l, err := scanPolicyLog(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *l)
	}
	return out, total, rows.Err()
}

// GetPolicyLog returns a single log.
func (s *Store) GetPolicyLog(ctx context.Context, id string) (*model.PolicyLog, error) {
	return scanPolicyLog(s.DB.QueryRowContext(ctx, `SELECT `+policyLogCols+` FROM policy_logs l JOIN policies p ON p.id = l.policy_id JOIN computers c ON c.id = l.computer_id WHERE l.id = $1`, id))
}

// FlushPolicyLogs deletes logs for a policy (optionally for one computer), re-arming frequency.
func (s *Store) FlushPolicyLogs(ctx context.Context, policyID, computerID string) (int64, error) {
	var res sql.Result
	var err error
	if computerID != "" {
		res, err = s.DB.ExecContext(ctx, `DELETE FROM policy_logs WHERE policy_id=$1 AND computer_id=$2`, policyID, computerID)
	} else {
		res, err = s.DB.ExecContext(ctx, `DELETE FROM policy_logs WHERE policy_id=$1`, policyID)
	}
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// FrequencySatisfied reports whether a policy's frequency allows a run now for a computer.
func (s *Store) FrequencySatisfied(ctx context.Context, policyID, computerID, frequency string) (bool, error) {
	if frequency == "ongoing" {
		// still avoid duplicates while a run is pending
		var pending int
		if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM policy_logs WHERE policy_id=$1 AND computer_id=$2 AND status IN ('pending','running') AND created_at > now() - interval '1 hour'`, policyID, computerID).Scan(&pending); err != nil {
			return false, err
		}
		return pending == 0, nil
	}
	var window string
	switch frequency {
	case "once_per_day":
		window = "1 day"
	case "once_per_week":
		window = "7 days"
	case "once_per_month":
		window = "30 days"
	default:
		window = "100 years"
	}
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM policy_logs WHERE policy_id=$1 AND computer_id=$2 AND (
		(status = 'completed' AND COALESCE(completed_at, created_at) > now() - $3::interval)
		OR (status IN ('pending','running') AND created_at > now() - interval '1 hour'))`, policyID, computerID, window).Scan(&n)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// PolicyStatusSummary counts log statuses for a policy.
func (s *Store) PolicyStatusSummary(ctx context.Context, policyID string) (map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT status, count(DISTINCT computer_id) FROM policy_logs WHERE policy_id = $1 GROUP BY status`, policyID)
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

// ExpireStalePolicyLogs marks old pending logs as failed.
func (s *Store) ExpireStalePolicyLogs(ctx context.Context) {
	s.DB.ExecContext(ctx, `UPDATE policy_logs SET status='failed', log = log || E'\nTimed out waiting for agent result.', completed_at = now()
		WHERE status IN ('pending','running') AND created_at < now() - interval '2 hours'`)
}

var _ = time.Now
