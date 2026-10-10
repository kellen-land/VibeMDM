package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CollectDiagnostics builds a gzipped tarball of agent and macOS MDM logs and
// uploads it for the given bundle. Sensitive values (the device token) are
// redacted; profile contents are listed by identifier only.
func (e *Executor) CollectDiagnostics(ctx context.Context, bundleID string) ExecResult {
	start := time.Now()
	os.MkdirAll(TmpDir, 0o700)
	path := filepath.Join(TmpDir, "diagnostics-"+bundleID+".tar.gz")
	defer os.Remove(path)
	f, err := os.Create(path)
	if err != nil {
		return e.diagFail(ctx, bundleID, err, start)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	add := func(name string, data []byte) {
		tw.WriteHeader(&tar.Header{Name: "vibemdm-diagnostics/" + name, Mode: 0o600, Size: int64(len(data)), ModTime: time.Now()})
		tw.Write(data)
	}
	cmd := func(timeout time.Duration, maxBytes int, name string, args ...string) []byte {
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		out, err := exec.CommandContext(cctx, name, args...).CombinedOutput()
		if err != nil && len(out) == 0 {
			out = []byte("error: " + err.Error())
		}
		if len(out) > maxBytes {
			out = append(out[len(out)-maxBytes:], []byte("\n…[truncated to last bytes]")...)
		}
		return out
	}
	tail := func(path string, maxBytes int64) []byte {
		fh, err := os.Open(path)
		if err != nil {
			return []byte("unavailable: " + err.Error())
		}
		defer fh.Close()
		if fi, err := fh.Stat(); err == nil && fi.Size() > maxBytes {
			fh.Seek(fi.Size()-maxBytes, io.SeekStart)
		}
		b, _ := io.ReadAll(fh)
		return b
	}

	var summary bytes.Buffer
	fmt.Fprintf(&summary, "VibeMDM diagnostics\ncollected: %s\nagent: %s\nserver: %s\ncomputer_id: %s\n",
		time.Now().UTC().Format(time.RFC3339), e.version, e.cfg.ServerURL, e.cfg.ComputerID)
	add("README.txt", summary.Bytes())
	add("agent.log", tail(LogPath, 5<<20))
	add("installomator.log", tail("/var/log/Installomator.log", 2<<20))
	// mdmclient is the macOS MDM client: its log shows check-ins, commands and errors from the device's side.
	add("mdmclient.ndjson", cmd(3*time.Minute, 20<<20, "/usr/bin/log", "show", "--style", "ndjson", "--last", "2h",
		"--predicate", `process == "mdmclient" OR subsystem == "com.apple.ManagedClient"`))
	add("profiles-enrollment.txt", cmd(30*time.Second, 1<<20, "/usr/bin/profiles", "status", "-type", "enrollment"))
	add("profiles-list.txt", cmd(30*time.Second, 1<<20, "/usr/bin/profiles", "list"))
	add("sw_vers.txt", cmd(10*time.Second, 64<<10, "/usr/bin/sw_vers"))
	add("system_profiler.json", cmd(90*time.Second, 4<<20, "/usr/sbin/system_profiler", "SPHardwareDataType", "SPSoftwareDataType", "-json"))
	add("launchd-agent.txt", cmd(10*time.Second, 256<<10, "/bin/launchctl", "print", "system/"+DaemonLabel))
	add("disk.txt", cmd(10*time.Second, 64<<10, "/bin/df", "-h"))
	cfgCopy := *e.cfg
	cfgCopy.DeviceToken = "[redacted]"
	cfgJSON, _ := json.MarshalIndent(cfgCopy, "", "  ")
	add("agent-config.json", cfgJSON)

	if err := tw.Close(); err != nil {
		return e.diagFail(ctx, bundleID, err, start)
	}
	if err := gz.Close(); err != nil {
		return e.diagFail(ctx, bundleID, err, start)
	}
	if err := f.Close(); err != nil {
		return e.diagFail(ctx, bundleID, err, start)
	}
	fi, _ := os.Stat(path)
	if err := e.client.UploadDiagnostics(ctx, bundleID, path); err != nil {
		return e.diagFail(ctx, bundleID, err, start)
	}
	msg := fmt.Sprintf("diagnostics uploaded (%d bytes)", fi.Size())
	return ExecResult{Stdout: msg, Combined: msg, Duration: time.Since(start)}
}

func (e *Executor) diagFail(ctx context.Context, bundleID string, err error, start time.Time) ExecResult {
	e.client.DiagnosticsFailed(ctx, bundleID, err.Error())
	return ExecResult{ExitCode: 1, Err: err, Combined: "collecting diagnostics failed: " + err.Error(), Duration: time.Since(start)}
}

var _ = strings.TrimSpace
