package mdmsvc

import (
	"context"
	"time"

	"github.com/micromdm/nanomdm/mdm"

	"github.com/vibemdm/vibemdm/internal/commands"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/webhook"
)

// mobileTokenUpdate completes (or refreshes) an iPhone or iPad enrollment.
func (s *Service) mobileTokenUpdate(ctx context.Context, udid string) {
	d, wasEnrolled, err := s.st.MarkMobileEnrolled(ctx, udid)
	if err != nil {
		s.log.Error("mark mobile device enrolled", "err", err, "udid", udid)
		return
	}
	s.log.Info("mdm: token update", "udid", udid, "mobile_device", d.Name, "new_enrollment", !wasEnrolled)
	if wasEnrolled {
		return
	}
	s.hooks.Fire(webhook.MobileDeviceEnrolled, map[string]any{"mobile_device_id": d.ID, "device": d.Name, "serial": d.SerialNumber,
		"udid": d.UDID, "device_type": d.DeviceType, "os_version": d.OSVersion})
	if s.cmdr == nil {
		return
	}
	go func(d model.MobileDevice) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		time.Sleep(2 * time.Second) // let the device settle, as for Macs
		if err := s.cmdr.RequestMobileInventory(ctx, &d, "system"); err != nil {
			s.log.Warn("initial mobile inventory request", "err", err, "mobile_device", d.Name)
		}
		if err := s.cmdr.SyncMobileProfilesForDevice(ctx, &d, "system"); err != nil {
			s.log.Warn("mobile profiles on enrollment", "err", err, "mobile_device", d.Name)
		}
	}(*d)
}

// processMobileResult applies a command result from an iPhone or iPad.
func (s *Service) processMobileResult(ctx context.Context, d *model.MobileDevice, requestType string, doc map[string]any, results *mdm.CommandResults, errText string) {
	if requestType != "" {
		s.hooks.Fire(webhook.MDMCommandCompleted, map[string]any{"mobile_device_id": d.ID, "device": d.Name, "command_uuid": results.CommandUUID,
			"request_type": requestType, "status": results.Status, "error": errText})
	}
	switch requestType {
	case "InstallProfile", "RemoveProfile":
		status := map[string]string{"InstallProfile": "installed", "RemoveProfile": "removed"}[requestType]
		if errText != "" {
			status = "failed"
		}
		s.st.SetMobileDeploymentStatusByCommand(ctx, results.CommandUUID, status, errText)
		if errText == "" {
			s.mobileRefresh(ctx, d, commands.ProfileList)
		}
		return
	}
	if errText != "" {
		return
	}
	switch requestType {
	case "DeviceInformation":
		if q, ok := doc["QueryResponses"].(map[string]any); ok {
			if err := s.st.ApplyMobileDeviceInformation(ctx, d.UDID, q); err != nil {
				s.log.Error("apply mobile DeviceInformation", "err", err)
			}
			if sup, _ := q["IsSupervised"].(bool); sup {
				s.collectActivationLockBypass(ctx, d)
			}
		}
	case "DeviceLocation":
		lat, okLat := doc["Latitude"].(float64)
		lon, okLon := doc["Longitude"].(float64)
		if okLat && okLon {
			var acc *float64
			if v, ok := doc["HorizontalAccuracy"].(float64); ok {
				acc = &v
			}
			if err := s.st.SetMobileLocation(ctx, d.UDID, lat, lon, acc); err != nil {
				s.log.Error("store mobile location", "err", err)
			}
		}
	case "EnableLostMode", "DisableLostMode":
		s.st.SetMobileLostMode(ctx, d.UDID, requestType == "EnableLostMode")
		s.mobileRefresh(ctx, d, commands.MobileDeviceInformation)
	case "Settings":
		s.mobileRefresh(ctx, d, commands.MobileDeviceInformation)
	case "ClearPasscode", "DeviceLock":
		s.mobileRefresh(ctx, d, commands.SecurityInfo)
	case "ActivationLockBypassCode":
		code, _ := doc["ActivationLockBypassCode"].(string)
		if code == "" {
			return // the device has no code (Activation Lock not allowed or already handled)
		}
		if err := s.st.SaveActivationLockBypass(ctx, d.ID, code); err != nil {
			s.log.Error("store Activation Lock bypass code", "err", err, "mobile_device", d.Name)
			return
		}
		// The raw result in NanoMDM's command_results would otherwise keep the code in clear text.
		if err := s.st.ScrubCommandResult(ctx, results.CommandUUID, code, "[sealed by VibeMDM]"); err != nil {
			s.log.Warn("scrubbing the bypass code from the command result", "err", err)
		}
		s.st.Audit(ctx, "system", "mobile_device.activation_lock_bypass_collected", "mobile_device", d.ID, nil)
		s.log.Info("Activation Lock bypass code collected", "mobile_device", d.Name)
	case "SecurityInfo":
		if sec, ok := doc["SecurityInfo"].(map[string]any); ok {
			if err := s.st.ApplyMobileSecurityInfo(ctx, d.UDID, sec); err != nil {
				s.log.Error("apply mobile SecurityInfo", "err", err)
			}
		}
	case "ProfileList":
		if list, ok := doc["ProfileList"].([]any); ok {
			profiles := make([]model.ComputerProfile, 0, len(list))
			for _, it := range list {
				m, _ := it.(map[string]any)
				if m == nil {
					continue
				}
				profiles = append(profiles, model.ComputerProfile{Identifier: str(m["PayloadIdentifier"]), UUID: str(m["PayloadUUID"]),
					DisplayName: str(m["PayloadDisplayName"]), Organization: str(m["PayloadOrganization"]), IsManaged: boolean(m["IsManaged"]), Source: "mdm"})
			}
			if err := s.st.ReplaceMobileProfiles(ctx, d.ID, profiles); err != nil {
				s.log.Error("replace mobile profiles", "err", err)
			}
		}
	case "InstalledApplicationList":
		if list, ok := doc["InstalledApplicationList"].([]any); ok {
			apps := make([]model.MobileDeviceApp, 0, len(list))
			for _, it := range list {
				m, _ := it.(map[string]any)
				if m == nil {
					continue
				}
				ver := str(m["ShortVersion"])
				if ver == "" {
					ver = str(m["Version"])
				}
				apps = append(apps, model.MobileDeviceApp{Name: str(m["Name"]), Version: ver, BundleID: str(m["Identifier"])})
			}
			if err := s.st.ReplaceMobileApps(ctx, d.ID, apps); err != nil {
				s.log.Error("replace mobile apps", "err", err)
			}
		}
	case "ManagedApplicationList":
		if list, ok := doc["ManagedApplicationList"].(map[string]any); ok {
			statuses := make(map[string]string, len(list))
			for bundle, v := range list {
				m, _ := v.(map[string]any)
				statuses[bundle] = str(m["Status"])
			}
			if err := s.st.SetMobileManagedApps(ctx, d.ID, statuses); err != nil {
				s.log.Error("apply mobile ManagedApplicationList", "err", err)
			}
		}
	}
}

// mobileRefresh asks the device for fresh state after a command changed it.
func (s *Service) mobileRefresh(ctx context.Context, d *model.MobileDevice, build func() (*mdm.Command, error)) {
	if s.cmdr == nil {
		return
	}
	cmd, err := build()
	if err != nil {
		return
	}
	if _, err := s.cmdr.EnqueueMobile(ctx, d, cmd, "inventory", "", "system"); err != nil {
		s.log.Debug("mobile refresh", "err", err, "mobile_device", d.Name)
	}
}

// collectActivationLockBypass asks a supervised device for its Activation Lock bypass code when
// none is stored (at most once a day). Without it, an Activation-Locked device that is erased can
// only be unlocked with the user's Apple Account.
func (s *Service) collectActivationLockBypass(ctx context.Context, d *model.MobileDevice) {
	if s.cmdr == nil || d.ActivationLockBypassAt != nil {
		return
	}
	if sent, err := s.st.MobileCommandSentSince(ctx, d.ID, "ActivationLockBypassCode", time.Now().Add(-24*time.Hour)); err != nil || sent {
		return
	}
	s.mobileRefresh(ctx, d, commands.ActivationLockBypassCode)
}
