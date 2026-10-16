package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

// CreateShellSession records a new (pending) remote shell session.
func (s *Store) CreateShellSession(ctx context.Context, computerID, actor, ip string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO shell_sessions (computer_id, actor, actor_ip, status) VALUES ($1,$2,$3,'pending') RETURNING id`,
		computerID, actor, ip).Scan(&id)
	return id, err
}

// MarkShellConnected flips a session to active.
func (s *Store) MarkShellConnected(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE shell_sessions SET status='active', connected_at=now() WHERE id=$1 AND status='pending'`, id)
	return err
}

// CloseShellSession finalizes a session with counters and transcript.
// Raw PTY output may hold NUL bytes or invalid UTF-8, which a TEXT column rejects; it is cleaned
// first so the record can always be written.
func (s *Store) CloseShellSession(ctx context.Context, id, status, reason, transcript string, in, out int64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE shell_sessions SET status=$2, reason=$3, transcript=$4, bytes_in=$5, bytes_out=$6, ended_at=now()
		WHERE id=$1 AND ended_at IS NULL`, id, status, reason, cleanText(transcript), in, out)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("shell session %s was already closed; transcript not recorded", id)
	}
	return nil
}

// ActiveShellCount returns the number of pending/active sessions for a computer.
func (s *Store) ActiveShellCount(ctx context.Context, computerID string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM shell_sessions WHERE computer_id=$1 AND status IN ('pending','active')`, computerID).Scan(&n)
	return n, err
}

// ListShellSessions returns recent shell sessions (optionally for one computer), transcript omitted.
func (s *Store) ListShellSessions(ctx context.Context, computerID string, limit, offset int) ([]model.ShellSession, int, error) {
	limit, offset = clampPage(limit, offset)
	where, args := "", []any{}
	if computerID != "" {
		where = " WHERE s.computer_id = $1"
		args = append(args, computerID)
	}
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM shell_sessions s`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.DB.QueryContext(ctx, `SELECT s.id, s.computer_id, c.name, s.actor, s.actor_ip, s.status, s.reason, s.bytes_in, s.bytes_out,
		s.started_at, s.connected_at, s.ended_at FROM shell_sessions s JOIN computers c ON c.id = s.computer_id`+where+
		` ORDER BY s.started_at DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.ShellSession{}
	for rows.Next() {
		var ss model.ShellSession
		var conn, end sql.NullTime
		if err := rows.Scan(&ss.ID, &ss.ComputerID, &ss.ComputerName, &ss.Actor, &ss.ActorIP, &ss.Status, &ss.Reason, &ss.BytesIn, &ss.BytesOut, &ss.StartedAt, &conn, &end); err != nil {
			return nil, 0, err
		}
		ss.ConnectedAt, ss.EndedAt = db.TimePtr(conn), db.TimePtr(end)
		out = append(out, ss)
	}
	return out, total, rows.Err()
}

// GetShellTranscript returns a session's recorded transcript.
func (s *Store) GetShellTranscript(ctx context.Context, id string) (model.ShellSession, string, error) {
	var ss model.ShellSession
	var conn, end sql.NullTime
	var transcript string
	err := s.DB.QueryRowContext(ctx, `SELECT s.id, s.computer_id, c.name, s.actor, s.actor_ip, s.status, s.reason, s.bytes_in, s.bytes_out,
		s.started_at, s.connected_at, s.ended_at, s.transcript FROM shell_sessions s JOIN computers c ON c.id = s.computer_id WHERE s.id=$1`, id).
		Scan(&ss.ID, &ss.ComputerID, &ss.ComputerName, &ss.Actor, &ss.ActorIP, &ss.Status, &ss.Reason, &ss.BytesIn, &ss.BytesOut, &ss.StartedAt, &conn, &end, &transcript)
	if err != nil {
		return ss, "", wrapNotFound(err)
	}
	ss.ConnectedAt, ss.EndedAt = db.TimePtr(conn), db.TimePtr(end)
	return ss, transcript, nil
}

// ExpireStaleShellSessions closes sessions that never finalized. Sessions may last up to 4 hours
// (shell.MaxSessionDuration), so only rows older than that are abandoned; expiring a live one would drop
// its transcript and free the one-shell-per-Mac slot while it still runs.
func (s *Store) ExpireStaleShellSessions(ctx context.Context) {
	s.DB.ExecContext(ctx, `UPDATE shell_sessions SET status='error', reason='server restarted or session abandoned', ended_at=now()
		WHERE status IN ('pending','active') AND started_at < now() - interval '4 hours 5 minutes'`)
}

var _ = time.Now
