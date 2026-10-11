package main

import (
	"fmt"
	"strings"
	"time"
)

// iPhone and iPad answers. Commands not handled here fall through to the shared code
// (profiles, certificates, lock, erase…).

func (d *device) imei() string {
	if strings.HasPrefix(d.model, "iPad") && d.idx%2 == 0 {
		return "" // Wi-Fi-only iPad
	}
	return fmt.Sprintf("35%013d", 4000000000000+int64(d.idx)*7919)
}

func (d *device) respondMobile(r map[string]any, rt string, cmd map[string]any) bool {
	switch rt {
	case "DeviceInformation":
		isPad := strings.HasPrefix(d.model, "iPad")
		q := map[string]any{
			"UDID": d.udid, "DeviceName": d.name, "OSVersion": d.osVer, "BuildVersion": d.build, "ModelName": modelName(d.model),
			"Model": d.model, "ProductName": d.model, "SerialNumber": d.serial, "DeviceCapacity": 128.0, "AvailableDeviceCapacity": 61.5 - float64(d.idx),
			"BatteryLevel": 0.42 + float64(d.idx%5)/10, "WiFiMAC": fmt.Sprintf("f0:18:98:%02x:%02x:%02x", d.idx, d.idx*5, d.idx*9),
			"BluetoothMAC": fmt.Sprintf("f0:18:98:%02x:%02x:%02x", d.idx, d.idx*5, d.idx*9+1), "IsSupervised": d.supervised,
			"IsActivationLockEnabled": !d.supervised, "IsDeviceLocatorServiceEnabled": !d.supervised, "IsMDMLostModeEnabled": d.lostMode,
			"IsCloudBackupEnabled": true, "LastCloudBackupDate": time.Now().Add(-time.Duration(d.idx*13) * time.Hour).UTC(),
			"IsMultiUser": false, "TimeZone": "America/Chicago", "Languages": []any{"en-US"}, "Locales": []any{"en_US"},
			"ManagementStatus": map[string]any{"EnrolledViaDEP": false, "IsUserEnrollment": false},
		}
		if imei := d.imei(); imei != "" {
			q["IMEI"], q["PhoneNumber"], q["CurrentCarrierNetwork"], q["CellularTechnology"] = imei, fmt.Sprintf("+1 (555) 01%02d-%04d", d.idx, d.idx*37), "Verizon", 3
			q["IsRoaming"] = false
		} else if !isPad {
			q["CellularTechnology"] = 0
		}
		r["QueryResponses"] = q
	case "SecurityInfo":
		r["SecurityInfo"] = map[string]any{"HardwareEncryptionCaps": 3, "PasscodePresent": d.passcode, "PasscodeCompliant": d.passcode,
			"PasscodeCompliantWithProfiles": d.passcode, "PasscodeLockGracePeriodEnforced": 0}
	case "InstalledApplicationList":
		apps := []any{
			map[string]any{"Name": "Safari", "ShortVersion": d.osVer, "Identifier": "com.apple.mobilesafari"},
			map[string]any{"Name": "Google Chrome", "ShortVersion": "140.0.7339.122", "Identifier": "com.google.chrome.ios"},
			map[string]any{"Name": "Microsoft Teams", "ShortVersion": "7.18.1", "Identifier": "com.microsoft.skype.teams"},
		}
		if strings.HasPrefix(d.model, "iPad") {
			apps = append(apps, map[string]any{"Name": "Pages", "ShortVersion": "14.4", "Identifier": "com.apple.Pages"})
		}
		r["InstalledApplicationList"] = apps
	case "ManagedApplicationList":
		r["ManagedApplicationList"] = map[string]any{
			"com.microsoft.skype.teams": map[string]any{"Status": "Managed", "ManagementFlags": 0},
		}
	case "ClearPasscode":
		if _, ok := cmd["UnlockToken"].([]byte); !ok {
			mobileError(r, "The unlock token is missing.")
			break
		}
		d.passcode = false
	case "EnableLostMode":
		if !d.supervised {
			mobileError(r, "The device is not supervised.")
			break
		}
		d.lostMode = true
	case "DisableLostMode":
		d.lostMode = false
	case "DeviceLocation":
		if !d.lostMode {
			mobileError(r, "The device is not in Lost Mode.")
			break
		}
		r["Latitude"], r["Longitude"], r["HorizontalAccuracy"] = 41.8781+float64(d.idx)/1000, -87.6298-float64(d.idx)/1000, 15.0
	case "PlayLostModeSound":
		if !d.lostMode {
			mobileError(r, "The device is not in Lost Mode.")
		}
	case "Settings":
		items, _ := cmd["Settings"].([]any)
		for _, it := range items {
			m, _ := it.(map[string]any)
			if str(m["Item"]) == "DeviceName" && str(m["DeviceName"]) != "" {
				d.name = str(m["DeviceName"])
			}
		}
	case "ActivationLockBypassCode":
		if !d.supervised {
			mobileError(r, "The device is not supervised.")
			break
		}
		r["ActivationLockBypassCode"] = fmt.Sprintf("SIM%02d-ABCD-EFGH-JKLM-NPQR-STUV", d.idx)
	case "RefreshCellularPlans", "DeviceLock", "EraseDevice", "RestartDevice", "ShutDownDevice":
		// acknowledged
	default:
		return false
	}
	return true
}

func mobileError(r map[string]any, msg string) {
	r["Status"] = "Error"
	r["ErrorChain"] = []any{map[string]any{"ErrorCode": 12021, "ErrorDomain": "MCMDMErrorDomain", "LocalizedDescription": msg}}
}
