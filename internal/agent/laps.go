package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/vibemdm/vibemdm/internal/laps"
	"github.com/vibemdm/vibemdm/internal/model"
)

// lapsPendingPath holds a freshly generated password from before it is applied until
// the server confirms it. If the agent dies between "changed the account" and
// "told the server", the next cycle finds this file and settles the question by
// testing the password against the account. Root-only (0600), like agent.json.
const lapsPendingPath = SupportDir + "/laps-pending.json"

type lapsPending struct {
	Account     string `json:"account"`
	Password    string `json:"password"`
	Fingerprint string `json:"fingerprint"`
}

// LAPSAPI is the server surface the LAPS cycle needs (the real Client implements it).
type LAPSAPI interface {
	LAPSConfig(ctx context.Context) (*model.AgentLAPSConfig, error)
	LAPSStage(ctx context.Context, account, password string) error
	LAPSCommit(ctx context.Context, fingerprint string, applied bool, errMsg string) error
	LAPSError(ctx context.Context, msg string) error
}

// LAPS manages the local administrator account's password.
type LAPS struct {
	API         LAPSAPI
	Run         laps.Runner
	PendingPath string
	Log         interface {
		Info(msg string, args ...any)
		Warn(msg string, args ...any)
	}
}

// RunCommand is the production laps.Runner.
func RunCommand(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

func (l *LAPS) pendingPath() string {
	if l.PendingPath != "" {
		return l.PendingPath
	}
	return lapsPendingPath
}

func (l *LAPS) readPending() (*lapsPending, error) {
	b, err := os.ReadFile(l.pendingPath())
	if err != nil {
		return nil, err
	}
	var p lapsPending
	if err := json.Unmarshal(b, &p); err != nil || p.Password == "" {
		os.Remove(l.pendingPath()) // unreadable: nothing recoverable
		return nil, os.ErrNotExist
	}
	return &p, nil
}

func (l *LAPS) writePending(p lapsPending) error {
	b, _ := json.Marshal(p)
	if err := os.MkdirAll(dirOf(l.pendingPath()), 0o755); err != nil {
		return err
	}
	tmp := l.pendingPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, l.pendingPath())
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}

// Cycle settles any unconfirmed password from an earlier interrupted run, then
// rotates if the server says it is due.
func (l *LAPS) Cycle(ctx context.Context) error {
	if err := l.settlePending(ctx); err != nil {
		return err
	}
	cfg, err := l.API.LAPSConfig(ctx)
	if err != nil {
		return err
	}
	if !cfg.Enabled || !cfg.Rotate {
		return nil
	}
	l.Log.Info("rotating local admin password", "component", "laps", "account", cfg.Account, "reason", cfg.Reason)
	start := time.Now()
	if err := l.rotate(ctx, cfg); err != nil {
		l.Log.Warn("local admin password rotation failed", "component", "laps", "account", cfg.Account, "err", err)
		return err
	}
	l.Log.Info("local admin password rotated", "component", "laps", "account", cfg.Account, "dur", time.Since(start).Round(time.Millisecond))
	return nil
}

// settlePending resolves a leftover pending password by testing it against the account.
func (l *LAPS) settlePending(ctx context.Context) error {
	p, err := l.readPending()
	if err != nil {
		return nil // none
	}
	applied := laps.Verify(ctx, l.Run, p.Account, p.Password)
	msg := ""
	if !applied {
		msg = "interrupted rotation: the new password was not in effect"
	}
	l.Log.Warn("settling an interrupted LAPS rotation", "component", "laps", "account", p.Account, "applied", applied)
	err = l.API.LAPSCommit(ctx, p.Fingerprint, applied, msg)
	if isConflict(err) {
		// The server never received the stage (or a newer one replaced it). Re-stage, then resolve.
		if applied {
			if err = l.API.LAPSStage(ctx, p.Account, p.Password); err == nil {
				err = l.API.LAPSCommit(ctx, p.Fingerprint, true, "")
			}
		} else {
			err = nil // nothing to discard
		}
	}
	if err != nil {
		return fmt.Errorf("settling the previous rotation: %w", err)
	}
	os.Remove(l.pendingPath())
	return nil
}

func isConflict(err error) bool {
	var ae *apiError
	return errors.As(err, &ae) && ae.Status == 409
}

func (l *LAPS) rotate(ctx context.Context, cfg *model.AgentLAPSConfig) error {
	exists := laps.Exists(ctx, l.Run, cfg.Account)
	if !exists && !cfg.CreateAccount {
		msg := fmt.Sprintf("account %q does not exist and account creation is off in the LAPS policy", cfg.Account)
		l.API.LAPSError(ctx, msg)
		return errors.New(msg)
	}
	pw, err := laps.Generate(cfg.PasswordLength)
	if err != nil {
		l.API.LAPSError(ctx, "generating a password failed")
		return err
	}
	p := lapsPending{Account: cfg.Account, Password: pw, Fingerprint: laps.Fingerprint(pw)}
	// 1. Remember it locally, 2. give it to the server, 3. only then touch the account.
	if err := l.writePending(p); err != nil {
		l.API.LAPSError(ctx, "cannot write the pending-password file")
		return err
	}
	if err := l.API.LAPSStage(ctx, cfg.Account, pw); err != nil {
		os.Remove(l.pendingPath()) // account untouched: nothing to settle
		return fmt.Errorf("escrowing the new password: %w", err)
	}
	var applyErr error
	if exists {
		applyErr = laps.SetPassword(ctx, l.Run, cfg.Account, cfg.CurrentPassword, pw)
	} else {
		applyErr = laps.Create(ctx, l.Run, cfg.Account, pw)
	}
	// Trust the account, not the exit status: a tool can fail after the change took effect.
	if laps.Verify(ctx, l.Run, cfg.Account, pw) {
		applyErr = nil
	} else if applyErr == nil {
		applyErr = errors.New("the new password did not authenticate after the change")
	}
	msg := ""
	if applyErr != nil {
		msg = laps.Scrub(applyErr.Error(), pw, cfg.CurrentPassword)
	}
	if err := l.API.LAPSCommit(ctx, p.Fingerprint, applyErr == nil, msg); err != nil {
		// Keep the pending file: the next cycle verifies and reports.
		return fmt.Errorf("reporting the result: %w", err)
	}
	os.Remove(l.pendingPath())
	if applyErr != nil {
		return errors.New(msg)
	}
	return nil
}

// LAPSCycle runs one LAPS pass for this Mac.
func (e *Executor) LAPSCycle(ctx context.Context) error {
	l := &LAPS{API: e.client, Run: RunCommand, Log: e.log}
	return l.Cycle(ctx)
}
