package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vibemdm/vibemdm/internal/laps"
	"github.com/vibemdm/vibemdm/internal/model"
)

func commit(fp string, applied bool, msg string) model.AgentLAPSCommit {
	return model.AgentLAPSCommit{Fingerprint: fp, Applied: applied, Error: msg, Guid: "GUID-1"}
}

func TestLAPSStageCommitLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	stamp := time.Now().Format("150405.000")
	comp, err := s.EnrollAgent(ctx, model.AgentEnrollRequest{SerialNumber: "LAPS" + stamp, UDID: "u" + stamp,
		Hostname: "laps.local", ComputerName: "laps-test", OSVersion: "15.0", Architecture: "arm64"}, HashToken("laps-test-token-"+stamp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteComputer(ctx, comp.ID) })

	cfg, err := s.SaveLAPSConfig(ctx, model.LAPSConfig{Enabled: true, Scope: model.Scope{AllComputers: true}, RotateAfterUseMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccountName == "" || cfg.PasswordFormat != laps.FormatJamf || cfg.PasswordLength != 30 || cfg.AutoRotate || cfg.RotationDays != 90 {
		t.Fatalf("Jamf-like defaults not applied: %+v", cfg)
	}

	// Nothing escrowed yet: the agent is told to rotate because the password is missing.
	ac, err := s.LAPSConfigForAgent(ctx, comp)
	if err != nil || !ac.Enabled || !ac.Rotate || ac.Reason != "missing" || ac.CurrentPassword != "" || ac.PasswordFormat != "jamf" {
		t.Fatalf("initial agent config: %+v %v", ac, err)
	}

	// Stage, then a failed apply discards it and records the error; no current password exists.
	fp, err := s.LAPSStage(ctx, comp.ID, cfg.AccountName, "Aa1-first-password-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.LAPSCommit(ctx, comp.ID, commit("wrong", true, "")); !errors.Is(err, ErrLAPSNoPending) {
		t.Fatalf("mismatched fingerprint must not resolve the stage: %v", err)
	}
	if err := s.LAPSCommit(ctx, comp.ID, commit(fp, false, "dscl failed")); err != nil {
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
	if err := s.LAPSCommit(ctx, comp.ID, commit(fp, true, "")); err != nil {
		t.Fatal(err)
	}
	if err := s.LAPSCommit(ctx, comp.ID, commit(fp, true, "")); !errors.Is(err, ErrLAPSNoPending) {
		t.Fatalf("a duplicate commit must be rejected, got %v", err)
	}
	st, _ = s.LAPSStatusFor(ctx, comp)
	if !st.HasPassword || st.Pending || st.RotationStatus != "ok" || st.RotatedCount != 1 || st.LastError != "" || st.Guid != "GUID-1" {
		t.Fatalf("after commit: %+v", st)
	}

	// Viewing returns the password, records the view and always schedules a rotation (Jamf behaviour).
	pw, st, err := s.RevealLAPS(ctx, comp.ID, "alice", "HELP-1")
	if err != nil || pw.Current != "Aa1-second-password-xyz" || pw.Pending != "" {
		t.Fatalf("reveal: %+v %v", pw, err)
	}
	if st.LastViewedBy != "alice" || st.RotateAfter == nil {
		t.Fatalf("view not recorded: %+v", st)
	}
	if ac, _ := s.LAPSConfigForAgent(ctx, comp); ac.Rotate {
		t.Fatal("rotation must wait for the rotation-after-viewing delay")
	}
	if p, _ := s.LAPSPendingRotations(ctx); len(p) != 1 || p[0].Reason != "viewed" || p[0].DueAt == nil {
		t.Fatalf("pending rotations: %+v", p)
	}
	s.DB.ExecContext(ctx, `UPDATE laps_passwords SET rotate_after = now() - interval '1 minute' WHERE computer_id = $1`, comp.ID)
	ac, _ = s.LAPSConfigForAgent(ctx, comp)
	if !ac.Rotate || ac.Reason != "used" || ac.CurrentPassword != "Aa1-second-password-xyz" {
		t.Fatalf("after use: %+v", ac)
	}
	fp3, _ := s.LAPSStage(ctx, comp.ID, cfg.AccountName, "Aa1-third-password-xyz")

	// An unconfirmed stage is revealed alongside the current password, since it may be the live one.
	pw, _, _ = s.RevealLAPS(ctx, comp.ID, "bob", "HELP-2")
	if pw.Current != "Aa1-second-password-xyz" || pw.Pending != "Aa1-third-password-xyz" {
		t.Fatalf("reveal with pending: %+v", pw)
	}
	if err := s.LAPSCommit(ctx, comp.ID, commit(fp3, true, "")); err != nil {
		t.Fatal(err)
	}

	// History keeps every password with who viewed it; the old one is retired.
	hist, err := s.LAPSHistory(ctx, comp.ID, true)
	if err != nil || len(hist) != 2 {
		t.Fatalf("history: %+v %v", hist, err)
	}
	if hist[0].Password != "Aa1-third-password-xyz" || hist[0].RetiredAt != nil || hist[1].RetiredAt == nil || len(hist[1].Views) != 2 {
		t.Fatalf("history entries: %+v", hist)
	}
	if h, _ := s.LAPSHistory(ctx, comp.ID, false); h[0].Password != "" {
		t.Fatal("passwords must only be decrypted for the audit view")
	}
	ev, _ := s.LAPSEvents(ctx, comp.ID, 0)
	seen := map[string]int{}
	for _, e := range ev {
		seen[e.EventType]++
	}
	if seen["ROTATED"] != 2 || seen["VIEWED"] != 2 || seen["ERROR"] != 1 || seen["PENDING"] < 2 {
		t.Fatalf("events: %v", seen)
	}

	// Expiry is opt-in, as in Jamf Pro ("Never" by default).
	s.DB.ExecContext(ctx, `UPDATE laps_passwords SET rotate_after = NULL, escrowed_at = now() - interval '100 days' WHERE computer_id = $1`, comp.ID)
	if ac, _ := s.LAPSConfigForAgent(ctx, comp); ac.Rotate {
		t.Fatalf("expired although auto-rotate is off: %+v", ac)
	}
	cfg.AutoRotate = true
	s.SaveLAPSConfig(ctx, cfg)
	if ac, _ := s.LAPSConfigForAgent(ctx, comp); !ac.Rotate || ac.Reason != "expired" {
		t.Fatalf("expiry: %+v", ac)
	}
	s.DB.ExecContext(ctx, `UPDATE laps_passwords SET escrowed_at = now() WHERE computer_id = $1`, comp.ID)

	// An administrator-chosen password is handed to the agent and cleared once applied.
	if err := s.LAPSSetPassword(ctx, comp.ID, cfg.AccountName, "short", "alice"); err == nil {
		t.Fatal("weak manual password accepted")
	}
	if err := s.LAPSSetPassword(ctx, comp.ID, cfg.AccountName, "Admin-Chosen-1234", "alice"); err != nil {
		t.Fatal(err)
	}
	ac, _ = s.LAPSConfigForAgent(ctx, comp)
	if !ac.Rotate || ac.Reason != "set" || ac.NewPassword != "Admin-Chosen-1234" {
		t.Fatalf("set-password: %+v", ac)
	}
	fp4, _ := s.LAPSStage(ctx, comp.ID, cfg.AccountName, "Admin-Chosen-1234")
	s.LAPSCommit(ctx, comp.ID, commit(fp4, true, ""))
	if ac, _ := s.LAPSConfigForAgent(ctx, comp); ac.Rotate || ac.NewPassword != "" {
		t.Fatalf("desired password not cleared: %+v", ac)
	}

	// Requested rotation, then out of scope: the agent is told nothing.
	s.LAPSRequestRotation(ctx, comp.ID, "alice")
	if ac, _ := s.LAPSConfigForAgent(ctx, comp); !ac.Rotate || ac.Reason != "requested" {
		t.Fatalf("requested: %+v", ac)
	}
	s.SaveLAPSConfig(ctx, model.LAPSConfig{Enabled: false})
	if ac, _ := s.LAPSConfigForAgent(ctx, comp); ac.Enabled || ac.Rotate {
		t.Fatalf("disabled: %+v", ac)
	}
}

func TestAgentBootstrapTokens(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	stamp := time.Now().Format("150405.000")
	comp, err := s.EnrollAgent(ctx, model.AgentEnrollRequest{SerialNumber: "BOOT" + stamp, UDID: "b" + stamp, Hostname: "b.local",
		ComputerName: "boot-test", OSVersion: "15.0", Architecture: "arm64"}, HashToken("boot-"+stamp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteComputer(ctx, comp.ID) })
	if err := s.CreateBootstrapToken(ctx, comp.ID, HashToken("vmbt_a"+stamp), time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeBootstrapToken(ctx, HashToken("vmbt_a"+stamp), "other-udid", "OTHER"); !errors.Is(err, ErrBootstrapDevice) {
		t.Fatalf("another Mac must not use the token: %v", err)
	}
	if id, err := s.ConsumeBootstrapToken(ctx, HashToken("vmbt_a"+stamp), "", "BOOT"+stamp); err != nil || id != comp.ID {
		t.Fatalf("own Mac: %q %v", id, err)
	}
	var reused *BootstrapReusedError
	if _, err := s.ConsumeBootstrapToken(ctx, HashToken("vmbt_a"+stamp), "", "BOOT"+stamp); !errors.As(err, &reused) || reused.ComputerID != comp.ID {
		t.Fatalf("token used twice: err = %v", err)
	}
	// Install attempts are never downgraded once the agent enrolled.
	s.RecordAgentInstall(ctx, comp.ID, "CMD-"+stamp, "test")
	s.MarkAgentInstalled(ctx, comp.ID)
	s.UpdateAgentInstall(ctx, "CMD-"+stamp, "sent", "late reply")
	if a, _ := s.LatestAgentInstall(ctx, comp.ID); a == nil || a.Status != "installed" {
		t.Fatalf("attempt: %+v", a)
	}
}

func TestLAPSSecureTokenGrant(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	stamp := time.Now().Format("150405.000")
	comp, err := s.EnrollAgent(ctx, model.AgentEnrollRequest{SerialNumber: "TOK" + stamp, UDID: "t" + stamp, Hostname: "t.local",
		ComputerName: "token-test", OSVersion: "26.0", Architecture: "arm64"}, HashToken("tok-"+stamp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteComputer(ctx, comp.ID) })
	cfg, _ := s.SaveLAPSConfig(ctx, model.LAPSConfig{Enabled: true, Scope: model.Scope{AllComputers: true}})
	fp, _ := s.LAPSStage(ctx, comp.ID, cfg.AccountName, "Laps-Password-12345")
	s.LAPSCommit(ctx, comp.ID, model.AgentLAPSCommit{Fingerprint: fp, Applied: true})

	if err := s.LAPSCreateTokenGrant(ctx, comp.ID, "alice", "Alice-Secret-pw", "admin", time.Minute); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.LAPSStatusFor(ctx, comp); !st.TokenGrantPending {
		t.Fatal("grant should be pending")
	}
	ac, _ := s.LAPSConfigForAgent(ctx, comp)
	if !ac.GrantSecureToken || ac.CurrentPassword != "Laps-Password-12345" {
		t.Fatalf("agent not told to grant: %+v", ac)
	}
	var stored string
	s.DB.QueryRowContext(ctx, `SELECT admin_password_encrypted FROM laps_token_grants WHERE computer_id = $1`, comp.ID).Scan(&stored)
	if s.Box.Enabled() && stored == "Alice-Secret-pw" {
		t.Fatal("admin password stored in plain text")
	}
	g, err := s.LAPSTakeTokenGrant(ctx, comp.ID)
	if err != nil || g.AdminUser != "alice" || g.AdminPassword != "Alice-Secret-pw" {
		t.Fatalf("take: %+v %v", g, err)
	}
	if _, err := s.LAPSTakeTokenGrant(ctx, comp.ID); err == nil {
		t.Fatal("credentials handed out twice")
	}
	yes := true
	if err := s.LAPSSetTokenState(ctx, comp.ID, model.AgentLAPSState{SecureToken: &yes, VolumeOwner: &yes, Granted: true}); err != nil {
		t.Fatal(err)
	}
	st, _ := s.LAPSStatusFor(ctx, comp)
	if st.SecureToken == nil || !*st.SecureToken || st.VolumeOwner == nil || !*st.VolumeOwner || st.TokenGrantPending {
		t.Fatalf("state: %+v", st)
	}
	// An expired grant is never handed out.
	s.LAPSCreateTokenGrant(ctx, comp.ID, "alice", "pw", "admin", -time.Minute)
	if _, err := s.LAPSTakeTokenGrant(ctx, comp.ID); err == nil {
		t.Fatal("expired grant handed out")
	}
}
