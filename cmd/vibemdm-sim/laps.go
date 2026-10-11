package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vibemdm/vibemdm/internal/agent"
)

// simAccounts is a tiny stand-in for the Mac's local account database, so the simulator
// runs the real agent LAPS code (generate, stage, apply, verify, commit) end to end.
type simAccounts struct {
	mu    sync.Mutex
	users map[string]string
}

func (a *simAccounts) run(_ context.Context, name string, args ...string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	tool := filepath.Base(name)
	arg := func(flag string) string {
		for i, v := range args {
			if v == flag && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}
	switch {
	case tool == "dscl" && len(args) > 2 && args[1] == "-read":
		if _, ok := a.users[strings.TrimPrefix(args[2], "/Users/")]; ok {
			return "", nil
		}
		return "", errors.New("no such user")
	case tool == "dscl" && len(args) > 3 && args[1] == "-authonly":
		if pw, ok := a.users[args[2]]; ok && pw == args[3] {
			return "", nil
		}
		return "", errors.New("authentication failed")
	case tool == "dscl" && len(args) > 3 && args[1] == "-passwd":
		a.users[strings.TrimPrefix(args[2], "/Users/")] = args[3]
		return "", nil
	case tool == "sysadminctl" && len(args) > 1 && args[0] == "-addUser":
		a.users[args[1]] = arg("-password")
		return "", nil
	case tool == "sysadminctl" && len(args) > 1 && args[0] == "-resetPasswordFor":
		if a.users[args[1]] != arg("-oldPassword") {
			return "old password mismatch", errors.New("exit 1")
		}
		a.users[args[1]] = arg("-newPassword")
		return "", nil
	}
	return "", errors.New("unsupported command in simulator")
}

func (d *device) lapsCycle(ctx context.Context, c *agent.Client) error {
	if d.accounts == nil {
		d.accounts = &simAccounts{users: map[string]string{}}
	}
	l := &agent.LAPS{API: c, Run: d.accounts.run, Log: d.log,
		PendingPath: filepath.Join(os.TempDir(), "vibemdm-sim-laps-"+d.serial+".json")}
	return l.Cycle(ctx)
}

func (d *device) laps(ctx context.Context, c *agent.Client) {
	if err := d.lapsCycle(ctx, c); err != nil {
		d.log.Warn("simulated LAPS cycle failed", "err", err)
	}
}
