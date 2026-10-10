package compliance

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Service schedules scans and turns agent results into stored results.
type Service struct {
	st     *store.Store
	notify func(computerID string)
	log    *slog.Logger
}

// NewService creates the compliance service. notify wakes a long-polling agent (may be nil).
func NewService(st *store.Store, notify func(string), logger *slog.Logger) *Service {
	return &Service{st: st, notify: notify, log: logger}
}

// PlannedRule is one rule as it applies to a specific Mac.
type PlannedRule struct {
	Rule     *Rule
	Check    string
	Result   *Result
	Fix      string
	Excluded string // exemption reason
	Manual   string // why it can't be scanned
	NotOnOS  bool   // not applicable to this macOS version or CPU architecture
}

// Plan resolves a baseline's rules for a Mac's macOS version and architecture.
func Plan(cat *Catalog, b *model.ComplianceBaseline, osVersion, arch string) []PlannedRule {
	major := MajorVersion(osVersion)
	parent := b.Benchmark
	if bl, ok := cat.Baselines[b.Benchmark]; ok {
		parent = bl.ParentValues
	}
	var out []PlannedRule
	for _, r := range cat.RulesFor(b.Benchmark, b.ExtraRules, nil) {
		pr := PlannedRule{Rule: r, Excluded: b.Exclusions[r.ID], Manual: r.ManualReason(), NotOnOS: !r.AppliesTo(major) || !r.AppliesToArch(arch)}
		check, res, fix := r.CheckFor(major)
		if v, ok := r.ODVValue(parent, b.ODVOverrides[r.ID]); ok {
			check, fix = Substitute(check, v), Substitute(fix, v)
			if res != nil {
				rv := *res
				rv.Value = Substitute(rv.Value, v)
				res = &rv
			}
		}
		pr.Check, pr.Result, pr.Fix = check, res, fix
		out = append(out, pr)
	}
	return out
}

// BuildScan returns the agent payload and the expected value per rule.
func BuildScan(cat *Catalog, b *model.ComplianceBaseline, osVersion, arch string, remediate bool) (model.AgentComplianceScan, map[string]string) {
	scan := model.AgentComplianceScan{Baseline: b.Benchmark, Remediate: remediate}
	expected := map[string]string{}
	for _, pr := range Plan(cat, b, osVersion, arch) {
		if pr.Excluded != "" || pr.Manual != "" || pr.NotOnOS || pr.Check == "" || pr.Result == nil {
			continue
		}
		ar := model.AgentComplianceRule{ID: pr.Rule.ID, Check: pr.Check, ExpectedType: pr.Result.Type, Expected: pr.Result.ExpectedOutput()}
		if remediate {
			ar.Fix = pr.Fix
		}
		scan.Rules = append(scan.Rules, ar)
		expected[pr.Rule.ID] = ar.Expected
	}
	sort.Slice(scan.Rules, func(i, j int) bool { return scan.Rules[i].ID < scan.Rules[j].ID })
	return scan, expected
}

// RequestScan queues a scan of a baseline on one computer through the agent.
func (s *Service) RequestScan(ctx context.Context, b *model.ComplianceBaseline, comp *model.Computer, trigger string, remediate bool, by string) (string, error) {
	if !comp.AgentEnrolled {
		return "", fmt.Errorf("%s does not have the VibeMDM agent; compliance scans run through the agent", comp.Name)
	}
	cat, err := Load()
	if err != nil {
		return "", err
	}
	payload, _ := BuildScan(cat, b, comp.OSVersion, comp.Architecture, remediate)
	if len(payload.Rules) == 0 {
		return "", fmt.Errorf("no scannable rules apply to %s (macOS %s)", comp.Name, comp.OSVersion)
	}
	scanID, err := s.st.CreateComplianceScan(ctx, b.ID, comp.ID, trigger, remediate)
	if err != nil {
		return "", err
	}
	payload.ScanID = scanID
	task, err := s.st.CreateTask(ctx, comp.ID, "compliance_scan", map[string]any{"scan": payload}, by)
	if err != nil {
		s.st.FailComplianceScan(ctx, scanID, err.Error())
		return "", err
	}
	s.st.SetComplianceScanTask(ctx, scanID, task.ID)
	if s.notify != nil {
		s.notify(comp.ID)
	}
	return scanID, nil
}

// Schedule queues due scans for every enabled baseline. It returns how many were queued.
func (s *Service) Schedule(ctx context.Context) int {
	baselines, err := s.st.ListComplianceBaselines(ctx)
	if err != nil {
		s.log.Warn("compliance: listing baselines", "err", err)
		return 0
	}
	queued := 0
	for i := range baselines {
		b := &baselines[i]
		if !b.Enabled {
			continue
		}
		ids, err := s.st.ResolveScope(ctx, b.Scope)
		if err != nil {
			s.log.Warn("compliance: resolving scope", "baseline", b.Name, "err", err)
			continue
		}
		interval := time.Duration(b.ScanIntervalHours) * time.Hour
		for _, id := range ids {
			due, err := s.st.ComplianceScanDue(ctx, b.ID, id, interval)
			if err != nil || !due {
				continue
			}
			comp, err := s.st.GetComputer(ctx, id)
			if err != nil || !comp.AgentEnrolled {
				continue
			}
			if _, err := s.RequestScan(ctx, b, comp, "scheduled", b.AutoRemediate, "system"); err != nil {
				s.log.Debug("compliance: queue scan", "baseline", b.Name, "computer", comp.Name, "err", err)
				continue
			}
			queued++
		}
	}
	if queued > 0 {
		s.log.Info("queued compliance scans", "count", queued)
	}
	return queued
}

// Submit validates and stores an agent's scan results.
func (s *Service) Submit(ctx context.Context, scanID string, comp *model.Computer, sub model.AgentComplianceSubmit) (*model.ComplianceScan, error) {
	scan, err := s.st.GetComplianceScan(ctx, scanID)
	if err != nil || scan.ComputerID != comp.ID {
		return nil, store.ErrNotFound
	}
	b, err := s.st.GetComplianceBaseline(ctx, scan.BaselineID)
	if err != nil {
		return nil, err
	}
	cat, err := Load()
	if err != nil {
		return nil, err
	}
	osv := sub.OSVersion
	if osv == "" {
		osv = comp.OSVersion
	}
	// Recompute what we asked for; results for anything else are ignored.
	_, expected := BuildScan(cat, b, osv, comp.Architecture, scan.Remediate)
	if len(expected) == 0 {
		_, expected = BuildScan(cat, b, comp.OSVersion, comp.Architecture, scan.Remediate)
	}
	return s.st.CompleteComplianceScan(ctx, scanID, comp.ID, sub, expected)
}

// Rescan queues a scan of a baseline on one computer using the baseline's remediation setting.
func (s *Service) Rescan(ctx context.Context, baselineID, computerID string) error {
	b, err := s.st.GetComplianceBaseline(ctx, baselineID)
	if err != nil {
		return err
	}
	comp, err := s.st.GetComputer(ctx, computerID)
	if err != nil {
		return err
	}
	_, err = s.RequestScan(ctx, b, comp, "health", b.AutoRemediate, "health")
	return err
}
