package schema

import (
	"strings"
	"testing"
)

const diskManager = `{
  "title": "Disk Manager (net.example.diskmanager)",
  "__preferencedomain": "net.example.diskmanager",
  "options": {"remove_empty_properties": true},
  "properties": {
    "askForPasswordDelay": {"title": "Ask For Password Delay", "type": "integer", "property_order": 5,
      "options": {"enum_titles": ["immediately","5 seconds","1 Minute"], "infoText": "Key name: askForPasswordDelay"},
      "enum": [0, 5, 60]},
    "diskCacheSize": {"title": "Disk Cache Size", "type": "integer", "default": 25600, "minimum": 0, "property_order": 10},
    "serverName": {"type": "string", "pattern": "^[a-z.]+$"},
    "ratio": {"type": "number"},
    "enabled": {"anyOf": [{"title": "Not Configured", "type": "null"}, {"title": "Configured", "type": "boolean"}]},
    "shares": {"type": "array", "items": {"type": "object", "properties": {"path": {"type": "string"}, "readOnly": {"type": "boolean"}}, "required": ["path"]}}
  }
}`

func TestParseValidate(t *testing.T) {
	s, err := Parse([]byte(diskManager))
	if err != nil {
		t.Fatal(err)
	}
	if s.PreferenceDomain != "net.example.diskmanager" || s.OrderedKeys()[0] != "askForPasswordDelay" {
		t.Fatalf("parse: %q %v", s.PreferenceDomain, s.OrderedKeys())
	}
	vals, errs := s.Validate(map[string]any{
		"askForPasswordDelay": float64(5), "diskCacheSize": float64(100), "serverName": "files.example", "ratio": 1.5,
		"enabled": true, "shares": []any{map[string]any{"path": "/srv", "readOnly": false}}, "empty": nil,
	})
	// "empty" is unknown even when nil
	if len(errs) != 1 || errs[0].Path != "empty" {
		t.Fatalf("errs = %v", errs)
	}
	if vals["askForPasswordDelay"] != int64(5) || vals["ratio"] != 1.5 || vals["enabled"] != true {
		t.Fatalf("coercion: %#v", vals)
	}
	_, errs = s.Validate(map[string]any{"askForPasswordDelay": float64(7), "diskCacheSize": -1.0, "serverName": "BAD NAME",
		"enabled": "yes", "shares": []any{map[string]any{"readOnly": true}}})
	got := map[string]bool{}
	for _, e := range errs {
		got[e.Path] = true
	}
	for _, want := range []string{"askForPasswordDelay", "diskCacheSize", "serverName", "enabled", "shares[0].path"} {
		if !got[want] {
			t.Errorf("expected an error for %s; got %v", want, errs)
		}
	}
}

func TestBuildProfile(t *testing.T) {
	b, err := BuildProfile(ProfileInput{Identifier: "com.vibemdm.custom.x", DisplayName: "Disk Manager", Domain: "net.example.diskmanager",
		Values: map[string]any{"diskCacheSize": int64(100), "enabled": true}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"com.apple.ManagedClient.preferences", "<key>net.example.diskmanager</key>", "mcx_preference_settings", "<integer>100</integer>"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
	b2, _ := BuildProfile(ProfileInput{Identifier: "com.vibemdm.custom.x", DisplayName: "Disk Manager", Domain: "net.example.diskmanager",
		Values: map[string]any{"diskCacheSize": int64(100), "enabled": true}})
	if string(b2) != s {
		t.Error("profile is not deterministic")
	}
	if _, err := BuildProfile(ProfileInput{Identifier: "x", Domain: "not a domain"}); err == nil {
		t.Error("bad domain accepted")
	}
}

func TestBuiltins(t *testing.T) {
	bs := Builtins()
	if len(bs) < 2 {
		t.Fatalf("builtins: %d", len(bs))
	}
	sup, ok := BuiltinByDomain("com.macjutsu.super")
	if !ok {
		t.Fatal("super schema missing")
	}
	vals, errs := sup.Schema.Validate(map[string]any{"DeadlineDaysHard": "14", "InstallMacOSMajorUpgrades": false, "DeferralTimerMenu": "60,240"})
	if len(errs) > 0 || vals["DeadlineDaysHard"] != "14" {
		t.Fatalf("super: %v %v", vals, errs)
	}
	if _, errs := sup.Schema.Validate(map[string]any{"DeadlineDaysHard": "two weeks"}); len(errs) == 0 {
		t.Error("non-numeric day count accepted")
	}
}

func TestDataFormat(t *testing.T) {
	x, _ := BuiltinByDomain("com.twocanoes.xcreds")
	vals, errs := x.Schema.Validate(map[string]any{"LicenseFile": "aGVsbG8="})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if b, ok := vals["LicenseFile"].([]byte); !ok || string(b) != "hello" {
		t.Fatalf("LicenseFile = %#v", vals["LicenseFile"])
	}
	p, _ := BuildProfile(ProfileInput{Identifier: "com.vibemdm.xcreds", Domain: "com.twocanoes.xcreds", Values: vals})
	if !strings.Contains(string(p), "<data>") {
		t.Error("license not emitted as <data>")
	}
	if _, errs := x.Schema.Validate(map[string]any{"LicenseFile": "not base64!"}); len(errs) == 0 {
		t.Error("invalid base64 accepted")
	}
}
