package agentapi

import (
	"bufio"
	_ "embed"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
	"github.com/vibemdm/vibemdm/internal/webhook"
)

// Installomator (https://github.com/Installomator/Installomator) is vendored
// at a pinned release and served to agents. Apache License 2.0; the license
// text ships alongside in assets/INSTALLOMATOR_LICENSE.
//
//go:embed assets/Installomator.sh
var installomatorScript []byte

//go:embed assets/INSTALLOMATOR_LICENSE
var installomatorLicense []byte

var (
	labelOnce sync.Once
	labels    []string
	labelRe   = regexp.MustCompile(`^([a-z0-9][a-zA-Z0-9_-]*)\)$`)
)

// InstallomatorLabels parses the vendored script's case labels once.
func InstallomatorLabels() []string {
	labelOnce.Do(func() {
		seen := map[string]bool{}
		sc := bufio.NewScanner(strings.NewReader(string(installomatorScript)))
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if m := labelRe.FindStringSubmatch(line); m != nil {
				name := m[1]
				switch name {
				case "version", "valuesfromarguments", "longversion", "broken":
					continue
				}
				if !seen[name] {
					seen[name] = true
					labels = append(labels, name)
				}
			}
		}
		sort.Strings(labels)
	})
	return labels
}

func (h *Handler) registerParity(mux *http.ServeMux) {
	a := func(fn http.HandlerFunc) http.Handler { return h.requireDevice(fn) }
	mux.Handle("GET /agent/v1/installomator", a(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/x-shellscript")
		w.Write(installomatorScript)
	}))
	mux.HandleFunc("GET /agent/installomator-license", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(installomatorLicense)
	})
	mux.Handle("POST /agent/v1/app-installs/{id}/result", a(h.appInstallResult))
	mux.Handle("POST /agent/v1/restricted-events", a(h.restrictedEvent))
	mux.Handle("GET /agent/v1/selfservice", a(h.selfServiceList))
	mux.Handle("POST /agent/v1/selfservice/run", a(h.selfServiceRun))
}

func (h *Handler) appInstallResult(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var req struct {
		Status string `json:"status"`
		Log    string `json:"log"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if err := h.st.CompleteAppInstallLog(r.Context(), r.PathValue("id"), comp.ID, req.Status, req.Log); err != nil {
		writeErr(w, 404, "app install log not found")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) restrictedEvent(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var rep model.AgentRestrictedReport
	if err := decode(r, &rep); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if err := h.st.RecordRestrictedEvent(r.Context(), rep.RuleID, comp.ID, rep.RuleName, rep.Process, rep.Username, rep.Killed); err != nil {
		h.log.Error("recording restricted event", "err", err)
	}
	h.log.Info("restricted software violation", "computer", comp.Name, "rule", rep.RuleName, "process", rep.Process, "killed", rep.Killed)
	h.hooks.Fire(webhook.RestrictedSoftwareViolation, map[string]any{
		"computer_id": comp.ID, "computer": comp.Name, "serial": comp.SerialNumber,
		"rule": rep.RuleName, "process": rep.Process, "username": rep.Username, "killed": rep.Killed,
	})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// selfServiceList returns the Self Service items in scope for this computer.
func (h *Handler) selfServiceList(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	cache := store.NewMembershipCache()
	items := []model.SelfServiceItem{}
	policies, err := h.st.ListPolicies(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, p := range policies {
		if !p.Enabled || !p.SelfService.Enabled {
			continue
		}
		in, err := h.st.InScopeComputer(r.Context(), comp, p.Scope, cache)
		if err != nil || !in {
			continue
		}
		name := p.SelfService.DisplayName
		if name == "" {
			name = p.Name
		}
		items = append(items, model.SelfServiceItem{Kind: "policy", ID: p.ID, Name: name, Description: p.SelfService.Description, ButtonText: p.SelfService.ButtonText})
	}
	apps, err := h.st.ListMacApps(r.Context())
	if err == nil {
		for _, a := range apps {
			if !a.Enabled || !a.SelfService.Enabled {
				continue
			}
			in, err := h.st.InScopeComputer(r.Context(), comp, a.Scope, cache)
			if err != nil || !in {
				continue
			}
			name := a.SelfService.DisplayName
			if name == "" {
				name = a.Name
			}
			items = append(items, model.SelfServiceItem{Kind: "app", ID: a.ID, Name: name, Description: a.SelfService.Description, ButtonText: a.SelfService.ButtonText})
		}
	}
	writeJSON(w, 200, map[string]any{"items": items, "org_name": func() string { s, _ := h.st.GetSettings(r.Context()); return s.OrgName }()})
}

// selfServiceRun prepares a self-service item for immediate local execution.
func (h *Handler) selfServiceRun(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var req struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	switch req.Kind {
	case "policy":
		p, err := h.st.GetPolicy(r.Context(), req.ID)
		if err != nil || !p.Enabled || !p.SelfService.Enabled {
			writeErr(w, 404, "item not available")
			return
		}
		in, err := h.st.InScopeComputer(r.Context(), comp, p.Scope, nil)
		if err != nil || !in {
			writeErr(w, 403, "not in scope")
			return
		}
		ap, err := h.engine.Prepare(r.Context(), p, "self_service")
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		logID, err := h.st.CreatePendingPolicyLog(r.Context(), p.ID, comp.ID, "self_service")
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		ap.LogID = logID
		writeJSON(w, 200, map[string]any{"policy": ap})
	case "app":
		a, err := h.st.GetMacApp(r.Context(), req.ID)
		if err != nil || !a.Enabled || !a.SelfService.Enabled {
			writeErr(w, 404, "item not available")
			return
		}
		in, err := h.st.InScopeComputer(r.Context(), comp, a.Scope, nil)
		if err != nil || !in {
			writeErr(w, 403, "not in scope")
			return
		}
		logID, err := h.st.CreateAppInstallLog(r.Context(), a.ID, comp.ID, "self_service")
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"app": model.AgentAppInstall{LogID: logID, AppID: a.ID, Name: a.Name, Label: a.Label}})
	default:
		writeErr(w, 400, "unknown kind")
	}
}
