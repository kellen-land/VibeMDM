package api

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/pushcert"
	"github.com/vibemdm/vibemdm/internal/store"
)

type settingsView struct {
	OrgName                     string `json:"org_name"`
	ServerURL                   string `json:"server_url"`
	AgentCheckinIntervalSeconds int    `json:"agent_checkin_interval_seconds"`
	AgentEnrollmentSecret       string `json:"agent_enrollment_secret,omitempty"`
	EnrollmentProfileName       string `json:"enrollment_profile_name"`
	EnrollmentProfileIdentifier string `json:"enrollment_profile_identifier"`
	OpenEnrollment              bool   `json:"open_enrollment"`
	InventoryIntervalHours      int    `json:"inventory_interval_hours"`
	SigningConfigured           bool   `json:"signing_configured"`
	SigningCertSubject          string `json:"signing_cert_subject,omitempty"`
	RemoteShellEnabled          bool   `json:"remote_shell_enabled"`
	RemoteShellIdleTimeoutSecs  int    `json:"remote_shell_idle_timeout_secs"`
	FileVaultAutoRemediate      bool   `json:"filevault_auto_remediate"`
	EncryptionAtRest            bool   `json:"encryption_at_rest"`
	LogRetentionDays            int    `json:"log_retention_days"`
	LogArchiveEnabled           bool   `json:"log_archive_enabled"`
	Version                     string `json:"version"`
}

func (h *Handler) view(s model.Settings, admin bool) settingsView {
	v := settingsView{
		OrgName: s.OrgName, ServerURL: s.ServerURL, AgentCheckinIntervalSeconds: s.AgentCheckinIntervalSeconds,
		EnrollmentProfileName: s.EnrollmentProfileName, EnrollmentProfileIdentifier: s.EnrollmentProfileIdentifier,
		OpenEnrollment: s.OpenEnrollment, InventoryIntervalHours: s.InventoryIntervalHours,
		SigningConfigured: s.SigningCertPEM != "" && s.SigningKeyPEM != "", Version: h.cfg.Version,
		RemoteShellEnabled: s.RemoteShellEnabled, RemoteShellIdleTimeoutSecs: s.RemoteShellIdleTimeoutSecs,
		FileVaultAutoRemediate: s.FileVaultAutoRemediate, EncryptionAtRest: h.st.Box.Enabled(),
		LogRetentionDays: s.LogRetentionDays, LogArchiveEnabled: s.LogArchiveEnabled,
	}
	if admin {
		v.AgentEnrollmentSecret = s.AgentEnrollmentSecret
	}
	if v.SigningConfigured {
		if blk, _ := pem.Decode([]byte(s.SigningCertPEM)); blk != nil {
			if c, err := x509.ParseCertificate(blk.Bytes); err == nil {
				v.SigningCertSubject = c.Subject.CommonName
			}
		}
	}
	return v
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.st.GetSettings(r.Context())
	if err != nil {
		h.fail(w, err, "settings")
		return
	}
	writeJSON(w, 200, h.view(s, auth.FromContext(r.Context()).IsAdmin()))
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrgName                     *string `json:"org_name"`
		ServerURL                   *string `json:"server_url"`
		AgentCheckinIntervalSeconds *int    `json:"agent_checkin_interval_seconds"`
		EnrollmentProfileName       *string `json:"enrollment_profile_name"`
		EnrollmentProfileIdentifier *string `json:"enrollment_profile_identifier"`
		OpenEnrollment              *bool   `json:"open_enrollment"`
		InventoryIntervalHours      *int    `json:"inventory_interval_hours"`
		SigningCertPEM              *string `json:"signing_cert_pem"`
		SigningKeyPEM               *string `json:"signing_key_pem"`
		ClearSigning                bool    `json:"clear_signing"`
		RemoteShellEnabled          *bool   `json:"remote_shell_enabled"`
		RemoteShellIdleTimeoutSecs  *int    `json:"remote_shell_idle_timeout_secs"`
		LogRetentionDays            *int    `json:"log_retention_days"`
		LogArchiveEnabled           *bool   `json:"log_archive_enabled"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s, err := h.st.GetSettings(r.Context())
	if err != nil {
		h.fail(w, err, "settings")
		return
	}
	if req.OrgName != nil && strings.TrimSpace(*req.OrgName) != "" {
		s.OrgName = strings.TrimSpace(*req.OrgName)
	}
	if req.ServerURL != nil {
		u := strings.TrimRight(strings.TrimSpace(*req.ServerURL), "/")
		if u != "" && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
			writeErr(w, 400, "server_url must start with https://")
			return
		}
		s.ServerURL = u
	}
	if req.AgentCheckinIntervalSeconds != nil {
		if *req.AgentCheckinIntervalSeconds < 30 || *req.AgentCheckinIntervalSeconds > 86400 {
			writeErr(w, 400, "check-in interval must be between 30 and 86400 seconds")
			return
		}
		s.AgentCheckinIntervalSeconds = *req.AgentCheckinIntervalSeconds
	}
	if req.EnrollmentProfileName != nil && *req.EnrollmentProfileName != "" {
		s.EnrollmentProfileName = *req.EnrollmentProfileName
	}
	if req.EnrollmentProfileIdentifier != nil && *req.EnrollmentProfileIdentifier != "" {
		s.EnrollmentProfileIdentifier = *req.EnrollmentProfileIdentifier
	}
	if req.OpenEnrollment != nil {
		s.OpenEnrollment = *req.OpenEnrollment
	}
	if req.InventoryIntervalHours != nil && *req.InventoryIntervalHours > 0 {
		s.InventoryIntervalHours = *req.InventoryIntervalHours
	}
	if req.RemoteShellEnabled != nil {
		s.RemoteShellEnabled = *req.RemoteShellEnabled
		h.st.Audit(r.Context(), actor(r), "settings.remote_shell", "settings", "", map[string]any{"enabled": *req.RemoteShellEnabled})
	}
	if req.RemoteShellIdleTimeoutSecs != nil && *req.RemoteShellIdleTimeoutSecs >= 30 {
		s.RemoteShellIdleTimeoutSecs = *req.RemoteShellIdleTimeoutSecs
	}
	if req.LogRetentionDays != nil {
		if *req.LogRetentionDays < 1 || *req.LogRetentionDays > 3650 {
			writeErr(w, 400, "log retention must be between 1 and 3650 days")
			return
		}
		s.LogRetentionDays = *req.LogRetentionDays
	}
	if req.LogArchiveEnabled != nil {
		s.LogArchiveEnabled = *req.LogArchiveEnabled
	}
	if req.ClearSigning {
		s.SigningCertPEM, s.SigningKeyPEM = "", ""
	} else if req.SigningCertPEM != nil && req.SigningKeyPEM != nil && *req.SigningCertPEM != "" && *req.SigningKeyPEM != "" {
		// Validate by signing a small payload.
		if _, err := signTest(*req.SigningCertPEM, *req.SigningKeyPEM); err != nil {
			writeErr(w, 400, "signing identity invalid: "+err.Error())
			return
		}
		s.SigningCertPEM, s.SigningKeyPEM = *req.SigningCertPEM, *req.SigningKeyPEM
	}
	if err := h.st.SaveSettings(r.Context(), s); err != nil {
		h.fail(w, err, "settings")
		return
	}
	h.st.Audit(r.Context(), actor(r), "settings.update", "settings", "", nil)
	writeJSON(w, 200, h.view(s, true))
}

func (h *Handler) rotateEnrollmentSecret(w http.ResponseWriter, r *http.Request) {
	s, err := h.st.GetSettings(r.Context())
	if err != nil {
		h.fail(w, err, "settings")
		return
	}
	s.AgentEnrollmentSecret = auth.RandomToken(24)
	if err := h.st.SaveSettings(r.Context(), s); err != nil {
		h.fail(w, err, "settings")
		return
	}
	h.st.Audit(r.Context(), actor(r), "settings.rotate_enrollment_secret", "settings", "", nil)
	writeJSON(w, 200, h.view(s, true))
}

// ---- APNs ----

type apnsCert struct {
	Topic     string    `json:"topic"`
	Subject   string    `json:"subject"`
	NotAfter  time.Time `json:"not_after"`
	NotBefore time.Time `json:"not_before"`
	Expired   bool      `json:"expired"`
}

func (h *Handler) apnsStatus(w http.ResponseWriter, r *http.Request) {
	topics, err := h.st.GetPushCertTopics(r.Context())
	if err != nil {
		h.fail(w, err, "apns")
		return
	}
	certs := []apnsCert{}
	for _, t := range topics {
		pemStr, err := h.st.GetPushCertPEM(r.Context(), t)
		if err != nil {
			continue
		}
		blk, _ := pem.Decode([]byte(pemStr))
		if blk == nil {
			continue
		}
		c, err := x509.ParseCertificate(blk.Bytes)
		if err != nil {
			continue
		}
		certs = append(certs, apnsCert{Topic: t, Subject: c.Subject.CommonName, NotAfter: c.NotAfter, NotBefore: c.NotBefore, Expired: time.Now().After(c.NotAfter)})
	}
	out := map[string]any{"configured": len(certs) > 0, "certificates": certs}
	if _, csr, _, created, err := h.st.GetPushCertRequest(r.Context()); err == nil {
		out["pending_request"] = map[string]any{"csr_pem": csr, "created_at": created}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) storePushCert(w http.ResponseWriter, r *http.Request, certIn, keyIn []byte) {
	certPEM, cert, err := pushcert.NormalizeCertificatePEM(certIn)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	keyPEM, key, err := pushcert.NormalizeKeyPEM(keyIn)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !pushcert.KeyMatchesCert(cert, key) {
		writeErr(w, 400, "the private key does not match the certificate")
		return
	}
	topic, err := pushcert.TopicFromCert(cert)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := h.pushStore.StorePushCert(r.Context(), []byte(certPEM), []byte(keyPEM)); err != nil {
		h.fail(w, err, "store push cert")
		return
	}
	h.st.Audit(r.Context(), actor(r), "apns.upload", "pushcert", topic, map[string]any{"not_after": cert.NotAfter})
	writeJSON(w, 200, apnsCert{Topic: topic, Subject: cert.Subject.CommonName, NotAfter: cert.NotAfter, NotBefore: cert.NotBefore})
}

func (h *Handler) apnsUpload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CertPEM string `json:"cert_pem"`
		KeyPEM  string `json:"key_pem"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.storePushCert(w, r, []byte(req.CertPEM), []byte(req.KeyPEM))
}

func (h *Handler) apnsNewRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email  string `json:"email"`
		Submit bool   `json:"submit"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s, _ := h.st.GetSettings(r.Context())
	pr, err := pushcert.NewRequest(s.OrgName, req.Email)
	if err != nil {
		h.fail(w, err, "generate request")
		return
	}
	if err := h.st.SavePushCertRequest(r.Context(), pr.KeyPEM, pr.CSRPEM, pr.CertPEM); err != nil {
		h.fail(w, err, "save request")
		return
	}
	submitted := false
	submitErr := ""
	if req.Submit {
		if err := pushcert.SubmitToMDMCertDownload(r.Context(), nil, pr, req.Email); err != nil {
			submitErr = err.Error()
		} else {
			submitted = true
		}
	}
	h.st.Audit(r.Context(), actor(r), "apns.request", "pushcert", "", map[string]any{"email": req.Email, "submitted": submitted})
	writeJSON(w, 200, map[string]any{"csr_pem": pr.CSRPEM, "submitted": submitted, "submit_error": submitErr})
}

func (h *Handler) apnsDecrypt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Encrypted string `json:"encrypted"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	keyPEM, csrPEM, certPEM, _, err := h.st.GetPushCertRequest(r.Context())
	if err != nil {
		writeErr(w, 400, "no pending push certificate request; generate one first")
		return
	}
	plistBytes, err := pushcert.DecryptSignedRequest([]byte(req.Encrypted), &pushcert.Request{KeyPEM: keyPEM, CSRPEM: csrPEM, CertPEM: certPEM})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="vibemdm_push_certificate_request.plist"`)
	w.Write(plistBytes)
}

func (h *Handler) apnsCompleteCertificate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CertPEM string `json:"cert_pem"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	keyPEM, _, _, _, err := h.st.GetPushCertRequest(r.Context())
	if err != nil {
		writeErr(w, 400, "no pending push certificate request; generate one first")
		return
	}
	h.storePushCert(w, r, []byte(req.CertPEM), []byte(keyPEM))
}

// ---- SCEP ----

func (h *Handler) scepStatus(w http.ResponseWriter, r *http.Request) {
	ca := h.scep.CACert()
	issued, active, _ := h.st.SCEPStats(r.Context())
	out := map[string]any{
		"subject": ca.Subject.CommonName, "not_after": ca.NotAfter, "not_before": ca.NotBefore,
		"issued": issued, "active": active, "ca_pem": string(h.scep.CAPEM()),
	}
	if auth.FromContext(r.Context()).IsAdmin() {
		s, _ := h.st.GetSettings(r.Context())
		out["challenge"] = s.SCEPChallenge
	}
	writeJSON(w, 200, out)
}

func (h *Handler) scepRegenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm string `json:"confirm"`
	}
	decode(r, &req)
	if req.Confirm != "REGENERATE" {
		writeErr(w, 400, `confirm must be "REGENERATE"; this invalidates every enrolled device identity`)
		return
	}
	s, _ := h.st.GetSettings(r.Context())
	if err := h.scep.Regenerate(r.Context(), s.OrgName); err != nil {
		h.fail(w, err, "regenerate CA")
		return
	}
	s.SCEPChallenge = auth.RandomToken(24)
	h.st.SaveSettings(r.Context(), s)
	h.st.Audit(r.Context(), actor(r), "scep.regenerate", "scep", "", nil)
	h.scepStatus(w, r)
}

// ---- enrollment ----

func (h *Handler) enrollmentStatus(w http.ResponseWriter, r *http.Request) {
	s, _ := h.st.GetSettings(r.Context())
	topics, _ := h.st.GetPushCertTopics(r.Context())
	invs, _ := h.st.ListInvitations(r.Context())
	agents := map[string]bool{}
	for _, p := range []string{"darwin-arm64", "darwin-amd64"} {
		_, err := os.Stat(filepath.Join(h.cfg.AgentDir, "vibemdm-agent-"+p))
		agents[p] = err == nil
	}
	issues := []string{}
	if s.ServerURL == "" {
		issues = append(issues, "Server URL is not set (Settings → General). Devices need the public HTTPS URL.")
	} else if !strings.HasPrefix(s.ServerURL, "https://") {
		issues = append(issues, "Server URL is not HTTPS. Apple devices require a valid TLS certificate for MDM.")
	}
	if len(topics) == 0 {
		issues = append(issues, "No APNs push certificate uploaded (Settings → APNs). MDM enrollment profiles cannot be generated without one.")
	}
	if !agents["darwin-arm64"] && !agents["darwin-amd64"] {
		issues = append(issues, "No agent binaries found on the server; the curl installer will fail. Build with `make agent`.")
	}
	out := map[string]any{
		"server_url": s.ServerURL, "apns_configured": len(topics) > 0, "topics": topics, "scep_ok": h.scep.CACert() != nil,
		"agent_binaries": agents, "open_enrollment": s.OpenEnrollment, "invitations": len(invs), "issues": issues,
		"enroll_url": s.ServerURL + "/enroll",
	}
	if auth.FromContext(r.Context()).IsAdmin() {
		out["install_command"] = fmt.Sprintf(`curl -fsSL "%s/agent/install.sh?token=%s" | sudo bash`, s.ServerURL, s.AgentEnrollmentSecret)
		out["configurator_url"] = fmt.Sprintf("%s/enroll/configurator/%s", s.ServerURL, s.AgentEnrollmentSecret)
	}
	writeJSON(w, 200, out)
}

func (h *Handler) downloadEnrollmentProfile(w http.ResponseWriter, r *http.Request) {
	b, err := h.profile(r.Context())
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="vibemdm-enroll.mobileconfig"`)
	w.Write(b)
}

func (h *Handler) listInvitations(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListInvitations(r.Context())
	if err != nil {
		h.fail(w, err, "invitations")
		return
	}
	writeJSON(w, 200, listResp[model.EnrollmentInvitation]{Items: items, Total: len(items)})
}

func (h *Handler) createInvitation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		MaxUses     *int   `json:"max_uses"`
		ExpiresDays *int   `json:"expires_days"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	tok := auth.RandomToken(24)
	var exp *time.Time
	if req.ExpiresDays != nil && *req.ExpiresDays > 0 {
		t := time.Now().Add(time.Duration(*req.ExpiresDays) * 24 * time.Hour)
		exp = &t
	}
	inv, err := h.st.CreateInvitation(r.Context(), req.Name, store.HashToken(tok), tok[:8], req.MaxUses, exp, auth.FromContext(r.Context()).UserID)
	if err != nil {
		h.fail(w, err, "invitation")
		return
	}
	s, _ := h.st.GetSettings(r.Context())
	h.st.Audit(r.Context(), actor(r), "enrollment.invitation_create", "invitation", inv.ID, map[string]any{"name": inv.Name})
	writeJSON(w, 201, map[string]any{
		"invitation": inv, "token": tok,
		"enroll_url":       fmt.Sprintf("%s/enroll?token=%s", s.ServerURL, tok),
		"install_command":  fmt.Sprintf(`curl -fsSL "%s/agent/install.sh?token=%s" | sudo bash`, s.ServerURL, tok),
		"configurator_url": fmt.Sprintf("%s/enroll/configurator/%s", s.ServerURL, tok),
	})
}

func (h *Handler) updateInvitation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := h.st.SetInvitationEnabled(r.Context(), r.PathValue("id"), req.Enabled); err != nil {
		h.fail(w, err, "invitation")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) deleteInvitation(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteInvitation(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "invitation")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- users & tokens ----

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListUsers(r.Context())
	if err != nil {
		h.fail(w, err, "users")
		return
	}
	writeJSON(w, 200, listResp[model.AdminUser]{Items: items, Total: len(items)})
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Role        string `json:"role"`
		Password    string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeErr(w, 400, "username required")
		return
	}
	if req.Role != "viewer" {
		req.Role = "admin"
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	u, err := h.st.CreateUser(r.Context(), req.Username, req.DisplayName, req.Email, req.Role, hash)
	if err != nil {
		h.fail(w, err, "user")
		return
	}
	h.st.Audit(r.Context(), actor(r), "user.create", "user", u.ID, map[string]any{"username": u.Username, "role": u.Role})
	writeJSON(w, 201, u)
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Role        string `json:"role"`
		Enabled     bool   `json:"enabled"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if req.Role != "viewer" {
		req.Role = "admin"
	}
	if r.PathValue("id") == auth.FromContext(r.Context()).UserID && (req.Role != "admin" || !req.Enabled) {
		writeErr(w, 400, "you cannot demote or disable your own account")
		return
	}
	if err := h.st.UpdateUser(r.Context(), r.PathValue("id"), req.DisplayName, req.Email, req.Role, req.Enabled); err != nil {
		h.fail(w, err, "user")
		return
	}
	u, err := h.st.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "user")
		return
	}
	h.st.Audit(r.Context(), actor(r), "user.update", "user", u.ID, nil)
	writeJSON(w, 200, u)
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") == auth.FromContext(r.Context()).UserID {
		writeErr(w, 400, "you cannot delete your own account")
		return
	}
	if err := h.st.DeleteUser(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "user")
		return
	}
	h.st.Audit(r.Context(), actor(r), "user.delete", "user", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) setUserPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := h.st.SetUserPassword(r.Context(), r.PathValue("id"), hash); err != nil {
		h.fail(w, err, "user")
		return
	}
	h.st.Audit(r.Context(), actor(r), "user.password_reset", "user", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) listAPITokens(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListAPITokens(r.Context())
	if err != nil {
		h.fail(w, err, "api tokens")
		return
	}
	writeJSON(w, 200, listResp[model.APIToken]{Items: items, Total: len(items)})
}

func (h *Handler) createAPIToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Role        string `json:"role"`
		ExpiresDays *int   `json:"expires_days"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, 400, "name required")
		return
	}
	if req.Role != "viewer" {
		req.Role = "admin"
	}
	tok, prefix := auth.NewAPIToken()
	var exp *time.Time
	if req.ExpiresDays != nil && *req.ExpiresDays > 0 {
		t := time.Now().Add(time.Duration(*req.ExpiresDays) * 24 * time.Hour)
		exp = &t
	}
	t, err := h.st.CreateAPIToken(r.Context(), req.Name, store.HashToken(tok), prefix, req.Role, auth.FromContext(r.Context()).UserID, exp)
	if err != nil {
		h.fail(w, err, "api token")
		return
	}
	h.st.Audit(r.Context(), actor(r), "api_token.create", "api_token", t.ID, map[string]any{"name": t.Name})
	writeJSON(w, 201, map[string]any{"token": tok, "api_token": t})
}

func (h *Handler) deleteAPIToken(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteAPIToken(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "api token")
		return
	}
	h.st.Audit(r.Context(), actor(r), "api_token.delete", "api_token", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) listAudit(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListAudit(r.Context(), qInt(r, "limit", 100), qInt(r, "offset", 0))
	if err != nil {
		h.fail(w, err, "audit")
		return
	}
	writeJSON(w, 200, listResp[model.AuditEntry]{Items: items, Total: total})
}
