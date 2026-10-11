package store

import (
	"context"
	"testing"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

func TestMobileDevicesAreSeparateFromComputers(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	stamp := time.Now().Format("150405.000")
	pad, err := s.UpsertMobileFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: "PAD-" + stamp, SerialNumber: "PADSER" + stamp, DeviceName: "Kellen's iPad",
		Model: "iPad11,6", ModelName: "iPad (8th generation)", ProductName: "iPad11,6", OSVersion: "18.6.2"})
	if err != nil {
		t.Fatal(err)
	}
	phone, _ := s.UpsertMobileFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: "PHONE-" + stamp, SerialNumber: "PHSER" + stamp, DeviceName: "iPhone 13 mini",
		Model: "iPhone14,4", ProductName: "iPhone14,4", OSVersion: "26.0"})
	t.Cleanup(func() { s.DeleteMobileDevice(ctx, pad.ID); s.DeleteMobileDevice(ctx, phone.ID) })
	if pad.DeviceType != "iPad" || phone.DeviceType != "iPhone" {
		t.Fatalf("device types: %q %q", pad.DeviceType, phone.DeviceType)
	}
	if _, err := s.GetComputerByUDID(ctx, pad.UDID); err == nil {
		t.Fatal("an iPad must not appear as a computer")
	}
	if _, was, err := s.MarkMobileEnrolled(ctx, pad.UDID); err != nil || was {
		t.Fatalf("enroll: %v %v", was, err)
	}

	// Inventory.
	err = s.ApplyMobileDeviceInformation(ctx, pad.UDID, map[string]any{"DeviceName": "Classroom iPad 1", "IsSupervised": true,
		"DeviceCapacity": 64.0, "AvailableDeviceCapacity": 21.5, "BatteryLevel": 0.8, "WiFiMAC": "aa:bb:cc:dd:ee:ff", "IsMDMLostModeEnabled": false})
	if err != nil {
		t.Fatal(err)
	}
	s.ApplyMobileSecurityInfo(ctx, pad.UDID, map[string]any{"PasscodePresent": true, "PasscodeCompliant": true, "PasscodeCompliantWithProfiles": false, "HardwareEncryptionCaps": 3})
	s.ReplaceMobileApps(ctx, pad.ID, []model.MobileDeviceApp{{Name: "Pages", Version: "14.4", BundleID: "com.apple.Pages"}})
	s.SetMobileManagedApps(ctx, pad.ID, map[string]string{"com.apple.Pages": "Managed", "com.example.pending": "Installing"})
	d, _ := s.GetMobileDevice(ctx, pad.ID)
	if d.Name != "Classroom iPad 1" || d.IsSupervised == nil || !*d.IsSupervised || *d.CapacityGB != 64 || d.PasscodeCompliant == nil || *d.PasscodeCompliant {
		t.Fatalf("inventory: %+v", d)
	}
	apps, _ := s.ListMobileApps(ctx, pad.ID)
	if len(apps) != 2 {
		t.Fatalf("apps: %+v", apps)
	}
	// Installed list replaced again: managed status survives.
	s.ReplaceMobileApps(ctx, pad.ID, []model.MobileDeviceApp{{Name: "Pages", Version: "14.5", BundleID: "com.apple.Pages"}})
	apps, _ = s.ListMobileApps(ctx, pad.ID)
	if len(apps) != 1 || !apps[0].Managed || apps[0].ManagedStatus != "Managed" {
		t.Fatalf("managed status lost: %+v", apps)
	}

	// Smart and static mobile groups; computer groups do not see mobile devices.
	g, err := s.CreateMobileGroup(ctx, model.ComputerGroup{Name: "iPads " + stamp, IsSmart: true,
		Criteria: []model.Criterion{{Field: "device_type", Operator: "is", Value: "iPad"}, {AndOr: "and", Field: "is_supervised", Operator: "is", Value: "true"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteMobileGroup(ctx, g.ID) })
	ids, err := s.MobileGroupMemberIDs(ctx, g.ID)
	if err != nil || len(ids) != 1 || ids[0] != pad.ID {
		t.Fatalf("smart group: %v %v", ids, err)
	}
	st, _ := s.CreateMobileGroup(ctx, model.ComputerGroup{Name: "Static " + stamp})
	t.Cleanup(func() { s.DeleteMobileGroup(ctx, st.ID) })
	s.SetMobileStaticMembers(ctx, st.ID, []string{phone.ID})
	if groups, _ := s.MobileDeviceGroups(ctx, phone.ID); len(groups) != 1 || groups[0].ID != st.ID {
		t.Fatalf("phone groups: %+v", groups)
	}
	if _, err := s.PreviewMobileCriteria(ctx, []model.Criterion{{Field: "filevault_enabled", Operator: "is", Value: "true"}}); err == nil {
		t.Fatal("a Mac-only field must be rejected for mobile devices")
	}
	list, total, _ := s.ListMobileDevices(ctx, MobileDeviceFilter{GroupID: g.ID})
	if total != 1 || list[0].ID != pad.ID {
		t.Fatalf("list by group: %d", total)
	}

	// Searches.
	sr, err := s.CreateMobileSearch(ctx, model.AdvancedSearch{Name: "phones " + stamp, Criteria: []model.Criterion{{Field: "device_type", Operator: "is", Value: "iPhone"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteMobileSearch(ctx, sr.ID) })
	if res, _ := s.RunMobileSearch(ctx, sr.Criteria, 0); len(res) == 0 || res[0].DeviceType != "iPhone" {
		t.Fatalf("search: %+v", res)
	}
}
