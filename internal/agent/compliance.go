package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

const (
	complianceCheckTimeout = 60 * time.Second
	complianceFixTimeout   = 5 * time.Minute
	complianceScanTimeout  = 45 * time.Minute
)

// complianceUser is the logged-in user that mSCP's own compliance script puts in CURRENT_USER
// (and CURR_USER_UID) before running any rule. Many per-user checks and fixes use it, e.g.
// `sudo -u "$CURRENT_USER" defaults -currentHost read …`.
type complianceUser struct {
	Name string
	UID  int
}

// needsUser reports whether a rule script relies on the logged-in user.
func needsUser(script string) bool {
	return strings.Contains(script, "CURRENT_USER") || strings.Contains(script, "CURR_USER_UID")
}

// complianceShell runs an mSCP check or fix the way mSCP's own script does: zsh, as root, with
// CURRENT_USER and CURR_USER_UID set like the script's preamble.
func complianceShell(ctx context.Context, script string, timeout time.Duration, user complianceUser) ExecResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/zsh", "-c", script)
	cmd.Dir = "/"
	cmd.Env = append(os.Environ(), "PATH=/usr/bin:/bin:/usr/sbin:/sbin:/usr/libexec", "CURRENT_USER="+user.Name)
	if user.Name != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("CURR_USER_UID=%d", user.UID))
	}
	r := runCmd(cmd)
	if ctx.Err() == context.DeadlineExceeded {
		r.Err = fmt.Errorf("timed out after %s", timeout)
	}
	return r
}

// checkOutput is what mSCP compares: stdout with trailing newlines removed (as $(...) does).
func checkOutput(r ExecResult) string { return strings.TrimRight(r.Stdout, "\n") }

// evaluateRule runs one check (and, if allowed, its fix followed by a re-check).
func evaluateRule(ctx context.Context, r model.AgentComplianceRule, remediate bool, user complianceUser) model.AgentComplianceRuleResult {
	res := model.AgentComplianceRuleResult{ID: r.ID}
	if user.Name == "" && needsUser(r.Check) {
		// Without a logged-in user `sudo -u ""` prints its usage and the check means nothing.
		res.Status, res.Detail = "error", "this check reads the logged-in user's settings; no one is logged in, so it was skipped"
		return res
	}
	out := complianceShell(ctx, r.Check, complianceCheckTimeout, user)
	if out.Err != nil {
		res.Status, res.Detail = "error", out.Err.Error()
		return res
	}
	res.Actual = checkOutput(out)
	if res.Actual == r.Expected {
		res.Status = "pass"
		return res
	}
	res.Status = "fail"
	if strings.TrimSpace(out.Stderr) != "" {
		res.Detail = "stderr: " + firstLines(out.Stderr, 3)
	}
	if !remediate || r.Fix == "" {
		return res
	}
	if user.Name == "" && needsUser(r.Fix) {
		res.Detail = "not fixed: the fix changes the logged-in user's settings and no one is logged in"
		return res
	}
	fix := complianceShell(ctx, r.Fix, complianceFixTimeout, user)
	if fix.Err != nil || fix.ExitCode != 0 {
		res.Detail = strings.TrimSpace(fmt.Sprintf("fix failed (exit %d): %s", fix.ExitCode, firstLines(fix.Combined, 3)))
		return res
	}
	again := complianceShell(ctx, r.Check, complianceCheckTimeout, user)
	if again.Err == nil && checkOutput(again) == r.Expected {
		res.Status, res.Actual, res.Detail = "remediated", checkOutput(again), "fixed by the mSCP fix script"
		return res
	}
	res.Actual = checkOutput(again)
	res.Detail = "fix ran but the check still fails (some settings need a configuration profile or a restart)"
	if res.Actual == "" {
		// e.g. pmset reports no "womp" (Wake for network access) on hardware or virtual machines without it.
		res.Detail = "fix ran but the check reports no value: this Mac may not have the setting (common on virtual machines)"
	}
	return res
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, " | ")
}

// RunComplianceScan evaluates an mSCP baseline and submits the results.
func (e *Executor) RunComplianceScan(ctx context.Context, scan model.AgentComplianceScan) ExecResult {
	ctx, cancel := context.WithTimeout(ctx, complianceScanTimeout)
	defer cancel()
	log := e.log.With("component", "compliance", "scan_id", scan.ScanID, "baseline", scan.Baseline)
	log.Info("compliance scan started", "rules", len(scan.Rules), "remediate", scan.Remediate)
	start := time.Now()
	sub := model.AgentComplianceSubmit{OSVersion: CollectSystemInfo(ctx).OSVersion}
	counts := map[string]int{}
	user := complianceUser{Name: ConsoleUser(ctx)}
	if user.Name != "" {
		user.UID = ConsoleUserUID(ctx, user.Name)
	}
	for _, r := range scan.Rules {
		if ctx.Err() != nil {
			sub.Error = "scan timed out; partial results submitted"
			break
		}
		rr := evaluateRule(ctx, r, scan.Remediate, user)
		counts[rr.Status]++
		sub.Results = append(sub.Results, rr)
	}
	sub.DurationMS = time.Since(start).Milliseconds()
	summary := fmt.Sprintf("%s: %d passed, %d failed, %d remediated, %d errors in %s", scan.Baseline,
		counts["pass"], counts["fail"], counts["remediated"], counts["error"], time.Since(start).Round(time.Second))
	log.Info("compliance scan finished", "pass", counts["pass"], "fail", counts["fail"],
		"remediated", counts["remediated"], "error", counts["error"], "dur", time.Since(start))
	// The results go to their own endpoint; the server needs them even if this task's result is lost.
	sctx, scancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer scancel()
	if err := e.client.SubmitComplianceScan(sctx, scan.ScanID, sub); err != nil {
		log.Error("submitting compliance results", "err", err)
		return ExecResult{ExitCode: 1, Stdout: summary, Err: fmt.Errorf("submitting results: %w", err)}
	}
	return ExecResult{Stdout: summary}
}
