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
	if err := SetPassword(context.Background(), run("sysadminctl"), "vibeadmin", "oldpw", "newpw"); err != nil || len(calls) != 2 || !strings.HasSuffix(calls[1].name, "dscl") {
		t.Fatalf("fallback calls=%v err=%v", calls, err)
	}
	calls = nil
	if err := SetPassword(context.Background(), run("none"), "vibeadmin", "", "newpw"); err != nil || !strings.HasSuffix(calls[0].name, "dscl") {
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
