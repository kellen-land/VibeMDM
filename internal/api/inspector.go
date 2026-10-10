package api

import (
	"net/http"

	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/inspector"
	"github.com/vibemdm/vibemdm/internal/model"
)

// wantsReveal reports whether the caller asked for raw payloads and may see them.
// Raw payloads are admin-only and every reveal is audited.
func (h *Handler) wantsReveal(r *http.Request, commandUUID string) bool {
	if r.URL.Query().Get("reveal") != "1" {
		return false
	}
	p := auth.FromContext(r.Context())
	if !p.IsAdmin() {
		return false
	}
	h.st.Audit(r.Context(), p.Username, "mdm.command_reveal", "command", commandUUID, map[string]any{"ip": auth.ClientIP(r)})
	return true
}

// inspectCommand returns a command's full story: summaries, timeline and
// (redacted by default) request/response plists.
func (h *Handler) inspectCommand(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	c, err := h.st.GetCommand(r.Context(), uuid)
	if err != nil {
		h.fail(w, err, "command")
		return
	}
	events, err := h.st.CommandEvents(r.Context(), uuid)
	if err != nil {
		h.fail(w, err, "command events")
		return
	}
	reveal := h.wantsReveal(r, uuid)
	out := map[string]any{
		"command":        c,
		"summary":        inspector.Summary(c.RequestType, []byte(c.Command)),
		"result_summary": inspector.ResultSummary(c.RequestType, c.Status, []byte(c.Result)),
		"events":         events,
		"redacted":       !reveal,
		"can_reveal":     auth.FromContext(r.Context()).IsAdmin(),
	}
	if !reveal {
		cp := *c
		cp.Command, cp.Result = inspector.Redact(c.Command), inspector.Redact(c.Result)
		out["command"] = &cp
	}
	writeJSON(w, 200, out)
}

func (h *Handler) computerTimeline(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ComputerTimeline(r.Context(), r.PathValue("id"), qInt(r, "limit", 100), qInt(r, "offset", 0))
	if err != nil {
		h.fail(w, err, "timeline")
		return
	}
	writeJSON(w, 200, listResp[model.CommandEvent]{Items: items, Total: total})
}

func (h *Handler) searchCommands(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hits, err := h.st.SearchCommands(r.Context(), q.Get("q"), q.Get("computer_id"), qInt(r, "limit", 50))
	if err != nil {
		h.fail(w, err, "search")
		return
	}
	writeJSON(w, 200, map[string]any{"items": hits, "total": len(hits)})
}
