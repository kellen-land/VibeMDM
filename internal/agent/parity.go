package agent

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

// ---- Installomator-backed app installs ----

const installomatorPath = SupportDir + "/Installomator.sh"

// ensureInstallomator downloads the server's vendored Installomator copy when
// missing or older than a day.
func (e *Executor) ensureInstallomator(ctx context.Context) (string, error) {
	if fi, err := os.Stat(installomatorPath); err == nil && time.Since(fi.ModTime()) < 24*time.Hour && fi.Size() > 100_000 {
		return installomatorPath, nil
	}
	tmp := installomatorPath + ".tmp"
	if err := e.client.DownloadTo(ctx, "/agent/v1/installomator", tmp); err != nil {
		if _, serr := os.Stat(installomatorPath); serr == nil {
			return installomatorPath, nil // stale copy beats no copy
		}
		return "", fmt.Errorf("downloading Installomator: %w", err)
	}
	os.Chmod(tmp, 0o700)
	if err := os.Rename(tmp, installomatorPath); err != nil {
		return "", err
	}
	return installomatorPath, nil
}

// InstallApp runs an app-catalog install through Installomator and reports the result.
// InstallApp runs an app-catalog install and reports it; it returns whether it succeeded.
func (e *Executor) InstallApp(ctx context.Context, app model.AgentAppInstall) bool {
	log := e.log.With("component", "apps", "app", app.Name, "label", app.Label, "log_id", app.LogID)
	log.Info("app install started")
	start := time.Now()
	status, logText, exit := "completed", "", 0
	script, err := e.ensureInstallomator(ctx)
	if err != nil {
		status, logText, exit = "failed", err.Error(), -1
	} else {
		r := e.runInstallomator(ctx, script, app.Label, app.Args)
		logText, exit = r.Combined, r.ExitCode
		if r.ExitCode != 0 {
			status = "failed"
		}
	}
	if status == "completed" {
		log.Info("app install finished", "status", status, "dur", time.Since(start), "output", Summarize(logText, 1, 200))
	} else {
		log.Warn("app install finished", "status", status, "exit", exit, "dur", time.Since(start), "output", Summarize(logText, 3, 500))
	}
	if err := e.client.SubmitAppInstallResult(ctx, app.LogID, status, logText); err != nil {
		log.Error("submitting app install result", "err", err)
	}
	return status == "completed"
}

// ---- restricted software enforcement ----

// RestrictedMonitor enforces restricted-software rules between check-ins.
type RestrictedMonitor struct {
	client *Client
	log    interface {
		Info(string, ...any)
		Warn(string, ...any)
	}
	mu       sync.Mutex
	rules    []model.AgentRestrictedRule
	notified map[string]time.Time // rule+pid -> last action
}

// NewRestrictedMonitor creates a monitor.
func NewRestrictedMonitor(client *Client, log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) *RestrictedMonitor {
	return &RestrictedMonitor{client: client, log: log, notified: map[string]time.Time{}}
}

// SetRules replaces the active rule set (from the latest check-in).
func (m *RestrictedMonitor) SetRules(rules []model.AgentRestrictedRule) {
	m.mu.Lock()
	m.rules = rules
	m.mu.Unlock()
}

// Run scans processes periodically until ctx ends.
func (m *RestrictedMonitor) Run(ctx context.Context) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.sweep(ctx)
		}
	}
}

type proc struct {
	pid  int
	user string
	comm string
}

func listProcs(ctx context.Context) []proc {
	out := run(ctx, 10*time.Second, "ps", "-axo", "pid=,user=,comm=")
	var procs []proc
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		procs = append(procs, proc{pid: pid, user: f[1], comm: strings.Join(f[2:], " ")})
	}
	return procs
}

func (m *RestrictedMonitor) matches(rule model.AgentRestrictedRule, comm string) bool {
	base := filepath.Base(comm)
	switch rule.MatchType {
	case "contains":
		return strings.Contains(strings.ToLower(comm), strings.ToLower(rule.ProcessName))
	case "regex":
		re, err := regexp.Compile("(?i)" + rule.ProcessName)
		return err == nil && (re.MatchString(comm) || re.MatchString(base))
	default:
		return strings.EqualFold(base, rule.ProcessName)
	}
}

func (m *RestrictedMonitor) sweep(ctx context.Context) {
	m.mu.Lock()
	rules := m.rules
	m.mu.Unlock()
	if len(rules) == 0 {
		return
	}
	procs := listProcs(ctx)
	for _, rule := range rules {
		for _, p := range procs {
			if p.pid <= 1 || !m.matches(rule, p.comm) {
				continue
			}
			key := rule.ID + ":" + strconv.Itoa(p.pid)
			m.mu.Lock()
			last, seen := m.notified[key]
			if seen && time.Since(last) < 5*time.Minute {
				m.mu.Unlock()
				continue
			}
			m.notified[key] = time.Now()
			if len(m.notified) > 4096 {
				m.notified = map[string]time.Time{}
			}
			m.mu.Unlock()

			killed := false
			if rule.KillProcess {
				if proc, err := os.FindProcess(p.pid); err == nil {
					killed = proc.Kill() == nil
				}
			}
			m.log.Warn("restricted software detected", "rule", rule.Name, "process", p.comm, "pid", p.pid, "user", p.user, "killed", killed)
			if rule.NotifyUser {
				msg := rule.DisplayMessage
				if msg == "" {
					msg = fmt.Sprintf("%s is not allowed on this computer.", rule.Name)
				}
				Notify(ctx, "Restricted Software", msg)
			}
			rep := model.AgentRestrictedReport{RuleID: rule.ID, RuleName: rule.Name, Process: filepath.Base(p.comm), Username: p.user, Killed: killed}
			if err := m.client.ReportRestricted(ctx, rep); err != nil {
				m.log.Warn("reporting restricted event", "err", err)
			}
		}
	}
}

var _ = http.StatusOK

// ---- FileVault key remediation (Escrow Buddy) ----

// Escrow Buddy (github.com/macadmins/escrow-buddy, Apache-2.0) is an
// authorization plugin that generates a new FileVault personal recovery key at
// the next login by a FileVault-enabled user; the FDERecoveryKeyEscrow profile
// then makes macOS escrow it to the MDM.
const (
	escrowBuddyBundle    = "/Library/Security/SecurityAgentPlugins/Escrow Buddy.bundle"
	escrowBuddyPrefs     = "/Library/Preferences/com.netflix.Escrow-Buddy"
	escrowBuddyMechanism = "Escrow Buddy:Invoke,privileged"
)

func escrowBuddyMechanismPresent(ctx context.Context) bool {
	out := run(ctx, 10*time.Second, "/usr/bin/security", "authorizationdb", "read", "system.login.console")
	return strings.Contains(out, escrowBuddyMechanism)
}

// RegenerateFileVaultKey installs/repairs Escrow Buddy and arms key generation.
func (e *Executor) RegenerateFileVaultKey(ctx context.Context) ExecResult {
	start := time.Now()
	var out strings.Builder
	if !strings.Contains(run(ctx, 15*time.Second, "/usr/bin/fdesetup", "status"), "FileVault is On") {
		return ExecResult{ExitCode: 1, Combined: "FileVault is not enabled on this Mac; nothing to escrow.", Duration: time.Since(start)}
	}
	_, bundleErr := os.Stat(escrowBuddyBundle)
	// macOS updates can reset the authorization database, deactivating the
	// plugin, so a missing mechanism also triggers a (forced) reinstall.
	if bundleErr != nil || !escrowBuddyMechanismPresent(ctx) {
		script, err := e.ensureInstallomator(ctx)
		if err != nil {
			return ExecResult{ExitCode: -1, Err: err, Combined: err.Error(), Duration: time.Since(start)}
		}
		out.WriteString("Installing Escrow Buddy with Installomator…\n")
		ictx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		r := runCmd(exec.CommandContext(ictx, "/bin/zsh", append([]string{script}, installomatorArgs([]string{"escrowbuddy", "NOTIFY=silent", "INSTALL=force"})...)...))
		cancel()
		out.WriteString(r.Combined)
		if r.ExitCode != 0 {
			return ExecResult{ExitCode: r.ExitCode, Combined: out.String(), Duration: time.Since(start)}
		}
	}
	if !escrowBuddyMechanismPresent(ctx) {
		out.WriteString("\nEscrow Buddy is installed but its authorization mechanism is missing from system.login.console.")
		return ExecResult{ExitCode: 1, Combined: out.String(), Duration: time.Since(start)}
	}
	r := runCmd(exec.CommandContext(ctx, "/usr/bin/defaults", "write", escrowBuddyPrefs, "GenerateNewKey", "-bool", "true"))
	out.WriteString(r.Combined)
	if r.ExitCode != 0 {
		return ExecResult{ExitCode: r.ExitCode, Combined: out.String(), Duration: time.Since(start)}
	}
	out.WriteString("\nEscrow Buddy armed. A new FileVault recovery key will be generated at the next login by a FileVault-enabled user and escrowed to the server.")
	return ExecResult{Combined: out.String(), Stdout: out.String(), Duration: time.Since(start)}
}

// runInstallomator runs Installomator with server-built arguments. Arguments
// are passed as argv (no shell); the server single-quotes every value because
// Installomator evaluates key=value arguments itself.
func (e *Executor) runInstallomator(ctx context.Context, script, label string, args []string) ExecResult {
	if len(args) == 0 { // older servers: label with defaults
		args = []string{label, "NOTIFY=silent", "BLOCKING_PROCESS_ACTION=prompt_user"}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/zsh", append([]string{script}, installomatorArgs(args)...)...)
	cmd.Dir = "/"
	return runCmd(cmd)
}

// RunAppStep runs one Installomator step inside a policy.
func (e *Executor) RunAppStep(ctx context.Context, step model.AgentAppStep) ExecResult {
	script, err := e.ensureInstallomator(ctx)
	if err != nil {
		return ExecResult{ExitCode: -1, Err: err, Combined: err.Error()}
	}
	return e.runInstallomator(ctx, script, step.Label, step.Args)
}

// installomatorArgs makes sure Installomator really installs. The script defaults to DEBUG=1
// (download only, exit 0); servers that predate the fix do not send DEBUG=0.
func installomatorArgs(args []string) []string {
	for _, a := range args {
		if strings.HasPrefix(a, "DEBUG=") {
			return args
		}
	}
	return append(append([]string{}, args...), "DEBUG=0")
}
