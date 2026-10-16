package api

import (
	"net/http"
	"strings"

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
	if p.SchemaValues != nil {
		// Application & Custom Settings values (e.g. an OIDC client secret, a license file).
		p.SchemaValues = redactTree(p.SchemaValues, nil, func(_ []string, key string, v any) bool {
			_, isStr := v.(string)
			return isStr && legacySecret.MatchString(key)
		}).(map[string]any)
	}
}

// redactPolicyFor hides what a policy runs from viewers: the "Files & Processes" command and
// script parameters, which Jamf-style scripts often use for API credentials.
func redactPolicyFor(r *http.Request, p *model.Policy) {
	if isAdmin(r) {
		return
	}
	if p.RunCommand != "" {
		p.RunCommand = hiddenForViewers
	}
	scripts := make([]model.PolicyScript, len(p.Scripts))
	for i, sc := range p.Scripts {
		scripts[i] = sc
		if len(sc.Parameters) > 0 {
			scripts[i].Parameters = make([]string, len(sc.Parameters))
			for j, v := range sc.Parameters {
				if v != "" {
					scripts[i].Parameters[j] = "[redacted]"
				}
			}
		}
	}
	p.Scripts = scripts
}

// redactEAFor hides extension attribute script bodies from viewers, like library scripts.
func redactEAFor(r *http.Request, e *model.ExtensionAttribute) {
	if !isAdmin(r) && e.Script != "" {
		e.Script = hiddenForViewers
	}
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

// attachmentHeaders makes a stored file download inert in the browser: a fixed content type
// (never sniffed from the uploaded file's extension), no script execution even if opened, and
// a quote-free filename.
func attachmentHeaders(w http.ResponseWriter, filename, contentType string) {
	filename = strings.NewReplacer(`"`, "", "\\", "", "\r", "", "\n", "").Replace(filename)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
}
