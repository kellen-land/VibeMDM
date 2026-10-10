// Package api implements the admin REST API consumed by the web UI.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/agentapi"
	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/config"
	"github.com/vibemdm/vibemdm/internal/escrow"
	"github.com/vibemdm/vibemdm/internal/mdmsvc"
	"github.com/vibemdm/vibemdm/internal/policy"
	"github.com/vibemdm/vibemdm/internal/scepsrv"
	"github.com/vibemdm/vibemdm/internal/shell"
	"github.com/vibemdm/vibemdm/internal/store"
	"github.com/vibemdm/vibemdm/internal/webhook"

	"github.com/micromdm/nanomdm/storage"
)

// Handler holds dependencies for admin API handlers.
type Handler struct {
	cfg       *config.Config
	st        *store.Store
	auth      *auth.Authenticator
	cmdr      *mdmsvc.Commander
	engine    *policy.Engine
	notifier  *agentapi.Notifier
	scep      *scepsrv.Server
	pushStore interface {
		storage.PushCertStore
		storage.PushCertStorer
	}
	profile  func(ctx context.Context) ([]byte, error)
	shellHub *shell.Hub
	hooks    *webhook.Dispatcher
	escrow   *escrow.Identity
	log      *slog.Logger
}

// Deps are the handler dependencies.
type Deps struct {
	Cfg       *config.Config
	Store     *store.Store
	Auth      *auth.Authenticator
	Commander *mdmsvc.Commander
	Engine    *policy.Engine
	Notifier  *agentapi.Notifier
	SCEP      *scepsrv.Server
	PushStore interface {
		storage.PushCertStore
		storage.PushCertStorer
	}
	Profile  func(ctx context.Context) ([]byte, error)
	ShellHub *shell.Hub
	Hooks    *webhook.Dispatcher
	Escrow   *escrow.Identity
	Log      *slog.Logger
}

// New creates the handler.
func New(d Deps) *Handler {
	return &Handler{cfg: d.Cfg, st: d.Store, auth: d.Auth, cmdr: d.Commander, engine: d.Engine, notifier: d.Notifier, scep: d.SCEP, pushStore: d.PushStore, profile: d.Profile, shellHub: d.ShellHub, hooks: d.Hooks, escrow: d.Escrow, log: d.Log}
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (h *Handler) fail(w http.ResponseWriter, err error, what string) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, what+" not found")
		return
	}
	if errors.Is(err, mdmsvc.ErrNotEnrolled) {
		writeErr(w, 409, err.Error())
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "duplicate key") {
		writeErr(w, 409, "a record with that name already exists")
		return
	}
	if strings.Contains(msg, "violates") || strings.Contains(msg, "invalid input syntax") {
		writeErr(w, 400, msg)
		return
	}
	h.log.Error(what, "err", err)
	writeErr(w, 500, msg)
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<20))
	if err := dec.Decode(v); err != nil {
		return errors.New("invalid JSON body: " + err.Error())
	}
	return nil
}

func actor(r *http.Request) string {
	if p := auth.FromContext(r.Context()); p != nil {
		return p.Username
	}
	return ""
}

func qInt(r *http.Request, key string, def int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return def
	}
	return v
}

type listResp[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// Register mounts all admin routes under /api/v1.
func (h *Handler) Register(mux *http.ServeMux) {
	// auth (public)
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.HandleFunc("POST /api/v1/auth/logout", h.logout)
	mux.HandleFunc("GET /api/v1/auth/me", h.me)
	mux.HandleFunc("GET /api/v1/status", h.publicStatus)

	// everything else requires a session/token; viewers may read, admins may write.
	r := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, h.auth.Require(false, auth.RequireWrite(fn)))
	}
	adm := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, h.auth.Require(true, fn))
	}

	r("GET /api/v1/dashboard", h.dashboard)

	r("GET /api/v1/computers", h.listComputers)
	r("GET /api/v1/computers/{id}", h.getComputer)
	r("PATCH /api/v1/computers/{id}", h.updateComputer)
	r("DELETE /api/v1/computers/{id}", h.deleteComputer)
	r("GET /api/v1/computers/{id}/commands", h.computerCommands)
	r("GET /api/v1/computers/{id}/policy-logs", h.computerPolicyLogs)
	r("GET /api/v1/computers/{id}/tasks", h.computerTasks)
	r("GET /api/v1/computers/{id}/inventory", h.computerInventory)
	r("POST /api/v1/computers/{id}/commands", h.sendCommand)
	r("POST /api/v1/computers/{id}/tasks", h.createTask)
	r("POST /api/v1/computers/{id}/push", h.pushComputer)
	r("POST /api/v1/computers/{id}/update-inventory", h.updateInventory)
	r("POST /api/v1/computers/{id}/checkin", h.checkinNow)
	mux.Handle("GET /api/v1/computers/{id}/shell", h.auth.Require(true, http.HandlerFunc(h.shellWS)))
	adm("GET /api/v1/shell-sessions", h.listShellSessions)
	adm("GET /api/v1/shell-sessions/{id}", h.getShellTranscript)
	r("POST /api/v1/computers/{id}/ea-values/{ea}", h.setEAValue)

	// Network organization (unified scoping vocabulary)
	r("GET /api/v1/buildings", h.listBuildings)
	r("GET /api/v1/departments", h.listDepartments)
	{
		c, ren, del := h.namedCRUD("buildings", "building", "building")
		r("POST /api/v1/buildings", c)
		r("PUT /api/v1/buildings/{id}", ren)
		r("DELETE /api/v1/buildings/{id}", del)
	}
	{
		c, ren, del := h.namedCRUD("departments", "department", "department")
		r("POST /api/v1/departments", c)
		r("PUT /api/v1/departments/{id}", ren)
		r("DELETE /api/v1/departments/{id}", del)
	}
	r("GET /api/v1/network-segments", h.listSegments)
	r("POST /api/v1/network-segments", h.createSegment)
	r("PUT /api/v1/network-segments/{id}", h.updateSegment)
	r("DELETE /api/v1/network-segments/{id}", h.deleteSegment)
	r("POST /api/v1/scope/preview", h.scopePreview)

	// Software updates (Declarative Device Management)
	r("GET /api/v1/updates/plans", h.listPlans)
	r("POST /api/v1/updates/plans", h.savePlan)
	r("GET /api/v1/updates/plans/{id}", h.getPlan)
	r("PUT /api/v1/updates/plans/{id}", h.savePlan)
	r("DELETE /api/v1/updates/plans/{id}", h.deletePlan)
	r("GET /api/v1/updates/overview", h.updatesOverview)
	r("GET /api/v1/updates/catalog", h.updatesCatalog)
	r("GET /api/v1/computers/{id}/ddm", h.computerDDM)
	r("POST /api/v1/computers/{id}/ddm/sync", h.syncComputerDDM)

	// Logs & diagnostics
	r("GET /api/v1/logs", h.listLogs)
	r("GET /api/v1/logs/components", h.logComponents)
	adm("GET /api/v1/logs/export", h.exportLogs)
	adm("GET /api/v1/logs/archives", h.listLogArchives)
	adm("GET /api/v1/logs/archives/{name}", h.downloadLogArchive)
	adm("POST /api/v1/computers/{id}/diagnostics", h.requestDiagnostics)
	r("GET /api/v1/computers/{id}/diagnostics", h.listDiagnostics)
	adm("GET /api/v1/diagnostics/{id}/download", h.downloadDiagnostics)

	// FileVault recovery key escrow
	r("GET /api/v1/filevault", h.filevaultOverview)
	adm("PUT /api/v1/filevault", h.filevaultProfiles)
	adm("POST /api/v1/filevault/check", h.checkMissingKeys)
	r("GET /api/v1/computers/{id}/filevault", h.computerFileVault)
	adm("POST /api/v1/computers/{id}/filevault/reveal", h.revealFileVaultKey)
	adm("POST /api/v1/computers/{id}/filevault/remediate", h.remediateFileVault)

	// Jamf-parity: restricted software, advanced searches, app catalog, webhooks
	r("GET /api/v1/restricted-software", h.listRestricted)
	r("POST /api/v1/restricted-software", h.createRestricted)
	r("GET /api/v1/restricted-software/events", h.restrictedEvents)
	r("GET /api/v1/restricted-software/{id}", h.getRestricted)
	r("PUT /api/v1/restricted-software/{id}", h.updateRestricted)
	r("DELETE /api/v1/restricted-software/{id}", h.deleteRestricted)

	r("GET /api/v1/advanced-searches", h.listSearches)
	r("POST /api/v1/advanced-searches", h.createSearch)
	r("GET /api/v1/advanced-searches/fields", h.searchFieldOptions)
	r("GET /api/v1/advanced-searches/{id}", h.getSearch)
	r("PUT /api/v1/advanced-searches/{id}", h.updateSearch)
	r("DELETE /api/v1/advanced-searches/{id}", h.deleteSearch)
	r("GET /api/v1/advanced-searches/{id}/run", h.runSearch)
	r("GET /api/v1/advanced-searches/{id}/export.csv", h.exportSearchCSV)
	r("GET /api/v1/computers-export.csv", h.exportComputersCSV)

	r("GET /api/v1/apps", h.listApps)
	r("POST /api/v1/apps", h.createApp)
	r("GET /api/v1/apps/labels", h.appLabels)
	r("GET /api/v1/custom-labels", h.listCustomLabels)
	r("POST /api/v1/custom-labels", h.saveCustomLabel)
	r("GET /api/v1/custom-labels/{id}", h.getCustomLabel)
	r("PUT /api/v1/custom-labels/{id}", h.saveCustomLabel)
	r("DELETE /api/v1/custom-labels/{id}", h.deleteCustomLabel)
	r("POST /api/v1/apps/installomator-preview", h.previewInstallomatorArgs)
	r("GET /api/v1/apps/logs", h.allAppLogs)
	r("GET /api/v1/apps/{id}", h.getApp)
	r("PUT /api/v1/apps/{id}", h.updateApp)
	r("DELETE /api/v1/apps/{id}", h.deleteApp)
	r("GET /api/v1/apps/{id}/logs", h.appLogs)

	adm("GET /api/v1/webhooks", h.listWebhooks)
	adm("POST /api/v1/webhooks", h.createWebhook)
	adm("POST /api/v1/webhooks/test", h.testWebhook)
	adm("GET /api/v1/webhooks/deliveries", h.webhookDeliveries)
	adm("PUT /api/v1/webhooks/{id}", h.updateWebhook)
	adm("DELETE /api/v1/webhooks/{id}", h.deleteWebhook)

	r("GET /api/v1/groups", h.listGroups)
	r("POST /api/v1/groups", h.createGroup)
	r("GET /api/v1/groups/criteria-fields", h.criteriaFields)
	r("POST /api/v1/groups/preview", h.previewGroup)
	r("GET /api/v1/groups/{id}", h.getGroup)
	r("PUT /api/v1/groups/{id}", h.updateGroup)
	r("DELETE /api/v1/groups/{id}", h.deleteGroup)
	r("GET /api/v1/groups/{id}/members", h.groupMembers)
	r("PUT /api/v1/groups/{id}/members", h.setGroupMembers)

	r("GET /api/v1/scripts", h.listScripts)
	r("POST /api/v1/scripts", h.createScript)
	r("GET /api/v1/scripts/{id}", h.getScript)
	r("PUT /api/v1/scripts/{id}", h.updateScript)
	r("DELETE /api/v1/scripts/{id}", h.deleteScript)
	r("POST /api/v1/scripts/{id}/run", h.runScript)

	r("GET /api/v1/packages", h.listPackages)
	r("POST /api/v1/packages", h.uploadPackage)
	r("GET /api/v1/packages/{id}", h.getPackage)
	r("PUT /api/v1/packages/{id}", h.updatePackage)
	r("DELETE /api/v1/packages/{id}", h.deletePackage)
	r("GET /api/v1/packages/{id}/download", h.downloadPackage)

	r("GET /api/v1/extension-attributes", h.listEAs)
	r("POST /api/v1/extension-attributes", h.createEA)
	r("GET /api/v1/extension-attributes/{id}", h.getEA)
	r("PUT /api/v1/extension-attributes/{id}", h.updateEA)
	r("DELETE /api/v1/extension-attributes/{id}", h.deleteEA)

	r("GET /api/v1/policies", h.listPolicies)
	r("POST /api/v1/policies", h.createPolicy)
	r("GET /api/v1/policies/{id}", h.getPolicy)
	r("PUT /api/v1/policies/{id}", h.updatePolicy)
	r("DELETE /api/v1/policies/{id}", h.deletePolicy)
	r("GET /api/v1/policies/{id}/logs", h.policyLogs)
	r("POST /api/v1/policies/{id}/flush", h.flushPolicy)
	r("POST /api/v1/policies/{id}/run", h.runPolicy)
	r("GET /api/v1/policy-logs", h.allPolicyLogs)
	r("GET /api/v1/policy-logs/{id}", h.getPolicyLog)

	r("GET /api/v1/profiles", h.listProfiles)
	r("POST /api/v1/profiles", h.createProfile)
	r("GET /api/v1/profiles/{id}", h.getProfile)
	r("PUT /api/v1/profiles/{id}", h.updateProfile)
	r("DELETE /api/v1/profiles/{id}", h.deleteProfile)
	r("GET /api/v1/profiles/{id}/deployments", h.profileDeployments)
	r("POST /api/v1/profiles/{id}/deploy", h.deployProfile)
	r("POST /api/v1/profiles/{id}/remove", h.removeProfileFrom)

	r("GET /api/v1/commands", h.listCommands)
	r("GET /api/v1/commands/search", h.searchCommands)
	r("GET /api/v1/commands/{uuid}", h.getCommand)
	r("GET /api/v1/commands/{uuid}/inspect", h.inspectCommand)
	r("GET /api/v1/computers/{id}/timeline", h.computerTimeline)
	r("DELETE /api/v1/commands/{uuid}", h.cancelCommand)
	r("GET /api/v1/tasks", h.listTasks)
	r("GET /api/v1/tasks/{id}", h.getTask)
	r("POST /api/v1/tasks/{id}/cancel", h.cancelTask)

	r("GET /api/v1/enrollment/status", h.enrollmentStatus)
	r("GET /api/v1/enrollment/profile", h.downloadEnrollmentProfile)
	r("GET /api/v1/enrollment/invitations", h.listInvitations)
	r("POST /api/v1/enrollment/invitations", h.createInvitation)
	r("PATCH /api/v1/enrollment/invitations/{id}", h.updateInvitation)
	r("DELETE /api/v1/enrollment/invitations/{id}", h.deleteInvitation)

	r("GET /api/v1/settings", h.getSettings)
	adm("PUT /api/v1/settings", h.updateSettings)
	adm("POST /api/v1/settings/rotate-enrollment-secret", h.rotateEnrollmentSecret)
	r("GET /api/v1/settings/apns", h.apnsStatus)
	adm("PUT /api/v1/settings/apns", h.apnsUpload)
	adm("POST /api/v1/settings/apns/request", h.apnsNewRequest)
	adm("POST /api/v1/settings/apns/decrypt", h.apnsDecrypt)
	adm("PUT /api/v1/settings/apns/certificate", h.apnsCompleteCertificate)
	r("GET /api/v1/settings/scep", h.scepStatus)
	adm("POST /api/v1/settings/scep/regenerate", h.scepRegenerate)

	adm("GET /api/v1/users", h.listUsers)
	adm("POST /api/v1/users", h.createUser)
	adm("PUT /api/v1/users/{id}", h.updateUser)
	adm("DELETE /api/v1/users/{id}", h.deleteUser)
	adm("POST /api/v1/users/{id}/password", h.setUserPassword)
	r("POST /api/v1/auth/password", h.changeOwnPassword)
	adm("GET /api/v1/api-tokens", h.listAPITokens)
	adm("POST /api/v1/api-tokens", h.createAPIToken)
	adm("DELETE /api/v1/api-tokens/{id}", h.deleteAPIToken)
	adm("GET /api/v1/audit", h.listAudit)

}

// ---- auth ----

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	u, err := h.auth.Login(w, r, req.Username, req.Password)
	if err != nil {
		writeErr(w, 401, err.Error())
		return
	}
	h.st.Audit(r.Context(), u.Username, "auth.login", "user", u.ID, map[string]any{"ip": auth.ClientIP(r)})
	writeJSON(w, 200, u)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	h.auth.Logout(w, r)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p := h.auth.Resolve(r)
	if p == nil {
		writeErr(w, 401, "not authenticated")
		return
	}
	if p.Via == "token" {
		writeJSON(w, 200, map[string]any{"id": p.UserID, "username": p.Username, "role": p.Role, "via": p.Via})
		return
	}
	u, err := h.st.GetUser(r.Context(), p.UserID)
	if err != nil {
		writeErr(w, 401, "not authenticated")
		return
	}
	writeJSON(w, 200, u)
}

func (h *Handler) publicStatus(w http.ResponseWriter, r *http.Request) {
	settings, _ := h.st.GetSettings(r.Context())
	writeJSON(w, 200, map[string]any{"org_name": settings.OrgName, "version": h.cfg.Version, "time": time.Now().UTC()})
}

func (h *Handler) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	p := auth.FromContext(r.Context())
	if p.Via != "session" {
		writeErr(w, 400, "password changes require a browser session")
		return
	}
	var req struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	u, hash, err := h.st.GetUserByUsername(r.Context(), p.Username)
	if err != nil || !auth.CheckPassword(hash, req.Current) {
		writeErr(w, 403, "current password is incorrect")
		return
	}
	newHash, err := auth.HashPassword(req.New)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := h.st.SetUserPassword(r.Context(), u.ID, newHash); err != nil {
		h.fail(w, err, "password")
		return
	}
	h.st.Audit(r.Context(), p.Username, "user.password_change", "user", u.ID, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	d, err := h.st.Dashboard(r.Context())
	if err != nil {
		h.fail(w, err, "dashboard")
		return
	}
	d.AgentsOnline = h.notifier.OnlineCount()
	writeJSON(w, 200, d)
}
