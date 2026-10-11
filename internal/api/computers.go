package api

import (
	"net/http"
	"strings"

	"github.com/micromdm/nanomdm/mdm"

	"github.com/vibemdm/vibemdm/internal/commands"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

func computerFilterFrom(r *http.Request, limit int) store.ComputerFilter {
	q := r.URL.Query()
	f := store.ComputerFilter{Query: q.Get("q"), GroupID: q.Get("group_id"), Sort: q.Get("sort"), Desc: q.Get("order") == "desc", Limit: limit, Offset: qInt(r, "offset", 0)}
	if ids := q.Get("ids"); ids != "" {
		f.IDs = strings.Split(ids, ",")
	}
	return f
}

func (h *Handler) listComputers(w http.ResponseWriter, r *http.Request) {
	f := computerFilterFrom(r, qInt(r, "limit", 50))
	items, total, err := h.st.ListComputers(r.Context(), f)
	if err != nil {
		h.fail(w, err, "computers")
		return
	}
	writeJSON(w, 200, listResp[model.Computer]{Items: items, Total: total})
}

type computerDetail struct {
	model.Computer
	Apps        []model.ComputerApp       `json:"apps"`
	Profiles    []model.ComputerProfile   `json:"profiles"`
	LocalUsers  []model.LocalUser         `json:"local_users"`
	EAValues    []model.EAValue           `json:"extension_attributes"`
	Groups      []model.ComputerGroup     `json:"groups"`
	Deployments []model.ProfileDeployment `json:"profile_deployments"`
	AgentOnline bool                      `json:"agent_online"`
	FileVault   *model.FileVaultStatus    `json:"filevault"`
}

func (h *Handler) getComputer(w http.ResponseWriter, r *http.Request) {
	c, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	d := computerDetail{Computer: *c}
	d.Apps, _ = h.st.ListComputerApps(r.Context(), c.ID)
	d.Profiles, _ = h.st.ListComputerProfiles(r.Context(), c.ID)
	d.LocalUsers, _ = h.st.ListComputerLocalUsers(r.Context(), c.ID)
	d.EAValues, _ = h.st.ListComputerEAValues(r.Context(), c.ID)
	d.Groups, _ = h.st.ComputerGroups(r.Context(), c.ID)
	d.Deployments, _ = h.st.ListDeployments(r.Context(), "", c.ID)
	d.AgentOnline = h.notifier.Waiting(c.ID)
	d.FileVault, _ = h.st.FileVaultStatusFor(r.Context(), c.ID)
	if d.Apps == nil {
		d.Apps = []model.ComputerApp{}
	}
	if d.Groups == nil {
		d.Groups = []model.ComputerGroup{}
	}
	writeJSON(w, 200, d)
}

func (h *Handler) computerInventory(w http.ResponseWriter, r *http.Request) {
	inv, err := h.st.GetComputerInventoryJSON(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	writeJSON(w, 200, inv)
}

func (h *Handler) updateComputer(w http.ResponseWriter, r *http.Request) {
	var e store.ComputerEdit
	if err := decode(r, &e); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := h.st.UpdateComputer(r.Context(), r.PathValue("id"), e); err != nil {
		h.fail(w, err, "computer")
		return
	}
	h.st.Audit(r.Context(), actor(r), "computer.update", "computer", r.PathValue("id"), nil)
	c, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	writeJSON(w, 200, c)
}

func (h *Handler) deleteComputer(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteComputer(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "computer")
		return
	}
	h.st.Audit(r.Context(), actor(r), "computer.delete", "computer", r.PathValue("id"), nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *Handler) computerCommands(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListCommands(r.Context(), store.CommandFilter{ComputerID: r.PathValue("id"), Status: r.URL.Query().Get("status"), Limit: qInt(r, "limit", 50), Offset: qInt(r, "offset", 0)})
	if err != nil {
		h.fail(w, err, "commands")
		return
	}
	writeJSON(w, 200, listResp[model.MDMCommand]{Items: items, Total: total})
}

func (h *Handler) computerPolicyLogs(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListPolicyLogs(r.Context(), store.PolicyLogFilter{ComputerID: r.PathValue("id"), Limit: qInt(r, "limit", 50), Offset: qInt(r, "offset", 0)})
	if err != nil {
		h.fail(w, err, "policy logs")
		return
	}
	writeJSON(w, 200, listResp[model.PolicyLog]{Items: items, Total: total})
}

func (h *Handler) computerTasks(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListTasks(r.Context(), store.TaskFilter{ComputerID: r.PathValue("id"), Limit: qInt(r, "limit", 50), Offset: qInt(r, "offset", 0)})
	if err != nil {
		h.fail(w, err, "tasks")
		return
	}
	redactTasksFor(r, items)
	writeJSON(w, 200, listResp[model.AgentTask]{Items: items, Total: total})
}

// sendCommand issues an MDM management command.
func (h *Handler) sendCommand(w http.ResponseWriter, r *http.Request) {
	c, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	var req struct {
		Type            string `json:"type"`
		PIN             string `json:"pin"`
		Message         string `json:"message"`
		Phone           string `json:"phone"`
		ProfileID       string `json:"profile_id"`
		Identifier      string `json:"identifier"`
		ManifestURL     string `json:"manifest_url"`
		ProductKey      string `json:"product_key"`
		InstallAction   string `json:"install_action"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		Plist           string `json:"plist"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var cmd *mdm.Command
	kind := "management"
	switch req.Type {
	case "DeviceInformation":
		cmd, err = commands.DeviceInformation()
		kind = "inventory"
	case "SecurityInfo":
		cmd, err = commands.SecurityInfo()
		kind = "inventory"
	case "ProfileList":
		cmd, err = commands.ProfileList()
		kind = "inventory"
	case "InstalledApplicationList":
		cmd, err = commands.InstalledApplicationList()
		kind = "inventory"
	case "CertificateList":
		cmd, err = commands.CertificateList()
		kind = "inventory"
	case "DeviceLock":
		cmd, err = commands.DeviceLock(req.PIN, req.Message, req.Phone)
	case "EraseDevice":
		cmd, err = commands.EraseDevice(req.PIN, "")
	case "RestartDevice":
		cmd, err = commands.RestartDevice()
	case "ShutDownDevice":
		cmd, err = commands.ShutDownDevice()
	case "EnableRemoteDesktop":
		cmd, err = commands.EnableRemoteDesktop()
	case "DisableRemoteDesktop":
		cmd, err = commands.DisableRemoteDesktop()
	case "InstallEnterpriseApplication":
		cmd, err = commands.InstallEnterpriseApplication(req.ManifestURL)
	case "AvailableOSUpdates":
		cmd, err = commands.AvailableOSUpdates()
	case "ScheduleOSUpdate":
		cmd, err = commands.ScheduleOSUpdate(req.ProductKey, req.InstallAction)
	case "SetRecoveryLock":
		cmd, err = commands.SetRecoveryLock(req.CurrentPassword, req.NewPassword)
	case "DeviceConfigured":
		cmd, err = commands.DeviceConfigured()
	case "InstallProfile":
		prof, perr := h.st.GetProfile(r.Context(), req.ProfileID)
		if perr != nil {
			h.fail(w, perr, "profile")
			return
		}
		if err := h.cmdr.InstallProfile(r.Context(), c, prof, actor(r)); err != nil {
			h.fail(w, err, "install profile")
			return
		}
		h.st.Audit(r.Context(), actor(r), "mdm.command", "computer", c.ID, map[string]any{"type": "InstallProfile", "profile": prof.Name})
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	case "RemoveProfile":
		if req.ProfileID != "" {
			prof, perr := h.st.GetProfile(r.Context(), req.ProfileID)
			if perr != nil {
				h.fail(w, perr, "profile")
				return
			}
			if err := h.cmdr.RemoveProfile(r.Context(), c, prof, actor(r)); err != nil {
				h.fail(w, err, "remove profile")
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true})
			return
		}
		cmd, err = commands.RemoveProfile(req.Identifier)
	case "RemoveMDMProfile":
		// Unmanage: remove the enrollment profile itself.
		settings, _ := h.st.GetSettings(r.Context())
		cmd, err = commands.RemoveProfile(settings.EnrollmentProfileIdentifier)
	case "Raw":
		cmd, err = commands.Raw([]byte(req.Plist))
		kind = "raw"
	default:
		writeErr(w, 400, "unknown command type")
		return
	}
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	res, err := h.cmdr.Enqueue(r.Context(), c, cmd, kind, "", actor(r))
	if err != nil {
		h.fail(w, err, "enqueue command")
		return
	}
	h.st.Audit(r.Context(), actor(r), "mdm.command", "computer", c.ID, map[string]any{"type": req.Type, "command_uuid": cmd.CommandUUID})
	pushErr := ""
	if res != nil && res.PushError != nil {
		pushErr = res.PushError.Error()
	} else if res != nil {
		for _, s := range res.Status {
			if s.PushError != nil {
				pushErr = s.PushError.Error()
			}
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "command_uuid": cmd.CommandUUID, "request_type": cmd.Command.RequestType, "push_error": pushErr})
}

func (h *Handler) pushComputer(w http.ResponseWriter, r *http.Request) {
	c, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	res, err := h.cmdr.Push(r.Context(), c)
	if err != nil {
		h.fail(w, err, "push")
		return
	}
	writeJSON(w, 200, res)
}

// createTask queues an agent task.
func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	c, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	var req struct {
		Type       string   `json:"type"`
		ScriptID   string   `json:"script_id"`
		Parameters []string `json:"parameters"`
		Command    string   `json:"command"`
		PolicyID   string   `json:"policy_id"`
		Trigger    string   `json:"trigger"`
		Message    string   `json:"message"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	t, err := h.queueTask(r, c, req.Type, req.ScriptID, req.Parameters, req.Command, req.PolicyID, req.Trigger, req.Message)
	if err != nil {
		h.fail(w, err, "task")
		return
	}
	writeJSON(w, 200, t)
}

func (h *Handler) queueTask(r *http.Request, c *model.Computer, typ, scriptID string, params []string, command, policyID, trigger, message string) (*model.AgentTask, error) {
	if !c.AgentEnrolled {
		return nil, errInvalid("the VibeMDM agent is not installed on this computer")
	}
	payload := map[string]any{}
	switch typ {
	case "run_script":
		sc, err := h.st.GetScript(r.Context(), scriptID)
		if err != nil {
			return nil, err
		}
		p := make([]string, 8)
		copy(p, params)
		payload = map[string]any{"script_id": sc.ID, "name": sc.Name, "contents": sc.Contents, "parameters": p}
	case "run_command":
		if strings.TrimSpace(command) == "" {
			return nil, errInvalid("command required")
		}
		payload = map[string]any{"command": command}
	case "run_policy":
		p, err := h.st.GetPolicy(r.Context(), policyID)
		if err != nil {
			return nil, err
		}
		ap, err := h.engine.PrepareFor(r.Context(), p, c, "manual")
		if err != nil {
			return nil, err
		}
		logID, err := h.st.CreatePendingPolicyLog(r.Context(), p.ID, c.ID, "manual")
		if err != nil {
			return nil, err
		}
		ap.LogID = logID
		payload = map[string]any{"policy": ap, "policy_id": p.ID, "name": p.Name}
	case "inventory":
	case "custom_trigger":
		if trigger == "" {
			return nil, errInvalid("trigger required")
		}
		payload = map[string]any{"trigger": trigger}
	case "filevault_regenerate":
		payload = map[string]any{"reason": "no escrowed FileVault recovery key"}
	case "laps_rotate":
		payload = map[string]any{"reason": "requested by " + actor(r)}
	case "restart", "shutdown", "logout_user":
		payload = map[string]any{"message": message}
	case "notify":
		payload = map[string]any{"message": message}
	default:
		return nil, errInvalid("unknown task type")
	}
	t, err := h.st.CreateTask(r.Context(), c.ID, typ, payload, actor(r))
	if err != nil {
		return nil, err
	}
	h.notifier.Notify(c.ID)
	h.st.Audit(r.Context(), actor(r), "agent.task", "computer", c.ID, map[string]any{"type": typ, "task_id": t.ID})
	return t, nil
}

// updateInventory asks both MDM and the agent for fresh inventory.
func (h *Handler) updateInventory(w http.ResponseWriter, r *http.Request) {
	c, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	var mdmErr, agentErr string
	if c.MDMEnrolled {
		if err := h.cmdr.RequestInventory(r.Context(), c, actor(r)); err != nil {
			mdmErr = err.Error()
		}
	} else {
		mdmErr = "not MDM enrolled"
	}
	if c.AgentEnrolled {
		if _, err := h.queueTask(r, c, "inventory", "", nil, "", "", "", ""); err != nil {
			agentErr = err.Error()
		}
	} else {
		agentErr = "agent not installed"
	}
	writeJSON(w, 200, map[string]any{"ok": true, "mdm_error": mdmErr, "agent_error": agentErr})
}

func (h *Handler) setEAValue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Value string `json:"value"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := h.st.SetEAValue(r.Context(), r.PathValue("id"), r.PathValue("ea"), req.Value); err != nil {
		h.fail(w, err, "extension attribute")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// checkinNow asks the agent to perform a full check-in immediately.
func (h *Handler) checkinNow(w http.ResponseWriter, r *http.Request) {
	c, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	if !c.AgentEnrolled {
		writeErr(w, 409, "the VibeMDM agent is not installed on this computer")
		return
	}
	online := h.notifier.Waiting(c.ID)
	h.notifier.RequestCheckin(c.ID)
	writeJSON(w, 200, map[string]any{"ok": true, "agent_online": online})
}

// resetAgent revokes a computer's agent token so the agent can be enrolled again
// (e.g. after reinstalling macOS). The old agent stops working immediately.
func (h *Handler) resetAgent(w http.ResponseWriter, r *http.Request) {
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	if err := h.st.UnenrollAgent(r.Context(), comp.ID); err != nil {
		h.fail(w, err, "computer")
		return
	}
	h.st.Audit(r.Context(), actor(r), "agent.reset", "computer", comp.ID, map[string]any{"computer": comp.Name})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
