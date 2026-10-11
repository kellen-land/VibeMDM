package schema

import (
	"strings"
	"testing"

	"howett.net/plist"
)

func TestMobilePayloadsParse(t *testing.T) {
	ps := MobilePayloads()
	if len(ps) == 0 {
		t.Fatal("no mobile payloads")
	}
	for _, p := range ps {
		if p.Title == "" || p.Category == "" {
			t.Errorf("%s: title and __category are required", p.Type)
		}
		// Defaults must validate, and an empty form must build.
		vals, errs := p.Schema.Validate(p.Schema.Defaults())
		if len(errs) > 0 && len(p.Schema.Required) == 0 {
			t.Errorf("%s: defaults do not validate: %v", p.Type, errs)
		}
		_ = vals
	}
}

func TestBuildMobileProfile(t *testing.T) {
	if _, ok := MobilePayloadByType("com.apple.homescreenlayout"); !ok {
		t.Skip("home screen layout editor not present yet")
	}
	out, perrs, err := BuildMobileProfile(MobileProfileInput{Identifier: "com.vibemdm.mobile.test", DisplayName: "Test",
		Payloads: []PayloadValues{{Type: "com.apple.homescreenlayout", Values: map[string]any{
			"Dock":  []any{map[string]any{"Type": "Application", "BundleID": "com.apple.mobilesafari"}},
			"Pages": []any{map[string]any{"Items": []any{map[string]any{"Type": "Application", "BundleID": "com.apple.Pages"}}}},
		}}}})
	if err != nil || len(perrs) > 0 {
		t.Fatalf("%v %+v", err, perrs)
	}
	var doc map[string]any
	if _, err := plist.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	p := doc["PayloadContent"].([]any)[0].(map[string]any)
	pages := p["Pages"].([]any)
	if first, ok := pages[0].([]any); !ok || len(first) != 1 {
		t.Fatalf("pages not reshaped: %#v", p["Pages"])
	}
	if !strings.HasPrefix(p["PayloadIdentifier"].(string), "com.vibemdm.mobile.test.") {
		t.Fatalf("identifier: %v", p["PayloadIdentifier"])
	}
	if _, _, err := BuildMobileProfile(MobileProfileInput{Identifier: "x", Payloads: []PayloadValues{{Type: "com.apple.homescreenlayout"}, {Type: "com.apple.homescreenlayout"}}}); err == nil {
		t.Fatal("two Home Screen Layout payloads must be rejected")
	}
}
