package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/laps"
	"github.com/vibemdm/vibemdm/internal/model"
)

// ErrLAPSNoPending means a commit referred to a password that is not staged.
var ErrLAPSNoPending = errors.New("no matching staged password")

// GetLAPSConfig returns the LAPS policy (defaults, disabled, when never saved).
func (s *Store) GetLAPSConfig(ctx context.Context) (model.LAPSConfig, error) {
	var raw []byte
	var c model.LAPSConfig
	err := s.DB.QueryRowContext(ctx, `SELECT config FROM laps_config WHERE id = 1`).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return c, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &c); err != nil {
			return c, err
		}
	}
	if err := laps.Normalize(&c); err != nil {
		return model.LAPSConfig{}, err
	}
	return c, nil
}

// SaveLAPSConfig validates and stores the LAPS policy.
func (s *Store) SaveLAPSConfig(ctx context.Context, c model.LAPSConfig) (model.LAPSConfig, error) {
	if err := laps.Normalize(&c); err != nil {
		return c, err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO laps_config (id, config, updated_at) VALUES (1, $1, now())
		ON CONFLICT (id) DO UPDATE SET config = EXCLUDED.config, updated_at = now()`, b)
	return c, err
}

// LAPSStage stores a newly generated password as pending (sealed), replacing any earlier pending one.
func (s *Store) LAPSStage(ctx context.Context, computerID, account, password string) (fingerprint string, err error) {
	sealed, err := s.Box.Seal(password)
	if err != nil {
		return "", err
	}
	fp := laps.Fingerprint(password)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO laps_passwords (computer_id, pending_encrypted, pending_fingerprint, pending_account, pending_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (computer_id) DO UPDATE SET pending_encrypted = EXCLUDED.pending_encrypted,
		pending_fingerprint = EXCLUDED.pending_fingerprint, pending_account = EXCLUDED.pending_account, pending_at = now()`,
		computerID, sealed, fp, account)
	return fp, err
}

// LAPSCommit resolves the staged password. When applied it becomes the current one;
// otherwise it is discarded and the error recorded. The fingerprint must match the
// staged password, so a late or duplicated report cannot resolve a newer stage.
func (s *Store) LAPSCommit(ctx context.Context, computerID, fingerprint string, applied bool, errMsg string) error {
	if applied {
		res, err := s.DB.ExecContext(ctx, `UPDATE laps_passwords SET
			password_encrypted = pending_encrypted, fingerprint = pending_fingerprint, account = pending_account,
			escrowed_at = now(), rotated_count = rotated_count + 1,
			pending_encrypted = NULL, pending_fingerprint = '', pending_account = '', pending_at = NULL,
			rotate_after = NULL, rotate_requested = FALSE, last_error = '', last_error_at = NULL
			WHERE computer_id = $1 AND pending_encrypted IS NOT NULL AND pending_fingerprint = $2`, computerID, fingerprint)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrLAPSNoPending
		}
		return nil
	}
	if len(errMsg) > 500 {
		errMsg = errMsg[:500]
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE laps_passwords SET
		pending_encrypted = NULL, pending_fingerprint = '', pending_account = '', pending_at = NULL,
		last_error = $3, last_error_at = now()
		WHERE computer_id = $1 AND pending_encrypted IS NOT NULL AND pending_fingerprint = $2`, computerID, fingerprint, errMsg)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrLAPSNoPending
	}
	return nil
}

// LAPSReportError records an agent-side failure that happened before anything was staged.
func (s *Store) LAPSReportError(ctx context.Context, computerID, msg string) error {
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO laps_passwords (computer_id, last_error, last_error_at) VALUES ($1, $2, now())
		ON CONFLICT (computer_id) DO UPDATE SET last_error = EXCLUDED.last_error, last_error_at = now()`, computerID, msg)
	return err
}

// lapsRow is the raw per-computer state used to derive status and rotation.
type lapsRow struct {
	account, fingerprint, lastViewedBy, lastError string
	hasPassword, rotateRequested, hasPending      bool
	escrowedAt, pendingAt, lastViewedAt           sql.NullTime
	rotateAfter, lastErrorAt                      sql.NullTime
	rotated                                       int
	sealed                                        sql.NullString
}

func (s *Store) lapsRow(ctx context.Context, computerID string) (*lapsRow, error) {
	r := &lapsRow{}
	var pending sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT account, password_encrypted, fingerprint, escrowed_at, rotated_count, pending_encrypted, pending_at,
		last_viewed_at, last_viewed_by, rotate_after, rotate_requested, last_error, last_error_at
		FROM laps_passwords WHERE computer_id = $1`, computerID).
		Scan(&r.account, &r.sealed, &r.fingerprint, &r.escrowedAt, &r.rotated, &pending, &r.pendingAt,
			&r.lastViewedAt, &r.lastViewedBy, &r.rotateAfter, &r.rotateRequested, &r.lastError, &r.lastErrorAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil // never seen: zero row
	}
	r.hasPassword, r.hasPending = r.sealed.Valid && r.sealed.String != "", pending.Valid
	return r, err
}

// lapsRotation decides whether a rotation is due and why.
func lapsRotation(r *lapsRow, cfg model.LAPSConfig, now time.Time) (bool, string) {
	switch {
	case !r.hasPassword:
		return true, "missing"
	case r.rotateRequested:
		return true, "requested"
	case r.rotateAfter.Valid && !r.rotateAfter.Time.After(now):
		return true, "used"
	case r.escrowedAt.Valid && !r.escrowedAt.Time.Add(time.Duration(cfg.RotationDays)*24*time.Hour).After(now):
		return true, "expired"
	case r.account != "" && r.account != cfg.AccountName:
		return true, "account_changed"
	}
	return false, ""
}

func lapsStatus(r *lapsRow, cfg model.LAPSConfig, inScope bool, now time.Time) model.LAPSStatus {
	st := model.LAPSStatus{InScope: inScope, HasPassword: r.hasPassword, Account: r.account, Fingerprint: r.fingerprint,
		EscrowedAt: db.TimePtr(r.escrowedAt), RotatedCount: r.rotated, Pending: r.hasPending, PendingAt: db.TimePtr(r.pendingAt),
		LastViewedAt: db.TimePtr(r.lastViewedAt), LastViewedBy: r.lastViewedBy, RotateAfter: db.TimePtr(r.rotateAfter),
		RotateRequest: r.rotateRequested, LastError: r.lastError, LastErrorAt: db.TimePtr(r.lastErrorAt)}
	if r.escrowedAt.Valid {
		t := r.escrowedAt.Time.Add(time.Duration(cfg.RotationDays) * 24 * time.Hour)
		st.ExpiresAt = &t
	}
	due, _ := lapsRotation(r, cfg, now)
	switch {
	case !inScope:
		st.RotationStatus = "out_of_scope"
	case r.hasPending:
		st.RotationStatus = "pending"
	case r.lastError != "" && (!r.escrowedAt.Valid || (r.lastErrorAt.Valid && r.lastErrorAt.Time.After(r.escrowedAt.Time))):
		st.RotationStatus = "failing"
	case !r.hasPassword:
		st.RotationStatus = "missing"
	case due:
		st.RotationStatus = "due"
	default:
		st.RotationStatus = "ok"
	}
	return st
}

// LAPSConfigForAgent returns what an agent should do now. The current password is
// included only when a rotation is due, for sysadminctl's old-password argument.
func (s *Store) LAPSConfigForAgent(ctx context.Context, c *model.Computer) (model.AgentLAPSConfig, error) {
	cfg, err := s.GetLAPSConfig(ctx)
	if err != nil {
		return model.AgentLAPSConfig{}, err
	}
	out := model.AgentLAPSConfig{Account: cfg.AccountName, CreateAccount: cfg.CreateAccount, PasswordLength: cfg.PasswordLength}
	if !cfg.Enabled {
		return out, nil
	}
	in, err := s.InScopeComputer(ctx, c, cfg.Scope, nil)
	if err != nil || !in {
		return out, err
	}
	out.Enabled = true
	r, err := s.lapsRow(ctx, c.ID)
	if err != nil {
		return out, err
	}
	if due, why := lapsRotation(r, cfg, time.Now()); due {
		out.Rotate, out.Reason = true, why
		if r.hasPassword {
			if pw, err := s.Box.Open(r.sealed.String); err == nil {
				out.CurrentPassword = pw
			}
		}
	}
	return out, nil
}

// LAPSStatusFor returns one computer's status (never a password).
func (s *Store) LAPSStatusFor(ctx context.Context, c *model.Computer) (*model.LAPSStatus, error) {
	cfg, err := s.GetLAPSConfig(ctx)
	if err != nil {
		return nil, err
	}
	in := false
	if cfg.Enabled {
		if in, err = s.InScopeComputer(ctx, c, cfg.Scope, nil); err != nil {
			return nil, err
		}
	}
	r, err := s.lapsRow(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	st := lapsStatus(r, cfg, in, time.Now())
	st.AgentEnrolled, st.ComputerID, st.ComputerName = c.AgentEnrolled, c.ID, c.Name
	return &st, nil
}

// LAPSPasswords are the decrypted values returned to an authorised administrator.
type LAPSPasswords struct {
	Current string
	// Pending is a staged password whose application was never confirmed; it may be the live one.
	Pending string
}

// RevealLAPS returns the decrypted password(s) and records the view. When the
// policy rotates after use, the rotation is scheduled for the grace period ahead.
func (s *Store) RevealLAPS(ctx context.Context, computerID, by string) (*LAPSPasswords, *model.LAPSStatus, error) {
	cfg, err := s.GetLAPSConfig(ctx)
	if err != nil {
		return nil, nil, err
	}
	var curSealed, penSealed sql.NullString
	err = s.DB.QueryRowContext(ctx, `SELECT password_encrypted, pending_encrypted FROM laps_passwords WHERE computer_id = $1`, computerID).Scan(&curSealed, &penSealed)
	if err != nil {
		return nil, nil, wrapNotFound(err)
	}
	if !curSealed.Valid && !penSealed.Valid {
		return nil, nil, ErrNotFound
	}
	out := &LAPSPasswords{}
	if curSealed.Valid {
		if out.Current, err = s.Box.Open(curSealed.String); err != nil {
			return nil, nil, err
		}
	}
	if penSealed.Valid {
		if out.Pending, err = s.Box.Open(penSealed.String); err != nil {
			return nil, nil, err
		}
	}
	var after any
	if cfg.RotateAfterUse {
		after = time.Now().Add(time.Duration(cfg.RotateAfterUseMinutes) * time.Minute)
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE laps_passwords SET last_viewed_at = now(), last_viewed_by = $2,
		rotate_after = COALESCE($3::timestamptz, rotate_after) WHERE computer_id = $1`, computerID, by, after); err != nil {
		return nil, nil, err
	}
	c, err := s.GetComputer(ctx, computerID)
	if err != nil {
		return nil, nil, err
	}
	st, err := s.LAPSStatusFor(ctx, c)
	return out, st, err
}

// LAPSRequestRotation marks a computer for rotation at its next agent cycle.
func (s *Store) LAPSRequestRotation(ctx context.Context, computerID string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO laps_passwords (computer_id, rotate_requested) VALUES ($1, TRUE)
		ON CONFLICT (computer_id) DO UPDATE SET rotate_requested = TRUE`, computerID)
	return err
}

// LAPSOverview lists every in-scope Mac with the agent, plus a summary.
func (s *Store) LAPSOverview(ctx context.Context) (model.LAPSConfig, []model.LAPSStatus, model.LAPSSummary, error) {
	var sum model.LAPSSummary
	cfg, err := s.GetLAPSConfig(ctx)
	if err != nil {
		return cfg, nil, sum, err
	}
	rows := []model.LAPSStatus{}
	if !cfg.Enabled {
		return cfg, rows, sum, nil
	}
	ids, err := s.ResolveScope(ctx, cfg.Scope)
	if err != nil {
		return cfg, nil, sum, err
	}
	if len(ids) == 0 {
		return cfg, rows, sum, nil
	}
	q := `SELECT c.id, c.name, COALESCE(c.serial_number,''), c.agent_enrolled, c.agent_last_checkin,
		COALESCE(l.account,''), l.password_encrypted IS NOT NULL, COALESCE(l.fingerprint,''), l.escrowed_at, COALESCE(l.rotated_count,0),
		l.pending_encrypted IS NOT NULL, l.pending_at, l.last_viewed_at, COALESCE(l.last_viewed_by,''), l.rotate_after,
		COALESCE(l.rotate_requested,FALSE), COALESCE(l.last_error,''), l.last_error_at
		FROM computers c LEFT JOIN laps_passwords l ON l.computer_id = c.id
		WHERE c.agent_enrolled AND c.id = ANY($1::uuid[]) ORDER BY lower(c.name)`
	rs, err := s.DB.QueryContext(ctx, q, "{"+strings.Join(ids, ",")+"}")
	if err != nil {
		return cfg, nil, sum, err
	}
	defer rs.Close()
	now := time.Now()
	for rs.Next() {
		var id, name, serial string
		var enrolled bool
		var seen sql.NullTime
		r := &lapsRow{}
		var hasPw, hasPend bool
		if err := rs.Scan(&id, &name, &serial, &enrolled, &seen, &r.account, &hasPw, &r.fingerprint, &r.escrowedAt, &r.rotated,
			&hasPend, &r.pendingAt, &r.lastViewedAt, &r.lastViewedBy, &r.rotateAfter, &r.rotateRequested, &r.lastError, &r.lastErrorAt); err != nil {
			return cfg, nil, sum, err
		}
		r.hasPassword, r.hasPending = hasPw, hasPend
		st := lapsStatus(r, cfg, true, now)
		st.ComputerID, st.ComputerName, st.SerialNumber, st.AgentEnrolled, st.AgentLastSeen = id, name, serial, enrolled, db.TimePtr(seen)
		sum.InScope++
		switch st.RotationStatus {
		case "ok":
			sum.OK++
		case "due":
			sum.Due++
		case "pending":
			sum.Pending++
		case "missing":
			sum.Missing++
		case "failing":
			sum.Failing++
		}
		rows = append(rows, st)
	}
	return cfg, rows, sum, rs.Err()
}
