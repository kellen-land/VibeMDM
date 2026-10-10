package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/vibemdm/vibemdm/internal/secrets"
)

// ---- escrow identity ----

// GetEscrowIdentity returns the FileVault escrow cert and (decrypted) key.
func (s *Store) GetEscrowIdentity(ctx context.Context) (certPEM, keyPEM string, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT cert_pem, key_pem FROM escrow_identity WHERE id = 1`).Scan(&certPEM, &keyPEM)
	if err != nil {
		return "", "", wrapNotFound(err)
	}
	keyPEM, err = s.Box.Open(keyPEM)
	return certPEM, keyPEM, err
}

// SaveEscrowIdentity stores the escrow identity (key sealed at rest).
func (s *Store) SaveEscrowIdentity(ctx context.Context, certPEM, keyPEM string) error {
	sealed, err := s.Box.Seal(keyPEM)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO escrow_identity (id, cert_pem, key_pem) VALUES (1, $1, $2)
		ON CONFLICT (id) DO UPDATE SET cert_pem = EXCLUDED.cert_pem, key_pem = EXCLUDED.key_pem, created_at = now()`, certPEM, sealed)
	return err
}

// ---- encrypted APNs push cert store (wraps NanoMDM's push_certs table) ----

// RetrievePushCertPEM returns the push cert and decrypted key with its stale token.
func (s *Store) RetrievePushCertPEM(ctx context.Context, topic string) (certPEM, keyPEM []byte, staleToken string, err error) {
	var c, k string
	var tok int
	if err = s.DB.QueryRowContext(ctx, `SELECT cert_pem, key_pem, stale_token FROM push_certs WHERE topic = $1`, topic).Scan(&c, &k, &tok); err != nil {
		return nil, nil, "", err
	}
	k, err = s.Box.Open(k)
	if err != nil {
		return nil, nil, "", err
	}
	return []byte(c), []byte(k), strconv.Itoa(tok), nil
}

// StorePushCertSealed upserts a push cert with its key sealed at rest, bumping
// the stale token so running push providers reload it.
func (s *Store) StorePushCertSealed(ctx context.Context, topic string, certPEM, keyPEM []byte) error {
	sealed, err := s.Box.Seal(string(keyPEM))
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO push_certs (topic, cert_pem, key_pem, stale_token) VALUES ($1, $2, $3, 0)
		ON CONFLICT (topic) DO UPDATE SET cert_pem = EXCLUDED.cert_pem, key_pem = EXCLUDED.key_pem, stale_token = push_certs.stale_token + 1`,
		topic, string(certPEM), sealed)
	return err
}

// ---- re-seal pass ----

// ResealSecrets encrypts any sensitive values still stored as plaintext. It is
// a no-op when encryption is disabled. Returns the number of values sealed.
func (s *Store) ResealSecrets(ctx context.Context) (int, error) {
	if !s.Box.Enabled() {
		return 0, nil
	}
	n := 0
	type target struct{ table, keyCol, idCol string }
	for _, t := range []target{
		{"scep_ca", "key_pem", "id"},
		{"push_cert_requests", "key_pem", "id"},
		{"escrow_identity", "key_pem", "id"},
		{"push_certs", "key_pem", "topic"},
		{"filevault_keys", "key_encrypted", "computer_id"},
	} {
		rows, err := s.DB.QueryContext(ctx, `SELECT `+t.idCol+`::text, `+t.keyCol+` FROM `+t.table)
		if err != nil {
			return n, err
		}
		type pending struct{ id, val string }
		var todo []pending
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.id, &p.val); err != nil {
				rows.Close()
				return n, err
			}
			if p.val != "" && !secrets.IsEncrypted(p.val) {
				todo = append(todo, p)
			}
		}
		rows.Close()
		for _, p := range todo {
			sealed, err := s.Box.Seal(p.val)
			if err != nil {
				return n, err
			}
			// Same plaintext, so push_certs' stale token is intentionally left alone.
			if _, err := s.DB.ExecContext(ctx, `UPDATE `+t.table+` SET `+t.keyCol+` = $2 WHERE `+t.idCol+`::text = $1`, p.id, sealed); err != nil {
				return n, err
			}
			n++
		}
	}
	// Settings: the profile signing key.
	var raw sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT value->>'signing_key_pem' FROM settings WHERE key = $1`, settingsKey).Scan(&raw); err == nil &&
		raw.Valid && raw.String != "" && !secrets.IsEncrypted(raw.String) {
		st, err := s.GetSettings(ctx)
		if err != nil {
			return n, err
		}
		if err := s.SaveSettings(ctx, st); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

var _ = time.Now

// ErrKeyMismatch means stored secrets cannot be opened with the configured key.
var ErrKeyMismatch = errors.New("stored secrets cannot be decrypted with VIBEMDM_ENCRYPTION_KEY (wrong key, or the key was removed); refusing to start so nothing is overwritten")

// VerifyEncryptionKey opens every stored ciphertext once. It fails if any value
// is encrypted but the configured key (or lack of one) cannot open it.
func (s *Store) VerifyEncryptionKey(ctx context.Context) error {
	checks := []struct{ table, col string }{
		{"scep_ca", "key_pem"}, {"push_cert_requests", "key_pem"}, {"escrow_identity", "key_pem"},
		{"push_certs", "key_pem"}, {"filevault_keys", "key_encrypted"},
	}
	for _, c := range checks {
		rows, err := s.DB.QueryContext(ctx, `SELECT `+c.col+` FROM `+c.table+` WHERE `+c.col+` LIKE 'enc:v1:%'`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				rows.Close()
				return err
			}
			if _, err := s.Box.Open(v); err != nil {
				rows.Close()
				return fmt.Errorf("%w (failed on %s)", ErrKeyMismatch, c.table)
			}
		}
		rows.Close()
	}
	var sk sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT value->>'signing_key_pem' FROM settings WHERE key = $1`, settingsKey).Scan(&sk); err == nil && sk.Valid && secrets.IsEncrypted(sk.String) {
		if _, err := s.Box.Open(sk.String); err != nil {
			return fmt.Errorf("%w (failed on settings)", ErrKeyMismatch)
		}
	}
	return nil
}

// RekeySecrets re-encrypts every stored secret from oldBox to newBox in one
// transaction. oldBox may be disabled (values are plaintext); newBox may be
// disabled to decrypt everything back to plaintext. Returns values rewritten.
func (s *Store) RekeySecrets(ctx context.Context, oldBox, newBox *secrets.Box) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	type target struct{ table, col, id string }
	for _, t := range []target{
		{"scep_ca", "key_pem", "id"}, {"push_cert_requests", "key_pem", "id"}, {"escrow_identity", "key_pem", "id"},
		{"push_certs", "key_pem", "topic"}, {"filevault_keys", "key_encrypted", "computer_id"},
	} {
		rows, err := tx.QueryContext(ctx, `SELECT `+t.id+`::text, `+t.col+` FROM `+t.table+` WHERE `+t.col+` <> ''`)
		if err != nil {
			return n, err
		}
		type item struct{ id, val string }
		var items []item
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.id, &it.val); err != nil {
				rows.Close()
				return n, err
			}
			items = append(items, it)
		}
		rows.Close()
		for _, it := range items {
			plain, err := oldBox.Open(it.val)
			if err != nil {
				return n, fmt.Errorf("%s: cannot open with the old key: %w", t.table, err)
			}
			sealed, err := newBox.Seal(plain)
			if err != nil {
				return n, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE `+t.table+` SET `+t.col+` = $2 WHERE `+t.id+`::text = $1`, it.id, sealed); err != nil {
				return n, err
			}
			n++
		}
	}
	var sk sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT value->>'signing_key_pem' FROM settings WHERE key = $1`, settingsKey).Scan(&sk); err == nil && sk.Valid && sk.String != "" {
		plain, err := oldBox.Open(sk.String)
		if err != nil {
			return n, fmt.Errorf("settings signing key: cannot open with the old key: %w", err)
		}
		sealed, err := newBox.Seal(plain)
		if err != nil {
			return n, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE settings SET value = jsonb_set(value, '{signing_key_pem}', to_jsonb($2::text)) WHERE key = $1`, settingsKey, sealed); err != nil {
			return n, err
		}
		n++
	}
	return n, tx.Commit()
}

// MintSCEPChallenge creates a SCEP challenge that expires after ttl and, when
// singleUse, works for exactly one certificate request. It returns the plaintext.
func (s *Store) MintSCEPChallenge(ctx context.Context, purpose string, ttl time.Duration, singleUse bool) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	ch := base64.RawURLEncoding.EncodeToString(b)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO scep_challenges (challenge_hash, purpose, single_use, expires_at) VALUES ($1,$2,$3,$4)`,
		HashToken(ch), purpose, singleUse, time.Now().Add(ttl))
	return ch, err
}

// ConsumeSCEPChallenge reports whether a minted challenge is valid, using it up if single-use.
func (s *Store) ConsumeSCEPChallenge(ctx context.Context, challenge string) bool {
	if challenge == "" {
		return false
	}
	var h string
	err := s.DB.QueryRowContext(ctx, `UPDATE scep_challenges SET used_at = now()
		WHERE challenge_hash = $1 AND expires_at > now() AND (used_at IS NULL OR NOT single_use) RETURNING challenge_hash`, HashToken(challenge)).Scan(&h)
	return err == nil
}

// PruneSCEPChallenges deletes expired challenges.
func (s *Store) PruneSCEPChallenges(ctx context.Context) {
	s.DB.ExecContext(ctx, `DELETE FROM scep_challenges WHERE expires_at < now() - interval '1 day'`)
}
