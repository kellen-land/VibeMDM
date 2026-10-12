// Command profile-schemas generates VibeMDM's configuration profile payload editors
// (internal/schema/payloads/<platform>/<PayloadType>.json) from Apple's device-management
// repository, for the payloads Jamf Pro offers (catalog.go).
//
//	git clone --depth 1 https://github.com/apple/device-management
//	go run ./cmd/profile-schemas -src device-management
//
// Every key Apple documents for the platform is included, with its type, allowed values,
// range, default and description; keys Apple has removed on the platform are left out.
// Titles come from Apple where it has one, from curated/ (hand-written wording carried over
// from earlier VibeMDM versions) or are derived from the key name. Per-key notes record
// "Supervised", the OS version that introduced a key and deprecations.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

type yOS struct {
	Introduced      string `yaml:"introduced"`
	Deprecated      string `yaml:"deprecated"`
	Removed         string `yaml:"removed"`
	Multiple        *bool  `yaml:"multiple"`
	DeviceChannel   *bool  `yaml:"devicechannel"`
	UserChannel     *bool  `yaml:"userchannel"`
	Supervised      *bool  `yaml:"supervised"`
	RequiresDEP     *bool  `yaml:"requiresdep"`
	UserApprovedMDM *bool  `yaml:"userapprovedmdm"`
}

type yRange struct {
	Min *float64 `yaml:"min"`
	Max *float64 `yaml:"max"`
}

type yKey struct {
	Key         string         `yaml:"key"`
	Title       string         `yaml:"title"`
	SupportedOS map[string]yOS `yaml:"supportedOS"`
	Type        string         `yaml:"type"`
	Presence    string         `yaml:"presence"`
	Default     any            `yaml:"default"`
	Content     string         `yaml:"content"`
	RangeList   []any          `yaml:"rangelist"`
	Range       *yRange        `yaml:"range"`
	Format      string         `yaml:"format"`
	Repetition  *yRange        `yaml:"repetition"`
	SubKeys     []*yKey        `yaml:"subkeys"`
}

type yPayload struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Payload     struct {
		PayloadType string         `yaml:"payloadtype"`
		SupportedOS map[string]yOS `yaml:"supportedOS"`
	} `yaml:"payload"`
	PayloadKeys []*yKey `yaml:"payloadkeys"`
}

// node is the JSON schema written for one key (see internal/schema).
type node struct {
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Type        string           `json:"type,omitempty"`
	Format      string           `json:"format,omitempty"`
	Pattern     string           `json:"pattern,omitempty"`
	Enum        []any            `json:"enum,omitempty"`
	Default     any              `json:"default,omitempty"`
	Minimum     *float64         `json:"minimum,omitempty"`
	Maximum     *float64         `json:"maximum,omitempty"`
	MinItems    *int             `json:"minItems,omitempty"`
	MaxItems    *int             `json:"maxItems,omitempty"`
	Required    []string         `json:"required,omitempty"`
	Order       *int             `json:"property_order,omitempty"`
	Properties  map[string]*node `json:"properties,omitempty"`
	Items       *node            `json:"items,omitempty"`
	Additional  *node            `json:"additionalProperties,omitempty"`
	Options     map[string]any   `json:"options,omitempty"`
	Supervised  bool             `json:"x-supervised,omitempty"`
	OS          string           `json:"x-os,omitempty"`
	Deprecated  string           `json:"x-deprecated,omitempty"`
	Secret      bool             `json:"x-secret,omitempty"`
	RefTypes    []string         `json:"x-ref-types,omitempty"`
	Channel     string           `json:"x-channel,omitempty"`
}

type doc struct {
	Title       string           `json:"title"`
	Description string           `json:"description"`
	ID          string           `json:"__id"`
	PayloadType string           `json:"__payloadtype"`
	Platform    string           `json:"__platform"`
	Category    string           `json:"__category"`
	Order       int              `json:"__order"`
	Multiple    bool             `json:"__multiple"`
	Channels    []string         `json:"__channels,omitempty"`
	Supervised  bool             `json:"__supervised,omitempty"`
	Requires    []string         `json:"__requires,omitempty"`
	OS          string           `json:"__os,omitempty"`
	Deprecated  string           `json:"__deprecated,omitempty"`
	Jamf        string           `json:"__jamf,omitempty"`
	Options     map[string]any   `json:"options"`
	Type        string           `json:"type"`
	Required    []string         `json:"required,omitempty"`
	Properties  map[string]*node `json:"properties"`
}

// platform maps VibeMDM's platform names to Apple's supportedOS keys.
var osName = map[string]string{"ios": "iOS", "macos": "macOS"}
var osLabel = map[string]string{"ios": "iOS", "macos": "macOS"}

// baseline is the oldest OS version still worth mentioning: version notes ("iOS 17.0+")
// are only shown for keys and payloads introduced after it.
var baseline = map[string]string{"ios": "16.0", "macos": "13.0"}

// certTypes are the payloads a certificate reference may point to.
var certTypes = []string{"com.apple.security.pkcs1", "com.apple.security.root", "com.apple.security.pem", "com.apple.security.pkcs12",
	"com.apple.security.scep", "com.apple.security.acme", "com.apple.ADCertificate.managed"}

var certRefKey = regexp.MustCompile(`(?i)(Certificate|CertificateAnchor|Identity)UUID$`)

// Keys every payload carries, which the profile builder sets itself.
var builderKeys = map[string]bool{"PayloadType": true, "PayloadVersion": true, "PayloadIdentifier": true, "PayloadUUID": true,
	"PayloadOrganization": true, "PayloadDescription": true, "PayloadScope": true, "PayloadRemovalDisallowed": true,
	"PayloadEnabled": true}

var secretKey = regexp.MustCompile(`(?i)(password|secret|challenge|presharedkey|^pin$|passphrase)`)

func main() {
	src := flag.String("src", "", "path to a checkout of github.com/apple/device-management")
	out := flag.String("out", "internal/schema/payloads", "output directory")
	curatedDir := flag.String("curated", "cmd/profile-schemas/curated", "hand-written titles and descriptions")
	flag.Parse()
	if *src == "" {
		log.Fatal("-src is required")
	}
	// By file name: several files share a PayloadType (com.apple.MCX, com.apple.extensiblesso).
	payloads := map[string]*yPayload{}
	files, _ := filepath.Glob(filepath.Join(*src, "mdm", "profiles", "*.yaml"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			log.Fatal(err)
		}
		var p yPayload
		if err := decodeYAML(b, &p); err != nil {
			log.Fatalf("%s: %v", f, err)
		}
		if p.Payload.PayloadType != "" {
			payloads[strings.TrimSuffix(filepath.Base(f), ".yaml")] = &p
		}
	}
	if len(payloads) == 0 {
		log.Fatal("no payload definitions found under " + filepath.Join(*src, "mdm", "profiles"))
	}
	for _, plat := range []string{"ios", "macos"} {
		dir := filepath.Join(*out, plat)
		os.RemoveAll(dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal(err)
		}
		n, keys := 0, 0
		for _, e := range catalog {
			if !e.on(plat) {
				continue
			}
			p := payloads[e.file()]
			if p == nil {
				log.Fatalf("%s: not in Apple's repository", e.file())
			}
			// An editor built from another payload's keys keeps its own type and OS support.
			meta := payloads[e.ID]
			if meta == nil {
				meta = p
			}
			cur, titlesOnly := loadCurated(*curatedDir, plat, e.ID)
			d, err := generate(plat, e, p, meta, cur, titlesOnly)
			if err != nil {
				log.Fatalf("%s %s: %v", plat, e.ID, err)
			}
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			if err := enc.Encode(d); err != nil {
				log.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, e.ID+".json"), buf.Bytes(), 0o644); err != nil {
				log.Fatal(err)
			}
			n++
			keys += countKeys(d.Properties)
		}
		fmt.Printf("%s: %d payloads, %d settings (%d titles derived from key names)\n", plat, n, keys, derivedTitles)
		derivedTitles = 0
	}
}

// decodeYAML decodes a file whose anchors refer to themselves (recursive structures such
// as folders in folders): aliases are expanded to a fixed depth first.
func decodeYAML(b []byte, out any) error {
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		return err
	}
	flat, err := yaml.Marshal(expand(&root, 0))
	if err != nil {
		return err
	}
	return yaml.Unmarshal(flat, out)
}

func expand(n *yaml.Node, aliases int) any {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil
		}
		return expand(n.Content[0], aliases)
	case yaml.AliasNode:
		if aliases > 3 {
			return nil
		}
		return expand(n.Alias, aliases+1)
	case yaml.MappingNode:
		m := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			m[n.Content[i].Value] = expand(n.Content[i+1], aliases)
		}
		return m
	case yaml.SequenceNode:
		l := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			if v := expand(c, aliases); v != nil {
				l = append(l, v)
			}
		}
		return l
	}
	var v any
	n.Decode(&v)
	return v
}

var derivedTitles int

func countKeys(p map[string]*node) int {
	c := 0
	for _, n := range p {
		c++
		c += countKeys(n.Properties)
		if n.Items != nil {
			c += countKeys(n.Items.Properties)
		}
	}
	return c
}

// loadCurated reads a hand-written schema whose wording wins over the generated one. The iOS
// wording is reused for the same payload on macOS (keys a platform lacks are ignored).
func loadCurated(dir, plat, typ string) (cur *node, titlesOnly bool) {
	b, err := os.ReadFile(filepath.Join(dir, plat, typ+".json"))
	if err != nil {
		if b, err = os.ReadFile(filepath.Join(dir, "ios", typ+".json")); err != nil {
			return nil, false
		}
		titlesOnly = true // the iOS descriptions don't fit macOS
	}
	var n node
	if err := json.Unmarshal(b, &n); err != nil {
		log.Fatalf("curated %s: %v", typ, err)
	}
	return &n, titlesOnly
}

func generate(plat string, e entry, p, meta *yPayload, cur *node, titlesOnly bool) (*doc, error) {
	ps := meta.Payload.SupportedOS[osName[plat]]
	d := &doc{
		Title: e.Title, Description: firstNonEmpty(e.Description, cleanText(p.Description, 400)), ID: e.ID, PayloadType: meta.Payload.PayloadType, Platform: plat,
		Category: e.Category, Order: e.Order, Options: map[string]any{"remove_empty_properties": true}, Type: "object", Jamf: e.Jamf,
	}
	if cur != nil && !titlesOnly && e.Description == "" && cur.Description != "" {
		d.Description = cur.Description
	}
	d.Multiple = ps.Multiple != nil && *ps.Multiple
	if e.Multiple != nil {
		d.Multiple = *e.Multiple
	}
	if plat == "macos" {
		if ps.DeviceChannel == nil || *ps.DeviceChannel {
			d.Channels = append(d.Channels, "device")
		}
		if ps.UserChannel != nil && *ps.UserChannel {
			d.Channels = append(d.Channels, "user")
		}
		if len(d.Channels) == 0 {
			d.Channels = []string{"device"}
		}
	}
	d.Supervised = ps.Supervised != nil && *ps.Supervised
	if ps.RequiresDEP != nil && *ps.RequiresDEP {
		d.Requires = append(d.Requires, "Automated Device Enrollment")
	}
	if ps.UserApprovedMDM != nil && *ps.UserApprovedMDM {
		d.Requires = append(d.Requires, "User-approved MDM")
	}
	if ps.Introduced != "" && ps.Introduced != "n/a" && newer(ps.Introduced, baseline[plat]) {
		d.OS = osLabel[plat] + " " + ps.Introduced + "+"
	}
	d.Deprecated = ps.Deprecated
	if e.Curated && cur != nil && !titlesOnly {
		// The hand-written editor is used as is (its shape differs from Apple's on purpose).
		d.Required, d.Properties = cur.Required, cur.Properties
		return d, nil
	}
	g := gen{plat: plat, payload: ps, skip: e.Skip, id: e.ID}
	props, req := g.object(p.PayloadKeys, "", 0, true)
	if len(props) == 0 {
		return nil, fmt.Errorf("no settings for %s", plat)
	}
	if e.Interface && plat == "macos" {
		// One Ethernet editor for Apple's per-interface payload types (the builder picks the type).
		o := 0
		props["__interface"] = &node{Title: "Interface", Description: "Which Ethernet interface the 802.1X settings apply to.", Type: "string",
			Enum: []any{"any", "first-active", "second-active", "third-active", "first", "second", "third"}, Default: "any", Order: &o,
			Options: map[string]any{"enum_titles": []string{"Any Ethernet", "First active Ethernet", "Second active Ethernet", "Third active Ethernet",
				"First Ethernet", "Second Ethernet", "Third Ethernet"}}}
		req = append(req, "__interface")
	}
	d.Properties, d.Required = props, req
	if cur != nil {
		applyCurated(d.Properties, cur.Properties, titlesOnly)
		if len(cur.Required) > 0 && !titlesOnly {
			d.Required = intersect(cur.Required, d.Properties)
		}
	}
	return d, nil
}

func intersect(keys []string, props map[string]*node) []string {
	var out []string
	for _, k := range keys {
		if _, ok := props[k]; ok {
			out = append(out, k)
		}
	}
	return out
}

// applyCurated copies wording, order and value labels from the hand-written schema.
func applyCurated(gen, cur map[string]*node, titlesOnly bool) {
	for k, c := range cur {
		g, ok := gen[k]
		if !ok || c == nil {
			continue
		}
		if c.Title != "" {
			g.Title = c.Title
		}
		if c.Description != "" && !titlesOnly {
			g.Description = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(c.Description), "Supervised only."))
			g.Description = strings.ReplaceAll(g.Description, " Supervised only.", "")
		}
		if c.Order != nil && !titlesOnly {
			g.Order = c.Order
		}
		if c.Options != nil {
			if g.Options == nil {
				g.Options = map[string]any{}
			}
			for ok, ov := range c.Options {
				if ok == "enum_titles" && !sameEnum(c.Enum, g.Enum) {
					continue
				}
				g.Options[ok] = ov
			}
		}
		if c.Properties != nil && g.Properties != nil {
			applyCurated(g.Properties, c.Properties, titlesOnly)
		}
		if c.Items != nil && g.Items != nil && c.Items.Properties != nil && g.Items.Properties != nil {
			applyCurated(g.Items.Properties, c.Items.Properties, titlesOnly)
			if c.Items.Title != "" {
				g.Items.Title = c.Items.Title
			}
		}
	}
}

func sameEnum(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if fmt.Sprint(a[i]) != fmt.Sprint(b[i]) {
			return false
		}
	}
	return true
}

type gen struct {
	id      string
	plat    string
	payload yOS
	skip    []string
}

func (g gen) skipped(path string) bool {
	for _, s := range g.skip {
		if s == path {
			return true
		}
	}
	return false
}

// supported reports whether a key exists on the platform, with its notes.
func (g gen) support(k *yKey) (ok bool, os yOS) {
	if k.SupportedOS == nil {
		return true, yOS{}
	}
	s, has := k.SupportedOS[osName[g.plat]]
	if !has {
		return true, yOS{}
	}
	if s.Introduced == "n/a" || s.Removed != "" {
		return false, s
	}
	return true, s
}

func (g gen) object(keys []*yKey, path string, depth int, top bool) (map[string]*node, []string) {
	props := map[string]*node{}
	var req []string
	for i, k := range keys {
		if k == nil || k.Key == "" || k.Key == "ANY" {
			continue
		}
		if top && builderKeys[k.Key] {
			continue
		}
		p := joinPath(path, k.Key)
		if g.skipped(p) {
			continue
		}
		ok, s := g.support(k)
		if !ok {
			continue
		}
		n := g.key(k, p, depth)
		if n == nil {
			continue
		}
		if t, ok := titles[g.id+"/"+p]; ok {
			n.Title = t
		}
		order := 1000 + i
		n.Order = &order
		g.notes(n, s)
		props[k.Key] = n
		if k.Presence == "required" && k.Default == nil {
			req = append(req, k.Key)
		}
	}
	return props, req
}

func (g gen) notes(n *node, s yOS) {
	if s.Supervised != nil && *s.Supervised && !(g.payload.Supervised != nil && *g.payload.Supervised) {
		n.Supervised = true
	}
	if s.Introduced != "" && s.Introduced != "n/a" && newer(s.Introduced, g.payload.Introduced) && newer(s.Introduced, baseline[g.plat]) {
		n.OS = osLabel[g.plat] + " " + s.Introduced + "+"
	}
	if s.Deprecated != "" {
		n.Deprecated = osLabel[g.plat] + " " + s.Deprecated
	}
	if g.plat == "macos" {
		dc, uc := s.DeviceChannel, s.UserChannel
		switch {
		case dc != nil && !*dc && uc != nil && *uc:
			n.Channel = "user"
		case uc != nil && !*uc && dc != nil && *dc:
			n.Channel = "device"
		}
	}
}

// newer reports whether version a is later than b ("16.4" > "16.0").
func newer(a, b string) bool {
	if b == "" || b == "n/a" {
		return false
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func (g gen) key(k *yKey, path string, depth int) *node {
	if depth > 8 {
		return nil
	}
	n := &node{Title: firstNonEmpty(cleanTitle(k.Title, k.Key), humanize(k.Key)), Description: cleanText(k.Content, 600)}
	if k.Title == "" {
		derivedTitles++
	}
	switch k.Type {
	case "<string>":
		n.Type = "string"
		if strings.HasPrefix(k.Format, "^") {
			n.Pattern = k.Format
		}
		if certRefKey.MatchString(k.Key) {
			n.Format, n.Pattern, n.RefTypes = "payload-ref", "", certTypes
		}
		if secretKey.MatchString(k.Key) {
			n.Secret = true
		}
	case "<integer>":
		n.Type = "integer"
	case "<real>":
		n.Type = "number"
	case "<boolean>":
		n.Type = "boolean"
	case "<date>":
		n.Type, n.Format = "string", "date-time"
	case "<data>":
		n.Type, n.Format = "string", "data"
		if secretKey.MatchString(k.Key) {
			n.Secret = true
		}
	case "<array>":
		n.Type = "array"
		if len(k.SubKeys) > 0 && k.SubKeys[0] != nil {
			n.Items = g.key(k.SubKeys[0], path+"[]", depth+1)
			if n.Items == nil {
				return nil
			}
			n.Items.Description = ""
			lt := strings.ToLower(n.Items.Title)
			if lt == "" || strings.HasSuffix(lt, "item") || strings.HasSuffix(lt, "element") || strings.HasSuffix(lt, "dict") || strings.HasSuffix(lt, "dictionary") {
				n.Items.Title = singular(n.Title)
			}
			if g.id == "com.apple.TCC.configuration-profile-policy" && strings.HasPrefix(path, "Services.") {
				n.Items.Title = "App"
			}
			if certRefKey.MatchString(k.Key) || strings.HasSuffix(k.Key, "AnchorUUID") {
				n.Items.Format, n.Items.Pattern, n.Items.RefTypes = "payload-ref", "", certTypes
			}
		} else {
			n.Items = &node{Type: "string"}
		}
		if k.Repetition != nil {
			if k.Repetition.Min != nil && *k.Repetition.Min > 0 {
				m := int(*k.Repetition.Min)
				n.MinItems = &m
			}
			if k.Repetition.Max != nil {
				m := int(*k.Repetition.Max)
				n.MaxItems = &m
			}
		}
	case "<dictionary>":
		n.Type = "object"
		props, req := g.object(k.SubKeys, path, depth+1, false)
		if len(props) > 0 {
			n.Properties, n.Required = props, req
		}
		for _, sk := range k.SubKeys {
			if sk != nil && sk.Key == "ANY" {
				if a := g.key(sk, path+".*", depth+1); a != nil {
					a.Order = nil
					n.Additional = a
				}
			}
		}
	default: // <any>: not editable as a form field
		return nil
	}
	if len(k.RangeList) > 0 && n.Type != "array" && n.Type != "object" && n.Type != "boolean" {
		n.Enum = normalizeEnum(k.RangeList, n.Type)
		if titles := enumTitles(n.Enum, k.Content); titles != nil {
			n.Options = map[string]any{"enum_titles": titles}
		}
	}
	if len(k.RangeList) > 0 && n.Type == "array" && n.Items != nil && len(n.Items.Enum) == 0 {
		n.Items.Enum = normalizeEnum(k.RangeList, n.Items.Type)
	}
	if n.Type == "array" && n.Items != nil && len(n.Items.Enum) > 0 && n.Items.Options["enum_titles"] == nil {
		// Apple often lists the values in the array's description rather than the item's.
		if titles := enumTitles(n.Items.Enum, k.Content); titles != nil {
			if n.Items.Options == nil {
				n.Items.Options = map[string]any{}
			}
			n.Items.Options["enum_titles"] = titles
		}
	}
	if k.Range != nil && (n.Type == "integer" || n.Type == "number") {
		n.Minimum, n.Maximum = k.Range.Min, k.Range.Max
	}
	if k.Default != nil && n.Type != "object" && n.Type != "array" && n.Format != "data" {
		n.Default = normalizeScalar(k.Default, n.Type)
	}
	return n
}

func normalizeEnum(vals []any, typ string) []any {
	out := make([]any, 0, len(vals))
	for _, v := range vals {
		out = append(out, normalizeScalar(v, typ))
	}
	return out
}

func normalizeScalar(v any, typ string) any {
	switch typ {
	case "string":
		return fmt.Sprint(v)
	case "integer":
		switch t := v.(type) {
		case int:
			return t
		case string:
			if i, err := strconv.Atoi(t); err == nil {
				return i
			}
		}
	case "number":
		switch t := v.(type) {
		case int:
			return float64(t)
		case string:
			if f, err := strconv.ParseFloat(t, 64); err == nil {
				return f
			}
		}
	case "boolean":
		if s, ok := v.(string); ok {
			return s == "true"
		}
	}
	return v
}

// enumTitleLine matches "- `value`: Title …" lines in Apple's descriptions.
var enumTitleLine = regexp.MustCompile("(?m)^\\s*[-*]\\s*`?([^`:\\s]+)`?\\s*[:–=-]\\s*(.+)$")

func enumTitles(enum []any, content string) []string {
	found := map[string]string{}
	for _, m := range enumTitleLine.FindAllStringSubmatch(content, -1) {
		t := strings.TrimSpace(strings.ReplaceAll(m[2], "`", ""))
		if i := strings.IndexAny(t, ".:"); i > 0 {
			t = strings.TrimSpace(t[:i])
		}
		// Only short labels ("Temporary Banner"); sentences stay in the description.
		if w := strings.Fields(t); len(w) > 5 || strings.ContainsAny(t, "(;,") {
			continue
		}
		found[m[1]] = t
	}
	out := make([]string, len(enum))
	any := false
	for i, v := range enum {
		s := fmt.Sprint(v)
		if t, ok := found[s]; ok && t != "" {
			out[i] = t
			any = true
		} else if _, isStr := v.(string); isStr {
			out[i] = humanizeValue(s)
			any = true
		} else {
			out[i] = s
		}
	}
	if !any {
		return nil
	}
	return out
}

var (
	mdLink      = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	notAvail    = regexp.MustCompile(`(?m)^\s*(Not available|Available) in [^\n]*$`)
	blankLines  = regexp.MustCompile(`\n{3,}`)
	sentenceEnd = regexp.MustCompile(`[.!?](\s|$)`)
)

// cleanText turns Apple's Markdown description into plain text of at most max characters.
func cleanText(s string, max int) string {
	s = mdLink.ReplaceAllString(s, "$1")
	s = notAvail.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "**", "")
	s = blankLines.ReplaceAllString(strings.TrimSpace(s), "\n\n")
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if locs := sentenceEnd.FindAllStringIndex(cut, -1); len(locs) > 0 {
		return strings.TrimSpace(cut[:locs[len(locs)-1][0]+1])
	}
	return strings.TrimSpace(cut) + "…"
}

func cleanTitle(t, key string) string {
	t = strings.TrimSpace(t)
	if t == "" || t == key || !strings.Contains(t, " ") && t != strings.ToUpper(t[:1])+strings.ToLower(t[1:]) {
		return "" // Apple's "title" is just the key: derive one instead
	}
	return fixWords(t)
}

// words keeps Apple's spelling of product names and acronyms in derived titles.
var words = map[string]string{
	"airdrop": "AirDrop", "airplay": "AirPlay", "airprint": "AirPrint", "icloud": "iCloud", "itunes": "iTunes", "imessage": "iMessage",
	"facetime": "FaceTime", "siri": "Siri", "safari": "Safari", "bluetooth": "Bluetooth", "wifi": "Wi-Fi", "wi-fi": "Wi-Fi",
	"vpn": "VPN", "url": "URL", "urls": "URLs", "id": "ID", "ids": "IDs", "ssid": "SSID", "dns": "DNS", "pin": "PIN", "usb": "USB",
	"nfc": "NFC", "ip": "IP", "http": "HTTP", "https": "HTTPS", "tls": "TLS", "eap": "EAP", "scep": "SCEP", "mdm": "MDM",
	"uuid": "UUID", "esim": "eSIM", "ios": "iOS", "macos": "macOS", "ipados": "iPadOS", "tvos": "tvOS", "app": "app",
	"filevault": "FileVault", "keychain": "Keychain", "spotlight": "Spotlight", "finder": "Finder", "dock": "Dock",
	"homepod": "HomePod", "carplay": "CarPlay", "ipad": "iPad", "iphone": "iPhone", "mac": "Mac", "apple": "Apple",
	"kerberos": "Kerberos", "smb": "SMB", "afp": "AFP", "nfs": "NFS", "ldap": "LDAP", "sso": "SSO", "pac": "PAC",
	"ocsp": "OCSP", "crl": "CRL", "ntp": "NTP", "fqdn": "FQDN", "ikev2": "IKEv2", "l2tp": "L2TP", "ipsec": "IPSec",
	"pptp": "PPTP", "smime": "S/MIME", "s/mime": "S/MIME", "imap": "IMAP", "pop": "POP", "smtp": "SMTP", "ews": "EWS",
	"oauth": "OAuth", "api": "API", "ui": "UI", "os": "OS", "gps": "GPS", "hdmi": "HDMI", "nat": "NAT", "mtu": "MTU",
	"ttl": "TTL", "acme": "ACME", "der": "DER", "pem": "PEM", "rsa": "RSA", "ecc": "ECC", "ca": "CA", "dhcp": "DHCP",
	"cfprefs": "cfprefs", "touchid": "Touch ID", "faceid": "Face ID", "opticid": "Optic ID", "applecare": "AppleCare",
	"appleid": "Apple ID", "gamecenter": "Game Center", "appstore": "App Store", "screentime": "Screen Time",
	"icloudkeychain": "iCloud Keychain", "dfu": "DFU", "tcc": "TCC", "kext": "kext", "kexts": "kexts", "pppc": "PPPC",
}

// humanize derives a sentence-case title from a key ("allowCloudBackup" → "Allow cloud backup").
func humanize(key string) string {
	key = strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(key)
	var parts []string
	for _, f := range strings.Fields(key) {
		parts = append(parts, camelWords(f)...)
	}
	for i, p := range parts {
		lp := strings.ToLower(p)
		switch {
		case words[lp] != "":
			parts[i] = words[lp]
		case isUpper(p) && len(p) > 1:
			// acronym: keep
		case i == 0:
			parts[i] = strings.ToUpper(lp[:1]) + lp[1:]
		default:
			parts[i] = lp
		}
	}
	return fixWords(strings.Join(parts, " "))
}

// fixWords repairs product names split by camel-case splitting.
func fixWords(s string) string {
	r := strings.NewReplacer("Air Drop", "AirDrop", "air drop", "AirDrop", "Air Play", "AirPlay", "air play", "AirPlay",
		"Air Print", "AirPrint", "air print", "AirPrint", "Face Time", "FaceTime", "face time", "FaceTime",
		"I Cloud", "iCloud", "i Cloud", "iCloud", "i cloud", "iCloud", "Wi Fi", "Wi-Fi", "wi fi", "Wi-Fi", "Wi-fi", "Wi-Fi",
		"File Vault", "FileVault", "file vault", "FileVault", "Car Play", "CarPlay", "car play", "CarPlay",
		"Touch ID", "Touch ID", "touch ID", "Touch ID", "Face ID", "Face ID", "face ID", "Face ID", "game center", "Game Center",
		"app store", "App Store", "App store", "App Store", "screen time", "Screen Time", "home pod", "HomePod")
	return r.Replace(s)
}

func humanizeValue(v string) string {
	if v == "" {
		return v
	}
	h := humanize(v)
	if h == "" {
		return v
	}
	return h
}

// camelWords splits "AEReceiverIdentifier" into "AE", "Receiver", "Identifier" and
// "allowVPNCreation" into "allow", "VPN", "Creation".
func camelWords(s string) []string {
	r := []rune(s)
	var out []string
	start := 0
	for i := 1; i < len(r); i++ {
		prev, cur := r[i-1], r[i]
		next := rune(0)
		if i+1 < len(r) {
			next = r[i+1]
		}
		split := (unicode.IsLower(prev) || unicode.IsDigit(prev)) && unicode.IsUpper(cur) ||
			unicode.IsUpper(prev) && unicode.IsUpper(cur) && unicode.IsLower(next)
		if split {
			out = append(out, string(r[start:i]))
			start = i
		}
	}
	return append(out, string(r[start:]))
}

func isUpper(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

func singular(t string) string {
	t = strings.TrimSpace(t)
	switch {
	case strings.HasSuffix(t, "ies"):
		return t[:len(t)-3] + "y"
	case strings.HasSuffix(t, "sses"), strings.HasSuffix(t, "ches"), strings.HasSuffix(t, "shes"), strings.HasSuffix(t, "xes"):
		return t[:len(t)-2]
	case strings.HasSuffix(t, "s") && !strings.HasSuffix(t, "ss"):
		return t[:len(t)-1]
	}
	return "Item"
}

func joinPath(p, k string) string {
	if p == "" {
		return k
	}
	return p + "." + k
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
