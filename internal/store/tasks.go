package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

const taskCols = `t.id, t.computer_id, c.name, t.type, t.payload, t.status, t.result, t.created_by, t.created_at, t.sent_at, t.completed_at`

func scanTask(row interface{ Scan(...any) error }) (*model.AgentTask, error) {
	var t model.AgentTask
	var payload, result db.JSON[map[string]any]
	var sent, done sql.NullTime
	if err := row.Scan(&t.ID, &t.ComputerID, &t.ComputerName, &t.Type, &payload, &t.Status, &result, &t.CreatedBy, &t.CreatedAt, &sent, &done); err != nil {
		return nil, wrapNotFound(err)
	}
	t.Payload, t.Result = payload.V, result.V
	if t.Payload == nil {
		t.Payload = map[string]any{}
	}
	if t.Result == nil {
		t.Result = map[string]any{}
	}
	t.SentAt, t.CompletedAt = db.TimePtr(sent), db.TimePtr(done)
	return &t, nil
}

// CreateTask queues a task for a computer.
func (s *Store) CreateTask(ctx context.Context, computerID, typ string, payload map[string]any, createdBy string) (*model.AgentTask, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO agent_tasks (computer_id, type, payload, created_by) VALUES ($1,$2,$3::jsonb,$4) RETURNING id`, computerID, typ, jsonArg(payload), createdBy).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetTask(ctx, id)
}

// GetTask returns a task.
func (s *Store) GetTask(ctx context.Context, id string) (*model.AgentTask, error) {
	return scanTask(s.DB.QueryRowContext(ctx, `SELECT `+taskCols+` FROM agent_tasks t JOIN computers c ON c.id = t.computer_id WHERE t.id = $1`, id))
}

// ClaimQueuedTasks returns queued tasks for a computer and marks them sent.
func (s *Store) ClaimQueuedTasks(ctx context.Context, computerID string) ([]model.AgentTask, error) {
	rows, err := s.DB.QueryContext(ctx, `UPDATE agent_tasks SET status='sent', sent_at=now() WHERE id IN (
		SELECT id FROM agent_tasks WHERE computer_id=$1 AND status='queued' ORDER BY created_at FOR UPDATE SKIP LOCKED) RETURNING id`, computerID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := []model.AgentTask{}
	for _, id := range ids {
		t, err := s.GetTask(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, nil
}

// HasQueuedTasks reports whether a computer has queued tasks.
func (s *Store) HasQueuedTasks(ctx context.Context, computerID string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM agent_tasks WHERE computer_id=$1 AND status='queued'`, computerID).Scan(&n)
	return n > 0, err
}

// CompleteTask records a task result.
func (s *Store) CompleteTask(ctx context.Context, id, computerID string, res model.AgentTaskResult) error {
	status := res.Status
	if status != "completed" && status != "failed" {
		status = "failed"
	}
	out, err := s.DB.ExecContext(ctx, `UPDATE agent_tasks SET status=$3, result=$4::jsonb, completed_at=now() WHERE id=$1 AND computer_id=$2 AND status IN ('queued','sent')`,
		id, computerID, status, jsonArg(res))
	if err != nil {
		return err
	}
	if n, _ := out.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CancelTask cancels a queued task.
func (s *Store) CancelTask(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE agent_tasks SET status='cancelled', completed_at=now() WHERE id=$1 AND status IN ('queued','sent')`, id)
	return err
}

// TaskFilter filters tasks.
type TaskFilter struct {
	ComputerID string
	Status     string
	Type       string
	Limit      int
	Offset     int
}

// ListTasks lists tasks.
func (s *Store) ListTasks(ctx context.Context, f TaskFilter) ([]model.AgentTask, int, error) {
	f.Limit, f.Offset = clampPage(f.Limit, f.Offset)
	var where []string
	var args []any
	if f.ComputerID != "" {
		args = append(args, f.ComputerID)
		where = append(where, fmt.Sprintf("t.computer_id = $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("t.status = $%d", len(args)))
	}
	if f.Type != "" {
		args = append(args, f.Type)
		where = append(where, fmt.Sprintf("t.type = $%d", len(args)))
	}
	w := ""
	if len(where) > 0 {
		w = " WHERE " + strings.Join(where, " AND ")
	}
	base := ` FROM agent_tasks t JOIN computers c ON c.id = t.computer_id` + w
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*)`+base, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+taskCols+base+fmt.Sprintf(` ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.AgentTask{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *t)
	}
	return out, total, rows.Err()
}

// ExpireStaleTasks fails tasks that were sent but never completed.
func (s *Store) ExpireStaleTasks(ctx context.Context) {
	s.DB.ExecContext(ctx, `UPDATE agent_tasks SET status='failed', result = jsonb_build_object('error','Timed out waiting for agent result'), completed_at=now()
		WHERE status='sent' AND sent_at < now() - interval '2 hours'`)
}
