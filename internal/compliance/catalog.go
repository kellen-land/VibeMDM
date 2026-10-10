// Package compliance integrates the macOS Security Compliance Project (mSCP):
// its rules and baselines are embedded as a catalog, scans run through the
// agent, and baselines can be enforced with a generated configuration profile.
//
// mSCP content is licensed under CC BY 4.0 (see internal/compliance/NOTICE.md).
package compliance

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed catalog.json.gz
var catalogGz []byte

// Result is the expected output of a rule's check.
type Result struct {
	Type  string `json:"type"` // string | integer | base64
	Value string `json:"value"`
}

// Benchmark ties a rule to a baseline on one macOS version.
type Benchmark struct {
	Name     string `json:"name"`
	Severity string `json:"severity,omitempty"`
}

// PlatformVersion is a rule's applicability (and optional overrides) for one macOS major version.
type PlatformVersion struct {
	Benchmarks []Benchmark `json:"benchmarks,omitempty"`
	Check      string      `json:"check,omitempty"`
	Result     *Result     `json:"result,omitempty"`
	Fix        string      `json:"fix,omitempty"`
}

// ODVHint describes an organization-defined value.
type ODVHint struct {
	Datatype    string   `json:"datatype"`
	Description string   `json:"description"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
}

// ODV is an organization-defined value with per-baseline defaults.
type ODV struct {
	Hint   ODVHint        `json:"hint"`
	Values map[string]any `json:"values"` // "recommended" and baseline keys
}

// Payload is configuration-profile content a rule needs.
type Payload struct {
	PayloadType string           `json:"payload_type"`
	Content     []map[string]any `json:"content"`
}

// Rule is one mSCP rule.
type Rule struct {
	ID           string                     `json:"id"`
	Title        string                     `json:"title"`
	Section      string                     `json:"section"`
	Discussion   string                     `json:"discussion"`
	Tags         []string                   `json:"tags"`
	References   map[string]any             `json:"references,omitempty"`
	Platforms    map[string]PlatformVersion `json:"platforms"`
	Check        string                     `json:"check,omitempty"`
	Result       *Result                    `json:"result,omitempty"`
	Fix          string                     `json:"fix,omitempty"`
	ODV          *ODV                       `json:"odv,omitempty"`
	Mobileconfig []Payload                  `json:"mobileconfig,omitempty"`
}

// BaselineSection groups rules for display.
type BaselineSection struct {
	Section string   `json:"section"`
	Rules   []string `json:"rules"`
}

// Baseline is an mSCP baseline (benchmark).
type Baseline struct {
	Key          string            `json:"key"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	ParentValues string            `json:"parent_values"`
	Sections     []BaselineSection `json:"sections"`
}

// Catalog is the embedded mSCP content.
type Catalog struct {
	Source    string              `json:"source"`
	Release   string              `json:"release"`
	Platform  string              `json:"platform"`
	Rules     map[string]*Rule    `json:"rules"`
	Baselines map[string]Baseline `json:"baselines"`
}

var (
	loadOnce sync.Once
	loaded   *Catalog
	loadErr  error
)

// Load returns the embedded catalog (parsed once).
func Load() (*Catalog, error) {
	loadOnce.Do(func() {
		zr, err := gzip.NewReader(bytes.NewReader(catalogGz))
		if err != nil {
			loadErr = err
			return
		}
		var c Catalog
		if err := json.NewDecoder(zr).Decode(&c); err != nil {
			loadErr = fmt.Errorf("decoding mSCP catalog: %w", err)
			return
		}
		loaded = &c
	})
	return loaded, loadErr
}

// nonScannable tags mark rules that cannot be evaluated by a script.
// (Mirrors mSCP's own compliance script; "manual" rules with a check are still scanned.)
var nonScannable = map[string]string{
	"inherent": "inherent to macOS", "permanent": "permanent finding (cannot be configured)",
	"n_a": "not applicable", "not_applicable": "not applicable", "supplemental": "supplemental guidance",
}

// ManualReason returns why a rule cannot be scanned automatically ("" if it can).
func (r *Rule) ManualReason() string {
	for _, t := range r.Tags {
		if why, ok := nonScannable[t]; ok {
			return why
		}
	}
	if strings.HasPrefix(r.ID, "supplemental") {
		return "supplemental guidance"
	}
	if r.Check == "" && !r.hasVersionCheck() {
		for _, t := range r.Tags {
			if t == "manual" {
				return "requires manual review"
			}
		}
		return "no automated check"
	}
	return ""
}

// Arch returns the CPU architecture a rule is limited to ("arm64", "i386" or "").
func (r *Rule) Arch() string {
	for _, t := range r.Tags {
		if t == "arm64" || t == "i386" {
			return t
		}
	}
	return ""
}

// AppliesToArch reports whether the rule applies to a Mac's architecture
// (as reported in inventory: "arm64", "x86_64"/"i386", or "" when unknown).
func (r *Rule) AppliesToArch(arch string) bool {
	want := r.Arch()
	if want == "" || arch == "" {
		return true
	}
	if want == "i386" {
		return arch == "i386" || arch == "x86_64"
	}
	return arch == want
}

// ExpectedOutput is the exact check output that means "pass". mSCP compares
// the check's output (trailing newlines removed) to the expected value; base64
// expectations are encoded because those checks pipe their output to base64.
func (res Result) ExpectedOutput() string {
	if res.Type == "base64" {
		return base64.StdEncoding.EncodeToString([]byte(res.Value))
	}
	return res.Value
}

func (r *Rule) hasVersionCheck() bool {
	for _, p := range r.Platforms {
		if p.Check != "" {
			return true
		}
	}
	return false
}

// MajorVersion maps "15.4.1" → "15.0", "26.1" → "26.0".
func MajorVersion(osVersion string) string {
	maj, _, _ := strings.Cut(strings.TrimSpace(osVersion), ".")
	if maj == "" {
		return ""
	}
	return maj + ".0"
}

// AppliesTo reports whether a rule covers a macOS major version ("" = unknown → assume yes).
func (r *Rule) AppliesTo(major string) bool {
	if major == "" || len(r.Platforms) == 0 {
		return true
	}
	_, ok := r.Platforms[major]
	return ok
}

// Severity returns the severity a baseline assigns to the rule on a version (may be "").
func (r *Rule) Severity(baseline, major string) string {
	try := func(p PlatformVersion) string {
		for _, b := range p.Benchmarks {
			if b.Name == baseline && b.Severity != "" {
				return b.Severity
			}
		}
		return ""
	}
	if p, ok := r.Platforms[major]; ok {
		if s := try(p); s != "" {
			return s
		}
	}
	for _, v := range sortedVersions(r.Platforms) {
		if s := try(r.Platforms[v]); s != "" {
			return s
		}
	}
	return ""
}

// CheckFor returns the check, expected result and fix for a macOS major version.
func (r *Rule) CheckFor(major string) (check string, res *Result, fix string) {
	check, res, fix = r.Check, r.Result, r.Fix
	if p, ok := r.Platforms[major]; ok {
		if p.Check != "" {
			check = p.Check
		}
		if p.Result != nil {
			res = p.Result
		}
		if p.Fix != "" {
			fix = p.Fix
		}
	}
	return check, res, fix
}

// ODVValue resolves the organization-defined value for a baseline, honoring an override.
func (r *Rule) ODVValue(parentValues string, override any) (any, bool) {
	if r.ODV == nil {
		return nil, false
	}
	if override != nil && override != "" {
		return override, true
	}
	if v, ok := r.ODV.Values[parentValues]; ok {
		return v, true
	}
	v, ok := r.ODV.Values["recommended"]
	return v, ok
}

// ValidateODV checks an override against the rule's hint.
func (r *Rule) ValidateODV(v any) error {
	if r.ODV == nil {
		return fmt.Errorf("%s has no organization-defined value", r.ID)
	}
	h := r.ODV.Hint
	switch h.Datatype {
	case "number", "integer":
		f, err := strconv.ParseFloat(fmt.Sprint(v), 64)
		if err != nil {
			return fmt.Errorf("%s: value must be a number", r.ID)
		}
		if h.Min != nil && f < *h.Min {
			return fmt.Errorf("%s: value must be at least %v", r.ID, *h.Min)
		}
		if h.Max != nil && f > *h.Max {
			return fmt.Errorf("%s: value must be at most %v", r.ID, *h.Max)
		}
	default:
		s := fmt.Sprint(v)
		if strings.ContainsAny(s, "\n\r") || len(s) > 1024 {
			return fmt.Errorf("%s: value must be a single line under 1 KB", r.ID)
		}
	}
	return nil
}

// odvText renders an ODV for substitution into shell, keeping scripts safe.
func odvText(v any) string {
	switch t := v.(type) {
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	}
	return fmt.Sprint(v)
}

// Substitute replaces $ODV in a script. Numeric ODVs are inserted verbatim;
// strings are inserted as-is too (mSCP checks embed them inside their own
// quoting), but they were validated as single-line values.
func Substitute(script string, v any) string {
	if v == nil {
		return script
	}
	return strings.ReplaceAll(script, "$ODV", odvText(v))
}

func sortedVersions(m map[string]PlatformVersion) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.ParseFloat(out[i], 64)
		b, _ := strconv.ParseFloat(out[j], 64)
		return a > b
	})
	return out
}

// RulesFor returns the scannable and manual rule IDs of a baseline plus extras, minus exclusions.
func (c *Catalog) RulesFor(baselineKey string, extra []string, excluded map[string]string) []*Rule {
	seen := map[string]bool{}
	var out []*Rule
	add := func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		if _, skip := excluded[id]; skip {
			return
		}
		if r, ok := c.Rules[id]; ok {
			out = append(out, r)
		}
	}
	if b, ok := c.Baselines[baselineKey]; ok {
		for _, s := range b.Sections {
			for _, id := range s.Rules {
				add(id)
			}
		}
	}
	for _, id := range extra {
		add(id)
	}
	return out
}
