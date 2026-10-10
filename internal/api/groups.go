package api

import (
	"net/http"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

func (h *Handler) listGroups(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListGroups(r.Context())
	if err != nil {
		h.fail(w, err, "groups")
		return
	}
	writeJSON(w, 200, listResp[model.ComputerGroup]{Items: items, Total: len(items)})
}

func (h *Handler) criteriaFields(w http.ResponseWriter, r *http.Request) {
	f, err := h.st.CriterionFields(r.Context())
	if err != nil {
		h.fail(w, err, "criteria")
		return
	}
	writeJSON(w, 200, f)
}

func (h *Handler) previewGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Criteria []model.Criterion `json:"criteria"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ids, err := h.st.PreviewCriteria(r.Context(), req.Criteria)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var comps []model.Computer
	if len(ids) > 0 {
		comps, _, err = h.st.ListComputers(r.Context(), store.ComputerFilter{IDs: ids, Limit: 500})
		if err != nil {
			h.fail(w, err, "preview")
			return
		}
	}
	if comps == nil {
		comps = []model.Computer{}
	}
	writeJSON(w, 200, map[string]any{"count": len(ids), "computers": comps})
}

func (h *Handler) getGroup(w http.ResponseWriter, r *http.Request) {
	g, err := h.st.GetGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "group")
		return
	}
	writeJSON(w, 200, g)
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	var g model.ComputerGroup
	if err := decode(r, &g); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if g.Name == "" {
		writeErr(w, 400, "name required")
		return
	}
	out, err := h.st.CreateGroup(r.Context(), g)
	if err != nil {
		h.fail(w, err, "group")
		return
	}
	h.st.Audit(r.Context(), actor(r), "group.create", "group", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 201, out)
}

func (h *Handler) updateGroup(w http.ResponseWriter, r *http.Request) {
	var g model.ComputerGroup
	if err := decode(r, &g); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	g.ID = r.PathValue("id")
	out, err := h.st.UpdateGroup(r.Context(), g)
	if err != nil {
		h.fail(w, err, "group")
		return
	}
	h.st.Audit(r.Context(), actor(r), "group.update", "group", out.ID, map[string]any{"name": out.Name})
	writeJSON(w, 200, out)
}

func (h *Handler) deleteGroup(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteGroup(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "group")
		return
	}
	h.st.Audit(r.Context(), actor(r), "group.delete", "group", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) groupMembers(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListComputers(r.Context(), store.ComputerFilter{GroupID: r.PathValue("id"), Limit: qInt(r, "limit", 500), Offset: qInt(r, "offset", 0), Sort: r.URL.Query().Get("sort")})
	if err != nil {
		h.fail(w, err, "members")
		return
	}
	writeJSON(w, 200, listResp[model.Computer]{Items: items, Total: total})
}

func (h *Handler) setGroupMembers(w http.ResponseWriter, r *http.Request) {
	g, err := h.st.GetGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "group")
		return
	}
	if g.IsSmart {
		writeErr(w, 400, "smart group membership is computed from criteria")
		return
	}
	var req struct {
		ComputerIDs []string `json:"computer_ids"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := h.st.SetStaticMembers(r.Context(), g.ID, req.ComputerIDs); err != nil {
		h.fail(w, err, "members")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "count": len(req.ComputerIDs)})
}
