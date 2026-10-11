package agentinstall

import (
	"strings"

	"github.com/google/uuid"
	"howett.net/plist"
)

// Identifiers used on the Mac.
const (
	BootstrapIdentifier = "com.vibemdm.agent.bootstrap"
	PrefsDomain         = "com.vibemdm.agent" // read by `vibemdm-agent enroll --from-mdm`
	BundleID            = "com.vibemdm.agent.pkg"
)

func stableUUID(s string) string {
	return strings.ToUpper(uuid.NewSHA1(uuid.NameSpaceURL, []byte("vibemdm:"+s)).String())
}

// BootstrapProfile builds the temporary profile: the package-signing root (trusted so the
// signed agent package installs) and managed preferences with the server URL and a
// single-use enrollment token for this Mac.
func BootstrapProfile(orgName, caCertPEM, serverURL, token string) ([]byte, error) {
	der, err := CADER(caCertPEM)
	if err != nil {
		return nil, err
	}
	if orgName == "" {
		orgName = "VibeMDM"
	}
	// A fresh UUID each time: the token inside changes, so this is a new profile version.
	top := map[string]any{
		"PayloadType": "Configuration", "PayloadVersion": 1, "PayloadIdentifier": BootstrapIdentifier,
		"PayloadUUID": strings.ToUpper(uuid.NewString()), "PayloadDisplayName": orgName + " Agent Setup",
		"PayloadDescription":  "Installs the " + orgName + " management agent. Removed automatically when the agent is set up.",
		"PayloadOrganization": orgName, "PayloadScope": "System",
		"PayloadContent": []any{
			map[string]any{
				"PayloadType": "com.apple.security.root", "PayloadVersion": 1,
				"PayloadIdentifier": BootstrapIdentifier + ".pkgca", "PayloadUUID": stableUUID(BootstrapIdentifier + ".pkgca"),
				"PayloadDisplayName": orgName + " Package Signing CA", "PayloadContent": der,
			},
			map[string]any{
				"PayloadType": "com.apple.ManagedClient.preferences", "PayloadVersion": 1,
				"PayloadIdentifier": BootstrapIdentifier + ".prefs", "PayloadUUID": stableUUID(BootstrapIdentifier + ".prefs"),
				"PayloadDisplayName": PrefsDomain,
				"PayloadContent": map[string]any{PrefsDomain: map[string]any{"Forced": []any{map[string]any{
					"mcx_preference_settings": map[string]any{"ServerURL": strings.TrimRight(serverURL, "/"), "EnrollToken": token},
				}}}},
			},
		},
	}
	return plist.MarshalIndent(top, plist.XMLFormat, "\t")
}

// Manifest builds the InstallEnterpriseApplication manifest for the package.
func Manifest(pkgURL string, size, chunk int64, md5s []string, version, title string) ([]byte, error) {
	asset := map[string]any{"kind": "software-package", "url": pkgURL, "md5-size": chunk, "md5s": md5s}
	doc := map[string]any{"items": []any{map[string]any{
		"assets": []any{asset},
		"metadata": map[string]any{"kind": "software", "title": title, "sizeInBytes": size,
			"bundle-identifier": BundleID, "bundle-version": version},
	}}}
	return plist.MarshalIndent(doc, plist.XMLFormat, "\t")
}
