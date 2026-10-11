package schema

import (
	"embed"
	"path"
	"sort"
	"strings"
)

//go:embed builtin/*.json mobile/*.json
var builtinFS embed.FS

// Builtin is a schema VibeMDM ships (written for VibeMDM from each project's
// public documentation; not copied from Jamf's or anyone's schema library).
type Builtin struct {
	ID     string  `json:"id"` // preference domain
	Schema *Schema `json:"-"`
	Raw    []byte  `json:"-"`
}

// Builtins returns the bundled schemas, sorted by domain.
func Builtins() []Builtin {
	entries, _ := builtinFS.ReadDir("builtin")
	var out []Builtin
	for _, e := range entries {
		raw, err := builtinFS.ReadFile(path.Join("builtin", e.Name()))
		if err != nil {
			continue
		}
		s, err := Parse(raw)
		if err != nil {
			panic("invalid built-in schema " + e.Name() + ": " + err.Error())
		}
		out = append(out, Builtin{ID: strings.TrimSuffix(e.Name(), ".json"), Schema: s, Raw: raw})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// BuiltinByDomain finds a bundled schema.
func BuiltinByDomain(domain string) (Builtin, bool) {
	for _, b := range Builtins() {
		if b.ID == domain {
			return b, true
		}
	}
	return Builtin{}, false
}
