package health

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

func TestDeclarationIssues(t *testing.T) {
	tree := map[string]any{
		"activations": []any{map[string]any{"identifier": "a1", "valid": "valid", "active": true}},
		"configurations": []any{
			map[string]any{"identifier": "c1", "valid": "invalid", "reasons": []any{map[string]any{"code": "Error.ConfigurationCannotBeApplied", "description": "needs supervision"}}},
			map[string]any{"identifier": "c2", "valid": false},
			map[string]any{"identifier": "c3", "valid": "unknown"},
		},
	}
	got := map[string]string{}
	declarationIssues(tree, got)
	if len(got) != 2 || got["c1"] != "needs supervision" {
		t.Fatalf("got %v", got)
	}
	if _, ok := got["c2"]; !ok {
		t.Fatal("boolean false validity not detected")
	}
}

func TestExpiryFinding(t *testing.T) {
	for _, tc := range []struct {
		left time.Duration
		sev  string
		bad  bool
	}{
		{-48 * time.Hour, Critical, true},
		{3 * 24 * time.Hour, Critical, true},
		{20 * 24 * time.Hour, Warning, true},
		{200 * 24 * time.Hour, "", false},
	} {
		sev, _, bad := expiryFinding(&x509.Certificate{NotAfter: time.Now().Add(tc.left)}, 30, 7)
		if sev != tc.sev || bad != tc.bad {
			t.Errorf("left=%v: got %q %v", tc.left, sev, bad)
		}
	}
}

func TestEffectiveSettings(t *testing.T) {
	c := Check{ID: "x", AutoHealDefault: true, SupportsHeal: true}
	if en, heal := effective(c, nil); !en || !heal {
		t.Fatal("defaults")
	}
	off := false
	if _, heal := effective(c, map[string]model.HealthCheckSetting{"x": {Enabled: true, AutoHeal: &off}}); heal {
		t.Fatal("override ignored")
	}
	c.SupportsHeal = false
	if _, heal := effective(c, nil); heal {
		t.Fatal("heal without support")
	}
}
