package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Paths used by the agent on macOS.
const (
	SupportDir   = "/Library/Application Support/VibeMDM"
	ConfigPath   = SupportDir + "/agent.json"
	DownloadsDir = SupportDir + "/Downloads"
	TmpDir       = SupportDir + "/tmp"
	LogPath      = "/var/log/vibemdm-agent.log"
	SocketPath   = "/var/run/vibemdm-agent.sock"
	LockPath     = "/var/run/vibemdm-agent.lock"
	BinaryPath   = "/usr/local/bin/vibemdm-agent"
	DaemonLabel  = "com.vibemdm.agent"
	DaemonPlist  = "/Library/LaunchDaemons/" + DaemonLabel + ".plist"
	LoginLabel   = "com.vibemdm.agent.login"
	LoginPlist   = "/Library/LaunchAgents/" + LoginLabel + ".plist"
)

// Config is persisted at ConfigPath.
type Config struct {
	ServerURL              string `json:"server_url"`
	DeviceToken            string `json:"device_token"`
	ComputerID             string `json:"computer_id"`
	OrgName                string `json:"org_name"`
	CheckinIntervalSeconds int    `json:"checkin_interval_seconds"`
	EnrolledAt             string `json:"enrolled_at"`
}

// ErrNotEnrolled indicates no config file.
var ErrNotEnrolled = errors.New("agent is not enrolled; run: vibemdm-agent enroll --server URL --token TOKEN")

// LoadConfig reads the config file.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = ConfigPath
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotEnrolled
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.ServerURL == "" || c.DeviceToken == "" {
		return nil, ErrNotEnrolled
	}
	if c.CheckinIntervalSeconds < 30 {
		c.CheckinIntervalSeconds = 300
	}
	return &c, nil
}

// Save writes the config with restrictive permissions.
func (c *Config) Save(path string) error {
	if path == "" {
		path = ConfigPath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
