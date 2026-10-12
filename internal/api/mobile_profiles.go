package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/vibemdm/vibemdm/internal/inspector"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/schema"
	"github.com/vibemdm/vibemdm/internal/store"
)

func (h *Handler) listMobilePayloads(w http.ResponseWriter, r *http.Request) {
	writePayloadEditors(w, r, schema.PlatformIOS)
}

type mobileProfileReq struct {
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	Category    string                      `json:"category"`
	Scope       model.MobileScope           `json:"scope"`
	Payloads    []model.MobilePayloadValues `json:"payloads"`
	// Payload is an uploaded .mobileconfig, used when Payloads is empty.
	Payload string `json:"payload"`
}

// buildMobilePayload renders the editor payloads; perrs is set when values are invalid.
func (h *Handler) buildMobilePayload(ctx context.Context, identifier, name, description string, payloads []model.MobilePayloadValues) (string, []schema.PayloadErrors, error) {
	return h.buildPayloadXML(ctx, schema.PlatformIOS, identifier, name, description, "", payloads)
}

func (h *Handler) previewMobileProfile(w http.ResponseWriter, r *http.Request) {
	var req mobileProfileReq
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	xml, perrs, err := h.buildMobilePayload(r.Context(), "com.vibemdm.mobile.preview", firstNonEmpty(req.Name, "New profile"), req.Description, req.Payloads)
	if err != nil {
		writeJSON(w, 200, map[string]any{"payload": "", "error": err.Error(), "payload_errors": []schema.PayloadErrors{}})
		return
	}
	if perrs == nil {
		perrs = []schema.PayloadErrors{}
	}
	writeJSON(w, 200, map[string]any{"payload": xml, "payload_errors": perrs})
}

// fillMobileProfile builds or normalizes the XML for a create or update. It
// writes the error response itself and returns false on failure.
func (h *Handler) fillMobileProfile(w http.ResponseWriter, r *http.Request, p *model.MobileConfigProfile, req mobileProfileReq, existing *model.MobileConfigProfile) bool {
	switch {
	case len(req.Payloads) > 0:
		xml, perrs, err := h.buildMobilePayload(r.Context(), p.Identifier, p.Name, p.Description, req.Payloads)
		if err != nil {
			writeErr(w, 400, err.Error())
			return false
		}
		if len(perrs) > 0 {
			writeJSON(w, 400, map[string]any{"error": "some values are invalid", "payload_errors": perrs})
			return false
		}
		p.Payload, p.Payloads = xml, req.Payloads
		p.UUID = ""
	case strings.TrimSpace(req.Payload) != "":
		cp := model.ConfigProfile{Name: p.Name, Description: p.Description, Identifier: p.Identifier, Payload: req.Payload}
		if err := normalizeProfilePayload(&cp); err != nil {
			writeErr(w, 400, err.Error())
			return false
		}
		p.Name, p.Description, p.Identifier, p.UUID, p.Payload, p.Payloads = cp.Name, cp.Description, cp.Identifier, cp.UUID, cp.Payload, nil
		return true
	case existing != nil:
		// Scope or name change only: keep the payload. Editor profiles are rebuilt so the name stays in sync.
		if existing.Payloads != nil {
			req.Payloads = existing.Payloads
			return h.fillMobileProfile(w, r, p, req, nil)
		}
		p.Payload, p.UUID, p.Payloads = existing.Payload, existing.UUID, nil
		return true
	default:
		writeErr(w, 400, "add at least one payload or upload a .mobileconfig")
		return false
	}
	if p.UUID == "" {
		p.UUID = stableProfileUUID(p.Payload)
	}
	return true
}

// stableProfileUUID reads the top-level PayloadUUID the builder set.
func stableProfileUUID(xml string) string {
	cp := model.ConfigProfile{Payload: xml}
	if normalizeProfilePayload(&cp) == nil {
		return cp.UUID
	}
	return ""
}

func (h *Handler) createMobileProfile(w http.ResponseWriter, r *http.Request) {
	var req mobileProfileReq
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p := model.MobileConfigProfile{Name: strings.TrimSpace(req.Name), Description: req.Description, Category: strings.TrimSpace(req.Category), Scope: req.Scope}
	if len(req.Payloads) > 0 {
		if p.Name == "" {
			writeErr(w, 400, "profile name required")
			return
		}
		p.Identifier = "com.vibemdm.mobile." + strings.Split(uuid.NewString(), "-")[0]
	}
	if !h.fillMobileProfile(w, r, &p, req, nil) {
		return
	}
	out, err := h.st.CreateMobileProfile(r.Context(), p)
	if err != nil {
		h.fail(w, err, "mobile profile")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_profile.create", "mobile_profile", out.ID, map[string]any{"name": out.Name})
	h.syncMobileProfileAsync(out, actor(r), false)
	writeJSON(w, 201, out)
}

func (h *Handler) updateMobileProfile(w http.ResponseWriter, r *http.Request) {
	var req mobileProfileReq
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	existing, err := h.st.GetMobileProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "mobile profile")
		return
	}
	p := model.MobileConfigProfile{ID: existing.ID, Name: firstNonEmpty(strings.TrimSpace(req.Name), existing.Name), Description: req.Description,
		Category: strings.TrimSpace(req.Category), Identifier: existing.Identifier, Scope: req.Scope}
	if !h.fillMobileProfile(w, r, &p, req, existing) {
		return
	}
	out, err := h.st.UpdateMobileProfile(r.Context(), p)
	if err != nil {
		h.fail(w, err, "mobile profile")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_profile.update", "mobile_profile", out.ID, map[string]any{"name": out.Name})
	h.syncMobileProfileAsync(out, actor(r), existing.Payload != out.Payload)
	writeJSON(w, 200, out)
}

func (h *Handler) syncMobileProfileAsync(p *model.MobileConfigProfile, by string, reinstall bool) {
	if h.cmdr == nil {
		return
	}
	prof := *p
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if _, _, err := h.cmdr.SyncMobileProfile(ctx, &prof, by, reinstall); err != nil {
			h.log.Warn("mobile profile sync", "profile", prof.Name, "err", err)
		}
	}()
}

type mobileProfileWithStatus struct {
	model.MobileConfigProfile
	Status map[string]int `json:"status"`
}

func (h *Handler) listMobileProfiles(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListMobileProfiles(r.Context())
	if err != nil {
		h.fail(w, err, "mobile profiles")
		return
	}
	counts, _ := h.st.MobileDeploymentStatusCounts(r.Context())
	out := make([]mobileProfileWithStatus, 0, len(items))
	for _, p := range items {
		st := counts[p.ID]
		if st == nil {
			st = map[string]int{}
		}
		redactMobileProfileFor(r, &p)
		out = append(out, mobileProfileWithStatus{MobileConfigProfile: p, Status: st})
	}
	writeJSON(w, 200, listResp[mobileProfileWithStatus]{Items: out, Total: len(out)})
}

func (h *Handler) getMobileProfile(w http.ResponseWriter, r *http.Request) {
	p, err := h.st.GetMobileProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "mobile profile")
		return
	}
	counts, _ := h.st.MobileDeploymentStatusCounts(r.Context())
	st := counts[p.ID]
	if st == nil {
		st = map[string]int{}
	}
	redactMobileProfileFor(r, p)
	writeJSON(w, 200, mobileProfileWithStatus{MobileConfigProfile: *p, Status: st})
}

func (h *Handler) deleteMobileProfile(w http.ResponseWriter, r *http.Request) {
	p, err := h.st.GetMobileProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "mobile profile")
		return
	}
	deps, _ := h.st.ListMobileDeployments(r.Context(), p.ID, "")
	removed := 0
	for _, d := range deps {
		if d.Status == "removed" || d.Status == "failed" || h.cmdr == nil { // nothing installed to remove
			continue
		}
		dev, err := h.st.GetMobileDevice(r.Context(), d.MobileDeviceID)
		if err != nil || !dev.MDMEnrolled {
			continue
		}
		if err := h.cmdr.RemoveMobileProfile(r.Context(), dev, p, actor(r)); err == nil {
			removed++
		}
	}
	if err := h.st.DeleteMobileProfile(r.Context(), p.ID); err != nil {
		h.fail(w, err, "mobile profile")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_profile.delete", "mobile_profile", p.ID, map[string]any{"name": p.Name, "removed_from": removed})
	writeJSON(w, 200, map[string]any{"ok": true, "removed_from": removed})
}

func (h *Handler) mobileProfileDeployments(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListMobileDeployments(r.Context(), r.PathValue("id"), "")
	if err != nil {
		h.fail(w, err, "deployments")
		return
	}
	writeJSON(w, 200, listResp[model.MobileProfileDeployment]{Items: items, Total: len(items)})
}

func (h *Handler) mobileDeviceProfileDeployments(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListMobileDeployments(r.Context(), "", r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "deployments")
		return
	}
	writeJSON(w, 200, listResp[model.MobileProfileDeployment]{Items: items, Total: len(items)})
}

func (h *Handler) deployMobileProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reinstall bool `json:"reinstall"`
	}
	decode(r, &req)
	p, err := h.st.GetMobileProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "mobile profile")
		return
	}
	if h.cmdr == nil {
		writeErr(w, 409, "MDM is not configured")
		return
	}
	installed, removed, err := h.cmdr.SyncMobileProfile(r.Context(), p, actor(r), req.Reinstall)
	if err != nil {
		h.fail(w, err, "deploy")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_profile.deploy", "mobile_profile", p.ID, map[string]any{"installed": installed, "removed": removed})
	writeJSON(w, 200, map[string]any{"installed": installed, "removed": removed})
}

func (h *Handler) mobileScopePreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scope model.MobileScope `json:"scope"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ids, err := h.st.ResolveMobileScope(r.Context(), req.Scope)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	devices := []model.MobileDevice{}
	if len(ids) > 0 {
		if devices, _, err = h.st.ListMobileDevices(r.Context(), store.MobileDeviceFilter{IDs: ids, Limit: 200}); err != nil {
			h.fail(w, err, "preview")
			return
		}
	}
	writeJSON(w, 200, map[string]any{"count": len(ids), "mobile_devices": devices})
}

// redactMobileProfileFor hides passwords, shared secrets and identity files from viewers.
func redactMobileProfileFor(r *http.Request, p *model.MobileConfigProfile) {
	if isAdmin(r) {
		return
	}
	if p.Payload != "" {
		p.Payload = inspector.Redact(p.Payload)
	}
	redactPayloads(schema.PlatformIOS, p.Payloads)
}
