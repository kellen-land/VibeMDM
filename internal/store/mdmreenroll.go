package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// MDMReenrollSilence is how long an enrollment must be silent before a different identity
// certificate may take it over without an admin reset (mirrors the agent takeover rule).
const MDMReenrollSilence = 30 * 24 * time.Hour

// MDMReenrollAllowed reports whether an Authenticate for enrollment id presenting the
// identity certificate with SHA-256 certHash may proceed.
//
// UDIDs are not secrets, so a device that already holds a live enrollment keeps it: a
// different certificate is accepted only when the enrollment is new, has checked out (or an
// admin allowed re-enrollment, which disables it), or has been silent for 30 days.
func (s *Store) MDMReenrollAllowed(ctx context.Context, id, certHash string) (bool, error) {
	if id == "" {
		return true, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT rtrim(sha256) FROM cert_auth_associations WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	known := false
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return false, err
		}
		if h == certHash {
			return true, nil // same certificate: an ordinary re-Authenticate
		}
		known = true
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if !known {
		return true, nil
	}
	var enabled bool
	var lastSeen time.Time
	err = s.DB.QueryRowContext(ctx, `SELECT enabled, last_seen_at FROM enrollments WHERE id = $1`, id).Scan(&enabled, &lastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !enabled || time.Since(lastSeen) > MDMReenrollSilence {
		return true, nil
	}
	return false, nil
}

// AllowMDMReenroll lets a wiped or replaced device enroll again under the same UDID with a new
// identity certificate by disabling its current enrollment (as a CheckOut would).
func (s *Store) AllowMDMReenroll(ctx context.Context, udid string) error {
	if udid == "" {
		return ErrNotFound
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE enrollments SET enabled = FALSE WHERE device_id = $1`, udid)
	return err
}

// revokeEnrollmentCerts records the identity certificates bound to udid as revoked and drops
// the NanoMDM associations. Called inside the transaction that deletes the device.
func revokeEnrollmentCerts(ctx context.Context, tx *sql.Tx, udid string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO revoked_cert_hashes (sha256, udid)
		SELECT rtrim(sha256), id FROM cert_auth_associations WHERE id = $1 ON CONFLICT DO NOTHING`, udid); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM cert_auth_associations WHERE id = $1`, udid)
	return err
}

// IsCertHashRevoked reports whether an identity certificate (SHA-256 of its DER) was revoked.
func (s *Store) IsCertHashRevoked(ctx context.Context, certHash string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM revoked_cert_hashes WHERE sha256 = $1`, certHash).Scan(&n)
	return n > 0, err
}
