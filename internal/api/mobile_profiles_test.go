package api

import "testing"

func TestRedactMobileValues(t *testing.T) {
	v := map[string]any{"SSID_STR": "School", "Password": "pw", "IKEv2": map[string]any{"SharedSecret": "s", "RemoteAddress": "vpn.example"},
		"List": []any{map[string]any{"AuthPassword": "x"}}}
	out := redactValues(v, false).(map[string]any)
	if out["Password"] != "[redacted]" || out["SSID_STR"] != "School" {
		t.Fatalf("top level: %v", out)
	}
	if ik := out["IKEv2"].(map[string]any); ik["SharedSecret"] != "[redacted]" || ik["RemoteAddress"] != "vpn.example" {
		t.Fatalf("nested: %v", ik)
	}
	if l := out["List"].([]any)[0].(map[string]any); l["AuthPassword"] != "[redacted]" {
		t.Fatalf("in arrays: %v", l)
	}
	if v["Password"] != "pw" {
		t.Fatal("the original values must not be modified")
	}
	id := redactValues(map[string]any{"PayloadContent": "MIIF…", "PayloadCertificateFileName": "a.p12"}, true).(map[string]any)
	if id["PayloadContent"] != "[redacted]" || id["PayloadCertificateFileName"] != "a.p12" {
		t.Fatalf("identity: %v", id)
	}
}
