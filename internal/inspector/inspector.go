// Package inspector gives every MDM command a readable story: a timeline of
// queue → push → device connection → delivery → response, plain-language
// summaries per command type, and redaction of sensitive plist values.
package inspector

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"howett.net/plist"
)

// Event names recorded in mdm_command_events.
const (
	Queued       = "queued"
	PushSent     = "push_sent"
	PushFailed   = "push_failed"
	Connected    = "connected" // device connected to /mdm (any status)
	Delivered    = "delivered" // command handed to the device
	NotNow       = "not_now"
	Acknowledged = "acknowledged"
	Error        = "error"
	Cancelled    = "cancelled"
)

// Describe returns a one-line explanation of an event for the timeline.
func Describe(event string, details map[string]any) string {
	switch event {
	case Queued:
		return "Command queued on the server"
	case PushSent:
		return "APNs push sent to wake the device"
	case PushFailed:
		return "APNs push failed: " + str(details["error"])
	case Connected:
		if s := str(details["status"]); s != "" && s != "Idle" {
			return "Device connected and reported " + s
		}
		return "Device connected (Idle) to ask for work"
	case Delivered:
		return "Command delivered to the device"
	case NotNow:
		return "Device answered NotNow (busy or locked); it will retry"
	case Acknowledged:
		return "Device acknowledged the command"
	case Error:
		return "Device returned an error: " + str(details["error"])
	case Cancelled:
		return "Cancelled by an administrator"
	}
	return event
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// sensitiveKeys lists plist keys whose values are redacted by default.
var sensitiveKeys = []string{
	"PIN", "Password", "NewPassword", "CurrentPassword", "Payload", "FDE_PersonalRecoveryKeyCMS", "PersonalRecoveryKey",
	"RecoveryKey", "BootstrapToken", "UnlockToken", "Token", "PushMagic", "ChallengePassword", "Challenge", "Passcode",
	"ActivationLockBypassCode", "EscrowKey", "RotateRecoveryKey", "SecurityToken", "Identity", "Data", "Certificate",
	"VPNPassword", "ProxyPassword", "SharedSecret", "AuthPassword", "PrivateKey", "Secret", "PayloadContent",
}

var sensitiveSet = func() map[string]bool {
	m := make(map[string]bool, len(sensitiveKeys))
	for _, k := range sensitiveKeys {
		m[k] = true
	}
	return m
}()

// secretKeyRe catches keys that are secrets by name in any payload (UserPassword,
// IncomingPassword, clientSecret, LDAPAccountPassword, KeyPassword, LicenseFile, ...).
var secretKeyRe = regexp.MustCompile(`(?i)(password|secret|passphrase|presharedkey|passcode|challenge|token|license|privatekey)`)

// keyValueRe matches <key>NAME</key>, optional whitespace and comments, then a
// <string>/<data>/<integer> value.
var keyValueRe = regexp.MustCompile(`(?s)(<key>([^<]*)</key>(?:\s|<!--.*?-->)*<(?:string|data|integer)>)(.*?)(</(?:string|data|integer)>)`)

// Redact replaces sensitive plist values with a placeholder noting the original size.
func Redact(raw string) string {
	if raw == "" {
		return raw
	}
	return keyValueRe.ReplaceAllStringFunc(raw, func(m string) string {
		sub := keyValueRe.FindStringSubmatch(m)
		if len(sub) < 5 {
			return m
		}
		key := strings.TrimSpace(sub[2])
		if !sensitiveSet[key] && !secretKeyRe.MatchString(key) {
			return m
		}
		n := len(strings.TrimSpace(sub[3]))
		return fmt.Sprintf("%s[redacted %d bytes]%s", sub[1], n, sub[4])
	})
}

// Summary describes a command in plain language from its plist.
func Summary(requestType string, raw []byte) string {
	var doc struct {
		Command map[string]any
	}
	plist.Unmarshal(raw, &doc)
	c := doc.Command
	switch requestType {
	case "DeviceInformation":
		if q, ok := c["Queries"].([]any); ok {
			return fmt.Sprintf("Ask the device for %d inventory attributes (serial, model, OS, capacity, battery, MACs…)", len(q))
		}
		return "Ask the device for inventory attributes"
	case "SecurityInfo":
		return "Ask for security state: FileVault, firewall, SIP, Secure Boot, bootstrap token (and the escrowed recovery key when the escrow profile is installed)"
	case "ProfileList":
		return "List the configuration profiles installed on the device"
	case "InstalledApplicationList":
		return "List installed applications"
	case "CertificateList":
		return "List installed certificates"
	case "InstallProfile":
		return "Install a configuration profile (payload redacted)"
	case "RemoveProfile":
		return "Remove the profile " + str(c["Identifier"])
	case "DeviceLock":
		return "Lock the device with a PIN (PIN redacted)" + optional(" and message: ", str(c["Message"]))
	case "EraseDevice":
		return "Erase the device (PIN redacted)"
	case "RestartDevice":
		return "Restart the device"
	case "ShutDownDevice":
		return "Shut the device down"
	case "EnableRemoteDesktop":
		return "Turn on Remote Management (Screen Sharing)"
	case "DisableRemoteDesktop":
		return "Turn off Remote Management"
	case "InstallEnterpriseApplication":
		return "Install a signed package from " + str(c["ManifestURL"])
	case "AvailableOSUpdates":
		return "List available macOS updates"
	case "ScheduleOSUpdate":
		return "Schedule macOS update(s) to download/install"
	case "SetRecoveryLock":
		return "Set the recovery lock password (Apple silicon) (passwords redacted)"
	case "DeclarativeManagement":
		return "Enable Declarative Device Management or ask the device to sync its declarations (the device then fetches tokens, the declaration manifest and any changed declarations)"
	case "DeviceConfigured":
		return "Tell an Automated Device Enrollment device that setup can continue"
	}
	return "MDM command " + requestType
}

func optional(prefix, v string) string {
	if v == "" {
		return ""
	}
	return prefix + v
}

// ResultSummary describes a command result.
func ResultSummary(requestType, status string, raw []byte) string {
	var doc map[string]any
	plist.Unmarshal(raw, &doc)
	if status == "Error" || status == "CommandFormatError" {
		if chain, ok := doc["ErrorChain"].([]any); ok && len(chain) > 0 {
			if m, ok := chain[0].(map[string]any); ok {
				return status + ": " + firstNonEmpty(str(m["LocalizedDescription"]), str(m["USEnglishDescription"]), str(m["ErrorDomain"]))
			}
		}
		return status
	}
	switch requestType {
	case "DeviceInformation":
		if q, ok := doc["QueryResponses"].(map[string]any); ok {
			keys := make([]string, 0, len(q))
			for k := range q {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return fmt.Sprintf("Returned %d attributes: %s", len(q), strings.Join(preview(keys, 6), ", "))
		}
	case "SecurityInfo":
		if s, ok := doc["SecurityInfo"].(map[string]any); ok {
			parts := []string{}
			if v, ok := s["FDE_Enabled"].(bool); ok {
				parts = append(parts, fmt.Sprintf("FileVault %s", onOff(v)))
			}
			if _, ok := s["FDE_PersonalRecoveryKeyCMS"]; ok {
				parts = append(parts, "recovery key escrowed (redacted)")
			}
			if v, ok := s["SystemIntegrityProtectionEnabled"].(bool); ok {
				parts = append(parts, "SIP "+onOff(v))
			}
			if fw, ok := s["FirewallSettings"].(map[string]any); ok {
				if v, ok := fw["FirewallEnabled"].(bool); ok {
					parts = append(parts, "firewall "+onOff(v))
				}
			}
			return "Security state: " + strings.Join(parts, ", ")
		}
	case "ProfileList":
		if l, ok := doc["ProfileList"].([]any); ok {
			return fmt.Sprintf("%d profiles installed", len(l))
		}
	case "InstalledApplicationList":
		if l, ok := doc["InstalledApplicationList"].([]any); ok {
			return fmt.Sprintf("%d applications reported", len(l))
		}
	case "CertificateList":
		if l, ok := doc["CertificateList"].([]any); ok {
			return fmt.Sprintf("%d certificates reported", len(l))
		}
	case "AvailableOSUpdates":
		if l, ok := doc["AvailableOSUpdates"].([]any); ok {
			return fmt.Sprintf("%d updates available", len(l))
		}
	}
	return status
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func preview(keys []string, n int) []string {
	if len(keys) <= n {
		return keys
	}
	return append(keys[:n], "…")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
