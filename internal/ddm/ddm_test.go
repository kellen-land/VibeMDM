package ddm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildAndTokens(t *testing.T) {
	base := Build(nil, nil)
	if len(base) != 2 || base[0].Type != TypeStatusSubscriptions || base[1].Kind() != "activation" {
		t.Fatalf("base set: %+v", base)
	}
	enf := &Enforcement{TargetOSVersion: "15.6", TargetLocalDateTime: "2026-11-01T17:00:00"}
	with := Build(enf, nil)
	if len(with) != 4 {
		t.Fatalf("with enforcement: %d declarations", len(with))
	}
	t1, t2 := DeclarationsToken(base), DeclarationsToken(with)
	if t1 == t2 {
		t.Fatal("token must change when declarations change")
	}
	if DeclarationsToken(Build(enf, nil)) != t2 {
		t.Fatal("token must be stable for identical declarations")
	}
	enf2 := *enf
	enf2.TargetLocalDateTime = "2026-11-02T17:00:00"
	if DeclarationsToken(Build(&enf2, nil)) == t2 {
		t.Fatal("token must change when payload changes")
	}
	// Activation references the enforcement configuration.
	act, err := Find(with, "declaration/activation/com.vibemdm.ddm.activation.softwareupdate.enforcement")
	if err != nil || act.Payload["StandardConfigurations"].([]string)[0] != "com.vibemdm.ddm.softwareupdate.enforcement" {
		t.Fatalf("activation: %+v %v", act, err)
	}
	if _, err := Find(with, "declaration/configuration/nope"); err == nil {
		t.Fatal("expected not found")
	}
}

func TestItemsResponseShape(t *testing.T) {
	b, err := ItemsResponse(Build(&Enforcement{TargetOSVersion: "15.6", TargetLocalDateTime: "2026-11-01T17:00:00"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Declarations      map[string][]map[string]string
		DeclarationsToken string
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"Activations", "Configurations", "Assets", "Management"} {
		if _, ok := doc.Declarations[k]; !ok {
			t.Errorf("manifest missing %s (Apple requires all four arrays)", k)
		}
	}
	if len(doc.Declarations["Configurations"]) != 2 || len(doc.Declarations["Activations"]) != 2 || doc.DeclarationsToken == "" {
		t.Fatalf("manifest: %s", b)
	}
}

func TestSettingsPayloadAndValidation(t *testing.T) {
	on := true
	s := Settings{Notifications: &on, MajorDeferralDays: 30, AutoInstallSecurityUpdates: "AlwaysOn"}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	d := Build(nil, &s)
	b, _ := json.Marshal(d[2].Payload)
	for _, want := range []string{`"Notifications":true`, `"MajorPeriodInDays":30`, `"InstallSecurityUpdate":"AlwaysOn"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("payload missing %s: %s", want, b)
		}
	}
	if (Settings{MajorDeferralDays: 120}).Validate() == nil || (Settings{AutoDownload: "Sometimes"}).Validate() == nil {
		t.Error("invalid settings accepted")
	}
	if (Enforcement{TargetOSVersion: "15.x", TargetLocalDateTime: "tomorrow"}).Validate() == nil {
		t.Error("invalid enforcement accepted")
	}
}

func TestMergeStatus(t *testing.T) {
	st := MergeStatus(nil, map[string]any{"softwareupdate": map[string]any{"install-state": "none", "pending-version": map[string]any{"os-version": "15.6"}}})
	st = MergeStatus(st, map[string]any{"softwareupdate": map[string]any{"install-state": "downloading"}})
	su := st["softwareupdate"].(map[string]any)
	if su["install-state"] != "downloading" || su["pending-version"] == nil {
		t.Fatalf("merge lost data: %+v", st)
	}
	if CompareVersions("15.10", "15.9") != 1 || CompareVersions("15.6", "15.6.0") != 0 || CompareVersions("14.7.6", "15") != -1 {
		t.Fatal("version compare")
	}
}
