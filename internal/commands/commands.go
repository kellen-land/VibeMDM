// Package commands builds Apple MDM command plists.
package commands

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/micromdm/nanomdm/mdm"
	"howett.net/plist"
)

// DeviceInformationQueries are the queries VibeMDM asks of a Mac.
var DeviceInformationQueries = []string{
	"UDID", "DeviceName", "OSVersion", "BuildVersion", "SupplementalBuildVersion", "SupplementalOSVersionExtra", "ModelName", "Model", "ProductName",
	"SerialNumber", "DeviceCapacity", "AvailableDeviceCapacity", "BatteryLevel", "IsSupervised", "IsActivationLockEnabled", "IsAppleSilicon",
	"LocalHostName", "HostName", "WiFiMAC", "EthernetMAC", "BluetoothMAC", "OSUpdateSettings", "SystemIntegrityProtectionEnabled",
	"ActiveManagedUsers", "AutoSetupAdminAccounts", "IsMultiUser", "ManagementStatus", "MDMOptions", "PINRequiredForEraseDevice",
	"PINRequiredForDeviceLock", "TimeZone", "AwaitingConfiguration", "EACSPreflight", "DeviceID",
}

// Build renders a command dict into an MDM command plist and parses it into a NanoMDM command.
func Build(requestType string, fields map[string]any) (*mdm.Command, error) {
	if requestType == "" {
		return nil, errors.New("empty RequestType")
	}
	cmd := map[string]any{"RequestType": requestType}
	for k, v := range fields {
		cmd[k] = v
	}
	doc := map[string]any{
		"CommandUUID": strings.ToUpper(uuid.NewString()),
		"Command":     cmd,
	}
	var buf bytes.Buffer
	enc := plist.NewEncoder(&buf)
	enc.Indent("\t")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return mdm.DecodeCommand(buf.Bytes())
}

// Raw parses a user-supplied raw command plist, assigning a CommandUUID when missing.
func Raw(raw []byte) (*mdm.Command, error) {
	var doc map[string]any
	if _, err := plist.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid plist: %w", err)
	}
	if _, ok := doc["CommandUUID"]; !ok {
		doc["CommandUUID"] = strings.ToUpper(uuid.NewString())
	}
	cmd, ok := doc["Command"].(map[string]any)
	if !ok {
		return nil, errors.New("plist must contain a Command dictionary")
	}
	if _, ok := cmd["RequestType"].(string); !ok {
		return nil, errors.New("Command must contain RequestType")
	}
	var buf bytes.Buffer
	enc := plist.NewEncoder(&buf)
	enc.Indent("\t")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return mdm.DecodeCommand(buf.Bytes())
}

// DeviceInformation requests inventory.
func DeviceInformation() (*mdm.Command, error) {
	return Build("DeviceInformation", map[string]any{"Queries": DeviceInformationQueries})
}

// SecurityInfo requests security state.
func SecurityInfo() (*mdm.Command, error) { return Build("SecurityInfo", nil) }

// ProfileList requests installed profiles.
func ProfileList() (*mdm.Command, error) {
	return Build("ProfileList", map[string]any{"ManagedOnly": false})
}

// InstalledApplicationList requests installed apps.
func InstalledApplicationList() (*mdm.Command, error) {
	return Build("InstalledApplicationList", map[string]any{"ManagedAppsOnly": false})
}

// CertificateList requests installed certificates.
func CertificateList() (*mdm.Command, error) {
	return Build("CertificateList", map[string]any{"ManagedOnly": false})
}

// InstallProfile installs a configuration profile (raw mobileconfig bytes).
func InstallProfile(mobileconfig []byte) (*mdm.Command, error) {
	return Build("InstallProfile", map[string]any{"Payload": mobileconfig})
}

// RemoveProfile removes a profile by identifier.
func RemoveProfile(identifier string) (*mdm.Command, error) {
	if identifier == "" {
		return nil, errors.New("identifier required")
	}
	return Build("RemoveProfile", map[string]any{"Identifier": identifier})
}

// DeviceLock locks the Mac. PIN is required on macOS (6 digits).
func DeviceLock(pin, message, phone string) (*mdm.Command, error) {
	if len(pin) != 6 {
		return nil, errors.New("a 6-digit PIN is required")
	}
	f := map[string]any{"PIN": pin}
	if message != "" {
		f["Message"] = message
	}
	if phone != "" {
		f["PhoneNumber"] = phone
	}
	return Build("DeviceLock", f)
}

// EraseDevice wipes the Mac.
func EraseDevice(pin string, obliterationBehavior string) (*mdm.Command, error) {
	f := map[string]any{}
	if pin != "" {
		if len(pin) != 6 {
			return nil, errors.New("PIN must be 6 digits")
		}
		f["PIN"] = pin
	}
	if obliterationBehavior != "" {
		f["ObliterationBehavior"] = obliterationBehavior
	}
	return Build("EraseDevice", f)
}

// RestartDevice restarts.
func RestartDevice() (*mdm.Command, error) { return Build("RestartDevice", nil) }

// ShutDownDevice shuts down.
func ShutDownDevice() (*mdm.Command, error) { return Build("ShutDownDevice", nil) }

// EnableRemoteDesktop turns on Remote Management.
func EnableRemoteDesktop() (*mdm.Command, error) { return Build("EnableRemoteDesktop", nil) }

// DisableRemoteDesktop turns off Remote Management.
func DisableRemoteDesktop() (*mdm.Command, error) { return Build("DisableRemoteDesktop", nil) }

// InstallEnterpriseApplication installs a signed pkg from a manifest URL.
func InstallEnterpriseApplication(manifestURL string) (*mdm.Command, error) {
	if manifestURL == "" {
		return nil, errors.New("manifest URL required")
	}
	return Build("InstallEnterpriseApplication", map[string]any{"ManifestURL": manifestURL, "InstallAsManaged": false})
}

// AvailableOSUpdates lists updates.
func AvailableOSUpdates() (*mdm.Command, error) { return Build("AvailableOSUpdates", nil) }

// ScheduleOSUpdate installs a specific update.
func ScheduleOSUpdate(productKey, installAction string) (*mdm.Command, error) {
	if installAction == "" {
		installAction = "Default"
	}
	upd := map[string]any{"InstallAction": installAction}
	if productKey != "" {
		upd["ProductKey"] = productKey
	}
	return Build("ScheduleOSUpdate", map[string]any{"Updates": []any{upd}})
}

// SetRecoveryLock sets the recovery lock password (Apple silicon).
func SetRecoveryLock(current, next string) (*mdm.Command, error) {
	return Build("SetRecoveryLock", map[string]any{"CurrentPassword": current, "NewPassword": next})
}

// DeviceConfigured releases an ADE device from Setup Assistant await.
func DeviceConfigured() (*mdm.Command, error) { return Build("DeviceConfigured", nil) }

// ParseResult decodes a command result plist into a generic map.
func ParseResult(raw []byte) (map[string]any, error) {
	var out map[string]any
	if _, err := plist.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DecodeBase64Payload is a helper for payloads embedded as base64 strings.
func DecodeBase64Payload(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
