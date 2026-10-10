package store

import (
	"context"

	"github.com/vibemdm/vibemdm/internal/model"
)

// Count is a label/count pair.
type Count struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// DashboardStats summarizes the fleet.
type DashboardStats struct {
	Computers           int               `json:"computers"`
	MDMEnrolled         int               `json:"mdm_enrolled"`
	AgentEnrolled       int               `json:"agent_enrolled"`
	AgentsOnline        int               `json:"agents_online"`
	CheckedIn24h        int               `json:"checked_in_24h"`
	NotSeen30d          int               `json:"not_seen_30d"`
	FileVaultEnabled    int               `json:"filevault_enabled"`
	FileVaultDisabled   int               `json:"filevault_disabled"`
	Policies            int               `json:"policies"`
	PoliciesEnabled     int               `json:"policies_enabled"`
	Scripts             int               `json:"scripts"`
	Profiles            int               `json:"profiles"`
	Groups              int               `json:"groups"`
	PendingCommands     int               `json:"pending_commands"`
	PendingTasks        int               `json:"pending_tasks"`
	FailedPolicies7d    int               `json:"failed_policies_7d"`
	CompletedPolicies7d int               `json:"completed_policies_7d"`
	OSVersions          []Count           `json:"os_versions"`
	Models              []Count           `json:"models"`
	RecentCheckins      []model.Computer  `json:"recent_checkins"`
	RecentPolicyLogs    []model.PolicyLog `json:"recent_policy_logs"`
	RecentTasks         []model.AgentTask `json:"recent_tasks"`
}

func (s *Store) counts(ctx context.Context, q string) ([]Count, error) {
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Count{}
	for rows.Next() {
		var c Count
		if err := rows.Scan(&c.Label, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Dashboard computes dashboard statistics.
func (s *Store) Dashboard(ctx context.Context) (*DashboardStats, error) {
	d := &DashboardStats{}
	err := s.DB.QueryRowContext(ctx, `SELECT count(*), count(*) FILTER (WHERE mdm_enrolled), count(*) FILTER (WHERE agent_enrolled),
		count(*) FILTER (WHERE GREATEST(agent_last_checkin, mdm_last_checkin) > now() - interval '24 hours'),
		count(*) FILTER (WHERE COALESCE(GREATEST(agent_last_checkin, mdm_last_checkin), created_at) < now() - interval '30 days'),
		count(*) FILTER (WHERE filevault_enabled), count(*) FILTER (WHERE filevault_enabled = FALSE) FROM computers`).
		Scan(&d.Computers, &d.MDMEnrolled, &d.AgentEnrolled, &d.CheckedIn24h, &d.NotSeen30d, &d.FileVaultEnabled, &d.FileVaultDisabled)
	if err != nil {
		return nil, err
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM policies), (SELECT count(*) FROM policies WHERE enabled), (SELECT count(*) FROM scripts),
		(SELECT count(*) FROM config_profiles), (SELECT count(*) FROM computer_groups), (SELECT count(*) FROM agent_tasks WHERE status IN ('queued','sent')),
		(SELECT count(*) FROM policy_logs WHERE status='failed' AND created_at > now() - interval '7 days'),
		(SELECT count(*) FROM policy_logs WHERE status='completed' AND created_at > now() - interval '7 days')`).
		Scan(&d.Policies, &d.PoliciesEnabled, &d.Scripts, &d.Profiles, &d.Groups, &d.PendingTasks, &d.FailedPolicies7d, &d.CompletedPolicies7d); err != nil {
		return nil, err
	}
	d.PendingCommands, _ = s.PendingCommandCount(ctx)
	if d.OSVersions, err = s.counts(ctx, `SELECT COALESCE(NULLIF(os_version,''),'Unknown'), count(*) FROM computers GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 12`); err != nil {
		return nil, err
	}
	if d.Models, err = s.counts(ctx, `SELECT COALESCE(NULLIF(model_name, ''), NULLIF(model,''), 'Unknown'), count(*) FROM computers GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 12`); err != nil {
		return nil, err
	}
	if d.RecentCheckins, _, err = s.ListComputers(ctx, ComputerFilter{Sort: "last_checkin", Desc: true, Limit: 8}); err != nil {
		return nil, err
	}
	if d.RecentPolicyLogs, _, err = s.ListPolicyLogs(ctx, PolicyLogFilter{Limit: 8}); err != nil {
		return nil, err
	}
	if d.RecentTasks, _, err = s.ListTasks(ctx, TaskFilter{Limit: 8}); err != nil {
		return nil, err
	}
	return d, nil
}
