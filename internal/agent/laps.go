package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
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
	Created     bool   `json:"created,omitempty"` // the account did not exist before this rotation
}

// LAPSAPI is the server surface the LAPS cycle needs (the real Client implements it).
type LAPSAPI interface {
	LAPSConfig(ctx context.Context) (*model.AgentLAPSConfig, error)
	LAPSStage(ctx context.Context, account, password string) error
	LAPSCommit(ctx context.Context, c model.AgentLAPSCommit) error
	LAPSError(ctx context.Context, msg string) error
	LAPSState(ctx context.Context, st model.AgentLAPSState) error
	LAPSTokenGrant(ctx context.Context) (*model.AgentLAPSTokenGrant, error)
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
	if !cfg.Enabled {
		return nil
	}
	// A Secure Token grant uses the current password, so it runs before any rotation.
	state := l.tokenState(ctx, cfg)
	if cfg.GrantSecureToken && cfg.CurrentPassword != "" {
		state = l.grantToken(ctx, cfg, state)
	}
	if state != nil && tokenReportDue(*state) {
		if err := l.API.LAPSState(ctx, *state); err != nil {
			l.Log.Warn("reporting Secure Token state", "component", "laps", "err", err)
		} else {
			tokenReported(*state)
		}
	}
	if !cfg.Rotate {
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
	if locked, _ := laps.LockedOut(ctx, l.Run, p.Account); locked {
		// Testing now would fail even if the password is right, and every failed try keeps the
		// account locked longer. Both passwords stay visible in the console until then.
		return errLockedOut
	}
	applied := laps.Verify(ctx, l.Run, p.Account, p.Password)
	msg := ""
	if !applied {
		msg = "interrupted rotation: the new password was not in effect"
	}
	l.Log.Warn("settling an interrupted LAPS rotation", "component", "laps", "account", p.Account, "applied", applied)
	commit := model.AgentLAPSCommit{Fingerprint: p.Fingerprint, Applied: applied, Error: msg, Created: p.Created}
	if applied {
		commit.Guid = laps.GUID(ctx, l.Run, p.Account)
	}
	err = l.API.LAPSCommit(ctx, commit)
	if isConflict(err) {
		// The server never received the stage (or a newer one replaced it). Re-stage, then resolve.
		if applied {
			if err = l.API.LAPSStage(ctx, p.Account, p.Password); err == nil {
				err = l.API.LAPSCommit(ctx, commit)
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

// errLockedOut stops a LAPS cycle while the password policy locks the account.
var errLockedOut = errors.New("the LAPS account is locked by the Mac's password policy after failed sign-ins; waiting until it unlocks")

func (l *LAPS) rotate(ctx context.Context, cfg *model.AgentLAPSConfig) error {
	exists := laps.Exists(ctx, l.Run, cfg.Account)
	if exists {
		if locked, _ := laps.LockedOut(ctx, l.Run, cfg.Account); locked {
			l.API.LAPSError(ctx, errLockedOut.Error()+"; the rotation will run then")
			return errLockedOut
		}
	}
	if !exists && !cfg.CreateAccount {
		msg := fmt.Sprintf("account %q does not exist and account creation is off in the LAPS policy", cfg.Account)
		l.API.LAPSError(ctx, msg)
		return errors.New(msg)
	}
	// An administrator-chosen password (Jamf: set-password) replaces the generated one.
	pw := cfg.NewPassword
	if pw == "" {
		var err error
		if pw, err = laps.GenerateFormat(cfg.PasswordFormat, cfg.PasswordLength); err != nil {
			l.API.LAPSError(ctx, "generating a password failed")
			return err
		}
	}
	p := lapsPending{Account: cfg.Account, Password: pw, Fingerprint: laps.Fingerprint(pw), Created: !exists}
	// 1. Remember it locally, 2. give it to the server, 3. only then touch the account.
	if err := l.writePending(p); err != nil {
		l.API.LAPSError(ctx, "cannot write the pending-password file")
		return err
	}
	if err := l.API.LAPSStage(ctx, cfg.Account, pw); err != nil {
		os.Remove(l.pendingPath()) // account untouched: nothing to settle
		return fmt.Errorf("escrowing the new password: %w", err)
	}
	// SetPassword and Create confirm the change by authenticating with the new password.
	var applyErr error
	if exists {
		applyErr = laps.SetPassword(ctx, l.Run, cfg.Account, cfg.CurrentPassword, pw)
	} else {
		applyErr = laps.Create(ctx, l.Run, cfg.Account, pw)
	}
	var notApplied *laps.NotAppliedError
	if applyErr != nil && !errors.As(applyErr, &notApplied) {
		// The outcome is open (for example the account is locked out, so even the right password
		// fails). Discarding the new password could lose the only working one: keep it staged as
		// unconfirmed, and let the next cycle settle it once the account can be tested.
		msg := laps.Scrub(applyErr.Error(), pw, cfg.CurrentPassword) + "; the new password is kept as unconfirmed and checked again at the next check-in"
		l.API.LAPSError(ctx, msg)
		return errors.New(msg)
	}
	if applyErr == nil && cfg.HideAccount {
		if err := laps.Hide(ctx, l.Run, cfg.Account); err != nil {
			l.Log.Warn("could not hide the local admin account", "component", "laps", "account", cfg.Account, "err", err)
		}
	}
	msg := ""
	commit := model.AgentLAPSCommit{Fingerprint: p.Fingerprint, Applied: applyErr == nil, Created: p.Created && applyErr == nil}
	if applyErr != nil {
		msg = laps.Scrub(applyErr.Error(), pw, cfg.CurrentPassword)
		commit.Error = msg
	} else {
		commit.Guid = laps.GUID(ctx, l.Run, cfg.Account)
	}
	if err := l.API.LAPSCommit(ctx, commit); err != nil {
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

// tokenState reads the account's Secure Token and volume ownership (nil if the account is missing).
func (l *LAPS) tokenState(ctx context.Context, cfg *model.AgentLAPSConfig) *model.AgentLAPSState {
	if !laps.Exists(ctx, l.Run, cfg.Account) {
		return nil
	}
	token := laps.HasSecureToken(ctx, l.Run, cfg.Account)
	return &model.AgentLAPSState{SecureToken: &token, VolumeOwner: laps.VolumeOwner(ctx, l.Run, laps.GUID(ctx, l.Run, cfg.Account))}
}

// grantToken fetches the one-time administrator credentials and gives the account a Secure Token.
func (l *LAPS) grantToken(ctx context.Context, cfg *model.AgentLAPSConfig, state *model.AgentLAPSState) *model.AgentLAPSState {
	if state == nil {
		return nil
	}
	if locked, _ := laps.LockedOut(ctx, l.Run, cfg.Account); locked {
		return state // the grant signs in with the account's password; wait until it unlocks
	}
	g, err := l.API.LAPSTokenGrant(ctx)
	if err != nil {
		l.Log.Warn("fetching Secure Token grant credentials", "component", "laps", "err", err)
		return state
	}
	if *state.SecureToken {
		state.Granted = true // already has one (e.g. granted at login by the bootstrap token)
		return state
	}
	l.Log.Info("granting a Secure Token to the local admin account", "component", "laps", "account", cfg.Account, "admin_user", g.AdminUser)
	if err := laps.GrantSecureToken(ctx, l.Run, cfg.Account, cfg.CurrentPassword, g.AdminUser, g.AdminPassword); err != nil {
		msg := laps.Scrub(err.Error(), cfg.CurrentPassword, g.AdminPassword)
		l.Log.Warn("Secure Token grant failed", "component", "laps", "account", cfg.Account, "err", msg)
		state.GrantError = msg
		return state
	}
	l.Log.Info("Secure Token granted", "component", "laps", "account", cfg.Account)
	return l.withGranted(ctx, cfg)
}

func (l *LAPS) withGranted(ctx context.Context, cfg *model.AgentLAPSConfig) *model.AgentLAPSState {
	st := l.tokenState(ctx, cfg)
	if st == nil {
		st = &model.AgentLAPSState{}
	}
	st.Granted = true
	return st
}

// The Secure Token state is reported when it changes, after a grant attempt, or every 6 hours.
var (
	tokenMu       sync.Mutex
	tokenLast     string
	tokenLastSent time.Time
)

func tokenKey(st model.AgentLAPSState) string {
	b := func(p *bool) string {
		if p == nil {
			return "?"
		}
		return fmt.Sprint(*p)
	}
	return b(st.SecureToken) + "/" + b(st.VolumeOwner)
}

func tokenReportDue(st model.AgentLAPSState) bool {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	return st.Granted || st.GrantError != "" || tokenKey(st) != tokenLast || time.Since(tokenLastSent) > 6*time.Hour
}

func tokenReported(st model.AgentLAPSState) {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	tokenLast, tokenLastSent = tokenKey(st), time.Now()
}
