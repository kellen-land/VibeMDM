package store

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

const computerCols = `c.id, COALESCE(c.udid,''), COALESCE(c.serial_number,''), c.name, c.hostname, c.local_hostname, c.model, c.model_name, c.model_identifier,
c.product_name, c.os_version, c.os_build, c.architecture, c.cpu, c.cpu_cores, c.memory_mb, c.disk_total_mb, c.disk_free_mb, c.ip_address, c.mac_address,
c.last_user, c.last_user_full_name, c.filevault_enabled, c.sip_enabled, c.firewall_enabled, c.gatekeeper_enabled, c.secure_boot_level, c.is_supervised,
c.activation_lock_enabled, c.bootstrap_token_escrowed, c.battery_percent, c.uptime_seconds, c.mdm_enrolled, c.mdm_enrolled_at, c.mdm_last_checkin,
c.mdm_unenrolled_at, c.mdm_profile_identifier, c.agent_enrolled, c.agent_enrolled_at, c.agent_last_checkin, c.agent_version, c.enrollment_complete_fired,
c.mdm_device_info, c.security_info, c.hardware, c.last_inventory_at, c.asset_tag, c.site, c.department, c.building, c.room, c.assigned_user,
c.assigned_real_name, c.assigned_email, c.assigned_phone, c.assigned_position, c.notes, c.created_at, c.updated_at`

func scanComputer(row interface{ Scan(...any) error }) (*model.Computer, error) {
	var c model.Computer
	var cores, battery sql.NullInt64
	var mem, diskT, diskF, uptime sql.NullInt64
	var fv, sip, fw, gk, sup, al, bt sql.NullBool
	var mdmEnrAt, mdmLast, mdmUnenr, agEnrAt, agLast, lastInv sql.NullTime
	var devInfo, secInfo, hw db.JSON[map[string]any]
	err := row.Scan(&c.ID, &c.UDID, &c.SerialNumber, &c.Name, &c.Hostname, &c.LocalHostname, &c.Model, &c.ModelName, &c.ModelIdentifier,
		&c.ProductName, &c.OSVersion, &c.OSBuild, &c.Architecture, &c.CPU, &cores, &mem, &diskT, &diskF, &c.IPAddress, &c.MACAddress,
		&c.LastUser, &c.LastUserFullName, &fv, &sip, &fw, &gk, &c.SecureBootLevel, &sup,
		&al, &bt, &battery, &uptime, &c.MDMEnrolled, &mdmEnrAt, &mdmLast,
		&mdmUnenr, &c.MDMProfileIdentifier, &c.AgentEnrolled, &agEnrAt, &agLast, &c.AgentVersion, &c.EnrollmentCompleteFired,
		&devInfo, &secInfo, &hw, &lastInv, &c.AssetTag, &c.Site, &c.Department, &c.Building, &c.Room, &c.AssignedUser,
		&c.AssignedRealName, &c.AssignedEmail, &c.AssignedPhone, &c.AssignedPosition, &c.Notes, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	intp := func(n sql.NullInt64) *int {
		if !n.Valid {
			return nil
		}
		v := int(n.Int64)
		return &v
	}
	int64p := func(n sql.NullInt64) *int64 {
		if !n.Valid {
			return nil
		}
		v := n.Int64
		return &v
	}
	boolp := func(b sql.NullBool) *bool {
		if !b.Valid {
			return nil
		}
		v := b.Bool
		return &v
	}
	c.CPUCores, c.BatteryPercent = intp(cores), intp(battery)
	c.MemoryMB, c.DiskTotalMB, c.DiskFreeMB, c.UptimeSeconds = int64p(mem), int64p(diskT), int64p(diskF), int64p(uptime)
	c.FileVaultEnabled, c.SIPEnabled, c.FirewallEnabled, c.GatekeeperEnabled = boolp(fv), boolp(sip), boolp(fw), boolp(gk)
	c.IsSupervised, c.ActivationLockEnabled, c.BootstrapTokenEscrowed = boolp(sup), boolp(al), boolp(bt)
	c.MDMEnrolledAt, c.MDMLastCheckin, c.MDMUnenrolledAt = db.TimePtr(mdmEnrAt), db.TimePtr(mdmLast), db.TimePtr(mdmUnenr)
	c.AgentEnrolledAt, c.AgentLastCheckin, c.LastInventoryAt = db.TimePtr(agEnrAt), db.TimePtr(agLast), db.TimePtr(lastInv)
	c.MDMDeviceInfo, c.SecurityInfo, c.Hardware = devInfo.V, secInfo.V, hw.V
	if c.MDMDeviceInfo == nil {
		c.MDMDeviceInfo = map[string]any{}
	}
	if c.SecurityInfo == nil {
		c.SecurityInfo = map[string]any{}
	}
	if c.Hardware == nil {
		c.Hardware = map[string]any{}
	}
	return &c, nil
}

// ComputerFilter filters the computer list.
type ComputerFilter struct {
	Query   string
	GroupID string
	Sort    string
	Desc    bool
	Limit   int
	Offset  int
	IDs     []string
}

var computerSortCols = map[string]string{
	"name": "lower(c.name)", "serial_number": "c.serial_number", "os_version": "vibemdm_version_array(c.os_version)",
	"model_name": "c.model_name", "last_user": "c.last_user", "last_checkin": "GREATEST(c.agent_last_checkin, c.mdm_last_checkin)",
	"created_at": "c.created_at", "last_inventory_at": "c.last_inventory_at", "agent_version": "c.agent_version",
}

// ListComputers returns computers matching a filter.
func (s *Store) ListComputers(ctx context.Context, f ComputerFilter) ([]model.Computer, int, error) {
	f.Limit, f.Offset = clampPage(f.Limit, f.Offset)
	var where []string
	var args []any
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, likeArg(q))
		n := len(args)
		where = append(where, fmt.Sprintf(`(c.name ILIKE $%d OR c.serial_number ILIKE $%d OR c.udid ILIKE $%d OR c.last_user ILIKE $%d OR c.model_name ILIKE $%d OR c.assigned_user ILIKE $%d OR c.hostname ILIKE $%d OR c.asset_tag ILIKE $%d OR c.ip_address ILIKE $%d)`, n, n, n, n, n, n, n, n, n))
	}
	if f.GroupID != "" {
		sub, subArgs, err := s.groupMembershipSQL(ctx, computerDomain, f.GroupID, len(args), 0)
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
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM computers c`+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := computerSortCols["name"]
	if col, ok := computerSortCols[f.Sort]; ok {
		order = col
	}
	dir := "ASC"
	if f.Desc {
		dir = "DESC"
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+computerCols+` FROM computers c`+w+fmt.Sprintf(` ORDER BY %s %s NULLS LAST, c.id LIMIT $%d OFFSET $%d`, order, dir, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.Computer{}
	for rows.Next() {
		c, err := scanComputer(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *c)
	}
	return out, total, rows.Err()
}

// GetComputer returns a computer by ID.
func (s *Store) GetComputer(ctx context.Context, id string) (*model.Computer, error) {
	return scanComputer(s.DB.QueryRowContext(ctx, `SELECT `+computerCols+` FROM computers c WHERE c.id = $1`, id))
}

// GetComputerByUDID returns a computer by MDM UDID.
func (s *Store) GetComputerByUDID(ctx context.Context, udid string) (*model.Computer, error) {
	return scanComputer(s.DB.QueryRowContext(ctx, `SELECT `+computerCols+` FROM computers c WHERE c.udid = $1`, udid))
}

// GetComputerByAgentToken returns a computer by agent token hash.
func (s *Store) GetComputerByAgentToken(ctx context.Context, tokenHash string) (*model.Computer, error) {
	return scanComputer(s.DB.QueryRowContext(ctx, `SELECT `+computerCols+` FROM computers c WHERE c.agent_token_hash = $1 AND c.agent_enrolled`, tokenHash))
}

// GetComputerInventoryJSON returns the raw inventory jsonb.
func (s *Store) GetComputerInventoryJSON(ctx context.Context, id string) (map[string]any, error) {
	var inv db.JSON[map[string]any]
	err := s.DB.QueryRowContext(ctx, `SELECT inventory FROM computers WHERE id = $1`, id).Scan(&inv)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	if inv.V == nil {
		inv.V = map[string]any{}
	}
	return inv.V, nil
}

// MDMAuthenticateInfo carries fields from an MDM Authenticate message.
type MDMAuthenticateInfo struct {
	UDID, SerialNumber, DeviceName, Model, ModelName, OSVersion, BuildVersion, ProductName string
}

// UpsertComputerFromAuthenticate creates or updates a computer for an MDM enrollment.
func (s *Store) UpsertComputerFromAuthenticate(ctx context.Context, a MDMAuthenticateInfo) (*model.Computer, error) {
	// Match by UDID first, then serial number (agent may have enrolled first).
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM computers WHERE udid = $1`, a.UDID).Scan(&id)
	if err == sql.ErrNoRows && a.SerialNumber != "" {
		// Merge only into a record no agent has claimed: an agent record without a UDID could have
		// been created by anyone who knows the serial number.
		err = s.DB.QueryRowContext(ctx, `SELECT id FROM computers WHERE serial_number = $1 AND udid IS NULL AND agent_token_hash IS NULL ORDER BY created_at LIMIT 1`, a.SerialNumber).Scan(&id)
	}
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == sql.ErrNoRows {
		err = s.DB.QueryRowContext(ctx, `INSERT INTO computers (udid, serial_number, name, model, model_name, model_identifier, os_version, os_build, product_name)
			VALUES ($1, NULLIF($2,''), $3, $4, $5, $4, $6, $7, $8) RETURNING id`,
			a.UDID, a.SerialNumber, a.DeviceName, a.Model, a.ModelName, a.OSVersion, a.BuildVersion, a.ProductName).Scan(&id)
		if err != nil {
			return nil, err
		}
	} else {
		_, err = s.DB.ExecContext(ctx, `UPDATE computers SET udid = $2, serial_number = COALESCE(NULLIF($3,''), serial_number),
			name = CASE WHEN $4 <> '' THEN $4 ELSE name END, model = CASE WHEN $5 <> '' THEN $5 ELSE model END,
			model_identifier = CASE WHEN $5 <> '' THEN $5 ELSE model_identifier END,
			model_name = CASE WHEN $6 <> '' THEN $6 ELSE model_name END, os_version = CASE WHEN $7 <> '' THEN $7 ELSE os_version END,
			os_build = CASE WHEN $8 <> '' THEN $8 ELSE os_build END, product_name = CASE WHEN $9 <> '' THEN $9 ELSE product_name END,
			updated_at = now() WHERE id = $1`,
			id, a.UDID, a.SerialNumber, a.DeviceName, a.Model, a.ModelName, a.OSVersion, a.BuildVersion, a.ProductName)
		if err != nil {
			return nil, err
		}
	}
	return s.GetComputer(ctx, id)
}

// MarkMDMEnrolled flags the computer as MDM-enrolled (after TokenUpdate).
func (s *Store) MarkMDMEnrolled(ctx context.Context, udid string) (*model.Computer, bool, error) {
	var id string
	var wasEnrolled bool
	err := s.DB.QueryRowContext(ctx, `UPDATE computers SET mdm_enrolled = TRUE, mdm_enrolled_at = CASE WHEN mdm_enrolled THEN mdm_enrolled_at ELSE now() END,
		mdm_last_checkin = now(), mdm_unenrolled_at = NULL, updated_at = now() WHERE udid = $1
		RETURNING id, (mdm_enrolled_at < now() - interval '1 second')`, udid).Scan(&id, &wasEnrolled)
	if err != nil {
		return nil, false, wrapNotFound(err)
	}
	c, err := s.GetComputer(ctx, id)
	return c, wasEnrolled, err
}

// MarkMDMUnenrolled flags the computer as not MDM-enrolled (CheckOut).
func (s *Store) MarkMDMUnenrolled(ctx context.Context, udid string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE computers SET mdm_enrolled = FALSE, mdm_unenrolled_at = now(), updated_at = now() WHERE udid = $1`, udid)
	return err
}

// TouchMDMCheckin updates the MDM last check-in time.
func (s *Store) TouchMDMCheckin(ctx context.Context, udid string) {
	s.DB.ExecContext(ctx, `UPDATE computers SET mdm_last_checkin = now() WHERE udid = $1`, udid)
	s.DB.ExecContext(ctx, `UPDATE mobile_devices SET mdm_last_checkin = now() WHERE udid = $1`, udid)
}

// ErrAgentActive is returned when an enrollment would replace a computer's active agent.
var ErrAgentActive = errors.New("this computer already has an active agent; reset the agent on the computer's page in the console first")

// agentReplaceAfter is how long an agent must be silent before a new enrollment may take over its record.
const agentReplaceAfter = 30 * 24 * time.Hour

// EnrollAgent creates or updates a computer for an agent enrollment and stores the token hash.
//
// UDIDs and serial numbers are not secrets, so an enrollment may only take over
// an existing record whose agent is still active if it proves possession of
// that agent's device token (a reinstall on the same Mac). Otherwise an admin
// resets the agent first, or the old agent must have been silent for 30 days.
func (s *Store) EnrollAgent(ctx context.Context, req model.AgentEnrollRequest, tokenHash string) (*model.Computer, error) {
	var id string
	var curHash sql.NullString
	var last sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT id, agent_token_hash, agent_last_checkin FROM computers WHERE udid = $1`, req.UDID).Scan(&id, &curHash, &last)
	if err == sql.ErrNoRows && req.SerialNumber != "" {
		// Serial numbers are printed on the box: only fall back to them for records that have no
		// UDID yet, so an agent can never attach itself to another Mac's MDM record by serial.
		err = s.DB.QueryRowContext(ctx, `SELECT id, agent_token_hash, agent_last_checkin FROM computers WHERE serial_number = $1 AND udid IS NULL ORDER BY created_at LIMIT 1`,
			req.SerialNumber).Scan(&id, &curHash, &last)
	}
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil && curHash.Valid && curHash.String != "" {
		proved := req.PreviousDeviceToken != "" && subtle.ConstantTimeCompare([]byte(HashToken(req.PreviousDeviceToken)), []byte(curHash.String)) == 1
		stale := !last.Valid || time.Since(last.Time) > agentReplaceAfter
		if !proved && !stale {
			return nil, ErrAgentActive
		}
	}
	name := req.ComputerName
	if name == "" {
		name = req.Hostname
	}
	if err == sql.ErrNoRows {
		err = s.DB.QueryRowContext(ctx, `INSERT INTO computers (udid, serial_number, name, hostname, os_version, architecture, agent_enrolled, agent_enrolled_at, agent_last_checkin, agent_version, agent_token_hash)
			VALUES (NULLIF($1,''), NULLIF($2,''), $3, $4, $5, $6, TRUE, now(), now(), $7, $8) RETURNING id`,
			req.UDID, req.SerialNumber, name, req.Hostname, req.OSVersion, req.Architecture, req.AgentVersion, tokenHash).Scan(&id)
	} else {
		_, err = s.DB.ExecContext(ctx, `UPDATE computers SET udid = COALESCE(udid, NULLIF($2,'')), serial_number = COALESCE(NULLIF($3,''), serial_number),
			name = CASE WHEN $4 <> '' THEN $4 ELSE name END, hostname = $5, os_version = CASE WHEN $6 <> '' THEN $6 ELSE os_version END,
			architecture = CASE WHEN $7 <> '' THEN $7 ELSE architecture END, agent_enrolled = TRUE, agent_enrolled_at = now(), agent_last_checkin = now(),
			agent_version = $8, agent_token_hash = $9, enrollment_complete_fired = FALSE, updated_at = now() WHERE id = $1`,
			id, req.UDID, req.SerialNumber, name, req.Hostname, req.OSVersion, req.Architecture, req.AgentVersion, tokenHash)
	}
	if err != nil {
		return nil, err
	}
	return s.GetComputer(ctx, id)
}

// UnenrollAgent clears the agent enrollment.
func (s *Store) UnenrollAgent(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE computers SET agent_enrolled = FALSE, agent_token_hash = NULL, updated_at = now() WHERE id = $1`, id)
	return err
}

// TouchAgentCheckin updates light inventory fields on each check-in.
func (s *Store) TouchAgentCheckin(ctx context.Context, id string, r model.AgentCheckinRequest) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE computers SET agent_last_checkin = now(),
		last_user = CASE WHEN $2 <> '' THEN $2 ELSE last_user END, hostname = CASE WHEN $3 <> '' THEN $3 ELSE hostname END,
		name = CASE WHEN $4 <> '' THEN $4 ELSE name END, os_version = CASE WHEN $5 <> '' THEN $5 ELSE os_version END,
		os_build = CASE WHEN $6 <> '' THEN $6 ELSE os_build END, ip_address = CASE WHEN $7 <> '' THEN $7 ELSE ip_address END,
		agent_version = CASE WHEN $8 <> '' THEN $8 ELSE agent_version END, uptime_seconds = CASE WHEN $9 > 0 THEN $9 ELSE uptime_seconds END,
		agent_capabilities = $10::jsonb, updated_at = now() WHERE id = $1`,
		id, r.ConsoleUser, r.Hostname, r.ComputerName, r.OSVersion, r.OSBuild, r.IPAddress, r.AgentVersion, r.UptimeSecs, jsonArg(nonNilStrings(r.Capabilities)))
	if err == nil && r.IPAddress != "" {
		s.ApplySegmentAssignments(ctx, id, r.IPAddress)
	}
	return err
}

// AgentCapabilities returns the features the computer's agent reported at its last check-in.
func (s *Store) AgentCapabilities(ctx context.Context, id string) ([]string, error) {
	var caps db.JSON[[]string]
	if err := s.DB.QueryRowContext(ctx, `SELECT agent_capabilities FROM computers WHERE id = $1`, id).Scan(&caps); err != nil {
		return nil, wrapNotFound(err)
	}
	return caps.V, nil
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// RecordPolicyRefusal logs a policy the server would not hand to a computer, at most once a day
// per policy and computer so ongoing policies do not fill the log.
func (s *Store) RecordPolicyRefusal(ctx context.Context, policyID, computerID, trigger, reason string) error {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM policy_logs WHERE policy_id = $1 AND computer_id = $2 AND status = 'failed'
		AND log = $3 AND created_at > now() - interval '1 day'`, policyID, computerID, reason).Scan(&n); err != nil || n > 0 {
		return err
	}
	now := time.Now().UTC()
	return s.InsertPolicyLog(ctx, policyID, computerID, model.AgentPolicyLogSubmit{Trigger: trigger, Status: "failed", StartedAt: now, CompletedAt: now, Log: reason,
		Steps: []model.PolicyStep{{Type: "agent", Name: "Agent version", Status: "failed", ExitCode: 1, Output: reason}}})
}

// MarkEnrollmentCompleteFired records that the enrollment_complete trigger ran.
func (s *Store) MarkEnrollmentCompleteFired(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE computers SET enrollment_complete_fired = TRUE WHERE id = $1`, id)
	return err
}

// UpdateComputerFields updates admin-editable fields.
type ComputerEdit struct {
	Name             *string `json:"name"`
	AssetTag         *string `json:"asset_tag"`
	Site             *string `json:"site"`
	Department       *string `json:"department"`
	Building         *string `json:"building"`
	Room             *string `json:"room"`
	AssignedUser     *string `json:"assigned_user"`
	AssignedRealName *string `json:"assigned_real_name"`
	AssignedEmail    *string `json:"assigned_email"`
	AssignedPhone    *string `json:"assigned_phone"`
	AssignedPosition *string `json:"assigned_position"`
	Notes            *string `json:"notes"`
}

// UpdateComputer applies an edit.
func (s *Store) UpdateComputer(ctx context.Context, id string, e ComputerEdit) error {
	sets := []string{"updated_at = now()"}
	args := []any{id}
	add := func(col string, v *string) {
		if v != nil {
			args = append(args, *v)
			sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
		}
	}
	add("name", e.Name)
	add("asset_tag", e.AssetTag)
	add("site", e.Site)
	add("department", e.Department)
	add("building", e.Building)
	add("room", e.Room)
	add("assigned_user", e.AssignedUser)
	add("assigned_real_name", e.AssignedRealName)
	add("assigned_email", e.AssignedEmail)
	add("assigned_phone", e.AssignedPhone)
	add("assigned_position", e.AssignedPosition)
	add("notes", e.Notes)
	res, err := s.DB.ExecContext(ctx, `UPDATE computers SET `+strings.Join(sets, ", ")+` WHERE id = $1`, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteComputer removes a computer and (if enrolled) the NanoMDM enrollment rows.
func (s *Store) DeleteComputer(ctx context.Context, id string) error {
	var udid sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT udid FROM computers WHERE id = $1`, id).Scan(&udid); err != nil {
		return wrapNotFound(err)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if udid.Valid {
		// NanoMDM tables: devices cascades to enrollments, queue, results.
		if _, err := tx.ExecContext(ctx, `DELETE FROM devices WHERE id = $1`, udid.String); err != nil {
			return err
		}
		if err := revokeEnrollmentCerts(ctx, tx, udid.String); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM computers WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ApplyMDMDeviceInformation merges a DeviceInformation QueryResponses dict.
func (s *Store) ApplyMDMDeviceInformation(ctx context.Context, udid string, q map[string]any) error {
	str := func(k string) string {
		if v, ok := q[k]; ok {
			switch t := v.(type) {
			case string:
				return t
			case fmt.Stringer:
				return t.String()
			default:
				return fmt.Sprint(t)
			}
		}
		return ""
	}
	boolp := func(k string) sql.NullBool {
		if v, ok := q[k].(bool); ok {
			return sql.NullBool{Bool: v, Valid: true}
		}
		return sql.NullBool{}
	}
	num := func(k string) sql.NullInt64 {
		switch v := q[k].(type) {
		case int64:
			return sql.NullInt64{Int64: v, Valid: true}
		case uint64:
			return sql.NullInt64{Int64: int64(v), Valid: true}
		case int:
			return sql.NullInt64{Int64: int64(v), Valid: true}
		case float64:
			return sql.NullInt64{Int64: int64(v), Valid: true}
		}
		return sql.NullInt64{}
	}
	// DeviceCapacity / AvailableDeviceCapacity are in GB (float).
	gbToMB := func(k string) sql.NullInt64 {
		if v, ok := q[k].(float64); ok {
			return sql.NullInt64{Int64: int64(v * 1024), Valid: true}
		}
		return sql.NullInt64{}
	}
	var battery sql.NullInt64
	if v, ok := q["BatteryLevel"].(float64); ok && v >= 0 {
		battery = sql.NullInt64{Int64: int64(v * 100), Valid: true}
	}
	_ = num
	_, err := s.DB.ExecContext(ctx, `UPDATE computers SET
		name = CASE WHEN $2 <> '' THEN $2 ELSE name END,
		os_version = CASE WHEN $3 <> '' THEN $3 ELSE os_version END,
		os_build = CASE WHEN $4 <> '' THEN $4 ELSE os_build END,
		model_name = CASE WHEN $5 <> '' THEN $5 ELSE model_name END,
		model = CASE WHEN $6 <> '' THEN $6 ELSE model END,
		model_identifier = CASE WHEN $6 <> '' THEN $6 ELSE model_identifier END,
		product_name = CASE WHEN $7 <> '' THEN $7 ELSE product_name END,
		serial_number = COALESCE(NULLIF($8,''), serial_number),
		disk_total_mb = COALESCE($9, disk_total_mb), disk_free_mb = COALESCE($10, disk_free_mb),
		is_supervised = COALESCE($11, is_supervised), activation_lock_enabled = COALESCE($12, activation_lock_enabled),
		local_hostname = CASE WHEN $13 <> '' THEN $13 ELSE local_hostname END, hostname = CASE WHEN $14 <> '' THEN $14 ELSE hostname END,
		mac_address = CASE WHEN $15 <> '' THEN $15 ELSE mac_address END, battery_percent = COALESCE($16, battery_percent),
		mdm_device_info = $17::jsonb, mdm_last_checkin = now(), last_inventory_at = now(), updated_at = now()
		WHERE udid = $1`,
		udid, str("DeviceName"), str("OSVersion"), str("BuildVersion"), str("ModelName"), str("Model"), str("ProductName"), str("SerialNumber"),
		gbToMB("DeviceCapacity"), gbToMB("AvailableDeviceCapacity"), boolp("IsSupervised"), boolp("IsActivationLockEnabled"),
		str("LocalHostName"), str("HostName"), firstNonEmpty(str("EthernetMAC"), str("WiFiMAC")), battery, jsonArg(q))
	return err
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ApplyMDMSecurityInfo stores a SecurityInfo response.
func (s *Store) ApplyMDMSecurityInfo(ctx context.Context, udid string, sec map[string]any) error {
	var fv, sip, fw, bt sql.NullBool
	if v, ok := sec["FDE_Enabled"].(bool); ok {
		fv = sql.NullBool{Bool: v, Valid: true}
	}
	if m, ok := sec["FirewallSettings"].(map[string]any); ok {
		if v, ok := m["FirewallEnabled"].(bool); ok {
			fw = sql.NullBool{Bool: v, Valid: true}
		}
	}
	if m, ok := sec["SecureBoot"].(map[string]any); ok {
		_ = m
	}
	// Not escrow: BootstrapTokenAllowedForAuthentication says whether the Secure Enclave lets the
	// bootstrap token authorize updates and kernel extensions ("disallowed" is normal without ADE).
	// Escrow is known only from SetBootstrapToken (see SetBootstrapTokenEscrowed); bt stays NULL here.
	if v, ok := sec["SystemIntegrityProtectionEnabled"].(bool); ok {
		sip = sql.NullBool{Bool: v, Valid: true}
	}
	level := ""
	if m, ok := sec["SecureBoot"].(map[string]any); ok {
		if v, ok := m["SecureBootLevel"].(string); ok {
			level = v
		}
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE computers SET filevault_enabled = COALESCE($2, filevault_enabled), sip_enabled = COALESCE($3, sip_enabled),
		firewall_enabled = COALESCE($4, firewall_enabled), bootstrap_token_escrowed = COALESCE($5, bootstrap_token_escrowed),
		secure_boot_level = CASE WHEN $6 <> '' THEN $6 ELSE secure_boot_level END, security_info = $7::jsonb, mdm_last_checkin = now(), updated_at = now()
		WHERE udid = $1`, udid, fv, sip, fw, bt, level, jsonArg(sec))
	return err
}

// ReplaceComputerApps replaces the app list for a computer from a source.
func (s *Store) ReplaceComputerApps(ctx context.Context, computerID, source string, apps []model.ComputerApp) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM computer_apps WHERE computer_id = $1 AND source = $2`, computerID, source); err != nil {
		return err
	}
	for _, a := range apps {
		if a.Name == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO computer_apps (computer_id, name, version, bundle_id, path, source) VALUES ($1,$2,$3,$4,$5,$6)`,
			computerID, a.Name, a.Version, a.BundleID, a.Path, source); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReplaceComputerProfiles replaces installed profiles for a computer from a source.
func (s *Store) ReplaceComputerProfiles(ctx context.Context, computerID, source string, profiles []model.ComputerProfile) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM computer_profiles WHERE computer_id = $1 AND source = $2`, computerID, source); err != nil {
		return err
	}
	for _, p := range profiles {
		if p.Identifier == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO computer_profiles (computer_id, identifier, uuid, display_name, organization, is_managed, source) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			computerID, p.Identifier, p.UUID, p.DisplayName, p.Organization, p.IsManaged, source); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListComputerApps lists apps.
func (s *Store) ListComputerApps(ctx context.Context, computerID string) ([]model.ComputerApp, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, version, bundle_id, path, source FROM computer_apps WHERE computer_id = $1 ORDER BY lower(name)`, computerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ComputerApp{}
	for rows.Next() {
		var a model.ComputerApp
		if err := rows.Scan(&a.Name, &a.Version, &a.BundleID, &a.Path, &a.Source); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// currentProfileSource limits computer_profiles rows (alias a) to the source whose report is newest.
func currentProfileSource(a string) string {
	return a + `.source = (SELECT n.source FROM computer_profiles n WHERE n.computer_id = ` + a + `.computer_id ORDER BY n.reported_at DESC LIMIT 1)`
}

// ListComputerProfiles lists installed profiles.
func (s *Store) ListComputerProfiles(ctx context.Context, computerID string) ([]model.ComputerProfile, error) {
	// The agent and MDM report the installed profiles separately; show the most recent report
	// once (an older one may still list removed profiles), managed if either source says so.
	rows, err := s.DB.QueryContext(ctx, `SELECT p.identifier, p.uuid, p.display_name, p.organization,
		p.is_managed OR EXISTS (SELECT 1 FROM computer_profiles o WHERE o.computer_id = p.computer_id AND o.identifier = p.identifier AND o.is_managed),
		p.source
		FROM computer_profiles p WHERE p.computer_id = $1 AND `+currentProfileSource("p")+`
		ORDER BY lower(p.display_name), p.identifier`, computerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ComputerProfile{}
	for rows.Next() {
		var p model.ComputerProfile
		if err := rows.Scan(&p.Identifier, &p.UUID, &p.DisplayName, &p.Organization, &p.IsManaged, &p.Source); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListComputerLocalUsers lists local accounts.
func (s *Store) ListComputerLocalUsers(ctx context.Context, computerID string) ([]model.LocalUser, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT username, uid, real_name, home, is_admin, secure_token FROM computer_local_users WHERE computer_id = $1 ORDER BY username`, computerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.LocalUser{}
	for rows.Next() {
		var u model.LocalUser
		var uid sql.NullInt64
		var st sql.NullBool
		if err := rows.Scan(&u.Username, &uid, &u.RealName, &u.Home, &u.IsAdmin, &st); err != nil {
			return nil, err
		}
		if uid.Valid {
			v := int(uid.Int64)
			u.UID = &v
		}
		if st.Valid {
			v := st.Bool
			u.SecureToken = &v
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ListComputerEAValues lists extension attribute values.
func (s *Store) ListComputerEAValues(ctx context.Context, computerID string) ([]model.EAValue, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id, e.name, e.data_type, COALESCE(v.value,''), COALESCE(v.updated_at, e.created_at)
		FROM extension_attributes e LEFT JOIN computer_ea_values v ON v.ea_id = e.id AND v.computer_id = $1 WHERE e.enabled ORDER BY e.name`, computerID)
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

// ApplyAgentInventory stores a full agent inventory report.
func (s *Store) ApplyAgentInventory(ctx context.Context, computerID string, inv model.AgentInventory) error {
	nb := func(b *bool) sql.NullBool {
		if b == nil {
			return sql.NullBool{}
		}
		return sql.NullBool{Bool: *b, Valid: true}
	}
	var battery sql.NullInt64
	if inv.BatteryPercent != nil {
		battery = sql.NullInt64{Int64: int64(*inv.BatteryPercent), Valid: true}
	}
	nz := func(v int64) sql.NullInt64 {
		if v <= 0 {
			return sql.NullInt64{}
		}
		return sql.NullInt64{Int64: v, Valid: true}
	}
	if inv.Hardware == nil {
		inv.Hardware = map[string]any{}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE computers SET
		name = CASE WHEN $2 <> '' THEN $2 ELSE name END, hostname = CASE WHEN $3 <> '' THEN $3 ELSE hostname END,
		local_hostname = CASE WHEN $4 <> '' THEN $4 ELSE local_hostname END, serial_number = COALESCE(NULLIF($5,''), serial_number),
		udid = COALESCE(udid, NULLIF($6,'')), model_identifier = CASE WHEN $7 <> '' THEN $7 ELSE model_identifier END,
		model = CASE WHEN $7 <> '' THEN $7 ELSE model END, model_name = CASE WHEN $8 <> '' THEN $8 ELSE model_name END,
		os_version = CASE WHEN $9 <> '' THEN $9 ELSE os_version END, os_build = CASE WHEN $10 <> '' THEN $10 ELSE os_build END,
		architecture = CASE WHEN $11 <> '' THEN $11 ELSE architecture END, cpu = CASE WHEN $12 <> '' THEN $12 ELSE cpu END,
		cpu_cores = COALESCE($13, cpu_cores), memory_mb = COALESCE($14, memory_mb), disk_total_mb = COALESCE($15, disk_total_mb), disk_free_mb = COALESCE($16, disk_free_mb),
		ip_address = CASE WHEN $17 <> '' THEN $17 ELSE ip_address END, mac_address = CASE WHEN $18 <> '' THEN $18 ELSE mac_address END,
		last_user = CASE WHEN $19 <> '' THEN $19 ELSE last_user END, last_user_full_name = CASE WHEN $20 <> '' THEN $20 ELSE last_user_full_name END,
		filevault_enabled = COALESCE($21, filevault_enabled), sip_enabled = COALESCE($22, sip_enabled), firewall_enabled = COALESCE($23, firewall_enabled),
		gatekeeper_enabled = COALESCE($24, gatekeeper_enabled), secure_boot_level = CASE WHEN $25 <> '' THEN $25 ELSE secure_boot_level END,
		battery_percent = COALESCE($26, battery_percent), uptime_seconds = COALESCE($27, uptime_seconds),
		agent_version = CASE WHEN $28 <> '' THEN $28 ELSE agent_version END, hardware = $29::jsonb, inventory = $30::jsonb,
		last_inventory_at = now(), agent_last_checkin = now(), updated_at = now() WHERE id = $1`,
		computerID, inv.ComputerName, inv.Hostname, inv.LocalHostname, inv.SerialNumber, inv.UDID, inv.ModelIdentifier, inv.ModelName,
		inv.OSVersion, inv.OSBuild, inv.Architecture, inv.CPU, nz(int64(inv.CPUCores)), nz(inv.MemoryMB), nz(inv.DiskTotalMB), nz(inv.DiskFreeMB),
		inv.IPAddress, inv.MACAddress, firstNonEmpty(inv.ConsoleUser, inv.LastUser), inv.ConsoleUserFullName,
		nb(inv.FileVaultEnabled), nb(inv.SIPEnabled), nb(inv.FirewallEnabled), nb(inv.GatekeeperEnabled), inv.SecureBootLevel,
		battery, nz(inv.UptimeSeconds), inv.AgentVersion, jsonArg(inv.Hardware), jsonArg(inv))
	if err != nil {
		return err
	}
	// Batch replace child rows with single statements (jsonb_to_recordset) so a
	// 500-app inventory is a handful of round-trips instead of hundreds.
	if _, err := tx.ExecContext(ctx, `DELETE FROM computer_apps WHERE computer_id = $1 AND source IN ('agent','mdm')`, computerID); err != nil {
		return err
	}
	if len(inv.Apps) > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO computer_apps (computer_id, name, version, bundle_id, path, source)
			SELECT $1, name, COALESCE(version,''), COALESCE(bundle_id,''), COALESCE(path,''), 'agent'
			FROM jsonb_to_recordset($2::jsonb) AS x(name TEXT, version TEXT, bundle_id TEXT, path TEXT) WHERE COALESCE(name,'') <> ''`,
			computerID, jsonArg(inv.Apps)); err != nil {
			return err
		}
	}
	if len(inv.Profiles) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM computer_profiles WHERE computer_id = $1 AND source = 'agent'`, computerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO computer_profiles (computer_id, identifier, uuid, display_name, organization, is_managed, source)
			SELECT $1, identifier, COALESCE(uuid,''), COALESCE(display_name,''), COALESCE(organization,''), COALESCE(is_managed,false), 'agent'
			FROM jsonb_to_recordset($2::jsonb) AS x(identifier TEXT, uuid TEXT, display_name TEXT, organization TEXT, is_managed BOOLEAN) WHERE COALESCE(identifier,'') <> ''`,
			computerID, jsonArg(inv.Profiles)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM computer_local_users WHERE computer_id = $1`, computerID); err != nil {
		return err
	}
	if len(inv.LocalUsers) > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO computer_local_users (computer_id, username, uid, real_name, home, is_admin, secure_token)
			SELECT $1, username, uid, COALESCE(real_name,''), COALESCE(home,''), COALESCE(is_admin,false), secure_token
			FROM jsonb_to_recordset($2::jsonb) AS x(username TEXT, uid INTEGER, real_name TEXT, home TEXT, is_admin BOOLEAN, secure_token BOOLEAN) WHERE COALESCE(username,'') <> ''`,
			computerID, jsonArg(inv.LocalUsers)); err != nil {
			return err
		}
	}
	if len(inv.ExtensionAttributes) > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO computer_ea_values (computer_id, ea_id, value, updated_at)
			SELECT $1, e.id, x.value, now() FROM jsonb_each_text($2::jsonb) AS x(key, value) JOIN extension_attributes e ON e.id::text = x.key
			WHERE e.input_type = 'script' AND e.enabled
			ON CONFLICT (computer_id, ea_id) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, computerID, jsonArg(inv.ExtensionAttributes)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ComputerGroups returns the groups (static and smart) a computer belongs to.
func (s *Store) ComputerGroups(ctx context.Context, computerID string) ([]model.ComputerGroup, error) {
	groups, err := s.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := []model.ComputerGroup{}
	for _, g := range groups {
		in, err := s.IsGroupMember(ctx, g.ID, computerID)
		if err != nil {
			return nil, err
		}
		if in {
			out = append(out, g)
		}
	}
	return out, nil
}

// SetInventoryRequested is a helper used by UpdateInventory flows: nothing to persist, inventory is requested via tasks.
var _ = time.Now

// SetBootstrapTokenEscrowed records whether the Mac has escrowed its bootstrap token (SetBootstrapToken).
func (s *Store) SetBootstrapTokenEscrowed(ctx context.Context, udid string, escrowed bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE computers SET bootstrap_token_escrowed = $2, updated_at = now() WHERE udid = $1`, udid, escrowed)
	return err
}
