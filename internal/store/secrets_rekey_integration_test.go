package store

import (
	"context"
	"testing"
	"time"

	"github.com/vibemdm/vibemdm/internal/secrets"
)

func TestRekeyCoversLAPSAndActivationLock(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	oldBox, _ := secrets.New("old-test-passphrase-0123456789")
	newBox, _ := secrets.New("new-test-passphrase-9876543210")
	s.Box = oldBox
	udid := "REKEY-" + time.Now().Format("150405.000000")
	comp, err := s.UpsertComputerFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: udid, SerialNumber: "K" + udid})
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := secrets.New("")
	// The test database is shared by other tests: start from and return to plaintext.
	if _, err := s.RekeySecrets(ctx, plain, oldBox); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.RekeySecrets(ctx, newBox, plain) })
	if _, err := s.LAPSStage(ctx, comp.ID, "vibeadmin", "Pending-Password-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RekeySecrets(ctx, oldBox, newBox); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := s.DB.QueryRowContext(ctx, `SELECT pending_encrypted FROM laps_passwords WHERE computer_id = $1`, comp.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if got, err := newBox.Open(stored); err != nil || got != "Pending-Password-123" {
		t.Fatalf("LAPS pending password not re-encrypted under the new key: %q %v", got, err)
	}
	s.Box = newBox
	if err := s.VerifyEncryptionKey(ctx); err != nil {
		t.Fatalf("verify after rekey: %v", err)
	}
	s.Box = oldBox
	if err := s.VerifyEncryptionKey(ctx); err == nil {
		t.Fatal("verify with the old key should fail once LAPS secrets moved to the new key")
	}
}
