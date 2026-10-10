package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"howett.net/plist"

	"github.com/vibemdm/vibemdm/internal/model"
)

type profileWithStatus struct {
	model.ConfigProfile
	Status map[string]int `json:"status"`
}

func (h *Handler) listProfiles(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListProfiles(r.Context())
	if err != nil {
		h.fail(w, err, "profiles")
		return
	}
	out := make([]profileWithStatus, 0, len(items))
	for _, p := range items {
		st, _ := h.st.DeploymentStatusCounts(r.Context(), p.ID)
		if st == nil {
			st = map[string]int{}
		}
		redactProfileFor(r, &p)
		out = append(out, profileWithStatus{ConfigProfile: p, Status: st})
	}
	writeJSON(w, 200, listResp[profileWithStatus]{Items: out, Total: len(out)})
}

func (h *Handler) getProfile(w http.ResponseWriter, r *http.Request) {
	p, err := h.st.GetProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "profile")
		return
	}
	st, _ := h.st.DeploymentStatusCounts(r.Context(), p.ID)
	if st == nil {
		st = map[string]int{}
	}
	redactProfileFor(r, p)
	writeJSON(w, 200, profileWithStatus{ConfigProfile: *p, Status: st})
}

// normalizeProfilePayload validates the mobileconfig and fills identifier/uuid/name from it.
func normalizeProfilePayload(p *model.ConfigProfile) error {
	var doc map[string]any
	if _, err := plist.Unmarshal([]byte(p.Payload), &doc); err != nil {
		return errInvalid("payload is not a valid property list: " + err.Error())
	}
	if t, _ := doc["PayloadType"].(string); t != "Configuration" {
		return errInvalid("payload must be a Configuration profile (PayloadType = Configuration)")
	}
	id, _ := doc["PayloadIdentifier"].(string)
	if p.Identifier == "" {
		p.Identifier = id
	}
	if p.Identifier == "" {
		return errInvalid("profile has no PayloadIdentifier")
	}
	if p.Identifier != id {
		doc["PayloadIdentifier"] = p.Identifier
	}
	if u, _ := doc["PayloadUUID"].(string); u != "" {
		p.UUID = u
	} else {
		p.UUID = strings.ToUpper(uuid.NewString())
		doc["PayloadUUID"] = p.UUID
	}
	if p.Name == "" {
		p.Name, _ = doc["PayloadDisplayName"].(string)
	}
	if p.Name == "" {
		p.Name = p.Identifier
	}
	if p.Description == "" {
		p.Description, _ = doc["PayloadDescription"].(string)
	}
	if scope, _ := doc["PayloadScope"].(string); strings.EqualFold(scope, "User") {
		p.Level = "user"
	} else {
		p.Level = "device"
		doc["PayloadScope"] = "System"
	}
	// Re-encode so modifications (identifier/uuid) persist.
	var sb strings.Builder
	enc := plist.NewEncoder(&sb)
	enc.Indent("\t")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	p.Payload = sb.String()
	return nil
}

type invalidErr string

func (e invalidErr) Error() string { return string(e) }
func errInvalid(s string) error    { return invalidErr(s) }

func (h *Handler) createProfile(w http.ResponseWriter, r *http.Request) {
	var p model.ConfigProfile
	if err := decode(r, &p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(p.Payload) == "" {
		writeErr(w, 400, "payload (mobileconfig XML) required")
		return
	}
	if err := normalizeProfilePayload(&p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	out, err := h.st.CreateProfile(r.Context(), p)
	if err != nil {
		h.fail(w, err, "profile")
		return
	}
	h.st.Audit(r.Context(), actor(r), "profile.create", "profile", out.ID, map[string]any{"name": out.Name})
	h.syncProfileAsync(out, actor(r), false)
	writeJSON(w, 201, out)
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	var p model.ConfigProfile
	if err := decode(r, &p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p.ID = r.PathValue("id")
	existing, err := h.st.GetProfile(r.Context(), p.ID)
	if err != nil {
		h.fail(w, err, "profile")
		return
	}
	if strings.TrimSpace(p.Payload) == "" {
		p.Payload = existing.Payload
	}
	if err := normalizeProfilePayload(&p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	out, err := h.st.UpdateProfile(r.Context(), p)
	if err != nil {
		h.fail(w, err, "profile")
		return
	}
	h.st.Audit(r.Context(), actor(r), "profile.update", "profile", out.ID, map[string]any{"name": out.Name})
	// Payload or scope changes re-push to scoped devices.
	h.syncProfileAsync(out, actor(r), existing.Payload != out.Payload)
	writeJSON(w, 200, out)
}

func (h *Handler) syncProfileAsync(p *model.ConfigProfile, by string, reinstall bool) {
	prof := *p
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if _, _, err := h.cmdr.SyncProfile(ctx, &prof, by, reinstall); err != nil {
			h.log.Warn("profile sync", "profile", prof.Name, "err", err)
		}
	}()
}

func (h *Handler) deleteProfile(w http.ResponseWriter, r *http.Request) {
	p, err := h.st.GetProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "profile")
		return
	}
	// Remove from devices that have it, then delete the record.
	deps, _ := h.st.ListDeployments(r.Context(), p.ID, "")
	removed := 0
	for _, d := range deps {
		if d.Status == "removed" {
			continue
		}
		c, err := h.st.GetComputer(r.Context(), d.ComputerID)
		if err != nil || !c.MDMEnrolled {
			continue
		}
		if err := h.cmdr.RemoveProfile(r.Context(), c, p, actor(r)); err == nil {
			removed++
		}
	}
	if err := h.st.DeleteProfile(r.Context(), p.ID); err != nil {
		h.fail(w, err, "profile")
		return
	}
	h.st.Audit(r.Context(), actor(r), "profile.delete", "profile", p.ID, map[string]any{"name": p.Name, "removed_from": removed})
	writeJSON(w, 200, map[string]any{"ok": true, "removed_from": removed})
}

func (h *Handler) profileDeployments(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListDeployments(r.Context(), r.PathValue("id"), "")
	if err != nil {
		h.fail(w, err, "deployments")
		return
	}
	writeJSON(w, 200, listResp[model.ProfileDeployment]{Items: items, Total: len(items)})
}

func (h *Handler) deployProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reinstall bool `json:"reinstall"`
	}
	decode(r, &req)
	p, err := h.st.GetProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "profile")
		return
	}
	installed, removed, err := h.cmdr.SyncProfile(r.Context(), p, actor(r), req.Reinstall)
	if err != nil {
		h.fail(w, err, "deploy")
		return
	}
	h.st.Audit(r.Context(), actor(r), "profile.deploy", "profile", p.ID, map[string]any{"installed": installed, "removed": removed})
	writeJSON(w, 200, map[string]any{"installed": installed, "removed": removed})
}

func (h *Handler) removeProfileFrom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ComputerIDs []string `json:"computer_ids"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p, err := h.st.GetProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "profile")
		return
	}
	n := 0
	for _, id := range req.ComputerIDs {
		c, err := h.st.GetComputer(r.Context(), id)
		if err != nil {
			continue
		}
		if err := h.cmdr.RemoveProfile(r.Context(), c, p, actor(r)); err == nil {
			n++
		}
	}
	writeJSON(w, 200, map[string]any{"removed": n})
}
