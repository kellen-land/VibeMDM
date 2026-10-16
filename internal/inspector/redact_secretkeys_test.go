package inspector

import (
	"strings"
	"testing"
)

func TestRedactSecretNamedKeys(t *testing.T) {
	raw := `<dict>
<key>UserPassword</key><string>eap-secret-1</string>
<key>IncomingPassword</key> <!-- note --> <string>mail-secret-2</string>
<key>clientSecret</key><string>oidc-secret-3</string>
<key>LicenseFile</key><data>bGljZW5zZQ==</data>
<key>SSID_STR</key><string>OfficeWiFi</string>
</dict>`
	out := Redact(raw)
	for _, leak := range []string{"eap-secret-1", "mail-secret-2", "oidc-secret-3", "bGljZW5zZQ=="} {
		if strings.Contains(out, leak) {
			t.Errorf("%q not redacted:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "OfficeWiFi") {
		t.Error("non-secret value was redacted")
	}
}
