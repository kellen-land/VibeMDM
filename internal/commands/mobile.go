package commands

import (
	"errors"
	"strings"

	"github.com/micromdm/nanomdm/mdm"
)

// iPhone and iPad commands. Apple's Device Management reference lists which need supervision;
// the console checks that before sending (see MobileNeedsSupervision).

// MobileNeedsSupervision lists commands that iOS and iPadOS accept only from supervised devices.
var MobileNeedsSupervision = map[string]bool{
	"RestartDevice": true, "ShutDownDevice": true, "EnableLostMode": true, "DisableLostMode": true,
	"DeviceLocation": true, "PlayLostModeSound": true, "ActivationLockBypassCode": true,
}

// SettingNeedsSupervision lists Settings items that iOS applies only on supervised devices.
var SettingNeedsSupervision = map[string]bool{"DeviceName": true, "Bluetooth": true, "TimeZone": true}

// MobileLock locks the screen, optionally showing a message and phone number.
func MobileLock(message, phone string) (*mdm.Command, error) {
	f := map[string]any{}
	if message != "" {
		f["Message"] = message
	}
	if phone != "" {
		f["PhoneNumber"] = phone
	}
	return Build("DeviceLock", f)
}

// ClearPasscode removes the passcode using the UnlockToken the device escrowed at enrollment.
func ClearPasscode(unlockToken []byte) (*mdm.Command, error) {
	if len(unlockToken) == 0 {
		return nil, errors.New("the device has not escrowed an unlock token (it is sent at enrollment of supervised devices and when a passcode is set)")
	}
	return Build("ClearPasscode", map[string]any{"UnlockToken": unlockToken})
}

// MobileErase erases all content and settings.
func MobileErase(preserveDataPlan, disallowProximitySetup bool) (*mdm.Command, error) {
	return Build("EraseDevice", map[string]any{"PreserveDataPlan": preserveDataPlan, "DisallowProximitySetup": disallowProximitySetup})
}

// EnableLostMode locks a supervised device into Lost Mode with a message.
func EnableLostMode(message, phone, footnote string) (*mdm.Command, error) {
	if strings.TrimSpace(message) == "" && strings.TrimSpace(phone) == "" {
		return nil, errors.New("Lost Mode needs a message or a phone number")
	}
	f := map[string]any{}
	if message != "" {
		f["Message"] = message
	}
	if phone != "" {
		f["PhoneNumber"] = phone
	}
	if footnote != "" {
		f["Footnote"] = footnote
	}
	return Build("EnableLostMode", f)
}

// DisableLostMode ends Lost Mode.
func DisableLostMode() (*mdm.Command, error) { return Build("DisableLostMode", nil) }

// DeviceLocation asks a device in Lost Mode for its location.
func DeviceLocation() (*mdm.Command, error) { return Build("DeviceLocation", nil) }

// PlayLostModeSound plays a sound on a device in Lost Mode.
func PlayLostModeSound() (*mdm.Command, error) { return Build("PlayLostModeSound", nil) }

// ActivationLockBypassCode asks a supervised device for its Activation Lock bypass code.
func ActivationLockBypassCode() (*mdm.Command, error) { return Build("ActivationLockBypassCode", nil) }

// RefreshCellularPlans asks the device to fetch eSIM plans from the carrier server.
func RefreshCellularPlans(esimServerURL string) (*mdm.Command, error) {
	if !strings.HasPrefix(esimServerURL, "https://") {
		return nil, errors.New("the carrier's eSIM server URL must start with https://")
	}
	return Build("RefreshCellularPlans", map[string]any{"eSIMServerURL": esimServerURL})
}

// MobileSetting is one item of the Settings command.
type MobileSetting struct {
	Item       string `json:"item"` // DeviceName | Bluetooth | DataRoaming | PersonalHotspot | TimeZone
	DeviceName string `json:"device_name,omitempty"`
	Enabled    *bool  `json:"enabled,omitempty"`
	TimeZone   string `json:"time_zone,omitempty"`
}

// Settings changes device settings (several items at once).
func Settings(items []MobileSetting) (*mdm.Command, error) {
	if len(items) == 0 {
		return nil, errors.New("no settings to change")
	}
	var out []any
	for _, it := range items {
		m := map[string]any{"Item": it.Item}
		switch it.Item {
		case "DeviceName":
			name := strings.TrimSpace(it.DeviceName)
			if name == "" || len(name) > 63 {
				return nil, errors.New("the device name must be 1-63 characters")
			}
			m["DeviceName"] = name
		case "Bluetooth", "DataRoaming", "PersonalHotspot":
			if it.Enabled == nil {
				return nil, errors.New(it.Item + " needs enabled true or false")
			}
			m["Enabled"] = *it.Enabled
		case "TimeZone":
			if !strings.Contains(it.TimeZone, "/") {
				return nil, errors.New("the time zone must be an IANA name such as America/Chicago")
			}
			m["TimeZone"] = it.TimeZone
		default:
			return nil, errors.New("unsupported setting " + it.Item)
		}
		out = append(out, m)
	}
	return Build("Settings", map[string]any{"Settings": out})
}
