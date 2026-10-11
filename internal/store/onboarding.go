package store

import (
	"context"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

const workflowCols = `id, name, enabled, priority, scope, config, created_at, updated_at`

func scanWorkflow(row interface{ Scan(...any) error }) (*model.OnboardingWorkflow, error) {
	var w model.OnboardingWorkflow
	var sc db.JSON[model.Scope]
	var cfg db.JSON[model.OnboardingConfig]
	if err := row.Scan(&w.ID, &w.Name, &w.Enabled, &w.Priority, &sc, &cfg, &w.CreatedAt, &w.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	w.Scope, w.Config = sc.V, cfg.V
	w.Scope.Normalize()
	if w.Config.Fields == nil {
		w.Config.Fields = []model.OnboardingField{}
	}
	if w.Config.Steps == nil {
		w.Config.Steps = []model.OnboardingStep{}
	}
	return &w, nil
}

// ListOnboardingWorkflows returns workflows by priority.
func (s *Store) ListOnboardingWorkflows(ctx context.Context) ([]model.OnboardingWorkflow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+workflowCols+` FROM onboarding_workflows ORDER BY priority, lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.OnboardingWorkflow{}
	for rows.Next() {
		w, err := scanWorkflow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

// GetOnboardingWorkflow loads a workflow.
func (s *Store) GetOnboardingWorkflow(ctx context.Context, id string) (*model.OnboardingWorkflow, error) {
	return scanWorkflow(s.DB.QueryRowContext(ctx, `SELECT `+workflowCols+` FROM onboarding_workflows WHERE id = $1`, id))
}

// SaveOnboardingWorkflow creates (empty ID) or updates a workflow.
func (s *Store) SaveOnboardingWorkflow(ctx context.Context, w model.OnboardingWorkflow) (*model.OnboardingWorkflow, error) {
	w.Scope.Normalize()
	if w.ID == "" {
		return scanWorkflow(s.DB.QueryRowContext(ctx, `INSERT INTO onboarding_workflows (name, enabled, priority, scope, config)
			VALUES ($1,$2,$3,$4::jsonb,$5::jsonb) RETURNING `+workflowCols, w.Name, w.Enabled, w.Priority, jsonArg(w.Scope), jsonArg(w.Config)))
	}
	return scanWorkflow(s.DB.QueryRowContext(ctx, `UPDATE onboarding_workflows SET name=$2, enabled=$3, priority=$4, scope=$5::jsonb, config=$6::jsonb,
		updated_at=now() WHERE id=$1 RETURNING `+workflowCols, w.ID, w.Name, w.Enabled, w.Priority, jsonArg(w.Scope), jsonArg(w.Config)))
}

// DeleteOnboardingWorkflow removes a workflow and its runs.
func (s *Store) DeleteOnboardingWorkflow(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM onboarding_workflows WHERE id = $1`, id)
	return err
}

// HasOnboardingRun reports whether a computer already has a run for a workflow.
func (s *Store) HasOnboardingRun(ctx context.Context, workflowID, computerID string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM onboarding_runs WHERE workflow_id = $1 AND computer_id = $2`, workflowID, computerID).Scan(&n)
	return n > 0, err
}

// CreateOnboardingRun records a queued run (fails if one exists).
func (s *Store) CreateOnboardingRun(ctx context.Context, workflowID, computerID string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO onboarding_runs (workflow_id, computer_id) VALUES ($1,$2) RETURNING id`, workflowID, computerID).Scan(&id)
	return id, err
}

// SetOnboardingRunTask links the agent task.
func (s *Store) SetOnboardingRunTask(ctx context.Context, runID, taskID string) {
	s.DB.ExecContext(ctx, `UPDATE onboarding_runs SET task_id = $2 WHERE id = $1`, runID, taskID)
}

// StartOnboardingRun marks a run running and stores the collected user entry.
func (s *Store) StartOnboardingRun(ctx context.Context, runID, computerID string, entry map[string]string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE onboarding_runs SET status='running', user_entry=$3::jsonb, started_at=COALESCE(started_at, now())
		WHERE id=$1 AND computer_id=$2 AND status IN ('queued','running')`, runID, computerID, jsonArg(entry))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CompleteOnboardingRun stores the final result.
func (s *Store) CompleteOnboardingRun(ctx context.Context, runID, computerID string, sub model.AgentOnboardingSubmit) error {
	status := "completed"
	if sub.Status == "failed" {
		status = "failed"
	}
	if sub.Steps == nil {
		sub.Steps = []model.OnboardingStepResult{}
	}
	if sub.UserEntry == nil {
		sub.UserEntry = map[string]string{}
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE onboarding_runs SET status=$3, steps=$4::jsonb, log=$5, completed_at=now(), started_at=COALESCE(started_at, now()),
		user_entry = CASE WHEN $6::jsonb = '{}'::jsonb THEN user_entry ELSE $6::jsonb END
		WHERE id=$1 AND computer_id=$2 AND status IN ('queued','running')`, runID, computerID, status, jsonArg(sub.Steps), cleanText(truncate(sub.Log, 200000)), jsonArg(sub.UserEntry))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// FailOnboardingRunByTask closes a run whose task failed.
func (s *Store) FailOnboardingRunByTask(ctx context.Context, taskID, msg string) {
	s.DB.ExecContext(ctx, `UPDATE onboarding_runs SET status='failed', log = log || $2, completed_at=now() WHERE task_id=$1 AND status IN ('queued','running')`,
		taskID, "\n"+truncate(msg, 2000))
}

// ListOnboardingRuns returns runs for a workflow and/or computer.
func (s *Store) ListOnboardingRuns(ctx context.Context, workflowID, computerID string, limit int) ([]model.OnboardingRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id, r.workflow_id, w.name, r.computer_id, COALESCE(c.name,''), r.status, r.user_entry, r.steps, r.log,
		r.created_at, r.started_at, r.completed_at FROM onboarding_runs r JOIN onboarding_workflows w ON w.id = r.workflow_id
		JOIN computers c ON c.id = r.computer_id
		WHERE ($1 = '' OR r.workflow_id::text = $1) AND ($2 = '' OR r.computer_id::text = $2) ORDER BY r.created_at DESC LIMIT `+itoa(limit), workflowID, computerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.OnboardingRun{}
	for rows.Next() {
		var r model.OnboardingRun
		var entry db.JSON[map[string]string]
		var steps db.JSON[[]model.OnboardingStepResult]
		if err := rows.Scan(&r.ID, &r.WorkflowID, &r.WorkflowName, &r.ComputerID, &r.ComputerName, &r.Status, &entry, &steps, &r.Log,
			&r.CreatedAt, &r.StartedAt, &r.CompletedAt); err != nil {
			return nil, err
		}
		r.UserEntry, r.Steps = entry.V, steps.V
		if r.UserEntry == nil {
			r.UserEntry = map[string]string{}
		}
		if r.Steps == nil {
			r.Steps = []model.OnboardingStepResult{}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ResetOnboardingRun deletes a computer's run so the workflow runs again.
func (s *Store) ResetOnboardingRun(ctx context.Context, workflowID, computerID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM onboarding_runs WHERE workflow_id = $1 AND computer_id = $2`, workflowID, computerID)
	return err
}
