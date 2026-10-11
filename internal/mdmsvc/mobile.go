package mdmsvc

import (
	"context"
	"time"

	"github.com/micromdm/nanomdm/mdm"

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
	}(*d)
}

// processMobileResult applies a command result from an iPhone or iPad.
func (s *Service) processMobileResult(ctx context.Context, d *model.MobileDevice, requestType string, doc map[string]any, results *mdm.CommandResults, errText string) {
	if requestType != "" {
		s.hooks.Fire(webhook.MDMCommandCompleted, map[string]any{"mobile_device_id": d.ID, "device": d.Name, "command_uuid": results.CommandUUID,
			"request_type": requestType, "status": results.Status, "error": errText})
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
		}
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
