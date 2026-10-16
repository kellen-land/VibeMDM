package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
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

// sealedColumn is one column that holds values sealed with s.Box.
type sealedColumn struct{ table, col, id string }

// sealedColumns lists every column written with s.Box.Seal. ResealSecrets, VerifyEncryptionKey
// and RekeySecrets all walk this list (plus the JSON-held secrets below), so a key rotation
// cannot leave a secret behind under the old key. TestSealedColumnsCoverSealCalls guards it.
var sealedColumns = []sealedColumn{
	{"scep_ca", "key_pem", "id"},
	{"push_cert_requests", "key_pem", "id"},
	{"escrow_identity", "key_pem", "id"},
	{"push_certs", "key_pem", "topic"},
	{"pkg_signing_ca", "key_pem", "id"},
	{"filevault_keys", "key_encrypted", "computer_id"},
	{"filevault_key_history", "key_encrypted", "id"},
	{"laps_passwords", "password_encrypted", "computer_id"},
	{"laps_passwords", "pending_encrypted", "computer_id"},
	{"laps_passwords", "desired_encrypted", "computer_id"},
	{"laps_password_history", "password_encrypted", "id"},
	{"laps_token_grants", "admin_password_encrypted", "computer_id"},
	{"mobile_device_activation_lock", "bypass_code_encrypted", "mobile_device_id"},
	{"super_config", "auth_password", "id"},
}

// queryer is the part of *sql.DB and *sql.Tx the secret passes need.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type sealedValue struct{ id, val string }

// readSealed returns the non-empty values of one column (nothing if the table does not exist,
// e.g. before its migration).
func readSealed(ctx context.Context, q queryer, c sealedColumn) ([]sealedValue, error) {
	var exists bool
	if err := q.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, c.table).Scan(&exists); err != nil || !exists {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT `+c.id+`::text, `+c.col+` FROM `+c.table+` WHERE `+c.col+` IS NOT NULL AND `+c.col+` <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sealedValue
	for rows.Next() {
		var v sealedValue
		if err := rows.Scan(&v.id, &v.val); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func writeSealed(ctx context.Context, q queryer, c sealedColumn, id, val string) error {
	_, err := q.ExecContext(ctx, `UPDATE `+c.table+` SET `+c.col+` = $2 WHERE `+c.id+`::text = $1`, id, val)
	return err
}

// policyAccountSecrets returns each policy's Local Accounts JSON (passwords sealed inside).
func policyAccountSecrets(ctx context.Context, q queryer) (map[string][]map[string]any, error) {
	rows, err := q.QueryContext(ctx, `SELECT id::text, accounts FROM policies WHERE accounts::text LIKE '%password_sealed%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]map[string]any{}
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var accs []map[string]any
		if err := json.Unmarshal(raw, &accs); err != nil {
			return nil, fmt.Errorf("policy %s accounts: %w", id, err)
		}
		out[id] = accs
	}
	return out, rows.Err()
}

// transformPolicyAccounts rewrites every password_sealed value with fn.
func transformPolicyAccounts(ctx context.Context, q queryer, fn func(string) (string, bool, error)) (int, error) {
	pols, err := policyAccountSecrets(ctx, q)
	if err != nil {
		return 0, err
	}
	n := 0
	for id, accs := range pols {
		changed := false
		for _, a := range accs {
			v, _ := a["password_sealed"].(string)
			if v == "" {
				continue
			}
			nv, ch, err := fn(v)
			if err != nil {
				return n, fmt.Errorf("policy %s account password: %w", id, err)
			}
			if ch {
				a["password_sealed"], changed = nv, true
				n++
			}
		}
		if changed {
			if _, err := q.ExecContext(ctx, `UPDATE policies SET accounts = $2::jsonb WHERE id::text = $1`, id, jsonArg(accs)); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

// ResealSecrets encrypts any sensitive values still stored as plaintext. It is
// a no-op when encryption is disabled. Returns the number of values sealed.
func (s *Store) ResealSecrets(ctx context.Context) (int, error) {
	if !s.Box.Enabled() {
		return 0, nil
	}
	n := 0
	for _, c := range sealedColumns {
		vals, err := readSealed(ctx, s.DB, c)
		if err != nil {
			return n, err
		}
		for _, v := range vals {
			if secrets.IsEncrypted(v.val) {
				continue
			}
			sealed, err := s.Box.Seal(v.val)
			if err != nil {
				return n, err
			}
			// Same plaintext, so push_certs' stale token is intentionally left alone.
			if err := writeSealed(ctx, s.DB, c, v.id, sealed); err != nil {
				return n, err
			}
			n++
		}
	}
	m, err := transformPolicyAccounts(ctx, s.DB, func(v string) (string, bool, error) {
		if secrets.IsEncrypted(v) {
			return v, false, nil
		}
		sealed, err := s.Box.Seal(v)
		return sealed, err == nil, err
	})
	n += m
	if err != nil {
		return n, err
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
	for _, c := range sealedColumns {
		vals, err := readSealed(ctx, s.DB, c)
		if err != nil {
			return err
		}
		for _, v := range vals {
			if !secrets.IsEncrypted(v.val) {
				continue
			}
			if _, err := s.Box.Open(v.val); err != nil {
				return fmt.Errorf("%w (failed on %s.%s)", ErrKeyMismatch, c.table, c.col)
			}
		}
	}
	if _, err := transformPolicyAccounts(ctx, s.DB, func(v string) (string, bool, error) {
		if secrets.IsEncrypted(v) {
			if _, err := s.Box.Open(v); err != nil {
				return v, false, fmt.Errorf("%w (failed on policies.accounts)", ErrKeyMismatch)
			}
		}
		return v, false, nil
	}); err != nil {
		return err
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
	rekey := func(v string) (string, bool, error) {
		plain, err := oldBox.Open(v)
		if err != nil {
			return "", false, fmt.Errorf("cannot open with the old key: %w", err)
		}
		sealed, err := newBox.Seal(plain)
		return sealed, err == nil, err
	}
	n := 0
	for _, c := range sealedColumns {
		vals, err := readSealed(ctx, tx, c)
		if err != nil {
			return n, err
		}
		for _, v := range vals {
			sealed, _, err := rekey(v.val)
			if err != nil {
				return n, fmt.Errorf("%s.%s: %w", c.table, c.col, err)
			}
			if err := writeSealed(ctx, tx, c, v.id, sealed); err != nil {
				return n, err
			}
			n++
		}
	}
	m, err := transformPolicyAccounts(ctx, tx, rekey)
	n += m
	if err != nil {
		return n, err
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
