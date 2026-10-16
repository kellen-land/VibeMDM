package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

// testStore opens a migrated database from VIBEMDM_TEST_DSN (a throwaway DB), or skips.
func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("VIBEMDM_TEST_DSN")
	if dsn == "" {
		t.Skip("set VIBEMDM_TEST_DSN to a throwaway PostgreSQL database to run integration tests")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return New(d)
}

func TestSCEPChallengeLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	single, err := s.MintSCEPChallenge(ctx, "test", time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	if !s.ConsumeSCEPChallenge(ctx, single) {
		t.Fatal("fresh single-use challenge rejected")
	}
	if s.ConsumeSCEPChallenge(ctx, single) {
		t.Fatal("single-use challenge accepted twice")
	}
	multi, _ := s.MintSCEPChallenge(ctx, "test", time.Hour, false)
	if !s.ConsumeSCEPChallenge(ctx, multi) || !s.ConsumeSCEPChallenge(ctx, multi) {
		t.Fatal("multi-use challenge should work repeatedly")
	}
	expired, _ := s.MintSCEPChallenge(ctx, "test", -time.Minute, false)
	if s.ConsumeSCEPChallenge(ctx, expired) || s.ConsumeSCEPChallenge(ctx, "bogus") || s.ConsumeSCEPChallenge(ctx, "") {
		t.Fatal("expired/unknown challenge accepted")
	}
}

func TestEnrollAgentRefusesTakeover(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	udid := "TEST-" + time.Now().Format("150405.000000")
	req := model.AgentEnrollRequest{UDID: udid, SerialNumber: "S" + udid, ComputerName: "victim"}
	first, err := s.EnrollAgent(ctx, req, HashToken("tok-1"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteComputer(ctx, first.ID) })
	if _, err := s.EnrollAgent(ctx, req, HashToken("tok-2")); !errors.Is(err, ErrAgentActive) {
		t.Fatalf("takeover without proof: err = %v, want ErrAgentActive", err)
	}
	// A serial number alone never reaches a record that already has a UDID: it gets its own record.
	bySerial := model.AgentEnrollRequest{SerialNumber: req.SerialNumber}
	other, err := s.EnrollAgent(ctx, bySerial, HashToken("tok-2"))
	if err != nil || other.ID == first.ID {
		t.Fatalf("enroll by serial attached to the existing record: %+v %v", other, err)
	}
	t.Cleanup(func() { s.DeleteComputer(ctx, other.ID) })
	if _, err := s.UpsertComputerFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: udid + "-new", SerialNumber: req.SerialNumber}); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.GetComputer(ctx, other.ID); c.UDID != "" {
		t.Fatal("MDM enrollment merged into an agent-only record matched by serial")
	}
	req.PreviousDeviceToken = "tok-1"
	if _, err := s.EnrollAgent(ctx, req, HashToken("tok-3")); err != nil {
		t.Fatalf("reinstall with proof refused: %v", err)
	}
	if err := s.UnenrollAgent(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	req.PreviousDeviceToken = ""
	if _, err := s.EnrollAgent(ctx, req, HashToken("tok-4")); err != nil {
		t.Fatalf("enroll after admin reset refused: %v", err)
	}
}
