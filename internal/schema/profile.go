package schema

import (
	"strings"

	"github.com/google/uuid"
	"howett.net/plist"
)

// ProfileInput describes a managed-preferences profile.
type ProfileInput struct {
	Identifier   string // top-level PayloadIdentifier, e.g. com.vibemdm.custom.<id>
	DisplayName  string
	Description  string
	Organization string
	Domain       string // preference domain, e.g. com.macjutsu.super
	Values       map[string]any
}

// BuildProfile renders a configuration profile that forces the values for the
// domain using the com.apple.ManagedClient.preferences (MCX "Forced") payload,
// the same shape Jamf's Application & Custom Settings payload produces.
// UUIDs derive from the identifier so an unchanged profile renders identically.
func BuildProfile(in ProfileInput) ([]byte, error) {
	if err := CheckDomain(in.Domain); err != nil {
		return nil, err
	}
	inner := in.Identifier + ".mcx"
	top := map[string]any{
		"PayloadType": "Configuration", "PayloadVersion": 1, "PayloadIdentifier": in.Identifier,
		"PayloadUUID": stableUUID(in.Identifier), "PayloadDisplayName": in.DisplayName, "PayloadDescription": in.Description,
		"PayloadOrganization": in.Organization, "PayloadScope": "System",
		"PayloadContent": []any{map[string]any{
			"PayloadType": "com.apple.ManagedClient.preferences", "PayloadVersion": 1, "PayloadIdentifier": inner,
			"PayloadUUID": stableUUID(inner), "PayloadDisplayName": in.Domain,
			"PayloadContent": map[string]any{in.Domain: map[string]any{"Forced": []any{map[string]any{"mcx_preference_settings": in.Values}}}},
		}},
	}
	return plist.MarshalIndent(top, plist.XMLFormat, "\t")
}

func stableUUID(identifier string) string {
	return strings.ToUpper(uuid.NewSHA1(uuid.NameSpaceURL, []byte("vibemdm:"+identifier)).String())
}
