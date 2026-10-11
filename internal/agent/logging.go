package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vibemdm/vibemdm/internal/logfmt"
)

// On-device logging.
//
// Every record goes to three places:
//   - a rotating text file under /Library/Logs/VibeMDM (Console.app lists this
//     folder under "Log Reports"; `vibemdm-agent logs` reads it),
//   - macOS unified logging, subsystem com.vibemdm.agent, one category per
//     component (only when the agent is built with cgo on a Mac; see oslog_darwin.go),
//   - the server, through LogShipper (info and above).
//
// Records carry a "component" attribute (policy, task, checkin, …) that becomes
// the unified-logging category and the [component] column in the file.
const (
	LogDir          = "/Library/Logs/VibeMDM"
	LogPath         = LogDir + "/vibemdm-agent.log"
	LaunchdLogPath  = LogDir + "/vibemdm-agent.launchd.log" // the daemon's stdout/stderr (crashes, panics)
	LogSettingsPath = SupportDir + "/logging.json"
	LogSubsystem    = "com.vibemdm.agent"

	logFileMax  = 10 << 20 // rotate at 10 MB
	logFileKeep = 5        // vibemdm-agent.log.1 … .5
	adminGID    = 80       // the macOS "admin" group may read the logs without sudo
)

// ParseLogLevel accepts debug, info, warn/warning and error (case-insensitive).
func ParseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return slog.LevelInfo, fmt.Errorf("unknown log level %q (use debug, info, warn or error)", s)
}

// LevelName is the lower-case name ParseLogLevel accepts.
func LevelName(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "debug"
	case l < slog.LevelWarn:
		return "info"
	case l < slog.LevelError:
		return "warn"
	}
	return "error"
}

type logSettings struct {
	Level string `json:"level"`
}

// LoadLogLevel returns the saved on-device log level (info when unset or unreadable).
func LoadLogLevel(path string) slog.Level {
	if path == "" {
		path = LogSettingsPath
	}
	var s logSettings
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &s) == nil {
		if l, err := ParseLogLevel(s.Level); err == nil {
			return l
		}
	}
	return slog.LevelInfo
}

// SaveLogLevel persists the on-device log level.
func SaveLogLevel(path string, l slog.Level) error {
	if path == "" {
		path = LogSettingsPath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.Marshal(logSettings{Level: LevelName(l)})
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func isSecretKey(k string) bool { return logfmt.IsSecretKey(k) }

// Summarize shortens command output for a log line: the last non-empty lines,
// joined with " | ", at most max bytes.
func Summarize(s string, lines, max int) string {
	var keep []string
	all := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := len(all) - 1; i >= 0 && len(keep) < lines; i-- {
		if t := strings.TrimSpace(all[i]); t != "" {
			keep = append([]string{t}, keep...)
		}
	}
	out := strings.Join(keep, " | ")
	if len(out) > max {
		out = "…" + out[len(out)-max:]
	}
	return out
}

// LogOptions configures NewLocalHandler.
type LogOptions struct {
	Level   *slog.LevelVar // minimum level for the file, stdout and unified log
	File    string         // log file; empty disables the file
	Rotate  bool           // only the daemon rotates; CLI runs just append
	Stdout  io.Writer      // optional console copy (CLI commands)
	Unified bool           // also write to macOS unified logging when available
}

// logSinks is shared by a handler and every handler derived from it.
type logSinks struct {
	level   *slog.LevelVar
	mu      sync.Mutex
	file    *rotatingFile
	stdout  io.Writer
	unified bool
}

// localHandler formats records as one line each:
//
//	2026-10-11T14:03:22.123-07:00 INFO  [policy] policy finished policy="Install Zoom" status=completed dur=12.4s
type localHandler struct {
	s         *logSinks
	component string
	attrs     []logfmt.KV // from WithAttrs
	group     string
}

// NewLocalHandler returns the on-device slog handler, which writes readable lines (see
// internal/logfmt) to the log file and stdout, and the message with details to unified logging.
func NewLocalHandler(o LogOptions) slog.Handler {
	if o.Level == nil {
		o.Level = new(slog.LevelVar)
	}
	s := &logSinks{level: o.Level, stdout: o.Stdout, unified: o.Unified && unifiedAvailable}
	if o.File != "" {
		s.file = &rotatingFile{path: o.File, max: logFileMax, keep: logFileKeep, rotate: o.Rotate}
	}
	return &localHandler{s: s, component: "agent"}
}

func (h *localHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.s.level.Level() }

func (h *localHandler) Handle(_ context.Context, r slog.Record) error {
	comp := h.component
	kvs := append([]logfmt.KV(nil), h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "component" {
			comp = a.Value.String()
			return true
		}
		kvs = logfmt.Flatten(kvs, h.group, a)
		return true
	})
	line := logfmt.Line(r.Time, r.Level, comp, r.Message, kvs)

	h.s.mu.Lock()
	defer h.s.mu.Unlock()
	if h.s.file != nil {
		h.s.file.Write([]byte(line)) // a full disk must never stop the agent
	}
	if h.s.stdout != nil {
		io.WriteString(h.s.stdout, line)
	}
	if h.s.unified {
		unifiedLog(comp, r.Level, logfmt.Body(logfmt.Explain(r.Level, comp, r.Message, kvs), kvs))
	}
	return nil
}

func (h *localHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	n := *h
	n.attrs = append([]logfmt.KV(nil), h.attrs...)
	for _, a := range attrs {
		if a.Key == "component" {
			n.component = a.Value.String()
			continue
		}
		n.attrs = logfmt.Flatten(n.attrs, h.group, a)
	}
	return &n
}

func (h *localHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	n := *h
	n.group = h.group + name + "."
	return &n
}

// ParseLogLine extracts the level and component of a line written by localHandler (current or
// older format). ok is false for lines in any other format (panics, other tools).
func ParseLogLine(line string) (lvl slog.Level, component string, ok bool) {
	return logfmt.Parse(line)
}

// LogFilter selects lines for `vibemdm-agent logs`.
type LogFilter struct {
	MinLevel  slog.Level // note: the zero value is slog.LevelInfo
	Component string     // exact match, case-insensitive; empty = all
	Contains  string     // substring, case-insensitive; empty = all
}

// Match reports whether a log line passes the filter. Lines in another format
// (a panic trace, a crash message) have no level, so a level filter never hides
// them; a component filter does.
func (f LogFilter) Match(line string) bool {
	lvl, comp, ok := ParseLogLine(line)
	if ok && lvl < f.MinLevel {
		return false
	}
	if f.Component != "" && (!ok || !strings.EqualFold(comp, f.Component)) {
		return false
	}
	return f.Contains == "" || strings.Contains(strings.ToLower(line), strings.ToLower(f.Contains))
}

// TailLogs returns the last n lines of path (continuing into path.1 when the
// current file is short) that pass the filter, oldest first.
func TailLogs(path string, n int, f LogFilter) ([]string, error) {
	var out []string
	for i, p := range []string{path, path + ".1"} {
		lines, err := matchingLines(p, f)
		if err != nil {
			if i == 0 && !os.IsNotExist(err) {
				return nil, err
			}
			if i == 0 {
				continue
			}
			break
		}
		out = append(lines, out...)
		if len(out) >= n {
			break
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out, nil
}

func matchingLines(path string, f LogFilter) ([]string, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	var out []string
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		if line := sc.Text(); f.Match(line) {
			out = append(out, line)
		}
	}
	return out, sc.Err()
}

// FollowLogs prints new matching lines of path to w until ctx ends, reopening
// the file after the daemon rotates it.
func FollowLogs(ctx context.Context, path string, w io.Writer, f LogFilter) error {
	var fh *os.File
	var rd *bufio.Reader
	var pos int64
	defer func() {
		if fh != nil {
			fh.Close()
		}
	}()
	open := func(seekEnd bool) {
		if fh != nil {
			fh.Close()
			fh = nil
		}
		nf, err := os.Open(path)
		if err != nil {
			return
		}
		pos = 0
		if seekEnd {
			pos, _ = nf.Seek(0, io.SeekEnd)
		}
		fh, rd = nf, bufio.NewReader(nf)
	}
	open(true)
	partial := ""
	for {
		if fh != nil {
			for {
				s, err := rd.ReadString('\n')
				pos += int64(len(s))
				if err != nil {
					partial += s // incomplete line: wait for the rest
					break
				}
				line := strings.TrimRight(partial+s, "\n")
				partial = ""
				if f.Match(line) {
					fmt.Fprintln(w, line)
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
		// Rotated (the path now names a new, shorter file) or not created yet.
		if st, err := os.Stat(path); err == nil && (fh == nil || st.Size() < pos || !sameFile(fh, st)) {
			partial = ""
			open(false)
		}
	}
}

func sameFile(fh *os.File, st os.FileInfo) bool {
	cur, err := fh.Stat()
	return err == nil && os.SameFile(cur, st)
}

// rotatingFile appends to path and, when rotate is set, renames it to path.1
// (shifting older copies up to path.<keep>) once it passes max bytes.
type rotatingFile struct {
	path   string
	max    int64
	keep   int
	rotate bool
	f      *os.File
	size   int64
	failed time.Time // last open failure; retried after a minute
}

func (r *rotatingFile) Write(p []byte) error {
	if r.f == nil {
		if !r.failed.IsZero() && time.Since(r.failed) < time.Minute {
			return nil
		}
		if err := r.open(); err != nil {
			r.failed = time.Now()
			return err
		}
	}
	if r.rotate && r.size > 0 && r.size+int64(len(p)) > r.max {
		r.doRotate()
		if err := r.open(); err != nil {
			r.failed = time.Now()
			return err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return err
}

func (r *rotatingFile) open() error {
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	os.Lchown(dir, 0, adminGID) // best effort: fails when not root, and on Linux test machines
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	f.Chown(0, adminGID)
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f, r.size, r.failed = f, st.Size(), time.Time{}
	return nil
}

func (r *rotatingFile) doRotate() {
	r.f.Close()
	r.f = nil
	os.Remove(fmt.Sprintf("%s.%d", r.path, r.keep))
	for i := r.keep - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", r.path, i), fmt.Sprintf("%s.%d", r.path, i+1))
	}
	os.Rename(r.path, r.path+".1")
}

// UnifiedLoggingAvailable reports whether this build writes to macOS unified logging.
func UnifiedLoggingAvailable() bool { return unifiedAvailable }
