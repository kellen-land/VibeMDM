// Package config loads server configuration from the environment.
package config

import (
	"fmt"
	"log/slog"
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
	AssetsDir     string // uploaded and built files Macs download (default: DataDir)
	AgentDir      string // directory with prebuilt agent binaries
	AdminPassword string // initial admin password (first boot only)
	Debug         bool
	DumpMDM       bool
	APNsURL       string     // override APNs server URL (testing)
	EncryptionKey string     // passphrase for at-rest encryption of secrets
	LogDBLevel    slog.Level // minimum level stored in the searchable log table
	SessionTTL    time.Duration
	Version       string
	// TrustedProxies may set X-Forwarded-For (CIDRs/IPs, comma separated).
	TrustedProxies []string
	// AllowHTTP permits a plain-http server URL (labs only: agents would talk without TLS).
	AllowHTTP bool
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
		AssetsDir:     env("VIBEMDM_ASSETS_DIR", ""),
		AdminPassword: env("VIBEMDM_ADMIN_PASSWORD", ""),
		Debug:         envBool("VIBEMDM_DEBUG", false),
		DumpMDM:       envBool("VIBEMDM_DUMP_MDM", false),
		APNsURL:       env("VIBEMDM_APNS_URL", ""),
		EncryptionKey: env("VIBEMDM_ENCRYPTION_KEY", ""),
		Version:       version,
		AllowHTTP:     envBool("VIBEMDM_ALLOW_HTTP", false),
	}
	if c.AssetsDir == "" {
		c.AssetsDir = c.DataDir
	}
	for _, p := range strings.Split(env("VIBEMDM_TRUSTED_PROXIES", ""), ",") {
		if p = strings.TrimSpace(p); p != "" {
			c.TrustedProxies = append(c.TrustedProxies, p)
		}
	}
	ttlHours, err := strconv.Atoi(env("VIBEMDM_SESSION_TTL_HOURS", "168"))
	if err != nil || ttlHours < 1 {
		return nil, fmt.Errorf("invalid VIBEMDM_SESSION_TTL_HOURS")
	}
	c.SessionTTL = time.Duration(ttlHours) * time.Hour
	if err := c.LogDBLevel.UnmarshalText([]byte(env("VIBEMDM_LOG_DB_LEVEL", "info"))); err != nil {
		return nil, fmt.Errorf("invalid VIBEMDM_LOG_DB_LEVEL (use debug, info, warn or error)")
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("VIBEMDM_DATABASE_URL is required (e.g. postgres://vibemdm:secret@db:5432/vibemdm?sslmode=disable)")
	}
	return c, nil
}
