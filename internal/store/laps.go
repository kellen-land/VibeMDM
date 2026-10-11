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

// lapsEvent appends to a Mac's LAPS event history.
func lapsEvent(ctx context.Context, x interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, computerID, account, typ, actor, detail string) error {
	if len(detail) > 500 {
		detail = detail[:500]
	}
	_, err := x.ExecContext(ctx, `INSERT INTO laps_events (computer_id, account, event_type, actor, detail) VALUES ($1, $2, $3, $4, $5)`,
		computerID, account, typ, actor, detail)
	return err
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

// LAPSCommit resolves the staged password. When applied it becomes the current one, the
// previous one is retired into the history, and an administrator-chosen password (if any)
// is cleared; otherwise the stage is discarded and the error recorded. The fingerprint must
// match the staged password, so a late or duplicated report cannot resolve a newer stage.
func (s *Store) LAPSCommit(ctx context.Context, computerID string, c model.AgentLAPSCommit) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sealed, account string
	err = tx.QueryRowContext(ctx, `SELECT pending_encrypted, pending_account FROM laps_passwords
		WHERE computer_id = $1 AND pending_encrypted IS NOT NULL AND pending_fingerprint = $2 FOR UPDATE`, computerID, c.Fingerprint).Scan(&sealed, &account)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrLAPSNoPending
	}
	if err != nil {
		return err
	}
	if c.Applied {
		if _, err := tx.ExecContext(ctx, `UPDATE laps_password_history SET retired_at = now() WHERE computer_id = $1 AND retired_at IS NULL`, computerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO laps_password_history (computer_id, account, guid, password_encrypted, fingerprint)
			VALUES ($1, $2, $3, $4, $5)`, computerID, account, c.Guid, sealed, c.Fingerprint); err != nil {
			return err
		}
		// Keep the last 50 passwords per Mac.
		if _, err := tx.ExecContext(ctx, `DELETE FROM laps_password_history WHERE computer_id = $1 AND id NOT IN
			(SELECT id FROM laps_password_history WHERE computer_id = $1 ORDER BY created_at DESC LIMIT 50)`, computerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE laps_passwords SET
			password_encrypted = pending_encrypted, fingerprint = pending_fingerprint, account = pending_account,
			guid = CASE WHEN $2 <> '' THEN $2 ELSE guid END, escrowed_at = now(), rotated_count = rotated_count + 1,
			pending_encrypted = NULL, pending_fingerprint = '', pending_account = '', pending_at = NULL,
			desired_encrypted = NULL, desired_by = '', desired_at = NULL,
			rotate_after = NULL, rotate_requested = FALSE, last_error = '', last_error_at = NULL
			WHERE computer_id = $1`, computerID, c.Guid); err != nil {
			return err
		}
		if c.Created {
			if err := lapsEvent(ctx, tx, computerID, account, "CREATED", "agent", "account created"); err != nil {
				return err
			}
		}
		if err := lapsEvent(ctx, tx, computerID, account, "ROTATED", "agent", "fingerprint "+c.Fingerprint); err != nil {
			return err
		}
		return tx.Commit()
	}
	msg := c.Error
	if len(msg) > 500 {
		msg = msg[:500]
	}
	if _, err := tx.ExecContext(ctx, `UPDATE laps_passwords SET
		pending_encrypted = NULL, pending_fingerprint = '', pending_account = '', pending_at = NULL,
		desired_encrypted = NULL, desired_by = '', desired_at = NULL,
		last_error = $2, last_error_at = now() WHERE computer_id = $1`, computerID, msg); err != nil {
		return err
	}
	if err := lapsEvent(ctx, tx, computerID, account, "ERROR", "agent", msg); err != nil {
		return err
	}
	return tx.Commit()
}

// LAPSReportError records an agent-side failure that happened before anything was staged.
func (s *Store) LAPSReportError(ctx context.Context, computerID, msg string) error {
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO laps_passwords (computer_id, last_error, last_error_at) VALUES ($1, $2, now())
		ON CONFLICT (computer_id) DO UPDATE SET last_error = EXCLUDED.last_error, last_error_at = now()`, computerID, msg)
	if err != nil {
		return err
	}
	return lapsEvent(ctx, s.DB, computerID, "", "ERROR", "agent", msg)
}

// lapsRow is the raw per-computer state used to derive status and rotation.
type lapsRow struct {
	account, fingerprint, lastViewedBy, lastError, guid, desiredBy string
	hasPassword, rotateRequested, hasPending, hasDesired           bool
	escrowedAt, pendingAt, lastViewedAt, desiredAt                 sql.NullTime
	rotateAfter, lastErrorAt                                       sql.NullTime
	rotated                                                        int
	sealed, desired                                                sql.NullString
	secureToken, volumeOwner                                       sql.NullBool
	tokenCheckedAt                                                 sql.NullTime
	grantPending                                                   bool
}

const lapsRowCols = `COALESCE(l.account,''), l.password_encrypted, COALESCE(l.fingerprint,''), l.escrowed_at, COALESCE(l.rotated_count,0),
	l.pending_encrypted IS NOT NULL, l.pending_at, l.last_viewed_at, COALESCE(l.last_viewed_by,''), l.rotate_after,
	COALESCE(l.rotate_requested,FALSE), COALESCE(l.last_error,''), l.last_error_at,
	COALESCE(l.guid,''), l.desired_encrypted, COALESCE(l.desired_by,''), l.desired_at,
	l.secure_token, l.volume_owner, l.token_checked_at,
	EXISTS (SELECT 1 FROM laps_token_grants tg WHERE tg.computer_id = l.computer_id AND tg.expires_at > now())`

func (r *lapsRow) scanDest() []any {
	return []any{&r.account, &r.sealed, &r.fingerprint, &r.escrowedAt, &r.rotated, &r.hasPending, &r.pendingAt,
		&r.lastViewedAt, &r.lastViewedBy, &r.rotateAfter, &r.rotateRequested, &r.lastError, &r.lastErrorAt,
		&r.guid, &r.desired, &r.desiredBy, &r.desiredAt, &r.secureToken, &r.volumeOwner, &r.tokenCheckedAt, &r.grantPending}
}

func (r *lapsRow) finish() {
	r.hasPassword = r.sealed.Valid && r.sealed.String != ""
	r.hasDesired = r.desired.Valid && r.desired.String != ""
}

func (s *Store) lapsRow(ctx context.Context, computerID string) (*lapsRow, error) {
	r := &lapsRow{}
	err := s.DB.QueryRowContext(ctx, `SELECT `+lapsRowCols+` FROM laps_passwords l WHERE l.computer_id = $1`, computerID).Scan(r.scanDest()...)
	if errors.Is(err, sql.ErrNoRows) {
		return &lapsRow{}, nil // never seen: zero row
	}
	r.finish()
	return r, err
}

// lapsRotation decides whether a rotation is due and why.
func lapsRotation(r *lapsRow, cfg model.LAPSConfig, now time.Time) (bool, string) {
	switch {
	case r.hasDesired:
		return true, "set"
	case !r.hasPassword:
		return true, "missing"
	case r.rotateRequested:
		return true, "requested"
	case r.rotateAfter.Valid && !r.rotateAfter.Time.After(now):
		return true, "used"
	case cfg.AutoRotate && r.escrowedAt.Valid && !r.escrowedAt.Time.Add(time.Duration(cfg.RotationDays)*24*time.Hour).After(now):
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
		RotateRequest: r.rotateRequested, LastError: r.lastError, LastErrorAt: db.TimePtr(r.lastErrorAt), Guid: r.guid}
	if r.hasDesired {
		st.PasswordSetBy = r.desiredBy
	}
	st.SecureToken, st.VolumeOwner, st.TokenCheckedAt, st.TokenGrantPending = db.BoolPtr(r.secureToken), db.BoolPtr(r.volumeOwner), db.TimePtr(r.tokenCheckedAt), r.grantPending
	if cfg.AutoRotate && r.escrowedAt.Valid {
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
	case !r.hasPassword && !r.hasDesired:
		st.RotationStatus = "missing"
	case due:
		st.RotationStatus = "due"
	default:
		st.RotationStatus = "ok"
	}
	return st
}

// LAPSConfigForAgent returns what an agent should do now. The current password is
// included only when a rotation is due, for sysadminctl's old-password argument, and an
// administrator-chosen password only while it waits to be applied.
func (s *Store) LAPSConfigForAgent(ctx context.Context, c *model.Computer) (model.AgentLAPSConfig, error) {
	cfg, err := s.GetLAPSConfig(ctx)
	if err != nil {
		return model.AgentLAPSConfig{}, err
	}
	out := model.AgentLAPSConfig{Account: cfg.AccountName, CreateAccount: cfg.CreateAccount, HideAccount: cfg.HideAccount,
		PasswordFormat: cfg.PasswordFormat, PasswordLength: cfg.PasswordLength}
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
	if r.grantPending && r.hasPassword {
		out.GrantSecureToken = true
		if pw, err := s.Box.Open(r.sealed.String); err == nil {
			out.CurrentPassword = pw
		}
	}
	if due, why := lapsRotation(r, cfg, time.Now()); due {
		out.Rotate, out.Reason = true, why
		if r.hasPassword {
			if pw, err := s.Box.Open(r.sealed.String); err == nil {
				out.CurrentPassword = pw
			}
		}
		if r.hasDesired {
			pw, err := s.Box.Open(r.desired.String)
			if err != nil {
				return out, err
			}
			out.NewPassword = pw
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

// RevealLAPS returns the decrypted password(s), records who viewed it and why, and, as in
// Jamf Pro, schedules a rotation for RotateAfterUseMinutes later (an earlier schedule is kept).
func (s *Store) RevealLAPS(ctx context.Context, computerID, by, reason string) (*LAPSPasswords, *model.LAPSStatus, error) {
	cfg, err := s.GetLAPSConfig(ctx)
	if err != nil {
		return nil, nil, err
	}
	var curSealed, penSealed sql.NullString
	var fp, account string
	err = s.DB.QueryRowContext(ctx, `SELECT password_encrypted, pending_encrypted, fingerprint, account FROM laps_passwords WHERE computer_id = $1`, computerID).
		Scan(&curSealed, &penSealed, &fp, &account)
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
	after := time.Now().Add(time.Duration(cfg.RotateAfterUseMinutes) * time.Minute)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE laps_passwords SET last_viewed_at = now(), last_viewed_by = $2,
		rotate_after = LEAST(COALESCE(rotate_after, $3::timestamptz), $3::timestamptz) WHERE computer_id = $1`, computerID, by, after); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO laps_views (computer_id, fingerprint, viewed_by, reason) VALUES ($1, $2, $3, $4)`,
		computerID, fp, by, reason); err != nil {
		return nil, nil, err
	}
	if err := lapsEvent(ctx, tx, computerID, account, "VIEWED", by, reason); err != nil {
		return nil, nil, err
	}
	if err := lapsEvent(ctx, tx, computerID, account, "PENDING", by, "rotation after viewing at "+after.UTC().Format(time.RFC3339)); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
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
func (s *Store) LAPSRequestRotation(ctx context.Context, computerID, by string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO laps_passwords (computer_id, rotate_requested) VALUES ($1, TRUE)
		ON CONFLICT (computer_id) DO UPDATE SET rotate_requested = TRUE`, computerID)
	if err != nil {
		return err
	}
	return lapsEvent(ctx, s.DB, computerID, "", "PENDING", by, "rotation requested")
}

// LAPSSetPassword stores an administrator-chosen password; the agent applies it at its next
// cycle through the usual stage → apply → verify → commit steps (Jamf: set-password).
func (s *Store) LAPSSetPassword(ctx context.Context, computerID, account, password, by string) error {
	if err := laps.ValidateManual(password); err != nil {
		return err
	}
	sealed, err := s.Box.Seal(password)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO laps_passwords (computer_id, desired_encrypted, desired_by, desired_at) VALUES ($1, $2, $3, now())
		ON CONFLICT (computer_id) DO UPDATE SET desired_encrypted = EXCLUDED.desired_encrypted, desired_by = EXCLUDED.desired_by, desired_at = now()`,
		computerID, sealed, by); err != nil {
		return err
	}
	return lapsEvent(ctx, s.DB, computerID, account, "SET", by, "password set by an administrator; waiting for the Mac")
}

// LAPSHistory lists every password a Mac has had, newest first, with who viewed each.
// Passwords are decrypted only when withPasswords is set (admin audit view).
func (s *Store) LAPSHistory(ctx context.Context, computerID string, withPasswords bool) ([]model.LAPSHistoryEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT account, guid, password_encrypted, fingerprint, created_at, retired_at
		FROM laps_password_history WHERE computer_id = $1 ORDER BY created_at DESC`, computerID)
	if err != nil {
		return nil, err
	}
	out := []model.LAPSHistoryEntry{}
	for rows.Next() {
		var e model.LAPSHistoryEntry
		var sealed string
		var retired sql.NullTime
		if err := rows.Scan(&e.Account, &e.Guid, &sealed, &e.Fingerprint, &e.CreatedAt, &retired); err != nil {
			rows.Close()
			return nil, err
		}
		e.RetiredAt = db.TimePtr(retired)
		if withPasswords {
			if e.Password, err = s.Box.Open(sealed); err != nil {
				rows.Close()
				return nil, err
			}
		}
		out = append(out, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	views, err := s.DB.QueryContext(ctx, `SELECT fingerprint, viewed_by, reason, viewed_at FROM laps_views WHERE computer_id = $1 ORDER BY viewed_at DESC`, computerID)
	if err != nil {
		return nil, err
	}
	defer views.Close()
	idx := map[string]int{}
	for i, e := range out {
		if _, ok := idx[e.Fingerprint]; !ok {
			idx[e.Fingerprint] = i
		}
		out[i].Views = []model.LAPSView{}
	}
	for views.Next() {
		var fp string
		var v model.LAPSView
		if err := views.Scan(&fp, &v.ViewedBy, &v.Reason, &v.ViewedAt); err != nil {
			return nil, err
		}
		if i, ok := idx[fp]; ok {
			out[i].Views = append(out[i].Views, v)
		}
	}
	return out, views.Err()
}

// LAPSEvents returns a Mac's LAPS event history, newest first.
func (s *Store) LAPSEvents(ctx context.Context, computerID string, limit int) ([]model.LAPSEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, account, event_type, actor, detail, at FROM laps_events
		WHERE computer_id = $1 ORDER BY at DESC, id DESC LIMIT $2`, computerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.LAPSEvent{}
	for rows.Next() {
		var e model.LAPSEvent
		if err := rows.Scan(&e.ID, &e.Account, &e.EventType, &e.Actor, &e.Detail, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LAPSPendingRotations lists Macs with a rotation queued but not yet confirmed (Jamf: pending-rotations).
func (s *Store) LAPSPendingRotations(ctx context.Context) ([]model.LAPSPendingRotation, error) {
	_, rows, _, err := s.LAPSOverview(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := s.GetLAPSConfig(ctx)
	if err != nil {
		return nil, err
	}
	out := []model.LAPSPendingRotation{}
	for _, st := range rows {
		p := model.LAPSPendingRotation{ComputerID: st.ComputerID, ComputerName: st.ComputerName, Account: cfg.AccountName, Guid: st.Guid}
		if st.Account != "" {
			p.Account = st.Account
		}
		switch {
		case st.Pending && st.PendingAt != nil:
			p.Reason, p.Since = "staged", *st.PendingAt
		case st.PasswordSetBy != "":
			p.Reason, p.Since = "set", time.Now()
		case st.RotateRequest:
			p.Reason, p.Since = "requested", time.Now()
		case st.RotateAfter != nil:
			p.Reason, p.DueAt = "viewed", st.RotateAfter
			if st.LastViewedAt != nil {
				p.Since = *st.LastViewedAt
			}
		case st.RotationStatus == "due" && st.ExpiresAt != nil:
			p.Reason, p.Since = "expired", *st.ExpiresAt
		case st.RotationStatus == "missing":
			p.Reason = "missing"
			if st.AgentLastSeen != nil {
				p.Since = *st.AgentLastSeen
			}
		default:
			continue
		}
		out = append(out, p)
	}
	return out, nil
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
	q := `SELECT c.id, c.name, COALESCE(c.serial_number,''), c.agent_enrolled, c.agent_last_checkin, ` + lapsRowCols + `
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
		if err := rs.Scan(append([]any{&id, &name, &serial, &enrolled, &seen}, r.scanDest()...)...); err != nil {
			return cfg, nil, sum, err
		}
		r.finish()
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

// LAPSSetTokenState records the agent's report of the account's Secure Token and volume
// ownership, plus the outcome of a grant attempt.
func (s *Store) LAPSSetTokenState(ctx context.Context, computerID string, st model.AgentLAPSState) error {
	nb := func(b *bool) sql.NullBool {
		if b == nil {
			return sql.NullBool{}
		}
		return sql.NullBool{Bool: *b, Valid: true}
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO laps_passwords (computer_id, secure_token, volume_owner, token_checked_at) VALUES ($1, $2, $3, now())
		ON CONFLICT (computer_id) DO UPDATE SET secure_token = EXCLUDED.secure_token, volume_owner = EXCLUDED.volume_owner, token_checked_at = now()`,
		computerID, nb(st.SecureToken), nb(st.VolumeOwner)); err != nil {
		return err
	}
	switch {
	case st.Granted:
		return lapsEvent(ctx, s.DB, computerID, "", "TOKEN_GRANTED", "agent", "Secure Token granted")
	case st.GrantError != "":
		return lapsEvent(ctx, s.DB, computerID, "", "ERROR", "agent", st.GrantError)
	}
	return nil
}

// LAPSCreateTokenGrant stores an existing Secure Token administrator's credentials, sealed, for
// the agent to fetch once within ttl.
func (s *Store) LAPSCreateTokenGrant(ctx context.Context, computerID, adminUser, adminPassword, by string, ttl time.Duration) error {
	sealed, err := s.Box.Seal(adminPassword)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO laps_token_grants (computer_id, admin_user, admin_password_encrypted, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (computer_id) DO UPDATE SET admin_user = EXCLUDED.admin_user,
		admin_password_encrypted = EXCLUDED.admin_password_encrypted, created_by = EXCLUDED.created_by, created_at = now(), expires_at = EXCLUDED.expires_at`,
		computerID, adminUser, sealed, by, time.Now().Add(ttl)); err != nil {
		return err
	}
	return lapsEvent(ctx, s.DB, computerID, "", "PENDING", by, "Secure Token grant requested (credentials of "+adminUser+")")
}

// LAPSTakeTokenGrant returns and deletes a pending grant (ErrNotFound if none or expired).
func (s *Store) LAPSTakeTokenGrant(ctx context.Context, computerID string) (*model.AgentLAPSTokenGrant, error) {
	var user, sealed string
	var fresh bool
	err := s.DB.QueryRowContext(ctx, `DELETE FROM laps_token_grants WHERE computer_id = $1 RETURNING admin_user, admin_password_encrypted, expires_at > now()`,
		computerID).Scan(&user, &sealed, &fresh)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	if !fresh {
		return nil, ErrNotFound
	}
	pw, err := s.Box.Open(sealed)
	if err != nil {
		return nil, err
	}
	return &model.AgentLAPSTokenGrant{AdminUser: user, AdminPassword: pw}, nil
}

// LAPSExpireTokenGrants deletes grants nobody fetched in time.
func (s *Store) LAPSExpireTokenGrants(ctx context.Context) {
	s.DB.ExecContext(ctx, `DELETE FROM laps_token_grants WHERE expires_at <= now()`)
}

// LAPSAdminCredentials returns a computer's LAPS account and its current password for use by
// the agent itself (Local Accounts actions that need a Secure Token administrator). Unlike a
// reveal it is not a human viewing the password, so it does not schedule a rotation.
func (s *Store) LAPSAdminCredentials(ctx context.Context, computerID string) (account, password string, err error) {
	var sealed sql.NullString
	if err = s.DB.QueryRowContext(ctx, `SELECT account, password_encrypted FROM laps_passwords WHERE computer_id = $1`, computerID).Scan(&account, &sealed); err != nil {
		return "", "", wrapNotFound(err)
	}
	if account == "" || !sealed.Valid || sealed.String == "" {
		return "", "", ErrNotFound
	}
	password, err = s.Box.Open(sealed.String)
	return account, password, err
}
