// Package model defines the domain types shared by the store, API, and agent protocol.
package model

import (
	"encoding/json"
	"time"
)

// Scope describes which computers an object (policy, profile) applies to.
// The structure mirrors Jamf Pro's scoping model: Targets select computers,
// Limitations narrow the targets, and Exclusions always win.
//
//	in scope  =  target  AND  passes limitations  AND NOT  excluded
type Scope struct {
	AllComputers bool             `json:"all_computers"`
	Targets      ScopeSelection   `json:"targets"`
	Limitations  ScopeLimitations `json:"limitations"`
	Exclusions   ScopeSelection   `json:"exclusions"`
}

// ScopeSelection names the objects a scope section refers to.
// Users and NetworkSegments are honored in Exclusions (and, for limitations,
// via ScopeLimitations); device targeting stays computer-centric like Jamf's.
type ScopeSelection struct {
	ComputerIDs     []string `json:"computer_ids"`
	GroupIDs        []string `json:"group_ids"`
	Buildings       []string `json:"buildings"`        // building names
	Departments     []string `json:"departments"`      // department names
	NetworkSegments []string `json:"network_segments"` // network segment IDs
	Users           []string `json:"users"`            // usernames (exclusions only)
}

// IsEmpty reports whether the selection names nothing.
func (s ScopeSelection) IsEmpty() bool {
	return len(s.ComputerIDs)+len(s.GroupIDs)+len(s.Buildings)+len(s.Departments)+len(s.NetworkSegments)+len(s.Users) == 0
}

// ScopeLimitations narrow the targets: a computer stays in scope only if it
// matches every limitation category that is set (categories AND together,
// values within a category OR together).
type ScopeLimitations struct {
	Users           []string `json:"users"`            // console/assigned usernames
	NetworkSegments []string `json:"network_segments"` // network segment IDs
}

// IsEmpty reports whether no limitations are set.
func (l ScopeLimitations) IsEmpty() bool {
	return len(l.Users)+len(l.NetworkSegments) == 0
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func (s *ScopeSelection) normalize() {
	s.ComputerIDs, s.GroupIDs, s.Buildings = nonNil(s.ComputerIDs), nonNil(s.GroupIDs), nonNil(s.Buildings)
	s.Departments, s.NetworkSegments, s.Users = nonNil(s.Departments), nonNil(s.NetworkSegments), nonNil(s.Users)
}

// Normalize ensures all slices are non-nil for JSON output.
func (s *Scope) Normalize() {
	s.Targets.normalize()
	s.Exclusions.normalize()
	s.Limitations.Users = nonNil(s.Limitations.Users)
	s.Limitations.NetworkSegments = nonNil(s.Limitations.NetworkSegments)
}

// UnmarshalJSON accepts both the current nested shape and the legacy flat
// shape ({computer_ids, group_ids, excluded_computer_ids, excluded_group_ids})
// so stored scopes and older API clients keep working.
func (s *Scope) UnmarshalJSON(b []byte) error {
	type alias Scope
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err == nil {
		legacy := func(key string, dst *[]string) {
			if v, ok := raw[key]; ok && len(*dst) == 0 {
				var vals []string
				if json.Unmarshal(v, &vals) == nil && len(vals) > 0 {
					*dst = vals
				}
			}
		}
		legacy("computer_ids", &a.Targets.ComputerIDs)
		legacy("group_ids", &a.Targets.GroupIDs)
		legacy("excluded_computer_ids", &a.Exclusions.ComputerIDs)
		legacy("excluded_group_ids", &a.Exclusions.GroupIDs)
	}
	*s = Scope(a)
	s.Normalize()
	return nil
}

// Building is a managed location (network organization object).
type Building struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Computers int       `json:"computers"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Department is a managed organizational unit.
type Department struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Computers int       `json:"computers"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NetworkSegment is an inclusive IP range computers are matched against by
// their last reported address. Like Jamf, a segment may auto-assign a
// building/department to computers seen inside it.
type NetworkSegment struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	StartingAddress  string    `json:"starting_address"`
	EndingAddress    string    `json:"ending_address"`
	AssignBuilding   string    `json:"assign_building"`
	AssignDepartment string    `json:"assign_department"`
	Computers        int       `json:"computers"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Computer is the unified record for a managed Mac.
type Computer struct {
	ID                      string         `json:"id"`
	UDID                    string         `json:"udid"`
	SerialNumber            string         `json:"serial_number"`
	Name                    string         `json:"name"`
	Hostname                string         `json:"hostname"`
	LocalHostname           string         `json:"local_hostname"`
	Model                   string         `json:"model"`
	ModelName               string         `json:"model_name"`
	ModelIdentifier         string         `json:"model_identifier"`
	ProductName             string         `json:"product_name"`
	OSVersion               string         `json:"os_version"`
	OSBuild                 string         `json:"os_build"`
	Architecture            string         `json:"architecture"`
	CPU                     string         `json:"cpu"`
	CPUCores                *int           `json:"cpu_cores"`
	MemoryMB                *int64         `json:"memory_mb"`
	DiskTotalMB             *int64         `json:"disk_total_mb"`
	DiskFreeMB              *int64         `json:"disk_free_mb"`
	IPAddress               string         `json:"ip_address"`
	MACAddress              string         `json:"mac_address"`
	LastUser                string         `json:"last_user"`
	LastUserFullName        string         `json:"last_user_full_name"`
	FileVaultEnabled        *bool          `json:"filevault_enabled"`
	SIPEnabled              *bool          `json:"sip_enabled"`
	FirewallEnabled         *bool          `json:"firewall_enabled"`
	GatekeeperEnabled       *bool          `json:"gatekeeper_enabled"`
	SecureBootLevel         string         `json:"secure_boot_level"`
	IsSupervised            *bool          `json:"is_supervised"`
	ActivationLockEnabled   *bool          `json:"activation_lock_enabled"`
	BootstrapTokenEscrowed  *bool          `json:"bootstrap_token_escrowed"`
	BatteryPercent          *int           `json:"battery_percent"`
	UptimeSeconds           *int64         `json:"uptime_seconds"`
	MDMEnrolled             bool           `json:"mdm_enrolled"`
	MDMEnrolledAt           *time.Time     `json:"mdm_enrolled_at"`
	MDMLastCheckin          *time.Time     `json:"mdm_last_checkin"`
	MDMUnenrolledAt         *time.Time     `json:"mdm_unenrolled_at"`
	MDMProfileIdentifier    string         `json:"mdm_profile_identifier"`
	AgentEnrolled           bool           `json:"agent_enrolled"`
	AgentEnrolledAt         *time.Time     `json:"agent_enrolled_at"`
	AgentLastCheckin        *time.Time     `json:"agent_last_checkin"`
	AgentVersion            string         `json:"agent_version"`
	EnrollmentCompleteFired bool           `json:"-"`
	MDMDeviceInfo           map[string]any `json:"mdm_device_info"`
	SecurityInfo            map[string]any `json:"security_info"`
	Hardware                map[string]any `json:"hardware"`
	Inventory               map[string]any `json:"inventory,omitempty"`
	LastInventoryAt         *time.Time     `json:"last_inventory_at"`
	AssetTag                string         `json:"asset_tag"`
	Site                    string         `json:"site"`
	Department              string         `json:"department"`
	Building                string         `json:"building"`
	Room                    string         `json:"room"`
	AssignedUser            string         `json:"assigned_user"`
	AssignedRealName        string         `json:"assigned_real_name"`
	AssignedEmail           string         `json:"assigned_email"`
	AssignedPhone           string         `json:"assigned_phone"`
	AssignedPosition        string         `json:"assigned_position"`
	Notes                   string         `json:"notes"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
}

// ComputerApp is an installed application.
type ComputerApp struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	BundleID string `json:"bundle_id"`
	Path     string `json:"path"`
	Source   string `json:"source"`
}

// ComputerProfile is an installed configuration profile.
type ComputerProfile struct {
	Identifier   string `json:"identifier"`
	UUID         string `json:"uuid"`
	DisplayName  string `json:"display_name"`
	Organization string `json:"organization"`
	IsManaged    bool   `json:"is_managed"`
	Source       string `json:"source"`
}

// LocalUser is a local macOS account.
type LocalUser struct {
	Username    string `json:"username"`
	UID         *int   `json:"uid"`
	RealName    string `json:"real_name"`
	Home        string `json:"home"`
	IsAdmin     bool   `json:"is_admin"`
	SecureToken *bool  `json:"secure_token"`
}

// EAValue is an extension attribute value for a computer.
type EAValue struct {
	EAID      string    `json:"ea_id"`
	Name      string    `json:"name"`
	DataType  string    `json:"data_type"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ExtensionAttribute is a custom inventory field.
type ExtensionAttribute struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	DataType    string    `json:"data_type"`
	InputType   string    `json:"input_type"`
	Script      string    `json:"script"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Criterion is a single smart group rule.
type Criterion struct {
	AndOr      string `json:"and_or"` // "and" | "or" (ignored for first)
	Field      string `json:"field"`
	Operator   string `json:"operator"`
	Value      string `json:"value"`
	OpenParen  bool   `json:"open_paren"`
	CloseParen bool   `json:"close_paren"`
}

// ComputerGroup is a static or smart group.
type ComputerGroup struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	IsSmart     bool        `json:"is_smart"`
	Criteria    []Criterion `json:"criteria"`
	MemberCount int         `json:"member_count"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// Script is a shell script in the library.
type Script struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Category        string    `json:"category"`
	Info            string    `json:"info"`
	Notes           string    `json:"notes"`
	Priority        string    `json:"priority"`
	ParameterLabels []string  `json:"parameter_labels"`
	Contents        string    `json:"contents"`
	OSRequirements  string    `json:"os_requirements"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Package is an installer in the distribution point.
type Package struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Filename  string    `json:"filename"`
	Category  string    `json:"category"`
	Info      string    `json:"info"`
	Notes     string    `json:"notes"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConfigProfile is a configuration profile (.mobileconfig) with scope.
type ConfigProfile struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Identifier  string    `json:"identifier"`
	UUID        string    `json:"uuid"`
	Level       string    `json:"level"`
	Payload     string    `json:"payload,omitempty"`
	Scope       Scope     `json:"scope"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// SchemaSource is "custom:<id>" or "builtin:<domain>" when built from a JSON schema.
	SchemaSource string         `json:"schema_source"`
	SchemaValues map[string]any `json:"schema_values,omitempty"`
}

// CustomSchema is an uploaded Jamf-style external application JSON schema.
type CustomSchema struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	PreferenceDomain string          `json:"preference_domain"`
	Schema           json.RawMessage `json:"schema,omitempty"`
	CreatedBy        string          `json:"created_by"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// ProfileDeployment tracks a profile on a computer.
type ProfileDeployment struct {
	ProfileID    string    `json:"profile_id"`
	ProfileName  string    `json:"profile_name,omitempty"`
	ComputerID   string    `json:"computer_id"`
	ComputerName string    `json:"computer_name,omitempty"`
	Status       string    `json:"status"`
	CommandUUID  string    `json:"command_uuid"`
	Error        string    `json:"error"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PolicyTriggers mirrors the policy trigger checkboxes.
type PolicyTriggers struct {
	Startup            bool   `json:"startup"`
	Login              bool   `json:"login"`
	Checkin            bool   `json:"checkin"`
	EnrollmentComplete bool   `json:"enrollment_complete"`
	NetworkChange      bool   `json:"network_change"`
	Custom             string `json:"custom"`
}

// Matches reports whether a trigger name fires these triggers.
func (t PolicyTriggers) Matches(trigger string) bool {
	switch trigger {
	case "startup":
		return t.Startup
	case "login":
		return t.Login
	case "checkin":
		return t.Checkin
	case "enrollment_complete":
		return t.EnrollmentComplete
	case "network_change":
		return t.NetworkChange
	case "":
		return false
	}
	return t.Custom != "" && t.Custom == trigger
}

// PolicyScript is a script payload inside a policy.
type PolicyScript struct {
	ScriptID   string   `json:"script_id"`
	Priority   string   `json:"priority"` // before | after
	Parameters []string `json:"parameters"`
}

// PolicyPackage is a package payload inside a policy.
type PolicyPackage struct {
	PackageID string `json:"package_id"`
	Action    string `json:"action"` // install
}

// InstallomatorOptions are runtime options for an Installomator run
// (mirrors installomator.Options; kept here to avoid an import cycle).
type InstallomatorOptions struct {
	Notify                string `json:"notify"`
	BlockingProcessAction string `json:"blocking_process_action"`
	Reopen                string `json:"reopen"`
	IgnoreAppStoreApps    string `json:"ignore_app_store_apps"`
	ForceInstall          bool   `json:"force_install"`
}

// PolicyApp is an Installomator step inside a policy. Label is a shipped
// Installomator label or "custom:<key>" for a custom label.
type PolicyApp struct {
	Label   string               `json:"label"`
	Options InstallomatorOptions `json:"options"`
}

// CustomLabel is an admin-defined Installomator label.
type CustomLabel struct {
	ID                string    `json:"id"`
	Key               string    `json:"key"`
	Name              string    `json:"name"`
	Type              string    `json:"type"`
	DownloadURL       string    `json:"download_url"`
	ExpectedTeamID    string    `json:"expected_team_id"`
	PackageID         string    `json:"package_id"`
	AppName           string    `json:"app_name"`
	ArchiveName       string    `json:"archive_name"`
	VersionKey        string    `json:"version_key"`
	AppNewVersion     string    `json:"app_new_version"`
	BlockingProcesses []string  `json:"blocking_processes"`
	Description       string    `json:"description"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// PolicyReboot configures restart behavior.
type PolicyReboot struct {
	Enabled      bool   `json:"enabled"`
	Message      string `json:"message"`
	DelayMinutes int    `json:"delay_minutes"`
}

// PolicyUserInteraction configures messages shown to the user.
type PolicyUserInteraction struct {
	StartMessage    string `json:"start_message"`
	CompleteMessage string `json:"complete_message"`
}

// PolicySelfService configures Self Service availability.
type PolicySelfService struct {
	Enabled     bool   `json:"enabled"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	ButtonText  string `json:"button_text"`
}

// Policy is the central automation object.
type Policy struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Enabled         bool                  `json:"enabled"`
	Category        string                `json:"category"`
	Triggers        PolicyTriggers        `json:"triggers"`
	Frequency       string                `json:"frequency"`
	Scope           Scope                 `json:"scope"`
	Scripts         []PolicyScript        `json:"scripts"`
	Packages        []PolicyPackage       `json:"packages"`
	Apps            []PolicyApp           `json:"apps"`
	RunCommand      string                `json:"run_command"`
	UpdateInventory bool                  `json:"update_inventory"`
	Reboot          PolicyReboot          `json:"reboot"`
	UserInteraction PolicyUserInteraction `json:"user_interaction"`
	SelfService     PolicySelfService     `json:"self_service"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

// PolicyStep is one executed step in a policy run.
type PolicyStep struct {
	Type       string `json:"type"` // script | package | command | inventory | reboot
	Name       string `json:"name"`
	Status     string `json:"status"` // completed | failed | skipped
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
	DurationMS int64  `json:"duration_ms"`
}

// PolicyLog is a record of a policy run on a computer.
type PolicyLog struct {
	ID           string       `json:"id"`
	PolicyID     string       `json:"policy_id"`
	PolicyName   string       `json:"policy_name,omitempty"`
	ComputerID   string       `json:"computer_id"`
	ComputerName string       `json:"computer_name,omitempty"`
	Trigger      string       `json:"trigger"`
	Status       string       `json:"status"`
	StartedAt    *time.Time   `json:"started_at"`
	CompletedAt  *time.Time   `json:"completed_at"`
	Log          string       `json:"log"`
	Steps        []PolicyStep `json:"steps"`
	CreatedAt    time.Time    `json:"created_at"`
}

// AgentTask is a one-off instruction for the agent on a computer.
type AgentTask struct {
	ID           string         `json:"id"`
	ComputerID   string         `json:"computer_id"`
	ComputerName string         `json:"computer_name,omitempty"`
	Type         string         `json:"type"`
	Payload      map[string]any `json:"payload"`
	Status       string         `json:"status"`
	Result       map[string]any `json:"result"`
	CreatedBy    string         `json:"created_by"`
	CreatedAt    time.Time      `json:"created_at"`
	SentAt       *time.Time     `json:"sent_at"`
	CompletedAt  *time.Time     `json:"completed_at"`
}

// MDMCommand is a row from the NanoMDM queue joined with our metadata.
type MDMCommand struct {
	CommandUUID     string     `json:"command_uuid"`
	EnrollmentID    string     `json:"enrollment_id"`
	ComputerID      string     `json:"computer_id"`
	MobileDeviceID  string     `json:"mobile_device_id,omitempty"`
	ComputerName    string     `json:"computer_name,omitempty"` // the device's name (computer or mobile device)
	RequestType     string     `json:"request_type"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"` // Pending | Acknowledged | Error | NotNow | CommandFormatError
	Active          bool       `json:"active"`
	CreatedAt       time.Time  `json:"created_at"`
	ResultUpdatedAt *time.Time `json:"result_updated_at"`
	Command         string     `json:"command,omitempty"`
	Result          string     `json:"result,omitempty"`
	CreatedBy       string     `json:"created_by"`
}

// AdminUser is a console user.
type AdminUser struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
	// MustChangePassword blocks everything but changing the password (bootstrap accounts).
	MustChangePassword bool `json:"must_change_password"`
}

// APIToken is a bearer token for automation.
type APIToken struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	Role        string     `json:"role"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

// EnrollmentInvitation is a shareable enrollment link / agent enrollment token.
type EnrollmentInvitation struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	Enabled     bool       `json:"enabled"`
	MaxUses     *int       `json:"max_uses"`
	Uses        int        `json:"uses"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// AuditEntry is an audit log row.
type AuditEntry struct {
	ID         int64          `json:"id"`
	At         time.Time      `json:"at"`
	Actor      string         `json:"actor"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Details    map[string]any `json:"details"`
}

// ShellSession is an audited remote shell session.
type ShellSession struct {
	ID           string     `json:"id"`
	ComputerID   string     `json:"computer_id"`
	ComputerName string     `json:"computer_name,omitempty"`
	Actor        string     `json:"actor"`
	ActorIP      string     `json:"actor_ip"`
	Status       string     `json:"status"`
	Reason       string     `json:"reason"`
	BytesIn      int64      `json:"bytes_in"`
	BytesOut     int64      `json:"bytes_out"`
	StartedAt    time.Time  `json:"started_at"`
	ConnectedAt  *time.Time `json:"connected_at"`
	EndedAt      *time.Time `json:"ended_at"`
}

// RestrictedSoftware is a process-blocking rule (Jamf-style restricted software).
type RestrictedSoftware struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	ProcessName    string    `json:"process_name"`
	MatchType      string    `json:"match_type"` // exact | contains | regex
	KillProcess    bool      `json:"kill_process"`
	NotifyUser     bool      `json:"notify_user"`
	DisplayMessage string    `json:"display_message"`
	Enabled        bool      `json:"enabled"`
	Scope          Scope     `json:"scope"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// RestrictedEvent is a reported violation.
type RestrictedEvent struct {
	ID           int64     `json:"id"`
	RestrictedID string    `json:"restricted_id"`
	ComputerID   string    `json:"computer_id"`
	ComputerName string    `json:"computer_name,omitempty"`
	RuleName     string    `json:"rule_name"`
	Process      string    `json:"process"`
	Username     string    `json:"username"`
	Killed       bool      `json:"killed"`
	At           time.Time `json:"at"`
}

// AdvancedSearch is a saved computer search with chosen display fields.
type AdvancedSearch struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Criteria      []Criterion `json:"criteria"`
	DisplayFields []string    `json:"display_fields"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// MacApp is an app catalog entry installed via Installomator.
type MacApp struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Label          string               `json:"label"`
	Category       string               `json:"category"`
	Description    string               `json:"description"`
	UpdateBehavior string               `json:"update_behavior"` // install_once | keep_updated
	Options        InstallomatorOptions `json:"options"`
	Enabled        bool                 `json:"enabled"`
	Scope          Scope                `json:"scope"`
	SelfService    PolicySelfService    `json:"self_service"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

// AppInstallLog is one app install run on a computer.
type AppInstallLog struct {
	ID           string     `json:"id"`
	AppID        string     `json:"app_id"`
	AppName      string     `json:"app_name,omitempty"`
	ComputerID   string     `json:"computer_id"`
	ComputerName string     `json:"computer_name,omitempty"`
	Status       string     `json:"status"`
	Trigger      string     `json:"trigger"`
	Log          string     `json:"log"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at"`
}

// FileVaultStatus is a computer's escrow state (never contains the key).
type FileVaultStatus struct {
	Escrowed            bool       `json:"escrowed"`
	Fingerprint         string     `json:"fingerprint"`
	EscrowedAt          *time.Time `json:"escrowed_at"`
	RotatedCount        int        `json:"rotated_count"`
	Source              string     `json:"source"`
	EscrowProfileStatus string     `json:"escrow_profile_status"` // deployment status of the escrow profile on this Mac
}

// FileVaultRow is one computer in the fleet escrow overview.
type FileVaultRow struct {
	ComputerID          string     `json:"computer_id"`
	ComputerName        string     `json:"computer_name"`
	SerialNumber        string     `json:"serial_number"`
	FileVaultEnabled    *bool      `json:"filevault_enabled"`
	MDMEnrolled         bool       `json:"mdm_enrolled"`
	AgentEnrolled       bool       `json:"agent_enrolled"`
	Escrowed            bool       `json:"escrowed"`
	Fingerprint         string     `json:"fingerprint"`
	EscrowedAt          *time.Time `json:"escrowed_at"`
	RotatedCount        int        `json:"rotated_count"`
	EscrowProfileStatus string     `json:"escrow_profile_status"`
}

// FileVaultSummary counts fleet escrow states.
type FileVaultSummary struct {
	Total        int `json:"total"`
	Escrowed     int `json:"escrowed"`      // FileVault on, key escrowed
	MissingKey   int `json:"missing_key"`   // FileVault on, no escrowed key (needs remediation)
	NotEncrypted int `json:"not_encrypted"` // FileVault off
	Unknown      int `json:"unknown"`       // not yet reported
}

// CommandEvent is one step in an MDM command's life (inspector timeline).
type CommandEvent struct {
	ID           int64          `json:"id"`
	CommandUUID  string         `json:"command_uuid"`
	EnrollmentID string         `json:"enrollment_id"`
	Event        string         `json:"event"`
	Description  string         `json:"description"`
	Details      map[string]any `json:"details"`
	RequestType  string         `json:"request_type"`
	ComputerID   string         `json:"computer_id"`
	ComputerName string         `json:"computer_name"`
	At           time.Time      `json:"at"`
}

// LogEntry is a stored server or agent log line.
type LogEntry struct {
	ID           int64          `json:"id"`
	Source       string         `json:"source"` // server | agent
	ComputerID   string         `json:"computer_id,omitempty"`
	ComputerName string         `json:"computer_name,omitempty"`
	At           time.Time      `json:"at"`
	Level        string         `json:"level"`
	Component    string         `json:"component"`
	Msg          string         `json:"msg"`
	Attrs        map[string]any `json:"attrs"`
}

// DiagnosticBundle is an on-demand log/diagnostics archive collected from a Mac.
type DiagnosticBundle struct {
	ID           string     `json:"id"`
	ComputerID   string     `json:"computer_id"`
	ComputerName string     `json:"computer_name"`
	Status       string     `json:"status"` // requested | uploaded | failed
	Filename     string     `json:"filename"`
	SizeBytes    int64      `json:"size_bytes"`
	SHA256       string     `json:"sha256"`
	Error        string     `json:"error"`
	RequestedBy  string     `json:"requested_by"`
	CreatedAt    time.Time  `json:"created_at"`
	UploadedAt   *time.Time `json:"uploaded_at"`
}

// UpdatePlan is a declarative software update plan (enforcement or settings).
type UpdatePlan struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Enabled             bool           `json:"enabled"`
	Kind                string         `json:"kind"` // enforcement | settings
	TargetOSVersion     string         `json:"target_os_version"`
	TargetBuildVersion  string         `json:"target_build_version"`
	TargetLocalDateTime string         `json:"target_local_datetime"`
	DetailsURL          string         `json:"details_url"`
	Settings            map[string]any `json:"settings"`
	Scope               Scope          `json:"scope"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// DDMState is a device's declarative management state.
type DDMState struct {
	ComputerID         string         `json:"computer_id"`
	ServedToken        string         `json:"served_token"`
	SyncSentToken      string         `json:"sync_sent_token"`
	SyncSentAt         *time.Time     `json:"sync_sent_at"`
	LastSyncAt         *time.Time     `json:"last_sync_at"`
	LastStatusAt       *time.Time     `json:"last_status_at"`
	Status             map[string]any `json:"status"`
	DeclarationsStatus map[string]any `json:"declarations_status"`
	Errors             []any          `json:"errors"`
}

// Settings are the editable server settings.
type Settings struct {
	OrgName                     string `json:"org_name"`
	ServerURL                   string `json:"server_url"`
	AgentCheckinIntervalSeconds int    `json:"agent_checkin_interval_seconds"`
	AgentEnrollmentSecret       string `json:"agent_enrollment_secret"`
	SCEPChallenge               string `json:"scep_challenge"`
	EnrollmentProfileName       string `json:"enrollment_profile_name"`
	EnrollmentProfileIdentifier string `json:"enrollment_profile_identifier"`
	OpenEnrollment              bool   `json:"open_enrollment"`
	AutoInstallAgent            bool   `json:"auto_install_agent"`
	InventoryIntervalHours      int    `json:"inventory_interval_hours"`
	SigningCertPEM              string `json:"signing_cert_pem"`
	SigningKeyPEM               string `json:"signing_key_pem"`
	RemoteShellEnabled          bool   `json:"remote_shell_enabled"`
	RemoteShellIdleTimeoutSecs  int    `json:"remote_shell_idle_timeout_secs"`
	FileVaultAutoRemediate      bool   `json:"filevault_auto_remediate"`
	LogRetentionDays            int    `json:"log_retention_days"`
	LogArchiveEnabled           bool   `json:"log_archive_enabled"`
	MCPEnabled                  bool   `json:"mcp_enabled"`      // expose the MCP server at /mcp
	MCPAllowWrites              bool   `json:"mcp_allow_writes"` // allow admin tokens to call write tools
}

// HealthRemediation describes a fix offered for a health finding.
type HealthRemediation struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Safe        bool   `json:"safe"`     // may run automatically (self-healing)
	Runnable    bool   `json:"runnable"` // the server can perform it; otherwise follow URL
	URL         string `json:"url,omitempty"`
}

// HealthFinding is one detected misconfiguration or problem with one object.
type HealthFinding struct {
	ID             string              `json:"id"`
	CheckID        string              `json:"check_id"`
	CheckTitle     string              `json:"check_title,omitempty"`
	Category       string              `json:"category,omitempty"`
	ObjectType     string              `json:"object_type"`
	ObjectID       string              `json:"object_id"`
	ObjectName     string              `json:"object_name"`
	Severity       string              `json:"severity"`
	Summary        string              `json:"summary"`
	Detail         string              `json:"detail"`
	Facts          map[string]any      `json:"facts"`
	Remediations   []HealthRemediation `json:"remediations"`
	State          string              `json:"state"` // pending | open | acknowledged | snoozed | resolved
	SnoozedUntil   *time.Time          `json:"snoozed_until"`
	AcknowledgedBy string              `json:"acknowledged_by"`
	FirstSeen      time.Time           `json:"first_seen"`
	LastSeen       time.Time           `json:"last_seen"`
	ResolvedAt     *time.Time          `json:"resolved_at"`
	Occurrences    int                 `json:"occurrences"`
	Misses         int                 `json:"-"`
	Notified       bool                `json:"-"`
	HealAttempts   int                 `json:"heal_attempts"`
	LastHealAt     *time.Time          `json:"last_heal_at"`
	LastHealError  string              `json:"last_heal_error"`
}

// HealthEvent is a lifecycle entry for a finding.
type HealthEvent struct {
	ID        int64     `json:"id"`
	FindingID string    `json:"finding_id"`
	CheckID   string    `json:"check_id"`
	Event     string    `json:"event"`
	Actor     string    `json:"actor"`
	Message   string    `json:"message"`
	At        time.Time `json:"at"`
}

// HealthCheckSetting is an admin override for a check.
type HealthCheckSetting struct {
	Enabled  bool  `json:"enabled"`
	AutoHeal *bool `json:"auto_heal"` // nil = the check's default
}

// ComplianceBaseline is an mSCP benchmark tailored and scoped by an admin.
type ComplianceBaseline struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Benchmark         string            `json:"benchmark"`
	Enabled           bool              `json:"enabled"`
	Scope             Scope             `json:"scope"`
	ExtraRules        []string          `json:"extra_rules"`
	Exclusions        map[string]string `json:"exclusions"`
	ODVOverrides      map[string]any    `json:"odv_overrides"`
	ScanIntervalHours int               `json:"scan_interval_hours"`
	AutoRemediate     bool              `json:"auto_remediate"`
	DeployProfile     bool              `json:"deploy_profile"`
	ProfileID         *string           `json:"profile_id"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// ComplianceScan is one scan of a baseline on a computer.
type ComplianceScan struct {
	ID           string     `json:"id"`
	BaselineID   string     `json:"baseline_id"`
	ComputerID   string     `json:"computer_id"`
	ComputerName string     `json:"computer_name,omitempty"`
	TaskID       *string    `json:"task_id"`
	Trigger      string     `json:"trigger"`
	Remediate    bool       `json:"remediate"`
	Status       string     `json:"status"`
	OSVersion    string     `json:"os_version"`
	Passed       int        `json:"passed"`
	Failed       int        `json:"failed"`
	Remediated   int        `json:"remediated"`
	Errors       int        `json:"errors"`
	Total        int        `json:"total"`
	DurationMS   int64      `json:"duration_ms"`
	Error        string     `json:"error"`
	RequestedAt  time.Time  `json:"requested_at"`
	CompletedAt  *time.Time `json:"completed_at"`
}

// ComplianceResult is the latest result of one rule on one computer.
type ComplianceResult struct {
	BaselineID     string    `json:"baseline_id"`
	ComputerID     string    `json:"computer_id"`
	ComputerName   string    `json:"computer_name,omitempty"`
	RuleID         string    `json:"rule_id"`
	Status         string    `json:"status"` // pass | fail | error | remediated
	PreviousStatus string    `json:"previous_status"`
	Actual         string    `json:"actual"`
	Expected       string    `json:"expected"`
	Detail         string    `json:"detail"`
	CheckedAt      time.Time `json:"checked_at"`
	ChangedAt      time.Time `json:"changed_at"`
}

// AgentComplianceRule is one check sent to the agent.
type AgentComplianceRule struct {
	ID           string `json:"id"`
	Check        string `json:"check"`
	ExpectedType string `json:"expected_type"` // string | integer | base64
	Expected     string `json:"expected"`
	Fix          string `json:"fix,omitempty"` // only when remediation is requested
}

// AgentComplianceScan is the payload of a compliance_scan agent task.
type AgentComplianceScan struct {
	ScanID    string                `json:"scan_id"`
	Baseline  string                `json:"baseline"`
	Remediate bool                  `json:"remediate"`
	Rules     []AgentComplianceRule `json:"rules"`
}

// AgentComplianceRuleResult is the agent's outcome for one rule.
type AgentComplianceRuleResult struct {
	ID     string `json:"id"`
	Status string `json:"status"` // pass | fail | error | remediated
	Actual string `json:"actual"`
	Detail string `json:"detail,omitempty"`
}

// AgentComplianceSubmit is posted by the agent when a scan finishes.
type AgentComplianceSubmit struct {
	OSVersion  string                      `json:"os_version"`
	DurationMS int64                       `json:"duration_ms"`
	Results    []AgentComplianceRuleResult `json:"results"`
	Error      string                      `json:"error,omitempty"`
}

// SuperConfig is the fleet-wide S.U.P.E.R.M.A.N. deployment.
type SuperConfig struct {
	Enabled     bool      `json:"enabled"`
	Scope       Scope     `json:"scope"`
	AuthMode    string    `json:"auth_mode"` // none | local | service
	AuthAccount string    `json:"auth_account"`
	HasPassword bool      `json:"has_password"`
	ProfileID   *string   `json:"profile_id"`
	Version     string    `json:"version"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// OnboardingField is a value collected from the user at enrollment.
type OnboardingField struct {
	Key         string   `json:"key"` // computer_name | asset_tag | building | department | room | assigned_user | assigned_real_name | assigned_email | assigned_position
	Label       string   `json:"label"`
	Required    bool     `json:"required"`
	Placeholder string   `json:"placeholder"`
	Options     []string `json:"options"` // shows a picker instead of a text field
	Regex       string   `json:"regex"`
	RegexError  string   `json:"regex_error"`
}

// OnboardingStep is one item in the onboarding progress list.
type OnboardingStep struct {
	Type            string `json:"type"` // app | policy | script | recon | wait
	Title           string `json:"title"`
	Subtitle        string `json:"subtitle"`
	Icon            string `json:"icon"`
	AppID           string `json:"app_id,omitempty"`
	PolicyID        string `json:"policy_id,omitempty"`
	ScriptID        string `json:"script_id,omitempty"`
	Seconds         int    `json:"seconds,omitempty"`
	ContinueOnError bool   `json:"continue_on_error"`
}

// OnboardingConfig is the content of a workflow.
type OnboardingConfig struct {
	Title                string            `json:"title"`
	Message              string            `json:"message"`
	Icon                 string            `json:"icon"`   // URL, file path or SF Symbol ("SF=laptopcomputer")
	Banner               string            `json:"banner"` // optional banner image URL
	BlurScreen           bool              `json:"blur_screen"`
	Fields               []OnboardingField `json:"fields"`
	ComputerNameTemplate string            `json:"computer_name_template"` // e.g. "MAC-{serial}" or "{asset_tag}-{user}"
	Steps                []OnboardingStep  `json:"steps"`
	FinalAction          string            `json:"final_action"` // none | restart | logout | shutdown
	FinalMessage         string            `json:"final_message"`
}

// OnboardingWorkflow is a scoped white-glove onboarding experience.
type OnboardingWorkflow struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Enabled   bool             `json:"enabled"`
	Priority  int              `json:"priority"`
	Scope     Scope            `json:"scope"`
	Config    OnboardingConfig `json:"config"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// OnboardingStepResult is the outcome of one step.
type OnboardingStepResult struct {
	Title      string `json:"title"`
	Type       string `json:"type"`
	Status     string `json:"status"` // success | failed | skipped
	Detail     string `json:"detail"`
	DurationMS int64  `json:"duration_ms"`
}

// OnboardingRun is one Mac's pass through a workflow.
type OnboardingRun struct {
	ID           string                 `json:"id"`
	WorkflowID   string                 `json:"workflow_id"`
	WorkflowName string                 `json:"workflow_name,omitempty"`
	ComputerID   string                 `json:"computer_id"`
	ComputerName string                 `json:"computer_name,omitempty"`
	Status       string                 `json:"status"`
	UserEntry    map[string]string      `json:"user_entry"`
	Steps        []OnboardingStepResult `json:"steps"`
	Log          string                 `json:"log,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	StartedAt    *time.Time             `json:"started_at"`
	CompletedAt  *time.Time             `json:"completed_at"`
}

// AgentOnboardingStep is a step resolved for execution.
type AgentOnboardingStep struct {
	Title           string        `json:"title"`
	Subtitle        string        `json:"subtitle"`
	Icon            string        `json:"icon"`
	Type            string        `json:"type"`
	App             *AgentAppStep `json:"app,omitempty"`
	Policy          *AgentPolicy  `json:"policy,omitempty"`
	ScriptName      string        `json:"script_name,omitempty"`
	ScriptContents  string        `json:"script_contents,omitempty"`
	Seconds         int           `json:"seconds,omitempty"`
	ContinueOnError bool          `json:"continue_on_error"`
}

// AgentOnboarding is the payload of an onboarding agent task.
type AgentOnboarding struct {
	RunID                string                `json:"run_id"`
	Title                string                `json:"title"`
	Message              string                `json:"message"`
	Icon                 string                `json:"icon"`
	Banner               string                `json:"banner"`
	BlurScreen           bool                  `json:"blur_screen"`
	Fields               []OnboardingField     `json:"fields"`
	ComputerNameTemplate string                `json:"computer_name_template"`
	Steps                []AgentOnboardingStep `json:"steps"`
	FinalAction          string                `json:"final_action"`
	FinalMessage         string                `json:"final_message"`
	DialogArgs           []string              `json:"dialog_args"` // Installomator args to install swiftDialog
	OrgName              string                `json:"org_name"`
}

// AgentOnboardingSubmit reports a finished run.
type AgentOnboardingSubmit struct {
	Status    string                 `json:"status"` // completed | failed
	UserEntry map[string]string      `json:"user_entry"`
	Steps     []OnboardingStepResult `json:"steps"`
	Log       string                 `json:"log"`
}

// LAPSConfig is the fleet-wide local administrator password (LAPS) policy.
type LAPSConfig struct {
	Enabled bool  `json:"enabled"`
	Scope   Scope `json:"scope"`
	// AccountName is the local account whose password is managed.
	AccountName string `json:"account_name"`
	// CreateAccount creates the account (as an administrator) when it is missing.
	CreateAccount bool `json:"create_account"`
	// HideAccount hides the account from the login window and Users & Groups.
	HideAccount bool `json:"hide_account"`
	// PasswordFormat is "jamf" (A-Z and 2-9 in dash-separated groups of five, like Jamf Pro)
	// or "complex" (all character classes).
	PasswordFormat string `json:"password_format"`
	PasswordLength int    `json:"password_length"`
	// AutoRotate expires a password that was never viewed after RotationDays (Jamf: autoRotateEnabled).
	AutoRotate   bool `json:"auto_rotate"`
	RotationDays int  `json:"rotation_days"`
	// RotateAfterUseMinutes is how long after a view the password is rotated (Jamf: passwordRotationTime).
	// Rotation after viewing is always on, as in Jamf Pro.
	RotateAfterUseMinutes int `json:"rotate_after_use_minutes"`
}

// LAPSStatus is one computer's LAPS state. It never contains a password.
type LAPSStatus struct {
	InScope     bool       `json:"in_scope"`
	HasPassword bool       `json:"has_password"`
	Account     string     `json:"account"`
	Fingerprint string     `json:"fingerprint"`
	EscrowedAt  *time.Time `json:"escrowed_at"`
	// ExpiresAt is when the scheduled rotation falls due.
	ExpiresAt    *time.Time `json:"expires_at"`
	RotatedCount int        `json:"rotated_count"`
	// Pending is true while a staged password has not been confirmed as applied.
	Pending        bool       `json:"pending"`
	PendingAt      *time.Time `json:"pending_at"`
	LastViewedAt   *time.Time `json:"last_viewed_at"`
	LastViewedBy   string     `json:"last_viewed_by"`
	RotateAfter    *time.Time `json:"rotate_after"` // set by a reveal: rotate once this time passes
	RotateRequest  bool       `json:"rotate_requested"`
	LastError      string     `json:"last_error"`
	LastErrorAt    *time.Time `json:"last_error_at"`
	AgentEnrolled  bool       `json:"agent_enrolled"`
	ComputerID     string     `json:"computer_id,omitempty"`
	ComputerName   string     `json:"computer_name,omitempty"`
	SerialNumber   string     `json:"serial_number,omitempty"`
	AgentLastSeen  *time.Time `json:"agent_last_seen,omitempty"`
	RotationStatus string     `json:"rotation_status,omitempty"` // ok | due | pending | missing | failing | out_of_scope
	Guid           string     `json:"guid"`
	// PasswordSetBy is set while an administrator-chosen password waits to be applied.
	PasswordSetBy string `json:"password_set_by,omitempty"`
}

// LAPSView records one administrator viewing a password.
type LAPSView struct {
	ViewedBy string    `json:"viewed_by"`
	Reason   string    `json:"reason"`
	ViewedAt time.Time `json:"viewed_at"`
}

// LAPSHistoryEntry is one password a Mac has had. Password is filled only for the admin audit view.
type LAPSHistoryEntry struct {
	Account     string     `json:"account"`
	Guid        string     `json:"guid"`
	Fingerprint string     `json:"fingerprint"`
	Password    string     `json:"password,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	RetiredAt   *time.Time `json:"retired_at"` // nil while current
	Views       []LAPSView `json:"views"`
}

// LAPSEvent is one entry in a Mac's LAPS event history.
type LAPSEvent struct {
	ID        int64     `json:"id"`
	Account   string    `json:"account"`
	EventType string    `json:"event_type"` // CREATED | ROTATED | VIEWED | PENDING | SET | ERROR
	Actor     string    `json:"actor"`
	Detail    string    `json:"detail"`
	At        time.Time `json:"at"`
}

// LAPSPendingRotation is a Mac with a rotation queued but not yet confirmed.
type LAPSPendingRotation struct {
	ComputerID   string    `json:"computer_id"`
	ComputerName string    `json:"computer_name"`
	Account      string    `json:"account"`
	Guid         string    `json:"guid"`
	Reason       string    `json:"reason"` // viewed | requested | set | expired | staged | missing
	Since        time.Time `json:"since"`
	// DueAt is when a post-view rotation falls due (nil = as soon as the agent checks in).
	DueAt *time.Time `json:"due_at"`
}

// LAPSSummary counts fleet LAPS states (Macs in scope with the agent).
type LAPSSummary struct {
	InScope int `json:"in_scope"`
	OK      int `json:"ok"`
	Due     int `json:"due"`
	Pending int `json:"pending"`
	Missing int `json:"missing"`
	Failing int `json:"failing"`
}

// AgentPackage describes the signed agent package served to mdmclient for automatic installs.
type AgentPackage struct {
	FileName   string    `json:"file_name"`
	Version    string    `json:"version"`
	Size       int64     `json:"size"`
	SHA256     string    `json:"sha256"`
	MD5Chunk   int64     `json:"md5_chunk"`
	MD5s       []string  `json:"md5s"`
	SignedBy   string    `json:"signed_by"`
	UploadedBy string    `json:"uploaded_by"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// AgentInstallAttempt is one MDM-driven agent install on a Mac.
type AgentInstallAttempt struct {
	ID          int64     `json:"id"`
	ComputerID  string    `json:"computer_id"`
	CommandUUID string    `json:"command_uuid"`
	Status      string    `json:"status"` // sent | installed | failed
	Detail      string    `json:"detail"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MobileDevice is an iPhone, iPad, iPod touch or Apple TV. Mobile devices are kept apart from
// computers (their own tables, groups, profiles and apps), as in Jamf Pro.
type MobileDevice struct {
	ID                    string         `json:"id"`
	UDID                  string         `json:"udid"`
	SerialNumber          string         `json:"serial_number"`
	Name                  string         `json:"name"`
	DeviceType            string         `json:"device_type"` // iPhone | iPad | iPod | AppleTV
	Model                 string         `json:"model"`
	ModelName             string         `json:"model_name"`
	ProductName           string         `json:"product_name"`
	OSVersion             string         `json:"os_version"`
	OSBuild               string         `json:"os_build"`
	CapacityGB            *float64       `json:"capacity_gb"`
	AvailableGB           *float64       `json:"available_gb"`
	BatteryLevel          *float64       `json:"battery_level"` // 0..1
	WiFiMAC               string         `json:"wifi_mac"`
	BluetoothMAC          string         `json:"bluetooth_mac"`
	IMEI                  string         `json:"imei"`
	MEID                  string         `json:"meid"`
	PhoneNumber           string         `json:"phone_number"`
	Carrier               string         `json:"carrier"`
	CellularTechnology    string         `json:"cellular_technology"`
	IsRoaming             *bool          `json:"is_roaming"`
	IsSupervised          *bool          `json:"is_supervised"`
	IsSharedIPad          *bool          `json:"is_shared_ipad"`
	ActivationLockEnabled *bool          `json:"activation_lock_enabled"`
	FindMyEnabled         *bool          `json:"find_my_enabled"`
	LostModeEnabled       *bool          `json:"lost_mode_enabled"`
	PasscodePresent       *bool          `json:"passcode_present"`
	PasscodeCompliant     *bool          `json:"passcode_compliant"`
	HardwareEncryption    *int           `json:"hardware_encryption"`
	LastBackupAt          *time.Time     `json:"last_backup_at"`
	MDMEnrolled           bool           `json:"mdm_enrolled"`
	MDMEnrolledAt         *time.Time     `json:"mdm_enrolled_at"`
	MDMLastCheckin        *time.Time     `json:"mdm_last_checkin"`
	MDMUnenrolledAt       *time.Time     `json:"mdm_unenrolled_at"`
	DeviceInfo            map[string]any `json:"device_info"`
	SecurityInfo          map[string]any `json:"security_info"`
	LastInventoryAt       *time.Time     `json:"last_inventory_at"`
	AssetTag              string         `json:"asset_tag"`
	Site                  string         `json:"site"`
	Department            string         `json:"department"`
	Building              string         `json:"building"`
	Room                  string         `json:"room"`
	AssignedUser          string         `json:"assigned_user"`
	AssignedRealName      string         `json:"assigned_real_name"`
	AssignedEmail         string         `json:"assigned_email"`
	Notes                 string         `json:"notes"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}

// MobileDeviceApp is an app installed on a mobile device.
type MobileDeviceApp struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	BundleID      string `json:"bundle_id"`
	Managed       bool   `json:"managed"`
	ManagedStatus string `json:"managed_status"`
}

// MobileDeviceDetail is a mobile device with its related inventory.
type MobileDeviceDetail struct {
	MobileDevice
	Apps                []MobileDeviceApp `json:"apps"`
	Profiles            []ComputerProfile `json:"profiles"`
	Groups              []ComputerGroup   `json:"groups"`
	ExtensionAttributes []EAValue         `json:"extension_attributes"`
}

// MobileExtensionAttribute is a custom inventory field for mobile devices. Values are entered by an
// administrator (there is no agent on iOS): a free text field or a pop-up menu.
type MobileExtensionAttribute struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	DataType     string    `json:"data_type"`  // string | integer | date
	InputType    string    `json:"input_type"` // text | popup
	PopupChoices []string  `json:"popup_choices"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
