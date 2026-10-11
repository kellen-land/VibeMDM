package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
	"github.com/vibemdm/vibemdm/internal/webhook"
)

// lapsOverview returns the policy and every in-scope Mac's status (no passwords).
func (h *Handler) lapsOverview(w http.ResponseWriter, r *http.Request) {
	cfg, rows, sum, err := h.st.LAPSOverview(r.Context())
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	writeJSON(w, 200, map[string]any{"config": cfg, "computers": rows, "summary": sum})
}

// saveLAPS validates and stores the LAPS policy.
func (h *Handler) saveLAPS(w http.ResponseWriter, r *http.Request) {
	var req model.LAPSConfig
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cfg, err := h.st.SaveLAPSConfig(r.Context(), req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.st.Audit(r.Context(), actor(r), "laps.config", "settings", "", map[string]any{"enabled": cfg.Enabled, "account": cfg.AccountName,
		"auto_rotate": cfg.AutoRotate, "rotation_days": cfg.RotationDays, "rotate_after_view_minutes": cfg.RotateAfterUseMinutes})
	h.lapsOverview(w, r)
}

// lapsPending lists Macs with a queued rotation.
func (h *Handler) lapsPending(w http.ResponseWriter, r *http.Request) {
	out, err := h.st.LAPSPendingRotations(r.Context())
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	writeJSON(w, 200, out)
}

// computerLAPS returns one computer's LAPS status (never a password).
func (h *Handler) computerLAPS(w http.ResponseWriter, r *http.Request) {
	c, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	st, err := h.st.LAPSStatusFor(r.Context(), c)
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	writeJSON(w, 200, st)
}

// computerLAPSHistory returns the event history and the password history with view
// records. Passwords are never included here.
func (h *Handler) computerLAPSHistory(w http.ResponseWriter, r *http.Request) {
	c, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	events, err := h.st.LAPSEvents(r.Context(), c.ID, 200)
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	passwords, err := h.st.LAPSHistory(r.Context(), c.ID, false)
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	writeJSON(w, 200, map[string]any{"events": events, "passwords": passwords})
}

// doRevealLAPS returns the decrypted password(s) and does the bookkeeping every reveal
// needs: audit entry, webhook, log line. As in Jamf Pro, the password rotates after viewing.
func (h *Handler) doRevealLAPS(r *http.Request, comp *model.Computer, reason string) (*store.LAPSPasswords, *model.LAPSStatus, error) {
	p := auth.FromContext(r.Context())
	pw, st, err := h.st.RevealLAPS(r.Context(), comp.ID, p.Username, reason)
	if err != nil {
		return nil, nil, err
	}
	h.st.Audit(r.Context(), p.Username, "laps.password_viewed", "computer", comp.ID,
		map[string]any{"reason": reason, "ip": auth.ClientIP(r), "fingerprint": st.Fingerprint, "rotate_after": st.RotateAfter})
	h.hooks.Fire(webhook.LAPSPasswordViewed, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "serial": comp.SerialNumber, "actor": p.Username, "reason": reason})
	h.log.Info("LAPS password viewed", "computer", comp.Name, "actor", p.Username)
	if h.notifier != nil {
		h.notifier.Notify(comp.ID) // so the agent is listening when the rotation falls due
	}
	return pw, st, nil
}

// revealLAPS returns the decrypted password. Admin-only; a reason is required;
// audited and announced by webhook. The password rotates after viewing.
func (h *Handler) revealLAPS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	decode(r, &req)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		writeErr(w, 400, "a reason is required")
		return
	}
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	pw, st, err := h.doRevealLAPS(r, comp, req.Reason)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "no local administrator password is escrowed for this computer")
			return
		}
		h.fail(w, err, "laps password")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	out := map[string]any{"account": st.Account, "password": pw.Current, "status": st}
	if pw.Pending != "" {
		// The agent staged this but never confirmed it: it may be the live password.
		out["unconfirmed_password"] = pw.Pending
	}
	writeJSON(w, 200, out)
}

// doRotateLAPS flags a rotation and wakes the agent.
func (h *Handler) doRotateLAPS(r *http.Request, comp *model.Computer) error {
	if err := h.st.LAPSRequestRotation(r.Context(), comp.ID, actor(r)); err != nil {
		return err
	}
	if _, err := h.queueTask(r, comp, "laps_rotate", "", nil, "", "", "", ""); err != nil {
		return err
	}
	h.st.Audit(r.Context(), actor(r), "laps.rotate_requested", "computer", comp.ID, nil)
	return nil
}

// rotateLAPS asks the agent to rotate now.
func (h *Handler) rotateLAPS(w http.ResponseWriter, r *http.Request) {
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	if err := h.doRotateLAPS(r, comp); err != nil {
		h.fail(w, err, "laps")
		return
	}
	h.computerLAPS(w, r)
}

// errLAPSAccount means a request named an account LAPS does not manage on that Mac.
var errLAPSAccount = errors.New("that account is not managed by LAPS on this computer")

// doSetLAPSPassword stores an administrator-chosen password and wakes the agent to apply it.
func (h *Handler) doSetLAPSPassword(r *http.Request, comp *model.Computer, username, password string) error {
	cfg, err := h.st.GetLAPSConfig(r.Context())
	if err != nil {
		return err
	}
	st, err := h.st.LAPSStatusFor(r.Context(), comp)
	if err != nil {
		return err
	}
	account := cfg.AccountName
	if st.Account != "" {
		account = st.Account
	}
	if !st.InScope {
		return errors.New("LAPS does not manage this computer")
	}
	if username != "" && username != account {
		return errLAPSAccount
	}
	if err := h.st.LAPSSetPassword(r.Context(), comp.ID, account, password, actor(r)); err != nil {
		return err
	}
	if _, err := h.queueTask(r, comp, "laps_rotate", "", nil, "", "", "", ""); err != nil {
		return err
	}
	h.st.Audit(r.Context(), actor(r), "laps.password_set", "computer", comp.ID, map[string]any{"account": account})
	return nil
}

// setLAPSPassword sets a specific password (admin).
func (h *Handler) setLAPSPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	if err := h.doSetLAPSPassword(r, comp, "", req.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.computerLAPS(w, r)
}

// grantLAPSSecureToken asks the agent to give the LAPS account a Secure Token (and, on Apple
// silicon, volume ownership) using an existing Secure Token administrator's credentials. They are
// sealed, handed to the agent once and deleted; they expire after 30 minutes if the Mac is offline.
func (h *Handler) grantLAPSSecureToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AdminUser     string `json:"admin_user"`
		AdminPassword string `json:"admin_password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	req.AdminUser = strings.TrimSpace(req.AdminUser)
	if req.AdminUser == "" || req.AdminPassword == "" || len(req.AdminUser) > 64 || len(req.AdminPassword) > 256 {
		writeErr(w, 400, "the administrator's account name and password are required")
		return
	}
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	st, err := h.st.LAPSStatusFor(r.Context(), comp)
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	if !st.InScope || !st.HasPassword {
		writeErr(w, 400, "LAPS does not manage an account with a known password on this computer yet")
		return
	}
	if req.AdminUser == st.Account {
		writeErr(w, 400, "use a different administrator that already has a Secure Token")
		return
	}
	if err := h.st.LAPSCreateTokenGrant(r.Context(), comp.ID, req.AdminUser, req.AdminPassword, actor(r), 30*time.Minute); err != nil {
		h.fail(w, err, "laps")
		return
	}
	if _, err := h.queueTask(r, comp, "laps_rotate", "", nil, "", "", "", ""); err != nil {
		h.fail(w, err, "task")
		return
	}
	h.st.Audit(r.Context(), actor(r), "laps.secure_token_requested", "computer", comp.ID, map[string]any{"admin_user": req.AdminUser, "ip": auth.ClientIP(r)})
	h.computerLAPS(w, r)
}

// ---- Jamf Pro-compatible API: /api/v2/local-admin-password/... ----
//
// Mirrors the Jamf Pro API's LAPS endpoints so scripts written for Jamf work with a VibeMDM
// API token. {clientManagementId} is the VibeMDM computer ID. userSource is always "JMF"
// (the agent-managed account); MDM LAPS (SetAutoAdminPassword) needs Automated Device
// Enrollment and is not offered, so autoDeployEnabled is always false.

func jamfTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

type jamfLapsSettings struct {
	AutoDeployEnabled        bool `json:"autoDeployEnabled"`
	PasswordRotationTime     int  `json:"passwordRotationTime"`
	AutoRotateEnabled        bool `json:"autoRotateEnabled"`
	AutoRotateExpirationTime int  `json:"autoRotateExpirationTime"`
}

func (h *Handler) jamfLapsGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.st.GetLAPSConfig(r.Context())
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	writeJSON(w, 200, jamfLapsSettings{PasswordRotationTime: cfg.RotateAfterUseMinutes * 60, AutoRotateEnabled: cfg.AutoRotate,
		AutoRotateExpirationTime: cfg.RotationDays * 86400})
}

func (h *Handler) jamfLapsPutSettings(w http.ResponseWriter, r *http.Request) {
	var req jamfLapsSettings
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if req.AutoDeployEnabled {
		writeErr(w, 400, "autoDeployEnabled (MDM LAPS) is not supported: it needs Automated Device Enrollment")
		return
	}
	cfg, err := h.st.GetLAPSConfig(r.Context())
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	cfg.RotateAfterUseMinutes = (req.PasswordRotationTime + 59) / 60
	cfg.AutoRotate = req.AutoRotateEnabled
	cfg.RotationDays = (req.AutoRotateExpirationTime + 86399) / 86400
	if _, err := h.st.SaveLAPSConfig(r.Context(), cfg); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.st.Audit(r.Context(), actor(r), "laps.config", "settings", "", map[string]any{"via": "jamf-api", "auto_rotate": cfg.AutoRotate,
		"rotation_days": cfg.RotationDays, "rotate_after_view_minutes": cfg.RotateAfterUseMinutes})
	h.jamfLapsGetSettings(w, r)
}

type jamfLapsUser struct {
	ClientManagementID string `json:"clientManagementId"`
	GUID               string `json:"guid"`
	Username           string `json:"username"`
	UserSource         string `json:"userSource"`
}

func (h *Handler) jamfLapsPending(w http.ResponseWriter, r *http.Request) {
	rows, err := h.st.LAPSPendingRotations(r.Context())
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	type item struct {
		LapsUser    jamfLapsUser `json:"lapsUser"`
		CreatedDate string       `json:"createdDate"`
	}
	out := []item{}
	for _, p := range rows {
		since := p.Since
		out = append(out, item{jamfLapsUser{p.ComputerID, p.Guid, p.Account, "JMF"}, jamfTime(&since)})
	}
	writeJSON(w, 200, map[string]any{"totalCount": len(out), "results": out})
}

// jamfLapsAccount resolves {clientManagementId} and, when given, {username} and {guid}.
func (h *Handler) jamfLapsAccount(w http.ResponseWriter, r *http.Request) (*model.Computer, *model.LAPSStatus, bool) {
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("cmid"))
	if err != nil {
		writeErr(w, 404, "computer not found")
		return nil, nil, false
	}
	st, err := h.st.LAPSStatusFor(r.Context(), comp)
	if err != nil {
		h.fail(w, err, "laps")
		return nil, nil, false
	}
	if u := r.PathValue("username"); u != "" && u != st.Account {
		writeErr(w, 404, "no LAPS account with that username on this computer")
		return nil, nil, false
	}
	if g := r.PathValue("guid"); g != "" && !strings.EqualFold(g, st.Guid) {
		writeErr(w, 404, "no LAPS account with that GUID on this computer")
		return nil, nil, false
	}
	return comp, st, true
}

func (h *Handler) jamfLapsAccounts(w http.ResponseWriter, r *http.Request) {
	comp, st, ok := h.jamfLapsAccount(w, r)
	if !ok {
		return
	}
	out := []jamfLapsUser{}
	if st.HasPassword {
		out = append(out, jamfLapsUser{comp.ID, st.Guid, st.Account, "JMF"})
	}
	writeJSON(w, 200, map[string]any{"totalCount": len(out), "results": out})
}

func (h *Handler) jamfLapsPassword(w http.ResponseWriter, r *http.Request) {
	comp, _, ok := h.jamfLapsAccount(w, r)
	if !ok {
		return
	}
	pw, _, err := h.doRevealLAPS(r, comp, "Jamf-compatible API")
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "no password is escrowed for this account")
			return
		}
		h.fail(w, err, "laps")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]string{"password": pw.Current})
}

// jamfLapsAudit returns every password with who viewed it (Jamf includes the passwords).
func (h *Handler) jamfLapsAudit(w http.ResponseWriter, r *http.Request) {
	comp, _, ok := h.jamfLapsAccount(w, r)
	if !ok {
		return
	}
	hist, err := h.st.LAPSHistory(r.Context(), comp.ID, true)
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	h.st.Audit(r.Context(), actor(r), "laps.history_viewed", "computer", comp.ID, map[string]any{"ip": auth.ClientIP(r), "passwords": len(hist)})
	type audit struct {
		ViewedBy string `json:"viewedBy"`
		DateSeen string `json:"dateSeen"`
	}
	type item struct {
		Password       string  `json:"password"`
		DateLastSeen   string  `json:"dateLastSeen"`
		ExpirationTime string  `json:"expirationTime"`
		Audits         []audit `json:"audits"`
	}
	out := []item{}
	for _, e := range hist {
		it := item{Password: e.Password, ExpirationTime: jamfTime(e.RetiredAt), Audits: []audit{}}
		for i, v := range e.Views {
			at := v.ViewedAt
			if i == 0 {
				it.DateLastSeen = jamfTime(&at)
			}
			it.Audits = append(it.Audits, audit{v.ViewedBy, jamfTime(&at)})
		}
		out = append(out, it)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"totalCount": len(out), "results": out})
}

// jamfLapsAccountHistory lists each password's lifecycle (Jamf rotationStatus values).
func (h *Handler) jamfLapsAccountHistory(w http.ResponseWriter, r *http.Request) {
	comp, st, ok := h.jamfLapsAccount(w, r)
	if !ok {
		return
	}
	hist, err := h.st.LAPSHistory(r.Context(), comp.ID, false)
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	type item struct {
		CreatedDate    string `json:"createdDate"`
		DateLastSeen   string `json:"dateLastSeen"`
		ExpirationTime string `json:"expirationTime"`
		RotationStatus string `json:"rotationStatus"`
	}
	out := []item{}
	for i, e := range hist {
		created := e.CreatedAt
		it := item{CreatedDate: jamfTime(&created), ExpirationTime: jamfTime(e.RetiredAt), RotationStatus: "COMPLETED"}
		if len(e.Views) > 0 {
			at := e.Views[0].ViewedAt
			it.DateLastSeen = jamfTime(&at)
			it.RotationStatus = "VIEWED"
		}
		if i == 0 && e.RetiredAt == nil {
			switch st.RotationStatus {
			case "pending", "due":
				it.RotationStatus = "PENDING"
			case "failing":
				it.RotationStatus = "ERROR"
			}
			if st.RotateAfter != nil {
				it.ExpirationTime = jamfTime(st.RotateAfter)
			} else if st.ExpiresAt != nil {
				it.ExpirationTime = jamfTime(st.ExpiresAt)
			}
		}
		out = append(out, it)
	}
	writeJSON(w, 200, map[string]any{"totalCount": len(out), "results": out})
}

// jamfLapsFullHistory is the computer-wide event list.
func (h *Handler) jamfLapsFullHistory(w http.ResponseWriter, r *http.Request) {
	comp, _, ok := h.jamfLapsAccount(w, r)
	if !ok {
		return
	}
	events, err := h.st.LAPSEvents(r.Context(), comp.ID, 1000)
	if err != nil {
		h.fail(w, err, "laps")
		return
	}
	type item struct {
		Username   string `json:"username"`
		EventType  string `json:"eventType"`
		EventTime  string `json:"eventTime"`
		ViewedBy   string `json:"viewedBy"`
		UserSource string `json:"userSource"`
	}
	out := []item{}
	for _, e := range events {
		at := e.At
		it := item{Username: e.Account, EventType: e.EventType, EventTime: jamfTime(&at), UserSource: "JMF"}
		if e.EventType == "VIEWED" {
			it.ViewedBy = e.Actor
		}
		out = append(out, it)
	}
	writeJSON(w, 200, map[string]any{"totalCount": len(out), "results": out})
}

func (h *Handler) jamfLapsSetPassword(w http.ResponseWriter, r *http.Request) {
	comp, _, ok := h.jamfLapsAccount(w, r)
	if !ok {
		return
	}
	var req struct {
		LapsUserPasswordList []struct {
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"lapsUserPasswordList"`
	}
	if err := decode(r, &req); err != nil || len(req.LapsUserPasswordList) == 0 {
		writeErr(w, 400, "lapsUserPasswordList is required")
		return
	}
	type done struct {
		Username string `json:"username"`
	}
	out := []done{}
	for _, u := range req.LapsUserPasswordList {
		if err := h.doSetLAPSPassword(r, comp, u.Username, u.Password); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		out = append(out, done{u.Username})
	}
	writeJSON(w, 200, map[string]any{"lapsUserPasswordList": out})
}
