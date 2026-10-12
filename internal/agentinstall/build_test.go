package agentinstall

import (
	"os"
	"strings"
	"testing"

	"howett.net/plist"
)

func TestBuildScriptCopiesAreIdentical(t *testing.T) {
	deployed, err := os.ReadFile("../../deploy/macos/build-agent-pkg.sh")
	if err != nil {
		t.Fatal(err)
	}
	if string(deployed) != buildScript {
		t.Fatal("deploy/macos/build-agent-pkg.sh and internal/agentinstall/build-agent-pkg.sh differ; keep them identical")
	}
	for _, want := range []string{"umask 022", "find-identity -v", "productsign"} {
		if !strings.Contains(buildScript, want) {
			t.Errorf("script lacks %q", want)
		}
	}
}

func TestCATrustProfileAndCommand(t *testing.T) {
	caPEM, _, err := NewCA("Lands")
	if err != nil {
		t.Fatal(err)
	}
	b, err := CATrustProfile("Lands", caPEM)
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if _, err := plist.Unmarshal(b, &p); err != nil || p["PayloadIdentifier"] != CATrustIdentifier {
		t.Fatalf("profile: %v %v", err, p["PayloadIdentifier"])
	}
	payload := p["PayloadContent"].([]any)[0].(map[string]any)
	if payload["PayloadType"] != "com.apple.security.root" || len(payload["PayloadContent"].([]byte)) < 500 {
		t.Fatalf("root payload: %v", payload["PayloadType"])
	}
	cmd := buildCommand("https://mdm.example.com", "vmdm_secret")
	if !strings.HasPrefix(cmd, "umask 022") || !strings.Contains(cmd, "VIBEMDM_API_TOKEN='vmdm_secret'") || !strings.Contains(cmd, `rm -f "$S"`) {
		t.Fatalf("command:\n%s", cmd)
	}
}
