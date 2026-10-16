package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/compliance"
	"github.com/vibemdm/vibemdm/internal/health"
	"github.com/vibemdm/vibemdm/internal/mdmsvc"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/policy"
	"github.com/vibemdm/vibemdm/internal/store"
	"github.com/vibemdm/vibemdm/internal/superwf"
)

// Deps are the services tools use.
type Deps struct {
	St         *store.Store
	Cmdr       *mdmsvc.Commander
	Health     *health.Engine
	Compliance *compliance.Service
	Engine     *policy.Engine
	Notify     func(computerID string)
}

type tools struct{ d Deps }

// resolveComputer finds a computer by ID, serial number or exact name.
func (t *tools) resolveComputer(ctx context.Context, ref string) (*model.Computer, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("computer is required")
	}
	if _, err := uuid.Parse(ref); err == nil {
		if c, err := t.d.St.GetComputer(ctx, ref); err == nil {
			return c, nil
		}
	}
	comps, _, err := t.d.St.ListComputers(ctx, store.ComputerFilter{Query: ref, Limit: 25})
	if err != nil {
		return nil, err
	}
	var matches []model.Computer
	for _, c := range comps {
		if strings.EqualFold(c.SerialNumber, ref) || strings.EqualFold(c.Name, ref) || strings.EqualFold(c.UDID, ref) {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("no computer matches %q (use search_computers)", ref)
	}
	return nil, fmt.Errorf("%d computers match %q; use the computer ID", len(matches), ref)
}

func computerBrief(c model.Computer) map[string]any {
	last := c.AgentLastCheckin
	if c.MDMLastCheckin != nil && (last == nil || c.MDMLastCheckin.After(*last)) {
		last = c.MDMLastCheckin
	}
	return map[string]any{"id": c.ID, "name": c.Name, "serial_number": c.SerialNumber, "model": c.ModelName, "os_version": c.OSVersion,
		"last_user": c.LastUser, "last_checkin": last, "mdm_enrolled": c.MDMEnrolled, "agent_enrolled": c.AgentEnrolled}
}

// Tools returns the MCP tool catalog.
func Tools(d Deps) []*Tool {
	t := &tools{d: d}
	return []*Tool{
		{Name: "fleet_summary", Title: "Fleet summary",
			Description: "Counts of computers, enrollment, check-ins, pending work and open health issues: a quick picture of the fleet.",
			Input:       schema(map[string]any{}), Idempotent: true, Run: t.fleetSummary},
		{Name: "search_computers", Title: "Search computers",
			Description: "Find Macs by name, serial number, user, model, asset tag, hostname or IP address.",
			Input:       schema(map[string]any{"query": str("Text to search for; empty lists computers"), "limit": num("Maximum results (default 25)", 1, 100)}),
			Idempotent:  true, Run: t.searchComputers},
		{Name: "get_computer", Title: "Computer details",
			Description: "Inventory, security state, groups, open health issues, compliance scores and super/update status for one Mac.",
			Input:       schema(map[string]any{"computer": str("Computer ID, serial number or exact name")}, "computer"), Idempotent: true, Run: t.getComputer},
		{Name: "list_health_issues", Title: "Health issues",
			Description: "Open misconfigurations and fleet problems detected by VibeMDM Health, grouped by check, with explanations and available fixes.",
			Input: schema(map[string]any{"severity": enum("Only this severity", "critical", "warning", "info"),
				"computer": str("Only issues for this computer (ID, serial or name)"), "include_acknowledged": boolean("Include acknowledged and snoozed issues")}),
			Idempotent: true, Run: t.listHealth},
		{Name: "list_policies", Title: "Policies",
			Description: "Policies with triggers, frequency, scope summary and recent run outcomes.",
			Input:       schema(map[string]any{}), Idempotent: true, Run: t.listPolicies},
		{Name: "get_policy_logs", Title: "Policy run logs",
			Description: "Recent policy runs, optionally filtered by policy, computer or status; output is truncated.",
			Input: schema(map[string]any{"policy": str("Policy ID or exact name"), "computer": str("Computer ID, serial or name"),
				"status": enum("Run status", "completed", "failed", "pending", "running"), "limit": num("Maximum results (default 20)", 1, 100)}),
			Idempotent: true, Run: t.policyLogs},
		{Name: "list_profiles", Title: "Configuration profiles",
			Description: "Configuration profiles with their identifiers and deployment status counts (payloads are not included).",
			Input:       schema(map[string]any{}), Idempotent: true, Run: t.listProfiles},
		{Name: "compliance_overview", Title: "Compliance",
			Description: "mSCP compliance baselines with fleet scores; with a baseline, also the most-failing rules and least-compliant Macs.",
			Input:       schema(map[string]any{"baseline": str("Baseline ID or exact name")}), Idempotent: true, Run: t.compliance},
		{Name: "software_update_status", Title: "Software updates",
			Description: "Declarative software update plans and super (S.U.P.E.R.M.A.N.) deployment status.",
			Input:       schema(map[string]any{}), Idempotent: true, Run: t.updates},
		{Name: "search_logs", Title: "Search logs",
			Description: "Search server and agent logs.",
			Input: schema(map[string]any{"query": str("Full-text search"), "source": enum("Log source", "server", "agent"),
				"min_level": enum("Minimum level", "DEBUG", "INFO", "WARN", "ERROR"), "computer": str("Agent logs for this computer"),
				"limit": num("Maximum entries (default 50)", 1, 200)}),
			Idempotent: true, Run: t.searchLogs},
		{Name: "recent_mdm_commands", Title: "Recent MDM commands",
			Description: "Recent MDM commands for a computer with their status (payloads are not included).",
			Input: schema(map[string]any{"computer": str("Computer ID, serial or name"), "limit": num("Maximum results (default 25)", 1, 100)},
				"computer"), Idempotent: true, Run: t.commands},
		{Name: "recent_audit_events", Title: "Audit log",
			Description: "Recent administrative actions recorded in the audit log.",
			Input:       schema(map[string]any{"limit": num("Maximum entries (default 50)", 1, 200)}), Idempotent: true, AdminOnly: true, Run: t.audit},

		// ---- write tools (admin token + "allow write tools") ----
		{Name: "send_push", Title: "Wake a Mac (APNs push)", Write: true, Idempotent: true,
			Description: "Send a blank APNs push so the Mac checks in with MDM and runs any queued commands.",
			Input:       schema(map[string]any{"computer": str("Computer ID, serial or name")}, "computer"), Run: t.push},
		{Name: "update_inventory", Title: "Update inventory", Write: true, Idempotent: true,
			Description: "Ask a Mac to send fresh inventory (through the agent when installed, otherwise MDM).",
			Input:       schema(map[string]any{"computer": str("Computer ID, serial or name")}, "computer"), Run: t.inventory},
		{Name: "run_policy", Title: "Run a policy on a Mac", Write: true,
			Description: "Run an existing policy on one Mac now, through the agent. Only policies an admin created can be run.",
			Input: schema(map[string]any{"policy": str("Policy ID or exact name"), "computer": str("Computer ID, serial or name")},
				"policy", "computer"), Run: t.runPolicy},
		{Name: "acknowledge_health_issue", Title: "Acknowledge or snooze a health issue", Write: true, Idempotent: true,
			Description: "Acknowledge a health finding, or snooze it for a number of hours.",
			Input: schema(map[string]any{"finding_id": str("Finding ID from list_health_issues"), "snooze_hours": num("Snooze instead of acknowledge", 1, 2160)},
				"finding_id"), Run: t.ackHealth},
		{Name: "remediate_health_issue", Title: "Apply a health fix", Write: true,
			Description: "Run one of the server-side remediations offered for a health finding (runnable remediations only).",
			Input: schema(map[string]any{"finding_id": str("Finding ID"), "remediation_id": str("Remediation ID from the finding")},
				"finding_id", "remediation_id"), Run: t.remediate},
		{Name: "start_compliance_scan", Title: "Start a compliance scan", Write: true,
			Description: "Scan one Mac (or the baseline's whole scope) against a compliance baseline. Remediation is never run from MCP.",
			Input: schema(map[string]any{"baseline": str("Baseline ID or exact name"), "computer": str("Computer ID, serial or name; omit for the whole scope")},
				"baseline"), Run: t.scan},
	}
}

func (t *tools) fleetSummary(ctx context.Context, _ *auth.Principal, _ map[string]any) (any, error) {
	d, err := t.d.St.Dashboard(ctx)
	if err != nil {
		return nil, err
	}
	fs, _ := t.d.St.ListHealthFindings(ctx, store.HealthFilter{States: []string{"open"}})
	counts := map[string]int{"critical": 0, "warning": 0, "info": 0}
	for _, f := range fs {
		counts[f.Severity]++
	}
	return map[string]any{"fleet": d, "open_health_issues": counts}, nil
}

func (t *tools) searchComputers(ctx context.Context, _ *auth.Principal, args map[string]any) (any, error) {
	q, _ := argString(args, "query", false)
	comps, total, err := t.d.St.ListComputers(ctx, store.ComputerFilter{Query: q, Limit: argInt(args, "limit", 25, 1, 100)})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(comps))
	for _, c := range comps {
		out = append(out, computerBrief(c))
	}
	return map[string]any{"total": total, "computers": out}, nil
}

func (t *tools) getComputer(ctx context.Context, _ *auth.Principal, args map[string]any) (any, error) {
	ref, err := argString(args, "computer", true)
	if err != nil {
		return nil, err
	}
	c, err := t.resolveComputer(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := computerBrief(*c)
	out["hardware"] = map[string]any{"model_identifier": c.ModelIdentifier, "architecture": c.Architecture, "memory_mb": c.MemoryMB,
		"disk_free_mb": c.DiskFreeMB, "disk_total_mb": c.DiskTotalMB}
	out["security"] = map[string]any{"filevault": c.FileVaultEnabled, "sip": c.SIPEnabled, "gatekeeper": c.GatekeeperEnabled,
		"firewall": c.FirewallEnabled, "supervised": c.IsSupervised}
	out["assignment"] = map[string]any{"asset_tag": c.AssetTag, "building": c.Building, "department": c.Department, "assigned_user": c.AssignedUser}
	if groups, err := t.d.St.ComputerGroups(ctx, c.ID); err == nil {
		names := []string{}
		for _, g := range groups {
			names = append(names, g.Name)
		}
		out["groups"] = names
	}
	if fs, err := t.d.St.ListHealthFindings(ctx, store.HealthFilter{ObjectType: "computer", ObjectID: c.ID}); err == nil {
		issues := []map[string]any{}
		for _, f := range fs {
			issues = append(issues, map[string]any{"id": f.ID, "check": f.CheckID, "severity": f.Severity, "summary": f.Summary, "state": f.State})
		}
		out["health_issues"] = issues
	}
	if bs, err := t.d.St.ListComplianceBaselines(ctx); err == nil {
		comp := []map[string]any{}
		for _, b := range bs {
			sums, _ := t.d.St.ComplianceComputerSummaries(ctx, b.ID)
			for _, s := range sums {
				if s.ComputerID == c.ID {
					comp = append(comp, map[string]any{"baseline": b.Name, "score": s.Score, "failed": s.Failed, "passed": s.Passed, "last_scan_at": s.LastScanAt})
				}
			}
		}
		out["compliance"] = comp
	}
	if rows, err := t.d.St.SuperStatus(ctx, []string{c.ID}); err == nil && len(rows) == 1 && rows[0].Super != nil {
		out["super"] = rows[0].Super
	}
	return out, nil
}

func (t *tools) listHealth(ctx context.Context, _ *auth.Principal, args map[string]any) (any, error) {
	f := store.HealthFilter{States: []string{"open"}, Limit: 500}
	if argBool(args, "include_acknowledged") {
		f.States = []string{"open", "acknowledged", "snoozed"}
	}
	f.Severity, _ = argString(args, "severity", false)
	if ref, _ := argString(args, "computer", false); ref != "" {
		c, err := t.resolveComputer(ctx, ref)
		if err != nil {
			return nil, err
		}
		f.ObjectType, f.ObjectID = "computer", c.ID
	}
	fs, err := t.d.St.ListHealthFindings(ctx, f)
	if err != nil {
		return nil, err
	}
	type group struct {
		Check       string           `json:"check"`
		Title       string           `json:"title"`
		Description string           `json:"description"`
		Findings    []map[string]any `json:"findings"`
	}
	groups := map[string]*group{}
	order := []string{}
	for _, fd := range fs {
		g := groups[fd.CheckID]
		if g == nil {
			g = &group{Check: fd.CheckID}
			if c, ok := t.d.Health.Registry().Get(fd.CheckID); ok {
				g.Title, g.Description = c.Title, c.Description
			}
			groups[fd.CheckID] = g
			order = append(order, fd.CheckID)
		}
		if len(g.Findings) >= 20 {
			continue
		}
		rems := []map[string]any{}
		for _, r := range fd.Remediations {
			rems = append(rems, map[string]any{"id": r.ID, "label": r.Label, "runnable": r.Runnable, "safe": r.Safe})
		}
		g.Findings = append(g.Findings, map[string]any{"id": fd.ID, "severity": fd.Severity, "object": fd.ObjectName, "object_type": fd.ObjectType,
			"summary": fd.Summary, "detail": fd.Detail, "state": fd.State, "first_seen": fd.FirstSeen, "remediations": rems})
	}
	out := []*group{}
	for _, k := range order {
		out = append(out, groups[k])
	}
	return map[string]any{"total": len(fs), "groups": out}, nil
}

func (t *tools) listPolicies(ctx context.Context, _ *auth.Principal, _ map[string]any) (any, error) {
	pols, err := t.d.St.ListPolicies(ctx)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, p := range pols {
		st, _ := t.d.St.PolicyStatusSummary(ctx, p.ID)
		out = append(out, map[string]any{"id": p.ID, "name": p.Name, "enabled": p.Enabled, "category": p.Category, "triggers": p.Triggers,
			"frequency": p.Frequency, "all_computers": p.Scope.AllComputers, "self_service": p.SelfService.Enabled, "status": st})
	}
	return map[string]any{"policies": out}, nil
}

func (t *tools) resolvePolicy(ctx context.Context, ref string) (*model.Policy, error) {
	if _, err := uuid.Parse(ref); err == nil {
		if p, err := t.d.St.GetPolicy(ctx, ref); err == nil {
			return p, nil
		}
	}
	pols, err := t.d.St.ListPolicies(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range pols {
		if strings.EqualFold(p.Name, ref) {
			return t.d.St.GetPolicy(ctx, p.ID)
		}
	}
	return nil, fmt.Errorf("no policy named %q (use list_policies)", ref)
}

func (t *tools) policyLogs(ctx context.Context, _ *auth.Principal, args map[string]any) (any, error) {
	f := store.PolicyLogFilter{Limit: argInt(args, "limit", 20, 1, 100)}
	f.Status, _ = argString(args, "status", false)
	if ref, _ := argString(args, "policy", false); ref != "" {
		p, err := t.resolvePolicy(ctx, ref)
		if err != nil {
			return nil, err
		}
		f.PolicyID = p.ID
	}
	if ref, _ := argString(args, "computer", false); ref != "" {
		c, err := t.resolveComputer(ctx, ref)
		if err != nil {
			return nil, err
		}
		f.ComputerID = c.ID
	}
	logs, total, err := t.d.St.ListPolicyLogs(ctx, f)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, l := range logs {
		text := l.Log
		if len(text) > 800 {
			text = "…" + text[len(text)-800:]
		}
		out = append(out, map[string]any{"policy": l.PolicyName, "computer": l.ComputerName, "status": l.Status, "trigger": l.Trigger,
			"started_at": l.StartedAt, "completed_at": l.CompletedAt, "log_tail": text})
	}
	return map[string]any{"total": total, "runs": out}, nil
}

func (t *tools) listProfiles(ctx context.Context, _ *auth.Principal, _ map[string]any) (any, error) {
	profs, err := t.d.St.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, p := range profs {
		st, _ := t.d.St.DeploymentStatusCounts(ctx, p.ID)
		out = append(out, map[string]any{"id": p.ID, "name": p.Name, "identifier": p.Identifier, "category": p.Category,
			"all_computers": p.Scope.AllComputers, "deployments": st, "built_from_schema": p.SchemaSource})
	}
	return map[string]any{"profiles": out}, nil
}

func (t *tools) resolveBaseline(ctx context.Context, ref string) (*model.ComplianceBaseline, error) {
	bs, err := t.d.St.ListComplianceBaselines(ctx)
	if err != nil {
		return nil, err
	}
	for i := range bs {
		if bs[i].ID == ref || strings.EqualFold(bs[i].Name, ref) {
			return &bs[i], nil
		}
	}
	return nil, fmt.Errorf("no compliance baseline %q (use compliance_overview)", ref)
}

func (t *tools) compliance(ctx context.Context, _ *auth.Principal, args map[string]any) (any, error) {
	ref, _ := argString(args, "baseline", false)
	if ref == "" {
		bs, err := t.d.St.ListComplianceBaselines(ctx)
		if err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, b := range bs {
			sums, _ := t.d.St.ComplianceComputerSummaries(ctx, b.ID)
			total, n, compliant := 0.0, 0, 0
			for _, s := range sums {
				if s.Total > 0 {
					n++
					total += s.Score
					if s.Failed == 0 && s.Errors == 0 {
						compliant++
					}
				}
			}
			avg := 0.0
			if n > 0 {
				avg = total / float64(n)
			}
			out = append(out, map[string]any{"id": b.ID, "name": b.Name, "benchmark": b.Benchmark, "enabled": b.Enabled,
				"scanned_macs": n, "compliant_macs": compliant, "average_score": avg})
		}
		return map[string]any{"baselines": out}, nil
	}
	b, err := t.resolveBaseline(ctx, ref)
	if err != nil {
		return nil, err
	}
	cat, err := compliance.Load()
	if err != nil {
		return nil, err
	}
	counts, _ := t.d.St.ComplianceRuleCounts(ctx, b.ID)
	type rule struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Failed  int    `json:"failed"`
		Drifted int    `json:"drifted"`
	}
	failing := []rule{}
	for id, c := range counts {
		if c.Failed == 0 {
			continue
		}
		title := id
		if r, ok := cat.Rules[id]; ok {
			title = r.Title
		}
		failing = append(failing, rule{id, title, c.Failed, c.Drifted})
	}
	sortBy(failing, func(a, b rule) bool { return a.Failed > b.Failed })
	if len(failing) > 15 {
		failing = failing[:15]
	}
	sums, _ := t.d.St.ComplianceComputerSummaries(ctx, b.ID)
	sortBy(sums, func(a, b store.ComplianceComputerSummary) bool { return a.Score < b.Score })
	worst := []map[string]any{}
	for i, s := range sums {
		if i >= 10 {
			break
		}
		worst = append(worst, map[string]any{"computer": s.ComputerName, "score": s.Score, "failed": s.Failed, "last_scan_at": s.LastScanAt})
	}
	return map[string]any{"baseline": map[string]any{"id": b.ID, "name": b.Name, "benchmark": b.Benchmark, "auto_remediate": b.AutoRemediate},
		"most_failing_rules": failing, "least_compliant_macs": worst}, nil
}

func (t *tools) updates(ctx context.Context, _ *auth.Principal, _ map[string]any) (any, error) {
	plans, err := t.d.St.ListUpdatePlans(ctx)
	if err != nil {
		return nil, err
	}
	ps := []map[string]any{}
	for _, p := range plans {
		ps = append(ps, map[string]any{"name": p.Name, "kind": p.Kind, "enabled": p.Enabled, "target_os_version": p.TargetOSVersion,
			"deadline": p.TargetLocalDateTime, "all_computers": p.Scope.AllComputers})
	}
	out := map[string]any{"ddm_plans": ps}
	if cfg, _, err := t.d.St.GetSuperConfig(ctx); err == nil {
		sup := map[string]any{"enabled": cfg.Enabled, "pinned_version": superwf.Version}
		if ids, err := t.d.St.ResolveScope(ctx, cfg.Scope); err == nil && cfg.Enabled {
			rows, _ := t.d.St.SuperStatus(ctx, ids)
			counts := map[string]int{"current": 0, "outdated": 0, "missing": 0}
			for _, r := range rows {
				switch v := superwf.InstalledVersion(r.Super); {
				case v == superwf.Version:
					counts["current"]++
				case v != "":
					counts["outdated"]++
				default:
					counts["missing"]++
				}
			}
			sup["macs"] = counts
		}
		out["super"] = sup
	}
	return out, nil
}

func (t *tools) searchLogs(ctx context.Context, _ *auth.Principal, args map[string]any) (any, error) {
	f := store.LogFilter{Limit: argInt(args, "limit", 50, 1, 200)}
	f.Q, _ = argString(args, "query", false)
	f.Source, _ = argString(args, "source", false)
	f.MinLevel, _ = argString(args, "min_level", false)
	if f.Source == "" {
		f.Source = "server"
	}
	if ref, _ := argString(args, "computer", false); ref != "" {
		c, err := t.resolveComputer(ctx, ref)
		if err != nil {
			return nil, err
		}
		f.ComputerID, f.Source = c.ID, "agent"
	}
	entries, err := t.d.St.QueryLogs(ctx, f)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, e := range entries {
		out = append(out, map[string]any{"at": e.At, "level": e.Level, "component": e.Component, "computer": e.ComputerName, "msg": e.Msg})
	}
	return map[string]any{"entries": out}, nil
}

func (t *tools) commands(ctx context.Context, _ *auth.Principal, args map[string]any) (any, error) {
	ref, err := argString(args, "computer", true)
	if err != nil {
		return nil, err
	}
	c, err := t.resolveComputer(ctx, ref)
	if err != nil {
		return nil, err
	}
	cmds, _, err := t.d.St.ListCommands(ctx, store.CommandFilter{ComputerID: c.ID, Limit: argInt(args, "limit", 25, 1, 100)})
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, m := range cmds {
		out = append(out, map[string]any{"command_uuid": m.CommandUUID, "request_type": m.RequestType, "status": m.Status,
			"created_at": m.CreatedAt, "updated_at": m.ResultUpdatedAt, "created_by": m.CreatedBy})
	}
	return map[string]any{"computer": c.Name, "commands": out}, nil
}

func (t *tools) audit(ctx context.Context, _ *auth.Principal, args map[string]any) (any, error) {
	items, _, err := t.d.St.ListAudit(ctx, argInt(args, "limit", 50, 1, 200), 0, "")
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, a := range items {
		out = append(out, map[string]any{"at": a.At, "actor": a.Actor, "action": a.Action, "target_type": a.TargetType, "target_id": a.TargetID})
	}
	return map[string]any{"events": out}, nil
}

// ---- write tools ----

func (t *tools) push(ctx context.Context, _ *auth.Principal, args map[string]any) (any, error) {
	ref, err := argString(args, "computer", true)
	if err != nil {
		return nil, err
	}
	c, err := t.resolveComputer(ctx, ref)
	if err != nil {
		return nil, err
	}
	if err := t.d.Cmdr.PushComputer(ctx, c); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "message": "Push sent to " + c.Name}, nil
}

func (t *tools) inventory(ctx context.Context, p *auth.Principal, args map[string]any) (any, error) {
	ref, err := argString(args, "computer", true)
	if err != nil {
		return nil, err
	}
	c, err := t.resolveComputer(ctx, ref)
	if err != nil {
		return nil, err
	}
	if c.AgentEnrolled {
		if _, err := t.d.St.CreateTask(ctx, c.ID, "inventory", map[string]any{}, p.Username); err != nil {
			return nil, err
		}
		if t.d.Notify != nil {
			t.d.Notify(c.ID)
		}
		return map[string]any{"ok": true, "message": "Inventory requested from the agent on " + c.Name}, nil
	}
	if err := t.d.Cmdr.RequestInventory(ctx, c, p.Username); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "message": "Inventory requested over MDM from " + c.Name}, nil
}

func (t *tools) runPolicy(ctx context.Context, p *auth.Principal, args map[string]any) (any, error) {
	pref, err := argString(args, "policy", true)
	if err != nil {
		return nil, err
	}
	cref, err := argString(args, "computer", true)
	if err != nil {
		return nil, err
	}
	pol, err := t.resolvePolicy(ctx, pref)
	if err != nil {
		return nil, err
	}
	c, err := t.resolveComputer(ctx, cref)
	if err != nil {
		return nil, err
	}
	if !c.AgentEnrolled {
		return nil, fmt.Errorf("%s does not have the VibeMDM agent", c.Name)
	}
	// An AI client acts on text it reads from Macs, so it may only run policies an admin has
	// enabled; the console's manual "Run policy" can still run disabled ones.
	if !pol.Enabled {
		return nil, fmt.Errorf("policy %q is disabled; enable it in the console before running it over MCP", pol.Name)
	}
	ap, err := t.d.Engine.PrepareFor(ctx, pol, c, "manual")
	if err != nil {
		return nil, err
	}
	logID, err := t.d.St.CreatePendingPolicyLog(ctx, pol.ID, c.ID, "manual")
	if err != nil {
		return nil, err
	}
	ap.LogID = logID
	if _, err := t.d.St.CreateTask(ctx, c.ID, "run_policy", map[string]any{"policy": ap, "policy_id": pol.ID, "name": pol.Name}, p.Username); err != nil {
		return nil, err
	}
	if t.d.Notify != nil {
		t.d.Notify(c.ID)
	}
	return map[string]any{"ok": true, "message": fmt.Sprintf("Policy %q queued on %s; check get_policy_logs for the result.", pol.Name, c.Name)}, nil
}

func (t *tools) ackHealth(ctx context.Context, p *auth.Principal, args map[string]any) (any, error) {
	id, err := argString(args, "finding_id", true)
	if err != nil {
		return nil, err
	}
	state, dur := "acknowledged", time.Duration(0)
	if h := argInt(args, "snooze_hours", 0, 0, 2160); h > 0 {
		state, dur = "snoozed", time.Duration(h)*time.Hour
	}
	f, err := t.d.Health.SetState(ctx, id, state, p.Username, dur)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "state": f.State, "summary": f.Summary}, nil
}

func (t *tools) remediate(ctx context.Context, p *auth.Principal, args map[string]any) (any, error) {
	id, err := argString(args, "finding_id", true)
	if err != nil {
		return nil, err
	}
	rid, err := argString(args, "remediation_id", true)
	if err != nil {
		return nil, err
	}
	label, err := t.d.Health.Remediate(ctx, id, rid, p.Username)
	if errors.Is(err, health.ErrAlreadyResolved) {
		return map[string]any{"ok": true, "message": "The condition already cleared; the finding was resolved."}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "message": label + " — done."}, nil
}

func (t *tools) scan(ctx context.Context, p *auth.Principal, args map[string]any) (any, error) {
	bref, err := argString(args, "baseline", true)
	if err != nil {
		return nil, err
	}
	b, err := t.resolveBaseline(ctx, bref)
	if err != nil {
		return nil, err
	}
	var ids []string
	if cref, _ := argString(args, "computer", false); cref != "" {
		c, err := t.resolveComputer(ctx, cref)
		if err != nil {
			return nil, err
		}
		ids = []string{c.ID}
	} else if ids, err = t.d.St.ResolveScope(ctx, b.Scope); err != nil {
		return nil, err
	}
	queued, skipped := 0, map[string]string{}
	for _, id := range ids {
		c, err := t.d.St.GetComputer(ctx, id)
		if err != nil {
			continue
		}
		if _, err := t.d.Compliance.RequestScan(ctx, b, c, "mcp", false, p.Username); err != nil {
			skipped[c.Name] = err.Error()
			continue
		}
		queued++
	}
	return map[string]any{"ok": true, "queued": queued, "skipped": skipped}, nil
}

func sortBy[T any](s []T, less func(a, b T) bool) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && less(s[j], s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
