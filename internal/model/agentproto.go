package model

import "time"

// ---- Agent <-> server protocol ----

// AgentEnrollRequest is sent by the agent to enroll.
type AgentEnrollRequest struct {
	Token        string `json:"token"`
	SerialNumber string `json:"serial_number"`
	UDID         string `json:"udid"`
	Hostname     string `json:"hostname"`
	ComputerName string `json:"computer_name"`
	OSVersion    string `json:"os_version"`
	Architecture string `json:"architecture"`
	AgentVersion string `json:"agent_version"`
	// PreviousDeviceToken proves a reinstall on the same Mac (the existing agent's token).
	PreviousDeviceToken string `json:"previous_device_token,omitempty"`
}

// AgentEnrollResponse returns the device token.
type AgentEnrollResponse struct {
	ComputerID             string `json:"computer_id"`
	DeviceToken            string `json:"device_token"`
	CheckinIntervalSeconds int    `json:"checkin_interval_seconds"`
	ServerURL              string `json:"server_url"`
	OrgName                string `json:"org_name"`
	MDMEnrolled            bool   `json:"mdm_enrolled"`
}

// AgentCheckinRequest is the periodic check-in.
type AgentCheckinRequest struct {
	Trigger      string `json:"trigger"`
	ConsoleUser  string `json:"console_user"`
	Hostname     string `json:"hostname"`
	ComputerName string `json:"computer_name"`
	OSVersion    string `json:"os_version"`
	OSBuild      string `json:"os_build"`
	IPAddress    string `json:"ip_address"`
	AgentVersion string `json:"agent_version"`
	UptimeSecs   int64  `json:"uptime_seconds"`
	// Capabilities lists optional features this agent supports (e.g. "policy_accounts").
	Capabilities []string `json:"capabilities"`
}

// AgentScriptStep is a script to run, with content inlined.
type AgentScriptStep struct {
	ScriptID   string   `json:"script_id"`
	Name       string   `json:"name"`
	Priority   string   `json:"priority"`
	Contents   string   `json:"contents"`
	Parameters []string `json:"parameters"`
}

// AgentPackageStep is a package to install.
type AgentPackageStep struct {
	PackageID string `json:"package_id"`
	Name      string `json:"name"`
	Filename  string `json:"filename"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	URL       string `json:"url"` // relative to server URL
}

// AgentAppStep is an Installomator run inside a policy. Args are built and
// shell-quoted by the server; the agent passes them to Installomator verbatim.
type AgentAppStep struct {
	Name  string   `json:"name"`
	Label string   `json:"label"`
	Args  []string `json:"args"`
}

// AgentPolicy is a policy ready for execution.
type AgentPolicy struct {
	LogID    string             `json:"log_id"`
	PolicyID string             `json:"policy_id"`
	Name     string             `json:"name"`
	Trigger  string             `json:"trigger"`
	Scripts  []AgentScriptStep  `json:"scripts"`
	Packages []AgentPackageStep `json:"packages"`
	Apps     []AgentAppStep     `json:"apps"`
	// Accounts carry no passwords: the agent fetches them during the run from
	// POST /agent/v1/policy-logs/{log_id}/account-secrets.
	Accounts        []PolicyAccount       `json:"accounts"`
	RunCommand      string                `json:"run_command"`
	UpdateInventory bool                  `json:"update_inventory"`
	Reboot          PolicyReboot          `json:"reboot"`
	UserInteraction PolicyUserInteraction `json:"user_interaction"`
}

// AgentRestrictedRule is a restricted-software rule the agent enforces.
type AgentRestrictedRule struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ProcessName    string `json:"process_name"`
	MatchType      string `json:"match_type"`
	KillProcess    bool   `json:"kill_process"`
	NotifyUser     bool   `json:"notify_user"`
	DisplayMessage string `json:"display_message"`
}

// AgentAppInstall is an app-catalog install for the agent to run via Installomator.
type AgentAppInstall struct {
	LogID string   `json:"log_id"`
	AppID string   `json:"app_id"`
	Name  string   `json:"name"`
	Label string   `json:"label"`
	Args  []string `json:"args"` // server-built Installomator arguments
}

// AgentCheckinResponse returns work for the agent.
type AgentCheckinResponse struct {
	ComputerID             string                `json:"computer_id"`
	ComputerName           string                `json:"computer_name"`
	Policies               []AgentPolicy         `json:"policies"`
	Tasks                  []AgentTask           `json:"tasks"`
	AppInstalls            []AgentAppInstall     `json:"app_installs"`
	RestrictedSoftware     []AgentRestrictedRule `json:"restricted_software"`
	CheckinIntervalSeconds int                   `json:"checkin_interval_seconds"`
	InventoryRequested     bool                  `json:"inventory_requested"`
	FireEnrollmentComplete bool                  `json:"fire_enrollment_complete"`
	ServerTime             time.Time             `json:"server_time"`
}

// AgentRestrictedReport is a violation report from the agent.
type AgentRestrictedReport struct {
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Process  string `json:"process"`
	Username string `json:"username"`
	Killed   bool   `json:"killed"`
}

// SelfServiceItem is an item shown in the agent's Self Service portal.
type SelfServiceItem struct {
	Kind        string `json:"kind"` // policy | app
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ButtonText  string `json:"button_text"`
}

// ShellInvite tells the agent to dial back and open a remote shell session.
type ShellInvite struct {
	SessionID string `json:"session_id"`
	Nonce     string `json:"nonce"`
}

// AgentPollResponse is returned by the long-poll endpoint.
type AgentPollResponse struct {
	Tasks   []AgentTask   `json:"tasks"`
	Checkin bool          `json:"checkin"` // server requests an immediate full check-in
	Shells  []ShellInvite `json:"shells,omitempty"`
}

// AgentPolicyLogSubmit reports a policy run.
type AgentPolicyLogSubmit struct {
	Trigger     string       `json:"trigger"`
	Status      string       `json:"status"`
	StartedAt   time.Time    `json:"started_at"`
	CompletedAt time.Time    `json:"completed_at"`
	Log         string       `json:"log"`
	Steps       []PolicyStep `json:"steps"`
}

// AgentTaskResult reports a task outcome.
type AgentTaskResult struct {
	Status      string    `json:"status"` // completed | failed
	ExitCode    int       `json:"exit_code"`
	Stdout      string    `json:"stdout"`
	Stderr      string    `json:"stderr"`
	Error       string    `json:"error"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// AgentReconConfig tells the agent what extension attributes to collect.
type AgentReconConfig struct {
	ExtensionAttributes []ExtensionAttribute `json:"extension_attributes"`
}

// AgentInventory is a full inventory report from the agent.
type AgentInventory struct {
	AgentVersion        string            `json:"agent_version"`
	ComputerName        string            `json:"computer_name"`
	Hostname            string            `json:"hostname"`
	LocalHostname       string            `json:"local_hostname"`
	SerialNumber        string            `json:"serial_number"`
	UDID                string            `json:"udid"`
	ModelIdentifier     string            `json:"model_identifier"`
	ModelName           string            `json:"model_name"`
	OSVersion           string            `json:"os_version"`
	OSBuild             string            `json:"os_build"`
	Architecture        string            `json:"architecture"`
	CPU                 string            `json:"cpu"`
	CPUCores            int               `json:"cpu_cores"`
	MemoryMB            int64             `json:"memory_mb"`
	DiskTotalMB         int64             `json:"disk_total_mb"`
	DiskFreeMB          int64             `json:"disk_free_mb"`
	IPAddress           string            `json:"ip_address"`
	MACAddress          string            `json:"mac_address"`
	ConsoleUser         string            `json:"console_user"`
	ConsoleUserFullName string            `json:"console_user_full_name"`
	LastUser            string            `json:"last_user"`
	FileVaultEnabled    *bool             `json:"filevault_enabled"`
	SIPEnabled          *bool             `json:"sip_enabled"`
	FirewallEnabled     *bool             `json:"firewall_enabled"`
	GatekeeperEnabled   *bool             `json:"gatekeeper_enabled"`
	SecureBootLevel     string            `json:"secure_boot_level"`
	BatteryPercent      *int              `json:"battery_percent"`
	UptimeSeconds       int64             `json:"uptime_seconds"`
	Apps                []ComputerApp     `json:"apps"`
	Profiles            []ComputerProfile `json:"profiles"`
	LocalUsers          []LocalUser       `json:"local_users"`
	ExtensionAttributes map[string]string `json:"extension_attributes"`
	Hardware            map[string]any    `json:"hardware"`
	Extra               map[string]any    `json:"extra"`
	CollectedAt         time.Time         `json:"collected_at"`
	CollectDurationMS   int64             `json:"collect_duration_ms"`
}

// AgentLogEntry is one log record shipped by the agent.
type AgentLogEntry struct {
	At        time.Time      `json:"at"`
	Level     string         `json:"level"`
	Component string         `json:"component"`
	Msg       string         `json:"msg"`
	Attrs     map[string]any `json:"attrs"`
}

// AgentLAPSConfig is what the server tells an agent about its local admin account.
// CurrentPassword is only present when a rotation is due and an escrowed password exists,
// so the agent can change the password with sysadminctl without breaking Secure Token.
type AgentLAPSConfig struct {
	Enabled         bool   `json:"enabled"`
	Account         string `json:"account"`
	CreateAccount   bool   `json:"create_account"`
	HideAccount     bool   `json:"hide_account"`
	PasswordFormat  string `json:"password_format"`
	PasswordLength  int    `json:"password_length"`
	Rotate          bool   `json:"rotate"`
	Reason          string `json:"reason,omitempty"` // missing | expired | used | requested | set | account_changed
	CurrentPassword string `json:"current_password,omitempty"`
	// NewPassword is an administrator-chosen password to apply instead of a generated one.
	NewPassword string `json:"new_password,omitempty"`
	// GrantSecureToken asks the agent to fetch one-time admin credentials (POST /agent/v1/laps/token-grant)
	// and give the account a Secure Token. CurrentPassword is included for it.
	GrantSecureToken bool `json:"grant_secure_token,omitempty"`
}

// AgentLAPSState reports the LAPS account's Secure Token and volume ownership.
type AgentLAPSState struct {
	SecureToken *bool  `json:"secure_token"`
	VolumeOwner *bool  `json:"volume_owner"` // nil: unknown or not applicable (Intel)
	GrantError  string `json:"grant_error,omitempty"`
	Granted     bool   `json:"granted,omitempty"`
}

// AgentLAPSTokenGrant carries an existing Secure Token administrator's credentials, once.
type AgentLAPSTokenGrant struct {
	AdminUser     string `json:"admin_user"`
	AdminPassword string `json:"admin_password"`
}

// AgentLAPSStage uploads a newly generated password before the agent applies it.
type AgentLAPSStage struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

// AgentLAPSCommit reports whether a staged password was applied.
type AgentLAPSCommit struct {
	Fingerprint string `json:"fingerprint"` // of the staged password being resolved
	Applied     bool   `json:"applied"`
	Error       string `json:"error,omitempty"`
	Guid        string `json:"guid,omitempty"`    // the account's GeneratedUID
	Created     bool   `json:"created,omitempty"` // the agent created the account in this rotation
}
