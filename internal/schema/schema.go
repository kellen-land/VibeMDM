// Package schema turns Jamf-style "external application" JSON schemas (a
// json-editor subset of JSON Schema) into validated managed preferences and
// configuration profiles.
//
// Supported keywords: title, description, type (string, integer, number,
// boolean, array, object), properties, items, enum, default, minimum, maximum,
// minLength, maxLength, pattern, required, property_order/propertyOrder, links,
// anyOf/oneOf (including the "Not Configured" null idiom), and options
// (enum_titles, infoText, inputAttributes.placeholder, remove_empty_properties).
// VibeMDM extension: "format": "data" marks a base64 string emitted as <data>.
// Header fields __preferencedomain and __version are read when present.
package schema

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Node is one schema node.
type Node struct {
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Type        any              `json:"type,omitempty"` // string or []string
	Properties  map[string]*Node `json:"properties,omitempty"`
	Items       *Node            `json:"items,omitempty"`
	Enum        []any            `json:"enum,omitempty"`
	Default     any              `json:"default,omitempty"`
	Minimum     *float64         `json:"minimum,omitempty"`
	Maximum     *float64         `json:"maximum,omitempty"`
	MinLength   *int             `json:"minLength,omitempty"`
	MaxLength   *int             `json:"maxLength,omitempty"`
	MinItems    *int             `json:"minItems,omitempty"`
	MaxItems    *int             `json:"maxItems,omitempty"`
	Pattern     string           `json:"pattern,omitempty"`
	// Format "data" (VibeMDM extension): a base64 string emitted as plist <data>.
	Format        string   `json:"format,omitempty"`
	Required      []string `json:"required,omitempty"`
	PropertyOrder *int     `json:"property_order,omitempty"`
	PropertyOrd2  *int     `json:"propertyOrder,omitempty"`
	AnyOf         []*Node  `json:"anyOf,omitempty"`
	OneOf         []*Node  `json:"oneOf,omitempty"`
	Links         []Link   `json:"links,omitempty"`
	Options       *Options `json:"options,omitempty"`
	// AdditionalProperties (an object schema) types the values of keys an object does not
	// list, as in Apple's dictionaries keyed by team ID or bundle ID. Booleans are ignored.
	RawAdditional json.RawMessage `json:"additionalProperties,omitempty"`

	// VibeMDM extensions (payload editors generated from Apple's device-management repository).
	Supervised bool     `json:"x-supervised,omitempty"` // only on supervised devices
	OS         string   `json:"x-os,omitempty"`         // e.g. "iOS 17.0+" when newer than the payload
	Deprecated string   `json:"x-deprecated,omitempty"` // e.g. "macOS 13.0"
	Secret     bool     `json:"x-secret,omitempty"`     // hidden from viewers
	RefTypes   []string `json:"x-ref-types,omitempty"`  // format "payload-ref": allowed payload types
	Channel    string   `json:"x-channel,omitempty"`    // macOS: "device" or "user" when only one applies

	re         *regexp.Regexp
	additional *Node
}

// Link is a documentation link shown next to a field.
type Link struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}

// Options are json-editor display hints.
type Options struct {
	EnumTitles            []string       `json:"enum_titles,omitempty"`
	InfoText              string         `json:"infoText,omitempty"`
	InputAttributes       map[string]any `json:"inputAttributes,omitempty"`
	RemoveEmptyProperties bool           `json:"remove_empty_properties,omitempty"`
}

// Schema is a parsed top-level schema.
type Schema struct {
	Node
	PreferenceDomain string `json:"__preferencedomain,omitempty"`
	Version          string `json:"__version,omitempty"`
}

const maxSchemaBytes = 2 << 20

// Parse reads and checks a schema.
func Parse(raw []byte) (*Schema, error) {
	if len(raw) > maxSchemaBytes {
		return nil, errors.New("schema is larger than 2 MB")
	}
	var s Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("schema is not valid JSON: %w", err)
	}
	if len(s.Properties) == 0 {
		return nil, errors.New(`schema has no "properties"`)
	}
	if err := prepare(&s.Node, "", 0); err != nil {
		return nil, err
	}
	return &s, nil
}

func prepare(n *Node, path string, depth int) error {
	if depth > 32 {
		return fmt.Errorf("%s: schema nests too deeply", path)
	}
	if n.Pattern != "" {
		re, err := regexp.Compile(n.Pattern)
		if err != nil {
			return fmt.Errorf("%s: invalid pattern: %w", label(path), err)
		}
		n.re = re
	}
	if len(n.RawAdditional) > 0 && n.RawAdditional[0] == '{' {
		var a Node
		if err := json.Unmarshal(n.RawAdditional, &a); err != nil {
			return fmt.Errorf("%s: invalid additionalProperties: %w", label(path), err)
		}
		if err := prepare(&a, join(path, "*"), depth+1); err != nil {
			return err
		}
		n.additional = &a
	}
	for k, c := range n.Properties {
		if c == nil {
			return fmt.Errorf("%s: property %q is empty", label(path), k)
		}
		if err := prepare(c, join(path, k), depth+1); err != nil {
			return err
		}
	}
	if n.Items != nil {
		if err := prepare(n.Items, path+"[]", depth+1); err != nil {
			return err
		}
	}
	for _, alt := range append(append([]*Node{}, n.AnyOf...), n.OneOf...) {
		if alt != nil {
			if err := prepare(alt, path, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func join(path, k string) string {
	if path == "" {
		return k
	}
	return path + "." + k
}

func label(path string) string {
	if path == "" {
		return "schema"
	}
	return path
}

// Additional returns the schema for keys the object does not list (nil when none).
func (n *Node) Additional() *Node { return n.additional }

// Types returns the node's declared types.
func (n *Node) Types() []string {
	switch t := n.Type.(type) {
	case string:
		return []string{t}
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	if len(n.Properties) > 0 {
		return []string{"object"}
	}
	return nil
}

// Order returns the display order hint (large when unset).
func (n *Node) Order() int {
	if n.PropertyOrder != nil {
		return *n.PropertyOrder
	}
	if n.PropertyOrd2 != nil {
		return *n.PropertyOrd2
	}
	return math.MaxInt32
}

// OrderedKeys returns property keys in display order.
func (n *Node) OrderedKeys() []string {
	keys := make([]string, 0, len(n.Properties))
	for k := range n.Properties {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		a, b := n.Properties[keys[i]].Order(), n.Properties[keys[j]].Order()
		if a != b {
			return a < b
		}
		return keys[i] < keys[j]
	})
	return keys
}

// FieldError is a validation problem at a path.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e FieldError) Error() string { return e.Path + ": " + e.Message }

// Validate checks values against the schema and returns them coerced to
// plist-friendly Go types (int64, float64, bool, string, []any, map[string]any).
// Unknown keys are rejected; keys with nil/empty values are dropped.
func (s *Schema) Validate(values map[string]any) (map[string]any, []FieldError) {
	var errs []FieldError
	out, _ := validateObject(&s.Node, values, "", &errs)
	if out == nil {
		out = map[string]any{}
	}
	return out, errs
}

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

func validateObject(n *Node, values map[string]any, path string, errs *[]FieldError) (map[string]any, bool) {
	out := map[string]any{}
	for k, v := range values {
		child, ok := n.Properties[k]
		if !ok && n.additional != nil && strings.TrimSpace(k) != "" {
			child, ok = n.additional, true
		}
		if !ok {
			*errs = append(*errs, FieldError{join(path, k), "is not defined by the schema"})
			continue
		}
		if isEmpty(v) {
			continue
		}
		if cv, ok := validateValue(child, v, join(path, k), errs); ok {
			out[k] = cv
		}
	}
	for _, req := range n.Required {
		if _, ok := out[req]; !ok {
			*errs = append(*errs, FieldError{join(path, req), "is required"})
		}
	}
	return out, true
}

// branch picks the anyOf/oneOf alternative that accepts v (ignoring null alternatives).
func branch(n *Node, v any, path string) (*Node, bool) {
	alts := append(append([]*Node{}, n.AnyOf...), n.OneOf...)
	if len(alts) == 0 {
		return n, true
	}
	for _, alt := range alts {
		if alt == nil {
			continue
		}
		ts := alt.Types()
		if len(ts) == 1 && ts[0] == "null" {
			continue
		}
		var probe []FieldError
		if _, ok := validateValue(alt, v, path, &probe); ok && len(probe) == 0 {
			return alt, true
		}
	}
	return nil, false
}

func validateValue(n *Node, v any, path string, errs *[]FieldError) (any, bool) {
	if len(n.AnyOf)+len(n.OneOf) > 0 {
		alt, ok := branch(n, v, path)
		if !ok {
			*errs = append(*errs, FieldError{path, "does not match any allowed form"})
			return nil, false
		}
		if alt != n {
			return validateValue(alt, v, path, errs)
		}
	}
	types := n.Types()
	if len(types) == 0 {
		types = []string{inferType(v)}
	}
	var lastErr string
	for _, t := range types {
		cv, msg := coerce(n, t, v, path, errs)
		if msg == "" {
			if len(n.Enum) > 0 && !inEnum(n.Enum, cv) {
				*errs = append(*errs, FieldError{path, "is not one of the allowed values"})
				return nil, false
			}
			return cv, true
		}
		lastErr = msg
	}
	*errs = append(*errs, FieldError{path, lastErr})
	return nil, false
}

func inferType(v any) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case float64, int, int64, json.Number:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "string"
}

func inEnum(enum []any, v any) bool {
	for _, e := range enum {
		if fmt.Sprint(normalizeNum(e)) == fmt.Sprint(normalizeNum(v)) {
			return true
		}
	}
	return false
}

func normalizeNum(v any) any {
	switch t := v.(type) {
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1e15 {
			return int64(t)
		}
	case int:
		return int64(t)
	}
	return v
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	}
	return 0, false
}

// coerce converts v to schema type t; it returns an error message on mismatch.
func coerce(n *Node, t string, v any, path string, errs *[]FieldError) (any, string) {
	switch t {
	case "string":
		s, ok := v.(string)
		if !ok {
			return nil, "must be text"
		}
		if n.MinLength != nil && len([]rune(s)) < *n.MinLength {
			return nil, fmt.Sprintf("must be at least %d characters", *n.MinLength)
		}
		if n.MaxLength != nil && len([]rune(s)) > *n.MaxLength {
			return nil, fmt.Sprintf("must be at most %d characters", *n.MaxLength)
		}
		if n.re != nil && !n.re.MatchString(s) {
			return nil, "does not match the required format"
		}
		if n.Format == "date-time" {
			for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
				if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
					return t.UTC(), ""
				}
			}
			return nil, "must be a date and time"
		}
		if n.Format == "data" {
			b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
			if err != nil {
				return nil, "must be base64-encoded data"
			}
			return b, ""
		}
		return s, ""
	case "integer", "number":
		f, ok := toFloat(v)
		if !ok {
			return nil, "must be a number"
		}
		if t == "integer" && f != math.Trunc(f) {
			return nil, "must be a whole number"
		}
		if n.Minimum != nil && f < *n.Minimum {
			return nil, fmt.Sprintf("must be at least %v", *n.Minimum)
		}
		if n.Maximum != nil && f > *n.Maximum {
			return nil, fmt.Sprintf("must be at most %v", *n.Maximum)
		}
		if t == "integer" {
			return int64(f), ""
		}
		return f, ""
	case "boolean":
		b, ok := v.(bool)
		if !ok {
			return nil, "must be true or false"
		}
		return b, ""
	case "array":
		arr, ok := v.([]any)
		if !ok {
			return nil, "must be a list"
		}
		if n.MinItems != nil && len(arr) < *n.MinItems {
			return nil, fmt.Sprintf("needs at least %d items", *n.MinItems)
		}
		if n.MaxItems != nil && len(arr) > *n.MaxItems {
			return nil, fmt.Sprintf("allows at most %d items", *n.MaxItems)
		}
		out := make([]any, 0, len(arr))
		for i, item := range arr {
			if n.Items == nil {
				out = append(out, item)
				continue
			}
			if cv, ok := validateValue(n.Items, item, fmt.Sprintf("%s[%d]", path, i), errs); ok {
				out = append(out, cv)
			}
		}
		return out, ""
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			return nil, "must be a group of settings"
		}
		if len(n.Properties) == 0 && n.additional == nil {
			return m, "" // free-form dictionary
		}
		out, _ := validateObject(n, m, path, errs)
		return out, ""
	case "null":
		if v == nil {
			return nil, ""
		}
		return nil, "must be empty"
	}
	return nil, "has an unsupported type " + t
}

// Defaults returns the schema's top-level defaults (useful to prefill a form).
func (s *Schema) Defaults() map[string]any {
	out := map[string]any{}
	for k, n := range s.Properties {
		if n.Default != nil {
			out[k] = n.Default
		}
	}
	return out
}

// DomainPattern validates a preference domain (reverse-DNS style).
var DomainPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*(\.[A-Za-z0-9_-]+)+$`)

// CheckDomain validates a preference domain.
func CheckDomain(d string) error {
	if !DomainPattern.MatchString(d) || strings.HasPrefix(d, "com.apple.ManagedClient") {
		return fmt.Errorf("%q is not a valid preference domain (e.g. com.example.app)", d)
	}
	return nil
}
