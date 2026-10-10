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

// complianceShell runs an mSCP check or fix the way mSCP's own script does: zsh, as root.
func complianceShell(ctx context.Context, script string, timeout time.Duration) ExecResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/zsh", "-c", script)
	cmd.Dir = "/"
	cmd.Env = append(os.Environ(), "PATH=/usr/bin:/bin:/usr/sbin:/sbin:/usr/libexec")
	r := runCmd(cmd)
	if ctx.Err() == context.DeadlineExceeded {
		r.Err = fmt.Errorf("timed out after %s", timeout)
	}
	return r
}

// checkOutput is what mSCP compares: stdout with trailing newlines removed (as $(...) does).
func checkOutput(r ExecResult) string { return strings.TrimRight(r.Stdout, "\n") }

// evaluateRule runs one check (and, if allowed, its fix followed by a re-check).
func evaluateRule(ctx context.Context, r model.AgentComplianceRule, remediate bool) model.AgentComplianceRuleResult {
	res := model.AgentComplianceRuleResult{ID: r.ID}
	out := complianceShell(ctx, r.Check, complianceCheckTimeout)
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
	fix := complianceShell(ctx, r.Fix, complianceFixTimeout)
	if fix.Err != nil || fix.ExitCode != 0 {
		res.Detail = strings.TrimSpace(fmt.Sprintf("fix failed (exit %d): %s", fix.ExitCode, firstLines(fix.Combined, 3)))
		return res
	}
	again := complianceShell(ctx, r.Check, complianceCheckTimeout)
	if again.Err == nil && checkOutput(again) == r.Expected {
		res.Status, res.Actual, res.Detail = "remediated", checkOutput(again), "fixed by the mSCP fix script"
		return res
	}
	res.Actual = checkOutput(again)
	res.Detail = "fix ran but the check still fails (some settings need a configuration profile or a restart)"
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
	start := time.Now()
	sub := model.AgentComplianceSubmit{OSVersion: CollectSystemInfo(ctx).OSVersion}
	counts := map[string]int{}
	for _, r := range scan.Rules {
		if ctx.Err() != nil {
			sub.Error = "scan timed out; partial results submitted"
			break
		}
		rr := evaluateRule(ctx, r, scan.Remediate)
		counts[rr.Status]++
		sub.Results = append(sub.Results, rr)
	}
	sub.DurationMS = time.Since(start).Milliseconds()
	summary := fmt.Sprintf("%s: %d passed, %d failed, %d remediated, %d errors in %s", scan.Baseline,
		counts["pass"], counts["fail"], counts["remediated"], counts["error"], time.Since(start).Round(time.Second))
	e.log.Info("compliance scan finished", "baseline", scan.Baseline, "pass", counts["pass"], "fail", counts["fail"],
		"remediated", counts["remediated"], "error", counts["error"])
	// The results go to their own endpoint; the server needs them even if this task's result is lost.
	sctx, scancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer scancel()
	if err := e.client.SubmitComplianceScan(sctx, scan.ScanID, sub); err != nil {
		return ExecResult{ExitCode: 1, Stdout: summary, Err: fmt.Errorf("submitting results: %w", err)}
	}
	return ExecResult{Stdout: summary}
}
