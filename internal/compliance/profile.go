package compliance

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"howett.net/plist"
)

// substituteValue replaces $ODV in profile content, keeping numeric types.
func substituteValue(v any, odv any, has bool) any {
	switch t := v.(type) {
	case string:
		if !has || !strings.Contains(t, "$ODV") {
			return t
		}
		if t == "$ODV" {
			switch o := odv.(type) {
			case int, int64, float64, bool:
				if f, ok := o.(float64); ok && f == float64(int64(f)) {
					return int64(f)
				}
				return o
			case string:
				if n, err := strconv.ParseInt(o, 10, 64); err == nil {
					return n
				}
				return o
			}
		}
		return strings.ReplaceAll(t, "$ODV", odvText(odv))
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = substituteValue(x, odv, has)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = substituteValue(x, odv, has)
		}
		return out
	case float64:
		if t == float64(int64(t)) {
			return int64(t)
		}
	}
	return v
}

// ProfileInput describes what to enforce.
type ProfileInput struct {
	Identifier   string // e.g. com.vibemdm.compliance.<id>
	DisplayName  string
	Organization string
	ParentValues string
	Rules        []*Rule
	ODVOverrides map[string]any
}

// BuildProfile merges the configuration-profile content of the rules into one
// mobileconfig. Payloads of the same type are merged key by key; managed
// preferences (com.apple.ManagedClient.preferences) are emitted per domain in
// MCX "Forced" form. It returns the plist and the rule IDs it covered.
func BuildProfile(in ProfileInput) ([]byte, []string, error) {
	typed := map[string]map[string]any{} // PayloadType -> merged keys
	mcx := map[string]map[string]any{}   // preference domain -> merged keys
	var covered []string
	for _, r := range in.Rules {
		if len(r.Mobileconfig) == 0 {
			continue
		}
		odv, has := r.ODVValue(in.ParentValues, in.ODVOverrides[r.ID])
		covered = append(covered, r.ID)
		for _, p := range r.Mobileconfig {
			for _, item := range p.Content {
				if p.PayloadType == "com.apple.ManagedClient.preferences" {
					for domain, keys := range item {
						km, ok := substituteValue(keys, odv, has).(map[string]any)
						if !ok {
							continue
						}
						if mcx[domain] == nil {
							mcx[domain] = map[string]any{}
						}
						for k, v := range km {
							mcx[domain][k] = v
						}
					}
					continue
				}
				if typed[p.PayloadType] == nil {
					typed[p.PayloadType] = map[string]any{}
				}
				for k, v := range item {
					typed[p.PayloadType][k] = substituteValue(v, odv, has)
				}
			}
		}
	}
	if len(typed)+len(mcx) == 0 {
		return nil, nil, fmt.Errorf("the selected rules have no configuration-profile settings")
	}
	var content []map[string]any
	n := 0
	ident := func() string { n++; return fmt.Sprintf("%s.p%d", in.Identifier, n) }
	for _, pt := range sortedKeys(typed) {
		p := map[string]any{}
		for k, v := range typed[pt] {
			p[k] = v
		}
		id := ident()
		p["PayloadType"], p["PayloadVersion"], p["PayloadIdentifier"], p["PayloadUUID"] = pt, 1, id, stableUUID(id)
		p["PayloadDisplayName"] = pt
		content = append(content, p)
	}
	for _, domain := range sortedKeys(mcx) {
		id := ident()
		content = append(content, map[string]any{
			"PayloadType": "com.apple.ManagedClient.preferences", "PayloadVersion": 1, "PayloadIdentifier": id,
			"PayloadUUID": stableUUID(id), "PayloadDisplayName": domain,
			"PayloadContent": map[string]any{domain: map[string]any{"Forced": []any{map[string]any{"mcx_preference_settings": mcx[domain]}}}},
		})
	}
	top := map[string]any{
		"PayloadType": "Configuration", "PayloadVersion": 1, "PayloadIdentifier": in.Identifier,
		"PayloadUUID": stableUUID(in.Identifier), "PayloadDisplayName": in.DisplayName,
		"PayloadDescription":  "Settings generated from the macOS Security Compliance Project (CC BY 4.0) by VibeMDM.",
		"PayloadOrganization": in.Organization, "PayloadScope": "System", "PayloadRemovalDisallowed": true,
		"PayloadContent": content,
	}
	b, err := plist.MarshalIndent(top, plist.XMLFormat, "\t")
	return b, covered, err
}

// stableUUID derives payload UUIDs from identifiers so regenerating an
// unchanged baseline yields an identical profile (no needless reinstall).
func stableUUID(identifier string) string {
	return strings.ToUpper(uuid.NewSHA1(uuid.NameSpaceURL, []byte("vibemdm:"+identifier)).String())
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
