package store

import (
	"context"
	"testing"
	"time"
)

func TestMDMReenrollAllowed(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	udid := "REENROLL-" + time.Now().Format("150405.000000")
	ok, err := s.MDMReenrollAllowed(ctx, udid, "hash-a")
	if err != nil || !ok {
		t.Fatalf("new enrollment refused: ok=%v err=%v", ok, err)
	}
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.DB.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO devices (id, authenticate, authenticate_at) VALUES ($1, 'x', now())`, udid)
	mustExec(`INSERT INTO enrollments (id, device_id, type, topic, push_magic, token_hex, last_seen_at) VALUES ($1, $1, 'Device', 't', 'p', 'aa', now())`, udid)
	mustExec(`INSERT INTO cert_auth_associations (id, sha256) VALUES ($1, 'hash-a')`, udid)

	if ok, _ := s.MDMReenrollAllowed(ctx, udid, "hash-a"); !ok {
		t.Fatal("same certificate refused")
	}
	if ok, _ := s.MDMReenrollAllowed(ctx, udid, "hash-b"); ok {
		t.Fatal("different certificate accepted for a live enrollment")
	}
	mustExec(`UPDATE enrollments SET last_seen_at = now() - interval '31 days' WHERE id = $1`, udid)
	if ok, _ := s.MDMReenrollAllowed(ctx, udid, "hash-b"); !ok {
		t.Fatal("enrollment silent for 31 days should be replaceable")
	}
	mustExec(`UPDATE enrollments SET last_seen_at = now() WHERE id = $1`, udid)
	if err := s.AllowMDMReenroll(ctx, udid); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.MDMReenrollAllowed(ctx, udid, "hash-b"); !ok {
		t.Fatal("admin-allowed re-enrollment refused")
	}
}

func TestDeleteComputerRevokesIdentityCert(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	udid := "REVOKE-" + time.Now().Format("150405.000000")
	comp, err := s.UpsertComputerFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: udid, SerialNumber: "R" + udid})
	if err != nil {
		t.Fatal(err)
	}
	hash := "c0ffee" + udid
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO cert_auth_associations (id, sha256) VALUES ($1, $2)`, udid, hash); err != nil {
		t.Fatal(err)
	}
	if revoked, _ := s.IsCertHashRevoked(ctx, hash); revoked {
		t.Fatal("certificate revoked before delete")
	}
	if err := s.DeleteComputer(ctx, comp.ID); err != nil {
		t.Fatal(err)
	}
	if revoked, err := s.IsCertHashRevoked(ctx, hash); err != nil || !revoked {
		t.Fatalf("certificate not revoked after delete: %v %v", revoked, err)
	}
	if ok, _ := s.MDMReenrollAllowed(ctx, udid, "fresh-cert"); !ok {
		t.Fatal("a fresh enrollment after delete should be allowed")
	}
}

func TestFileVaultKeyHistoryKeepsReplacedKeys(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	udid := "FVHIST-" + time.Now().Format("150405.000000")
	comp, err := s.UpsertComputerFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: udid, SerialNumber: "F" + udid})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"KEY-ONE", "KEY-ONE", "KEY-TWO", "KEY-THREE"} {
		if _, err := s.SaveFileVaultKey(ctx, comp.ID, k, "test"); err != nil {
			t.Fatal(err)
		}
	}
	cur, _, err := s.GetFileVaultKey(ctx, comp.ID)
	if err != nil || cur != "KEY-THREE" {
		t.Fatalf("current key = %q, %v", cur, err)
	}
	hist, err := s.FileVaultKeyHistory(ctx, comp.ID)
	if err != nil || len(hist) != 2 || hist[0].Key != "KEY-TWO" || hist[1].Key != "KEY-ONE" {
		t.Fatalf("history = %+v, %v", hist, err)
	}
}
