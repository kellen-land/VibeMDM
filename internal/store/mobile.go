package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

// MobileDeviceType returns iPhone, iPad, iPod or AppleTV for an MDM ProductName/Model
// (e.g. "iPhone14,4", "iPad11,6"), or "" for a Mac.
func MobileDeviceType(productName, modelID string) string {
	for _, v := range []string{productName, modelID} {
		switch {
		case strings.HasPrefix(v, "iPhone"):
			return "iPhone"
		case strings.HasPrefix(v, "iPad"):
			return "iPad"
		case strings.HasPrefix(v, "iPod"):
			return "iPod"
		case strings.HasPrefix(v, "AppleTV"):
			return "AppleTV"
		}
	}
	return ""
}

// mobileFieldDefs are the smart-group and search fields for mobile devices.
var mobileFieldDefs = map[string]fieldDef{
	"device_name":             {"Device Name", "string", "c.name", ""},
	"serial_number":           {"Serial Number", "string", "c.serial_number", ""},
	"udid":                    {"UDID", "string", "c.udid", ""},
	"device_type":             {"Device Type", "string", "c.device_type", ""},
	"model_name":              {"Model", "string", "c.model_name", ""},
	"model":                   {"Model Identifier", "string", "c.model", ""},
	"os_version":              {"OS Version", "version", "c.os_version", ""},
	"os_build":                {"OS Build", "string", "c.os_build", ""},
	"capacity_gb":             {"Capacity (GB)", "number", "c.capacity_gb", ""},
	"available_gb":            {"Available Space (GB)", "number", "c.available_gb", ""},
	"battery_percent":         {"Battery Level (%)", "number", "round(c.battery_level * 100)", ""},
	"wifi_mac":                {"Wi-Fi MAC Address", "string", "c.wifi_mac", ""},
	"imei":                    {"IMEI", "string", "c.imei", ""},
	"phone_number":            {"Phone Number", "string", "c.phone_number", ""},
	"carrier":                 {"Carrier", "string", "c.carrier", ""},
	"is_supervised":           {"Supervised", "bool", "c.is_supervised", ""},
	"is_shared_ipad":          {"Shared iPad", "bool", "c.is_shared_ipad", ""},
	"activation_lock_enabled": {"Activation Lock Enabled", "bool", "c.activation_lock_enabled", ""},
	"find_my_enabled":         {"Find My Enabled", "bool", "c.find_my_enabled", ""},
	"lost_mode_enabled":       {"Lost Mode Enabled", "bool", "c.lost_mode_enabled", ""},
	"passcode_present":        {"Passcode Set", "bool", "c.passcode_present", ""},
	"passcode_compliant":      {"Passcode Compliant", "bool", "c.passcode_compliant", ""},
	"mdm_enrolled":            {"MDM Enrolled", "bool", "c.mdm_enrolled", ""},
	"assigned_user":           {"Assigned Username", "string", "c.assigned_user", ""},
	"assigned_email":          {"Assigned Email", "string", "c.assigned_email", ""},
	"department":              {"Department", "string", "c.department", ""},
	"building":                {"Building", "string", "c.building", ""},
	"site":                    {"Site", "string", "c.site", ""},
	"asset_tag":               {"Asset Tag", "string", "c.asset_tag", ""},
	"last_checkin":            {"Last Check-in", "date", "c.mdm_last_checkin", ""},
	"last_inventory":          {"Last Inventory Update", "date", "c.last_inventory_at", ""},
	"last_backup":             {"Last iCloud Backup", "date", "c.last_backup_at", ""},
	"mdm_enrolled_at":         {"Enrollment Date", "date", "c.mdm_enrolled_at", ""},
	"created_at":              {"Record Created", "date", "c.created_at", ""},
	"application_title":       {"App Name", "string", "", `EXISTS (SELECT 1 FROM mobile_device_apps x WHERE x.mobile_device_id = c.id AND %s)`},
	"application_version":     {"App Version", "string", "", `EXISTS (SELECT 1 FROM mobile_device_apps x WHERE x.mobile_device_id = c.id AND %s)`},
	"application_bundle_id":   {"App Identifier", "string", "", `EXISTS (SELECT 1 FROM mobile_device_apps x WHERE x.mobile_device_id = c.id AND %s)`},
	"profile_identifier":      {"Profile Identifier", "string", "", `EXISTS (SELECT 1 FROM mobile_device_installed_profiles x WHERE x.mobile_device_id = c.id AND %s)`},
	"profile_name":            {"Profile Name", "string", "", `EXISTS (SELECT 1 FROM mobile_device_installed_profiles x WHERE x.mobile_device_id = c.id AND %s)`},
	"group_membership":        {"Mobile Device Group", "group", "", ""},
}

var mobileDomain = &groupDomain{devices: "mobile_devices", groups: "mobile_device_groups", members: "mobile_device_group_members",
	memberCol: "mobile_device_id", eaValues: "mobile_device_ea_values", eaTable: "mobile_extension_attributes", fields: mobileFieldDefs,
	existsCols: map[string]string{"application_title": "x.name", "application_version": "x.version", "application_bundle_id": "x.bundle_id",
		"profile_identifier": "x.identifier", "profile_name": "x.display_name"},
	order: []string{"device_name", "serial_number", "udid", "device_type", "model_name", "model", "os_version", "os_build", "capacity_gb",
		"available_gb", "battery_percent", "wifi_mac", "imei", "phone_number", "carrier", "is_supervised", "is_shared_ipad",
		"activation_lock_enabled", "find_my_enabled", "lost_mode_enabled", "passcode_present", "passcode_compliant", "mdm_enrolled",
		"assigned_user", "assigned_email", "department", "building", "site", "asset_tag", "last_checkin", "last_inventory", "last_backup",
		"mdm_enrolled_at", "created_at", "application_title", "application_version", "application_bundle_id", "profile_identifier",
		"profile_name", "group_membership"},
}

// ---- mobile device groups ----

// MobileCriterionFields lists the smart-group fields for mobile devices, including extension attributes.
func (s *Store) MobileCriterionFields(ctx context.Context) ([]CriterionField, error) {
	out := domainFields(mobileDomain)
	eas, err := s.ListMobileEAs(ctx)
	if err != nil {
		return nil, err
	}
	for _, ea := range eas {
		typ := map[string]string{"integer": "number", "date": "date"}[ea.DataType]
		if typ == "" {
			typ = "string"
		}
		out = append(out, CriterionField{Key: "ea:" + ea.ID, Label: "EA: " + ea.Name, Type: typ, Operators: opsFor(typ)})
	}
	return out, nil
}

// ListMobileGroups returns all mobile device groups with member counts.
func (s *Store) ListMobileGroups(ctx context.Context) ([]model.ComputerGroup, error) {
	return s.listGroups(ctx, mobileDomain)
}

// GetMobileGroup returns a mobile device group.
func (s *Store) GetMobileGroup(ctx context.Context, id string) (*model.ComputerGroup, error) {
	return s.getGroup(ctx, mobileDomain, id)
}

// CreateMobileGroup inserts a mobile device group.
func (s *Store) CreateMobileGroup(ctx context.Context, g model.ComputerGroup) (*model.ComputerGroup, error) {
	return s.createGroup(ctx, mobileDomain, g)
}

// UpdateMobileGroup updates a mobile device group.
func (s *Store) UpdateMobileGroup(ctx context.Context, g model.ComputerGroup) (*model.ComputerGroup, error) {
	return s.updateGroup(ctx, mobileDomain, g)
}

// DeleteMobileGroup removes a mobile device group.
func (s *Store) DeleteMobileGroup(ctx context.Context, id string) error {
	return s.deleteGroup(ctx, mobileDomain, id)
}

// SetMobileStaticMembers replaces a static mobile group's members.
func (s *Store) SetMobileStaticMembers(ctx context.Context, groupID string, ids []string) error {
	return s.setStaticMembers(ctx, mobileDomain, groupID, ids)
}

// AddMobileStaticMembers adds mobile devices to a static group.
func (s *Store) AddMobileStaticMembers(ctx context.Context, groupID string, ids []string) error {
	return s.addStaticMembers(ctx, mobileDomain, groupID, ids)
}

// RemoveMobileStaticMember removes a mobile device from a static group.
func (s *Store) RemoveMobileStaticMember(ctx context.Context, groupID, id string) error {
	return s.removeStaticMember(ctx, mobileDomain, groupID, id)
}

// MobileGroupMemberIDs resolves a mobile group's member IDs.
func (s *Store) MobileGroupMemberIDs(ctx context.Context, groupID string) ([]string, error) {
	return s.groupMemberIDs(ctx, mobileDomain, groupID)
}

// PreviewMobileCriteria evaluates criteria against mobile devices.
func (s *Store) PreviewMobileCriteria(ctx context.Context, criteria []model.Criterion) ([]string, error) {
	return s.previewCriteria(ctx, mobileDomain, criteria)
}

// MobileDeviceGroups lists the groups a mobile device belongs to.
func (s *Store) MobileDeviceGroups(ctx context.Context, id string) ([]model.ComputerGroup, error) {
	groups, err := s.listGroups(ctx, mobileDomain)
	if err != nil {
		return nil, err
	}
	out := []model.ComputerGroup{}
	for _, g := range groups {
		in, err := s.isGroupMember(ctx, mobileDomain, g.ID, id)
		if err != nil {
			return nil, err
		}
		if in {
			out = append(out, g)
		}
	}
	return out, nil
}

// ---- mobile devices ----

const mobileCols = `c.id, c.udid, COALESCE(c.serial_number,''), c.name, c.device_type, c.model, c.model_name, c.product_name, c.os_version, c.os_build,
c.capacity_gb, c.available_gb, c.battery_level, c.wifi_mac, c.bluetooth_mac, c.imei, c.meid, c.phone_number, c.carrier, c.cellular_technology,
c.is_roaming, c.is_supervised, c.is_shared_ipad, c.activation_lock_enabled, c.find_my_enabled, c.lost_mode_enabled, c.passcode_present,
c.passcode_compliant, c.hardware_encryption, c.last_backup_at, c.mdm_enrolled, c.mdm_enrolled_at, c.mdm_last_checkin, c.mdm_unenrolled_at,
c.device_info, c.security_info, c.last_inventory_at, c.asset_tag, c.site, c.department, c.building, c.room, c.assigned_user,
c.assigned_real_name, c.assigned_email, c.notes, c.created_at, c.updated_at,
c.last_latitude, c.last_longitude, c.last_location_accuracy, c.last_location_at,
(SELECT al.collected_at FROM mobile_device_activation_lock al WHERE al.mobile_device_id = c.id)`

func scanMobile(row interface{ Scan(...any) error }) (*model.MobileDevice, error) {
	var d model.MobileDevice
	var capGB, availGB, battery sql.NullFloat64
	var roaming, sup, shared, al, findMy, lost, pcPresent, pcCompliant sql.NullBool
	var hwEnc sql.NullInt64
	var backup, enrAt, last, unenr, inv sql.NullTime
	var devInfo, secInfo db.JSON[map[string]any]
	var lat, lon, acc sql.NullFloat64
	var locAt, alAt sql.NullTime
	err := row.Scan(&d.ID, &d.UDID, &d.SerialNumber, &d.Name, &d.DeviceType, &d.Model, &d.ModelName, &d.ProductName, &d.OSVersion, &d.OSBuild,
		&capGB, &availGB, &battery, &d.WiFiMAC, &d.BluetoothMAC, &d.IMEI, &d.MEID, &d.PhoneNumber, &d.Carrier, &d.CellularTechnology,
		&roaming, &sup, &shared, &al, &findMy, &lost, &pcPresent, &pcCompliant, &hwEnc, &backup, &d.MDMEnrolled, &enrAt, &last, &unenr,
		&devInfo, &secInfo, &inv, &d.AssetTag, &d.Site, &d.Department, &d.Building, &d.Room, &d.AssignedUser,
		&d.AssignedRealName, &d.AssignedEmail, &d.Notes, &d.CreatedAt, &d.UpdatedAt, &lat, &lon, &acc, &locAt, &alAt)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	if lat.Valid && lon.Valid && locAt.Valid {
		d.Location = &model.DeviceLocation{Latitude: lat.Float64, Longitude: lon.Float64, At: locAt.Time}
		if acc.Valid {
			v := acc.Float64
			d.Location.Accuracy = &v
		}
	}
	d.ActivationLockBypassAt = db.TimePtr(alAt)
	f := func(n sql.NullFloat64) *float64 {
		if !n.Valid {
			return nil
		}
		v := n.Float64
		return &v
	}
	b := func(n sql.NullBool) *bool {
		if !n.Valid {
			return nil
		}
		v := n.Bool
		return &v
	}
	d.CapacityGB, d.AvailableGB, d.BatteryLevel = f(capGB), f(availGB), f(battery)
	d.IsRoaming, d.IsSupervised, d.IsSharedIPad, d.ActivationLockEnabled = b(roaming), b(sup), b(shared), b(al)
	d.FindMyEnabled, d.LostModeEnabled, d.PasscodePresent, d.PasscodeCompliant = b(findMy), b(lost), b(pcPresent), b(pcCompliant)
	if hwEnc.Valid {
		v := int(hwEnc.Int64)
		d.HardwareEncryption = &v
	}
	d.LastBackupAt, d.MDMEnrolledAt, d.MDMLastCheckin, d.MDMUnenrolledAt, d.LastInventoryAt = db.TimePtr(backup), db.TimePtr(enrAt), db.TimePtr(last), db.TimePtr(unenr), db.TimePtr(inv)
	d.DeviceInfo, d.SecurityInfo = devInfo.V, secInfo.V
	if d.DeviceInfo == nil {
		d.DeviceInfo = map[string]any{}
	}
	if d.SecurityInfo == nil {
		d.SecurityInfo = map[string]any{}
	}
	return &d, nil
}

// MobileDeviceFilter filters the mobile device list.
type MobileDeviceFilter struct {
	Export  bool // CSV export: up to ExportLimit rows instead of one page
	Query   string
	GroupID string
	Sort    string
	Desc    bool
	Limit   int
	Offset  int
	IDs     []string
}

var mobileSortCols = map[string]string{
	"name": "lower(c.name)", "serial_number": "c.serial_number", "os_version": "vibemdm_version_array(c.os_version)",
	"model_name": "c.model_name", "device_type": "c.device_type", "last_checkin": "c.mdm_last_checkin",
	"last_inventory_at": "c.last_inventory_at", "created_at": "c.created_at", "assigned_user": "c.assigned_user",
}

// ListMobileDevices returns mobile devices matching a filter, and the total count.
func (s *Store) ListMobileDevices(ctx context.Context, f MobileDeviceFilter) ([]model.MobileDevice, int, error) {
	f.Limit, f.Offset = clampFilterPage(f.Limit, f.Offset, f.Export)
	var where []string
	var args []any
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, likeArg(q))
		n := len(args)
		where = append(where, fmt.Sprintf(`(c.name ILIKE $%d OR c.serial_number ILIKE $%d OR c.udid ILIKE $%d OR c.model_name ILIKE $%d OR c.assigned_user ILIKE $%d OR c.asset_tag ILIKE $%d OR c.phone_number ILIKE $%d OR c.imei ILIKE $%d)`, n, n, n, n, n, n, n, n))
	}
	if f.GroupID != "" {
		sub, subArgs, err := s.groupMembershipSQL(ctx, mobileDomain, f.GroupID, len(args), 0)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, subArgs...)
		where = append(where, sub)
	}
	if len(f.IDs) > 0 {
		args = append(args, f.IDs)
		where = append(where, fmt.Sprintf(`c.id::text = ANY($%d::text[])`, len(args)))
	}
	w := ""
	if len(where) > 0 {
		w = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM mobile_devices c`+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := mobileSortCols["name"]
	if col, ok := mobileSortCols[f.Sort]; ok {
		order = col
	}
	dir := "ASC"
	if f.Desc {
		dir = "DESC"
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+mobileCols+` FROM mobile_devices c`+w+fmt.Sprintf(` ORDER BY %s %s NULLS LAST, c.id LIMIT $%d OFFSET $%d`, order, dir, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.MobileDevice{}
	for rows.Next() {
		d, err := scanMobile(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *d)
	}
	return out, total, rows.Err()
}

// GetMobileDevice returns a mobile device by ID.
func (s *Store) GetMobileDevice(ctx context.Context, id string) (*model.MobileDevice, error) {
	return scanMobile(s.DB.QueryRowContext(ctx, `SELECT `+mobileCols+` FROM mobile_devices c WHERE c.id::text = $1`, id))
}

// GetMobileDeviceByUDID returns a mobile device by UDID.
func (s *Store) GetMobileDeviceByUDID(ctx context.Context, udid string) (*model.MobileDevice, error) {
	return scanMobile(s.DB.QueryRowContext(ctx, `SELECT `+mobileCols+` FROM mobile_devices c WHERE c.udid = $1`, udid))
}

// UpsertMobileFromAuthenticate creates or updates a mobile device from an MDM Authenticate.
func (s *Store) UpsertMobileFromAuthenticate(ctx context.Context, a MDMAuthenticateInfo) (*model.MobileDevice, error) {
	typ := MobileDeviceType(a.ProductName, a.Model)
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO mobile_devices (udid, serial_number, name, device_type, model, model_name, product_name, os_version, os_build)
		VALUES ($1, NULLIF($2,''), $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (udid) DO UPDATE SET serial_number = COALESCE(NULLIF(EXCLUDED.serial_number,''), mobile_devices.serial_number),
		name = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.name ELSE mobile_devices.name END,
		device_type = CASE WHEN EXCLUDED.device_type <> '' THEN EXCLUDED.device_type ELSE mobile_devices.device_type END,
		model = CASE WHEN EXCLUDED.model <> '' THEN EXCLUDED.model ELSE mobile_devices.model END,
		model_name = CASE WHEN EXCLUDED.model_name <> '' THEN EXCLUDED.model_name ELSE mobile_devices.model_name END,
		product_name = CASE WHEN EXCLUDED.product_name <> '' THEN EXCLUDED.product_name ELSE mobile_devices.product_name END,
		os_version = CASE WHEN EXCLUDED.os_version <> '' THEN EXCLUDED.os_version ELSE mobile_devices.os_version END,
		os_build = CASE WHEN EXCLUDED.os_build <> '' THEN EXCLUDED.os_build ELSE mobile_devices.os_build END,
		updated_at = now() RETURNING id`,
		a.UDID, a.SerialNumber, a.DeviceName, typ, a.Model, a.ModelName, a.ProductName, a.OSVersion, a.BuildVersion).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetMobileDevice(ctx, id)
}

// MarkMobileEnrolled flags a mobile device as MDM-enrolled (TokenUpdate). wasEnrolled is false
// for a new enrollment.
func (s *Store) MarkMobileEnrolled(ctx context.Context, udid string) (*model.MobileDevice, bool, error) {
	var id string
	var wasEnrolled bool
	err := s.DB.QueryRowContext(ctx, `UPDATE mobile_devices SET mdm_enrolled = TRUE,
		mdm_enrolled_at = CASE WHEN mdm_enrolled THEN mdm_enrolled_at ELSE now() END,
		mdm_last_checkin = now(), mdm_unenrolled_at = NULL, updated_at = now() WHERE udid = $1
		RETURNING id, (mdm_enrolled_at < now() - interval '1 second')`, udid).Scan(&id, &wasEnrolled)
	if err != nil {
		return nil, false, wrapNotFound(err)
	}
	d, err := s.GetMobileDevice(ctx, id)
	return d, wasEnrolled, err
}

// MarkMobileUnenrolled flags a mobile device as no longer enrolled (CheckOut).
func (s *Store) MarkMobileUnenrolled(ctx context.Context, udid string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE mobile_devices SET mdm_enrolled = FALSE, mdm_unenrolled_at = now(), updated_at = now() WHERE udid = $1`, udid)
	return err
}

// MobileDeviceEdit holds the editable fields of a mobile device record.
type MobileDeviceEdit struct {
	AssetTag         *string `json:"asset_tag"`
	Site             *string `json:"site"`
	Department       *string `json:"department"`
	Building         *string `json:"building"`
	Room             *string `json:"room"`
	AssignedUser     *string `json:"assigned_user"`
	AssignedRealName *string `json:"assigned_real_name"`
	AssignedEmail    *string `json:"assigned_email"`
	Notes            *string `json:"notes"`
}

// UpdateMobileDevice applies an edit. The device name is changed with the Settings command, not here.
func (s *Store) UpdateMobileDevice(ctx context.Context, id string, e MobileDeviceEdit) error {
	sets := []string{"updated_at = now()"}
	args := []any{id}
	add := func(col string, v *string) {
		if v != nil {
			args = append(args, *v)
			sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
		}
	}
	add("asset_tag", e.AssetTag)
	add("site", e.Site)
	add("department", e.Department)
	add("building", e.Building)
	add("room", e.Room)
	add("assigned_user", e.AssignedUser)
	add("assigned_real_name", e.AssignedRealName)
	add("assigned_email", e.AssignedEmail)
	add("notes", e.Notes)
	res, err := s.DB.ExecContext(ctx, `UPDATE mobile_devices SET `+strings.Join(sets, ", ")+` WHERE id::text = $1`, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMobileDevice removes a mobile device record (its MDM enrollment is untouched).
func (s *Store) DeleteMobileDevice(ctx context.Context, id string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM mobile_devices WHERE id::text = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- inventory from MDM ----

func anyFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	case uint64:
		return float64(x), true
	case int:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

// ApplyMobileDeviceInformation stores a DeviceInformation response.
func (s *Store) ApplyMobileDeviceInformation(ctx context.Context, udid string, q map[string]any) error {
	str := func(k string) string { v, _ := q[k].(string); return v }
	nb := func(k string) sql.NullBool {
		v, ok := q[k].(bool)
		return sql.NullBool{Bool: v, Valid: ok}
	}
	nf := func(k string) sql.NullFloat64 {
		v, ok := anyFloat(q[k])
		return sql.NullFloat64{Float64: v, Valid: ok}
	}
	carrier := str("CurrentCarrierNetwork")
	if carrier == "" {
		carrier = str("SubscriberCarrierNetwork")
	}
	imei, phone := str("IMEI"), str("PhoneNumber")
	// Dual-SIM devices report cellular details per subscription.
	if subs, ok := q["ServiceSubscriptions"].([]any); ok && len(subs) > 0 {
		if m, ok := subs[0].(map[string]any); ok {
			if imei == "" {
				imei, _ = m["IMEI"].(string)
			}
			if phone == "" {
				phone, _ = m["PhoneNumber"].(string)
			}
			if carrier == "" {
				carrier, _ = m["CurrentCarrierNetwork"].(string)
			}
		}
	}
	tech := ""
	if v, ok := anyFloat(q["CellularTechnology"]); ok {
		tech = map[int]string{0: "None", 1: "GSM", 2: "CDMA", 3: "GSM and CDMA"}[int(v)]
	}
	var backup sql.NullTime
	if t, ok := q["LastCloudBackupDate"].(time.Time); ok {
		backup = sql.NullTime{Time: t, Valid: true}
	}
	typ := MobileDeviceType(str("ProductName"), str("Model"))
	_, err := s.DB.ExecContext(ctx, `UPDATE mobile_devices SET
		name = CASE WHEN $2 <> '' THEN $2 ELSE name END, os_version = CASE WHEN $3 <> '' THEN $3 ELSE os_version END,
		os_build = CASE WHEN $4 <> '' THEN $4 ELSE os_build END, model_name = CASE WHEN $5 <> '' THEN $5 ELSE model_name END,
		model = CASE WHEN $6 <> '' THEN $6 ELSE model END, product_name = CASE WHEN $7 <> '' THEN $7 ELSE product_name END,
		device_type = CASE WHEN $8 <> '' THEN $8 ELSE device_type END,
		serial_number = COALESCE(NULLIF($9,''), serial_number),
		capacity_gb = COALESCE($10, capacity_gb), available_gb = COALESCE($11, available_gb), battery_level = COALESCE($12, battery_level),
		wifi_mac = CASE WHEN $13 <> '' THEN $13 ELSE wifi_mac END, bluetooth_mac = CASE WHEN $14 <> '' THEN $14 ELSE bluetooth_mac END,
		imei = $15, meid = $16, phone_number = $17, carrier = $18, cellular_technology = $19,
		is_roaming = COALESCE($20, is_roaming), is_supervised = COALESCE($21, is_supervised), is_shared_ipad = COALESCE($22, is_shared_ipad),
		activation_lock_enabled = COALESCE($23, activation_lock_enabled), find_my_enabled = COALESCE($24, find_my_enabled),
		lost_mode_enabled = COALESCE($25, lost_mode_enabled), last_backup_at = COALESCE($26, last_backup_at),
		device_info = $27::jsonb, last_inventory_at = now(), mdm_last_checkin = now(), updated_at = now()
		WHERE udid = $1`,
		udid, str("DeviceName"), str("OSVersion"), str("BuildVersion"), str("ModelName"), str("Model"), str("ProductName"), typ,
		str("SerialNumber"), nf("DeviceCapacity"), nf("AvailableDeviceCapacity"), nf("BatteryLevel"), str("WiFiMAC"), str("BluetoothMAC"),
		imei, str("MEID"), phone, carrier, tech, nb("IsRoaming"), nb("IsSupervised"), nb("IsMultiUser"), nb("IsActivationLockEnabled"),
		nb("IsDeviceLocatorServiceEnabled"), nb("IsMDMLostModeEnabled"), backup, jsonArg(q))
	return err
}

// ApplyMobileSecurityInfo stores a SecurityInfo response.
func (s *Store) ApplyMobileSecurityInfo(ctx context.Context, udid string, sec map[string]any) error {
	nb := func(k string) sql.NullBool {
		v, ok := sec[k].(bool)
		return sql.NullBool{Bool: v, Valid: ok}
	}
	var hw sql.NullInt64
	if v, ok := anyFloat(sec["HardwareEncryptionCaps"]); ok {
		hw = sql.NullInt64{Int64: int64(v), Valid: true}
	}
	compliant := nb("PasscodeCompliant")
	if p := nb("PasscodeCompliantWithProfiles"); p.Valid && compliant.Valid {
		compliant.Bool = compliant.Bool && p.Bool
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE mobile_devices SET passcode_present = COALESCE($2, passcode_present),
		passcode_compliant = COALESCE($3, passcode_compliant), hardware_encryption = COALESCE($4, hardware_encryption),
		security_info = $5::jsonb, mdm_last_checkin = now(), updated_at = now() WHERE udid = $1`,
		udid, nb("PasscodePresent"), compliant, hw, jsonArg(sec))
	return err
}

// ReplaceMobileApps replaces a device's installed app list (InstalledApplicationList), keeping
// managed status from the last ManagedApplicationList.
func (s *Store) ReplaceMobileApps(ctx context.Context, id string, apps []model.MobileDeviceApp) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	managed := map[string]string{}
	rows, err := tx.QueryContext(ctx, `SELECT bundle_id, managed_status FROM mobile_device_apps WHERE mobile_device_id = $1 AND managed`, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var b, st string
		if err := rows.Scan(&b, &st); err != nil {
			rows.Close()
			return err
		}
		managed[b] = st
	}
	rows.Close()
	if _, err := tx.ExecContext(ctx, `DELETE FROM mobile_device_apps WHERE mobile_device_id = $1`, id); err != nil {
		return err
	}
	for _, a := range apps {
		st, isManaged := managed[a.BundleID]
		if a.Managed {
			isManaged, st = true, a.ManagedStatus
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO mobile_device_apps (mobile_device_id, name, version, bundle_id, managed, managed_status) VALUES ($1,$2,$3,$4,$5,$6)`,
			id, a.Name, a.Version, a.BundleID, isManaged, st); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetMobileManagedApps applies a ManagedApplicationList response: bundle ID → status.
func (s *Store) SetMobileManagedApps(ctx context.Context, id string, statuses map[string]string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE mobile_device_apps SET managed = FALSE, managed_status = '' WHERE mobile_device_id = $1`, id); err != nil {
		return err
	}
	for b, st := range statuses {
		res, err := tx.ExecContext(ctx, `UPDATE mobile_device_apps SET managed = TRUE, managed_status = $3 WHERE mobile_device_id = $1 AND bundle_id = $2`, id, b, st)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// Managed but not (yet) installed, e.g. Installing or Prompting.
			if _, err := tx.ExecContext(ctx, `INSERT INTO mobile_device_apps (mobile_device_id, bundle_id, name, managed, managed_status) VALUES ($1,$2,$2,TRUE,$3)`, id, b, st); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ListMobileApps returns a device's apps.
func (s *Store) ListMobileApps(ctx context.Context, id string) ([]model.MobileDeviceApp, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, version, bundle_id, managed, managed_status FROM mobile_device_apps WHERE mobile_device_id = $1 ORDER BY lower(name)`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.MobileDeviceApp{}
	for rows.Next() {
		var a model.MobileDeviceApp
		if err := rows.Scan(&a.Name, &a.Version, &a.BundleID, &a.Managed, &a.ManagedStatus); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ReplaceMobileProfiles replaces a device's installed profile list (ProfileList).
func (s *Store) ReplaceMobileProfiles(ctx context.Context, id string, profiles []model.ComputerProfile) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM mobile_device_installed_profiles WHERE mobile_device_id = $1`, id); err != nil {
		return err
	}
	for _, p := range profiles {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mobile_device_installed_profiles (mobile_device_id, identifier, uuid, display_name, organization, is_managed) VALUES ($1,$2,$3,$4,$5,$6)`,
			id, p.Identifier, p.UUID, p.DisplayName, p.Organization, p.IsManaged); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListMobileInstalledProfiles returns a device's installed profiles (from ProfileList).
func (s *Store) ListMobileInstalledProfiles(ctx context.Context, id string) ([]model.ComputerProfile, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT identifier, uuid, display_name, organization, is_managed FROM mobile_device_installed_profiles WHERE mobile_device_id = $1 ORDER BY lower(display_name)`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ComputerProfile{}
	for rows.Next() {
		p := model.ComputerProfile{Source: "mdm"}
		if err := rows.Scan(&p.Identifier, &p.UUID, &p.DisplayName, &p.Organization, &p.IsManaged); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// MobileDevicesNeedingInventory returns enrolled devices whose inventory is older than maxAge.
func (s *Store) MobileDevicesNeedingInventory(ctx context.Context, maxAge time.Duration, limit int) ([]model.MobileDevice, error) {
	ids, err := s.deviceIDs(ctx, `SELECT c.id FROM mobile_devices c WHERE c.mdm_enrolled
		AND (c.last_inventory_at IS NULL OR c.last_inventory_at < now() - make_interval(secs => $1))
		AND NOT EXISTS (SELECT 1 FROM mdm_commands_meta m WHERE m.mobile_device_id = c.id AND m.kind = 'inventory'
			AND m.created_at > now() - interval '6 hours') -- an offline device is asked again only every 6 hours
		ORDER BY c.last_inventory_at NULLS FIRST LIMIT $2`,
		maxAge.Seconds(), limit)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	out, _, err := s.ListMobileDevices(ctx, MobileDeviceFilter{IDs: ids, Limit: len(ids)})
	return out, err
}

// ErrNotMobile is returned when a UDID belongs to no mobile device.
var ErrNotMobile = errors.New("not a mobile device")

// ---- mobile device searches ----

// ListMobileSearches returns saved mobile device searches.
func (s *Store) ListMobileSearches(ctx context.Context) ([]model.AdvancedSearch, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+searchCols+` FROM mobile_device_searches ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AdvancedSearch{}
	for rows.Next() {
		a, err := scanSearch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// GetMobileSearch returns one saved mobile device search.
func (s *Store) GetMobileSearch(ctx context.Context, id string) (*model.AdvancedSearch, error) {
	return scanSearch(s.DB.QueryRowContext(ctx, `SELECT `+searchCols+` FROM mobile_device_searches WHERE id = $1`, id))
}

// CreateMobileSearch inserts a saved mobile device search.
func (s *Store) CreateMobileSearch(ctx context.Context, a model.AdvancedSearch) (*model.AdvancedSearch, error) {
	return scanSearch(s.DB.QueryRowContext(ctx, `INSERT INTO mobile_device_searches (name, criteria, display_fields) VALUES ($1,$2::jsonb,$3::jsonb) RETURNING `+searchCols,
		a.Name, jsonArg(a.Criteria), jsonArg(a.DisplayFields)))
}

// UpdateMobileSearch updates a saved mobile device search.
func (s *Store) UpdateMobileSearch(ctx context.Context, a model.AdvancedSearch) (*model.AdvancedSearch, error) {
	return scanSearch(s.DB.QueryRowContext(ctx, `UPDATE mobile_device_searches SET name=$2, criteria=$3::jsonb, display_fields=$4::jsonb, updated_at=now() WHERE id=$1 RETURNING `+searchCols,
		a.ID, a.Name, jsonArg(a.Criteria), jsonArg(a.DisplayFields)))
}

// DeleteMobileSearch removes a saved mobile device search.
func (s *Store) DeleteMobileSearch(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM mobile_device_searches WHERE id=$1`, id)
	return err
}

// RunMobileSearch evaluates criteria and returns matching mobile devices.
func (s *Store) RunMobileSearch(ctx context.Context, criteria []model.Criterion, limit int) ([]model.MobileDevice, error) {
	ids, err := s.PreviewMobileCriteria(ctx, criteria)
	if err != nil || len(ids) == 0 {
		return []model.MobileDevice{}, err
	}
	if limit <= 0 {
		limit = 500
	}
	out, _, err := s.ListMobileDevices(ctx, MobileDeviceFilter{IDs: ids, Limit: limit, Export: limit > 500})
	return out, err
}

// ---- mobile device extension attributes ----

const mobileEACols = `id, name, description, data_type, input_type, popup_choices, created_at, updated_at`

func scanMobileEA(row interface{ Scan(...any) error }) (*model.MobileExtensionAttribute, error) {
	var e model.MobileExtensionAttribute
	var choices db.JSON[[]string]
	if err := row.Scan(&e.ID, &e.Name, &e.Description, &e.DataType, &e.InputType, &choices, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	e.PopupChoices = choices.V
	if e.PopupChoices == nil {
		e.PopupChoices = []string{}
	}
	return &e, nil
}

// ListMobileEAs returns the mobile device extension attributes.
func (s *Store) ListMobileEAs(ctx context.Context) ([]model.MobileExtensionAttribute, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+mobileEACols+` FROM mobile_extension_attributes ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.MobileExtensionAttribute{}
	for rows.Next() {
		e, err := scanMobileEA(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// GetMobileEA returns one mobile extension attribute.
func (s *Store) GetMobileEA(ctx context.Context, id string) (*model.MobileExtensionAttribute, error) {
	return scanMobileEA(s.DB.QueryRowContext(ctx, `SELECT `+mobileEACols+` FROM mobile_extension_attributes WHERE id::text = $1`, id))
}

// SaveMobileEA creates (empty ID) or updates a mobile extension attribute.
func (s *Store) SaveMobileEA(ctx context.Context, e model.MobileExtensionAttribute) (*model.MobileExtensionAttribute, error) {
	if e.PopupChoices == nil {
		e.PopupChoices = []string{}
	}
	if e.ID == "" {
		return scanMobileEA(s.DB.QueryRowContext(ctx, `INSERT INTO mobile_extension_attributes (name, description, data_type, input_type, popup_choices)
			VALUES ($1,$2,$3,$4,$5::jsonb) RETURNING `+mobileEACols, e.Name, e.Description, e.DataType, e.InputType, jsonArg(e.PopupChoices)))
	}
	return scanMobileEA(s.DB.QueryRowContext(ctx, `UPDATE mobile_extension_attributes SET name=$2, description=$3, data_type=$4, input_type=$5,
		popup_choices=$6::jsonb, updated_at=now() WHERE id::text=$1 RETURNING `+mobileEACols, e.ID, e.Name, e.Description, e.DataType, e.InputType, jsonArg(e.PopupChoices)))
}

// DeleteMobileEA removes a mobile extension attribute and its values.
func (s *Store) DeleteMobileEA(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM mobile_extension_attributes WHERE id::text = $1`, id)
	return err
}

// SetMobileEAValue stores one extension attribute value for a device ("" clears it).
func (s *Store) SetMobileEAValue(ctx context.Context, deviceID, eaID, value string) error {
	if value == "" {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM mobile_device_ea_values WHERE mobile_device_id::text = $1 AND ea_id::text = $2`, deviceID, eaID)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO mobile_device_ea_values (mobile_device_id, ea_id, value) VALUES ($1, $2, $3)
		ON CONFLICT (mobile_device_id, ea_id) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, deviceID, eaID, value)
	return err
}

// ListMobileEAValues returns every mobile extension attribute with this device's value ("" if unset).
func (s *Store) ListMobileEAValues(ctx context.Context, deviceID string) ([]model.EAValue, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id, e.name, e.data_type, COALESCE(v.value, ''), COALESCE(v.updated_at, e.updated_at)
		FROM mobile_extension_attributes e LEFT JOIN mobile_device_ea_values v ON v.ea_id = e.id AND v.mobile_device_id::text = $1
		ORDER BY lower(e.name)`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.EAValue{}
	for rows.Next() {
		var v model.EAValue
		if err := rows.Scan(&v.EAID, &v.Name, &v.DataType, &v.Value, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---- commands support ----

// MobileUnlockToken returns the UnlockToken NanoMDM stored for a device (nil if none).
func (s *Store) MobileUnlockToken(ctx context.Context, udid string) ([]byte, error) {
	var tok []byte
	err := s.DB.QueryRowContext(ctx, `SELECT unlock_token FROM devices WHERE id = $1`, udid).Scan(&tok)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return tok, err
}

// SetMobileLocation stores a DeviceLocation answer.
func (s *Store) SetMobileLocation(ctx context.Context, udid string, lat, lon float64, accuracy *float64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE mobile_devices SET last_latitude = $2, last_longitude = $3, last_location_accuracy = $4,
		last_location_at = now(), updated_at = now() WHERE udid = $1`, udid, lat, lon, accuracy)
	return err
}

// SetMobileLostMode records the Lost Mode state after an Enable/DisableLostMode command.
func (s *Store) SetMobileLostMode(ctx context.Context, udid string, on bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE mobile_devices SET lost_mode_enabled = $2, updated_at = now() WHERE udid = $1`, udid, on)
	return err
}

// SaveActivationLockBypass stores a device's Activation Lock bypass code, sealed.
func (s *Store) SaveActivationLockBypass(ctx context.Context, deviceID, code string) error {
	sealed, err := s.Box.Seal(code)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO mobile_device_activation_lock (mobile_device_id, bypass_code_encrypted) VALUES ($1, $2)
		ON CONFLICT (mobile_device_id) DO UPDATE SET bypass_code_encrypted = EXCLUDED.bypass_code_encrypted, collected_at = now()`, deviceID, sealed)
	return err
}

// ActivationLockBypass returns a device's bypass code (ErrNotFound if none).
func (s *Store) ActivationLockBypass(ctx context.Context, deviceID string) (string, error) {
	var sealed string
	if err := s.DB.QueryRowContext(ctx, `SELECT bypass_code_encrypted FROM mobile_device_activation_lock WHERE mobile_device_id::text = $1`, deviceID).Scan(&sealed); err != nil {
		return "", wrapNotFound(err)
	}
	return s.Box.Open(sealed)
}

// ScrubCommandResult replaces a secret in a stored command result (NanoMDM keeps the raw plist),
// once VibeMDM has sealed it elsewhere.
func (s *Store) ScrubCommandResult(ctx context.Context, commandUUID, secret, replacement string) error {
	if secret == "" {
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE command_results SET result = replace(result, $2, $3) WHERE command_uuid = $1`, commandUUID, secret, replacement)
	return err
}

// MobileCommandSentSince reports whether a command of this request type was queued for the device since t.
func (s *Store) MobileCommandSentSince(ctx context.Context, deviceID, requestType string, t time.Time) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM mdm_commands_meta WHERE mobile_device_id::text = $1 AND request_type = $2 AND created_at > $3`,
		deviceID, requestType, t).Scan(&n)
	return n > 0, err
}
