package enroll

import (
	"strings"
	"testing"

	"howett.net/plist"
)

func TestBuild(t *testing.T) {
	out, err := Build(Params{ServerURL: "https://mdm.example.com/", Topic: "com.apple.mgmt.External.abc", SCEPChallenge: "secret", OrgName: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if _, err := plist.Unmarshal(out, &doc); err != nil {
		t.Fatalf("output is not a plist: %v", err)
	}
	s := string(out)
	for _, want := range []string{"https://mdm.example.com/scep", "https://mdm.example.com/mdm", "com.apple.mgmt.External.abc", "<string>secret</string>", "com.apple.mdm", "com.apple.security.scep", "Acme"} {
		if !strings.Contains(s, want) {
			t.Errorf("profile missing %q", want)
		}
	}
	if doc["PayloadType"] != "Configuration" {
		t.Errorf("PayloadType = %v", doc["PayloadType"])
	}
}

func TestBuildRequiresTopic(t *testing.T) {
	if _, err := Build(Params{ServerURL: "https://x", SCEPChallenge: "c"}); err == nil {
		t.Fatal("expected error without topic")
	}
}
