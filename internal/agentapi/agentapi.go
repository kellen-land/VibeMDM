// Package agentapi serves the API used by the macOS agent.
package agentapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/coder/websocket"

	"github.com/vibemdm/vibemdm/internal/appicons"
	"github.com/vibemdm/vibemdm/internal/assets"
	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/compliance"
	"github.com/vibemdm/vibemdm/internal/enroll"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/onboarding"
	"github.com/vibemdm/vibemdm/internal/policy"
	"github.com/vibemdm/vibemdm/internal/shell"
	"github.com/vibemdm/vibemdm/internal/store"
	"github.com/vibemdm/vibemdm/internal/webhook"
)

// EnrollmentProfileFunc renders the current enrollment profile.
type EnrollmentProfileFunc func(ctx context.Context) ([]byte, error)

// Handler serves agent endpoints.
type Handler struct {
	st        *store.Store
	engine    *policy.Engine
	notifier  *Notifier
	log       *slog.Logger
	dataDir   string
	icons     appicons.Dir
	assetsDir string
	agentDir  string
	profile   EnrollmentProfileFunc
	shellHub  *shell.Hub
	hooks     *webhook.Dispatcher
	version   string

	compliance *compliance.Service
	onboarding *onboarding.Service
	logLimit   *deviceLimiter

	// OnAgentEnrolled runs after a successful agent enrollment (removes the MDM bootstrap profile).
	OnAgentEnrolled func(ctx context.Context, comp *model.Computer)
}

// WithOnboarding enables white-glove onboarding.
func (h *Handler) WithOnboarding(svc *onboarding.Service) *Handler {
	h.onboarding = svc
	return h
}

// WithCompliance enables mSCP compliance result submission.
func (h *Handler) WithCompliance(svc *compliance.Service) *Handler {
	h.compliance = svc
	return h
}

// New creates an agent API handler.
func New(st *store.Store, engine *policy.Engine, notifier *Notifier, hub *shell.Hub, hooks *webhook.Dispatcher, dataDir, assetsDir, agentDir, version string, profile EnrollmentProfileFunc, logger *slog.Logger) *Handler {
	return &Handler{st: st, engine: engine, notifier: notifier, shellHub: hub, hooks: hooks, log: logger, dataDir: dataDir, assetsDir: assetsDir, icons: appicons.Dir{Path: filepath.Join(assetsDir, assets.AppIcons)}, agentDir: agentDir, profile: profile, version: version,
		logLimit: newDeviceLimiter(30, 6*time.Second)}
}

type ctxKey struct{}

func computerFrom(ctx context.Context) *model.Computer {
	c, _ := ctx.Value(ctxKey{}).(*model.Computer)
	return c
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	return dec.Decode(v)
}

// Register mounts routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// public
	mux.HandleFunc("POST /agent/v1/enroll", h.enroll)
	mux.HandleFunc("GET /agent/install.sh", h.installScript)
	mux.HandleFunc("GET /agent/download/{platform}", h.download)
	mux.HandleFunc("GET /agent/v1/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"version": h.version})
	})
	// authenticated
	a := func(fn http.HandlerFunc) http.Handler { return h.requireDevice(fn) }
	mux.Handle("POST /agent/v1/checkin", a(h.checkin))
	mux.Handle("GET /agent/v1/tasks", a(h.tasks))
	mux.Handle("POST /agent/v1/tasks/{id}/result", a(h.taskResult))
	mux.Handle("POST /agent/v1/policy-logs/{id}", a(h.policyLog))
	mux.Handle("POST /agent/v1/policy-logs/{id}/account-secrets", a(h.policyAccountSecrets))
	mux.Handle("GET /agent/v1/recon-config", a(h.reconConfig))
	mux.Handle("POST /agent/v1/inventory", a(h.inventory))
	mux.Handle("GET /agent/v1/packages/{id}", a(h.packageDownload))
	mux.Handle("GET /agent/v1/enrollment-profile", a(h.enrollmentProfile))
	mux.Handle("POST /agent/v1/unenroll", a(h.unenroll))
	mux.Handle("GET /agent/v1/shell", a(h.shell))
	mux.Handle("GET /agent/v1/laps", a(h.lapsConfig))
	mux.Handle("POST /agent/v1/laps/stage", a(h.lapsStage))
	mux.Handle("POST /agent/v1/laps/commit", a(h.lapsCommit))
	mux.Handle("POST /agent/v1/laps/error", a(h.lapsError))
	mux.Handle("POST /agent/v1/laps/state", a(h.lapsState))
	mux.Handle("POST /agent/v1/laps/token-grant", a(h.lapsTokenGrant))
	h.registerParity(mux)
}

// shell is the agent-side callback for a remote shell invite. The agent dials
// here after seeing a shell invite on its authenticated poll connection; this
// hands the connection to the waiting admin relay.
func (h *Handler) shell(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	sessionID := r.URL.Query().Get("session")
	nonce := r.URL.Query().Get("nonce")
	if sessionID == "" || nonce == "" {
		writeErr(w, 400, "session and nonce required")
		return
	}
	// The agent is not a browser; it is authenticated by its device token.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	done, err := h.shellHub.Claim(sessionID, comp.ID, nonce, shell.Adapt(c))
	if err != nil {
		h.log.Warn("agent shell claim rejected", "err", err, "computer", comp.Name)
		c.Close(websocket.StatusPolicyViolation, "claim rejected")
		return
	}
	// Block until the admin relay finishes; it owns closing this connection.
	select {
	case <-done:
	case <-r.Context().Done():
	}
}

func (h *Handler) requireDevice(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("Authorization")
		if !strings.HasPrefix(hdr, "Bearer ") {
			writeErr(w, http.StatusUnauthorized, "device token required")
			return
		}
		tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer "))
		comp, err := h.st.GetComputerByAgentToken(r.Context(), store.HashToken(tok))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid device token; re-enroll the agent")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, comp)))
	})
}

func (h *Handler) validateEnrollmentToken(ctx context.Context, token string, req model.AgentEnrollRequest) error {
	if token == "" {
		return errors.New("enrollment token required")
	}
	// A bootstrap token from the MDM-installed profile works once, and only for its own Mac.
	if strings.HasPrefix(token, "vmbt_") {
		if _, err := h.st.ConsumeBootstrapToken(ctx, store.HashToken(token), req.UDID, req.SerialNumber); err != nil {
			if errors.Is(err, store.ErrBootstrapDevice) {
				return err
			}
			var reused *store.BootstrapReusedError
			if errors.As(err, &reused) {
				// Fail closed: whoever enrolled first with this token loses its identity too.
				if uerr := h.st.UnenrollAgent(ctx, reused.ComputerID); uerr != nil {
					h.log.Error("revoking agent after bootstrap token reuse", "err", uerr, "computer", reused.ComputerID)
				}
				h.log.Warn("agent bootstrap token presented twice; agent identity revoked", "computer", reused.ComputerID)
				h.st.Audit(ctx, "agent", "agent.bootstrap_reused", "computer", reused.ComputerID, map[string]any{"udid": req.UDID, "serial": req.SerialNumber})
				return err
			}
			return errors.New("invalid or expired enrollment token")
		}
		return nil
	}
	settings, err := h.st.GetSettings(ctx)
	if err != nil {
		return err
	}
	if settings.AgentEnrollmentSecret != "" && subtle.ConstantTimeCompare([]byte(settings.AgentEnrollmentSecret), []byte(token)) == 1 {
		return nil
	}
	if _, err := h.st.ConsumeInvitation(ctx, store.HashToken(token)); err == nil {
		return nil
	}
	return errors.New("invalid or expired enrollment token")
}

func (h *Handler) enroll(w http.ResponseWriter, r *http.Request) {
	var req model.AgentEnrollRequest
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if err := h.validateEnrollmentToken(r.Context(), req.Token, req); err != nil {
		h.log.Warn("agent enroll rejected", "err", err, "serial", req.SerialNumber, "remote", r.RemoteAddr)
		writeErr(w, 403, err.Error())
		return
	}
	if req.UDID == "" && req.SerialNumber == "" {
		writeErr(w, 400, "udid or serial_number required")
		return
	}
	devTok := "vmda_" + auth.RandomToken(32)
	comp, err := h.st.EnrollAgent(r.Context(), req, store.HashToken(devTok))
	if errors.Is(err, store.ErrAgentActive) {
		h.log.Warn("agent enroll refused: computer already has an active agent", "serial", req.SerialNumber, "udid", req.UDID, "remote", auth.ClientIP(r))
		h.st.Audit(r.Context(), "agent", "agent.enroll_refused", "computer", "", map[string]any{"serial": req.SerialNumber, "udid": req.UDID, "ip": auth.ClientIP(r)})
		writeErr(w, 409, err.Error())
		return
	}
	if err != nil {
		h.log.Error("agent enroll", "err", err)
		writeErr(w, 500, "enrollment failed")
		return
	}
	settings, _ := h.st.GetSettings(r.Context())
	h.st.Audit(r.Context(), "agent", "agent.enroll", "computer", comp.ID, map[string]any{"serial": req.SerialNumber, "hostname": req.Hostname,
		"reinstall": req.PreviousDeviceToken != "", "ip": auth.ClientIP(r)})
	h.log.Info("agent enrolled", "computer", comp.Name, "serial", comp.SerialNumber, "id", comp.ID)
	h.hooks.Fire(webhook.AgentEnrolled, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "serial": comp.SerialNumber, "udid": comp.UDID})
	if h.OnAgentEnrolled != nil {
		go h.OnAgentEnrolled(context.WithoutCancel(r.Context()), comp)
	}
	writeJSON(w, 200, model.AgentEnrollResponse{
		ComputerID: comp.ID, DeviceToken: devTok, CheckinIntervalSeconds: settings.AgentCheckinIntervalSeconds,
		ServerURL: settings.ServerURL, OrgName: settings.OrgName, MDMEnrolled: comp.MDMEnrolled,
	})
}

func (h *Handler) checkin(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var req model.AgentCheckinRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if req.Trigger == "" {
		req.Trigger = "checkin"
	}
	if h.onboarding != nil {
		h.onboarding.MaybeStart(r.Context(), comp, req.ConsoleUser)
	}
	if err := h.st.TouchAgentCheckin(r.Context(), comp.ID, req); err != nil {
		h.log.Error("touch checkin", "err", err)
	}
	settings, _ := h.st.GetSettings(r.Context())
	resp := model.AgentCheckinResponse{
		ComputerID: comp.ID, ComputerName: comp.Name, Policies: []model.AgentPolicy{}, Tasks: []model.AgentTask{},
		CheckinIntervalSeconds: settings.AgentCheckinIntervalSeconds, ServerTime: time.Now().UTC(),
		SelfService: selfServiceConfig(settings),
	}
	// Enrollment-complete trigger fires once, on the first regular check-in.
	if req.Trigger == "checkin" && !comp.EnrollmentCompleteFired {
		pols, err := h.engine.ForTrigger(r.Context(), comp, "enrollment_complete")
		if err != nil {
			h.log.Error("policy eval", "err", err)
		} else {
			resp.Policies = append(resp.Policies, pols...)
			resp.FireEnrollmentComplete = true
		}
		h.st.MarkEnrollmentCompleteFired(r.Context(), comp.ID)
		resp.InventoryRequested = true
	}
	pols, err := h.engine.ForTrigger(r.Context(), comp, req.Trigger)
	if err != nil {
		h.log.Error("policy eval", "err", err)
		writeErr(w, 500, "policy evaluation failed")
		return
	}
	resp.Policies = append(resp.Policies, pols...)
	// App catalog installs due now (Installomator).
	if req.Trigger == "checkin" || req.Trigger == "startup" {
		cache := store.NewMembershipCache()
		if apps, err := h.st.ListMacApps(r.Context()); err == nil {
			for _, a := range apps {
				if !a.Enabled || a.Label == "" {
					continue
				}
				in, err := h.st.InScopeComputer(r.Context(), comp, a.Scope, cache)
				if err != nil || !in {
					continue
				}
				if a.Deployment == "self_service" {
					// Installed only from Self Service; "keep up to date" updates it where it is installed.
					if a.UpdateBehavior != "keep_updated" {
						continue
					}
					if has, err := h.st.AppInstalledOn(r.Context(), a.ID, comp.ID, a.Name); err != nil || !has {
						continue
					}
				}
				due, err := h.st.AppInstallDue(r.Context(), a.ID, comp.ID, a.UpdateBehavior)
				if err != nil || !due {
					continue
				}
				args, _, err := h.st.InstallomatorArgs(r.Context(), a.Label, a.Options)
				if err != nil {
					h.log.Warn("app install skipped", "app", a.Name, "err", err)
					continue
				}
				logID, err := h.st.CreateAppInstallLog(r.Context(), a.ID, comp.ID, req.Trigger)
				if err != nil {
					continue
				}
				resp.AppInstalls = append(resp.AppInstalls, model.AgentAppInstall{LogID: logID, AppID: a.ID, Name: a.Name, Label: a.Label, Args: args})
			}
		}
		// Restricted software rules for this computer.
		if rules, err := h.st.RestrictedForComputer(r.Context(), comp); err == nil {
			for _, ru := range rules {
				resp.RestrictedSoftware = append(resp.RestrictedSoftware, model.AgentRestrictedRule{
					ID: ru.ID, Name: ru.Name, ProcessName: ru.ProcessName, MatchType: ru.MatchType,
					KillProcess: ru.KillProcess, NotifyUser: ru.NotifyUser, DisplayMessage: ru.DisplayMessage,
				})
			}
		}
	}
	if resp.AppInstalls == nil {
		resp.AppInstalls = []model.AgentAppInstall{}
	}
	if resp.RestrictedSoftware == nil {
		resp.RestrictedSoftware = []model.AgentRestrictedRule{}
	}
	h.hooks.Fire(webhook.ComputerCheckIn, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "serial": comp.SerialNumber,
		"udid": comp.UDID, "trigger": req.Trigger, "user": req.ConsoleUser, "os_version": req.OSVersion, "ip": req.IPAddress})
	h.autoRemediateFileVault(r.Context(), comp, settings)
	tasks, err := h.st.ClaimQueuedTasks(r.Context(), comp.ID)
	if err != nil {
		h.log.Error("claim tasks", "err", err)
	} else {
		resp.Tasks = tasks
	}
	// Periodic inventory.
	if comp.LastInventoryAt == nil || time.Since(*comp.LastInventoryAt) > time.Duration(settings.InventoryIntervalHours)*time.Hour {
		resp.InventoryRequested = true
	}
	h.log.Debug("agent checkin", "computer", comp.Name, "trigger", req.Trigger, "policies", len(resp.Policies), "tasks", len(resp.Tasks))
	writeJSON(w, 200, resp)
}

// tasks long-polls for queued tasks.
func (h *Handler) tasks(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	if wait > 55 {
		wait = 55
	}
	respond := func() bool {
		tasks, err := h.st.ClaimQueuedTasks(r.Context(), comp.ID)
		if err != nil {
			h.log.Error("claim tasks", "err", err)
		}
		if tasks == nil {
			tasks = []model.AgentTask{}
		}
		checkin := h.notifier.TakeCheckin(comp.ID)
		shells := h.notifier.TakeShellInvites(comp.ID)
		if len(tasks) == 0 && !checkin && len(shells) == 0 {
			return false
		}
		writeJSON(w, 200, model.AgentPollResponse{Tasks: tasks, Checkin: checkin, Shells: shells})
		return true
	}
	if respond() {
		return
	}
	if wait <= 0 {
		writeJSON(w, 200, model.AgentPollResponse{Tasks: []model.AgentTask{}})
		return
	}
	ch, unsub := h.notifier.Subscribe(comp.ID)
	defer unsub()
	h.st.DB.ExecContext(r.Context(), `UPDATE computers SET agent_last_checkin = now() WHERE id = $1`, comp.ID)
	select {
	case <-ch:
	case <-time.After(time.Duration(wait) * time.Second):
	case <-r.Context().Done():
		return
	}
	if !respond() {
		writeJSON(w, 200, model.AgentPollResponse{Tasks: []model.AgentTask{}})
	}
}

func (h *Handler) taskResult(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var res model.AgentTaskResult
	if err := decode(r, &res); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if err := h.st.CompleteTask(r.Context(), r.PathValue("id"), comp.ID, res); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "task not found")
			return
		}
		h.log.Error("storing a task result", "err", err, "computer", comp.Name, "task_id", r.PathValue("id"))
		writeErr(w, 500, "could not store the task result")
		return
	}
	if res.Status == "failed" {
		// A compliance scan whose task failed before submitting results is closed here.
		h.st.FailComplianceScanByTask(r.Context(), r.PathValue("id"), firstNonEmptyStr(res.Error, res.Stderr, "agent task failed"))
		h.st.FailOnboardingRunByTask(r.Context(), r.PathValue("id"), firstNonEmptyStr(res.Error, res.Stderr, "agent task failed"))
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// policyAccountSecrets gives the agent the Local Accounts passwords of a policy it is running.
// They are never part of the policy itself (which is stored with tasks), and are only released
// while that computer's run of the policy is still open.
func (h *Handler) policyAccountSecrets(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	policyID, err := h.st.PendingPolicyLog(r.Context(), r.PathValue("id"), comp.ID)
	if err != nil {
		writeErr(w, 404, "no open run of this policy on this computer")
		return
	}
	pws, err := h.st.PolicyAccountPasswords(r.Context(), policyID)
	if err != nil {
		h.log.Error("reading policy account passwords", "err", err, "policy_id", policyID)
		writeErr(w, 500, "could not read the account passwords")
		return
	}
	out := map[string]any{"passwords": pws}
	// Granting a Secure Token or resetting a Secure Token account needs a Secure Token
	// administrator: the LAPS account (Jamf Pro uses its management account the same way).
	adminSent := false
	if pol, err := h.st.GetPolicy(r.Context(), policyID); err == nil && needsTokenAdmin(pol.Accounts) {
		if user, pw, err := h.st.LAPSAdminCredentials(r.Context(), comp.ID); err == nil {
			out["admin_user"], out["admin_password"] = user, pw
			adminSent = true
		}
	}
	h.st.Audit(r.Context(), "agent:"+comp.Name, "policy.account_passwords_sent", "policy", policyID,
		map[string]any{"computer_id": comp.ID, "log_id": r.PathValue("id"), "laps_admin": adminSent})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, out)
}

// needsTokenAdmin reports whether Local Accounts actions may need a Secure Token administrator.
func needsTokenAdmin(accounts []model.PolicyAccount) bool {
	for _, a := range accounts {
		if a.Action == "reset_password" || (a.Action == "create" && a.EnableFileVault) {
			return true
		}
	}
	return false
}

func (h *Handler) policyLog(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var sub model.AgentPolicyLogSubmit
	if err := decode(r, &sub); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if err := h.st.CompletePolicyLog(r.Context(), r.PathValue("id"), comp.ID, sub); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "policy log not found")
			return
		}
		h.log.Error("storing a policy log", "err", err, "computer", comp.Name, "log_id", r.PathValue("id"))
		writeErr(w, 500, "could not store the policy log")
		return
	}
	ev := webhook.PolicyCompleted
	if sub.Status != "completed" {
		ev = webhook.PolicyFailed
	}
	h.hooks.Fire(ev, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "log_id": r.PathValue("id"), "trigger": sub.Trigger, "status": sub.Status})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) reconConfig(w http.ResponseWriter, r *http.Request) {
	eas, err := h.st.ListExtensionAttributes(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := []model.ExtensionAttribute{}
	for _, e := range eas {
		if e.Enabled && e.InputType == "script" && strings.TrimSpace(e.Script) != "" {
			out = append(out, e)
		}
	}
	writeJSON(w, 200, model.AgentReconConfig{ExtensionAttributes: out})
}

func (h *Handler) inventory(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	var inv model.AgentInventory
	if err := decode(r, &inv); err != nil {
		writeErr(w, 400, "invalid JSON: "+err.Error())
		return
	}
	if err := h.st.ApplyAgentInventory(r.Context(), comp.ID, inv); err != nil {
		h.log.Error("apply inventory", "err", err, "computer", comp.Name)
		writeErr(w, 500, "storing inventory failed")
		return
	}
	h.log.Info("inventory received", "computer", comp.Name, "apps", len(inv.Apps))
	h.hooks.Fire(webhook.ComputerInventoryCompleted, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "serial": comp.SerialNumber, "apps": len(inv.Apps), "os_version": inv.OSVersion})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) packageDownload(w http.ResponseWriter, r *http.Request) {
	pkg, err := h.st.GetPackage(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "package not found")
		return
	}
	path := filepath.Join(h.assetsDir, assets.Packages, filepath.Base(pkg.Filename))
	if _, err := os.Stat(path); err != nil {
		writeErr(w, 404, "package file missing on server")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(pkg.Filename)))
	http.ServeFile(w, r, path)
}

func (h *Handler) enrollmentProfile(w http.ResponseWriter, r *http.Request) {
	b, err := h.profile(enroll.WithPurpose(r.Context(), enroll.PurposeAgent))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="enroll.mobileconfig"`)
	w.Write(b)
}

func (h *Handler) unenroll(w http.ResponseWriter, r *http.Request) {
	comp := computerFrom(r.Context())
	h.st.UnenrollAgent(r.Context(), comp.ID)
	h.st.Audit(r.Context(), "agent", "agent.unenroll", "computer", comp.ID, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	platform := r.PathValue("platform")
	switch platform {
	case "darwin-arm64", "darwin-amd64":
	default:
		writeErr(w, 404, "unknown platform")
		return
	}
	path := filepath.Join(h.agentDir, "vibemdm-agent-"+platform)
	if _, err := os.Stat(path); err != nil {
		writeErr(w, 404, "agent binary not available on this server (build with `make agent`)")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="vibemdm-agent"`)
	http.ServeFile(w, r, path)
}

var installTemplate = template.Must(template.New("install").Parse(`#!/bin/bash
# VibeMDM agent installer — generated by {{.ServerURL}}
set -euo pipefail
if [ "$(id -u)" -ne 0 ]; then echo "Run with sudo: curl -fsSL '{{.ServerURL}}/agent/install.sh?token=...' | sudo bash"; exit 1; fi
SERVER="{{.ServerURL}}"
TOKEN="{{.Token}}"
ARCH="$(uname -m)"
case "$ARCH" in
  arm64) PLATFORM="darwin-arm64" ;;
  x86_64) PLATFORM="darwin-amd64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac
BIN_DIR="/usr/local/bin"
mkdir -p "$BIN_DIR"
echo "Downloading VibeMDM agent ($PLATFORM)..."
curl -fsSL "$SERVER/agent/download/$PLATFORM" -o "$BIN_DIR/vibemdm-agent.tmp"
mv "$BIN_DIR/vibemdm-agent.tmp" "$BIN_DIR/vibemdm-agent"
chmod 755 "$BIN_DIR/vibemdm-agent"
xattr -d com.apple.quarantine "$BIN_DIR/vibemdm-agent" 2>/dev/null || true
echo "Enrolling with $SERVER..."
VIBEMDM_ENROLL_TOKEN="$TOKEN" "$BIN_DIR/vibemdm-agent" enroll --server "$SERVER" {{if .MDM}}--mdm{{end}} {{if .Insecure}}--allow-insecure-http{{end}}
echo "Done. The VibeMDM agent is installed and running."
`))

func (h *Handler) installScript(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	settings, err := h.st.GetSettings(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// Never derive the URL from the Host header (it would be reflected into the script).
	serverURL := settings.ServerURL
	if serverURL == "" {
		writeErr(w, 503, "the server URL is not configured (Settings → General)")
		return
	}
	if token == "" {
		writeErr(w, 400, "token query parameter required")
		return
	}
	// Validate the token without consuming it so the script fails fast when it is wrong.
	validSecret := settings.AgentEnrollmentSecret != "" && subtle.ConstantTimeCompare([]byte(settings.AgentEnrollmentSecret), []byte(token)) == 1
	if !validSecret {
		if _, err := h.st.CheckInvitation(r.Context(), store.HashToken(token)); err != nil {
			writeErr(w, 403, "invalid or expired enrollment token")
			return
		}
	}
	w.Header().Set("Content-Type", "text/x-shellscript")
	installTemplate.Execute(w, map[string]any{"ServerURL": serverURL, "Token": token, "MDM": r.URL.Query().Get("mdm") != "0",
		"Insecure": !strings.HasPrefix(serverURL, "https://")})
}

// autoRemediateFileVault queues Escrow Buddy when FileVault is on, the escrow
// profile is installed, and no key has been escrowed (at most once a day).
func (h *Handler) autoRemediateFileVault(ctx context.Context, comp *model.Computer, settings model.Settings) {
	if !settings.FileVaultAutoRemediate || comp.FileVaultEnabled == nil || !*comp.FileVaultEnabled {
		return
	}
	st, err := h.st.FileVaultStatusFor(ctx, comp.ID)
	if err != nil || st.Escrowed || st.EscrowProfileStatus != "installed" {
		return
	}
	if recent, err := h.st.HasRecentTask(ctx, comp.ID, "filevault_regenerate", 24*time.Hour); err != nil || recent {
		return
	}
	if _, err := h.st.CreateTask(ctx, comp.ID, "filevault_regenerate", map[string]any{"reason": "auto-remediation: no escrowed FileVault recovery key"}, "system"); err == nil {
		h.log.Info("queued FileVault key remediation (Escrow Buddy)", "computer", comp.Name)
		h.st.Audit(ctx, "system", "filevault.auto_remediate", "computer", comp.ID, nil)
	}
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
