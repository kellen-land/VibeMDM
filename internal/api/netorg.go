package api

import (
	"net"
	"net/http"
	"strings"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Network organization: buildings, departments, network segments — the managed
// vocabulary the unified scope model (targets/limitations/exclusions) refers to.

func (h *Handler) listBuildings(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListBuildings(r.Context())
	if err != nil {
		h.fail(w, err, "buildings")
		return
	}
	writeJSON(w, 200, listResp[model.Building]{Items: items, Total: len(items)})
}

func (h *Handler) listDepartments(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListDepartments(r.Context())
	if err != nil {
		h.fail(w, err, "departments")
		return
	}
	writeJSON(w, 200, listResp[model.Department]{Items: items, Total: len(items)})
}

func (h *Handler) namedCRUD(table, col, what string) (create, rename, del http.HandlerFunc) {
	create = func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := decode(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
			writeErr(w, 400, "name required")
			return
		}
		id, err := h.st.CreateNamed(r.Context(), table, req.Name)
		if err != nil {
			h.fail(w, err, what)
			return
		}
		h.st.Audit(r.Context(), actor(r), what+".create", what, id, map[string]any{"name": req.Name})
		writeJSON(w, 201, map[string]string{"id": id, "name": strings.TrimSpace(req.Name)})
	}
	rename = func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := decode(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
			writeErr(w, 400, "name required")
			return
		}
		if err := h.st.RenameNamed(r.Context(), table, col, r.PathValue("id"), req.Name); err != nil {
			h.fail(w, err, what)
			return
		}
		h.st.Audit(r.Context(), actor(r), what+".rename", what, r.PathValue("id"), map[string]any{"name": req.Name})
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
	del = func(w http.ResponseWriter, r *http.Request) {
		if err := h.st.DeleteNamed(r.Context(), table, r.PathValue("id")); err != nil {
			h.fail(w, err, what)
			return
		}
		h.st.Audit(r.Context(), actor(r), what+".delete", what, r.PathValue("id"), nil)
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
	return
}

func validSegment(n *model.NetworkSegment) string {
	if strings.TrimSpace(n.Name) == "" {
		return "name required"
	}
	start, end := net.ParseIP(strings.TrimSpace(n.StartingAddress)), net.ParseIP(strings.TrimSpace(n.EndingAddress))
	if start == nil || end == nil {
		return "starting_address and ending_address must be valid IP addresses"
	}
	n.StartingAddress, n.EndingAddress = start.String(), end.String()
	return ""
}

func (h *Handler) listSegments(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListNetworkSegments(r.Context())
	if err != nil {
		h.fail(w, err, "network segments")
		return
	}
	writeJSON(w, 200, listResp[model.NetworkSegment]{Items: items, Total: len(items)})
}

func (h *Handler) createSegment(w http.ResponseWriter, r *http.Request) {
	var n model.NetworkSegment
	if err := decode(r, &n); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if msg := validSegment(&n); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	out, err := h.st.CreateNetworkSegment(r.Context(), n)
	if err != nil {
		h.fail(w, err, "network segment")
		return
	}
	h.st.Audit(r.Context(), actor(r), "network_segment.create", "network_segment", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 201, out)
}

func (h *Handler) updateSegment(w http.ResponseWriter, r *http.Request) {
	var n model.NetworkSegment
	if err := decode(r, &n); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	n.ID = r.PathValue("id")
	if msg := validSegment(&n); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	out, err := h.st.UpdateNetworkSegment(r.Context(), n)
	if err != nil {
		h.fail(w, err, "network segment")
		return
	}
	h.st.Audit(r.Context(), actor(r), "network_segment.update", "network_segment", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 200, out)
}

func (h *Handler) deleteSegment(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteNetworkSegment(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "network segment")
		return
	}
	h.st.Audit(r.Context(), actor(r), "network_segment.delete", "network_segment", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// scopePreview resolves an arbitrary scope to its member computers.
func (h *Handler) scopePreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scope model.Scope `json:"scope"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ids, err := h.st.ResolveScope(r.Context(), req.Scope)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	comps := []model.Computer{}
	if len(ids) > 0 {
		comps, _, err = h.st.ListComputers(r.Context(), store.ComputerFilter{IDs: ids, Limit: 500})
		if err != nil {
			h.fail(w, err, "preview")
			return
		}
	}
	writeJSON(w, 200, map[string]any{"count": len(ids), "computers": comps})
}
