package api

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

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

// ---- mobile device searches ----

// mobileCSVFields are the columns a mobile device search can show and export.
var mobileCSVFields = []struct {
	Key   string
	Label string
	Get   func(d *model.MobileDevice) string
}{
	{"name", "Name", func(d *model.MobileDevice) string { return d.Name }},
	{"serial_number", "Serial Number", func(d *model.MobileDevice) string { return d.SerialNumber }},
	{"udid", "UDID", func(d *model.MobileDevice) string { return d.UDID }},
	{"device_type", "Device Type", func(d *model.MobileDevice) string { return d.DeviceType }},
	{"model_name", "Model", func(d *model.MobileDevice) string { return d.ModelName }},
	{"model", "Model Identifier", func(d *model.MobileDevice) string { return d.Model }},
	{"os_version", "OS Version", func(d *model.MobileDevice) string { return d.OSVersion }},
	{"capacity_gb", "Capacity (GB)", func(d *model.MobileDevice) string { return fmtFloat(d.CapacityGB) }},
	{"available_gb", "Available (GB)", func(d *model.MobileDevice) string { return fmtFloat(d.AvailableGB) }},
	{"phone_number", "Phone Number", func(d *model.MobileDevice) string { return d.PhoneNumber }},
	{"imei", "IMEI", func(d *model.MobileDevice) string { return d.IMEI }},
	{"carrier", "Carrier", func(d *model.MobileDevice) string { return d.Carrier }},
	{"is_supervised", "Supervised", func(d *model.MobileDevice) string { return fmtBool(d.IsSupervised) }},
	{"passcode_present", "Passcode Set", func(d *model.MobileDevice) string { return fmtBool(d.PasscodePresent) }},
	{"lost_mode_enabled", "Lost Mode", func(d *model.MobileDevice) string { return fmtBool(d.LostModeEnabled) }},
	{"assigned_user", "Assigned User", func(d *model.MobileDevice) string { return d.AssignedUser }},
	{"department", "Department", func(d *model.MobileDevice) string { return d.Department }},
	{"building", "Building", func(d *model.MobileDevice) string { return d.Building }},
	{"asset_tag", "Asset Tag", func(d *model.MobileDevice) string { return d.AssetTag }},
	{"last_checkin", "Last Check-in", func(d *model.MobileDevice) string { return fmtTime(d.MDMLastCheckin) }},
	{"last_inventory", "Last Inventory", func(d *model.MobileDevice) string { return fmtTime(d.LastInventoryAt) }},
}

func fmtFloat(f *float64) string {
	if f == nil {
		return ""
	}
	return strconv.FormatFloat(*f, 'f', -1, 64)
}

func fmtBool(b *bool) string {
	if b == nil {
		return ""
	}
	return strconv.FormatBool(*b)
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (h *Handler) mobileSearchFields(w http.ResponseWriter, r *http.Request) {
	type opt struct{ Key, Label string }
	out := make([]opt, len(mobileCSVFields))
	for i, f := range mobileCSVFields {
		out[i] = opt{f.Key, f.Label}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) listMobileSearches(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListMobileSearches(r.Context())
	if err != nil {
		h.fail(w, err, "searches")
		return
	}
	writeJSON(w, 200, listResp[model.AdvancedSearch]{Items: items, Total: len(items)})
}

func (h *Handler) getMobileSearch(w http.ResponseWriter, r *http.Request) {
	it, err := h.st.GetMobileSearch(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "search")
		return
	}
	writeJSON(w, 200, it)
}

func (h *Handler) saveMobileSearch(w http.ResponseWriter, r *http.Request) {
	var a model.AdvancedSearch
	if err := decode(r, &a); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(a.Name) == "" {
		writeErr(w, 400, "name required")
		return
	}
	if _, err := h.st.PreviewMobileCriteria(r.Context(), a.Criteria); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var out *model.AdvancedSearch
	var err error
	if id := r.PathValue("id"); id != "" {
		a.ID = id
		out, err = h.st.UpdateMobileSearch(r.Context(), a)
	} else {
		out, err = h.st.CreateMobileSearch(r.Context(), a)
	}
	if err != nil {
		h.fail(w, err, "search")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_search.save", "mobile_search", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 200, out)
}

func (h *Handler) deleteMobileSearch(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteMobileSearch(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "search")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mobile_search.delete", "mobile_search", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) runMobileSearch(w http.ResponseWriter, r *http.Request) {
	it, err := h.st.GetMobileSearch(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "search")
		return
	}
	devices, err := h.st.RunMobileSearch(r.Context(), it.Criteria, qInt(r, "limit", 500))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"search": it, "count": len(devices), "mobile_devices": devices})
}

func (h *Handler) exportMobileSearchCSV(w http.ResponseWriter, r *http.Request) {
	it, err := h.st.GetMobileSearch(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "search")
		return
	}
	devices, err := h.st.RunMobileSearch(r.Context(), it.Criteria, 10000)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cols := mobileCSVFields[:0:0]
	for _, key := range it.DisplayFields {
		for _, f := range mobileCSVFields {
			if f.Key == key {
				cols = append(cols, f)
			}
		}
	}
	if len(cols) == 0 {
		cols = mobileCSVFields[:7]
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="mobile-devices.csv"`)
	cw := csv.NewWriter(w)
	head := make([]string, len(cols))
	for i, c := range cols {
		head[i] = c.Label
	}
	cw.Write(head)
	for i := range devices {
		row := make([]string, len(cols))
		for j, c := range cols {
			row[j] = c.Get(&devices[i])
		}
		cw.Write(csvSafe(row))
	}
	cw.Flush()
	h.st.Audit(r.Context(), actor(r), "mobile_search.export", "mobile_search", it.ID, map[string]any{"rows": len(devices)})
}
