package agent

import (
	"html/template"
	"strings"
	"testing"
)

func TestSelfServicePageUsesNonceAndNoInlineHandlers(t *testing.T) {
	var b strings.Builder
	err := ssPage.Execute(&b, map[string]any{"AppName": "Self Service", "Accent": template.CSS("#22d3ee"), "Nonce": "TESTNONCE",
		"ConfigJSON": ssConfig{AppName: "Self Service"}})
	if err != nil {
		t.Fatal(err)
	}
	page := b.String()
	if !strings.Contains(page, `<script nonce="TESTNONCE">`) {
		t.Fatal("script tag lacks the CSP nonce")
	}
	for _, h := range []string{"onerror=", "onclick=", "onload="} {
		if strings.Contains(page, h) {
			t.Fatalf("inline handler %q would be blocked by the CSP", h)
		}
	}
}
