package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/agentapi"
	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/webhook"
)

// ---- webhooks ----

func (h *Handler) listWebhooks(w http.ResponseWriter, r *http.Request) {
	rows, err := h.st.DB.QueryContext(r.Context(), `SELECT id, name, url, secret, enabled, events, created_at, updated_at FROM webhooks ORDER BY lower(name)`)
	if err != nil {
		h.fail(w, err, "webhooks")
		return
	}
	defer rows.Close()
	items := []webhook.Hook{}
	for rows.Next() {
		var hk webhook.Hook
		var ev db.JSON[[]string]
		if err := rows.Scan(&hk.ID, &hk.Name, &hk.URL, &hk.Secret, &hk.Enabled, &ev, &hk.CreatedAt, &hk.UpdatedAt); err != nil {
			h.fail(w, err, "webhooks")
			return
		}
		hk.Events = ev.V
		if hk.Secret != "" {
			hk.Secret = "********"
		}
		items = append(items, hk)
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items), "available_events": webhook.AllEvents})
}

type webhookReq struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Secret  *string  `json:"secret"`
	Enabled bool     `json:"enabled"`
	Events  []string `json:"events"`
}

func (h *Handler) createWebhook(w http.ResponseWriter, r *http.Request) {
	var req webhookReq
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" || !strings.HasPrefix(req.URL, "http") {
		writeErr(w, 400, "name and an http(s) url are required")
		return
	}
	if len(req.Events) == 0 {
		req.Events = []string{"*"}
	}
	secret := ""
	if req.Secret != nil {
		secret = *req.Secret
	}
	var id string
	err := h.st.DB.QueryRowContext(r.Context(), `INSERT INTO webhooks (name, url, secret, enabled, events) VALUES ($1,$2,$3,$4,$5::jsonb) RETURNING id`,
		req.Name, req.URL, secret, req.Enabled, db.MustMarshal(req.Events)).Scan(&id)
	if err != nil {
		h.fail(w, err, "webhook")
		return
	}
	h.hooks.Invalidate()
	h.st.Audit(r.Context(), actor(r), "webhook.create", "webhook", id, map[string]any{"name": req.Name, "url": req.URL})
	writeJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) updateWebhook(w http.ResponseWriter, r *http.Request) {
	var req webhookReq
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(req.Events) == 0 {
		req.Events = []string{"*"}
	}
	q := `UPDATE webhooks SET name=$2, url=$3, enabled=$4, events=$5::jsonb, updated_at=now() WHERE id=$1`
	args := []any{r.PathValue("id"), req.Name, req.URL, req.Enabled, db.MustMarshal(req.Events)}
	if req.Secret != nil { // only replace the secret when explicitly provided
		q = `UPDATE webhooks SET name=$2, url=$3, enabled=$4, events=$5::jsonb, secret=$6, updated_at=now() WHERE id=$1`
		args = append(args, *req.Secret)
	}
	res, err := h.st.DB.ExecContext(r.Context(), q, args...)
	if err != nil {
		h.fail(w, err, "webhook")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 404, "webhook not found")
		return
	}
	h.hooks.Invalidate()
	h.st.Audit(r.Context(), actor(r), "webhook.update", "webhook", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	h.st.DB.ExecContext(r.Context(), `DELETE FROM webhooks WHERE id=$1`, r.PathValue("id"))
	h.hooks.Invalidate()
	h.st.Audit(r.Context(), actor(r), "webhook.delete", "webhook", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) testWebhook(w http.ResponseWriter, r *http.Request) {
	h.hooks.Invalidate()
	h.hooks.Fire("Test", map[string]any{"message": "VibeMDM webhook test", "fired_by": actor(r), "at": time.Now().UTC()})
	writeJSON(w, 200, map[string]any{"ok": true, "note": "a Test event was queued to all enabled webhooks subscribed to it or to *"})
}

func (h *Handler) webhookDeliveries(w http.ResponseWriter, r *http.Request) {
	rows, err := h.st.DB.QueryContext(r.Context(), `SELECT d.id, d.webhook_id, wh.name, d.event, d.status, d.error, d.attempts, d.at
		FROM webhook_deliveries d JOIN webhooks wh ON wh.id = d.webhook_id ORDER BY d.at DESC LIMIT 200`)
	if err != nil {
		h.fail(w, err, "deliveries")
		return
	}
	defer rows.Close()
	type delivery struct {
		ID       int64     `json:"id"`
		HookID   string    `json:"webhook_id"`
		HookName string    `json:"webhook_name"`
		Event    string    `json:"event"`
		Status   int       `json:"status"`
		Error    string    `json:"error"`
		Attempts int       `json:"attempts"`
		At       time.Time `json:"at"`
	}
	items := []delivery{}
	for rows.Next() {
		var d delivery
		if err := rows.Scan(&d.ID, &d.HookID, &d.HookName, &d.Event, &d.Status, &d.Error, &d.Attempts, &d.At); err != nil {
			h.fail(w, err, "deliveries")
			return
		}
		items = append(items, d)
	}
	writeJSON(w, 200, listResp[delivery]{Items: items, Total: len(items)})
}

// ---- restricted software ----

func (h *Handler) listRestricted(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListRestrictedSoftware(r.Context())
	if err != nil {
		h.fail(w, err, "restricted software")
		return
	}
	writeJSON(w, 200, listResp[model.RestrictedSoftware]{Items: items, Total: len(items)})
}

func (h *Handler) getRestricted(w http.ResponseWriter, r *http.Request) {
	it, err := h.st.GetRestrictedSoftware(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "restricted software")
		return
	}
	writeJSON(w, 200, it)
}

func (h *Handler) createRestricted(w http.ResponseWriter, r *http.Request) {
	var m model.RestrictedSoftware
	if err := decode(r, &m); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.ProcessName) == "" {
		writeErr(w, 400, "name and process_name are required")
		return
	}
	out, err := h.st.CreateRestrictedSoftware(r.Context(), m)
	if err != nil {
		h.fail(w, err, "restricted software")
		return
	}
	h.st.Audit(r.Context(), actor(r), "restricted.create", "restricted_software", out.ID, map[string]any{"name": out.Name})
	h.wakeScope(out.Scope, out.Enabled)
	writeJSON(w, 201, out)
}

func (h *Handler) updateRestricted(w http.ResponseWriter, r *http.Request) {
	var m model.RestrictedSoftware
	if err := decode(r, &m); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	m.ID = r.PathValue("id")
	out, err := h.st.UpdateRestrictedSoftware(r.Context(), m)
	if err != nil {
		h.fail(w, err, "restricted software")
		return
	}
	h.st.Audit(r.Context(), actor(r), "restricted.update", "restricted_software", out.ID, map[string]any{"name": out.Name})
	h.wakeScope(out.Scope, true)
	writeJSON(w, 200, out)
}

func (h *Handler) deleteRestricted(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteRestrictedSoftware(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "restricted software")
		return
	}
	h.st.Audit(r.Context(), actor(r), "restricted.delete", "restricted_software", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) restrictedEvents(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListRestrictedEvents(r.Context(), r.URL.Query().Get("computer_id"), qInt(r, "limit", 50), qInt(r, "offset", 0))
	if err != nil {
		h.fail(w, err, "restricted events")
		return
	}
	writeJSON(w, 200, listResp[model.RestrictedEvent]{Items: items, Total: total})
}

// ---- advanced searches & CSV export ----

func (h *Handler) listSearches(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListAdvancedSearches(r.Context())
	if err != nil {
		h.fail(w, err, "searches")
		return
	}
	writeJSON(w, 200, listResp[model.AdvancedSearch]{Items: items, Total: len(items)})
}

func (h *Handler) getSearch(w http.ResponseWriter, r *http.Request) {
	it, err := h.st.GetAdvancedSearch(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "search")
		return
	}
	writeJSON(w, 200, it)
}

func (h *Handler) createSearch(w http.ResponseWriter, r *http.Request) {
	var m model.AdvancedSearch
	if err := decode(r, &m); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(m.Name) == "" {
		writeErr(w, 400, "name required")
		return
	}
	out, err := h.st.CreateAdvancedSearch(r.Context(), m)
	if err != nil {
		h.fail(w, err, "search")
		return
	}
	h.st.Audit(r.Context(), actor(r), "search.create", "advanced_search", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 201, out)
}

func (h *Handler) updateSearch(w http.ResponseWriter, r *http.Request) {
	var m model.AdvancedSearch
	if err := decode(r, &m); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	m.ID = r.PathValue("id")
	out, err := h.st.UpdateAdvancedSearch(r.Context(), m)
	if err != nil {
		h.fail(w, err, "search")
		return
	}
	writeJSON(w, 200, out)
}

func (h *Handler) deleteSearch(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteAdvancedSearch(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "search")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) runSearch(w http.ResponseWriter, r *http.Request) {
	it, err := h.st.GetAdvancedSearch(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "search")
		return
	}
	comps, err := h.st.RunAdvancedSearch(r.Context(), it.Criteria, qInt(r, "limit", 500))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"search": it, "count": len(comps), "computers": comps})
}

// csvFields maps export field keys to extractors.
var csvFields = []struct {
	Key   string
	Label string
	Get   func(c *model.Computer) string
}{
	{"name", "Name", func(c *model.Computer) string { return c.Name }},
	{"serial_number", "Serial Number", func(c *model.Computer) string { return c.SerialNumber }},
	{"udid", "UDID", func(c *model.Computer) string { return c.UDID }},
	{"model_name", "Model", func(c *model.Computer) string { return c.ModelName }},
	{"model_identifier", "Model Identifier", func(c *model.Computer) string { return c.ModelIdentifier }},
	{"os_version", "OS Version", func(c *model.Computer) string { return c.OSVersion }},
	{"os_build", "OS Build", func(c *model.Computer) string { return c.OSBuild }},
	{"architecture", "Architecture", func(c *model.Computer) string { return c.Architecture }},
	{"cpu", "Processor", func(c *model.Computer) string { return c.CPU }},
	{"memory_mb", "RAM (MB)", func(c *model.Computer) string { return i64s(c.MemoryMB) }},
	{"disk_total_mb", "Disk (MB)", func(c *model.Computer) string { return i64s(c.DiskTotalMB) }},
	{"disk_free_mb", "Disk Free (MB)", func(c *model.Computer) string { return i64s(c.DiskFreeMB) }},
	{"ip_address", "IP Address", func(c *model.Computer) string { return c.IPAddress }},
	{"mac_address", "MAC Address", func(c *model.Computer) string { return c.MACAddress }},
	{"last_user", "Last User", func(c *model.Computer) string { return c.LastUser }},
	{"assigned_user", "Assigned User", func(c *model.Computer) string { return c.AssignedUser }},
	{"building", "Building", func(c *model.Computer) string { return c.Building }},
	{"department", "Department", func(c *model.Computer) string { return c.Department }},
	{"site", "Site", func(c *model.Computer) string { return c.Site }},
	{"asset_tag", "Asset Tag", func(c *model.Computer) string { return c.AssetTag }},
	{"mdm_enrolled", "MDM Enrolled", func(c *model.Computer) string { return strconv.FormatBool(c.MDMEnrolled) }},
	{"agent_enrolled", "Agent Installed", func(c *model.Computer) string { return strconv.FormatBool(c.AgentEnrolled) }},
	{"agent_version", "Agent Version", func(c *model.Computer) string { return c.AgentVersion }},
	{"filevault_enabled", "FileVault", func(c *model.Computer) string { return bps(c.FileVaultEnabled) }},
	{"sip_enabled", "SIP", func(c *model.Computer) string { return bps(c.SIPEnabled) }},
	{"firewall_enabled", "Firewall", func(c *model.Computer) string { return bps(c.FirewallEnabled) }},
	{"gatekeeper_enabled", "Gatekeeper", func(c *model.Computer) string { return bps(c.GatekeeperEnabled) }},
	{"last_checkin", "Last Check-in", func(c *model.Computer) string {
		t := c.AgentLastCheckin
		if c.MDMLastCheckin != nil && (t == nil || c.MDMLastCheckin.After(*t)) {
			t = c.MDMLastCheckin
		}
		if t == nil {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	}},
	{"last_inventory_at", "Last Inventory", func(c *model.Computer) string {
		if c.LastInventoryAt == nil {
			return ""
		}
		return c.LastInventoryAt.UTC().Format(time.RFC3339)
	}},
}

func i64s(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func bps(v *bool) string {
	if v == nil {
		return ""
	}
	return strconv.FormatBool(*v)
}

// DefaultCSVFields is used when a search has no display fields.
var defaultCSVKeys = []string{"name", "serial_number", "model_name", "os_version", "last_user", "building", "department", "mdm_enrolled", "agent_enrolled", "filevault_enabled", "last_checkin"}

func (h *Handler) writeComputersCSV(w http.ResponseWriter, filename string, comps []model.Computer, keys []string) {
	if len(keys) == 0 {
		keys = defaultCSVKeys
	}
	sel := []int{}
	for _, k := range keys {
		for i, f := range csvFields {
			if f.Key == k {
				sel = append(sel, i)
				break
			}
		}
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	cw := csv.NewWriter(w)
	head := make([]string, len(sel))
	for i, fi := range sel {
		head[i] = csvFields[fi].Label
	}
	cw.Write(head)
	for i := range comps {
		row := make([]string, len(sel))
		for j, fi := range sel {
			row[j] = csvFields[fi].Get(&comps[i])
		}
		cw.Write(row)
	}
	cw.Flush()
}

func (h *Handler) exportSearchCSV(w http.ResponseWriter, r *http.Request) {
	it, err := h.st.GetAdvancedSearch(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "search")
		return
	}
	comps, err := h.st.RunAdvancedSearch(r.Context(), it.Criteria, 10000)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.writeComputersCSV(w, strings.ReplaceAll(it.Name, " ", "_")+".csv", comps, it.DisplayFields)
}

func (h *Handler) exportComputersCSV(w http.ResponseWriter, r *http.Request) {
	comps, _, err := h.st.ListComputers(r.Context(), computerFilterFrom(r, 10000))
	if err != nil {
		h.fail(w, err, "computers")
		return
	}
	h.writeComputersCSV(w, "computers.csv", comps, strings.FieldsFunc(r.URL.Query().Get("fields"), func(c rune) bool { return c == ',' }))
}

func (h *Handler) searchFieldOptions(w http.ResponseWriter, r *http.Request) {
	type opt struct{ Key, Label string }
	out := make([]opt, len(csvFields))
	for i, f := range csvFields {
		out[i] = opt{f.Key, f.Label}
	}
	writeJSON(w, 200, out)
}

// ---- mac apps (Installomator catalog) ----

func (h *Handler) listApps(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListMacApps(r.Context())
	if err != nil {
		h.fail(w, err, "apps")
		return
	}
	type appWithStatus struct {
		model.MacApp
		Status map[string]int `json:"status"`
	}
	out := make([]appWithStatus, 0, len(items))
	for _, a := range items {
		st, _ := h.st.AppStatusSummary(r.Context(), a.ID)
		if st == nil {
			st = map[string]int{}
		}
		out = append(out, appWithStatus{MacApp: a, Status: st})
	}
	writeJSON(w, 200, listResp[appWithStatus]{Items: out, Total: len(out)})
}

func (h *Handler) appLabels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"labels": agentapi.InstallomatorLabels(), "source": "Installomator v10.8 (Apache-2.0)", "license_url": "/agent/installomator-license"})
}

func (h *Handler) getApp(w http.ResponseWriter, r *http.Request) {
	a, err := h.st.GetMacApp(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "app")
		return
	}
	writeJSON(w, 200, a)
}

func (h *Handler) validAppLabel(label string) bool {
	for _, l := range agentapi.InstallomatorLabels() {
		if l == label {
			return true
		}
	}
	return false
}

func (h *Handler) createApp(w http.ResponseWriter, r *http.Request) {
	var m model.MacApp
	if err := decode(r, &m); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(m.Name) == "" || !h.validAppLabel(strings.TrimSpace(m.Label)) {
		writeErr(w, 400, "name and a valid Installomator label are required")
		return
	}
	out, err := h.st.CreateMacApp(r.Context(), m)
	if err != nil {
		h.fail(w, err, "app")
		return
	}
	h.st.Audit(r.Context(), actor(r), "app.create", "mac_app", out.ID, map[string]any{"name": out.Name, "label": out.Label})
	h.wakeScope(out.Scope, out.Enabled)
	writeJSON(w, 201, out)
}

func (h *Handler) updateApp(w http.ResponseWriter, r *http.Request) {
	var m model.MacApp
	if err := decode(r, &m); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	m.ID = r.PathValue("id")
	if !h.validAppLabel(strings.TrimSpace(m.Label)) {
		writeErr(w, 400, "unknown Installomator label")
		return
	}
	out, err := h.st.UpdateMacApp(r.Context(), m)
	if err != nil {
		h.fail(w, err, "app")
		return
	}
	h.st.Audit(r.Context(), actor(r), "app.update", "mac_app", out.ID, map[string]any{"name": out.Name})
	h.wakeScope(out.Scope, out.Enabled)
	writeJSON(w, 200, out)
}

func (h *Handler) deleteApp(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteMacApp(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "app")
		return
	}
	h.st.Audit(r.Context(), actor(r), "app.delete", "mac_app", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) appLogs(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListAppInstallLogs(r.Context(), r.PathValue("id"), r.URL.Query().Get("computer_id"), qInt(r, "limit", 50), qInt(r, "offset", 0))
	if err != nil {
		h.fail(w, err, "app logs")
		return
	}
	writeJSON(w, 200, listResp[model.AppInstallLog]{Items: items, Total: total})
}

func (h *Handler) allAppLogs(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListAppInstallLogs(r.Context(), "", r.URL.Query().Get("computer_id"), qInt(r, "limit", 50), qInt(r, "offset", 0))
	if err != nil {
		h.fail(w, err, "app logs")
		return
	}
	writeJSON(w, 200, listResp[model.AppInstallLog]{Items: items, Total: total})
}
