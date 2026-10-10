package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// super (S.U.P.E.R.M.A.N., github.com/Macjutsu/super, Apache-2.0) status plist locations.
var superStatusPaths = []string{
	"/Library/Management/super/com.macjutsu.super.plist", // 5.x
	"/Library/Preferences/com.macjutsu.super.plist",      // 6.x
}

// CollectSuperStatus reads super's status plist (nil when super is not installed).
func CollectSuperStatus(ctx context.Context) map[string]any {
	for _, p := range superStatusPaths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		out := run(ctx, 10*time.Second, "/usr/bin/plutil", "-convert", "json", "-o", "-", p)
		var m map[string]any
		if json.Unmarshal([]byte(out), &m) != nil {
			continue
		}
		keep := map[string]any{"path": p}
		for _, k := range []string{"SuperVersion", "SuperStatus", "WorkflowTarget", "WorkflowTargetTimestamp", "WorkflowScheduledInstall",
			"NextAutoLaunch", "DeadlineCounterFocus", "DeadlineCounterSoft", "DeadlineCounterHard", "DeadlineDaysSoftDate", "DeadlineDaysHardDate",
			"LastSuccessfulCheckDate", "MacOSMSULabelDownloaded", "MacOSInstallerDownloaded", "AuthLocalAccount", "AuthServiceAccount"} {
			if v, ok := m[k]; ok {
				keep[k] = v
			}
		}
		return keep
	}
	if _, err := os.Stat("/usr/local/bin/super"); err == nil {
		return map[string]any{"path": "/usr/local/bin/super"}
	}
	return nil
}

// InstallSuper downloads the pinned super script, verifies its SHA-256, and runs
// it as root, which makes super install itself and start its workflow.
func (e *Executor) InstallSuper(ctx context.Context, url, wantSHA, authMode, account, password string) ExecResult {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	dir, err := os.MkdirTemp(TmpDir, "super-")
	if err != nil {
		return ExecResult{ExitCode: -1, Err: err}
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "super")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ExecResult{ExitCode: -1, Err: fmt.Errorf("downloading super: %w", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ExecResult{ExitCode: -1, Err: fmt.Errorf("downloading super: %s", resp.Status)}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return ExecResult{ExitCode: -1, Err: err}
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 16<<20))
	f.Close()
	if err != nil {
		return ExecResult{ExitCode: -1, Err: err}
	}
	if got := hex.EncodeToString(h.Sum(nil)); wantSHA != "" && got != wantSHA {
		return ExecResult{ExitCode: -1, Err: fmt.Errorf("super checksum mismatch (got %s, want %s); refusing to run", got, wantSHA)}
	}
	args := []string{path}
	switch authMode {
	case "local":
		args = append(args, "--auth-local-account="+account, "--auth-local-password="+password)
	case "service":
		args = append(args, "--auth-service-account="+account, "--auth-service-password="+password)
	}
	cmd := exec.CommandContext(ctx, "/bin/bash", args...)
	cmd.Dir = "/"
	r := runCmd(cmd)
	if r.ExitCode == 0 {
		r.Stdout = "super installed; " + r.Stdout
	}
	return r
}
