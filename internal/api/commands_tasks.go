package api

import (
	"net/http"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

func (h *Handler) listCommands(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, total, err := h.st.ListCommands(r.Context(), store.CommandFilter{ComputerID: q.Get("computer_id"), Status: q.Get("status"), Limit: qInt(r, "limit", 50), Offset: qInt(r, "offset", 0)})
	if err != nil {
		h.fail(w, err, "commands")
		return
	}
	writeJSON(w, 200, listResp[model.MDMCommand]{Items: items, Total: total})
}

func (h *Handler) getCommand(w http.ResponseWriter, r *http.Request) {
	c, err := h.st.GetCommand(r.Context(), r.PathValue("uuid"))
	if err != nil {
		h.fail(w, err, "command")
		return
	}
	writeJSON(w, 200, c)
}

func (h *Handler) cancelCommand(w http.ResponseWriter, r *http.Request) {
	if err := h.st.CancelCommand(r.Context(), r.PathValue("uuid")); err != nil {
		h.fail(w, err, "command")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mdm.command_cancel", "command", r.PathValue("uuid"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, total, err := h.st.ListTasks(r.Context(), store.TaskFilter{ComputerID: q.Get("computer_id"), Status: q.Get("status"), Type: q.Get("type"), Limit: qInt(r, "limit", 50), Offset: qInt(r, "offset", 0)})
	if err != nil {
		h.fail(w, err, "tasks")
		return
	}
	writeJSON(w, 200, listResp[model.AgentTask]{Items: items, Total: total})
}

func (h *Handler) getTask(w http.ResponseWriter, r *http.Request) {
	t, err := h.st.GetTask(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "task")
		return
	}
	writeJSON(w, 200, t)
}

func (h *Handler) cancelTask(w http.ResponseWriter, r *http.Request) {
	if err := h.st.CancelTask(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "task")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
