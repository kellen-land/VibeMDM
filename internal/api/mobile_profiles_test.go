package api

import (
	"net/http/httptest"
	"testing"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/schema"
)

func TestRedactPayloads(t *testing.T) {
	v := map[string]any{"SSID_STR": "School", "Password": "pw", "EAPClientConfiguration": map[string]any{"UserPassword": "s", "UserName": "u"}}
	ps := []model.PayloadValues{{Type: "com.apple.wifi.managed", Values: v},
		{Type: "com.apple.security.pkcs12", Values: map[string]any{"PayloadContent": "MIIF…", "PayloadCertificateFileName": "a.p12"}},
		{Type: "com.apple.vpn.managed", Values: map[string]any{"IKEv2": map[string]any{"SharedSecret": "s", "RemoteAddress": "vpn.example"}}}}
	redactPayloads(schema.PlatformIOS, ps)
	out := ps[0].Values
	if out["Password"] != "[redacted]" || out["SSID_STR"] != "School" {
		t.Fatalf("top level: %v", out)
	}
	if e := out["EAPClientConfiguration"].(map[string]any); e["UserPassword"] != "[redacted]" || e["UserName"] != "u" {
		t.Fatalf("nested: %v", e)
	}
	if ik := ps[2].Values["IKEv2"].(map[string]any); ik["SharedSecret"] != "[redacted]" || ik["RemoteAddress"] != "vpn.example" {
		t.Fatalf("vpn: %v", ik)
	}
	if v["Password"] != "pw" {
		t.Fatal("the original values must not be modified")
	}
	if id := ps[1].Values; id["PayloadContent"] != "[redacted]" || id["PayloadCertificateFileName"] != "a.p12" {
		t.Fatalf("identity: %v", id)
	}
}

func TestInvalidInputIsA400(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	h.fail(rec, errInvalid("unknown task type"), "task")
	if rec.Code != 400 {
		t.Fatalf("got %d", rec.Code)
	}
}
