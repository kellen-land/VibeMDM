package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vibemdm/vibemdm/internal/ddm"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

func validatePlan(p *model.UpdatePlan) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return errors.New("name required")
	}
	switch p.Kind {
	case "enforcement":
		p.Settings = map[string]any{}
		return ddm.Enforcement{TargetOSVersion: p.TargetOSVersion, TargetBuildVersion: p.TargetBuildVersion,
			TargetLocalDateTime: p.TargetLocalDateTime, DetailsURL: p.DetailsURL}.Validate()
	case "settings":
		p.TargetOSVersion, p.TargetBuildVersion, p.TargetLocalDateTime, p.DetailsURL = "", "", "", ""
		return store.SettingsFromMap(p.Settings).Validate()
	}
	return errors.New(`kind must be "enforcement" or "settings"`)
}

func (h *Handler) reconcileDDMAsync() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, err := h.cmdr.ReconcileDDM(ctx); err != nil {
			h.log.Warn("DDM reconcile", "err", err)
		}
	}()
}

func (h *Handler) listPlans(w http.ResponseWriter, r *http.Request) {
	items, err := h.st.ListUpdatePlans(r.Context())
	if err != nil {
		h.fail(w, err, "update plans")
		return
	}
	writeJSON(w, 200, listResp[model.UpdatePlan]{Items: items, Total: len(items)})
}

func (h *Handler) getPlan(w http.ResponseWriter, r *http.Request) {
	p, err := h.st.GetUpdatePlan(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "update plan")
		return
	}
	writeJSON(w, 200, p)
}

func (h *Handler) savePlan(w http.ResponseWriter, r *http.Request) {
	var p model.UpdatePlan
	if err := decode(r, &p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p.ID = r.PathValue("id")
	if err := validatePlan(&p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	out, err := h.st.SaveUpdatePlan(r.Context(), p)
	if err != nil {
		h.fail(w, err, "update plan")
		return
	}
	h.st.Audit(r.Context(), actor(r), "update_plan.save", "update_plan", out.ID, map[string]any{"name": out.Name, "kind": out.Kind, "target": out.TargetOSVersion, "deadline": out.TargetLocalDateTime})
	h.reconcileDDMAsync()
	code := 200
	if p.ID == "" {
		code = 201
	}
	writeJSON(w, code, out)
}

func (h *Handler) deletePlan(w http.ResponseWriter, r *http.Request) {
	if err := h.st.DeleteUpdatePlan(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, err, "update plan")
		return
	}
	h.st.Audit(r.Context(), actor(r), "update_plan.delete", "update_plan", r.PathValue("id"), nil)
	h.reconcileDDMAsync()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type updateRow struct {
	ComputerID         string         `json:"computer_id"`
	ComputerName       string         `json:"computer_name"`
	OSVersion          string         `json:"os_version"`
	OSBuild            string         `json:"os_build"`
	MDMEnrolled        bool           `json:"mdm_enrolled"`
	EnforcementPlan    string         `json:"enforcement_plan"`
	TargetOSVersion    string         `json:"target_os_version"`
	Deadline           string         `json:"deadline"`
	SettingsPlan       string         `json:"settings_plan"`
	State              string         `json:"state"` // up_to_date | pending | failed | not_targeted | no_ddm | unsupported
	InstallState       string         `json:"install_state"`
	PendingVersion     map[string]any `json:"pending_version,omitempty"`
	FailureReason      map[string]any `json:"failure_reason,omitempty"`
	DeclarationsInSync bool           `json:"declarations_in_sync"`
	LastStatusAt       *time.Time     `json:"last_status_at"`
}

func getMap(m map[string]any, path ...string) map[string]any {
	cur := m
	for _, p := range path {
		next, ok := cur[p].(map[string]any)
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func (h *Handler) updateRowFor(ctx context.Context, c *model.Computer) updateRow {
	row := updateRow{ComputerID: c.ID, ComputerName: c.Name, OSVersion: c.OSVersion, OSBuild: c.OSBuild, MDMEnrolled: c.MDMEnrolled}
	enf, set, _ := h.st.EffectivePlans(ctx, c)
	if enf != nil {
		row.EnforcementPlan, row.TargetOSVersion, row.Deadline = enf.Name, enf.TargetOSVersion, enf.TargetLocalDateTime
	}
	if set != nil {
		row.SettingsPlan = set.Name
	}
	st, _ := h.st.GetDDMState(ctx, c.ID)
	if st != nil {
		row.LastStatusAt = st.LastStatusAt
		if su := getMap(st.Status, "softwareupdate"); su != nil {
			row.InstallState, _ = su["install-state"].(string)
			row.PendingVersion, _ = su["pending-version"].(map[string]any)
			row.FailureReason, _ = su["failure-reason"].(map[string]any)
		}
		if decls, err := h.st.DeclarationsFor(ctx, c); err == nil {
			row.DeclarationsInSync = st.ServedToken == ddm.DeclarationsToken(decls)
		}
	}
	switch {
	case !c.MDMEnrolled:
		row.State = "no_ddm"
	case c.OSVersion != "" && ddm.CompareVersions(c.OSVersion, "14.0") < 0:
		row.State = "unsupported" // enforcement needs macOS 14+
	case st == nil || st.LastStatusAt == nil:
		row.State = "no_ddm"
	case len(row.FailureReason) > 0:
		row.State = "failed"
	case enf == nil:
		row.State = "not_targeted"
	case ddm.CompareVersions(c.OSVersion, enf.TargetOSVersion) >= 0:
		row.State = "up_to_date"
	default:
		row.State = "pending"
	}
	return row
}

// updatesOverview reports update status for every Mac.
func (h *Handler) updatesOverview(w http.ResponseWriter, r *http.Request) {
	comps, _, err := h.st.ListComputers(r.Context(), store.ComputerFilter{Limit: 500})
	if err != nil {
		h.fail(w, err, "computers")
		return
	}
	rows := make([]updateRow, 0, len(comps))
	counts := map[string]int{}
	for i := range comps {
		row := h.updateRowFor(r.Context(), &comps[i])
		counts[row.State]++
		rows = append(rows, row)
	}
	writeJSON(w, 200, map[string]any{"computers": rows, "counts": counts})
}

// computerDDM shows a Mac's declarations exactly as served, plus its reported state.
func (h *Handler) computerDDM(w http.ResponseWriter, r *http.Request) {
	c, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	decls, err := h.st.DeclarationsFor(r.Context(), c)
	if err != nil {
		h.fail(w, err, "declarations")
		return
	}
	st, _ := h.st.GetDDMState(r.Context(), c.ID)
	reports, _ := h.st.RecentDDMReports(r.Context(), c.ID, 10)
	writeJSON(w, 200, map[string]any{
		"summary": h.updateRowFor(r.Context(), c), "declarations": decls, "declarations_token": ddm.DeclarationsToken(decls),
		"state": st, "recent_reports": reports,
	})
}

func (h *Handler) syncComputerDDM(w http.ResponseWriter, r *http.Request) {
	c, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	if err := h.cmdr.SyncDDM(r.Context(), c, actor(r)); err != nil {
		h.fail(w, err, "sync")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- macOS version catalog (SOFA feed by Mac Admins Open Source) ----

const sofaURL = "https://sofafeed.macadmins.io/v1/macos_data_feed.json"

var sofaCache struct {
	sync.Mutex
	at       time.Time
	body     []map[string]any
	err      error
	failedAt time.Time // back off after a failed fetch
}

func (h *Handler) updatesCatalog(w http.ResponseWriter, r *http.Request) {
	sofaCache.Lock()
	defer sofaCache.Unlock()
	if (sofaCache.body == nil || time.Since(sofaCache.at) > 6*time.Hour) && time.Since(sofaCache.failedAt) > 5*time.Minute {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, sofaURL, nil)
		req.Header.Set("User-Agent", "vibemdm")
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == 200 {
			var feed struct {
				OSVersions []struct {
					OSVersion        string
					Latest           map[string]any
					SecurityReleases []map[string]any
				}
			}
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
			resp.Body.Close()
			if err = json.Unmarshal(b, &feed); err == nil {
				out := []map[string]any{}
				for _, o := range feed.OSVersions {
					rel := []map[string]any{}
					for i, s := range o.SecurityReleases {
						if i >= 8 {
							break
						}
						rel = append(rel, map[string]any{"version": s["ProductVersion"], "release_date": s["ReleaseDate"], "update_name": s["UpdateName"]})
					}
					out = append(out, map[string]any{
						"name": o.OSVersion, "latest_version": o.Latest["ProductVersion"], "latest_build": o.Latest["Build"],
						"release_date": o.Latest["ReleaseDate"], "actively_exploited_cves": o.Latest["ActivelyExploitedCVEs"], "recent_releases": rel,
					})
				}
				sofaCache.body, sofaCache.at, sofaCache.err = out, time.Now(), nil
			}
		} else if err == nil {
			resp.Body.Close()
			err = errors.New(resp.Status)
		}
		if err != nil {
			sofaCache.err, sofaCache.failedAt = err, time.Now()
			h.log.Warn("fetching the SOFA macOS feed failed", "err", err)
		}
	}
	if sofaCache.body == nil {
		if sofaCache.err == nil {
			sofaCache.err = errors.New("not fetched yet")
		}
		writeErr(w, 502, "macOS version catalog unavailable: "+sofaCache.err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"versions": sofaCache.body, "source": "SOFA feed by Mac Admins Open Source (sofa.macadmins.io)", "fetched_at": sofaCache.at})
}
