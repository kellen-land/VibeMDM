package agentapi

import (
	"bufio"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/onboarding"
	"github.com/vibemdm/vibemdm/internal/policy"
	"github.com/vibemdm/vibemdm/internal/selfservice"
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
	mux.Handle("POST /agent/v1/compliance-scans/{id}", a(h.complianceResult))
	mux.Handle("POST /agent/v1/onboarding/{id}/user-entry", a(h.onboardingUserEntry))
	mux.Handle("POST /agent/v1/onboarding/{id}", a(h.onboardingResult))
	mux.Handle("POST /agent/v1/restricted-events", a(h.restrictedEvent))
	mux.Handle("POST /agent/v1/logs", a(h.ingestLogs))
	mux.Handle("PUT /agent/v1/diagnostics/{id}", a(h.uploadDiagnostics))
	mux.Handle("POST /agent/v1/diagnostics/{id}/failed", a(h.diagnosticsFailed))
	mux.Handle("GET /agent/v1/selfservice", a(h.selfServiceList))
	mux.Handle("GET /agent/v1/app-icons/{id}", a(h.appIcon))
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

func (h *Handler) complianceResult(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	if h.compliance == nil {
		writeErr(w, 503, "compliance is not available")
		return
	}
	var sub model.AgentComplianceSubmit
	if err := decode(r, &sub); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	scan, err := h.compliance.Submit(r.Context(), r.PathValue("id"), comp, sub)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "compliance scan not found or already completed")
			return
		}
		h.log.Error("storing compliance results", "computer", comp.Name, "err", err)
		writeErr(w, 500, "storing results failed")
		return
	}
	h.log.Info("compliance scan stored", "computer", comp.Name, "passed", scan.Passed, "failed", scan.Failed, "remediated", scan.Remediated, "errors", scan.Errors)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// onboardingUserEntry stores what the user typed in the welcome dialog and
// applies it to the computer record (name, asset tag, building, ...).
func (h *Handler) onboardingUserEntry(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var req struct {
		UserEntry map[string]string `json:"user_entry"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	clean := map[string]string{}
	for k, v := range req.UserEntry {
		if _, ok := onboarding.FieldKeys[k]; ok && len(v) <= 256 {
			clean[k] = v
		}
	}
	if err := h.st.StartOnboardingRun(r.Context(), r.PathValue("id"), comp.ID, clean); err != nil {
		writeErr(w, 404, "onboarding run not found")
		return
	}
	if h.onboarding != nil {
		if err := h.onboarding.ApplyUserEntry(r.Context(), comp, clean); err != nil {
			h.log.Warn("applying onboarding user entry", "computer", comp.Name, "err", err)
		}
	}
	h.st.Audit(r.Context(), "agent", "onboarding.user_entry", "computer", comp.ID, map[string]any{"fields": len(clean)})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) onboardingResult(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var sub model.AgentOnboardingSubmit
	if err := decode(r, &sub); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if err := h.st.CompleteOnboardingRun(r.Context(), r.PathValue("id"), comp.ID, sub); err != nil {
		writeErr(w, 404, "onboarding run not found or already finished")
		return
	}
	h.log.Info("onboarding finished", "computer", comp.Name, "status", sub.Status, "steps", len(sub.Steps))
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
	icons := h.icons.Versions()
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
		items = append(items, model.SelfServiceItem{Kind: "policy", ID: p.ID, Name: name, Description: p.SelfService.Description, ButtonText: p.SelfService.ButtonText,
			Category: firstNonEmptyStr(p.SelfService.Category, p.Category), Featured: p.SelfService.Featured})
	}
	apps, err := h.st.ListMacApps(r.Context())
	if err == nil {
		for _, a := range apps {
			if !a.Enabled || !store.SelfServiceVisible(a) {
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
			items = append(items, model.SelfServiceItem{Kind: "app", ID: a.ID, Name: name, Description: firstNonEmptyStr(a.SelfService.Description, a.Description),
				ButtonText: a.SelfService.ButtonText, Category: firstNonEmptyStr(a.SelfService.Category, a.Category), Featured: a.SelfService.Featured, AppName: a.Name,
				IconVersion: icons[a.ID]})
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
		ap, err := h.engine.PrepareFor(r.Context(), p, comp, "self_service")
		if err != nil {
			var tooOld *policy.ErrAgentTooOld
			if errors.As(err, &tooOld) {
				writeErr(w, 409, err.Error())
				return
			}
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
		if err != nil || !a.Enabled || !store.SelfServiceVisible(*a) {
			writeErr(w, 404, "item not available")
			return
		}
		in, err := h.st.InScopeComputer(r.Context(), comp, a.Scope, nil)
		if err != nil || !in {
			writeErr(w, 403, "not in scope")
			return
		}
		args, _, err := h.st.InstallomatorArgs(r.Context(), a.Label, a.Options)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		logID, err := h.st.CreateAppInstallLog(r.Context(), a.ID, comp.ID, "self_service")
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"app": model.AgentAppInstall{LogID: logID, AppID: a.ID, Name: a.Name, Label: a.Label, Args: args}})
	default:
		writeErr(w, 400, "unknown kind")
	}
}

func (h *Handler) ingestLogs(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var req struct {
		Entries []model.AgentLogEntry `json:"entries"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	n, err := h.st.InsertAgentLogs(r.Context(), comp.ID, req.Entries)
	if err != nil {
		h.log.Error("storing agent logs", "err", err, "computer", comp.Name)
		writeErr(w, 500, "storing logs failed")
		return
	}
	writeJSON(w, 200, map[string]int{"stored": n})
}

// uploadDiagnostics receives a gzipped tarball for a requested bundle (max 512 MiB).
func (h *Handler) uploadDiagnostics(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	b, err := h.st.GetDiagnosticBundle(r.Context(), r.PathValue("id"))
	if err != nil || b.ComputerID != comp.ID || b.Status != "requested" {
		writeErr(w, 404, "diagnostic bundle not found")
		return
	}
	dir := filepath.Join(h.dataDir, "diagnostics")
	os.MkdirAll(dir, 0o750)
	dst := filepath.Join(dir, b.ID+".tar.gz")
	f, err := os.OpenFile(dst+".part", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		writeErr(w, 500, "cannot store bundle")
		return
	}
	hsh := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hsh), http.MaxBytesReader(w, r.Body, 512<<20))
	f.Close()
	if err != nil {
		os.Remove(dst + ".part")
		writeErr(w, 400, "upload failed: "+err.Error())
		return
	}
	os.Rename(dst+".part", dst)
	name := fmt.Sprintf("vibemdm-diagnostics-%s-%s.tar.gz", sanitizeName(comp.Name), time.Now().UTC().Format("20060102-150405"))
	if err := h.st.CompleteDiagnosticBundle(r.Context(), b.ID, comp.ID, "uploaded", name, hex.EncodeToString(hsh.Sum(nil)), n, ""); err != nil {
		writeErr(w, 409, "bundle no longer pending")
		return
	}
	h.log.Info("diagnostic bundle uploaded", "computer", comp.Name, "bytes", n)
	writeJSON(w, 200, map[string]any{"ok": true, "bytes": n})
}

func (h *Handler) diagnosticsFailed(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var req struct {
		Error string `json:"error"`
	}
	decode(r, &req)
	if err := h.st.CompleteDiagnosticBundle(r.Context(), r.PathValue("id"), comp.ID, "failed", "", "", 0, req.Error); err != nil {
		writeErr(w, 404, "diagnostic bundle not found")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func sanitizeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out = append(out, r)
		} else {
			out = append(out, '-')
		}
	}
	return string(out)
}

// selfServiceConfig is the Self Service app configuration sent to agents.
func selfServiceConfig(s model.Settings) *model.AgentSelfService {
	return &model.AgentSelfService{Enabled: !s.SelfServiceAppDisabled, AppName: selfservice.AppName(s.SelfServiceAppName, s.OrgName), OrgName: s.OrgName, Accent: s.SelfServiceAccentColor, Welcome: s.SelfServiceWelcome}
}

// appIcon serves a Mac App's uploaded icon to agents (Self Service and notifications).
func (h *Handler) appIcon(w http.ResponseWriter, r *http.Request) {
	b, v, err := h.icons.Read(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "no icon")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Icon-Version", v)
	w.Write(b)
}
