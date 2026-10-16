// Package store contains all SQL for VibeMDM's own tables.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/secrets"
	"github.com/vibemdm/vibemdm/internal/selfservice"
)

// ErrNotFound is returned when a row does not exist. It wraps fs.ErrNotExist so
// packages that don't import store (e.g. scepsrv) can still recognise it.
var ErrNotFound = fmt.Errorf("not found: %w", fs.ErrNotExist)

// Store wraps the database. Box (optional) encrypts sensitive values at rest.
type Store struct {
	DB  *sql.DB
	Box *secrets.Box
}

// New creates a Store.
func New(d *sql.DB) *Store { return &Store{DB: d} }

// HashToken returns the hex SHA-256 of a secret for storage/lookup.
func HashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

func wrapNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func jsonArg(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	// jsonb rejects the NUL escape that tool output can contain (json.Marshal already
	// replaces invalid UTF-8).
	return strings.ReplaceAll(string(b), `\u0000`, `\ufffd`)
}

// cleanText makes command output storable in a text column: PostgreSQL rejects NUL bytes
// and invalid UTF-8 ("invalid byte sequence for encoding UTF8"), which tool output can contain.
func cleanText(s string) string {
	return strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), "\uFFFD")
}

// ---- settings ----

const settingsKey = "core"

// DefaultSettings returns default settings.
func DefaultSettings() model.Settings {
	return model.Settings{
		OrgName:                     "VibeMDM",
		AgentCheckinIntervalSeconds: 300,
		EnrollmentProfileName:       "VibeMDM Enrollment",
		EnrollmentProfileIdentifier: "com.vibemdm.enrollment",
		InventoryIntervalHours:      24,
		RemoteShellIdleTimeoutSecs:  600,
		LogRetentionDays:            30,
	}
}

// GetSettings loads settings, filling defaults for missing fields.
func (s *Store) GetSettings(ctx context.Context) (model.Settings, error) {
	st := DefaultSettings()
	var raw db.JSON[model.Settings]
	err := s.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, settingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	v := raw.V
	if v.AgentCheckinIntervalSeconds == 0 {
		v.AgentCheckinIntervalSeconds = st.AgentCheckinIntervalSeconds
	}
	if v.EnrollmentProfileName == "" {
		v.EnrollmentProfileName = st.EnrollmentProfileName
	}
	if v.EnrollmentProfileIdentifier == "" {
		v.EnrollmentProfileIdentifier = st.EnrollmentProfileIdentifier
	}
	if v.OrgName == "" {
		v.OrgName = st.OrgName
	}
	if v.SelfServiceAccentColor == "" || strings.EqualFold(v.SelfServiceAccentColor, selfservice.LegacyAccent) {
		v.SelfServiceAccentColor = selfservice.DefaultAccent
	}
	if v.InventoryIntervalHours == 0 {
		v.InventoryIntervalHours = st.InventoryIntervalHours
	}
	if v.RemoteShellIdleTimeoutSecs == 0 {
		v.RemoteShellIdleTimeoutSecs = st.RemoteShellIdleTimeoutSecs
	}
	if v.LogRetentionDays == 0 {
		v.LogRetentionDays = st.LogRetentionDays
	}
	if v.SigningKeyPEM != "" {
		k, err := s.Box.Open(v.SigningKeyPEM)
		if err != nil {
			return v, fmt.Errorf("profile signing key: %w", err)
		}
		v.SigningKeyPEM = k
	}
	return v, nil
}

// SaveSettings stores settings (the profile signing key is sealed at rest).
func (s *Store) SaveSettings(ctx context.Context, st model.Settings) error {
	if st.SigningKeyPEM != "" && !secrets.IsEncrypted(st.SigningKeyPEM) {
		sealed, err := s.Box.Seal(st.SigningKeyPEM)
		if err != nil {
			return err
		}
		st.SigningKeyPEM = sealed
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, settingsKey, jsonArg(st))
	return err
}

// ---- admin users ----

const userCols = `id, username, display_name, email, role, enabled, created_at, last_login_at, must_change_password`

func scanUser(row interface{ Scan(...any) error }) (*model.AdminUser, string, error) {
	var u model.AdminUser
	var hash string
	var last sql.NullTime
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Role, &u.Enabled, &u.CreatedAt, &last, &u.MustChangePassword, &hash)
	if err != nil {
		return nil, "", wrapNotFound(err)
	}
	u.LastLoginAt = db.TimePtr(last)
	return &u, hash, nil
}

// CountUsers returns the number of admin users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM admin_users`).Scan(&n)
	return n, err
}

// CreateUser inserts a user.
func (s *Store) CreateUser(ctx context.Context, username, displayName, email, role, passwordHash string) (*model.AdminUser, error) {
	row := s.DB.QueryRowContext(ctx, `INSERT INTO admin_users (username, display_name, email, role, password_hash)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+userCols+`, password_hash`, username, displayName, email, role, passwordHash)
	u, _, err := scanUser(row)
	return u, err
}

// GetUserByUsername returns a user and their password hash.
// GetUserWithHash returns a user and their password hash by ID.
func (s *Store) GetUserWithHash(ctx context.Context, id string) (*model.AdminUser, string, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+`, password_hash FROM admin_users WHERE id = $1`, id))
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*model.AdminUser, string, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+userCols+`, password_hash FROM admin_users WHERE lower(username) = lower($1)`, username)
	return scanUser(row)
}

// GetUser returns a user by ID.
func (s *Store) GetUser(ctx context.Context, id string) (*model.AdminUser, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+userCols+`, password_hash FROM admin_users WHERE id = $1`, id)
	u, _, err := scanUser(row)
	return u, err
}

// ListUsers returns all users.
func (s *Store) ListUsers(ctx context.Context) ([]model.AdminUser, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+userCols+`, password_hash FROM admin_users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AdminUser{}
	for rows.Next() {
		u, _, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// UpdateUser updates profile fields.
func (s *Store) UpdateUser(ctx context.Context, id, displayName, email, role string, enabled bool) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE admin_users SET display_name=$2, email=$3, role=$4, enabled=$5, updated_at=now() WHERE id=$1`,
		id, displayName, email, role, enabled)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetUserPassword updates a password hash.
func (s *Store) SetUserPassword(ctx context.Context, id, hash string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE admin_users SET password_hash=$2, must_change_password=FALSE, updated_at=now() WHERE id=$1`, id, hash)
	return err
}

// SetMustChangePassword flags (or clears) a forced password change.
func (s *Store) SetMustChangePassword(ctx context.Context, id string, must bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE admin_users SET must_change_password=$2 WHERE id=$1`, id, must)
	return err
}

// DeleteUserSessions ends a user's sessions, optionally keeping one (by token hash).
func (s *Store) DeleteUserSessions(ctx context.Context, userID, exceptHash string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, userID, exceptHash)
	return err
}

// DeleteUser removes a user.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// The user's API tokens go with them (api_tokens.created_by would otherwise just become NULL).
	if _, err := tx.ExecContext(ctx, `DELETE FROM api_tokens WHERE created_by = $1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_users WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// TouchUserLogin records a login.
func (s *Store) TouchUserLogin(ctx context.Context, id string) {
	s.DB.ExecContext(ctx, `UPDATE admin_users SET last_login_at=now() WHERE id=$1`, id)
}

// ---- sessions ----

// CreateSession stores a session token hash.
func (s *Store) CreateSession(ctx context.Context, tokenHash, userID string, ttl time.Duration, ua, ip string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at, user_agent, ip) VALUES ($1, $2, $3, $4, $5)`,
		tokenHash, userID, time.Now().Add(ttl), ua, ip)
	return err
}

// GetSessionUser returns the user for a valid session.
// A session idle for longer than idle (0 = never) no longer counts; each use refreshes it.
func (s *Store) GetSessionUser(ctx context.Context, tokenHash string, idle time.Duration) (*model.AdminUser, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+prefixCols("u.", userCols)+`, u.password_hash FROM sessions s JOIN admin_users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now() AND u.enabled
		AND ($2::float8 = 0 OR s.last_seen_at > now() - make_interval(secs => $2::float8))`, tokenHash, idle.Seconds())
	u, _, err := scanUser(row)
	if err == nil {
		s.DB.ExecContext(ctx, `UPDATE sessions SET last_seen_at = now() WHERE token_hash = $1 AND last_seen_at < now() - interval '30 seconds'`, tokenHash)
	}
	return u, err
}

// DeleteSession removes a session.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=$1`, tokenHash)
	return err
}

// DeleteExpiredSessions prunes old sessions.
func (s *Store) DeleteExpiredSessions(ctx context.Context) {
	s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
}

func prefixCols(prefix, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

// ---- API tokens ----

// CreateAPIToken stores a token.
func (s *Store) CreateAPIToken(ctx context.Context, name, tokenHash, prefix, role, createdBy string, expires *time.Time) (*model.APIToken, error) {
	var t model.APIToken
	var exp, last sql.NullTime
	err := s.DB.QueryRowContext(ctx, `INSERT INTO api_tokens (name, token_hash, token_prefix, role, created_by, expires_at)
		VALUES ($1, $2, $3, $4, NULLIF($5,'')::uuid, $6) RETURNING id, name, token_prefix, role, created_at, last_used_at, expires_at`,
		name, tokenHash, prefix, role, createdBy, db.NullTime(expires)).Scan(&t.ID, &t.Name, &t.TokenPrefix, &t.Role, &t.CreatedAt, &last, &exp)
	if err != nil {
		return nil, err
	}
	t.LastUsedAt, t.ExpiresAt = db.TimePtr(last), db.TimePtr(exp)
	return &t, nil
}

// GetAPIToken looks up a token by hash (valid, unexpired).
func (s *Store) GetAPIToken(ctx context.Context, tokenHash string) (*model.APIToken, error) {
	var t model.APIToken
	var exp, last sql.NullTime
	// A token stops working when the user who created it is disabled (deleting a user deletes them).
	err := s.DB.QueryRowContext(ctx, `SELECT t.id, t.name, t.token_prefix, t.role, t.created_at, t.last_used_at, t.expires_at FROM api_tokens t
		LEFT JOIN admin_users u ON u.id = t.created_by
		WHERE t.token_hash = $1 AND (t.expires_at IS NULL OR t.expires_at > now()) AND (t.created_by IS NULL OR u.enabled)`, tokenHash).Scan(&t.ID, &t.Name, &t.TokenPrefix, &t.Role, &t.CreatedAt, &last, &exp)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	t.LastUsedAt, t.ExpiresAt = db.TimePtr(last), db.TimePtr(exp)
	s.DB.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = now() WHERE id = $1`, t.ID)
	return &t, nil
}

// ListAPITokens returns tokens.
func (s *Store) ListAPITokens(ctx context.Context) ([]model.APIToken, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, token_prefix, role, created_at, last_used_at, expires_at FROM api_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.APIToken{}
	for rows.Next() {
		var t model.APIToken
		var exp, last sql.NullTime
		if err := rows.Scan(&t.ID, &t.Name, &t.TokenPrefix, &t.Role, &t.CreatedAt, &last, &exp); err != nil {
			return nil, err
		}
		t.LastUsedAt, t.ExpiresAt = db.TimePtr(last), db.TimePtr(exp)
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteAPIToken removes a token.
func (s *Store) DeleteAPIToken(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM api_tokens WHERE id=$1`, id)
	return err
}

// ---- enrollment invitations ----

const invCols = `id, name, token_prefix, enabled, max_uses, uses, expires_at, created_at`

func scanInvitation(row interface{ Scan(...any) error }) (*model.EnrollmentInvitation, error) {
	var i model.EnrollmentInvitation
	var max sql.NullInt64
	var exp sql.NullTime
	if err := row.Scan(&i.ID, &i.Name, &i.TokenPrefix, &i.Enabled, &max, &i.Uses, &exp, &i.CreatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	if max.Valid {
		v := int(max.Int64)
		i.MaxUses = &v
	}
	i.ExpiresAt = db.TimePtr(exp)
	return &i, nil
}

// CreateInvitation stores an invitation.
func (s *Store) CreateInvitation(ctx context.Context, name, tokenHash, prefix string, maxUses *int, expires *time.Time, createdBy string) (*model.EnrollmentInvitation, error) {
	var max sql.NullInt64
	if maxUses != nil {
		max = sql.NullInt64{Int64: int64(*maxUses), Valid: true}
	}
	row := s.DB.QueryRowContext(ctx, `INSERT INTO enrollment_invitations (name, token_hash, token_prefix, max_uses, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6,'')::uuid) RETURNING `+invCols, name, tokenHash, prefix, max, db.NullTime(expires), createdBy)
	return scanInvitation(row)
}

// ListInvitations returns invitations.
func (s *Store) ListInvitations(ctx context.Context) ([]model.EnrollmentInvitation, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+invCols+` FROM enrollment_invitations ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.EnrollmentInvitation{}
	for rows.Next() {
		i, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

// SetInvitationEnabled toggles an invitation.
func (s *Store) SetInvitationEnabled(ctx context.Context, id string, enabled bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE enrollment_invitations SET enabled=$2 WHERE id=$1`, id, enabled)
	return err
}

// DeleteInvitation removes an invitation.
func (s *Store) DeleteInvitation(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM enrollment_invitations WHERE id=$1`, id)
	return err
}

// CheckInvitation validates a token without consuming it.
func (s *Store) CheckInvitation(ctx context.Context, tokenHash string) (*model.EnrollmentInvitation, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+invCols+` FROM enrollment_invitations WHERE token_hash=$1 AND enabled
		AND (expires_at IS NULL OR expires_at > now()) AND (max_uses IS NULL OR uses < max_uses)`, tokenHash)
	return scanInvitation(row)
}

// ConsumeInvitation validates and increments uses.
func (s *Store) ConsumeInvitation(ctx context.Context, tokenHash string) (*model.EnrollmentInvitation, error) {
	row := s.DB.QueryRowContext(ctx, `UPDATE enrollment_invitations SET uses = uses + 1 WHERE token_hash=$1 AND enabled
		AND (expires_at IS NULL OR expires_at > now()) AND (max_uses IS NULL OR uses < max_uses) RETURNING `+invCols, tokenHash)
	return scanInvitation(row)
}

// ---- audit ----

// Audit records an audit entry (errors are ignored; auditing must not break requests).
func (s *Store) Audit(ctx context.Context, actor, action, targetType, targetID string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	s.DB.ExecContext(ctx, `INSERT INTO audit_log (actor, action, target_type, target_id, details) VALUES ($1,$2,$3,$4,$5::jsonb)`,
		actor, action, targetType, targetID, jsonArg(details))
}

// AuditStrict records an audit entry and reports failure. Secret reveals use it and withhold
// the secret when the record could not be written.
func (s *Store) AuditStrict(ctx context.Context, actor, action, targetType, targetID string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO audit_log (actor, action, target_type, target_id, details) VALUES ($1,$2,$3,$4,$5::jsonb)`,
		actor, action, targetType, targetID, jsonArg(details))
	return err
}

// ListAudit returns recent audit entries.
func (s *Store) ListAudit(ctx context.Context, limit, offset int, targetID string) ([]model.AuditEntry, int, error) {
	// targetID ("" for all) limits the log to one object's history.
	limit, offset = clampPage(limit, offset)
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE $1 = '' OR target_id = $1`, targetID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, at, actor, action, target_type, target_id, details FROM audit_log WHERE $3 = '' OR target_id = $3
		ORDER BY at DESC LIMIT $1 OFFSET $2`, limit, offset, targetID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.AuditEntry{}
	for rows.Next() {
		var e model.AuditEntry
		var d db.JSON[map[string]any]
		if err := rows.Scan(&e.ID, &e.At, &e.Actor, &e.Action, &e.TargetType, &e.TargetID, &d); err != nil {
			return nil, 0, err
		}
		e.Details = d.V
		if e.Details == nil {
			e.Details = map[string]any{}
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// ---- SCEP depot ----

// GetSCEPCA returns the CA cert and key PEM.
func (s *Store) GetSCEPCA(ctx context.Context) (certPEM, keyPEM string, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT cert_pem, key_pem FROM scep_ca WHERE id = 1`).Scan(&certPEM, &keyPEM)
	if err != nil {
		return "", "", wrapNotFound(err)
	}
	keyPEM, err = s.Box.Open(keyPEM)
	return certPEM, keyPEM, err
}

// SaveSCEPCA stores (replaces) the CA.
func (s *Store) SaveSCEPCA(ctx context.Context, certPEM, keyPEM string) error {
	keyPEM, err := s.Box.Seal(keyPEM)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO scep_ca (id, cert_pem, key_pem) VALUES (1, $1, $2)
		ON CONFLICT (id) DO UPDATE SET cert_pem = EXCLUDED.cert_pem, key_pem = EXCLUDED.key_pem, created_at = now()`, certPEM, keyPEM)
	return err
}

// CreateSCEPCA stores the CA only if none exists (first boot). It never
// overwrites: ErrCAExists is returned when a CA is already stored.
func (s *Store) CreateSCEPCA(ctx context.Context, certPEM, keyPEM string) error {
	keyPEM, err := s.Box.Seal(keyPEM)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO scep_ca (id, cert_pem, key_pem) VALUES (1, $1, $2) ON CONFLICT (id) DO NOTHING`, certPEM, keyPEM)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCAExists
	}
	return nil
}

// ErrCAExists is returned by CreateSCEPCA when a CA is already stored.
var ErrCAExists = errors.New("a SCEP CA already exists")

// NextSCEPSerial returns a new serial number.
func (s *Store) NextSCEPSerial(ctx context.Context) (int64, error) {
	var n int64
	err := s.DB.QueryRowContext(ctx, `SELECT nextval('scep_serial_seq')`).Scan(&n)
	return n, err
}

// PutSCEPCert records an issued certificate.
func (s *Store) PutSCEPCert(ctx context.Context, serial string, cn, certPEM string, notBefore, notAfter time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO scep_certs (serial, common_name, cert_pem, not_before, not_after) VALUES ($1::numeric, $2, $3, $4, $5)
		ON CONFLICT (serial) DO NOTHING`, serial, cn, certPEM, notBefore, notAfter)
	return err
}

// SCEPStats returns counts of issued certs.
func (s *Store) SCEPStats(ctx context.Context) (issued int, active int, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT count(*), count(*) FILTER (WHERE not_after > now() AND revoked_at IS NULL) FROM scep_certs`).Scan(&issued, &active)
	return
}

// ---- push cert request ----

// SavePushCertRequest stores the APNs CSR key material.
func (s *Store) SavePushCertRequest(ctx context.Context, keyPEM, csrPEM, certPEM string) error {
	keyPEM, err := s.Box.Seal(keyPEM)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO push_cert_requests (id, key_pem, csr_pem, cert_pem) VALUES (1, $1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET key_pem = EXCLUDED.key_pem, csr_pem = EXCLUDED.csr_pem, cert_pem = EXCLUDED.cert_pem, created_at = now()`, keyPEM, csrPEM, certPEM)
	return err
}

// GetPushCertRequest returns the stored APNs CSR key material.
func (s *Store) GetPushCertRequest(ctx context.Context) (keyPEM, csrPEM, certPEM string, createdAt time.Time, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT key_pem, csr_pem, cert_pem, created_at FROM push_cert_requests WHERE id = 1`).Scan(&keyPEM, &csrPEM, &certPEM, &createdAt)
	if err != nil {
		return "", "", "", createdAt, wrapNotFound(err)
	}
	keyPEM, err = s.Box.Open(keyPEM)
	return keyPEM, csrPEM, certPEM, createdAt, err
}

// GetPushCertTopics lists push cert topics with expiry (from the NanoMDM table).
func (s *Store) GetPushCertTopics(ctx context.Context) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT topic FROM push_certs ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetPushCertPEM returns the stored push cert PEM for a topic.
func (s *Store) GetPushCertPEM(ctx context.Context, topic string) (string, error) {
	var pem string
	err := s.DB.QueryRowContext(ctx, `SELECT cert_pem FROM push_certs WHERE topic = $1`, topic).Scan(&pem)
	return pem, wrapNotFound(err)
}

// Helper for paging.
// ExportLimit caps CSV exports.
const ExportLimit = 10000

// clampFilterPage is clampPage, except that exports may return up to ExportLimit rows.
func clampFilterPage(limit, offset int, export bool) (int, int) {
	if !export {
		return clampPage(limit, offset)
	}
	if limit <= 0 || limit > ExportLimit {
		limit = ExportLimit
	}
	return limit, max(offset, 0)
}

func clampPage(limit, offset int) (int, int) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// PageInfo is paging metadata.
type PageInfo struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func likeArg(s string) string {
	return "%" + strings.ReplaceAll(strings.ReplaceAll(s, "%", `\%`), "_", `\_`) + "%"
}

var _ = fmt.Sprintf
