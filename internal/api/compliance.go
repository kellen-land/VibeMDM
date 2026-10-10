package api

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/vibemdm/vibemdm/internal/compliance"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

func catalogOr500(w http.ResponseWriter) *compliance.Catalog {
	cat, err := compliance.Load()
	if err != nil {
		writeErr(w, 500, err.Error())
		return nil
	}
	return cat
}

// complianceCatalog lists the embedded mSCP baselines.
func (h *Handler) complianceCatalog(w http.ResponseWriter, r *http.Request) {
	cat := catalogOr500(w)
	if cat == nil {
		return
	}
	type bl struct {
		Key         string `json:"key"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Rules       int    `json:"rules"`
		Scannable   int    `json:"scannable"`
		Profile     int    `json:"with_profile"`
	}
	out := []bl{}
	for key, b := range cat.Baselines {
		x := bl{Key: key, Title: b.Title, Description: b.Description}
		for _, rule := range cat.RulesFor(key, nil, nil) {
			x.Rules++
			if rule.ManualReason() == "" {
				x.Scannable++
			}
			if len(rule.Mobileconfig) > 0 {
				x.Profile++
			}
		}
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	writeJSON(w, 200, map[string]any{"source": cat.Source, "release": cat.Release, "platform": cat.Platform, "license": "CC BY 4.0", "baselines": out})
}

type ruleView struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Section      string         `json:"section"`
	Discussion   string         `json:"discussion"`
	Tags         []string       `json:"tags"`
	Severity     string         `json:"severity"`
	ManualReason string         `json:"manual_reason"`
	HasFix       bool           `json:"has_fix"`
	HasProfile   bool           `json:"has_profile"`
	Platforms    []string       `json:"platforms"`
	Arch         string         `json:"arch,omitempty"`
	InBenchmark  bool           `json:"in_benchmark"`
	ODV          *odvView       `json:"odv,omitempty"`
	References   map[string]any `json:"references,omitempty"`
	Check        string         `json:"check,omitempty"`
	Fix          string         `json:"fix,omitempty"`
	Expected     string         `json:"expected,omitempty"`
}

type odvView struct {
	Hint    compliance.ODVHint `json:"hint"`
	Default any                `json:"default"`
}

func viewRule(cat *compliance.Catalog, rule *compliance.Rule, benchmark string, inBenchmark, withScripts bool) ruleView {
	v := ruleView{ID: rule.ID, Title: rule.Title, Section: rule.Section, Discussion: rule.Discussion, Tags: rule.Tags,
		Severity: rule.Severity(benchmark, cat.Platform), ManualReason: rule.ManualReason(), HasProfile: len(rule.Mobileconfig) > 0,
		Arch: rule.Arch(), InBenchmark: inBenchmark, References: rule.References}
	_, res, fix := rule.CheckFor(cat.Platform)
	v.HasFix = fix != ""
	for ver := range rule.Platforms {
		v.Platforms = append(v.Platforms, ver)
	}
	sort.Strings(v.Platforms)
	if rule.ODV != nil {
		parent := benchmark
		if b, ok := cat.Baselines[benchmark]; ok {
			parent = b.ParentValues
		}
		def, _ := rule.ODVValue(parent, nil)
		v.ODV = &odvView{Hint: rule.ODV.Hint, Default: def}
	}
	if withScripts {
		v.Check, v.Fix = rule.Check, rule.Fix
		check, _, fix := rule.CheckFor(cat.Platform)
		v.Check, v.Fix = check, fix
		if res != nil {
			v.Expected = res.Type + ": " + res.Value
		}
	}
	return v
}

// complianceRules lists rules; with ?benchmark= it marks membership and sorts benchmark rules first.
func (h *Handler) complianceRules(w http.ResponseWriter, r *http.Request) {
	cat := catalogOr500(w)
	if cat == nil {
		return
	}
	benchmark := r.URL.Query().Get("benchmark")
	in := map[string]bool{}
	for _, rule := range cat.RulesFor(benchmark, nil, nil) {
		in[rule.ID] = true
	}
	scripts := r.URL.Query().Get("scripts") == "1"
	out := make([]ruleView, 0, len(cat.Rules))
	for _, rule := range cat.Rules {
		if r.URL.Query().Get("all") != "1" && benchmark != "" && !in[rule.ID] {
			continue
		}
		out = append(out, viewRule(cat, rule, benchmark, in[rule.ID], scripts))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].InBenchmark != out[j].InBenchmark {
			return out[i].InBenchmark
		}
		if out[i].Section != out[j].Section {
			return out[i].Section < out[j].Section
		}
		return out[i].ID < out[j].ID
	})
	writeJSON(w, 200, out)
}

func (h *Handler) complianceRule(w http.ResponseWriter, r *http.Request) {
	cat := catalogOr500(w)
	if cat == nil {
		return
	}
	rule, ok := cat.Rules[r.PathValue("rule")]
	if !ok {
		writeErr(w, 404, "rule not found")
		return
	}
	writeJSON(w, 200, viewRule(cat, rule, r.URL.Query().Get("benchmark"), false, true))
}

func validateBaseline(cat *compliance.Catalog, b *model.ComplianceBaseline) error {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return errors.New("name required")
	}
	if _, ok := cat.Baselines[b.Benchmark]; !ok {
		return fmt.Errorf("unknown benchmark %q", b.Benchmark)
	}
	if b.ScanIntervalHours == 0 {
		b.ScanIntervalHours = 24
	}
	if b.ScanIntervalHours < 1 || b.ScanIntervalHours > 720 {
		return errors.New("scan interval must be between 1 and 720 hours")
	}
	var errs []string
	seen := map[string]bool{}
	extra := []string{}
	for _, id := range b.ExtraRules {
		if _, ok := cat.Rules[id]; !ok {
			errs = append(errs, "unknown rule "+id)
		} else if !seen[id] {
			seen[id] = true
			extra = append(extra, id)
		}
	}
	b.ExtraRules = extra
	for id, reason := range b.Exclusions {
		if _, ok := cat.Rules[id]; !ok {
			errs = append(errs, "unknown excluded rule "+id)
		}
		if strings.TrimSpace(reason) == "" {
			errs = append(errs, "exemption for "+id+" needs a reason")
		}
	}
	for id, v := range b.ODVOverrides {
		rule, ok := cat.Rules[id]
		if !ok {
			errs = append(errs, "unknown rule "+id)
			continue
		}
		if v == nil || v == "" {
			delete(b.ODVOverrides, id)
			continue
		}
		if err := rule.ValidateODV(v); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

func complianceProfileIdentifier(id string) string { return "com.vibemdm.compliance." + id }

// buildComplianceProfile renders the enforcement profile for a baseline.
func (h *Handler) buildComplianceProfile(r *http.Request, cat *compliance.Catalog, b *model.ComplianceBaseline) ([]byte, []string, error) {
	settings, _ := h.st.GetSettings(r.Context())
	parent := cat.Baselines[b.Benchmark].ParentValues
	return compliance.BuildProfile(compliance.ProfileInput{
		Identifier: complianceProfileIdentifier(b.ID), DisplayName: "Compliance: " + b.Name, Organization: settings.OrgName,
		ParentValues: parent, Rules: cat.RulesFor(b.Benchmark, b.ExtraRules, b.Exclusions), ODVOverrides: b.ODVOverrides,
	})
}

// syncComplianceProfile creates/updates (or withdraws) the baseline's enforcement profile.
func (h *Handler) syncComplianceProfile(r *http.Request, cat *compliance.Catalog, b *model.ComplianceBaseline) error {
	enabled := b.Enabled && b.DeployProfile
	var payload []byte
	if enabled {
		var err error
		payload, _, err = h.buildComplianceProfile(r, cat, b)
		if err != nil {
			return err
		}
	} else if b.ProfileID == nil {
		return nil
	} else if existing, err := h.st.GetProfile(r.Context(), *b.ProfileID); err == nil {
		payload = []byte(existing.Payload)
	} else {
		return nil
	}
	p, err := h.upsertBuiltin(r, complianceProfileIdentifier(b.ID), "Compliance: "+b.Name,
		"Generated from the mSCP "+b.Benchmark+" benchmark by the compliance baseline \""+b.Name+"\". Edit the baseline, not this profile.",
		payload, enabled, b.Scope)
	if err != nil {
		return err
	}
	if p != nil && (b.ProfileID == nil || *b.ProfileID != p.ID) {
		h.st.SetComplianceProfile(r.Context(), b.ID, &p.ID)
		b.ProfileID = &p.ID
	}
	return nil
}

func (h *Handler) listBaselines(w http.ResponseWriter, r *http.Request) {
	bs, err := h.st.ListComplianceBaselines(r.Context())
	if err != nil {
		h.fail(w, err, "baselines")
		return
	}
	type row struct {
		model.ComplianceBaseline
		Computers int     `json:"computers"`
		Compliant int     `json:"compliant"`
		Score     float64 `json:"score"`
	}
	out := make([]row, 0, len(bs))
	for _, b := range bs {
		x := row{ComplianceBaseline: b}
		sums, err := h.st.ComplianceComputerSummaries(r.Context(), b.ID)
		if err == nil {
			total := 0.0
			for _, s := range sums {
				if s.Total == 0 {
					continue
				}
				x.Computers++
				total += s.Score
				if s.Failed == 0 && s.Errors == 0 {
					x.Compliant++
				}
			}
			if x.Computers > 0 {
				x.Score = total / float64(x.Computers)
			}
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
}

func (h *Handler) getBaseline(w http.ResponseWriter, r *http.Request) {
	b, err := h.st.GetComplianceBaseline(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "baseline")
		return
	}
	writeJSON(w, 200, b)
}

func (h *Handler) saveBaseline(w http.ResponseWriter, r *http.Request) {
	cat := catalogOr500(w)
	if cat == nil {
		return
	}
	var b model.ComplianceBaseline
	if err := decode(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	b.ID = r.PathValue("id")
	if err := validateBaseline(cat, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.ID != "" {
		cur, err := h.st.GetComplianceBaseline(r.Context(), b.ID)
		if err != nil {
			h.fail(w, err, "baseline")
			return
		}
		b.ProfileID = cur.ProfileID
	}
	out, err := h.st.SaveComplianceBaseline(r.Context(), b)
	if err != nil {
		h.fail(w, err, "baseline")
		return
	}
	out.ProfileID = b.ProfileID
	if err := h.syncComplianceProfile(r, cat, out); err != nil {
		writeErr(w, 422, "baseline saved, but the enforcement profile could not be generated: "+err.Error())
		return
	}
	h.st.Audit(r.Context(), actor(r), "compliance.baseline_save", "compliance_baseline", out.ID, map[string]any{"name": out.Name, "benchmark": out.Benchmark})
	code := 200
	if r.PathValue("id") == "" {
		code = 201
	}
	writeJSON(w, code, out)
}

func (h *Handler) deleteBaseline(w http.ResponseWriter, r *http.Request) {
	cat := catalogOr500(w)
	if cat == nil {
		return
	}
	b, err := h.st.GetComplianceBaseline(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "baseline")
		return
	}
	// Withdraw the enforcement profile from every Mac before forgetting the baseline.
	b.Enabled = false
	if err := h.syncComplianceProfile(r, cat, b); err != nil {
		h.log.Warn("withdrawing compliance profile", "baseline", b.Name, "err", err)
	}
	if err := h.st.DeleteComplianceBaseline(r.Context(), b.ID); err != nil {
		h.fail(w, err, "baseline")
		return
	}
	h.st.Audit(r.Context(), actor(r), "compliance.baseline_delete", "compliance_baseline", b.ID, map[string]any{"name": b.Name})
	w.WriteHeader(204)
}

type reportRule struct {
	ruleView
	store.ComplianceRuleCount
	Excluded string `json:"excluded"`
}

// baselineReport summarizes a baseline across the fleet.
func (h *Handler) baselineReport(w http.ResponseWriter, r *http.Request) {
	cat := catalogOr500(w)
	if cat == nil {
		return
	}
	b, err := h.st.GetComplianceBaseline(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "baseline")
		return
	}
	sums, err := h.st.ComplianceComputerSummaries(r.Context(), b.ID)
	if err != nil {
		h.fail(w, err, "report")
		return
	}
	counts, err := h.st.ComplianceRuleCounts(r.Context(), b.ID)
	if err != nil {
		h.fail(w, err, "report")
		return
	}
	targets, _ := h.st.ResolveScope(r.Context(), b.Scope)
	rules := []reportRule{}
	for _, rule := range cat.RulesFor(b.Benchmark, b.ExtraRules, nil) {
		c := counts[rule.ID]
		c.RuleID = rule.ID
		rules = append(rules, reportRule{ruleView: viewRule(cat, rule, b.Benchmark, true, false), ComplianceRuleCount: c, Excluded: b.Exclusions[rule.ID]})
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Failed != rules[j].Failed {
			return rules[i].Failed > rules[j].Failed
		}
		return rules[i].ID < rules[j].ID
	})
	summary := map[string]any{"targeted": len(targets), "scanned": 0, "compliant": 0, "score": 0.0, "failing_rules": 0, "drifted": 0, "remediated": 0}
	total, scanned, compliant := 0.0, 0, 0
	for _, s := range sums {
		if s.Total == 0 {
			continue
		}
		scanned++
		total += s.Score
		if s.Failed == 0 && s.Errors == 0 {
			compliant++
		}
	}
	failing, drifted, remediated := 0, 0, 0
	for _, c := range counts {
		if c.Failed > 0 {
			failing++
		}
		drifted += c.Drifted
		remediated += c.Remediated
	}
	summary["scanned"], summary["compliant"], summary["failing_rules"], summary["drifted"], summary["remediated"] = scanned, compliant, failing, drifted, remediated
	if scanned > 0 {
		summary["score"] = total / float64(scanned)
	}
	writeJSON(w, 200, map[string]any{"baseline": b, "summary": summary, "computers": sums, "rules": rules,
		"release": cat.Release, "benchmark_title": cat.Baselines[b.Benchmark].Title})
}

type computerRuleRow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Section  string `json:"section"`
	Severity string `json:"severity"`
	// Status: pass | fail | error | remediated | not_scanned | excluded | manual | not_applicable
	Status    string  `json:"status"`
	Previous  string  `json:"previous_status,omitempty"`
	Actual    string  `json:"actual,omitempty"`
	Expected  string  `json:"expected,omitempty"`
	Detail    string  `json:"detail,omitempty"`
	CheckedAt *string `json:"checked_at,omitempty"`
	HasFix    bool    `json:"has_fix"`
}

// baselineComputer shows every rule of a baseline for one computer.
func (h *Handler) baselineComputer(w http.ResponseWriter, r *http.Request) {
	cat := catalogOr500(w)
	if cat == nil {
		return
	}
	b, err := h.st.GetComplianceBaseline(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "baseline")
		return
	}
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("cid"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	results, err := h.st.ComplianceResults(r.Context(), b.ID, comp.ID)
	if err != nil {
		h.fail(w, err, "results")
		return
	}
	byRule := map[string]model.ComplianceResult{}
	for _, res := range results {
		byRule[res.RuleID] = res
	}
	rows := []computerRuleRow{}
	for _, pr := range compliance.Plan(cat, b, comp.OSVersion, comp.Architecture) {
		row := computerRuleRow{ID: pr.Rule.ID, Title: pr.Rule.Title, Section: pr.Rule.Section,
			Severity: pr.Rule.Severity(b.Benchmark, compliance.MajorVersion(comp.OSVersion)), HasFix: pr.Fix != ""}
		switch {
		case pr.Excluded != "":
			row.Status, row.Detail = "excluded", pr.Excluded
		case pr.Manual != "":
			row.Status, row.Detail = "manual", pr.Manual
		case pr.NotOnOS:
			row.Status = "not_applicable"
		default:
			if res, ok := byRule[pr.Rule.ID]; ok {
				t := res.CheckedAt.UTC().Format("2006-01-02T15:04:05Z")
				row.Status, row.Previous, row.Actual, row.Expected, row.Detail, row.CheckedAt = res.Status, res.PreviousStatus, res.Actual, res.Expected, res.Detail, &t
			} else {
				row.Status = "not_scanned"
			}
		}
		rows = append(rows, row)
	}
	scans, _ := h.st.ListComplianceScans(r.Context(), b.ID, comp.ID, 10)
	writeJSON(w, 200, map[string]any{"baseline": b, "computer": map[string]any{"id": comp.ID, "name": comp.Name, "os_version": comp.OSVersion,
		"agent_enrolled": comp.AgentEnrolled}, "rules": rows, "scans": scans})
}

// computerCompliance lists every baseline that targets or has scanned a computer.
func (h *Handler) computerCompliance(w http.ResponseWriter, r *http.Request) {
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	bs, err := h.st.ListComplianceBaselines(r.Context())
	if err != nil {
		h.fail(w, err, "baselines")
		return
	}
	type row struct {
		Baseline model.ComplianceBaseline         `json:"baseline"`
		InScope  bool                             `json:"in_scope"`
		Summary  *store.ComplianceComputerSummary `json:"summary"`
	}
	out := []row{}
	cache := store.NewMembershipCache()
	for _, b := range bs {
		in, _ := h.st.InScopeComputer(r.Context(), comp, b.Scope, cache)
		x := row{Baseline: b, InScope: in}
		if sums, err := h.st.ComplianceComputerSummaries(r.Context(), b.ID); err == nil {
			for i := range sums {
				if sums[i].ComputerID == comp.ID {
					x.Summary = &sums[i]
				}
			}
		}
		if in || x.Summary != nil {
			out = append(out, x)
		}
	}
	writeJSON(w, 200, out)
}

// scanBaseline queues scans now: for given computers, or the whole scope.
func (h *Handler) scanBaseline(w http.ResponseWriter, r *http.Request) {
	b, err := h.st.GetComplianceBaseline(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "baseline")
		return
	}
	var req struct {
		ComputerIDs []string `json:"computer_ids"`
		Remediate   *bool    `json:"remediate"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	remediate := b.AutoRemediate
	if req.Remediate != nil {
		remediate = *req.Remediate
	}
	ids := req.ComputerIDs
	if len(ids) == 0 {
		if ids, err = h.st.ResolveScope(r.Context(), b.Scope); err != nil {
			h.fail(w, err, "scope")
			return
		}
	}
	queued, skipped := 0, map[string]string{}
	trigger := "manual"
	if remediate {
		trigger = "manual_remediate"
	}
	for _, id := range ids {
		comp, err := h.st.GetComputer(r.Context(), id)
		if err != nil {
			skipped[id] = "computer not found"
			continue
		}
		if _, err := h.compliance.RequestScan(r.Context(), b, comp, trigger, remediate, actor(r)); err != nil {
			skipped[comp.Name] = err.Error()
			continue
		}
		queued++
	}
	h.st.Audit(r.Context(), actor(r), "compliance.scan", "compliance_baseline", b.ID, map[string]any{"queued": queued, "remediate": remediate})
	writeJSON(w, 200, map[string]any{"queued": queued, "skipped": skipped})
}

func (h *Handler) listBaselineScans(w http.ResponseWriter, r *http.Request) {
	scans, err := h.st.ListComplianceScans(r.Context(), r.PathValue("id"), r.URL.Query().Get("computer_id"), qInt(r, "limit", 100))
	if err != nil {
		h.fail(w, err, "scans")
		return
	}
	writeJSON(w, 200, scans)
}

// baselineProfilePreview returns the profile the baseline would deploy.
func (h *Handler) baselineProfilePreview(w http.ResponseWriter, r *http.Request) {
	cat := catalogOr500(w)
	if cat == nil {
		return
	}
	b, err := h.st.GetComplianceBaseline(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "baseline")
		return
	}
	payload, covered, err := h.buildComplianceProfile(r, cat, b)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"payload": string(payload), "rules": covered, "profile_id": b.ProfileID, "deployed": b.DeployProfile && b.Enabled})
}

// exportBaselineCSV writes every latest result as CSV.
func (h *Handler) exportBaselineCSV(w http.ResponseWriter, r *http.Request) {
	cat := catalogOr500(w)
	if cat == nil {
		return
	}
	b, err := h.st.GetComplianceBaseline(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "baseline")
		return
	}
	results, err := h.st.ComplianceResults(r.Context(), b.ID, "")
	if err != nil {
		h.fail(w, err, "results")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="compliance-%s.csv"`, safeFilename.ReplaceAllString(b.Name, "-")))
	cw := csv.NewWriter(w)
	cw.Write([]string{"computer", "rule_id", "title", "severity", "status", "previous_status", "actual", "expected", "detail", "checked_at"})
	for _, res := range results {
		title, sev := res.RuleID, ""
		if rule, ok := cat.Rules[res.RuleID]; ok {
			title, sev = rule.Title, rule.Severity(b.Benchmark, cat.Platform)
		}
		cw.Write(csvSafe([]string{res.ComputerName, res.RuleID, title, sev, res.Status, res.PreviousStatus, res.Actual, res.Expected, res.Detail,
			res.CheckedAt.UTC().Format("2006-01-02T15:04:05Z")}))
	}
	cw.Flush()
}

// csvSafe neutralizes spreadsheet formula injection in exported cells.
func csvSafe(row []string) []string {
	for i, v := range row {
		if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
			row[i] = "'" + v
		}
	}
	return row
}
