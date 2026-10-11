package agent

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// ManagedPrefsPath is where macOS writes the com.vibemdm.agent managed preferences that the
// MDM bootstrap profile installs (internal/agentinstall).
const ManagedPrefsPath = "/Library/Managed Preferences/com.vibemdm.agent"

// MDMBootstrap is what the bootstrap profile hands to the agent package's postinstall.
type MDMBootstrap struct {
	ServerURL   string
	EnrollToken string
}

// ReadPref reads one managed preference value ("" if absent).
type ReadPref func(ctx context.Context, key string) string

func defaultsRead(ctx context.Context, key string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", ManagedPrefsPath, key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// WaitForMDMBootstrap polls managed preferences until the server URL and token appear or
// wait elapses. The profile and the package are sent together, so the profile is usually
// in place before the postinstall runs, but the order is not guaranteed.
func WaitForMDMBootstrap(ctx context.Context, wait time.Duration, read ReadPref) (*MDMBootstrap, error) {
	if read == nil {
		read = defaultsRead
	}
	deadline := time.Now().Add(wait)
	for {
		b := &MDMBootstrap{ServerURL: read(ctx, "ServerURL"), EnrollToken: read(ctx, "EnrollToken")}
		if b.ServerURL != "" && b.EnrollToken != "" {
			return b, nil
		}
		if time.Now().After(deadline) {
			return nil, errors.New("no enrollment settings from MDM (" + ManagedPrefsPath + "); is the Mac enrolled and was the agent sent from VibeMDM?")
		}
		if !sleepCtx(ctx, 5*time.Second) {
			return nil, ctx.Err()
		}
	}
}
