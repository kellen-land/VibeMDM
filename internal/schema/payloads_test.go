package schema

import (
	"testing"
	"time"

	"howett.net/plist"
)

func build(t *testing.T, in PayloadProfileInput) map[string]any {
	t.Helper()
	out, perrs, err := BuildPayloadProfile(in)
	if err != nil || len(perrs) > 0 {
		t.Fatalf("%v %+v", err, perrs)
	}
	var doc map[string]any
	if _, err := plist.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestEveryEditorLoads(t *testing.T) {
	for _, plat := range []string{PlatformIOS, PlatformMacOS} {
		ps := Payloads(plat)
		if len(ps) < 40 {
			t.Fatalf("%s: only %d payload editors", plat, len(ps))
		}
		for _, p := range ps {
			if p.Title == "" || p.Category == "" || p.PayloadType == "" {
				t.Errorf("%s %s: title, category and payload type are required", plat, p.Type)
			}
			if plat == PlatformMacOS && len(p.Channels) == 0 {
				t.Errorf("%s: no channels", p.Type)
			}
		}
	}
}

func TestCertificateReference(t *testing.T) {
	cert := "11111111-2222-3333-4444-555555555555"
	in := PayloadProfileInput{Platform: PlatformMacOS, Identifier: "com.vibemdm.profile.t", DisplayName: "Wi-Fi", Payloads: []PayloadValues{
		{Type: "com.apple.security.pkcs12", UUID: cert, Values: map[string]any{"PayloadContent": "AAEC"}},
		{Type: "com.apple.wifi.managed", Values: map[string]any{"SSID_STR": "Staff", "PayloadCertificateUUID": cert,
			"EAPClientConfiguration": map[string]any{"AcceptEAPTypes": []any{float64(13)}}}},
	}}
	doc := build(t, in)
	pc := doc["PayloadContent"].([]any)
	if pc[0].(map[string]any)["PayloadUUID"] != cert || pc[1].(map[string]any)["PayloadCertificateUUID"] != cert {
		t.Fatalf("reference not kept: %v", pc)
	}
	in.Payloads[1].Values["PayloadCertificateUUID"] = "99999999-2222-3333-4444-555555555555"
	if _, perrs, _ := BuildPayloadProfile(in); len(perrs) == 0 {
		t.Fatal("a dangling certificate reference must be rejected")
	}
}

func TestEthernetInterface(t *testing.T) {
	doc := build(t, PayloadProfileInput{Platform: PlatformMacOS, Identifier: "x", Payloads: []PayloadValues{
		{Type: "com.apple.globalethernet.managed", Values: map[string]any{InterfaceKey: "first-active"}}}})
	p := doc["PayloadContent"].([]any)[0].(map[string]any)
	if p["PayloadType"] != "com.apple.firstactiveethernet.managed" || p["Interface"] != "FirstActiveEthernet" || p[InterfaceKey] != nil {
		t.Fatalf("ethernet: %v", p)
	}
}

func TestMapsAndLevels(t *testing.T) {
	doc := build(t, PayloadProfileInput{Platform: PlatformMacOS, Identifier: "x", Payloads: []PayloadValues{
		{Type: "com.apple.syspolicy.kernel-extension-policy", Values: map[string]any{"AllowedKernelExtensions": map[string]any{"ABCDE12345": []any{"com.example.kext"}}}}}})
	p := doc["PayloadContent"].([]any)[0].(map[string]any)
	if ke := p["AllowedKernelExtensions"].(map[string]any)["ABCDE12345"].([]any); ke[0] != "com.example.kext" {
		t.Fatalf("map: %v", p)
	}
	if doc["PayloadScope"] != "System" {
		t.Fatal("computer level must be System")
	}
	// A computer-only payload in a user-level profile is refused; a user-capable one is User scope.
	if _, _, err := BuildPayloadProfile(PayloadProfileInput{Platform: PlatformMacOS, Identifier: "x", Level: "user", Payloads: []PayloadValues{
		{Type: "com.apple.syspolicy.kernel-extension-policy", Values: map[string]any{}}}}); err == nil {
		t.Fatal("computer-only payload accepted at user level")
	}
	u := build(t, PayloadProfileInput{Platform: PlatformMacOS, Identifier: "x", Level: "user", Payloads: []PayloadValues{
		{Type: "com.apple.dock", Values: map[string]any{"tilesize": float64(48)}}}})
	if u["PayloadScope"] != "User" {
		t.Fatal("user level must be User scope")
	}
}

func TestDates(t *testing.T) {
	n := &Node{Type: "string", Format: "date-time"}
	var errs []FieldError
	v, ok := validateValue(n, "2026-10-12T08:30", "d", &errs)
	if !ok || len(errs) > 0 {
		t.Fatal(errs)
	}
	if tm, _ := v.(time.Time); tm.Hour() != 8 {
		t.Fatalf("date: %v", v)
	}
}
