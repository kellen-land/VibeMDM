package server

import (
	"html/template"
	"net/http"

	"github.com/vibemdm/vibemdm/internal/store"
)

var enrollPage = template.Must(template.New("enroll").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Enroll in {{.Org}}</title>
<style>
:root{--bg:#f6f7f9;--card:#fff;--fg:#1a1d23;--muted:#5d6472;--accent:#4f46e5;--border:#e3e6ea}
@media (prefers-color-scheme:dark){:root{--bg:#0f1115;--card:#171a21;--fg:#e8eaf0;--muted:#9aa3b2;--accent:#818cf8;--border:#262b36}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.5 -apple-system,BlinkMacSystemFont,"SF Pro Text",Inter,system-ui,sans-serif}
main{max-width:680px;margin:0 auto;padding:48px 16px}h1{font-size:28px;margin:0 0 4px}.sub{color:var(--muted);margin:0 0 32px}
.card{background:var(--card);border:1px solid var(--border);border-radius:14px;padding:22px 24px;margin-bottom:16px}
.step{display:flex;gap:16px}.num{flex:none;width:32px;height:32px;border-radius:50%;background:var(--accent);color:#fff;display:flex;align-items:center;justify-content:center;font-weight:600}
h2{font-size:17px;margin:2px 0 6px}p{margin:0 0 10px;color:var(--muted)}
.btn{display:inline-block;background:var(--accent);color:#fff;text-decoration:none;padding:10px 18px;border-radius:9px;font-weight:600}
code,pre{font:13px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace}pre{background:var(--bg);border:1px solid var(--border);border-radius:9px;padding:12px;overflow:auto;white-space:pre-wrap;word-break:break-all}
.err{background:#fde8e8;border:1px solid #f5b5b5;color:#8a1c1c;border-radius:10px;padding:14px 16px}
footer{color:var(--muted);font-size:13px;text-align:center;margin-top:32px}
</style></head><body><main>
<h1>Enroll your Mac in {{.Org}}</h1>
<p class="sub">Device management powered by VibeMDM</p>
{{if .Error}}<div class="err">{{.Error}}</div>{{else}}
<div class="card"><div class="step"><div class="num">1</div><div>
<h2>Download the enrollment profile</h2>
<p>This installs the management certificate and connects your Mac to {{.Org}}.</p>
<a class="btn" href="{{.ProfileURL}}">Download Enrollment Profile</a>
</div></div></div>
<div class="card"><div class="step"><div class="num">2</div><div>
<h2>Approve the profile</h2>
<p>Open <strong>System Settings → General → Device Management</strong> (or <strong>Privacy &amp; Security → Profiles</strong>), select the downloaded profile and click <strong>Install</strong>. You will need an administrator password.</p>
</div></div></div>
<div class="card"><div class="step"><div class="num">3</div><div>
<h2>Install the management agent <span style="color:var(--muted);font-weight:400">(administrators)</span></h2>
<p>The agent runs scripts, policies and inventory. Paste this into Terminal:</p>
<pre>{{.InstallCommand}}</pre>
</div></div></div>
{{end}}
<footer>{{.Org}} · VibeMDM</footer>
</main></body></html>`))

// enrollHandler renders the user-initiated enrollment page.
func (s *Server) enrollHandler(w http.ResponseWriter, r *http.Request) {
	settings, err := s.st.GetSettings(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	token := r.URL.Query().Get("token")
	data := map[string]any{"Org": settings.OrgName}
	ok := settings.OpenEnrollment
	if !ok && token != "" {
		if _, err := s.st.CheckInvitation(r.Context(), store.HashToken(token)); err == nil {
			ok = true
		} else if token == settings.AgentEnrollmentSecret && token != "" {
			ok = true
		}
	}
	if !ok {
		data["Error"] = "This enrollment link is invalid or has expired. Ask your administrator for a new enrollment link."
	} else {
		base := settings.ServerURL
		if base == "" {
			base = "https://" + r.Host
		}
		q := ""
		if token != "" {
			q = "?token=" + template.URLQueryEscaper(token)
		}
		data["ProfileURL"] = "/enroll/profile" + q
		tok := token
		if tok == "" {
			tok = settings.AgentEnrollmentSecret
		}
		data["InstallCommand"] = `curl -fsSL "` + base + `/agent/install.sh?token=` + tok + `" | sudo bash`
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	enrollPage.Execute(w, data)
}

// enrollProfileHandler serves the mobileconfig to authorized requests.
func (s *Server) enrollProfileHandler(w http.ResponseWriter, r *http.Request) {
	settings, err := s.st.GetSettings(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	token := r.URL.Query().Get("token")
	ok := settings.OpenEnrollment
	if !ok && token != "" {
		if _, err := s.st.CheckInvitation(r.Context(), store.HashToken(token)); err == nil || token == settings.AgentEnrollmentSecret {
			ok = true
		}
	}
	if !ok {
		http.Error(w, "invalid or expired enrollment token", http.StatusForbidden)
		return
	}
	b, err := s.enrollmentProfile(r.Context())
	if err != nil {
		http.Error(w, "enrollment is not fully configured: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="`+settings.OrgName+` Enrollment.mobileconfig"`)
	w.Write(b)
}
