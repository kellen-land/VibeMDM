package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

// GetPkgSigningCA returns the package-signing CA (key unsealed), or ErrNotFound.
func (s *Store) GetPkgSigningCA(ctx context.Context) (certPEM, keyPEM string, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT cert_pem, key_pem FROM pkg_signing_ca WHERE id = 1`).Scan(&certPEM, &keyPEM)
	if err != nil {
		return "", "", wrapNotFound(err)
	}
	keyPEM, err = s.Box.Open(keyPEM)
	return certPEM, keyPEM, err
}

// CreatePkgSigningCA stores the CA only if none exists. Like the SCEP CA it is never
// replaced implicitly: packages signed under the old CA would stop installing.
func (s *Store) CreatePkgSigningCA(ctx context.Context, certPEM, keyPEM string) error {
	sealed, err := s.Box.Seal(keyPEM)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO pkg_signing_ca (id, cert_pem, key_pem) VALUES (1, $1, $2) ON CONFLICT (id) DO NOTHING`, certPEM, sealed)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCAExists
	}
	return nil
}

// SaveAgentPackage records the uploaded package (the file itself lives in the data directory).
func (s *Store) SaveAgentPackage(ctx context.Context, p model.AgentPackage) error {
	md5s, _ := json.Marshal(p.MD5s)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO agent_package (id, file_name, version, size, sha256, md5_chunk, md5s, signed_by, uploaded_by, uploaded_at)
		VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (id) DO UPDATE SET file_name = EXCLUDED.file_name, version = EXCLUDED.version, size = EXCLUDED.size,
		sha256 = EXCLUDED.sha256, md5_chunk = EXCLUDED.md5_chunk, md5s = EXCLUDED.md5s, signed_by = EXCLUDED.signed_by,
		uploaded_by = EXCLUDED.uploaded_by, uploaded_at = now()`,
		p.FileName, p.Version, p.Size, p.SHA256, p.MD5Chunk, md5s, p.SignedBy, p.UploadedBy)
	return err
}

// GetAgentPackage returns the package record, or ErrNotFound.
func (s *Store) GetAgentPackage(ctx context.Context) (*model.AgentPackage, error) {
	var p model.AgentPackage
	var md5s []byte
	err := s.DB.QueryRowContext(ctx, `SELECT file_name, version, size, sha256, md5_chunk, md5s, signed_by, uploaded_by, uploaded_at
		FROM agent_package WHERE id = 1`).Scan(&p.FileName, &p.Version, &p.Size, &p.SHA256, &p.MD5Chunk, &md5s, &p.SignedBy, &p.UploadedBy, &p.UploadedAt)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	json.Unmarshal(md5s, &p.MD5s)
	return &p, nil
}

// CreateBootstrapToken stores a single-use agent enrollment token for one Mac.
func (s *Store) CreateBootstrapToken(ctx context.Context, computerID, tokenHash string, ttl time.Duration) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO agent_bootstrap_tokens (token_hash, computer_id, expires_at) VALUES ($1, $2, $3)`,
		tokenHash, computerID, time.Now().Add(ttl))
	return err
}

// ErrBootstrapDevice means a bootstrap token was presented by a different Mac.
var ErrBootstrapDevice = errors.New("this enrollment token belongs to a different Mac")

// ConsumeBootstrapToken validates and uses a bootstrap token. It only works for the Mac it
// was issued to (matched by UDID or serial number).
func (s *Store) ConsumeBootstrapToken(ctx context.Context, tokenHash, udid, serial string) (computerID string, err error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var cUDID, cSerial string
	err = tx.QueryRowContext(ctx, `SELECT t.computer_id, COALESCE(c.udid,''), COALESCE(c.serial_number,'')
		FROM agent_bootstrap_tokens t JOIN computers c ON c.id = t.computer_id
		WHERE t.token_hash = $1 AND t.used_at IS NULL AND t.expires_at > now() FOR UPDATE OF t`, tokenHash).Scan(&computerID, &cUDID, &cSerial)
	if err != nil {
		return "", wrapNotFound(err)
	}
	if !((udid != "" && udid == cUDID) || (serial != "" && serial == cSerial)) {
		return "", ErrBootstrapDevice
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_bootstrap_tokens SET used_at = now() WHERE token_hash = $1`, tokenHash); err != nil {
		return "", err
	}
	return computerID, tx.Commit()
}

// HasBootstrap reports whether a bootstrap profile was sent to the Mac (any token issued).
func (s *Store) HasBootstrap(ctx context.Context, computerID string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM agent_bootstrap_tokens WHERE computer_id = $1`, computerID).Scan(&n)
	return n > 0, err
}

// ClearBootstrap forgets a Mac's bootstrap tokens (after the profile is removed).
func (s *Store) ClearBootstrap(ctx context.Context, computerID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM agent_bootstrap_tokens WHERE computer_id = $1`, computerID)
	return err
}

// RecordAgentInstall logs an install attempt.
func (s *Store) RecordAgentInstall(ctx context.Context, computerID, commandUUID, createdBy string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO agent_install_attempts (computer_id, command_uuid, created_by) VALUES ($1, $2, $3)`,
		computerID, commandUUID, createdBy)
	return err
}

// UpdateAgentInstall sets the outcome of the attempt with this command UUID.
func (s *Store) UpdateAgentInstall(ctx context.Context, commandUUID, status, detail string) (bool, error) {
	if len(detail) > 500 {
		detail = detail[:500]
	}
	// The Mac's command reply can arrive after the agent already enrolled: never downgrade "installed".
	res, err := s.DB.ExecContext(ctx, `UPDATE agent_install_attempts SET status = $2, detail = $3, updated_at = now()
		WHERE command_uuid = $1 AND status <> 'installed'`, commandUUID, status, detail)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MarkAgentInstalled marks a Mac's latest attempt as installed once its agent enrolls.
func (s *Store) MarkAgentInstalled(ctx context.Context, computerID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE agent_install_attempts SET status = 'installed', detail = 'agent enrolled', updated_at = now()
		WHERE id = (SELECT id FROM agent_install_attempts WHERE computer_id = $1 ORDER BY created_at DESC LIMIT 1)`, computerID)
	return err
}

// LatestAgentInstall returns a Mac's most recent install attempt (nil if none).
func (s *Store) LatestAgentInstall(ctx context.Context, computerID string) (*model.AgentInstallAttempt, error) {
	var a model.AgentInstallAttempt
	err := s.DB.QueryRowContext(ctx, `SELECT id, computer_id, command_uuid, status, detail, created_by, created_at, updated_at
		FROM agent_install_attempts WHERE computer_id = $1 ORDER BY created_at DESC LIMIT 1`, computerID).
		Scan(&a.ID, &a.ComputerID, &a.CommandUUID, &a.Status, &a.Detail, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &a, err
}

// AgentInstallsSince counts install attempts for a Mac since t.
func (s *Store) AgentInstallsSince(ctx context.Context, computerID string, t time.Time) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM agent_install_attempts WHERE computer_id = $1 AND created_at > $2`, computerID, t).Scan(&n)
	return n, err
}
