package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibemdm/vibemdm/internal/model"
)

const uiScope = `{"all_computers":false,"targets":{"computer_ids":[],"group_ids":[],"buildings":[],"departments":[],"network_segments":[],"users":[]},"limitations":{"users":[],"network_segments":[]},"exclusions":{"computer_ids":[],"group_ids":[],"buildings":[],"departments":[],"network_segments":[],"users":[]}}`

// The console's "new" forms send created_at/updated_at as "" (see blank() in
// PolicyEditorPage.tsx and AppEditorPage.tsx). These are the exact bodies; they
// used to fail with `parsing time "" as "2006-01-02T15:04:05Z07:00"`.
func TestDecodeAcceptsUIBlankPolicy(t *testing.T) {
	body := `{"id":"","name":"p","enabled":true,"category":"","triggers":{"startup":false,"login":false,"checkin":true,"enrollment_complete":false,"network_change":false,"custom":""},"frequency":"once_per_computer","scope":` + uiScope + `,"scripts":[],"packages":[],"apps":[],"run_command":"","update_inventory":false,"reboot":{"enabled":false,"message":"","delay_minutes":0},"user_interaction":{"start_message":"","complete_message":""},"self_service":{"enabled":false,"display_name":"","description":"","button_text":"Install"},"created_at":"","updated_at":""}`
	var p model.Policy
	if err := decode(httptest.NewRequest("POST", "/api/v1/policies", strings.NewReader(body)), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Name != "p" || !p.Triggers.Checkin || !p.CreatedAt.IsZero() || !p.UpdatedAt.IsZero() {
		t.Fatalf("unexpected decode result: %+v", p)
	}
}

func TestDecodeAcceptsUIBlankApp(t *testing.T) {
	body := `{"id":"","name":"a","label":"firefox","category":"","description":"","update_behavior":"install_once","enabled":true,"scope":` + uiScope + `,"self_service":{"enabled":false,"display_name":"","description":"","button_text":"Install"},"created_at":"","updated_at":""}`
	var a model.MacApp
	if err := decode(httptest.NewRequest("POST", "/api/v1/apps", strings.NewReader(body)), &a); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if a.Label != "firefox" || !a.CreatedAt.IsZero() {
		t.Fatalf("unexpected decode result: %+v", a)
	}
}

func TestDecodeKeepsRealTimestampsAndRejectsBadOnes(t *testing.T) {
	var p model.Policy
	ok := `{"name":"p","created_at":"2026-10-11T01:05:53Z"}`
	if err := decode(httptest.NewRequest("PUT", "/x", strings.NewReader(ok)), &p); err != nil || p.CreatedAt.IsZero() {
		t.Fatalf("valid timestamp lost: err=%v created=%v", err, p.CreatedAt)
	}
	// Only the empty string is tolerated; garbage still reports a clear 400 message.
	bad := `{"name":"p","created_at":"yesterday"}`
	if err := decode(httptest.NewRequest("PUT", "/x", strings.NewReader(bad)), &p); err == nil {
		t.Fatal("expected an error for an unparseable timestamp")
	}
	if err := decode(httptest.NewRequest("PUT", "/x", strings.NewReader(`{`)), &p); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}
