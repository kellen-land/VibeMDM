package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vibemdm/vibemdm/internal/laps"
	"github.com/vibemdm/vibemdm/internal/model"
)

type fakeLAPSAPI struct {
	cfg        model.AgentLAPSConfig
	staged     map[string]string // fingerprint -> password
	commits    []bool
	stageErr   error
	commitErr  error
	forget     bool // commit returns 409 as if the server lost the stage
	lastErrMsg string
	lastCommit model.AgentLAPSCommit
}

func (f *fakeLAPSAPI) LAPSConfig(context.Context) (*model.AgentLAPSConfig, error) {
	c := f.cfg
	return &c, nil
}
func (f *fakeLAPSAPI) LAPSStage(_ context.Context, _, pw string) error {
	if f.stageErr != nil {
		return f.stageErr
	}
	if f.staged == nil {
		f.staged = map[string]string{}
	}
	f.staged[fp(pw)] = pw
	f.forget = false
	return nil
}
func (f *fakeLAPSAPI) LAPSCommit(_ context.Context, c model.AgentLAPSCommit) error {
	fingerprint, applied, msg := c.Fingerprint, c.Applied, c.Error
	f.lastCommit = c
	if f.commitErr != nil {
		return f.commitErr
	}
	if _, ok := f.staged[fingerprint]; !ok || f.forget {
		return &apiError{Status: 409, Msg: "no matching staged password"}
	}
	f.commits = append(f.commits, applied)
	f.lastErrMsg = msg
	delete(f.staged, fingerprint)
	return nil
}
func (f *fakeLAPSAPI) LAPSError(_ context.Context, msg string) error { f.lastErrMsg = msg; return nil }

// fakeMac models the local account database behind dscl/sysadminctl.
type fakeMac struct {
	users       map[string]string
	failSet     bool
	setThenFail bool // change takes effect but the tool exits non-zero
	calls       []string
	hidden      []string
}

func (m *fakeMac) run(_ context.Context, name string, args ...string) (string, error) {
	m.calls = append(m.calls, filepath.Base(name))
	switch {
	case strings.HasSuffix(name, "dscl") && args[1] == "-read" && len(args) > 3 && args[3] == "GeneratedUID":
		if _, ok := m.users[strings.TrimPrefix(args[2], "/Users/")]; ok {
			return "GeneratedUID: 11111111-2222-3333-4444-555555555555\n", nil
		}
		return "", errors.New("no such user")
	case strings.HasSuffix(name, "dscl") && args[1] == "-create" && args[3] == "IsHidden":
		m.hidden = append(m.hidden, strings.TrimPrefix(args[2], "/Users/"))
		return "", nil
	case strings.HasSuffix(name, "dscl") && args[1] == "-read":
		if _, ok := m.users[strings.TrimPrefix(args[2], "/Users/")]; ok {
			return "", nil
		}
		return "", errors.New("no such user")
	case strings.HasSuffix(name, "dscl") && args[1] == "-authonly":
		if pw, ok := m.users[args[2]]; ok && pw == args[3] {
			return "", nil
		}
		return "", errors.New("auth failed")
	case strings.HasSuffix(name, "sysadminctl") && args[0] == "-addUser":
		m.users[args[1]] = args[args2(args, "-password")]
		return "", nil
	case strings.HasSuffix(name, "sysadminctl") && args[0] == "-resetPasswordFor":
		if m.failSet {
			return "failed", errors.New("exit 1")
		}
		m.users[args[1]] = args[args2(args, "-newPassword")]
		if m.setThenFail {
			return "", errors.New("exit 1")
		}
		return "", nil
	case strings.HasSuffix(name, "dscl") && args[1] == "-passwd":
		if m.failSet {
			return "failed", errors.New("exit 1")
		}
		m.users[strings.TrimPrefix(args[2], "/Users/")] = args[3]
		return "", nil
	}
	return "", errors.New("unexpected command")
}

func args2(args []string, flag string) int {
	for i, a := range args {
		if a == flag {
			return i + 1
		}
	}
	return 0
}

func fp(pw string) string { return laps.Fingerprint(pw) }

type nopLog struct{}

func (nopLog) Info(string, ...any) {}
func (nopLog) Warn(string, ...any) {}

func newLAPS(t *testing.T, api *fakeLAPSAPI, mac *fakeMac) *LAPS {
	return &LAPS{API: api, Run: mac.run, PendingPath: filepath.Join(t.TempDir(), "pending.json"), Log: nopLog{}}
}

func dueCfg() model.AgentLAPSConfig {
	return model.AgentLAPSConfig{Enabled: true, Account: "vibeadmin", PasswordLength: 20, Rotate: true, Reason: "expired", CurrentPassword: "old-Password-1"}
}

func TestLAPSRotateHappyPath(t *testing.T) {
	api, mac := &fakeLAPSAPI{cfg: dueCfg()}, &fakeMac{users: map[string]string{"vibeadmin": "old-Password-1"}}
	l := newLAPS(t, api, mac)
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(api.commits) != 1 || !api.commits[0] {
		t.Fatalf("commits: %v", api.commits)
	}
	if mac.users["vibeadmin"] == "old-Password-1" || len(mac.users["vibeadmin"]) != 20 {
		t.Fatal("password not changed")
	}
	if _, err := os.Stat(l.PendingPath); err == nil {
		t.Fatal("pending file left behind")
	}
}

func TestLAPSStageFailureLeavesAccountUntouched(t *testing.T) {
	api, mac := &fakeLAPSAPI{cfg: dueCfg(), stageErr: errors.New("network down")}, &fakeMac{users: map[string]string{"vibeadmin": "old-Password-1"}}
	l := newLAPS(t, api, mac)
	if err := l.Cycle(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if mac.users["vibeadmin"] != "old-Password-1" {
		t.Fatal("the account password changed although the server never received the new one")
	}
	if _, err := os.Stat(l.PendingPath); err == nil {
		t.Fatal("pending file should be removed when nothing was changed")
	}
}

func TestLAPSApplyFailureIsReportedAndDiscarded(t *testing.T) {
	api, mac := &fakeLAPSAPI{cfg: dueCfg()}, &fakeMac{users: map[string]string{"vibeadmin": "old-Password-1"}, failSet: true}
	l := newLAPS(t, api, mac)
	if err := l.Cycle(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if len(api.commits) != 1 || api.commits[0] {
		t.Fatalf("expected one failed commit, got %v", api.commits)
	}
	if mac.users["vibeadmin"] != "old-Password-1" {
		t.Fatal("password should be unchanged")
	}
}

func TestLAPSToolErrorAfterChangeStillCountsAsApplied(t *testing.T) {
	api, mac := &fakeLAPSAPI{cfg: dueCfg()}, &fakeMac{users: map[string]string{"vibeadmin": "old-Password-1"}, setThenFail: true}
	l := newLAPS(t, api, mac)
	l.Cycle(context.Background())
	if len(api.commits) != 1 || !api.commits[0] {
		t.Fatalf("a change that took effect must be committed as applied: %v", api.commits)
	}
}

func TestLAPSInterruptedRotationIsSettledNextCycle(t *testing.T) {
	// The Mac changed, but the commit upload failed. The pending file survives.
	api, mac := &fakeLAPSAPI{cfg: dueCfg(), commitErr: errors.New("connection reset")}, &fakeMac{users: map[string]string{"vibeadmin": "old-Password-1"}}
	l := newLAPS(t, api, mac)
	if err := l.Cycle(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(l.PendingPath); err != nil {
		t.Fatal("pending file must survive a failed commit")
	}
	api.commitErr, api.cfg.Rotate = nil, false
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(api.commits) != 1 || !api.commits[0] {
		t.Fatalf("next cycle should commit applied: %v", api.commits)
	}
	if _, err := os.Stat(l.PendingPath); err == nil {
		t.Fatal("pending file should be cleared")
	}
}

func TestLAPSServerLostStageIsRestaged(t *testing.T) {
	api, mac := &fakeLAPSAPI{cfg: dueCfg()}, &fakeMac{users: map[string]string{"vibeadmin": "Pending-Pw-0123456789"}}
	l := newLAPS(t, api, mac)
	l.writePending(lapsPending{Account: "vibeadmin", Password: "Pending-Pw-0123456789", Fingerprint: fp("Pending-Pw-0123456789")})
	api.cfg.Rotate = false // nothing staged on the server, but the account has the pending password
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(api.commits) != 1 || !api.commits[0] {
		t.Fatalf("expected a re-stage and an applied commit: %v", api.commits)
	}
}

func TestLAPSCreatesMissingAccountOnlyWhenAllowed(t *testing.T) {
	cfg := dueCfg()
	cfg.CurrentPassword = ""
	api, mac := &fakeLAPSAPI{cfg: cfg}, &fakeMac{users: map[string]string{}}
	if err := newLAPS(t, api, mac).Cycle(context.Background()); err == nil || api.lastErrMsg == "" {
		t.Fatal("must refuse to create the account when the policy forbids it")
	}
	api.cfg.CreateAccount = true
	if err := newLAPS(t, api, mac).Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(mac.users["vibeadmin"]) != 20 {
		t.Fatal("account not created with a generated password")
	}
}

func TestLAPSJamfFormatSetPasswordHideAndGUID(t *testing.T) {
	cfg := dueCfg()
	cfg.PasswordFormat, cfg.PasswordLength, cfg.HideAccount = "jamf", 30, true
	api, mac := &fakeLAPSAPI{cfg: cfg}, &fakeMac{users: map[string]string{"vibeadmin": "old-Password-1"}}
	if err := newLAPS(t, api, mac).Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	pw := mac.users["vibeadmin"]
	if len(pw) != 30 || strings.Count(pw, "-") != 4 || strings.ToUpper(pw) != pw {
		t.Fatalf("not a Jamf-style password: %q", pw)
	}
	if api.lastCommit.Guid == "" || len(mac.hidden) != 1 {
		t.Fatalf("guid %q hidden %v", api.lastCommit.Guid, mac.hidden)
	}
	// An administrator-chosen password is applied verbatim.
	api.cfg.NewPassword, api.cfg.CurrentPassword = "Chosen-by-admin-42", pw
	if err := newLAPS(t, api, mac).Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if mac.users["vibeadmin"] != "Chosen-by-admin-42" {
		t.Fatal("set-password not applied")
	}
}
