package store

import (
	"bufio"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

// LogFilter selects log entries.
type LogFilter struct {
	Source     string // server | agent
	MinLevel   string // DEBUG | INFO | WARN | ERROR
	Component  string
	ComputerID string
	Q          string
	SinceID    int64 // tail: entries with id > SinceID, oldest first
	BeforeID   int64 // page back: entries with id < BeforeID, newest first
	From, To   *time.Time
	Limit      int
}

const levelRank = `CASE upper(level) WHEN 'DEBUG' THEN 0 WHEN 'INFO' THEN 1 WHEN 'WARN' THEN 2 WHEN 'ERROR' THEN 3 ELSE 1 END`

func rank(level string) int {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return 0
	case "WARN", "WARNING":
		return 2
	case "ERROR":
		return 3
	}
	return 1
}

func (f LogFilter) where() (string, string, []any) {
	table := "server_logs l"
	if f.Source == "agent" {
		table = "agent_logs l"
	}
	var conds []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.MinLevel != "" {
		add(levelRank+` >= $%d`, rank(f.MinLevel))
	}
	if f.Component != "" {
		add(`l.component = $%d`, f.Component)
	}
	if f.Source == "agent" && f.ComputerID != "" {
		add(`l.computer_id::text = $%d`, f.ComputerID)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		add(`l.search LIKE $%d`, likeArg(strings.ToLower(q)))
	}
	if f.SinceID > 0 {
		add(`l.id > $%d`, f.SinceID)
	}
	if f.BeforeID > 0 {
		add(`l.id < $%d`, f.BeforeID)
	}
	if f.From != nil {
		add(`l.at >= $%d`, *f.From)
	}
	if f.To != nil {
		add(`l.at < $%d`, *f.To)
	}
	w := ""
	if len(conds) > 0 {
		w = " WHERE " + strings.Join(conds, " AND ")
	}
	return table, w, args
}

func (s *Store) scanLogs(rows *sql.Rows, source string) ([]model.LogEntry, error) {
	defer rows.Close()
	out := []model.LogEntry{}
	for rows.Next() {
		e := model.LogEntry{Source: source}
		var attrs db.JSON[map[string]any]
		if err := rows.Scan(&e.ID, &e.At, &e.Level, &e.Component, &e.Msg, &attrs, &e.ComputerID, &e.ComputerName); err != nil {
			return nil, err
		}
		e.Attrs = attrs.V
		if e.Attrs == nil {
			e.Attrs = map[string]any{}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func logSelect(source string) string {
	if source == "agent" {
		return `SELECT l.id, l.at, l.level, l.component, l.msg, l.attrs, l.computer_id::text, COALESCE(c.name,'') `
	}
	return `SELECT l.id, l.at, l.level, l.component, l.msg, l.attrs, '', '' `
}

// QueryLogs returns log entries newest first, or oldest first when tailing (SinceID).
func (s *Store) QueryLogs(ctx context.Context, f LogFilter) ([]model.LogEntry, error) {
	if f.Source != "agent" {
		f.Source = "server"
	}
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 200
	}
	table, w, args := f.where()
	join := ""
	if f.Source == "agent" {
		join = " LEFT JOIN computers c ON c.id = l.computer_id"
	}
	order := " ORDER BY l.id DESC"
	if f.SinceID > 0 {
		order = " ORDER BY l.id ASC"
	}
	args = append(args, f.Limit)
	rows, err := s.DB.QueryContext(ctx, logSelect(f.Source)+`FROM `+table+join+w+order+fmt.Sprintf(` LIMIT $%d`, len(args)), args...)
	if err != nil {
		return nil, err
	}
	return s.scanLogs(rows, f.Source)
}

// LogComponents lists components seen in the last 7 days.
func (s *Store) LogComponents(ctx context.Context, source string) ([]string, error) {
	table := "server_logs"
	if source == "agent" {
		table = "agent_logs"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT component FROM `+table+` WHERE at > now() - interval '7 days' AND component <> '' ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// InsertAgentLogs stores a batch of agent log entries for a computer.
func (s *Store) InsertAgentLogs(ctx context.Context, computerID string, entries []model.AgentLogEntry) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	if len(entries) > 2000 {
		entries = entries[len(entries)-2000:]
	}
	for i := range entries {
		if len(entries[i].Msg) > 4096 {
			entries[i].Msg = entries[i].Msg[:4096] + "…"
		}
		if entries[i].At.IsZero() || entries[i].At.After(time.Now().Add(time.Hour)) {
			entries[i].At = time.Now().UTC()
		}
		if entries[i].Attrs == nil {
			entries[i].Attrs = map[string]any{}
		}
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO agent_logs (computer_id, at, level, component, msg, attrs)
		SELECT $1, at, upper(COALESCE(level,'INFO')), COALESCE(component,''), COALESCE(msg,''), COALESCE(attrs,'{}'::jsonb)
		FROM jsonb_to_recordset($2::jsonb) AS x(at timestamptz, level text, component text, msg text, attrs jsonb)`, computerID, jsonArg(entries))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ExportLogs streams matching entries as NDJSON (oldest first).
func (s *Store) ExportLogs(ctx context.Context, f LogFilter, w io.Writer) (int, error) {
	if f.Source != "agent" {
		f.Source = "server"
	}
	f.SinceID, f.BeforeID = 0, 0
	table, wh, args := f.where()
	join := ""
	if f.Source == "agent" {
		join = " LEFT JOIN computers c ON c.id = l.computer_id"
	}
	rows, err := s.DB.QueryContext(ctx, logSelect(f.Source)+`FROM `+table+join+wh+` ORDER BY l.id ASC`, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	bw := bufio.NewWriterSize(w, 64<<10)
	enc := json.NewEncoder(bw)
	n := 0
	for rows.Next() {
		e := model.LogEntry{Source: f.Source}
		var attrs db.JSON[map[string]any]
		if err := rows.Scan(&e.ID, &e.At, &e.Level, &e.Component, &e.Msg, &attrs, &e.ComputerID, &e.ComputerName); err != nil {
			return n, err
		}
		e.Attrs = attrs.V
		if err := enc.Encode(e); err != nil {
			return n, err
		}
		n++
	}
	if err := bw.Flush(); err != nil {
		return n, err
	}
	return n, rows.Err()
}

// ArchiveLogDay writes one UTC day of logs to <dir>/<YYYY-MM-DD>-<source>.ndjson.gz.
// Existing archives are left untouched. Returns the path and entry count.
func (s *Store) ArchiveLogDay(ctx context.Context, source string, day time.Time, dir string) (string, int, error) {
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", 0, err
	}
	path := filepath.Join(dir, day.Format("2006-01-02")+"-"+source+".ndjson.gz")
	if _, err := os.Stat(path); err == nil {
		return path, 0, nil
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return "", 0, err
	}
	gz := gzip.NewWriter(f)
	end := day.Add(24 * time.Hour)
	n, err := s.ExportLogs(ctx, LogFilter{Source: source, From: &day, To: &end}, gz)
	if cerr := gz.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	return path, n, os.Rename(tmp, path)
}

// PruneLogs deletes log entries older than the retention period.
func (s *Store) PruneLogs(ctx context.Context, days int) {
	if days < 1 {
		days = 1
	}
	s.DB.ExecContext(ctx, `DELETE FROM server_logs WHERE at < now() - make_interval(days => $1)`, days)
	s.DB.ExecContext(ctx, `DELETE FROM agent_logs WHERE at < now() - make_interval(days => $1)`, days)
}

// ---- diagnostic bundles ----

const bundleCols = `b.id, b.computer_id, c.name, b.status, b.filename, b.size_bytes, b.sha256, b.error, b.requested_by, b.created_at, b.uploaded_at`

func scanBundle(row interface{ Scan(...any) error }) (*model.DiagnosticBundle, error) {
	var b model.DiagnosticBundle
	var up sql.NullTime
	if err := row.Scan(&b.ID, &b.ComputerID, &b.ComputerName, &b.Status, &b.Filename, &b.SizeBytes, &b.SHA256, &b.Error, &b.RequestedBy, &b.CreatedAt, &up); err != nil {
		return nil, wrapNotFound(err)
	}
	b.UploadedAt = db.TimePtr(up)
	return &b, nil
}

// CreateDiagnosticBundle records a requested bundle.
func (s *Store) CreateDiagnosticBundle(ctx context.Context, computerID, requestedBy string) (*model.DiagnosticBundle, error) {
	var id string
	if err := s.DB.QueryRowContext(ctx, `INSERT INTO diagnostic_bundles (computer_id, requested_by) VALUES ($1,$2) RETURNING id`, computerID, requestedBy).Scan(&id); err != nil {
		return nil, err
	}
	return s.GetDiagnosticBundle(ctx, id)
}

// GetDiagnosticBundle returns a bundle record.
func (s *Store) GetDiagnosticBundle(ctx context.Context, id string) (*model.DiagnosticBundle, error) {
	return scanBundle(s.DB.QueryRowContext(ctx, `SELECT `+bundleCols+` FROM diagnostic_bundles b JOIN computers c ON c.id = b.computer_id WHERE b.id::text = $1`, id))
}

// ListDiagnosticBundles lists bundles for a computer.
func (s *Store) ListDiagnosticBundles(ctx context.Context, computerID string) ([]model.DiagnosticBundle, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+bundleCols+` FROM diagnostic_bundles b JOIN computers c ON c.id = b.computer_id WHERE b.computer_id = $1 ORDER BY b.created_at DESC LIMIT 50`, computerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DiagnosticBundle{}
	for rows.Next() {
		b, err := scanBundle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// CompleteDiagnosticBundle marks a bundle uploaded or failed.
func (s *Store) CompleteDiagnosticBundle(ctx context.Context, id, computerID, status, filename, sha string, size int64, errMsg string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE diagnostic_bundles SET status=$3, filename=$4, sha256=$5, size_bytes=$6, error=$7, uploaded_at=now()
		WHERE id::text=$1 AND computer_id=$2 AND status='requested'`, id, computerID, status, filename, sha, size, errMsg)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
