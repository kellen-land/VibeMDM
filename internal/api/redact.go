package api

import (
	"net/http"

	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/inspector"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/schema"
)

// Viewers can see what is configured but not secrets embedded in it: profile
// payloads are redacted like MDM command plists, script bodies are hidden, and
// task payloads lose command text, script bodies and parameters.

func isAdmin(r *http.Request) bool { return auth.FromContext(r.Context()).IsAdmin() }

const hiddenForViewers = "# Visible to administrators only."

func redactProfileFor(r *http.Request, p *model.ConfigProfile) {
	if isAdmin(r) {
		return
	}
	if p.Payload != "" {
		p.Payload = inspector.Redact(p.Payload)
	}
	redactPayloads(schema.PlatformMacOS, p.Payloads)
}

func redactScriptFor(r *http.Request, s *model.Script) {
	if !isAdmin(r) && s.Contents != "" {
		s.Contents = hiddenForViewers
	}
}

var taskSecretKeys = []string{"command", "contents", "parameters", "env", "script", "auth_password"}

func redactTasksFor(r *http.Request, ts []model.AgentTask) {
	if isAdmin(r) {
		return
	}
	for i := range ts {
		if ts[i].Payload == nil {
			continue
		}
		cp := make(map[string]any, len(ts[i].Payload))
		for k, v := range ts[i].Payload {
			cp[k] = v
		}
		for _, k := range taskSecretKeys {
			if _, ok := cp[k]; ok {
				cp[k] = "[redacted]"
			}
		}
		ts[i].Payload = cp
	}
}
