// Package logfmt renders log records as readable single lines for the server's stdout and the
// agent's log file, without dropping any detail:
//
//	2026-10-11 11:21:49.644 -07:00 INFO  policy      Policy step finished · step=app name=visualstudiocode status=completed exit=0 dur=1m10s · policy_id=55d0ec33-…
//
// Date, time and UTC offset; a padded level; the component in a fixed column; the message
// (first letter capitalised, acronyms kept); then "·" and the details in the order they were
// logged, with identifiers moved to a last "·" group so the human-relevant values come first.
// Every record stays on one line: values with spaces, quotes or newlines are quoted.
package logfmt

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// TimeLayout is the timestamp layout (three fields: date, time, offset).
const TimeLayout = "2006-01-02 15:04:05.000 -07:00"

// KV is one flattened attribute.
type KV struct{ Key, Value string }

// secretKeys are attribute-key fragments whose values are never written. A safety net: code
// should not pass secrets to the logger at all.
var secretKeys = []string{"token", "password", "passwd", "secret", "passphrase", "challenge", "recovery_key", "credential"}

// IsSecretKey reports whether an attribute's value must be redacted.
func IsSecretKey(k string) bool {
	k = strings.ToLower(k)
	for _, s := range secretKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// Flatten appends a (possibly grouped) attribute as key=value pairs, redacting secrets.
func Flatten(out []KV, group string, a slog.Attr) []KV {
	v := a.Value.Resolve()
	if a.Key == "" && v.Kind() != slog.KindGroup {
		return out
	}
	if v.Kind() == slog.KindGroup {
		prefix := group
		if a.Key != "" {
			prefix += a.Key + "."
		}
		for _, ga := range v.Group() {
			out = Flatten(out, prefix, ga)
		}
		return out
	}
	val := Value(v)
	if IsSecretKey(a.Key) {
		val = "[redacted]"
	}
	return append(out, KV{Key: group + a.Key, Value: val})
}

// Value renders an attribute value: durations shortened for reading, times in RFC 3339, errors as text.
func Value(v slog.Value) string {
	switch v.Kind() {
	case slog.KindDuration:
		return Duration(v.Duration())
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return err.Error()
		}
	}
	return v.String()
}

// Duration shortens a duration to what a person needs: 4ms, 1.23s, 16.5s, 1m10s, 2h5m.
func Duration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return d.Round(time.Microsecond).String()
	case d < time.Second:
		return d.Round(time.Millisecond).String()
	case d < 10*time.Second:
		return strconv.FormatFloat(d.Seconds(), 'f', 2, 64) + "s"
	case d < time.Minute:
		return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
	case d < time.Hour:
		return d.Round(time.Second).String()
	default:
		return strings.TrimSuffix(d.Round(time.Minute).String(), "0s")
	}
}

// isID reports attributes that identify things rather than describe them; they go last.
func isID(k string) bool {
	k = strings.ToLower(k)
	if i := strings.LastIndexByte(k, '.'); i >= 0 {
		k = k[i+1:]
	}
	switch k {
	case "id", "uuid", "udid", "session", "command_uuid", "fingerprint", "request_id":
		return true
	}
	return strings.HasSuffix(k, "_id") || strings.HasSuffix(k, "_uuid")
}

// acronyms keep their case when they start a message ("mdm: …" → "MDM: …").
var acronyms = map[string]string{"mdm": "MDM", "ddm": "DDM", "laps": "LAPS", "apns": "APNs", "scep": "SCEP", "mcp": "MCP",
	"api": "API", "ade": "ADE", "dep": "DEP", "pkg": "PKG", "ea": "EA", "ssh": "SSH"}

// Message capitalises the first word (or restores a known acronym).
func Message(m string) string {
	if m == "" {
		return m
	}
	end := strings.IndexFunc(m, func(r rune) bool { return !unicode.IsLetter(r) })
	if end < 0 {
		end = len(m)
	}
	if up, ok := acronyms[m[:end]]; ok {
		return up + m[end:]
	}
	r, n := utf8.DecodeRuneInString(m)
	if unicode.IsLower(r) {
		return string(unicode.ToUpper(r)) + m[n:]
	}
	return m
}

// Quote quotes a value when it would otherwise break the key=value layout or the line.
func Quote(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\r\n\"=·") || !strconv.CanBackquote(s) {
		return strconv.Quote(s)
	}
	return s
}

// Body is the message and details without timestamp, level or component (also used for
// macOS unified logging, which records those itself).
func Body(msg string, kvs []KV) string {
	var b strings.Builder
	b.WriteString(Message(msg))
	var ids []KV
	first := true
	for _, kv := range kvs {
		if isID(kv.Key) {
			ids = append(ids, kv)
			continue
		}
		if first {
			b.WriteString(" ·")
			first = false
		}
		b.WriteString(" " + kv.Key + "=" + Quote(kv.Value))
	}
	if len(ids) > 0 {
		b.WriteString(" ·")
		for _, kv := range ids {
			b.WriteString(" " + kv.Key + "=" + Quote(kv.Value))
		}
	}
	return b.String()
}

// Line renders a full record, ending in a newline.
func Line(t time.Time, level slog.Level, component, msg string, kvs []KV) string {
	return fmt.Sprintf("%s %-5s %-11s %s\n", t.Format(TimeLayout), level.String(), component, Body(Explain(level, component, msg, kvs), kvs))
}

// failureWords mark messages that already say what went wrong.
var failureWords = []string{"fail", "error", "could not", "cannot", "can't", "unable", "refus", "reject", "denied", "invalid",
	"missing", "not ", "bad ", "lost", "timeout", "timed out", "throttled", "skipped", "panic", "unknown", "expired", "dropped", "stale",
	"locked", "offline", "unavailable", "too old", "conflict", "busy", "no ", "without", "retry", "retrying", "giving up", "aborted", "unreachable", "interrupted"}

// Explain makes a message read as a sentence in context: a leading "component: " that repeats
// the component column is dropped, and a warning or error that carries an error but is only a
// phrase ("replace apps") gets "failed" appended.
func Explain(level slog.Level, component, msg string, kvs []KV) string {
	hasErr := false
	for _, kv := range kvs {
		if kv.Key == "err" || kv.Key == "error" {
			hasErr = true
			break
		}
	}
	return explain(level, component, msg, hasErr)
}

// Readable is the message as shown in the console's log viewer: Explain plus capitalisation.
func Readable(level slog.Level, component, msg string, hasErr bool) string {
	return Message(explain(level, component, msg, hasErr))
}

func explain(level slog.Level, component, msg string, hasErr bool) string {
	if p, rest, ok := strings.Cut(msg, ": "); ok && strings.EqualFold(p, component) && rest != "" {
		msg = rest
	}
	if level < slog.LevelWarn || !hasErr {
		return msg
	}
	low := strings.ToLower(msg)
	for _, w := range failureWords {
		if strings.Contains(low, w) {
			return msg
		}
	}
	return msg + " failed"
}

// Parse extracts the level and component of a line written by Line, or by the older agent
// format ("2026-10-11T13:54:11.043Z INFO  [policy] …"). ok is false for other lines.
func Parse(line string) (lvl slog.Level, component string, ok bool) {
	f := strings.Fields(line)
	if len(f) >= 5 {
		if _, err := time.Parse(TimeLayout, f[0]+" "+f[1]+" "+f[2]); err == nil {
			if err := lvl.UnmarshalText([]byte(f[3])); err == nil {
				return lvl, f[4], true
			}
		}
	}
	if len(f) >= 3 && strings.HasPrefix(f[2], "[") && strings.HasSuffix(f[2], "]") {
		if _, err := time.Parse("2006-01-02T15:04:05.000Z07:00", f[0]); err == nil {
			if err := lvl.UnmarshalText([]byte(f[1])); err == nil {
				return lvl, strings.Trim(f[2], "[]"), true
			}
		}
	}
	return 0, "", false
}

// Handler is a slog.Handler writing Line output to w (the server's stdout). The "component"
// attribute becomes the component column (default given to NewHandler).
type Handler struct {
	w         io.Writer
	mu        *sync.Mutex
	level     slog.Leveler
	component string
	attrs     []KV
	group     string
}

// NewHandler returns a readable handler.
func NewHandler(w io.Writer, level slog.Leveler, component string) *Handler {
	return &Handler{w: w, mu: &sync.Mutex{}, level: level, component: component}
}

func (h *Handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level.Level() }

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	comp := h.component
	kvs := append([]KV(nil), h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "component" && h.group == "" {
			comp = a.Value.String()
			return true
		}
		kvs = Flatten(kvs, h.group, a)
		return true
	})
	line := Line(r.Time, r.Level, comp, r.Message, kvs)
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, line)
	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	n := *h
	n.attrs = append([]KV(nil), h.attrs...)
	for _, a := range attrs {
		if a.Key == "component" && h.group == "" {
			n.component = a.Value.String()
			continue
		}
		n.attrs = Flatten(n.attrs, h.group, a)
	}
	return &n
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	n := *h
	n.group = h.group + name + "."
	return &n
}
