package logfmt

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLine(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(NewHandler(&buf, slog.LevelInfo, "server"))
	log.With("component", "mdm").Info("mdm: token update", "udid", "CD011E6D", "computer", "LAND-TESTMAC01",
		"new_enrollment", true, "dur", 1500*time.Millisecond, "err", errors.New("boom"), "device_token", "secret-1", "output", "a b\nc")
	line := buf.String()
	if strings.Count(line, "\n") != 1 || strings.Contains(line, "secret-1") {
		t.Fatalf("line %q", line)
	}
	for _, want := range []string{" INFO  mdm         Token update · computer=LAND-TESTMAC01 new_enrollment=true dur=1.50s err=boom",
		"device_token=[redacted]", `output="a b\nc"`, " · udid=CD011E6D\n"} {
		if !strings.Contains(line, want) {
			t.Errorf("%q lacks %q", line, want)
		}
	}
	lvl, comp, ok := Parse(line)
	if !ok || lvl != slog.LevelInfo || comp != "mdm" {
		t.Errorf("Parse = %v %q %v", lvl, comp, ok)
	}
	if _, comp, ok := Parse("2026-10-11T13:54:11.043Z WARN  [policy] policy finished"); !ok || comp != "policy" {
		t.Error("the older agent format must still parse")
	}
	if _, _, ok := Parse("panic: boom"); ok {
		t.Error("parsed a non-log line")
	}
}

func TestDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{4 * time.Millisecond: "4ms", 1234 * time.Millisecond: "1.23s", 16521 * time.Millisecond: "16.5s",
		70345 * time.Millisecond: "1m10s", 2*time.Hour + 5*time.Minute + 20*time.Second: "2h5m"} {
		if got := Duration(d); got != want {
			t.Errorf("%v: %q, want %q", d, got, want)
		}
	}
}

func TestMessage(t *testing.T) {
	for in, want := range map[string]string{"policy finished": "Policy finished", "mdm: command result": "MDM: command result",
		"LAPS rotated": "LAPS rotated", "laps cycle": "LAPS cycle", "": "", "éclair": "Éclair"} {
		if got := Message(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestExplain(t *testing.T) {
	err := []KV{{"err", "boom"}}
	for _, c := range []struct {
		lvl       slog.Level
		comp, msg string
		kvs       []KV
		want      string
	}{
		{slog.LevelInfo, "mdm", "mdm: token update", nil, "token update"},
		{slog.LevelInfo, "server", "mdm: token update", nil, "mdm: token update"},
		{slog.LevelError, "mdm", "replace apps", err, "replace apps failed"},
		{slog.LevelWarn, "server", "profile sync", err, "profile sync failed"},
		{slog.LevelWarn, "server", "startup failed", err, "startup failed"},
		{slog.LevelWarn, "laps", "account is locked out", err, "account is locked out"},
		{slog.LevelWarn, "x", "check-in", nil, "check-in"},
		{slog.LevelInfo, "x", "replace apps", err, "replace apps"},
	} {
		if got := Explain(c.lvl, c.comp, c.msg, c.kvs); got != c.want {
			t.Errorf("%v %s %q: %q, want %q", c.lvl, c.comp, c.msg, got, c.want)
		}
	}
}
