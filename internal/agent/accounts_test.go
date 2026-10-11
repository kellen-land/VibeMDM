package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/vibemdm/vibemdm/internal/localaccount"
	"github.com/vibemdm/vibemdm/internal/model"
)

type fakeUser struct {
	pw    string
	uid   int
	token bool
	admin bool
	home  string
}

// accountsMac models the account database behind sysadminctl, dscl and fdesetup. Like the real
// sysadminctl it exits 0 when it fails.
type accountsMac struct {
	users   map[string]*fakeUser
	fv      []string // FileVault users
	fvOn    bool
	console string
	calls   []string
}

func (m *accountsMac) run(_ context.Context, name string, args ...string) (string, error) {
	tool := name[strings.LastIndex(name, "/")+1:]
	m.calls = append(m.calls, tool+" "+strings.Join(args, " "))
	arg := func(flag string) string {
		if i := slices.Index(args, flag); i >= 0 && i+1 < len(args) {
			return args[i+1]
		}
		return ""
	}
	switch {
	case tool == "dscl" && args[1] == "-read":
		if _, ok := m.users[strings.TrimPrefix(args[2], "/Users/")]; ok {
			return "UniqueID: 501", nil
		}
		return "", errors.New("no such user")
	case tool == "dscl" && args[1] == "-list":
		var b strings.Builder
		for n, u := range m.users {
			fmt.Fprintf(&b, "%s %d\n", n, u.uid)
		}
		b.WriteString("_mbsetupuser 248\n")
		return b.String(), nil
	case tool == "dscl" && args[1] == "-authonly":
		if u, ok := m.users[args[2]]; ok && u.pw == args[3] {
			return "", nil
		}
		return "auth failed", errors.New("exit 1")
	case tool == "dscl" && args[1] == "-passwd":
		m.users[strings.TrimPrefix(args[2], "/Users/")].pw = args[3]
		return "", nil
	case tool == "sysadminctl" && args[0] == "-secureTokenStatus":
		if u, ok := m.users[args[1]]; ok && u.token {
			return "Secure token is ENABLED for user " + args[1], nil
		}
		return "Secure token is DISABLED for user " + args[1], nil
	case tool == "sysadminctl" && args[0] == "-addUser":
		m.users[args[1]] = &fakeUser{pw: arg("-password"), uid: 502, admin: slices.Contains(args, "-admin"), home: arg("-home")}
		return "", nil
	case tool == "sysadminctl" && args[0] == "-secureTokenOn":
		if a, ok := m.users[arg("-adminUser")]; ok && a.token && a.pw == arg("-adminPassword") && m.users[args[1]].pw == arg("-password") {
			m.users[args[1]].token = true
		}
		return "", nil
	case tool == "sysadminctl" && args[0] == "-resetPasswordFor":
		if a, ok := m.users[arg("-adminUser")]; ok && a.token && a.pw == arg("-adminPassword") {
			m.users[args[1]].pw = arg("-newPassword")
			return "", nil
		}
		return "Operation is not permitted without secure token unlock. admin pw " + arg("-adminPassword"), nil
	case tool == "sysadminctl" && args[0] == "-deleteUser":
		delete(m.users, args[1])
		return "", nil
	case tool == "createhomedir":
		return "", nil
	case tool == "fdesetup" && args[0] == "status":
		if m.fvOn {
			return "FileVault is On.", nil
		}
		return "FileVault is Off.", nil
	case tool == "fdesetup" && args[0] == "list":
		var b strings.Builder
		for _, u := range m.fv {
			b.WriteString(u + ",11111111-2222-3333-4444-555555555555\n")
		}
		return b.String(), nil
	case tool == "fdesetup" && args[0] == "remove":
		m.fv = slices.DeleteFunc(m.fv, func(u string) bool { return u == arg("-user") })
		return "", nil
	}
	return "", errors.New("unexpected command " + tool)
}

func newOps(m *accountsMac, pws []string, laps *model.AgentLAPSConfig) (*accountOps, *int) {
	fetches := 0
	return &accountOps{
		run: m.run,
		secrets: func(context.Context) ([]string, error) {
			fetches++
			return pws, nil
		},
		lapsCfg: func(context.Context) (*model.AgentLAPSConfig, error) {
			if laps == nil {
				return &model.AgentLAPSConfig{}, nil
			}
			return laps, nil
		},
		console: func(context.Context) string { return m.console },
	}, &fetches
}

func TestAccountsCreate(t *testing.T) {
	m := &accountsMac{users: map[string]*fakeUser{"initadmin": {pw: "admin-pw", uid: 501, token: true, admin: true}}}
	ops, fetches := newOps(m, []string{"Student-Pw-1", "Teacher-Pw-2"}, nil)
	r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.Create, Username: "student", FullName: "Student", PasswordHint: "school"})
	if r.ExitCode != 0 || m.users["student"] == nil || m.users["student"].pw != "Student-Pw-1" || m.users["student"].admin || m.users["student"].home != "/Users/student" {
		t.Fatalf("create: %+v %+v", r, m.users["student"])
	}
	r = ops.apply(context.Background(), 1, model.PolicyAccount{Action: localaccount.Create, Username: "teacher", Admin: true, HomeDirectory: "/Users/t", EnableFileVault: true})
	if r.ExitCode != 0 || !m.users["teacher"].admin || m.users["teacher"].home != "/Users/t" || m.users["teacher"].token {
		t.Fatalf("admin create: %+v %+v", r, m.users["teacher"])
	}
	if !strings.Contains(r.Combined, "bootstrap token") {
		t.Fatalf("without a Secure Token admin the FileVault note is missing: %q", r.Combined)
	}
	if *fetches != 1 {
		t.Fatalf("passwords must be fetched once per run, got %d", *fetches)
	}
	// An existing account is left alone (policies may run repeatedly).
	m.users["student"].pw = "changed-by-user"
	r = ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.Create, Username: "student"})
	if r.ExitCode != 0 || m.users["student"].pw != "changed-by-user" || !strings.Contains(r.Combined, "already exists") {
		t.Fatalf("existing: %+v", r)
	}
	for _, c := range m.calls {
		if strings.Contains(r.Combined, "Student-Pw-1") || strings.Contains(r.Combined, "Teacher-Pw-2") {
			t.Fatalf("output leaks a password: %q (%s)", r.Combined, c)
		}
	}
}

func TestAccountsCreateWithFileVaultUsesTheLAPSAdmin(t *testing.T) {
	m := &accountsMac{users: map[string]*fakeUser{"vibeadmin": {pw: "laps-pw", uid: 501, token: true, admin: true}}}
	ops, _ := newOps(m, []string{"Teacher-Pw-2"}, &model.AgentLAPSConfig{Enabled: true, Account: "vibeadmin", CurrentPassword: "laps-pw"})
	r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.Create, Username: "teacher", EnableFileVault: true})
	if r.ExitCode != 0 || !m.users["teacher"].token || strings.Contains(r.Combined, "laps-pw") {
		t.Fatalf("FileVault grant: %+v", r)
	}
	// The LAPS account itself is off limits.
	r = ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.Delete, Username: "vibeadmin"})
	if r.ExitCode == 0 || m.users["vibeadmin"] == nil {
		t.Fatal("a policy deleted the LAPS account")
	}
}

func TestAccountsResetPassword(t *testing.T) {
	m := &accountsMac{users: map[string]*fakeUser{
		"student":   {pw: "old", uid: 502},
		"teacher":   {pw: "old", uid: 503, token: true},
		"vibeadmin": {pw: "laps-pw", uid: 501, token: true},
	}}
	ops, _ := newOps(m, []string{"New-Pw-111", "New-Pw-222"}, nil)
	if r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.ResetPassword, Username: "student"}); r.ExitCode != 0 || m.users["student"].pw != "New-Pw-111" {
		t.Fatalf("no token: %+v", r)
	}
	r := ops.apply(context.Background(), 1, model.PolicyAccount{Action: localaccount.ResetPassword, Username: "teacher"})
	if r.ExitCode == 0 || m.users["teacher"].pw != "old" || !strings.Contains(r.Combined, "Secure Token administrator") {
		t.Fatalf("token account without an admin must be refused, not reset with dscl: %+v", r)
	}
	ops, _ = newOps(m, []string{"New-Pw-111", "New-Pw-222"}, &model.AgentLAPSConfig{Enabled: true, Account: "vibeadmin", CurrentPassword: "laps-pw"})
	if r := ops.apply(context.Background(), 1, model.PolicyAccount{Action: localaccount.ResetPassword, Username: "teacher"}); r.ExitCode != 0 || m.users["teacher"].pw != "New-Pw-222" {
		t.Fatalf("token account with the LAPS admin: %+v", r)
	}
	if r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.ResetPassword, Username: "nobody2"}); r.ExitCode == 0 {
		t.Fatal("missing account must fail")
	}
}

func TestAccountsDeleteSafety(t *testing.T) {
	m := &accountsMac{console: "student", users: map[string]*fakeUser{
		"initadmin": {pw: "x", uid: 501, token: true},
		"student":   {pw: "x", uid: 502},
		"olduser":   {pw: "x", uid: 503},
	}}
	ops, _ := newOps(m, nil, nil)
	if r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.Delete, Username: "student"}); r.ExitCode == 0 || m.users["student"] == nil {
		t.Fatal("deleted the logged-in user")
	}
	if r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.Delete, Username: "initadmin"}); r.ExitCode == 0 || m.users["initadmin"] == nil {
		t.Fatal("deleted the only Secure Token holder")
	}
	r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.Delete, Username: "olduser"})
	if r.ExitCode != 0 || m.users["olduser"] != nil || !slices.Contains(m.calls, "sysadminctl -deleteUser olduser -keepHome") {
		t.Fatalf("delete keeping home: %+v %v", r, m.calls)
	}
	if r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.Delete, Username: "olduser"}); r.ExitCode != 0 {
		t.Fatalf("deleting a missing account is a no-op: %+v", r)
	}
}

func TestAccountsDisableFileVault(t *testing.T) {
	m := &accountsMac{fvOn: true, fv: []string{"initadmin"}, users: map[string]*fakeUser{"initadmin": {uid: 501, token: true}, "student": {uid: 502, token: true}}}
	ops, _ := newOps(m, nil, nil)
	if r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.DisableFileVault, Username: "initadmin"}); r.ExitCode == 0 {
		t.Fatal("removed the last FileVault user")
	}
	m.fv = append(m.fv, "student")
	if r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.DisableFileVault, Username: "student"}); r.ExitCode != 0 || slices.Contains(m.fv, "student") {
		t.Fatalf("disable: %+v %v", r, m.fv)
	}
	m.fvOn = false
	if r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.DisableFileVault, Username: "initadmin"}); r.ExitCode != 0 || !strings.Contains(r.Combined, "FileVault is off") {
		t.Fatalf("FileVault off: %+v", r)
	}
}

func TestAccountsPasswordFetchFailure(t *testing.T) {
	m := &accountsMac{users: map[string]*fakeUser{}}
	ops, _ := newOps(m, nil, nil)
	ops.secrets = func(context.Context) ([]string, error) { return nil, errors.New("404 no open run") }
	if r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.Create, Username: "student"}); r.ExitCode == 0 || m.users["student"] != nil {
		t.Fatalf("created an account without a password: %+v", r)
	}
}

func TestAccountsExplainPasswordPolicyRejection(t *testing.T) {
	m := &accountsMac{users: map[string]*fakeUser{}}
	ops, _ := newOps(m, []string{"Short-1"}, nil)
	ops.run = func(ctx context.Context, name string, args ...string) (string, error) {
		if strings.HasSuffix(name, "sysadminctl") && args[0] == "-addUser" {
			return "sysadminctl[4664:48486] New account password error.  (5402)", nil // exits 0, creates nothing
		}
		return m.run(ctx, name, args...)
	}
	r := ops.apply(context.Background(), 0, model.PolicyAccount{Action: localaccount.Create, Username: "testuser"})
	if r.ExitCode == 0 || !strings.Contains(r.Combined, "password policy") || strings.Contains(r.Combined, "Short-1") {
		t.Fatalf("%+v", r)
	}
}
