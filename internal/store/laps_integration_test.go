package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

func TestLAPSStageCommitLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	comp, err := s.EnrollAgent(ctx, model.AgentEnrollRequest{SerialNumber: "LAPS" + time.Now().Format("150405.000"), UDID: "u" + time.Now().Format("150405.000"),
		Hostname: "laps.local", ComputerName: "laps-test", OSVersion: "15.0", Architecture: "arm64"}, HashToken("laps-test-token-"+time.Now().String()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteComputer(ctx, comp.ID) })

	cfg, err := s.SaveLAPSConfig(ctx, model.LAPSConfig{Enabled: true, Scope: model.Scope{AllComputers: true}, RotateAfterUse: true, RotateAfterUseMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccountName == "" {
		t.Fatal("defaults not applied")
	}

	// Nothing escrowed yet: the agent is told to rotate because the password is missing.
	ac, err := s.LAPSConfigForAgent(ctx, comp)
	if err != nil || !ac.Enabled || !ac.Rotate || ac.Reason != "missing" || ac.CurrentPassword != "" {
		t.Fatalf("initial agent config: %+v %v", ac, err)
	}

	// Stage, then a failed apply discards it and records the error; no current password exists.
	fp, err := s.LAPSStage(ctx, comp.ID, cfg.AccountName, "Aa1-first-password-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.LAPSCommit(ctx, comp.ID, "…-wrong", true, ""); !errors.Is(err, ErrLAPSNoPending) {
		t.Fatalf("mismatched fingerprint must not resolve the stage: %v", err)
	}
	if err := s.LAPSCommit(ctx, comp.ID, fp, false, "dscl failed"); err != nil {
		t.Fatal(err)
	}
	st, _ := s.LAPSStatusFor(ctx, comp)
	if st.HasPassword || st.Pending || st.RotationStatus != "failing" || st.LastError == "" {
		t.Fatalf("after failed apply: %+v", st)
	}

	// Stage and commit successfully.
	fp, _ = s.LAPSStage(ctx, comp.ID, cfg.AccountName, "Aa1-second-password-xyz")
	if st, _ := s.LAPSStatusFor(ctx, comp); !st.Pending || st.RotationStatus != "pending" {
		t.Fatalf("staged state: %+v", st)
	}
	if err := s.LAPSCommit(ctx, comp.ID, fp, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.LAPSCommit(ctx, comp.ID, fp, true, ""); !errors.Is(err, ErrLAPSNoPending) {
		t.Fatalf("a duplicate commit must be rejected, got %v", err)
	}
	st, _ = s.LAPSStatusFor(ctx, comp)
	if !st.HasPassword || st.Pending || st.RotationStatus != "ok" || st.RotatedCount != 1 || st.LastError != "" {
		t.Fatalf("after commit: %+v", st)
	}

	// Revealing returns the password, records the view and schedules rotation after use.
	pw, st, err := s.RevealLAPS(ctx, comp.ID, "alice")
	if err != nil || pw.Current != "Aa1-second-password-xyz" || pw.Pending != "" {
		t.Fatalf("reveal: %+v %v", pw, err)
	}
	if st.LastViewedBy != "alice" || st.RotateAfter == nil {
		t.Fatalf("view not recorded: %+v", st)
	}
	if ac, _ := s.LAPSConfigForAgent(ctx, comp); ac.Rotate {
		t.Fatal("rotation must wait for the grace period")
	}
	s.DB.ExecContext(ctx, `UPDATE laps_passwords SET rotate_after = now() - interval '1 minute' WHERE computer_id = $1`, comp.ID)
	ac, _ = s.LAPSConfigForAgent(ctx, comp)
	if !ac.Rotate || ac.Reason != "used" || ac.CurrentPassword != "Aa1-second-password-xyz" {
		t.Fatalf("after use: %+v", ac)
	}

	// An unconfirmed stage is revealed alongside the current password, since it may be the live one.
	s.LAPSStage(ctx, comp.ID, cfg.AccountName, "Aa1-third-password-xyz")
	pw, _, _ = s.RevealLAPS(ctx, comp.ID, "alice")
	if pw.Current != "Aa1-second-password-xyz" || pw.Pending != "Aa1-third-password-xyz" {
		t.Fatalf("reveal with pending: %+v", pw)
	}

	// Expiry and an admin-requested rotation.
	s.DB.ExecContext(ctx, `UPDATE laps_passwords SET rotate_after = NULL, pending_encrypted = NULL, escrowed_at = now() - interval '40 days' WHERE computer_id = $1`, comp.ID)
	if ac, _ := s.LAPSConfigForAgent(ctx, comp); !ac.Rotate || ac.Reason != "expired" {
		t.Fatalf("expiry: %+v", ac)
	}
	s.DB.ExecContext(ctx, `UPDATE laps_passwords SET escrowed_at = now() WHERE computer_id = $1`, comp.ID)
	s.LAPSRequestRotation(ctx, comp.ID)
	if ac, _ := s.LAPSConfigForAgent(ctx, comp); !ac.Rotate || ac.Reason != "requested" {
		t.Fatalf("requested: %+v", ac)
	}

	// Out of scope: the agent is told nothing.
	s.SaveLAPSConfig(ctx, model.LAPSConfig{Enabled: false})
	if ac, _ := s.LAPSConfigForAgent(ctx, comp); ac.Enabled || ac.Rotate {
		t.Fatalf("disabled: %+v", ac)
	}
}
