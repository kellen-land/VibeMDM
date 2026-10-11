package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

const (
	maxOutput     = 64 * 1024
	scriptTimeout = 2 * time.Hour
)

// Executor runs scripts, packages and policies.
type Executor struct {
	client  *Client
	cfg     *Config
	log     *slog.Logger
	version string

	// OnRestrictedRules, when set, receives the restricted-software rule set
	// returned by each check-in (the daemon points this at its monitor).
	OnRestrictedRules func([]model.AgentRestrictedRule)
}

// NewExecutor creates an executor.
func NewExecutor(client *Client, cfg *Config, version string, logger *slog.Logger) *Executor {
	return &Executor{client: client, cfg: cfg, log: logger, version: version}
}

// ExecResult is a captured command result.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Combined string
	Duration time.Duration
	Err      error
}

func truncate(s string) string {
	if len(s) > maxOutput {
		return s[:maxOutput] + "\n…[output truncated]"
	}
	return s
}

// RunScript writes the script to a temp file and executes it with the
// standard parameter convention: $1 mount point, $2 computer name,
// $3 console user, $4–$11 custom parameters.
func (e *Executor) RunScript(ctx context.Context, name, contents string, params []string, env map[string]string) ExecResult {
	os.MkdirAll(TmpDir, 0o700)
	f, err := os.CreateTemp(TmpDir, "script-*.sh")
	if err != nil {
		return ExecResult{ExitCode: -1, Err: err}
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.WriteString(contents); err != nil {
		f.Close()
		return ExecResult{ExitCode: -1, Err: err}
	}
	f.Close()
	os.Chmod(path, 0o700)

	si := CollectSystemInfo(ctx)
	args := []string{"/", si.ComputerName, si.ConsoleUser}
	for i := 0; i < 8; i++ {
		if i < len(params) {
			args = append(args, params[i])
		} else {
			args = append(args, "")
		}
	}
	var cmd *exec.Cmd
	ctx, cancel := context.WithTimeout(ctx, scriptTimeout)
	defer cancel()
	if strings.HasPrefix(contents, "#!") {
		cmd = exec.CommandContext(ctx, path, args...)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/bash", append([]string{path}, args...)...)
	}
	cmd.Dir = "/"
	cmd.Env = append(os.Environ(), "VIBEMDM_SERVER_URL="+e.cfg.ServerURL, "VIBEMDM_COMPUTER_ID="+e.cfg.ComputerID, "VIBEMDM_SCRIPT_NAME="+name)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return runCmd(cmd)
}

// RunCommand runs a shell command string.
func (e *Executor) RunCommand(ctx context.Context, command string) ExecResult {
	ctx, cancel := context.WithTimeout(ctx, scriptTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", command)
	cmd.Dir = "/"
	cmd.Env = append(os.Environ(), "VIBEMDM_SERVER_URL="+e.cfg.ServerURL, "VIBEMDM_COMPUTER_ID="+e.cfg.ComputerID)
	return runCmd(cmd)
}

func runCmd(cmd *exec.Cmd) ExecResult {
	var stdout, stderr, combined bytes.Buffer
	cmd.Stdout = io.MultiWriter(&stdout, &combined)
	cmd.Stderr = io.MultiWriter(&stderr, &combined)
	start := time.Now()
	err := cmd.Run()
	res := ExecResult{Stdout: truncate(stdout.String()), Stderr: truncate(stderr.String()), Combined: truncate(combined.String()), Duration: time.Since(start)}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
			res.Err = err
		}
	}
	return res
}

// cachePath returns the local cache location for a package (keyed by checksum so
// re-runs and other policies referencing the same package never re-download).
func cachePath(p model.AgentPackageStep) string {
	key := p.SHA256
	if key == "" {
		key = p.PackageID
	}
	return filepath.Join(DownloadsDir, key+"-"+filepath.Base(p.Filename))
}

// FetchPackage downloads a package into the cache (if not already present and valid).
func (e *Executor) FetchPackage(ctx context.Context, p model.AgentPackageStep) (string, error) {
	os.MkdirAll(DownloadsDir, 0o700)
	dst := cachePath(p)
	if p.SHA256 != "" {
		if sum, err := fileSHA256(dst); err == nil && strings.EqualFold(sum, p.SHA256) {
			e.log.Debug("package cache hit", "component", "package", "package", p.Name)
			return dst, nil
		}
	}
	log := e.log.With("component", "package", "package", p.Name, "package_id", p.PackageID)
	log.Info("downloading package", "file", filepath.Base(p.Filename))
	start := time.Now()
	tmp := dst + ".part"
	if err := e.client.DownloadPackage(ctx, p.URL, tmp); err != nil {
		os.Remove(tmp)
		log.Warn("package download failed", "dur", time.Since(start), "err", err)
		return "", fmt.Errorf("download: %w", err)
	}
	if p.SHA256 != "" {
		sum, err := fileSHA256(tmp)
		if err != nil || !strings.EqualFold(sum, p.SHA256) {
			os.Remove(tmp)
			log.Error("package checksum mismatch; refusing to install", "want_sha256", p.SHA256, "got_sha256", sum)
			return "", errors.New("checksum mismatch; refusing to install")
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	var size int64
	if st, err := os.Stat(dst); err == nil {
		size = st.Size()
	}
	log.Info("package downloaded", "bytes", size, "dur", time.Since(start))
	return dst, nil
}

// PrefetchPackages downloads all packages concurrently (bounded) before installation starts.
func (e *Executor) PrefetchPackages(ctx context.Context, pkgs []model.AgentPackageStep) map[string]error {
	errs := make(map[string]error, len(pkgs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	for _, p := range pkgs {
		wg.Add(1)
		go func(p model.AgentPackageStep) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			_, err := e.FetchPackage(ctx, p)
			mu.Lock()
			errs[p.PackageID] = err
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return errs
}

// PruneCache removes cached packages older than 14 days.
func PruneCache() {
	entries, err := os.ReadDir(DownloadsDir)
	if err != nil {
		return
	}
	for _, en := range entries {
		info, err := en.Info()
		if err == nil && time.Since(info.ModTime()) > 14*24*time.Hour {
			os.Remove(filepath.Join(DownloadsDir, en.Name()))
		}
	}
}

// InstallPackage downloads (or reuses from cache), verifies and installs a package.
func (e *Executor) InstallPackage(ctx context.Context, p model.AgentPackageStep) ExecResult {
	start := time.Now()
	dst, err := e.FetchPackage(ctx, p)
	if err != nil {
		return ExecResult{ExitCode: -1, Err: err, Combined: err.Error(), Duration: time.Since(start)}
	}
	var res ExecResult
	switch strings.ToLower(filepath.Ext(dst)) {
	case ".pkg", ".mpkg":
		res = runCmd(exec.CommandContext(ctx, "/usr/sbin/installer", "-verboseR", "-pkg", dst, "-target", "/"))
	case ".dmg":
		res = e.installDMG(ctx, dst)
	case ".zip":
		res = e.installZip(ctx, dst)
	default:
		res = ExecResult{ExitCode: -1, Err: errors.New("unsupported package type"), Combined: "unsupported package type"}
	}
	res.Duration = time.Since(start)
	return res
}

func (e *Executor) installDMG(ctx context.Context, dmg string) ExecResult {
	mount, err := os.MkdirTemp(TmpDir, "mnt-")
	if err != nil {
		return ExecResult{ExitCode: -1, Err: err}
	}
	defer os.RemoveAll(mount)
	att := runCmd(exec.CommandContext(ctx, "hdiutil", "attach", "-nobrowse", "-readonly", "-noverify", "-mountpoint", mount, dmg))
	if att.ExitCode != 0 {
		att.Combined = "hdiutil attach failed:\n" + att.Combined
		return att
	}
	defer exec.Command("hdiutil", "detach", mount, "-force").Run()
	entries, _ := os.ReadDir(mount)
	var out strings.Builder
	exit := 0
	for _, en := range entries {
		p := filepath.Join(mount, en.Name())
		switch strings.ToLower(filepath.Ext(en.Name())) {
		case ".pkg", ".mpkg":
			r := runCmd(exec.CommandContext(ctx, "/usr/sbin/installer", "-verboseR", "-pkg", p, "-target", "/"))
			out.WriteString(r.Combined)
			if r.ExitCode != 0 {
				exit = r.ExitCode
			}
		case ".app":
			target := filepath.Join("/Applications", en.Name())
			os.RemoveAll(target)
			r := runCmd(exec.CommandContext(ctx, "ditto", p, target))
			out.WriteString(fmt.Sprintf("copied %s to /Applications\n%s", en.Name(), r.Combined))
			if r.ExitCode != 0 {
				exit = r.ExitCode
			}
		}
	}
	if out.Len() == 0 {
		return ExecResult{ExitCode: 1, Combined: "no .pkg or .app found in disk image"}
	}
	return ExecResult{ExitCode: exit, Combined: truncate(out.String())}
}

func (e *Executor) installZip(ctx context.Context, zipPath string) ExecResult {
	dir, err := os.MkdirTemp(TmpDir, "zip-")
	if err != nil {
		return ExecResult{ExitCode: -1, Err: err}
	}
	defer os.RemoveAll(dir)
	r := runCmd(exec.CommandContext(ctx, "ditto", "-x", "-k", zipPath, dir))
	if r.ExitCode != 0 {
		return r
	}
	entries, _ := os.ReadDir(dir)
	var out strings.Builder
	exit := 0
	for _, en := range entries {
		p := filepath.Join(dir, en.Name())
		switch strings.ToLower(filepath.Ext(en.Name())) {
		case ".pkg", ".mpkg":
			rr := runCmd(exec.CommandContext(ctx, "/usr/sbin/installer", "-verboseR", "-pkg", p, "-target", "/"))
			out.WriteString(rr.Combined)
			if rr.ExitCode != 0 {
				exit = rr.ExitCode
			}
		case ".app":
			target := filepath.Join("/Applications", en.Name())
			os.RemoveAll(target)
			rr := runCmd(exec.CommandContext(ctx, "ditto", p, target))
			out.WriteString(fmt.Sprintf("copied %s to /Applications\n%s", en.Name(), rr.Combined))
			if rr.ExitCode != 0 {
				exit = rr.ExitCode
			}
		}
	}
	if out.Len() == 0 {
		return ExecResult{ExitCode: 1, Combined: "no .pkg or .app found in archive"}
	}
	return ExecResult{ExitCode: exit, Combined: truncate(out.String())}
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Notify shows a notification to the console user.
func Notify(ctx context.Context, title, message string) {
	user := ConsoleUser(ctx)
	if user == "" || message == "" {
		return
	}
	uid := ConsoleUserUID(ctx, user)
	script := fmt.Sprintf(`display notification %q with title %q`, message, title)
	exec.CommandContext(ctx, "launchctl", "asuser", fmt.Sprint(uid), "sudo", "-u", user, "osascript", "-e", script).Run()
}

// Dialog shows a blocking dialog to the console user (best effort, 2 minute timeout).
func Dialog(ctx context.Context, title, message string) {
	user := ConsoleUser(ctx)
	if user == "" || message == "" {
		return
	}
	uid := ConsoleUserUID(ctx, user)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	script := fmt.Sprintf(`display dialog %q with title %q buttons {"OK"} default button "OK" giving up after 110`, message, title)
	exec.CommandContext(ctx, "launchctl", "asuser", fmt.Sprint(uid), "sudo", "-u", user, "osascript", "-e", script).Run()
}

// RunPolicy executes a policy and submits the log. Returns the submitted log.
func (e *Executor) RunPolicy(ctx context.Context, p model.AgentPolicy) model.AgentPolicyLogSubmit {
	log := e.log.With("component", "policy", "policy", p.Name, "policy_id", p.PolicyID)
	log.Info("policy started", "trigger", p.Trigger, "log_id", p.LogID, "scripts", len(p.Scripts), "packages", len(p.Packages),
		"apps", len(p.Apps), "command", strings.TrimSpace(p.RunCommand) != "", "inventory", p.UpdateInventory)
	sub := model.AgentPolicyLogSubmit{Trigger: p.Trigger, StartedAt: time.Now().UTC(), Steps: []model.PolicyStep{}}
	var logb strings.Builder
	failed := false
	logf := func(format string, a ...any) {
		line := fmt.Sprintf(format, a...)
		logb.WriteString(time.Now().Format("15:04:05") + " " + line + "\n")
	}
	addStep := func(typ, name string, r ExecResult) {
		st := "completed"
		if r.ExitCode != 0 {
			st = "failed"
			failed = true
		}
		out := r.Combined
		if r.Err != nil {
			out = r.Err.Error() + "\n" + out
		}
		sub.Steps = append(sub.Steps, model.PolicyStep{Type: typ, Name: name, Status: st, ExitCode: r.ExitCode, Output: out, DurationMS: r.Duration.Milliseconds()})
		logf("%s %q %s (exit %d, %s)", typ, name, st, r.ExitCode, r.Duration.Round(time.Millisecond))
		logStep(log, typ, name, st, r, out)
	}
	logf("Executing policy %q (trigger: %s)", p.Name, p.Trigger)
	if p.UserInteraction.StartMessage != "" {
		Notify(ctx, p.Name, p.UserInteraction.StartMessage)
	}
	runScripts := func(prio string) {
		for _, s := range p.Scripts {
			if s.Priority != prio {
				continue
			}
			logf("Running script %q", s.Name)
			r := e.RunScript(ctx, s.Name, s.Contents, s.Parameters, map[string]string{"VIBEMDM_POLICY_NAME": p.Name, "VIBEMDM_POLICY_ID": p.PolicyID})
			addStep("script", s.Name, r)
		}
	}
	// Start package downloads immediately so they overlap with "before" scripts.
	var prefetch map[string]error
	prefetchDone := make(chan struct{})
	if len(p.Packages) > 0 {
		go func() { prefetch = e.PrefetchPackages(ctx, p.Packages); close(prefetchDone) }()
	} else {
		close(prefetchDone)
	}
	runScripts("before")
	<-prefetchDone
	for _, pk := range p.Packages {
		if err := prefetch[pk.PackageID]; err != nil {
			logf("Package %q download failed: %v", pk.Name, err)
			addStep("package", pk.Name, ExecResult{ExitCode: -1, Err: err, Combined: err.Error()})
			continue
		}
		logf("Installing package %q", pk.Name)
		r := e.InstallPackage(ctx, pk)
		addStep("package", pk.Name, r)
	}
	for _, app := range p.Apps {
		logf("Installing %q with Installomator", app.Name)
		r := e.RunAppStep(ctx, app)
		addStep("app", app.Name, r)
	}
	if strings.TrimSpace(p.RunCommand) != "" {
		logf("Running command")
		r := e.RunCommand(ctx, p.RunCommand)
		addStep("command", "Execute Command", r)
	}
	runScripts("after")
	if p.UpdateInventory {
		logf("Updating inventory")
		start := time.Now()
		err := e.Recon(ctx)
		r := ExecResult{Duration: time.Since(start), Combined: "inventory submitted"}
		if err != nil {
			r.ExitCode, r.Err, r.Combined = 1, err, err.Error()
		}
		addStep("inventory", "Update Inventory", r)
	}
	sub.CompletedAt = time.Now().UTC()
	sub.Status = "completed"
	if failed {
		sub.Status = "failed"
	}
	logf("Policy %s", sub.Status)
	if p.UserInteraction.CompleteMessage != "" {
		Notify(ctx, p.Name, p.UserInteraction.CompleteMessage)
	}
	sub.Log = truncate(logb.String())
	done := log.Info
	if failed {
		done = log.Warn
	}
	done("policy finished", "status", sub.Status, "steps", len(sub.Steps), "dur", sub.CompletedAt.Sub(sub.StartedAt))
	if p.LogID != "" {
		if err := e.client.SubmitPolicyLog(ctx, p.LogID, sub); err != nil {
			log.Error("submitting policy log; spooling for retry", "log_id", p.LogID, "err", err)
			spoolWrite(spoolItem{Kind: "policy", ID: p.LogID, Policy: &sub, CreatedAt: time.Now().UTC()})
		}
	}
	if p.Reboot.Enabled {
		delay := p.Reboot.DelayMinutes
		msg := p.Reboot.Message
		if msg == "" {
			msg = "Your computer will restart to complete management tasks."
		}
		Notify(ctx, p.Name, msg)
		arg := "now"
		if delay > 0 {
			arg = fmt.Sprintf("+%d", delay)
		}
		log.Warn("policy scheduled a restart", "delay", arg)
		exec.Command("/sbin/shutdown", "-r", arg).Start()
	}
	return sub
}

// Recon collects and submits full inventory.
func (e *Executor) Recon(ctx context.Context) error {
	log := e.log.With("component", "inventory")
	log.Info("inventory started")
	rc, err := e.client.ReconConfig(ctx)
	if err != nil {
		log.Warn("fetching recon config; extension attributes skipped", "err", err)
		rc = &model.AgentReconConfig{}
	}
	inv := CollectInventory(ctx, e.version, rc.ExtensionAttributes, func(ctx context.Context, ea model.ExtensionAttribute) string {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		r := e.RunScript(ctx, "EA: "+ea.Name, ea.Script, nil, nil)
		if r.ExitCode != 0 && r.Stdout == "" {
			return "error: " + strings.TrimSpace(r.Stderr)
		}
		return ParseEAResult(r.Stdout)
	})
	start := time.Now()
	err = e.client.SubmitInventory(ctx, inv)
	if err != nil {
		log.Warn("inventory upload failed", "collect_ms", inv.CollectDurationMS, "upload", time.Since(start), "err", err)
		return err
	}
	log.Info("inventory submitted", "apps", len(inv.Apps), "extension_attributes", len(rc.ExtensionAttributes),
		"collect_ms", inv.CollectDurationMS, "upload", time.Since(start))
	return nil
}

// RunTask executes an ad-hoc task and reports the result.
func (e *Executor) RunTask(ctx context.Context, t model.AgentTask) {
	// The payload is never logged: it can carry secrets (install_super's auth_password).
	log := e.log.With("component", "task", "task_id", t.ID, "task_type", t.Type)
	log.Info("task started")
	res := model.AgentTaskResult{StartedAt: time.Now().UTC(), Status: "completed"}
	apply := func(r ExecResult) {
		res.ExitCode, res.Stdout, res.Stderr = r.ExitCode, r.Stdout, r.Stderr
		if r.Err != nil {
			res.Error = r.Err.Error()
		}
		if r.ExitCode != 0 {
			res.Status = "failed"
		}
		if res.Stdout == "" && r.Combined != "" {
			res.Stdout = r.Combined
		}
	}
	switch t.Type {
	case "run_script":
		contents, _ := t.Payload["contents"].(string)
		name, _ := t.Payload["name"].(string)
		var params []string
		if raw, ok := t.Payload["parameters"].([]any); ok {
			for _, p := range raw {
				params = append(params, str(p))
			}
		}
		apply(e.RunScript(ctx, name, contents, params, nil))
	case "run_command":
		apply(e.RunCommand(ctx, str(t.Payload["command"])))
	case "run_policy":
		var ap model.AgentPolicy
		if b, err := jsonMarshal(t.Payload["policy"]); err == nil {
			jsonUnmarshal(b, &ap)
		}
		if ap.PolicyID == "" {
			res.Status, res.Error = "failed", "task payload missing policy"
			break
		}
		sub := e.RunPolicy(ctx, ap)
		res.Stdout = sub.Log
		if sub.Status != "completed" {
			res.Status = "failed"
		}
	case "inventory":
		if err := e.Recon(ctx); err != nil {
			res.Status, res.Error = "failed", err.Error()
		} else {
			res.Stdout = "inventory submitted"
		}
	case "custom_trigger":
		trig := str(t.Payload["trigger"])
		n, err := e.CheckinAndRun(ctx, trig)
		if err != nil {
			res.Status, res.Error = "failed", err.Error()
		}
		res.Stdout = fmt.Sprintf("trigger %q: executed %d policies", trig, n)
	case "filevault_regenerate":
		apply(e.RegenerateFileVaultKey(ctx))
	case "compliance_scan":
		var scan model.AgentComplianceScan
		if b, err := jsonMarshal(t.Payload["scan"]); err == nil {
			jsonUnmarshal(b, &scan)
		}
		if scan.ScanID == "" {
			res.Status, res.Error = "failed", "task payload missing scan"
			break
		}
		apply(e.RunComplianceScan(ctx, scan))
	case "install_super":
		apply(e.InstallSuper(ctx, str(t.Payload["url"]), str(t.Payload["sha256"]), str(t.Payload["auth_mode"]),
			str(t.Payload["auth_account"]), str(t.Payload["auth_password"])))
		if res.Stdout != "" {
			res.Stdout = redactSecret(res.Stdout, str(t.Payload["auth_password"]))
		}
		res.Stderr = redactSecret(res.Stderr, str(t.Payload["auth_password"]))
		if err := e.Recon(ctx); err != nil {
			log.Warn("inventory after installing super", "err", err)
		}
	case "onboarding":
		var ob model.AgentOnboarding
		if b, err := jsonMarshal(t.Payload["onboarding"]); err == nil {
			jsonUnmarshal(b, &ob)
		}
		if ob.RunID == "" {
			res.Status, res.Error = "failed", "task payload missing onboarding"
			break
		}
		apply(e.RunOnboarding(ctx, ob))
	case "collect_diagnostics":
		apply(e.CollectDiagnostics(ctx, str(t.Payload["bundle_id"])))
	case "notify":
		Notify(ctx, e.cfg.OrgName, str(t.Payload["message"]))
		res.Stdout = "notification shown"
	case "restart":
		Notify(ctx, e.cfg.OrgName, firstNonEmpty(str(t.Payload["message"]), "Your computer will restart in one minute."))
		exec.Command("/sbin/shutdown", "-r", "+1").Start()
		res.Stdout = "restart scheduled in 1 minute"
	case "shutdown":
		Notify(ctx, e.cfg.OrgName, firstNonEmpty(str(t.Payload["message"]), "Your computer will shut down in one minute."))
		exec.Command("/sbin/shutdown", "-h", "+1").Start()
		res.Stdout = "shutdown scheduled in 1 minute"
	case "logout_user":
		user := ConsoleUser(ctx)
		if user == "" {
			res.Stdout = "no console user"
		} else {
			uid := ConsoleUserUID(ctx, user)
			apply(runCmd(exec.CommandContext(ctx, "launchctl", "bootout", fmt.Sprintf("gui/%d", uid))))
		}
	default:
		res.Status, res.Error = "failed", "unknown task type "+t.Type
	}
	res.CompletedAt = time.Now().UTC()
	attrs := []any{"status", res.Status, "exit", res.ExitCode, "dur", res.CompletedAt.Sub(res.StartedAt)}
	if res.Error != "" {
		attrs = append(attrs, "err", res.Error)
	}
	if res.Status == "completed" {
		log.Info("task finished", append(attrs, "output", Summarize(res.Stdout, 1, 200))...)
	} else {
		out := res.Stdout // falls back to the combined output, which already holds stderr
		if e := strings.TrimSpace(res.Stderr); e != "" && !strings.Contains(out, e) {
			out += "\n" + res.Stderr
		}
		log.Warn("task finished", append(attrs, "output", Summarize(out, 3, 500))...)
	}
	if err := e.client.SubmitTaskResult(ctx, t.ID, res); err != nil {
		log.Error("submitting task result; spooling for retry", "err", err)
		spoolWrite(spoolItem{Kind: "task", ID: t.ID, Task: &res, CreatedAt: time.Now().UTC()})
	}
}

// CheckinAndRun performs a check-in for a trigger and executes everything returned.
func (e *Executor) CheckinAndRun(ctx context.Context, trigger string) (int, error) {
	log := e.log.With("component", "checkin", "trigger", trigger)
	start := time.Now()
	si := CollectSystemInfo(ctx)
	resp, err := e.client.Checkin(ctx, model.AgentCheckinRequest{
		Trigger: trigger, ConsoleUser: si.ConsoleUser, Hostname: si.Hostname, ComputerName: si.ComputerName, OSVersion: si.OSVersion,
		OSBuild: si.OSBuild, IPAddress: si.IP, AgentVersion: e.version, UptimeSecs: int64(si.Uptime.Seconds()),
	})
	if err != nil {
		log.Warn("check-in failed", "dur", time.Since(start), "err", err)
		return 0, err
	}
	if resp.CheckinIntervalSeconds >= 30 && resp.CheckinIntervalSeconds != e.cfg.CheckinIntervalSeconds {
		log.Info("check-in interval changed by the server", "from_secs", e.cfg.CheckinIntervalSeconds, "to_secs", resp.CheckinIntervalSeconds)
		e.cfg.CheckinIntervalSeconds = resp.CheckinIntervalSeconds
		e.cfg.Save("")
	}
	work := len(resp.Policies) + len(resp.Tasks) + len(resp.AppInstalls)
	if work > 0 || resp.InventoryRequested || trigger != "checkin" {
		log.Info("check-in accepted", "policies", len(resp.Policies), "tasks", len(resp.Tasks), "app_installs", len(resp.AppInstalls),
			"inventory_requested", resp.InventoryRequested, "console_user", si.ConsoleUser)
	}
	for _, p := range resp.Policies {
		e.RunPolicy(ctx, p)
	}
	for _, t := range resp.Tasks {
		e.RunTask(ctx, t)
	}
	for _, app := range resp.AppInstalls {
		e.InstallApp(ctx, app)
	}
	if e.OnRestrictedRules != nil {
		e.OnRestrictedRules(resp.RestrictedSoftware)
	}
	if resp.InventoryRequested {
		e.Recon(ctx) // Recon logs its own failures
	}
	// A routine check-in with nothing to do is debug-level so it does not flood the log.
	done := log.Debug
	if work > 0 || resp.InventoryRequested || trigger != "checkin" {
		done = log.Info
	}
	done("check-in complete", "policies", len(resp.Policies), "dur", time.Since(start))
	return len(resp.Policies), nil
}

// logStep records one policy step: a one-line summary at info (warn with more
// output when it failed), and the full output at debug.
func logStep(log *slog.Logger, typ, name, status string, r ExecResult, out string) {
	attrs := []any{"step", typ, "name", name, "status", status, "exit", r.ExitCode, "dur", r.Duration}
	if status == "failed" {
		log.Warn("policy step failed", append(attrs, "output", Summarize(out, 3, 500))...)
	} else {
		log.Info("policy step finished", append(attrs, "output", Summarize(out, 1, 200))...)
	}
	if log.Enabled(context.Background(), slog.LevelDebug) && strings.TrimSpace(out) != "" {
		if len(out) > 4096 {
			out = out[len(out)-4096:]
		}
		log.Debug("policy step output", "step", typ, "name", name, "output", out)
	}
}

// redactSecret removes a secret value from command output.
func redactSecret(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "[redacted]")
}
