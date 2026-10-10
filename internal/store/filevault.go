package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/escrow"
	"github.com/vibemdm/vibemdm/internal/model"
)

// CreateEscrowIdentity stores the escrow identity only if none exists.
func (s *Store) CreateEscrowIdentity(ctx context.Context, certPEM, keyPEM string) error {
	sealed, err := s.Box.Seal(keyPEM)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO escrow_identity (id, cert_pem, key_pem) VALUES (1, $1, $2) ON CONFLICT (id) DO NOTHING`, certPEM, sealed)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("an escrow identity already exists")
	}
	return nil
}

// SaveFileVaultKey stores (sealed) a computer's personal recovery key.
// Returns changed=false when the same key was already escrowed.
func (s *Store) SaveFileVaultKey(ctx context.Context, computerID, prk, source string) (changed bool, err error) {
	var existing string
	err = s.DB.QueryRowContext(ctx, `SELECT key_encrypted FROM filevault_keys WHERE computer_id = $1`, computerID).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil {
		if old, oerr := s.Box.Open(existing); oerr == nil && old == prk {
			return false, nil
		}
	}
	sealed, err := s.Box.Seal(prk)
	if err != nil {
		return false, err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO filevault_keys (computer_id, key_encrypted, fingerprint, source, escrowed_at, rotated_count)
		VALUES ($1, $2, $3, $4, now(), 0)
		ON CONFLICT (computer_id) DO UPDATE SET key_encrypted = EXCLUDED.key_encrypted, fingerprint = EXCLUDED.fingerprint,
		source = EXCLUDED.source, escrowed_at = now(), rotated_count = filevault_keys.rotated_count + 1`,
		computerID, sealed, escrow.Fingerprint(prk), source)
	return err == nil, err
}

// GetFileVaultKey returns a computer's decrypted recovery key.
func (s *Store) GetFileVaultKey(ctx context.Context, computerID string) (prk string, st *model.FileVaultStatus, err error) {
	var sealed string
	st = &model.FileVaultStatus{}
	var at time.Time
	err = s.DB.QueryRowContext(ctx, `SELECT key_encrypted, fingerprint, escrowed_at, rotated_count, source FROM filevault_keys WHERE computer_id = $1`, computerID).
		Scan(&sealed, &st.Fingerprint, &at, &st.RotatedCount, &st.Source)
	if err != nil {
		return "", nil, wrapNotFound(err)
	}
	st.Escrowed, st.EscrowedAt = true, &at
	prk, err = s.Box.Open(sealed)
	return prk, st, err
}

// FileVaultStatusFor returns escrow status (never the key) for one computer.
func (s *Store) FileVaultStatusFor(ctx context.Context, computerID string) (*model.FileVaultStatus, error) {
	st := &model.FileVaultStatus{}
	var at sql.NullTime
	var fp, src sql.NullString
	var rot sql.NullInt64
	var profileStatus sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT k.fingerprint, k.escrowed_at, k.rotated_count, k.source,
		(SELECT d.status FROM profile_deployments d JOIN config_profiles p ON p.id = d.profile_id WHERE p.identifier = $2 AND d.computer_id = c.id)
		FROM computers c LEFT JOIN filevault_keys k ON k.computer_id = c.id WHERE c.id = $1`, computerID, escrow.EscrowProfileIdentifier).
		Scan(&fp, &at, &rot, &src, &profileStatus)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	st.Escrowed = at.Valid
	st.EscrowedAt = db.TimePtr(at)
	st.Fingerprint, st.Source, st.RotatedCount, st.EscrowProfileStatus = fp.String, src.String, int(rot.Int64), profileStatus.String
	return st, nil
}

// FileVaultOverview returns the fleet escrow picture.
func (s *Store) FileVaultOverview(ctx context.Context) ([]model.FileVaultRow, model.FileVaultSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id, c.name, COALESCE(c.serial_number,''), c.filevault_enabled, c.mdm_enrolled, c.agent_enrolled,
		k.fingerprint, k.escrowed_at, k.rotated_count,
		(SELECT d.status FROM profile_deployments d JOIN config_profiles p ON p.id = d.profile_id WHERE p.identifier = $1 AND d.computer_id = c.id)
		FROM computers c LEFT JOIN filevault_keys k ON k.computer_id = c.id ORDER BY lower(c.name)`, escrow.EscrowProfileIdentifier)
	var sum model.FileVaultSummary
	if err != nil {
		return nil, sum, err
	}
	defer rows.Close()
	out := []model.FileVaultRow{}
	for rows.Next() {
		var r model.FileVaultRow
		var fv sql.NullBool
		var fp, ps sql.NullString
		var at sql.NullTime
		var rot sql.NullInt64
		if err := rows.Scan(&r.ComputerID, &r.ComputerName, &r.SerialNumber, &fv, &r.MDMEnrolled, &r.AgentEnrolled, &fp, &at, &rot, &ps); err != nil {
			return nil, sum, err
		}
		if fv.Valid {
			v := fv.Bool
			r.FileVaultEnabled = &v
		}
		r.Escrowed, r.EscrowedAt, r.Fingerprint, r.RotatedCount, r.EscrowProfileStatus = at.Valid, db.TimePtr(at), fp.String, int(rot.Int64), ps.String
		sum.Total++
		switch {
		case r.FileVaultEnabled != nil && *r.FileVaultEnabled && r.Escrowed:
			sum.Escrowed++
		case r.FileVaultEnabled != nil && *r.FileVaultEnabled:
			sum.MissingKey++
		case r.FileVaultEnabled != nil:
			sum.NotEncrypted++
		default:
			sum.Unknown++
		}
		out = append(out, r)
	}
	return out, sum, rows.Err()
}

// ComputersNeedingKeyCheck returns MDM-enrolled Macs with FileVault on and no
// escrowed key that haven't been asked for SecurityInfo recently.
func (s *Store) ComputersNeedingKeyCheck(ctx context.Context, within time.Duration) ([]model.Computer, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id FROM computers c
		WHERE c.mdm_enrolled AND c.filevault_enabled
		AND NOT EXISTS (SELECT 1 FROM filevault_keys k WHERE k.computer_id = c.id)
		AND NOT EXISTS (SELECT 1 FROM mdm_commands_meta m WHERE m.computer_id = c.id AND m.request_type = 'SecurityInfo'
			AND m.created_at > now() - make_interval(secs => $1))`, within.Seconds())
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
	out := []model.Computer{}
	for _, id := range ids {
		c, err := s.GetComputer(ctx, id)
		if err == nil {
			out = append(out, *c)
		}
	}
	return out, nil
}

// HasRecentTask reports whether a task of the given type was created for the computer within the window.
func (s *Store) HasRecentTask(ctx context.Context, computerID, typ string, within time.Duration) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM agent_tasks WHERE computer_id = $1 AND type = $2
		AND created_at > now() - make_interval(secs => $3) AND status <> 'cancelled'`, computerID, typ, within.Seconds()).Scan(&n)
	return n > 0, err
}
