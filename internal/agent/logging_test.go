package agent

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestLogger(t *testing.T, opts LogOptions) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	opts.Stdout = &buf
	return slog.New(NewLocalHandler(opts)), &buf
}

func TestLocalHandlerFormat(t *testing.T) {
	log, buf := newTestLogger(t, LogOptions{})
	log.With("component", "policy", "policy", "Install Zoom").Info("policy finished",
		"status", "completed", "dur", 1234*time.Millisecond, "err", errors.New("boom"), "output", "line one\nline \"two\"")
	line := strings.TrimSuffix(buf.String(), "\n")
	if strings.Count(buf.String(), "\n") != 1 {
		t.Fatalf("record spans several lines: %q", buf.String())
	}
	for _, want := range []string{" INFO  [policy] policy finished", `policy="Install Zoom"`, "status=completed", "dur=1.234s",
		"err=boom", `output="line one\nline \"two\""`} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q lacks %q", line, want)
		}
	}
	lvl, comp, ok := ParseLogLine(line)
	if !ok || lvl != slog.LevelInfo || comp != "policy" {
		t.Errorf("ParseLogLine = %v %q %v", lvl, comp, ok)
	}
}

func TestLocalHandlerDefaultComponentAndGroups(t *testing.T) {
	log, buf := newTestLogger(t, LogOptions{})
	log.WithGroup("req").Warn("hello", "id", 7, slog.Group("sub", "k", "v"))
	if got := buf.String(); !strings.Contains(got, "WARN  [agent] hello req.id=7 req.sub.k=v") {
		t.Errorf("got %q", got)
	}
}

func TestSecretsAreRedacted(t *testing.T) {
	log, buf := newTestLogger(t, LogOptions{})
	log.With("device_token", "tok-123").Info("x", "auth_password", "hunter2", "enrollment_token", "e-1", "scep_challenge", "c-1", "user", "kellen")
	got := buf.String()
	for _, secret := range []string{"tok-123", "hunter2", "e-1", "c-1"} {
		if strings.Contains(got, secret) {
			t.Errorf("secret %q leaked: %q", secret, got)
		}
	}
	if !strings.Contains(got, "user=kellen") || !strings.Contains(got, "auth_password=[redacted]") {
		t.Errorf("got %q", got)
	}

	// The server copy is redacted too.
	ship := &LogShipper{minLvl: slog.LevelInfo}
	slog.New(ship.Wrap(NewLocalHandler(LogOptions{}))).Info("y", "device_token", "tok-456")
	if v := ship.buf[0].Attrs["device_token"]; v != "[redacted]" {
		t.Errorf("shipped device_token = %v", v)
	}
}

func TestLevelVarIsLive(t *testing.T) {
	lv := new(slog.LevelVar)
	lv.Set(slog.LevelInfo)
	log, buf := newTestLogger(t, LogOptions{Level: lv})
	log.Debug("hidden")
	lv.Set(slog.LevelDebug)
	log.Debug("shown")
	if got := buf.String(); strings.Contains(got, "hidden") || !strings.Contains(got, "DEBUG [agent] shown") {
		t.Errorf("got %q", got)
	}
}

func TestLogLevelSettings(t *testing.T) {
	p := filepath.Join(t.TempDir(), "logging.json")
	if l := LoadLogLevel(p); l != slog.LevelInfo {
		t.Errorf("default = %v", l)
	}
	if err := SaveLogLevel(p, slog.LevelDebug); err != nil {
		t.Fatal(err)
	}
	if l := LoadLogLevel(p); l != slog.LevelDebug {
		t.Errorf("saved = %v", l)
	}
	if _, err := ParseLogLevel("loud"); err == nil {
		t.Error("ParseLogLevel accepted a bad level")
	}
	for _, s := range []string{"debug", "info", "warn", "error"} {
		l, _ := ParseLogLevel(s)
		if LevelName(l) != s {
			t.Errorf("round trip %s -> %s", s, LevelName(l))
		}
	}
}

func TestRotatingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "agent.log")
	r := &rotatingFile{path: path, max: 100, keep: 2, rotate: true}
	for i := 0; i < 10; i++ {
		if err := r.Write([]byte(fmt.Sprintf("%02d %s\n", i, strings.Repeat("x", 40)))); err != nil {
			t.Fatal(err)
		}
	}
	r.f.Close()
	for _, p := range []string{path, path + ".1", path + ".2"} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if st.Size() > 100 {
			t.Errorf("%s is %d bytes, over the limit", p, st.Size())
		}
	}
	if _, err := os.Stat(path + ".3"); err == nil {
		t.Error("kept more than 2 rotated files")
	}
	cur, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(cur), "08 ") {
		t.Errorf("current file starts with %q", string(cur)[:3])
	}
}

func TestTailAndFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	h := NewLocalHandler(LogOptions{File: path, Level: func() *slog.LevelVar { v := new(slog.LevelVar); v.Set(slog.LevelDebug); return v }()})
	log := slog.New(h)
	log.With("component", "policy").Info("policy started")
	log.With("component", "task").Warn("task finished", "status", "failed")
	log.With("component", "checkin").Debug("check-in complete")
	os.WriteFile(path+".1", []byte("2026-10-10T10:00:00.000Z INFO  [daemon] vibemdm agent starting\n"), 0o600)
	appendLine := func(s string) {
		f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		f.WriteString(s + "\n")
		f.Close()
	}
	appendLine("panic: something broke") // a line in another format

	all, err := TailLogs(path, 10, LogFilter{MinLevel: slog.LevelDebug})
	if err != nil || len(all) != 5 || !strings.Contains(all[0], "agent starting") {
		t.Fatalf("all = %q, %v", all, err)
	}
	last2, _ := TailLogs(path, 2, LogFilter{MinLevel: slog.LevelDebug})
	if len(last2) != 2 || last2[1] != "panic: something broke" {
		t.Errorf("last2 = %q", last2)
	}
	warn, _ := TailLogs(path, 10, LogFilter{MinLevel: slog.LevelWarn}) // the panic line has no level, so it stays
	if len(warn) != 2 || !strings.Contains(warn[0], "[task]") || !strings.HasPrefix(warn[1], "panic:") {
		t.Errorf("warn = %q", warn)
	}
	pol, _ := TailLogs(path, 10, LogFilter{MinLevel: slog.LevelDebug, Component: "POLICY"})
	if len(pol) != 1 {
		t.Errorf("policy = %q", pol)
	}
	g, _ := TailLogs(path, 10, LogFilter{MinLevel: slog.LevelDebug, Contains: "FAILED"})
	if len(g) != 1 {
		t.Errorf("grep = %q", g)
	}
	if _, err := TailLogs(filepath.Join(dir, "missing.log"), 10, LogFilter{}); err != nil {
		t.Errorf("missing file: %v", err)
	}
}

func TestFollowLogsHandlesRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	os.WriteFile(path, []byte("old line\n"), 0o600)
	ctx, cancel := context.WithCancel(context.Background())
	var out syncBuffer
	done := make(chan struct{})
	go func() { FollowLogs(ctx, path, &out, LogFilter{MinLevel: slog.LevelDebug}); close(done) }()
	time.Sleep(200 * time.Millisecond)
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("new line 1\n")
	f.Close()
	time.Sleep(700 * time.Millisecond)
	os.Rename(path, path+".1")
	os.WriteFile(path, []byte("after rotation\n"), 0o600)
	time.Sleep(1200 * time.Millisecond)
	cancel()
	<-done
	got := out.String()
	if strings.Contains(got, "old line") || !strings.Contains(got, "new line 1") || !strings.Contains(got, "after rotation") {
		t.Errorf("followed output = %q", got)
	}
}

func TestSummarize(t *testing.T) {
	if got := Summarize("a\n\nb\nc\n\n", 2, 100); got != "b | c" {
		t.Errorf("got %q", got)
	}
	if got := Summarize(strings.Repeat("x", 50), 1, 10); got != "…"+strings.Repeat("x", 10) {
		t.Errorf("got %q", got)
	}
}

// syncBuffer is a bytes.Buffer safe for one writer goroutine and a reader.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func TestDaemonSetLogLevel(t *testing.T) {
	lv := new(slog.LevelVar)
	d := &Daemon{log: slog.New(NewLocalHandler(LogOptions{Level: lv})), LogLevel: lv, checkinNow: make(chan string, 1)}
	send := func(restricted bool, msg string) string {
		c1, c2 := net.Pipe()
		go d.handleConn(c2, restricted)
		c1.Write([]byte(msg + "\n"))
		b, _ := bufio.NewReader(c1).ReadString('\n')
		c1.Close()
		return b
	}
	if got := send(true, `{"log_level":"debug"}`); !strings.Contains(got, "requires root") || lv.Level() != slog.LevelInfo {
		t.Errorf("unprivileged socket: %q, level %v", got, lv.Level())
	}
	if got := send(false, `{"log_level":"loud"}`); !strings.Contains(got, "unknown log level") {
		t.Errorf("bad level: %q", got)
	}
	if got := send(false, `{"log_level":"debug"}`); !strings.Contains(got, `"ok":true`) || lv.Level() != slog.LevelDebug {
		t.Errorf("root socket: %q, level %v", got, lv.Level())
	}
	if got := send(false, `{"trigger":"checkin"}`); !strings.Contains(got, `"ok":true`) || <-d.checkinNow != "checkin" {
		t.Errorf("trigger still works: %q", got)
	}
}
