// Package selfservice holds rules shared by the server and the agent for the Self Service app.
package selfservice

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
)

// DefaultAccent is the accent colour when none is set.
const DefaultAccent = "#0FB5AE"

var accentRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// ValidAccent reports whether c is a #RRGGBB colour.
func ValidAccent(c string) bool { return accentRe.MatchString(c) }

// CheckAppName validates the Self Service app's name, which becomes /Applications/<name>.app.
func CheckAppName(name string) error {
	if name == "" {
		return nil // the default ("<org> Self Service") is used
	}
	if len(name) > 60 {
		return errors.New("the app name can be at most 60 characters")
	}
	if strings.ContainsAny(name, "/:\\") || strings.HasPrefix(name, ".") || strings.TrimSpace(name) != name {
		return errors.New("the app name cannot contain / : \\, start with a dot or have leading or trailing spaces")
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return errors.New("the app name must be printable text")
		}
	}
	return nil
}

// AppName returns the app's name for an organisation, falling back when name is unusable.
func AppName(name, org string) string {
	if CheckAppName(name) == nil && name != "" {
		return name
	}
	fallback := strings.TrimSpace(org) + " Self Service"
	if CheckAppName(fallback) != nil {
		return "Self Service"
	}
	return fallback
}
