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
	ComputerName    string     `json:"computer_name,omitempty"`
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
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Label          string            `json:"label"`
	Category       string            `json:"category"`
	Description    string            `json:"description"`
	UpdateBehavior string            `json:"update_behavior"` // install_once | keep_updated
	Enabled        bool              `json:"enabled"`
	Scope          Scope             `json:"scope"`
	SelfService    PolicySelfService `json:"self_service"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
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
}
