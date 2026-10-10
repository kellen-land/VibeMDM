// Package config loads server configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all server configuration.
type Config struct {
	Listen        string
	DatabaseURL   string
	ServerURL     string // public base URL, e.g. https://mdm.example.com
	DataDir       string
	AgentDir      string // directory with prebuilt agent binaries
	AdminPassword string // initial admin password (first boot only)
	Debug         bool
	DumpMDM       bool
	APNsURL       string // override APNs server URL (testing)
	SessionTTL    time.Duration
	Version       string
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.ToLower(env(key, ""))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

// FromEnv builds a Config from VIBEMDM_* environment variables.
func FromEnv(version string) (*Config, error) {
	c := &Config{
		Listen:        env("VIBEMDM_LISTEN", ":8080"),
		DatabaseURL:   env("VIBEMDM_DATABASE_URL", ""),
		ServerURL:     strings.TrimRight(env("VIBEMDM_SERVER_URL", ""), "/"),
		DataDir:       env("VIBEMDM_DATA_DIR", "/data"),
		AgentDir:      env("VIBEMDM_AGENT_DIR", "/app/agents"),
		AdminPassword: env("VIBEMDM_ADMIN_PASSWORD", ""),
		Debug:         envBool("VIBEMDM_DEBUG", false),
		DumpMDM:       envBool("VIBEMDM_DUMP_MDM", false),
		APNsURL:       env("VIBEMDM_APNS_URL", ""),
		Version:       version,
	}
	ttlHours, err := strconv.Atoi(env("VIBEMDM_SESSION_TTL_HOURS", "168"))
	if err != nil || ttlHours < 1 {
		return nil, fmt.Errorf("invalid VIBEMDM_SESSION_TTL_HOURS")
	}
	c.SessionTTL = time.Duration(ttlHours) * time.Hour
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("VIBEMDM_DATABASE_URL is required (e.g. postgres://vibemdm:secret@db:5432/vibemdm?sslmode=disable)")
	}
	return c, nil
}
