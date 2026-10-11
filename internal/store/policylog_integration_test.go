package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/secrets"
)

func TestPolicyLogAcceptsAnyToolOutput(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	stamp := time.Now().Format("150405.000")
	c, err := s.UpsertComputerFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: "PLOG-" + stamp, SerialNumber: "PLOG" + stamp, DeviceName: "plog", Model: "Mac14,2", ProductName: "Mac14,2"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteComputer(ctx, c.ID) })
	p, err := s.CreatePolicy(ctx, model.Policy{Name: "plog " + stamp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeletePolicy(ctx, p.ID) })
	id, err := s.CreatePendingPolicyLog(ctx, p.ID, c.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	bad := "ok\x00nul and \xff\xfe invalid UTF-8"
	now := time.Now()
	sub := model.AgentPolicyLogSubmit{Status: "completed", StartedAt: now, CompletedAt: now, Log: bad,
		Steps: []model.PolicyStep{{Type: "app", Name: "visualstudiocode", Status: "completed", Output: bad}}}
	if err := s.CompletePolicyLog(ctx, id, c.ID, sub); err != nil {
		t.Fatalf("policy log with NUL/invalid UTF-8 rejected: %v", err)
	}
	l, err := s.GetPolicyLog(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(l.Log, "nul and") || strings.ContainsRune(l.Log, 0) || !strings.Contains(l.Steps[0].Output, "invalid UTF-8") {
		t.Fatalf("stored log: %q / %q", l.Log, l.Steps[0].Output)
	}
}

func TestPolicyAccountPasswordsAreSealedAndKept(t *testing.T) {
	s := testStore(t)
	box, _ := secrets.New("test passphrase for the store tests")
	s.Box = box
	ctx := context.Background()
	stamp := time.Now().Format("150405.000")
	c, _ := s.UpsertComputerFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: "ACCT-" + stamp, SerialNumber: "ACCT" + stamp, DeviceName: "acct", Model: "Mac14,2", ProductName: "Mac14,2"})
	t.Cleanup(func() { s.DeleteComputer(ctx, c.ID) })
	p, err := s.CreatePolicy(ctx, model.Policy{Name: "accounts " + stamp, Accounts: []model.PolicyAccount{
		{Action: "create", Username: "student", Password: "Student-Pw-1"},
		{Action: "delete", Username: "olduser"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeletePolicy(ctx, p.ID) })
	if p.Accounts[0].Password != "" || !p.Accounts[0].PasswordSet || p.Accounts[1].PasswordSet {
		t.Fatalf("returned accounts: %+v", p.Accounts)
	}
	var raw string
	s.DB.QueryRowContext(ctx, `SELECT accounts::text FROM policies WHERE id = $1`, p.ID).Scan(&raw)
	if strings.Contains(raw, "Student-Pw-1") || !strings.Contains(raw, "enc:v1:") {
		t.Fatalf("password not sealed at rest: %s", raw)
	}
	// Editing without resending the password keeps it; a new entry needs its own.
	p.Accounts = []model.PolicyAccount{{Action: "create", Username: "student", FullName: "Student"}, {Action: "reset_password", Username: "teacher", Password: "Teacher-Pw-2"}}
	if p, err = s.UpdatePolicy(ctx, *p); err != nil {
		t.Fatal(err)
	}
	pws, err := s.PolicyAccountPasswords(ctx, p.ID)
	if err != nil || len(pws) != 2 || pws[0] != "Student-Pw-1" || pws[1] != "Teacher-Pw-2" {
		t.Fatalf("passwords: %v %v", pws, err)
	}
	// Released only for an open run on that computer.
	logID, _ := s.CreatePendingPolicyLog(ctx, p.ID, c.ID, "manual")
	if pid, err := s.PendingPolicyLog(ctx, logID, c.ID); err != nil || pid != p.ID {
		t.Fatalf("open run: %v %v", pid, err)
	}
	if _, err := s.PendingPolicyLog(ctx, logID, "00000000-0000-0000-0000-000000000000"); err != ErrNotFound {
		t.Fatalf("another computer: %v", err)
	}
	s.CompletePolicyLog(ctx, logID, c.ID, model.AgentPolicyLogSubmit{Status: "completed"})
	if _, err := s.PendingPolicyLog(ctx, logID, c.ID); err != ErrNotFound {
		t.Fatalf("finished run: %v", err)
	}
}

func TestComputerProfilesShowTheNewestReportOnce(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	stamp := time.Now().Format("150405.000")
	c, _ := s.UpsertComputerFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: "PROF-" + stamp, SerialNumber: "PROFC" + stamp, DeviceName: "prof", Model: "Mac14,2", ProductName: "Mac14,2"})
	t.Cleanup(func() { s.DeleteComputer(ctx, c.ID) })
	// MDM's older ProfileList still has the CIS profile; the agent's newer report does not.
	s.ReplaceComputerProfiles(ctx, c.ID, "mdm", []model.ComputerProfile{{Identifier: "com.vibemdm.enrollment", IsManaged: true}, {Identifier: "com.vibemdm.compliance.cis", IsManaged: true}})
	time.Sleep(5 * time.Millisecond)
	s.ReplaceComputerProfiles(ctx, c.ID, "agent", []model.ComputerProfile{{Identifier: "com.vibemdm.enrollment"}})
	list, err := s.ListComputerProfiles(ctx, c.ID)
	if err != nil || len(list) != 1 || list[0].Identifier != "com.vibemdm.enrollment" || !list[0].IsManaged {
		t.Fatalf("want the enrollment profile once, managed: %+v %v", list, err)
	}
	ids, err := s.PreviewCriteria(ctx, []model.Criterion{{Field: "profile_identifier", Operator: "is", Value: "com.vibemdm.compliance.cis"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if id == c.ID {
			t.Fatal("a smart group still matches a removed profile")
		}
	}
	// A newer MDM report wins again.
	time.Sleep(5 * time.Millisecond)
	s.ReplaceComputerProfiles(ctx, c.ID, "mdm", []model.ComputerProfile{{Identifier: "com.vibemdm.enrollment", IsManaged: true}, {Identifier: "com.example.new", IsManaged: true}})
	if list, _ := s.ListComputerProfiles(ctx, c.ID); len(list) != 2 {
		t.Fatalf("newer MDM report: %+v", list)
	}
}

func TestLAPSAdminCredentialsDoNotCountAsViewing(t *testing.T) {
	s := testStore(t)
	box, _ := secrets.New("test passphrase for the store tests")
	s.Box = box
	ctx := context.Background()
	stamp := time.Now().Format("150405.000")
	c, _ := s.UpsertComputerFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: "LADM-" + stamp, SerialNumber: "LADM" + stamp, DeviceName: "ladm", Model: "Mac14,2", ProductName: "Mac14,2"})
	t.Cleanup(func() { s.DeleteComputer(ctx, c.ID) })
	if _, _, err := s.LAPSAdminCredentials(ctx, c.ID); err != ErrNotFound {
		t.Fatalf("no LAPS row: %v", err)
	}
	sealed, _ := box.Seal("Laps-Current-1")
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO laps_passwords (computer_id, account, password_encrypted, fingerprint, escrowed_at) VALUES ($1, 'vibeadmin', $2, 'x', now())`, c.ID, sealed); err != nil {
		t.Fatal(err)
	}
	user, pw, err := s.LAPSAdminCredentials(ctx, c.ID)
	if err != nil || user != "vibeadmin" || pw != "Laps-Current-1" {
		t.Fatalf("%q %q %v", user, pw, err)
	}
	var viewed sql.NullTime
	s.DB.QueryRowContext(ctx, `SELECT last_viewed_at FROM laps_passwords WHERE computer_id = $1`, c.ID).Scan(&viewed)
	if viewed.Valid {
		t.Fatal("agent use must not count as a human viewing the password")
	}
}
