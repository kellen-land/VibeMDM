package compliance

import (
	"strings"
	"testing"
)

func TestCatalog(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Rules) < 200 || len(c.Baselines) < 5 {
		t.Fatalf("catalog too small: %d rules, %d baselines", len(c.Rules), len(c.Baselines))
	}
	gk := c.Rules["os_gatekeeper_enable"]
	if gk == nil || gk.Check == "" || gk.Result == nil || gk.Result.Value != "PASS" {
		t.Fatalf("gatekeeper rule: %+v", gk)
	}
	if gk.Severity("disa_stig", "27.0") != "high" {
		t.Errorf("severity = %q", gk.Severity("disa_stig", "27.0"))
	}
	if !gk.AppliesTo(MajorVersion("15.4.1")) || gk.AppliesTo("12.0") {
		t.Error("AppliesTo")
	}
	lock := c.Rules["pwpolicy_account_lockout_timeout_enforce"]
	if v, ok := lock.ODVValue("cis_lvl1", nil); !ok || odvText(v) != "15" {
		t.Errorf("odv = %v", v)
	}
	if err := lock.ValidateODV(0); err == nil {
		t.Error("min not enforced")
	}
	scannable, manual := 0, 0
	for _, r := range c.RulesFor("cis_lvl1", nil, nil) {
		if r.ManualReason() == "" {
			scannable++
		} else {
			manual++
		}
	}
	t.Logf("cis_lvl1: %d scannable, %d manual", scannable, manual)
	if scannable < 50 {
		t.Errorf("only %d scannable rules", scannable)
	}
}

func TestBuildProfile(t *testing.T) {
	c, _ := Load()
	rules := c.RulesFor("cis_lvl1", nil, nil)
	b, covered, err := BuildProfile(ProfileInput{Identifier: "com.vibemdm.compliance.test", DisplayName: "CIS L1", ParentValues: "cis_lvl1", Rules: rules,
		ODVOverrides: map[string]any{"pwpolicy_account_lockout_timeout_enforce": 30}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"com.apple.systempolicy.control", "<key>EnableAssessment</key>", "mcx_preference_settings", "minutesUntilFailedLoginReset</key>\n\t\t\t\t<integer>30</integer>"} {
		if !strings.Contains(s, want) {
			t.Errorf("profile missing %q", want)
		}
	}
	if strings.Contains(s, "$ODV") {
		t.Error("unsubstituted $ODV in profile")
	}
	t.Logf("%d rules covered, %d bytes", len(covered), len(b))
}
