package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vibemdm/vibemdm/internal/agent"
)

// simAccounts is a tiny stand-in for the Mac's local account database, so the simulator
// runs the real agent LAPS code (generate, stage, apply, verify, commit) end to end.
type simAccounts struct {
	mu     sync.Mutex
	users  map[string]string
	tokens map[string]bool // accounts with a Secure Token
}

// simTokenAdmin is a pretend Secure Token administrator on every simulated Mac, so the
// console's Grant Secure Token flow can be tried: account "simadmin", password "simadmin-pw".
const simTokenAdmin, simTokenAdminPassword = "simadmin", "simadmin-pw"

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
	case tool == "dscl" && len(args) > 3 && args[1] == "-read" && args[3] == "GeneratedUID":
		if _, ok := a.users[strings.TrimPrefix(args[2], "/Users/")]; ok {
			return "GeneratedUID: " + strings.ToUpper(fmt.Sprintf("%x", sha256.Sum256([]byte(args[2]+fmt.Sprintf("%p", a))))[:8]) + "-SIM\n", nil
		}
		return "", errors.New("no such user")
	case tool == "sysadminctl" && len(args) > 1 && args[0] == "-secureTokenStatus":
		if a.tokens[args[1]] {
			return "Secure token is ENABLED for user " + args[1], nil
		}
		return "Secure token is DISABLED for user " + args[1], nil
	case tool == "sysadminctl" && len(args) > 7 && args[0] == "-secureTokenOn":
		if args[5] == simTokenAdmin && args[7] == simTokenAdminPassword && a.users[args[1]] == args[3] {
			if a.tokens == nil {
				a.tokens = map[string]bool{}
			}
			a.tokens[args[1]] = true
			return "", nil
		}
		return "Operation is not permitted without secure token unlock.", nil // exits 0, like the real tool
	case tool == "diskutil":
		out := "Cryptographic users for disk3s5\n"
		for u := range a.tokens {
			out += "+-- " + strings.ToUpper(fmt.Sprintf("%x", sha256.Sum256([]byte("/Users/"+u+fmt.Sprintf("%p", a))))[:8]) + "-SIM\n|   Volume Owner: Yes\n"
		}
		return out, nil
	case tool == "dscl" && len(args) > 4 && args[1] == "-create" && args[3] == "IsHidden":
		return "", nil
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
