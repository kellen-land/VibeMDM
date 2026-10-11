package api

import (
	"net/http"
	"strings"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Mobile devices (iPhone, iPad) have their own API, parallel to computers, as in Jamf Pro.

func mobileFilterFrom(r *http.Request, limit int) store.MobileDeviceFilter {
	q := r.URL.Query()
	f := store.MobileDeviceFilter{Query: q.Get("q"), GroupID: q.Get("group_id"), Sort: q.Get("sort"), Desc: q.Get("order") == "desc", Limit: limit, Offset: qInt(r, "offset", 0)}
	if ids := q.Get("ids"); ids != "" {
		f.IDs = strings.Split(ids, ",")
	}
	return f
}

func (h *Handler) listMobileDevices(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListMobileDevices(r.Context(), mobileFilterFrom(r, qInt(r, "limit", 50)))
	if err != nil {
		h.fail(w, err, "mobile devices")
		return
	}
	writeJSON(w, 200, listResp[model.MobileDevice]{Items: items, Total: total})
}

func (h *Handler) getMobileDevice(w http.ResponseWriter, r *http.Request) {
	d, err := h.st.GetMobileDevice(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "mobile device")
		return
	}
	out := model.MobileDeviceDetail{MobileDevice: *d}
	if out.Apps, err = h.st.ListMobileApps(r.Context(), d.ID); err != nil {
		h.fail(w, err, "mobile device")
		return
	}
	if out.Profiles, err = h.st.ListMobileProfiles(r.Context(), d.ID); err != nil {
		h.fail(w, err, "mobile device")
		return
	}
	if out.Groups, err = h.st.MobileDeviceGroups(r.Context(), d.ID); err != nil {
		h.fail(w, err, "mobile device")
		return
	}
	writeJSON(w, 200, out)
}

func (h *Handler) updateMobileDevice(w http.ResponseWriter, r *http.Request) {
	var e store.MobileDeviceEdit
	if err := decode(r, &e); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := h.st.UpdateMobileDevice(r.Context(), r.PathValue("id"), e); err != nil {
		h.fail(w, err, "mobile device")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_device.update", "mobile_device", r.PathValue("id"), nil)
	h.getMobileDevice(w, r)
}

func (h *Handler) deleteMobileDevice(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteMobileDevice(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "mobile device")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_device.delete", "mobile_device", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) mobileDeviceCommands(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListCommands(r.Context(), store.CommandFilter{MobileDeviceID: r.PathValue("id"), Status: r.URL.Query().Get("status"),
		Limit: qInt(r, "limit", 50), Offset: qInt(r, "offset", 0)})
	if err != nil {
		h.fail(w, err, "commands")
		return
	}
	writeJSON(w, 200, listResp[model.MDMCommand]{Items: items, Total: total})
}

func (h *Handler) mobileUpdateInventory(w http.ResponseWriter, r *http.Request) {
	d, err := h.st.GetMobileDevice(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "mobile device")
		return
	}
	if err := h.cmdr.RequestMobileInventory(r.Context(), d, actor(r)); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_device.update_inventory", "mobile_device", d.ID, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- mobile device groups ----

func (h *Handler) listMobileGroups(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListMobileGroups(r.Context())
	if err != nil {
		h.fail(w, err, "groups")
		return
	}
	writeJSON(w, 200, listResp[model.ComputerGroup]{Items: items, Total: len(items)})
}

func (h *Handler) mobileCriteriaFields(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, h.st.MobileCriterionFields())
}

func (h *Handler) previewMobileGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Criteria []model.Criterion `json:"criteria"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ids, err := h.st.PreviewMobileCriteria(r.Context(), req.Criteria)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	devices := []model.MobileDevice{}
	if len(ids) > 0 {
		if devices, _, err = h.st.ListMobileDevices(r.Context(), store.MobileDeviceFilter{IDs: ids, Limit: 500}); err != nil {
			h.fail(w, err, "preview")
			return
		}
	}
	writeJSON(w, 200, map[string]any{"count": len(ids), "mobile_devices": devices})
}

func (h *Handler) getMobileGroup(w http.ResponseWriter, r *http.Request) {
	g, err := h.st.GetMobileGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "group")
		return
	}
	writeJSON(w, 200, g)
}

func (h *Handler) createMobileGroup(w http.ResponseWriter, r *http.Request) {
	var g model.ComputerGroup
	if err := decode(r, &g); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(g.Name) == "" {
		writeErr(w, 400, "name required")
		return
	}
	out, err := h.st.CreateMobileGroup(r.Context(), g)
	if err != nil {
		h.fail(w, err, "group")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_group.create", "mobile_group", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 201, out)
}

func (h *Handler) updateMobileGroup(w http.ResponseWriter, r *http.Request) {
	var g model.ComputerGroup
	if err := decode(r, &g); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	g.ID = r.PathValue("id")
	out, err := h.st.UpdateMobileGroup(r.Context(), g)
	if err != nil {
		h.fail(w, err, "group")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_group.update", "mobile_group", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 200, out)
}

func (h *Handler) deleteMobileGroup(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteMobileGroup(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "group")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_group.delete", "mobile_group", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) mobileGroupMembers(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListMobileDevices(r.Context(), store.MobileDeviceFilter{GroupID: r.PathValue("id"), Limit: qInt(r, "limit", 500),
		Offset: qInt(r, "offset", 0), Sort: r.URL.Query().Get("sort")})
	if err != nil {
		h.fail(w, err, "members")
		return
	}
	writeJSON(w, 200, listResp[model.MobileDevice]{Items: items, Total: total})
}

func (h *Handler) setMobileGroupMembers(w http.ResponseWriter, r *http.Request) {
	g, err := h.st.GetMobileGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "group")
		return
	}
	if g.IsSmart {
		writeErr(w, 400, "smart group membership is computed from criteria")
		return
	}
	var req struct {
		MobileDeviceIDs []string `json:"mobile_device_ids"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := h.st.SetMobileStaticMembers(r.Context(), g.ID, req.MobileDeviceIDs); err != nil {
		h.fail(w, err, "members")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_group.members", "mobile_group", g.ID, map[string]any{"count": len(req.MobileDeviceIDs)})
	writeJSON(w, 200, map[string]any{"ok": true, "count": len(req.MobileDeviceIDs)})
}
