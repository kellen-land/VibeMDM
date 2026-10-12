// Package policy evaluates which policies should run on a computer for a trigger
// and prepares them for the agent.
package policy

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sort"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Engine evaluates policies.
type Engine struct {
	st  *store.Store
	log *slog.Logger
}

// New creates an Engine.
func New(st *store.Store, logger *slog.Logger) *Engine { return &Engine{st: st, log: logger} }

// ForTrigger returns policies that should run now for a computer and trigger,
// creating pending log rows for each.
func (e *Engine) ForTrigger(ctx context.Context, comp *model.Computer, trigger string) ([]model.AgentPolicy, error) {
	policies, err := e.st.ListPolicies(ctx)
	if err != nil {
		return nil, err
	}
	out := []model.AgentPolicy{}
	cache := store.NewMembershipCache()
	for i := range policies {
		p := &policies[i]
		if !p.Enabled || !p.Triggers.Matches(trigger) {
			continue
		}
		in, err := e.st.InScopeComputer(ctx, comp, p.Scope, cache)
		if err != nil {
			return nil, err
		}
		if !in {
			continue
		}
		ok, err := e.st.FrequencySatisfied(ctx, p.ID, comp.ID, p.Frequency)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		ap, err := e.PrepareFor(ctx, p, comp, trigger)
		var tooOld *ErrAgentTooOld
		if errors.As(err, &tooOld) {
			if err := e.st.RecordPolicyRefusal(ctx, p.ID, comp.ID, trigger, tooOld.Reason); err != nil {
				e.log.Warn("recording a refused policy", "policy", p.Name, "err", err)
			}
			continue
		}
		if err != nil {
			e.log.Warn("preparing policy", "policy", p.Name, "err", err)
			continue
		}
		logID, err := e.st.CreatePendingPolicyLog(ctx, p.ID, comp.ID, trigger)
		if err != nil {
			return nil, err
		}
		ap.LogID = logID
		out = append(out, *ap)
	}
	return out, nil
}

// Agent capabilities: optional features an agent must report before it is given a policy that
// uses them (older agents ignore fields they do not know and would report success).
const CapLocalAccounts = "policy_accounts"

// ErrAgentTooOld means the computer's agent cannot run every part of the policy.
type ErrAgentTooOld struct{ Reason string }

func (e *ErrAgentTooOld) Error() string { return e.Reason }

// missingCapability explains why an agent with caps cannot run p ("" when it can).
func missingCapability(p *model.Policy, caps []string) string {
	if len(p.Accounts) > 0 && !slices.Contains(caps, CapLocalAccounts) {
		return "This Mac's VibeMDM agent is too old for Local Accounts actions, so the policy was not sent (it would have skipped them). Update the agent and run the policy again."
	}
	return ""
}

// PrepareFor is Prepare for one computer: it refuses (*ErrAgentTooOld) when the computer's
// agent lacks a feature the policy uses.
func (e *Engine) PrepareFor(ctx context.Context, p *model.Policy, comp *model.Computer, trigger string) (*model.AgentPolicy, error) {
	caps, err := e.st.AgentCapabilities(ctx, comp.ID)
	if err != nil {
		return nil, err
	}
	if reason := missingCapability(p, caps); reason != "" {
		return nil, &ErrAgentTooOld{Reason: reason}
	}
	return e.Prepare(ctx, p, trigger)
}

// Prepare inlines scripts and packages so the agent can execute the policy.
func (e *Engine) Prepare(ctx context.Context, p *model.Policy, trigger string) (*model.AgentPolicy, error) {
	ap := &model.AgentPolicy{
		PolicyID: p.ID, Name: p.Name, Trigger: trigger, RunCommand: p.RunCommand, UpdateInventory: p.UpdateInventory,
		Reboot: p.Reboot, UserInteraction: p.UserInteraction, Scripts: []model.AgentScriptStep{}, Packages: []model.AgentPackageStep{},
	}
	for _, ps := range p.Scripts {
		sc, err := e.st.GetScript(ctx, ps.ScriptID)
		if err != nil {
			e.log.Warn("policy references missing script", "policy", p.Name, "script_id", ps.ScriptID)
			continue
		}
		params := make([]string, 8)
		copy(params, ps.Parameters)
		prio := ps.Priority
		if prio == "" {
			prio = sc.Priority
		}
		ap.Scripts = append(ap.Scripts, model.AgentScriptStep{ScriptID: sc.ID, Name: sc.Name, Priority: prio, Contents: sc.Contents, Parameters: params})
	}
	// "before" scripts first, preserving configured order within each group.
	sort.SliceStable(ap.Scripts, func(i, j int) bool {
		return ap.Scripts[i].Priority == "before" && ap.Scripts[j].Priority != "before"
	})
	pkgs := []model.Package{}
	for _, pp := range p.Packages {
		pkg, err := e.st.GetPackage(ctx, pp.PackageID)
		if err != nil {
			e.log.Warn("policy references missing package", "policy", p.Name, "package_id", pp.PackageID)
			continue
		}
		pkgs = append(pkgs, *pkg)
	}
	sort.SliceStable(pkgs, func(i, j int) bool { return pkgs[i].Priority < pkgs[j].Priority })
	ap.Apps = []model.AgentAppStep{}
	for _, pa := range p.Apps {
		if pa.AppID != "" { // a Mac App from the catalog: its label and options
			app, err := e.st.GetMacApp(ctx, pa.AppID)
			if err != nil {
				e.log.Warn("policy refers to a Mac App that no longer exists", "policy", p.Name, "app_id", pa.AppID)
				continue
			}
			args, _, err := e.st.InstallomatorArgs(ctx, app.Label, app.Options)
			if err != nil {
				e.log.Warn("policy Installomator step skipped", "policy", p.Name, "app", app.Name, "err", err)
				continue
			}
			ap.Apps = append(ap.Apps, model.AgentAppStep{AppID: app.ID, Name: app.Name, Label: app.Label, Args: args})
			continue
		}
		args, name, err := e.st.InstallomatorArgs(ctx, pa.Label, pa.Options)
		if err != nil {
			e.log.Warn("policy Installomator step skipped", "policy", p.Name, "label", pa.Label, "err", err)
			continue
		}
		ap.Apps = append(ap.Apps, model.AgentAppStep{Name: name, Label: pa.Label, Args: args})
	}
	ap.Accounts = make([]model.PolicyAccount, 0, len(p.Accounts))
	for _, a := range p.Accounts {
		a.Password = "" // fetched by the agent during the run, never sent with the policy
		ap.Accounts = append(ap.Accounts, a)
	}
	for _, pkg := range pkgs {
		ap.Packages = append(ap.Packages, model.AgentPackageStep{
			PackageID: pkg.ID, Name: pkg.Name, Filename: pkg.Filename, SHA256: pkg.SHA256, SizeBytes: pkg.SizeBytes,
			URL: "/agent/v1/packages/" + pkg.ID,
		})
	}
	return ap, nil
}
