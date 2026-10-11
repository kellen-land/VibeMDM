package laps

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vibemdm/vibemdm/internal/model"
)

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

func TestSetPasswordPrefersSysadminctlThenFallsBack(t *testing.T) {
	var calls []call
	run := func(fail string) Runner {
		return func(_ context.Context, name string, args ...string) (string, error) {
			calls = append(calls, call{name, args})
			if strings.HasSuffix(name, fail) {
				return "boom oldpw newpw", errors.New("exit 1")
			}
			return "", nil
		}
	}
	if err := SetPassword(context.Background(), run("none"), "vibeadmin", "oldpw", "newpw"); err != nil || len(calls) != 1 || !strings.HasSuffix(calls[0].name, "sysadminctl") {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
	calls = nil
	if err := SetPassword(context.Background(), run("sysadminctl"), "vibeadmin", "oldpw", "newpw"); err != nil || !strings.HasSuffix(calls[len(calls)-1].name, "dscl") {
		t.Fatalf("fallback calls=%v err=%v", calls, err)
	}
	calls = nil
	if err := SetPassword(context.Background(), run("none"), "vibeadmin", "", "newpw"); err != nil || !strings.HasSuffix(calls[len(calls)-1].name, "dscl") {
		t.Fatalf("no-old calls=%v err=%v", calls, err)
	}
	err := SetPassword(context.Background(), run("dscl"), "vibeadmin", "", "newpw")
	if err == nil || strings.Contains(err.Error(), "newpw") {
		t.Fatalf("error must fail and not leak the password: %v", err)
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
	var calls []string
	run := func(_ context.Context, name string, args ...string) (string, error) {
		calls = append(calls, args[0])
		switch {
		case strings.HasSuffix(name, "sysadminctl") && args[0] == "-secureTokenStatus":
			return "sysadminctl[1:2] Secure token is ENABLED for user vibeadmin", nil
		case strings.HasSuffix(name, "sysadminctl"):
			return "error", errors.New("exit 1")
		}
		return "", nil
	}
	if err := SetPassword(context.Background(), run, "vibeadmin", "", "newpw"); err == nil || !strings.Contains(err.Error(), "Secure Token") {
		t.Fatalf("expected refusal, got %v", err)
	}
	if err := SetPassword(context.Background(), run, "vibeadmin", "oldpw", "newpw"); err == nil {
		t.Fatal("expected failure when sysadminctl fails on a Secure Token account")
	}
	for _, c := range calls {
		if c == "." {
			t.Fatal("dscl must not run on a Secure Token account")
		}
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
