// Package onboarding runs white-glove enrollment workflows: when a Mac in scope
// first has a user logged in, the agent shows a swiftDialog (MIT,
// github.com/swiftDialog/swiftDialog) window that collects details such as the
// computer name and asset tag, then installs apps, runs policies and scripts
// with a live progress list, and finishes with an optional restart.
package onboarding

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/policy"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Service starts and resolves onboarding runs.
type Service struct {
	st     *store.Store
	engine *policy.Engine
	notify func(string)
	log    *slog.Logger
}

// New creates the service.
func New(st *store.Store, engine *policy.Engine, notify func(string), logger *slog.Logger) *Service {
	return &Service{st: st, engine: engine, notify: notify, log: logger}
}

// Field keys an onboarding form may collect.
var FieldKeys = map[string]string{
	"computer_name": "Computer name", "asset_tag": "Asset tag", "building": "Building", "department": "Department", "room": "Room",
	"assigned_user": "Username", "assigned_real_name": "Full name", "assigned_email": "Email", "assigned_position": "Position",
}

// Validate checks a workflow before it is saved.
func Validate(ctx context.Context, st *store.Store, w *model.OnboardingWorkflow) error {
	w.Name = strings.TrimSpace(w.Name)
	if w.Name == "" {
		return fmt.Errorf("name required")
	}
	c := &w.Config
	if strings.TrimSpace(c.Title) == "" {
		c.Title = "Setting up your Mac"
	}
	switch c.FinalAction {
	case "", "none", "restart", "logout", "shutdown":
	default:
		return fmt.Errorf("final action must be none, restart, logout or shutdown")
	}
	var errs []string
	seen := map[string]bool{}
	for i, f := range c.Fields {
		if _, ok := FieldKeys[f.Key]; !ok {
			errs = append(errs, fmt.Sprintf("field %d: unknown key %q", i+1, f.Key))
		}
		if seen[f.Key] {
			errs = append(errs, fmt.Sprintf("field %q appears twice", f.Key))
		}
		seen[f.Key] = true
		if strings.TrimSpace(f.Label) == "" {
			c.Fields[i].Label = FieldKeys[f.Key]
		}
		// swiftDialog separates field options with commas.
		for _, s := range append([]string{f.Label, f.Placeholder, f.RegexError, f.Regex}, f.Options...) {
			if strings.Contains(s, ",") && s != f.Regex {
				errs = append(errs, fmt.Sprintf("field %q: commas are not allowed in labels, placeholders or options", f.Key))
				break
			}
		}
	}
	if len(c.Steps) == 0 && len(c.Fields) == 0 {
		errs = append(errs, "add at least one field or step")
	}
	for i, s := range c.Steps {
		n := fmt.Sprintf("step %d", i+1)
		if strings.Contains(s.Title, ",") || strings.TrimSpace(s.Title) == "" {
			errs = append(errs, n+": a title without commas is required")
		}
		switch s.Type {
		case "app":
			if _, err := st.GetMacApp(ctx, s.AppID); err != nil {
				errs = append(errs, n+": choose an app from the Mac Apps catalog")
			}
		case "policy":
			if _, err := st.GetPolicy(ctx, s.PolicyID); err != nil {
				errs = append(errs, n+": choose a policy")
			}
		case "script":
			if _, err := st.GetScript(ctx, s.ScriptID); err != nil {
				errs = append(errs, n+": choose a script")
			}
		case "wait":
			if s.Seconds < 1 || s.Seconds > 3600 {
				errs = append(errs, n+": wait between 1 and 3600 seconds")
			}
		case "recon":
		default:
			errs = append(errs, n+": unknown type "+s.Type)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return nil
}

// BuildPayload resolves a workflow for one computer.
func (s *Service) BuildPayload(ctx context.Context, w *model.OnboardingWorkflow, comp *model.Computer, runID string) (*model.AgentOnboarding, error) {
	settings, _ := s.st.GetSettings(ctx)
	c := w.Config
	p := &model.AgentOnboarding{RunID: runID, Title: c.Title, Message: c.Message, Icon: c.Icon, Banner: c.Banner, BlurScreen: c.BlurScreen,
		Fields: c.Fields, ComputerNameTemplate: c.ComputerNameTemplate, FinalAction: c.FinalAction, FinalMessage: c.FinalMessage, OrgName: settings.OrgName}
	if args, _, err := s.st.InstallomatorArgs(ctx, "swiftdialog", model.InstallomatorOptions{Notify: "silent"}); err == nil {
		p.DialogArgs = args
	}
	for _, st := range c.Steps {
		as := model.AgentOnboardingStep{Title: st.Title, Subtitle: st.Subtitle, Icon: st.Icon, Type: st.Type, Seconds: st.Seconds, ContinueOnError: st.ContinueOnError}
		switch st.Type {
		case "app":
			app, err := s.st.GetMacApp(ctx, st.AppID)
			if err != nil {
				return nil, fmt.Errorf("step %q: app no longer exists", st.Title)
			}
			args, _, err := s.st.InstallomatorArgs(ctx, app.Label, app.Options)
			if err != nil {
				return nil, fmt.Errorf("step %q: %w", st.Title, err)
			}
			as.App = &model.AgentAppStep{Name: app.Name, Label: app.Label, Args: args}
		case "policy":
			pol, err := s.st.GetPolicy(ctx, st.PolicyID)
			if err != nil {
				return nil, fmt.Errorf("step %q: policy no longer exists", st.Title)
			}
			ap, err := s.engine.PrepareFor(ctx, pol, comp, "onboarding")
			if err != nil {
				return nil, err
			}
			if logID, err := s.st.CreatePendingPolicyLog(ctx, pol.ID, comp.ID, "onboarding"); err == nil {
				ap.LogID = logID
			}
			as.Policy = ap
		case "script":
			sc, err := s.st.GetScript(ctx, st.ScriptID)
			if err != nil {
				return nil, fmt.Errorf("step %q: script no longer exists", st.Title)
			}
			as.ScriptName, as.ScriptContents = sc.Name, sc.Contents
		}
		p.Steps = append(p.Steps, as)
	}
	return p, nil
}

// Start queues a run of a workflow on a computer (replacing a previous run when force).
func (s *Service) Start(ctx context.Context, w *model.OnboardingWorkflow, comp *model.Computer, by string, force bool) (string, error) {
	if !comp.AgentEnrolled {
		return "", fmt.Errorf("%s does not have the VibeMDM agent", comp.Name)
	}
	if force {
		s.st.ResetOnboardingRun(ctx, w.ID, comp.ID)
	}
	runID, err := s.st.CreateOnboardingRun(ctx, w.ID, comp.ID)
	if err != nil {
		return "", fmt.Errorf("an onboarding run already exists for %s (reset it to run again)", comp.Name)
	}
	payload, err := s.BuildPayload(ctx, w, comp, runID)
	if err != nil {
		s.st.CompleteOnboardingRun(ctx, runID, comp.ID, model.AgentOnboardingSubmit{Status: "failed", Log: err.Error()})
		return "", err
	}
	task, err := s.st.CreateTask(ctx, comp.ID, "onboarding", map[string]any{"onboarding": payload}, by)
	if err != nil {
		return "", err
	}
	s.st.SetOnboardingRunTask(ctx, runID, task.ID)
	if s.notify != nil {
		s.notify(comp.ID)
	}
	s.log.Info("onboarding queued", "workflow", w.Name, "computer", comp.Name)
	return runID, nil
}

// systemUsers are console "users" that are not a person at the desk.
var systemUsers = map[string]bool{"": true, "root": true, "_mbsetupuser": true, "loginwindow": true}

// MaybeStart starts the highest-priority in-scope workflow the Mac has not run,
// once a real user is logged in (swiftDialog needs a console user).
func (s *Service) MaybeStart(ctx context.Context, comp *model.Computer, consoleUser string) {
	if systemUsers[consoleUser] || !comp.AgentEnrolled {
		return
	}
	wfs, err := s.st.ListOnboardingWorkflows(ctx)
	if err != nil {
		return
	}
	cache := store.NewMembershipCache()
	for i := range wfs {
		w := &wfs[i]
		if !w.Enabled {
			continue
		}
		in, err := s.st.InScopeComputer(ctx, comp, w.Scope, cache)
		if err != nil || !in {
			continue
		}
		done, err := s.st.HasOnboardingRun(ctx, w.ID, comp.ID)
		if err != nil {
			return
		}
		if !done {
			if _, err := s.Start(ctx, w, comp, "system", false); err != nil {
				s.log.Warn("starting onboarding", "workflow", w.Name, "computer", comp.Name, "err", err)
			}
		}
		return // only the highest-priority workflow in scope applies
	}
}

// ApplyUserEntry writes collected values to the computer record.
func (s *Service) ApplyUserEntry(ctx context.Context, comp *model.Computer, entry map[string]string) error {
	var e store.ComputerEdit
	set := func(dst **string, key string) {
		if v, ok := entry[key]; ok && strings.TrimSpace(v) != "" {
			v = strings.TrimSpace(v)
			*dst = &v
		}
	}
	set(&e.Name, "computer_name")
	set(&e.AssetTag, "asset_tag")
	set(&e.Building, "building")
	set(&e.Department, "department")
	set(&e.Room, "room")
	set(&e.AssignedUser, "assigned_user")
	set(&e.AssignedRealName, "assigned_real_name")
	set(&e.AssignedEmail, "assigned_email")
	set(&e.AssignedPosition, "assigned_position")
	return s.st.UpdateComputer(ctx, comp.ID, e)
}
