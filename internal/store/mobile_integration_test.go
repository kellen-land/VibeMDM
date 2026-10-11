package store

import (
	"context"
	"testing"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/secrets"
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

func TestMobileExtensionAttributes(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	stamp := time.Now().Format("150405.000")
	d, _ := s.UpsertMobileFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: "EA-" + stamp, Model: "iPad11,6", ProductName: "iPad11,6", DeviceName: "cart iPad"})
	t.Cleanup(func() { s.DeleteMobileDevice(ctx, d.ID) })
	ea, err := s.SaveMobileEA(ctx, model.MobileExtensionAttribute{Name: "Cart " + stamp, DataType: "string", InputType: "popup", PopupChoices: []string{"A", "B"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteMobileEA(ctx, ea.ID) })
	if err := s.SetMobileEAValue(ctx, d.ID, ea.ID, "B"); err != nil {
		t.Fatal(err)
	}
	vals, _ := s.ListMobileEAValues(ctx, d.ID)
	found := false
	for _, v := range vals {
		if v.EAID == ea.ID && v.Value == "B" {
			found = true
		}
	}
	if !found {
		t.Fatalf("value not listed: %+v", vals)
	}
	ids, err := s.PreviewMobileCriteria(ctx, []model.Criterion{{Field: "ea:" + ea.ID, Operator: "is", Value: "B"}})
	if err != nil || len(ids) != 1 || ids[0] != d.ID {
		t.Fatalf("EA criterion: %v %v", ids, err)
	}
	fields, _ := s.MobileCriterionFields(ctx)
	if fields[len(fields)-1].Key != "ea:"+ea.ID && !func() bool {
		for _, f := range fields {
			if f.Key == "ea:"+ea.ID {
				return true
			}
		}
		return false
	}() {
		t.Fatal("EA missing from criteria fields")
	}
	s.SetMobileEAValue(ctx, d.ID, ea.ID, "")
	if ids, _ := s.PreviewMobileCriteria(ctx, []model.Criterion{{Field: "ea:" + ea.ID, Operator: "has_value"}}); len(ids) != 0 {
		t.Fatal("cleared value still matches")
	}
}

func TestMobileLostModeAndActivationLockBypass(t *testing.T) {
	s := testStore(t)
	box, err := secrets.New("test passphrase for the store tests")
	if err != nil {
		t.Fatal(err)
	}
	s.Box = box
	ctx := context.Background()
	stamp := time.Now().Format("150405.000")
	udid := "LOST-" + stamp
	d, _ := s.UpsertMobileFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: udid, Model: "iPhone14,4", ProductName: "iPhone14,4", DeviceName: "lost phone"})
	t.Cleanup(func() {
		s.DeleteMobileDevice(ctx, d.ID)
		s.DB.ExecContext(ctx, `DELETE FROM commands WHERE command_uuid = $1`, "cmd-"+udid)
		s.DB.ExecContext(ctx, `DELETE FROM devices WHERE id = $1`, udid)
	})

	// Lost Mode and location.
	acc := 12.5
	if err := s.SetMobileLostMode(ctx, udid, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMobileLocation(ctx, udid, 41.8781, -87.6298, &acc); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetMobileDevice(ctx, d.ID)
	if got.LostModeEnabled == nil || !*got.LostModeEnabled || got.Location == nil || got.Location.Latitude != 41.8781 || got.Location.Accuracy == nil || *got.Location.Accuracy != 12.5 {
		t.Fatalf("lost mode/location: %+v %+v", got.LostModeEnabled, got.Location)
	}

	// Bypass code: absent, then sealed at rest and readable through the store.
	if _, err := s.ActivationLockBypass(ctx, d.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	const code = "ABCD-EFGH-JKLM-NPQR-STUV-WXYZ"
	if err := s.SaveActivationLockBypass(ctx, d.ID, code); err != nil {
		t.Fatal(err)
	}
	var raw string
	s.DB.QueryRowContext(ctx, `SELECT bypass_code_encrypted FROM mobile_device_activation_lock WHERE mobile_device_id = $1`, d.ID).Scan(&raw)
	if raw == "" || raw == code {
		t.Fatalf("bypass code not sealed: %q", raw)
	}
	if c, err := s.ActivationLockBypass(ctx, d.ID); err != nil || c != code {
		t.Fatalf("bypass code: %q %v", c, err)
	}
	if got, _ := s.GetMobileDevice(ctx, d.ID); got.ActivationLockBypassAt == nil {
		t.Fatal("activation_lock_bypass_at not set")
	}

	// The raw command result NanoMDM keeps loses the plaintext code.
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO devices (id, authenticate, authenticate_at) VALUES ($1, '<?xml auth', now())`, udid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO enrollments (id, device_id, type, topic, push_magic, token_hex, last_seen_at) VALUES ($1, $1, 'Device', 't', 'm', 'aa', now())`, udid); err != nil {
		t.Fatal(err)
	}
	cmd := "cmd-" + udid
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO commands (command_uuid, request_type, command) VALUES ($1, 'ActivationLockBypassCode', '<?xml cmd')`, cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO command_results (id, command_uuid, status, result) VALUES ($1, $2, 'Acknowledged', $3)`,
		udid, cmd, "<?xml <string>"+code+"</string>"); err != nil {
		t.Fatal(err)
	}
	if err := s.ScrubCommandResult(ctx, cmd, code, "[sealed by VibeMDM]"); err != nil {
		t.Fatal(err)
	}
	var result string
	s.DB.QueryRowContext(ctx, `SELECT result FROM command_results WHERE command_uuid = $1`, cmd).Scan(&result)
	if result != "<?xml <string>[sealed by VibeMDM]</string>" {
		t.Fatalf("result not scrubbed: %q", result)
	}
}
