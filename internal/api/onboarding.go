package api

import (
	"net/http"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/onboarding"
)

func (h *Handler) listWorkflows(w http.ResponseWriter, r *http.Request) {
	wfs, err := h.st.ListOnboardingWorkflows(r.Context())
	if err != nil {
		h.fail(w, err, "workflows")
		return
	}
	writeJSON(w, 200, map[string]any{"items": wfs, "field_keys": onboarding.FieldKeys})
}

func (h *Handler) getWorkflow(w http.ResponseWriter, r *http.Request) {
	wf, err := h.st.GetOnboardingWorkflow(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "workflow")
		return
	}
	writeJSON(w, 200, wf)
}

func (h *Handler) saveWorkflow(w http.ResponseWriter, r *http.Request) {
	var wf model.OnboardingWorkflow
	if err := decode(r, &wf); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	wf.ID = r.PathValue("id")
	if wf.Priority == 0 {
		wf.Priority = 100
	}
	if err := onboarding.Validate(r.Context(), h.st, &wf); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	out, err := h.st.SaveOnboardingWorkflow(r.Context(), wf)
	if err != nil {
		h.fail(w, err, "workflow")
		return
	}
	h.st.Audit(r.Context(), actor(r), "onboarding.workflow_save", "onboarding_workflow", out.ID, map[string]any{"name": out.Name})
	code := 200
	if r.PathValue("id") == "" {
		code = 201
	}
	writeJSON(w, code, out)
}

func (h *Handler) deleteWorkflow(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteOnboardingWorkflow(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "workflow")
		return
	}
	h.st.Audit(r.Context(), actor(r), "onboarding.workflow_delete", "onboarding_workflow", r.PathValue("id"), nil)
	w.WriteHeader(204)
}

func (h *Handler) listOnboardingRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.st.ListOnboardingRuns(r.Context(), r.PathValue("id"), r.URL.Query().Get("computer_id"), qInt(r, "limit", 100))
	if err != nil {
		h.fail(w, err, "runs")
		return
	}
	writeJSON(w, 200, runs)
}

func (h *Handler) computerOnboarding(w http.ResponseWriter, r *http.Request) {
	runs, err := h.st.ListOnboardingRuns(r.Context(), "", r.PathValue("id"), 20)
	if err != nil {
		h.fail(w, err, "runs")
		return
	}
	writeJSON(w, 200, runs)
}

// runWorkflow starts a workflow now on specific computers (force re-runs completed ones).
func (h *Handler) runWorkflow(w http.ResponseWriter, r *http.Request) {
	wf, err := h.st.GetOnboardingWorkflow(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "workflow")
		return
	}
	var req struct {
		ComputerIDs []string `json:"computer_ids"`
		Force       bool     `json:"force"`
	}
	if err := decode(r, &req); err != nil || len(req.ComputerIDs) == 0 {
		writeErr(w, 400, "computer_ids required")
		return
	}
	queued, skipped := 0, map[string]string{}
	for _, id := range req.ComputerIDs {
		comp, err := h.st.GetComputer(r.Context(), id)
		if err != nil {
			skipped[id] = "not found"
			continue
		}
		if _, err := h.onboardingSvc.Start(r.Context(), wf, comp, actor(r), req.Force); err != nil {
			skipped[comp.Name] = err.Error()
			continue
		}
		queued++
	}
	h.st.Audit(r.Context(), actor(r), "onboarding.run", "onboarding_workflow", wf.ID, map[string]any{"queued": queued, "force": req.Force})
	writeJSON(w, 200, map[string]any{"queued": queued, "skipped": skipped})
}
