package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

const baselineCols = `id, name, description, benchmark, enabled, scope, extra_rules, exclusions, odv_overrides, scan_interval_hours,
	auto_remediate, deploy_profile, profile_id, created_at, updated_at`

func scanBaseline(row interface{ Scan(...any) error }) (*model.ComplianceBaseline, error) {
	var b model.ComplianceBaseline
	var sc db.JSON[model.Scope]
	var extra db.JSON[[]string]
	var excl db.JSON[map[string]string]
	var odv db.JSON[map[string]any]
	if err := row.Scan(&b.ID, &b.Name, &b.Description, &b.Benchmark, &b.Enabled, &sc, &extra, &excl, &odv, &b.ScanIntervalHours,
		&b.AutoRemediate, &b.DeployProfile, &b.ProfileID, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	b.Scope, b.ExtraRules, b.Exclusions, b.ODVOverrides = sc.V, extra.V, excl.V, odv.V
	b.Scope.Normalize()
	if b.ExtraRules == nil {
		b.ExtraRules = []string{}
	}
	if b.Exclusions == nil {
		b.Exclusions = map[string]string{}
	}
	if b.ODVOverrides == nil {
		b.ODVOverrides = map[string]any{}
	}
	return &b, nil
}

// ListComplianceBaselines returns all baselines.
func (s *Store) ListComplianceBaselines(ctx context.Context) ([]model.ComplianceBaseline, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+baselineCols+` FROM compliance_baselines ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ComplianceBaseline{}
	for rows.Next() {
		b, err := scanBaseline(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// GetComplianceBaseline returns one baseline.
func (s *Store) GetComplianceBaseline(ctx context.Context, id string) (*model.ComplianceBaseline, error) {
	return scanBaseline(s.DB.QueryRowContext(ctx, `SELECT `+baselineCols+` FROM compliance_baselines WHERE id = $1`, id))
}

// SaveComplianceBaseline creates (empty ID) or updates a baseline. Callers validate first.
func (s *Store) SaveComplianceBaseline(ctx context.Context, b model.ComplianceBaseline) (*model.ComplianceBaseline, error) {
	b.Scope.Normalize()
	if b.ExtraRules == nil {
		b.ExtraRules = []string{}
	}
	if b.Exclusions == nil {
		b.Exclusions = map[string]string{}
	}
	if b.ODVOverrides == nil {
		b.ODVOverrides = map[string]any{}
	}
	if b.ID == "" {
		return scanBaseline(s.DB.QueryRowContext(ctx, `INSERT INTO compliance_baselines (name, description, benchmark, enabled, scope, extra_rules,
			exclusions, odv_overrides, scan_interval_hours, auto_remediate, deploy_profile)
			VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7::jsonb,$8::jsonb,$9,$10,$11) RETURNING `+baselineCols,
			b.Name, b.Description, b.Benchmark, b.Enabled, jsonArg(b.Scope), jsonArg(b.ExtraRules), jsonArg(b.Exclusions), jsonArg(b.ODVOverrides),
			b.ScanIntervalHours, b.AutoRemediate, b.DeployProfile))
	}
	return scanBaseline(s.DB.QueryRowContext(ctx, `UPDATE compliance_baselines SET name=$2, description=$3, benchmark=$4, enabled=$5, scope=$6::jsonb,
		extra_rules=$7::jsonb, exclusions=$8::jsonb, odv_overrides=$9::jsonb, scan_interval_hours=$10, auto_remediate=$11, deploy_profile=$12,
		updated_at=now() WHERE id=$1 RETURNING `+baselineCols,
		b.ID, b.Name, b.Description, b.Benchmark, b.Enabled, jsonArg(b.Scope), jsonArg(b.ExtraRules), jsonArg(b.Exclusions), jsonArg(b.ODVOverrides),
		b.ScanIntervalHours, b.AutoRemediate, b.DeployProfile))
}

// SetComplianceProfile links the generated profile to a baseline.
func (s *Store) SetComplianceProfile(ctx context.Context, baselineID string, profileID *string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE compliance_baselines SET profile_id = $2 WHERE id = $1`, baselineID, profileID)
	return err
}

// DeleteComplianceBaseline removes a baseline with its scans and results.
func (s *Store) DeleteComplianceBaseline(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM compliance_baselines WHERE id = $1`, id)
	return err
}

const scanCols = `s.id, s.baseline_id, s.computer_id, COALESCE(c.name, ''), s.task_id, s.trigger, s.remediate, s.status, s.os_version,
	s.passed, s.failed, s.remediated, s.errors, s.total, s.duration_ms, s.error, s.requested_at, s.completed_at`

func scanScan(row interface{ Scan(...any) error }) (*model.ComplianceScan, error) {
	var x model.ComplianceScan
	if err := row.Scan(&x.ID, &x.BaselineID, &x.ComputerID, &x.ComputerName, &x.TaskID, &x.Trigger, &x.Remediate, &x.Status, &x.OSVersion,
		&x.Passed, &x.Failed, &x.Remediated, &x.Errors, &x.Total, &x.DurationMS, &x.Error, &x.RequestedAt, &x.CompletedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	return &x, nil
}

// CreateComplianceScan records a requested scan.
func (s *Store) CreateComplianceScan(ctx context.Context, baselineID, computerID, trigger string, remediate bool) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO compliance_scans (baseline_id, computer_id, trigger, remediate) VALUES ($1,$2,$3,$4) RETURNING id`,
		baselineID, computerID, trigger, remediate).Scan(&id)
	return id, err
}

// SetComplianceScanTask links the agent task that carries a scan.
func (s *Store) SetComplianceScanTask(ctx context.Context, scanID, taskID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE compliance_scans SET task_id = $2 WHERE id = $1`, scanID, taskID)
	return err
}

// GetComplianceScan loads a scan.
func (s *Store) GetComplianceScan(ctx context.Context, id string) (*model.ComplianceScan, error) {
	return scanScan(s.DB.QueryRowContext(ctx, `SELECT `+scanCols+` FROM compliance_scans s LEFT JOIN computers c ON c.id = s.computer_id WHERE s.id = $1`, id))
}

// ListComplianceScans returns recent scans for a baseline and/or computer.
func (s *Store) ListComplianceScans(ctx context.Context, baselineID, computerID string, limit int) ([]model.ComplianceScan, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+scanCols+` FROM compliance_scans s LEFT JOIN computers c ON c.id = s.computer_id
		WHERE ($1 = '' OR s.baseline_id::text = $1) AND ($2 = '' OR s.computer_id::text = $2) ORDER BY s.requested_at DESC LIMIT `+itoa(limit), baselineID, computerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ComplianceScan{}
	for rows.Next() {
		x, err := scanScan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// ComplianceScanDue reports whether a computer needs a scheduled scan of a baseline.
func (s *Store) ComplianceScanDue(ctx context.Context, baselineID, computerID string, interval time.Duration) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM compliance_scans WHERE baseline_id = $1 AND computer_id = $2
		AND (status = 'pending' OR requested_at > $3)`, baselineID, computerID, time.Now().Add(-interval)).Scan(&n)
	return n == 0, err
}

// CompleteComplianceScan stores a scan's results. Only a pending scan owned by
// the computer is accepted. Results for rules no longer in the baseline are removed.
func (s *Store) CompleteComplianceScan(ctx context.Context, scanID, computerID string, sub model.AgentComplianceSubmit, expected map[string]string) (*model.ComplianceScan, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var baselineID string
	if err := tx.QueryRowContext(ctx, `SELECT baseline_id FROM compliance_scans WHERE id = $1 AND computer_id = $2 AND status = 'pending' FOR UPDATE`,
		scanID, computerID).Scan(&baselineID); err != nil {
		return nil, wrapNotFound(err)
	}
	counts := map[string]int{}
	ids := make([]string, 0, len(sub.Results))
	for _, r := range sub.Results {
		switch r.Status {
		case "pass", "fail", "error", "remediated":
		default:
			r.Status = "error"
		}
		if _, ok := expected[r.ID]; !ok {
			continue // not part of what we asked for
		}
		counts[r.Status]++
		ids = append(ids, r.ID)
		if _, err := tx.ExecContext(ctx, `INSERT INTO compliance_results (baseline_id, computer_id, rule_id, status, actual, expected, detail)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (baseline_id, computer_id, rule_id) DO UPDATE SET
				previous_status = compliance_results.status, status = EXCLUDED.status, actual = EXCLUDED.actual, expected = EXCLUDED.expected,
				detail = EXCLUDED.detail, checked_at = now(),
				changed_at = CASE WHEN compliance_results.status <> EXCLUDED.status THEN now() ELSE compliance_results.changed_at END`,
			baselineID, computerID, r.ID, r.Status, truncate(r.Actual, 2000), expected[r.ID], truncate(r.Detail, 2000)); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM compliance_results WHERE baseline_id = $1 AND computer_id = $2 AND NOT (rule_id = ANY($3))`,
		baselineID, computerID, "{"+joinQuoted(ids)+"}"); err != nil {
		return nil, err
	}
	status := "completed"
	if sub.Error != "" && len(ids) == 0 {
		status = "failed"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE compliance_scans SET status=$2, os_version=$3, passed=$4, failed=$5, remediated=$6, errors=$7, total=$8,
		duration_ms=$9, error=$10, completed_at=now() WHERE id=$1`,
		scanID, status, sub.OSVersion, counts["pass"], counts["fail"], counts["remediated"], counts["error"], len(ids), sub.DurationMS, truncate(sub.Error, 2000)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetComplianceScan(ctx, scanID)
}

// FailComplianceScan marks a pending scan failed (e.g. the agent task failed).
func (s *Store) FailComplianceScan(ctx context.Context, scanID, msg string) {
	s.DB.ExecContext(ctx, `UPDATE compliance_scans SET status = 'failed', error = $2, completed_at = now() WHERE id = $1 AND status = 'pending'`, scanID, truncate(msg, 2000))
}

// FailComplianceScanByTask closes the pending scan carried by a failed agent task.
func (s *Store) FailComplianceScanByTask(ctx context.Context, taskID, msg string) {
	s.DB.ExecContext(ctx, `UPDATE compliance_scans SET status = 'failed', error = $2, completed_at = now() WHERE task_id = $1 AND status = 'pending'`, taskID, truncate(msg, 2000))
}

// ExpireComplianceScans gives up on scans that never came back.
func (s *Store) ExpireComplianceScans(ctx context.Context) {
	s.DB.ExecContext(ctx, `UPDATE compliance_scans SET status = 'expired', completed_at = now(), error = 'no result from the agent within 24 hours'
		WHERE status = 'pending' AND requested_at < now() - interval '24 hours'`)
	s.DB.ExecContext(ctx, `DELETE FROM compliance_scans WHERE requested_at < now() - interval '90 days'`)
}

// ComplianceResults returns latest results for a baseline, optionally one computer.
func (s *Store) ComplianceResults(ctx context.Context, baselineID, computerID string) ([]model.ComplianceResult, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.baseline_id, r.computer_id, COALESCE(c.name, ''), r.rule_id, r.status, r.previous_status, r.actual,
		r.expected, r.detail, r.checked_at, r.changed_at FROM compliance_results r JOIN computers c ON c.id = r.computer_id
		WHERE ($1 = '' OR r.baseline_id::text = $1) AND ($2 = '' OR r.computer_id::text = $2) ORDER BY r.rule_id, lower(c.name)`, baselineID, computerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ComplianceResult{}
	for rows.Next() {
		var r model.ComplianceResult
		if err := rows.Scan(&r.BaselineID, &r.ComputerID, &r.ComputerName, &r.RuleID, &r.Status, &r.PreviousStatus, &r.Actual, &r.Expected,
			&r.Detail, &r.CheckedAt, &r.ChangedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ComplianceComputerSummary is one computer's latest standing on a baseline.
type ComplianceComputerSummary struct {
	ComputerID   string     `json:"computer_id"`
	ComputerName string     `json:"computer_name"`
	SerialNumber string     `json:"serial_number"`
	OSVersion    string     `json:"os_version"`
	Passed       int        `json:"passed"`
	Failed       int        `json:"failed"`
	Errors       int        `json:"errors"`
	Remediated   int        `json:"remediated"`
	Total        int        `json:"total"`
	Score        float64    `json:"score"` // percent passing (remediated counts as passing)
	LastScanAt   *time.Time `json:"last_scan_at"`
	LastStatus   string     `json:"last_scan_status"`
}

// ComplianceComputerSummaries aggregates results per computer for a baseline.
func (s *Store) ComplianceComputerSummaries(ctx context.Context, baselineID string) ([]ComplianceComputerSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id, c.name, COALESCE(c.serial_number, ''), c.os_version,
		count(*) FILTER (WHERE r.status = 'pass'), count(*) FILTER (WHERE r.status = 'fail'), count(*) FILTER (WHERE r.status = 'error'),
		count(*) FILTER (WHERE r.status = 'remediated'), count(r.rule_id),
		(SELECT max(completed_at) FROM compliance_scans x WHERE x.baseline_id = $1 AND x.computer_id = c.id),
		COALESCE((SELECT status FROM compliance_scans x WHERE x.baseline_id = $1 AND x.computer_id = c.id ORDER BY requested_at DESC LIMIT 1), '')
		FROM computers c LEFT JOIN compliance_results r ON r.computer_id = c.id AND r.baseline_id = $1
		WHERE EXISTS (SELECT 1 FROM compliance_scans x WHERE x.baseline_id = $1 AND x.computer_id = c.id)
		GROUP BY c.id ORDER BY lower(c.name)`, baselineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ComplianceComputerSummary{}
	for rows.Next() {
		var x ComplianceComputerSummary
		var last sql.NullTime
		if err := rows.Scan(&x.ComputerID, &x.ComputerName, &x.SerialNumber, &x.OSVersion, &x.Passed, &x.Failed, &x.Errors, &x.Remediated,
			&x.Total, &last, &x.LastStatus); err != nil {
			return nil, err
		}
		if last.Valid {
			x.LastScanAt = &last.Time
		}
		if x.Total > 0 {
			x.Score = float64(x.Passed+x.Remediated) * 100 / float64(x.Total)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ComplianceRuleCount is the fleet-wide tally for one rule.
type ComplianceRuleCount struct {
	RuleID     string `json:"rule_id"`
	Passed     int    `json:"passed"`
	Failed     int    `json:"failed"`
	Errors     int    `json:"errors"`
	Remediated int    `json:"remediated"`
	Drifted    int    `json:"drifted"` // passed before, failing now
}

// ComplianceRuleCounts tallies results per rule for a baseline.
func (s *Store) ComplianceRuleCounts(ctx context.Context, baselineID string) (map[string]ComplianceRuleCount, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT rule_id, count(*) FILTER (WHERE status = 'pass'), count(*) FILTER (WHERE status = 'fail'),
		count(*) FILTER (WHERE status = 'error'), count(*) FILTER (WHERE status = 'remediated'),
		count(*) FILTER (WHERE status = 'fail' AND previous_status IN ('pass','remediated'))
		FROM compliance_results WHERE baseline_id = $1 GROUP BY rule_id`, baselineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ComplianceRuleCount{}
	for rows.Next() {
		var x ComplianceRuleCount
		if err := rows.Scan(&x.RuleID, &x.Passed, &x.Failed, &x.Errors, &x.Remediated, &x.Drifted); err != nil {
			return nil, err
		}
		out[x.RuleID] = x
	}
	return out, rows.Err()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return strings.ToValidUTF8(s, "")
	}
	return strings.ToValidUTF8(s[:n], "")
}

func joinQuoted(ids []string) string {
	b := make([]byte, 0, len(ids)*24)
	for i, id := range ids {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '"')
		for _, r := range id {
			if r == '"' || r == '\\' {
				b = append(b, '\\')
			}
			b = append(b, string(r)...)
		}
		b = append(b, '"')
	}
	return string(b)
}
