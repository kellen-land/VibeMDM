package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/inspector"
	"github.com/vibemdm/vibemdm/internal/model"
)

// RecordCommandEvent appends a step to a command's timeline. Failures are
// logged by callers only; the inspector must never break command delivery.
func (s *Store) RecordCommandEvent(ctx context.Context, commandUUID, enrollmentID, event string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO mdm_command_events (command_uuid, enrollment_id, event, details) VALUES ($1, $2, $3, $4::jsonb)`,
		commandUUID, enrollmentID, event, jsonArg(details))
	return err
}

func scanEvents(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}) ([]model.CommandEvent, error) {
	defer rows.Close()
	out := []model.CommandEvent{}
	for rows.Next() {
		var e model.CommandEvent
		var d db.JSON[map[string]any]
		if err := rows.Scan(&e.ID, &e.CommandUUID, &e.EnrollmentID, &e.Event, &d, &e.At, &e.RequestType, &e.ComputerID, &e.ComputerName); err != nil {
			return nil, err
		}
		e.Details = d.V
		if e.Details == nil {
			e.Details = map[string]any{}
		}
		e.Description = inspector.Describe(e.Event, e.Details)
		out = append(out, e)
	}
	return out, rows.Err()
}

const eventSelect = `SELECT e.id, e.command_uuid, e.enrollment_id, e.event, e.details, e.at,
	COALESCE(c.request_type, m.request_type, ''), COALESCE(comp.id::text, ''), COALESCE(comp.name, '')
	FROM mdm_command_events e
	LEFT JOIN commands c ON c.command_uuid = e.command_uuid
	LEFT JOIN mdm_commands_meta m ON m.command_uuid = e.command_uuid
	LEFT JOIN computers comp ON comp.udid = split_part(e.enrollment_id, ':', 1)`

// CommandEvents returns a command's timeline, oldest first.
func (s *Store) CommandEvents(ctx context.Context, commandUUID string) ([]model.CommandEvent, error) {
	rows, err := s.DB.QueryContext(ctx, eventSelect+` WHERE e.command_uuid = $1 ORDER BY e.at, e.id`, commandUUID)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// ComputerTimeline returns command events for a computer, newest first.
func (s *Store) ComputerTimeline(ctx context.Context, computerID string, limit, offset int) ([]model.CommandEvent, int, error) {
	limit, offset = clampPage(limit, offset)
	var udid string
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(udid,'') FROM computers WHERE id = $1`, computerID).Scan(&udid); err != nil {
		return nil, 0, wrapNotFound(err)
	}
	if udid == "" {
		return []model.CommandEvent{}, 0, nil
	}
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM mdm_command_events WHERE split_part(enrollment_id, ':', 1) = $1`, udid).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, eventSelect+` WHERE split_part(e.enrollment_id, ':', 1) = $1 ORDER BY e.at DESC, e.id DESC LIMIT $2 OFFSET $3`, udid, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	ev, err := scanEvents(rows)
	return ev, total, err
}

// CommandSearchHit is a search result with redacted context.
type CommandSearchHit struct {
	CommandUUID  string `json:"command_uuid"`
	RequestType  string `json:"request_type"`
	ComputerID   string `json:"computer_id"`
	ComputerName string `json:"computer_name"`
	Status       string `json:"status"`
	MatchedIn    string `json:"matched_in"` // command | result
	Snippet      string `json:"snippet"`
	CreatedAt    string `json:"created_at"`
}

// SearchCommands searches command and result plists. Matching happens on the
// redacted text, so search can't be used to probe redacted secrets.
func (s *Store) SearchCommands(ctx context.Context, q, computerID string, limit int) ([]CommandSearchHit, error) {
	q = strings.TrimSpace(q)
	if len(q) < 2 {
		return []CommandSearchHit{}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{likeArg(q)}
	where := `(c.command ILIKE $1 OR COALESCE(r.result,'') ILIKE $1 OR c.request_type ILIKE $1)`
	if computerID != "" {
		args = append(args, computerID)
		where += fmt.Sprintf(` AND comp.id::text = $%d`, len(args))
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT q.command_uuid, c.request_type, COALESCE(comp.id::text,''), COALESCE(comp.name,''), COALESCE(r.status,'Pending'),
		c.command, COALESCE(r.result,''), q.created_at
		FROM enrollment_queue q JOIN commands c ON c.command_uuid = q.command_uuid
		LEFT JOIN command_results r ON r.command_uuid = q.command_uuid AND r.id = q.id
		LEFT JOIN computers comp ON comp.udid = split_part(q.id, ':', 1)
		WHERE `+where+` ORDER BY q.created_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	needle := strings.ToLower(q)
	out := []CommandSearchHit{}
	for rows.Next() && len(out) < limit {
		var h CommandSearchHit
		var cmd, res string
		var at any
		if err := rows.Scan(&h.CommandUUID, &h.RequestType, &h.ComputerID, &h.ComputerName, &h.Status, &cmd, &res, &at); err != nil {
			return nil, err
		}
		h.CreatedAt = fmt.Sprint(at)
		rc, rr := inspector.Redact(cmd), inspector.Redact(res)
		switch {
		case strings.Contains(strings.ToLower(h.RequestType), needle):
			h.MatchedIn, h.Snippet = "command", snippet(rc, h.RequestType)
		case strings.Contains(strings.ToLower(rc), needle):
			h.MatchedIn, h.Snippet = "command", snippet(rc, q)
		case strings.Contains(strings.ToLower(rr), needle):
			h.MatchedIn, h.Snippet = "result", snippet(rr, q)
		default:
			continue // matched only inside a redacted value
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func snippet(text, q string) string {
	i := strings.Index(strings.ToLower(text), strings.ToLower(q))
	if i < 0 {
		return ""
	}
	start, end := i-80, i+len(q)+80
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	return strings.Join(strings.Fields(text[start:end]), " ")
}

// PruneCommandEvents removes timeline events older than the given number of days.
func (s *Store) PruneCommandEvents(ctx context.Context, days int) {
	s.DB.ExecContext(ctx, `DELETE FROM mdm_command_events WHERE at < now() - make_interval(days => $1)`, days)
}
