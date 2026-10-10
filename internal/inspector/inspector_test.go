package inspector

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	raw := `<dict><key>RequestType</key><string>DeviceLock</string><key>PIN</key><string>123456</string><key>Message</key><string>hi</string>
<key>Payload</key><data>
AAAA
BBBB
</data></dict>`
	out := Redact(raw)
	if strings.Contains(out, "123456") || strings.Contains(out, "AAAA") {
		t.Fatalf("not redacted: %s", out)
	}
	if !strings.Contains(out, "<key>Message</key><string>hi</string>") {
		t.Fatal("non-sensitive value altered")
	}
	if !strings.Contains(out, "[redacted 6 bytes]") {
		t.Fatalf("missing placeholder: %s", out)
	}
}

func TestSummary(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?><plist version="1.0"><dict><key>Command</key><dict><key>RequestType</key><string>RemoveProfile</string><key>Identifier</key><string>com.x.y</string></dict></dict></plist>`)
	if s := Summary("RemoveProfile", raw); s != "Remove the profile com.x.y" {
		t.Errorf("got %q", s)
	}
	res := []byte(`<plist version="1.0"><dict><key>Status</key><string>Error</string><key>ErrorChain</key><array><dict><key>LocalizedDescription</key><string>Nope</string></dict></array></dict></plist>`)
	if s := ResultSummary("InstallProfile", "Error", res); s != "Error: Nope" {
		t.Errorf("got %q", s)
	}
}
