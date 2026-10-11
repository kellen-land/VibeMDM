package schema

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"howett.net/plist"
)

// MobilePayload is a built-in editor for one iOS/iPadOS payload type
// (internal/schema/mobile/<PayloadType>.json). Besides the usual schema
// keywords the file carries header fields:
//
//	__payloadtype  the PayloadType it produces, e.g. com.apple.applicationaccess
//	__category     the group it is listed under in the payload picker
//	__multiple     true when a profile may hold several (Wi-Fi, Web Clip, …)
//	__order        sort order inside the category
type MobilePayload struct {
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	Multiple    bool    `json:"multiple"`
	Order       int     `json:"order"`
	Schema      *Schema `json:"-"`
	Raw         []byte  `json:"-"`
}

type mobileHeader struct {
	PayloadType string `json:"__payloadtype"`
	Category    string `json:"__category"`
	Multiple    bool   `json:"__multiple"`
	Order       int    `json:"__order"`
}

// MobilePayloads returns the built-in mobile payload editors, by category then order.
func MobilePayloads() []MobilePayload {
	entries, _ := builtinFS.ReadDir("mobile")
	var out []MobilePayload
	for _, e := range entries {
		raw, err := builtinFS.ReadFile(path.Join("mobile", e.Name()))
		if err != nil {
			continue
		}
		s, err := Parse(raw)
		if err != nil {
			panic("invalid mobile payload schema " + e.Name() + ": " + err.Error())
		}
		var h mobileHeader
		json.Unmarshal(raw, &h)
		if h.PayloadType == "" || h.PayloadType+".json" != e.Name() {
			panic("mobile payload schema " + e.Name() + ": __payloadtype must match the file name")
		}
		out = append(out, MobilePayload{Type: h.PayloadType, Title: s.Title, Description: s.Description, Category: h.Category,
			Multiple: h.Multiple, Order: h.Order, Schema: s, Raw: raw})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ci, cj := categoryRank(out[i].Category), categoryRank(out[j].Category); ci != cj {
			return ci < cj
		}
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Title < out[j].Title
	})
	return out
}

// MobilePayloadByType finds a built-in mobile payload editor.
func MobilePayloadByType(t string) (MobilePayload, bool) {
	for _, p := range MobilePayloads() {
		if p.Type == t {
			return p, true
		}
	}
	return MobilePayload{}, false
}

// PayloadValues is one payload in a mobile profile: its type and the form values.
type PayloadValues struct {
	Type   string         `json:"type"`
	Values map[string]any `json:"values"`
}

// MobileProfileInput describes a configuration profile built from payload editors.
type MobileProfileInput struct {
	Identifier   string
	DisplayName  string
	Description  string
	Organization string
	Payloads     []PayloadValues
}

// PayloadErrors are validation errors for one payload (by index in the profile).
type PayloadErrors struct {
	Index  int          `json:"index"`
	Errors []FieldError `json:"errors"`
}

// BuildMobileProfile validates every payload against its editor and renders the
// configuration profile. UUIDs derive from the identifier and position so an
// unchanged profile renders identically.
func BuildMobileProfile(in MobileProfileInput) ([]byte, []PayloadErrors, error) {
	if len(in.Payloads) == 0 {
		return nil, nil, fmt.Errorf("add at least one payload")
	}
	var content []any
	var perrs []PayloadErrors
	seen := map[string]bool{}
	for i, pv := range in.Payloads {
		def, ok := MobilePayloadByType(pv.Type)
		if !ok {
			return nil, nil, fmt.Errorf("unknown payload type %q", pv.Type)
		}
		if seen[pv.Type] && !def.Multiple {
			return nil, nil, fmt.Errorf("a profile can hold only one %s payload", def.Title)
		}
		seen[pv.Type] = true
		vals, errs := def.Schema.Validate(pv.Values)
		if len(errs) > 0 {
			perrs = append(perrs, PayloadErrors{Index: i, Errors: errs})
			continue
		}
		vals = transformPayload(pv.Type, vals)
		id := fmt.Sprintf("%s.%s.%d", in.Identifier, strings.TrimPrefix(pv.Type, "com.apple."), i+1)
		p := map[string]any{}
		for k, v := range vals {
			p[k] = v
		}
		p["PayloadType"] = pv.Type
		p["PayloadVersion"] = 1
		p["PayloadIdentifier"] = id
		p["PayloadUUID"] = stableUUID(id)
		if _, ok := p["PayloadDisplayName"]; !ok {
			p["PayloadDisplayName"] = def.Title
		}
		content = append(content, p)
	}
	if len(perrs) > 0 {
		return nil, perrs, nil
	}
	top := map[string]any{
		"PayloadType": "Configuration", "PayloadVersion": 1, "PayloadIdentifier": in.Identifier,
		"PayloadUUID": stableUUID(in.Identifier), "PayloadDisplayName": in.DisplayName, "PayloadDescription": in.Description,
		"PayloadOrganization": in.Organization, "PayloadScope": "System", "PayloadRemovalDisallowed": true,
		"PayloadContent": content,
	}
	out, err := plist.MarshalIndent(top, plist.XMLFormat, "\t")
	return out, nil, err
}

// transformPayload reshapes form values where Apple's format can't be expressed
// in the form engine directly.
func transformPayload(t string, v map[string]any) map[string]any {
	switch t {
	case "com.apple.homescreenlayout":
		// The form edits Pages as [{"Items": [...]}, …]; Apple wants an array of arrays.
		if pages, ok := v["Pages"].([]any); ok {
			out := make([]any, 0, len(pages))
			for _, pg := range pages {
				m, _ := pg.(map[string]any)
				items, _ := m["Items"].([]any)
				if items == nil {
					items = []any{}
				}
				out = append(out, items)
			}
			v["Pages"] = out
		}
	}
	return v
}

var mobileCategories = []string{"General", "Security", "Network", "Apps and Home Screen", "Certificates"}

// categoryRank orders the payload picker; unknown categories go last.
func categoryRank(c string) int {
	for i, x := range mobileCategories {
		if x == c {
			return i
		}
	}
	return len(mobileCategories)
}
