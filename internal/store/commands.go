package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

// InsertCommandMeta records metadata for an enqueued MDM command.
// Exactly one of computerID and mobileDeviceID is set.
func (s *Store) InsertCommandMeta(ctx context.Context, commandUUID, computerID, mobileDeviceID, enrollmentID, requestType, kind, refID, createdBy string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO mdm_commands_meta (command_uuid, computer_id, mobile_device_id, enrollment_id, request_type, kind, ref_id, created_by)
		VALUES ($1, NULLIF($2,'')::uuid, NULLIF($3,'')::uuid, $4, $5, $6, NULLIF($7,'')::uuid, $8) ON CONFLICT (command_uuid) DO NOTHING`,
		commandUUID, computerID, mobileDeviceID, enrollmentID, requestType, kind, refID, createdBy)
	return err
}

// CommandMeta is our metadata for a command.
type CommandMeta struct {
	CommandUUID  string
	ComputerID   string
	EnrollmentID string
	RequestType  string
	Kind         string
	RefID        string
}

// GetCommandMeta returns metadata for a command.
func (s *Store) GetCommandMeta(ctx context.Context, commandUUID string) (*CommandMeta, error) {
	var m CommandMeta
	var cid, ref sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT command_uuid, computer_id::text, enrollment_id, request_type, kind, ref_id::text FROM mdm_commands_meta WHERE command_uuid = $1`, commandUUID).
		Scan(&m.CommandUUID, &cid, &m.EnrollmentID, &m.RequestType, &m.Kind, &ref)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	m.ComputerID, m.RefID = cid.String, ref.String
	return &m, nil
}

// CommandFilter filters the command log.
type CommandFilter struct {
	EnrollmentID   string
	ComputerID     string
	MobileDeviceID string
	Status         string // Pending | Acknowledged | Error | NotNow
	Limit          int
	Offset         int
}

const commandSelect = `SELECT q.command_uuid, q.id, COALESCE(m.computer_id::text, comp.id::text, ''), COALESCE(m.mobile_device_id::text, md.id::text, ''),
	COALESCE(comp.name, md.name, ''), c.request_type, COALESCE(m.kind,''),
	COALESCE(r.status, 'Pending'), q.active, q.created_at, r.updated_at, COALESCE(m.created_by,'')
	FROM enrollment_queue q
	JOIN commands c ON c.command_uuid = q.command_uuid
	LEFT JOIN command_results r ON r.command_uuid = q.command_uuid AND r.id = q.id
	LEFT JOIN mdm_commands_meta m ON m.command_uuid = q.command_uuid
	LEFT JOIN computers comp ON comp.udid = split_part(q.id, ':', 1)
	LEFT JOIN mobile_devices md ON md.udid = split_part(q.id, ':', 1)`

func scanCommand(row interface{ Scan(...any) error }) (*model.MDMCommand, error) {
	var c model.MDMCommand
	var upd sql.NullTime
	if err := row.Scan(&c.CommandUUID, &c.EnrollmentID, &c.ComputerID, &c.MobileDeviceID, &c.ComputerName, &c.RequestType, &c.Kind, &c.Status, &c.Active, &c.CreatedAt, &upd, &c.CreatedBy); err != nil {
		return nil, wrapNotFound(err)
	}
	c.ResultUpdatedAt = db.TimePtr(upd)
	if !c.Active && c.Status == "Pending" {
		c.Status = "Cancelled"
	}
	return &c, nil
}

// ListCommands returns the MDM command log.
func (s *Store) ListCommands(ctx context.Context, f CommandFilter) ([]model.MDMCommand, int, error) {
	f.Limit, f.Offset = clampPage(f.Limit, f.Offset)
	var where []string
	var args []any
	if f.EnrollmentID != "" {
		args = append(args, f.EnrollmentID)
		where = append(where, fmt.Sprintf("split_part(q.id, ':', 1) = $%d", len(args)))
	}
	if f.ComputerID != "" {
		args = append(args, f.ComputerID)
		where = append(where, fmt.Sprintf("(m.computer_id::text = $%d OR comp.id::text = $%d)", len(args), len(args)))
	}
	if f.MobileDeviceID != "" {
		args = append(args, f.MobileDeviceID)
		where = append(where, fmt.Sprintf("(m.mobile_device_id::text = $%d OR md.id::text = $%d)", len(args), len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		if f.Status == "Pending" {
			where = append(where, "r.status IS NULL AND q.active")
			args = args[:len(args)-1]
		} else {
			where = append(where, fmt.Sprintf("r.status = $%d", len(args)))
		}
	}
	w := ""
	if len(where) > 0 {
		w = " WHERE " + strings.Join(where, " AND ")
	}
	countSQL := `SELECT count(*) FROM enrollment_queue q JOIN commands c ON c.command_uuid = q.command_uuid
		LEFT JOIN command_results r ON r.command_uuid = q.command_uuid AND r.id = q.id
		LEFT JOIN mdm_commands_meta m ON m.command_uuid = q.command_uuid
		LEFT JOIN computers comp ON comp.udid = split_part(q.id, ':', 1)
		LEFT JOIN mobile_devices md ON md.udid = split_part(q.id, ':', 1)` + w
	var total int
	if err := s.DB.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.DB.QueryContext(ctx, commandSelect+w+fmt.Sprintf(` ORDER BY q.created_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.MDMCommand{}
	for rows.Next() {
		c, err := scanCommand(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *c)
	}
	return out, total, rows.Err()
}

// GetCommand returns a command with raw plist and result.
func (s *Store) GetCommand(ctx context.Context, commandUUID string) (*model.MDMCommand, error) {
	c, err := scanCommand(s.DB.QueryRowContext(ctx, commandSelect+` WHERE q.command_uuid = $1 LIMIT 1`, commandUUID))
	if err != nil {
		return nil, err
	}
	var result sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT c.command, r.result FROM commands c LEFT JOIN command_results r ON r.command_uuid = c.command_uuid WHERE c.command_uuid = $1 LIMIT 1`, commandUUID).Scan(&c.Command, &result); err != nil {
		return nil, wrapNotFound(err)
	}
	c.Result = result.String
	return c, nil
}

// CancelCommand deactivates a queued command.
func (s *Store) CancelCommand(ctx context.Context, commandUUID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE enrollment_queue SET active = FALSE WHERE command_uuid = $1`, commandUUID)
	return err
}

// PendingCommandCount counts outstanding active commands.
func (s *Store) PendingCommandCount(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM enrollment_queue q LEFT JOIN command_results r ON r.command_uuid = q.command_uuid AND r.id = q.id
		WHERE q.active AND (r.status IS NULL OR r.status = 'NotNow')`).Scan(&n)
	return n, err
}

// EnrollmentEnabled reports whether a NanoMDM enrollment is enabled (has push info).
func (s *Store) EnrollmentEnabled(ctx context.Context, enrollmentID string) (bool, error) {
	var enabled bool
	err := s.DB.QueryRowContext(ctx, `SELECT enabled FROM enrollments WHERE id = $1`, enrollmentID).Scan(&enabled)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return enabled, err
}
