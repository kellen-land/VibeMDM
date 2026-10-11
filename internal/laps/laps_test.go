package laps

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vibemdm/vibemdm/internal/model"
)

func TestGeneratedPasswordsAvoidSimpleRunsAndLeadingSymbols(t *testing.T) {
	for i := 0; i < 2000; i++ {
		pw, _ := Generate(20)
		if hasSimpleRun(pw) || strings.ContainsRune(symbols, rune(pw[0])) {
			t.Fatalf("generated %q", pw)
		}
		j, _ := GenerateFormat(FormatJamf, 30)
		if hasSimpleRun(j) {
			t.Fatalf("generated %q", j)
		}
	}
	for s, want := range map[string]bool{"abc": true, "CBA": true, "x777y": true, "234": true, "a1b2c3": false, "AB-CD": false} {
		if hasSimpleRun(s) != want {
			t.Errorf("hasSimpleRun(%q) = %v", s, !want)
		}
	}
}

func TestGenerate(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		p, err := Generate(20)
		if err != nil {
			t.Fatal(err)
		}
		if len(p) != 20 || seen[p] {
			t.Fatalf("bad or repeated password %q", p)
		}
		seen[p] = true
		for name, set := range map[string]string{"lower": lower, "upper": upper, "digit": digits, "symbol": symbols} {
			if !strings.ContainsAny(p, set) {
				t.Fatalf("%q lacks %s", p, name)
			}
		}
		if strings.ContainsAny(p, "0O1lI'\"\\ `$") {
			t.Fatalf("%q contains an ambiguous or shell-special character", p)
		}
	}
	if _, err := Generate(MinLength - 1); err == nil {
		t.Error("short length accepted")
	}
}

func TestNormalize(t *testing.T) {
	c := model.LAPSConfig{}
	if err := Normalize(&c); err != nil || c.AccountName != DefaultAccount || c.PasswordLength != DefaultLength || c.RotationDays != DefaultRotateDays {
		t.Fatalf("defaults: %+v %v", c, err)
	}
	for _, bad := range []string{"root", "Admin", "bad name", "a;b", strings.Repeat("a", 40)} {
		c := model.LAPSConfig{AccountName: bad}
		if Normalize(&c) == nil {
			t.Errorf("account %q accepted", bad)
		}
	}
	if Normalize(&model.LAPSConfig{PasswordLength: 5}) == nil || Normalize(&model.LAPSConfig{RotationDays: 9999}) == nil {
		t.Error("out-of-range values accepted")
	}
}

type call struct {
	name string
	args []string
}

// mac is a stateful stand-in for one local account. Like the real tools, sysadminctl exits 0
// even when it changes nothing; only -authonly tells the truth.
type mac struct {
	pw       string
	token    bool
	policy   func(string) bool // false: the password policy rejects it
	calls    []string
	exitCode bool // sysadminctl reports failures with a non-zero exit (older behaviour)
}

func (m *mac) run(_ context.Context, name string, args ...string) (string, error) {
	tool := name[strings.LastIndex(name, "/")+1:]
	m.calls = append(m.calls, tool+" "+args[0]+" "+args[1])
	ok := func(pw string) bool { return m.policy == nil || m.policy(pw) }
	fail := func(msg string) (string, error) {
		if m.exitCode {
			return msg, errors.New("exit 1")
		}
		return msg, nil
	}
	switch {
	case tool == "sysadminctl" && args[0] == "-secureTokenStatus":
		if m.token {
			return "Secure token is ENABLED for user " + args[1], nil
		}
		return "Secure token is DISABLED for user " + args[1], nil
	case tool == "sysadminctl" && args[0] == "-resetPasswordFor":
		if args[5] != m.pw {
			return fail("sysadminctl[123:456] Operation is not permitted without secure token unlock. Error:-14090 old " + args[5])
		}
		if !ok(args[3]) {
			return fail("sysadminctl[123:456] Password does not meet the requirements set by policy. Error:-14165 new " + args[3])
		}
		m.pw = args[3]
		return "", nil
	case tool == "dscl" && args[1] == "-authonly":
		if args[3] == m.pw {
			return "", nil
		}
		return "Authentication for node /Local/Default failed. (-14090, eDSAuthFailed)", errors.New("exit 1")
	case tool == "dscl" && args[1] == "-passwd":
		if !ok(args[3]) {
			return "DS Error: -14165 (eDSAuthPasswordQualityCheckFailed)", errors.New("exit 1")
		}
		m.pw = args[3]
		return "", nil
	}
	return "", errors.New("unexpected command")
}

func (m *mac) used(cmd string) bool {
	for _, c := range m.calls {
		if strings.HasPrefix(c, cmd) {
			return true
		}
	}
	return false
}

func TestSetPasswordPrefersSysadminctlThenFallsBack(t *testing.T) {
	ctx := context.Background()
	m := &mac{pw: "oldpw"}
	if err := SetPassword(ctx, m.run, "vibeadmin", "oldpw", "newpw"); err != nil || m.pw != "newpw" || m.used("dscl . -passwd") {
		t.Fatalf("sysadminctl path: err=%v pw=%q calls=%v", err, m.pw, m.calls)
	}
	// A stale escrowed password: sysadminctl exits 0 but changes nothing; dscl takes over.
	for _, exit := range []bool{false, true} {
		m = &mac{pw: "actual", exitCode: exit}
		if err := SetPassword(ctx, m.run, "vibeadmin", "stale", "newpw"); err != nil || m.pw != "newpw" || !m.used("dscl . -passwd") {
			t.Fatalf("fallback (exit code %v): err=%v pw=%q calls=%v", exit, err, m.pw, m.calls)
		}
	}
	m = &mac{pw: "unknown"}
	if err := SetPassword(ctx, m.run, "vibeadmin", "", "newpw"); err != nil || m.pw != "newpw" || m.used("sysadminctl -resetPasswordFor") {
		t.Fatalf("no old password: err=%v calls=%v", err, m.calls)
	}
}

func TestSetPasswordExplainsPolicyRejection(t *testing.T) {
	m := &mac{pw: "oldpw", policy: func(pw string) bool { return strings.ToUpper(pw) != pw }} // must contain a lowercase letter
	err := SetPassword(context.Background(), m.run, "vibeadmin", "oldpw", "NEWPW-NOLOWER")
	if err == nil || !strings.Contains(err.Error(), "password policy") {
		t.Fatalf("expected a policy explanation, got %v", err)
	}
	if strings.Contains(err.Error(), "NEWPW-NOLOWER") || strings.Contains(err.Error(), "oldpw") {
		t.Fatalf("error leaks a password: %v", err)
	}
	if m.pw != "oldpw" {
		t.Fatal("password changed despite the policy")
	}
}

func TestGenerateJamfFormat(t *testing.T) {
	for _, n := range []int{14, 29, 30, 31, 64} {
		p, err := GenerateFormat(FormatJamf, n)
		if err != nil || len(p) != n {
			t.Fatalf("%d: %q %v", n, p, err)
		}
		if strings.HasPrefix(p, "-") || strings.HasSuffix(p, "-") || strings.Contains(p, "--") {
			t.Fatalf("bad dashes: %q", p)
		}
		for _, c := range p {
			if c != '-' && !strings.ContainsRune(jamfAlphabet, c) {
				t.Fatalf("unexpected %q in %q", c, p)
			}
		}
	}
	if p, _ := GenerateFormat(FormatJamf, 30); p[5] != '-' || p[11] != '-' {
		t.Fatalf("groups of five expected: %q", p)
	}
	if ValidateManual("short") == nil || ValidateManual(" padded-password ") == nil || ValidateManual("fine-password-1") != nil {
		t.Fatal("ValidateManual")
	}
}

const listUsersAppleSilicon = `Cryptographic users for disk3s5 (3 found)
|
+-- 0E2A8F3C-1111-2222-3333-444455556666
|   Type: Local Open Directory User
|   Volume Owner: Yes
|
+-- 9F1B7D2E-AAAA-BBBB-CCCC-DDDDEEEEFFFF
|   Type: Local Open Directory User
|   Volume Owner: No
|
+-- EBC6C064-0000-11AA-AA11-00306543ECAC
    Type: MDM Bootstrap Token External Key
    Volume Owner: Yes
`

func TestParseVolumeOwner(t *testing.T) {
	if v := parseVolumeOwner(listUsersAppleSilicon, "0e2a8f3c-1111-2222-3333-444455556666"); v == nil || !*v {
		t.Fatalf("owner: %v", v)
	}
	if v := parseVolumeOwner(listUsersAppleSilicon, "9F1B7D2E-AAAA-BBBB-CCCC-DDDDEEEEFFFF"); v == nil || *v {
		t.Fatalf("not owner: %v", v)
	}
	intel := "Cryptographic users for disk1s1 (1 found)\n|\n+-- 0E2A8F3C-1111-2222-3333-444455556666\n    Type: Local Open Directory User\n"
	if v := parseVolumeOwner(intel, "0E2A8F3C-1111-2222-3333-444455556666"); v != nil {
		t.Fatalf("Intel has no volume owners: %v", *v)
	}
	if v := parseVolumeOwner(listUsersAppleSilicon, "MISSING"); v != nil {
		t.Fatal("unknown GUID")
	}
}

// A Secure Token account must never fall back to dscl: that desynchronises FileVault.
func TestSetPasswordRefusesDsclOnSecureTokenAccount(t *testing.T) {
	ctx := context.Background()
	m := &mac{pw: "actual", token: true}
	if err := SetPassword(ctx, m.run, "vibeadmin", "", "newpw"); err == nil || !strings.Contains(err.Error(), "Secure Token") {
		t.Fatalf("expected refusal, got %v", err)
	}
	err := SetPassword(ctx, m.run, "vibeadmin", "stale", "newpw")
	if err == nil || !strings.Contains(err.Error(), "not the account's password") || strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected a stale-password explanation without the password, got %v", err)
	}
	if m.used("dscl . -passwd") || m.pw != "actual" {
		t.Fatal("dscl must not run on a Secure Token account")
	}
	if err := SetPassword(ctx, m.run, "vibeadmin", "actual", "newpw"); err != nil || m.pw != "newpw" {
		t.Fatalf("with the right old password: %v", err)
	}
}

func TestGrantSecureTokenChecksStatus(t *testing.T) {
	granted := false
	run := func(_ context.Context, name string, args ...string) (string, error) {
		switch args[0] {
		case "-secureTokenOn":
			if args[7] == "right-pw" {
				granted = true
			}
			return "done (admin pw " + args[7] + ")", nil // sysadminctl exits 0 even on failure
		case "-secureTokenStatus":
			if granted {
				return "Secure token is ENABLED for user vibeadmin", nil
			}
			return "Secure token is DISABLED for user vibeadmin", nil
		}
		return "", nil
	}
	err := GrantSecureToken(context.Background(), run, "vibeadmin", "laps-pw", "alice", "wrong-pw")
	if err == nil || strings.Contains(err.Error(), "wrong-pw") {
		t.Fatalf("must fail without leaking the admin password: %v", err)
	}
	if err := GrantSecureToken(context.Background(), run, "vibeadmin", "laps-pw", "alice", "right-pw"); err != nil {
		t.Fatal(err)
	}
}
