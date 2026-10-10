// Package ddm implements Apple's Declarative Device Management protocol for
// VibeMDM: it builds each device's declaration set, computes sync tokens, and
// serves the DeclarativeManagement check-in endpoints (tokens,
// declaration-items, declaration/<type>/<id>, status).
//
// Declaration types and payload keys follow Apple's published schema
// (github.com/apple/device-management, MIT licensed).
package ddm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Declaration is one DDM declaration as served to a device.
type Declaration struct {
	Type        string         `json:"Type"`
	Identifier  string         `json:"Identifier"`
	ServerToken string         `json:"ServerToken"`
	Payload     map[string]any `json:"Payload"`
}

// Kind returns the manifest section a declaration belongs to.
func (d Declaration) Kind() string {
	switch {
	case strings.HasPrefix(d.Type, "com.apple.activation."):
		return "activation"
	case strings.HasPrefix(d.Type, "com.apple.asset."):
		return "asset"
	case strings.HasPrefix(d.Type, "com.apple.management."):
		return "management"
	default:
		return "configuration"
	}
}

// Declaration types used by VibeMDM.
const (
	TypeStatusSubscriptions = "com.apple.configuration.management.status-subscriptions"
	TypeUpdateEnforcement   = "com.apple.configuration.softwareupdate.enforcement.specific"
	TypeUpdateSettings      = "com.apple.configuration.softwareupdate.settings"
	TypeActivationSimple    = "com.apple.activation.simple"

	idPrefix = "com.vibemdm.ddm."
)

// StatusItems VibeMDM subscribes to on every device.
var StatusItems = []string{
	"device.operating-system.version",
	"device.operating-system.build-version",
	"device.model.identifier",
	"softwareupdate.install-state",
	"softwareupdate.pending-version",
	"softwareupdate.failure-reason",
	"softwareupdate.install-reason",
}

// Enforcement is a target OS version with a deadline (macOS 14+).
type Enforcement struct {
	PlanID              string
	TargetOSVersion     string
	TargetBuildVersion  string
	TargetLocalDateTime string // "YYYY-MM-DDTHH:MM:SS" in the device's local time
	DetailsURL          string
}

// Settings controls software update behavior (macOS 15+).
type Settings struct {
	Notifications              *bool  `json:"notifications,omitempty"`
	AllowStandardUserOSUpdates *bool  `json:"allow_standard_user_os_updates,omitempty"`
	MajorDeferralDays          int    `json:"major_deferral_days,omitempty"`
	MinorDeferralDays          int    `json:"minor_deferral_days,omitempty"`
	SystemDeferralDays         int    `json:"system_deferral_days,omitempty"`
	AutoDownload               string `json:"auto_download,omitempty"`                 // Allowed | AlwaysOn | AlwaysOff
	AutoInstallOSUpdates       string `json:"auto_install_os_updates,omitempty"`       // Allowed | AlwaysOn | AlwaysOff
	AutoInstallSecurityUpdates string `json:"auto_install_security_updates,omitempty"` // Allowed | AlwaysOn | AlwaysOff
	RapidSecurityResponse      *bool  `json:"rapid_security_response,omitempty"`
	RapidSecurityRollback      *bool  `json:"rapid_security_response_rollback,omitempty"`
	PlanID                     string `json:"-"`
}

var (
	versionRe  = regexp.MustCompile(`^\d+(\.\d+){0,2}$`)
	buildRe    = regexp.MustCompile(`^[0-9]{2}[A-Z][0-9]{1,5}[a-z]?$`)
	localDTRe  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`)
	actionVals = map[string]bool{"": true, "Allowed": true, "AlwaysOn": true, "AlwaysOff": true}
)

// Validate checks an enforcement target.
func (e Enforcement) Validate() error {
	var errs []error
	if !versionRe.MatchString(e.TargetOSVersion) {
		errs = append(errs, errors.New("target OS version must look like 15.6 or 15.6.1"))
	}
	if e.TargetBuildVersion != "" && !buildRe.MatchString(e.TargetBuildVersion) {
		errs = append(errs, errors.New("target build version must look like 24G84"))
	}
	if !localDTRe.MatchString(e.TargetLocalDateTime) {
		errs = append(errs, errors.New("deadline must be a local date and time like 2026-11-01T17:00:00"))
	} else if _, err := time.Parse("2006-01-02T15:04:05", e.TargetLocalDateTime); err != nil {
		errs = append(errs, errors.New("deadline is not a valid date and time"))
	}
	if e.DetailsURL != "" && !strings.HasPrefix(e.DetailsURL, "https://") {
		errs = append(errs, errors.New("details URL must start with https://"))
	}
	return errors.Join(errs...)
}

// Validate checks settings values against Apple's allowed ranges.
func (s Settings) Validate() error {
	var errs []error
	for name, v := range map[string]int{"major deferral": s.MajorDeferralDays, "minor deferral": s.MinorDeferralDays, "system deferral": s.SystemDeferralDays} {
		if v != 0 && (v < 1 || v > 90) {
			errs = append(errs, fmt.Errorf("%s must be between 1 and 90 days", name))
		}
	}
	for name, v := range map[string]string{"automatic download": s.AutoDownload, "automatic OS update install": s.AutoInstallOSUpdates, "automatic security update install": s.AutoInstallSecurityUpdates} {
		if !actionVals[v] {
			errs = append(errs, fmt.Errorf("%s must be Allowed, AlwaysOn or AlwaysOff", name))
		}
	}
	return errors.Join(errs...)
}

func (s Settings) payload() map[string]any {
	p := map[string]any{}
	if s.Notifications != nil {
		p["Notifications"] = *s.Notifications
	}
	if s.AllowStandardUserOSUpdates != nil {
		p["AllowStandardUserOSUpdates"] = *s.AllowStandardUserOSUpdates
	}
	def := map[string]any{}
	if s.MajorDeferralDays > 0 {
		def["MajorPeriodInDays"] = s.MajorDeferralDays
	}
	if s.MinorDeferralDays > 0 {
		def["MinorPeriodInDays"] = s.MinorDeferralDays
	}
	if s.SystemDeferralDays > 0 {
		def["SystemPeriodInDays"] = s.SystemDeferralDays
	}
	if len(def) > 0 {
		p["Deferrals"] = def
	}
	act := map[string]any{}
	if s.AutoDownload != "" {
		act["Download"] = s.AutoDownload
	}
	if s.AutoInstallOSUpdates != "" {
		act["InstallOSUpdates"] = s.AutoInstallOSUpdates
	}
	if s.AutoInstallSecurityUpdates != "" {
		act["InstallSecurityUpdate"] = s.AutoInstallSecurityUpdates
	}
	if len(act) > 0 {
		p["AutomaticActions"] = act
	}
	if s.RapidSecurityResponse != nil || s.RapidSecurityRollback != nil {
		rsr := map[string]any{}
		if s.RapidSecurityResponse != nil {
			rsr["Enable"] = *s.RapidSecurityResponse
		}
		if s.RapidSecurityRollback != nil {
			rsr["EnableRollback"] = *s.RapidSecurityRollback
		}
		p["RapidSecurityResponse"] = rsr
	}
	return p
}

// serverToken is a stable hash of a declaration's type and payload: it changes
// exactly when the declaration's content changes.
func serverToken(typ string, payload map[string]any) string {
	b, _ := json.Marshal(struct {
		T string
		P map[string]any
	}{typ, payload}) // encoding/json sorts map keys, so this is canonical
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func decl(typ, id string, payload map[string]any) Declaration {
	return Declaration{Type: typ, Identifier: id, ServerToken: serverToken(typ, payload), Payload: payload}
}

func activation(id string, configs ...string) Declaration {
	return decl(TypeActivationSimple, id, map[string]any{"StandardConfigurations": configs})
}

// Build returns the complete declaration set for one device.
func Build(enf *Enforcement, set *Settings) []Declaration {
	items := make([]any, len(StatusItems))
	for i, n := range StatusItems {
		items[i] = map[string]any{"Name": n}
	}
	status := decl(TypeStatusSubscriptions, idPrefix+"status-subscriptions", map[string]any{"StatusItems": items})
	out := []Declaration{status, activation(idPrefix+"activation.status", status.Identifier)}
	if enf != nil {
		p := map[string]any{"TargetOSVersion": enf.TargetOSVersion, "TargetLocalDateTime": enf.TargetLocalDateTime}
		if enf.TargetBuildVersion != "" {
			p["TargetBuildVersion"] = enf.TargetBuildVersion
		}
		if enf.DetailsURL != "" {
			p["DetailsURL"] = enf.DetailsURL
		}
		d := decl(TypeUpdateEnforcement, idPrefix+"softwareupdate.enforcement", p)
		out = append(out, d, activation(idPrefix+"activation.softwareupdate.enforcement", d.Identifier))
	}
	if set != nil {
		if p := set.payload(); len(p) > 0 {
			d := decl(TypeUpdateSettings, idPrefix+"softwareupdate.settings", p)
			out = append(out, d, activation(idPrefix+"activation.softwareupdate.settings", d.Identifier))
		}
	}
	return out
}

// DeclarationsToken identifies a whole declaration set; the device re-syncs
// when it changes.
func DeclarationsToken(decls []Declaration) string {
	parts := make([]string, len(decls))
	for i, d := range decls {
		parts[i] = d.Identifier + ":" + d.ServerToken
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:12])
}

// TokensResponse renders the "tokens" endpoint.
func TokensResponse(token string, ts time.Time) ([]byte, error) {
	return json.Marshal(map[string]any{"SyncTokens": map[string]any{"DeclarationsToken": token, "Timestamp": ts.UTC().Format(time.RFC3339)}})
}

// ItemsResponse renders the "declaration-items" manifest.
func ItemsResponse(decls []Declaration) ([]byte, error) {
	sections := map[string][]map[string]string{"Activations": {}, "Configurations": {}, "Assets": {}, "Management": {}}
	key := map[string]string{"activation": "Activations", "configuration": "Configurations", "asset": "Assets", "management": "Management"}
	for _, d := range decls {
		sections[key[d.Kind()]] = append(sections[key[d.Kind()]], map[string]string{"Identifier": d.Identifier, "ServerToken": d.ServerToken})
	}
	return json.Marshal(map[string]any{"Declarations": sections, "DeclarationsToken": DeclarationsToken(decls)})
}

// Find returns the declaration addressed by a "declaration/<kind>/<id>" endpoint.
func Find(decls []Declaration, endpoint string) (*Declaration, error) {
	rest, ok := strings.CutPrefix(endpoint, "declaration/")
	if !ok {
		return nil, fmt.Errorf("not a declaration endpoint: %s", endpoint)
	}
	kind, id, ok := strings.Cut(rest, "/")
	if !ok {
		return nil, fmt.Errorf("malformed declaration endpoint: %s", endpoint)
	}
	for i := range decls {
		if decls[i].Identifier == id && decls[i].Kind() == kind {
			return &decls[i], nil
		}
	}
	return nil, fmt.Errorf("declaration %s/%s not found", kind, id)
}

// StatusReport is the parsed body of a "status" check-in.
type StatusReport struct {
	StatusItems  map[string]any `json:"StatusItems"`
	Errors       []any          `json:"Errors"`
	Declarations map[string]any `json:"Declarations"`
}

// MergeStatus deep-merges a (possibly incremental) status report into the
// stored state. Apple sends a full report first, then only changed items.
func MergeStatus(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			dm, _ := dst[k].(map[string]any)
			dst[k] = MergeStatus(dm, sm)
			continue
		}
		dst[k] = v
	}
	return dst
}

// CompareVersions compares dotted versions numerically (-1, 0, 1).
func CompareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			fmt.Sscanf(pa[i], "%d", &x)
		}
		if i < len(pb) {
			fmt.Sscanf(pb[i], "%d", &y)
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
