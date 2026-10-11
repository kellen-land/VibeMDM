package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

const (
	logSpoolPath  = SupportDir + "/spool/logs.ndjson"
	logBufferMax  = 5000
	logBatchSize  = 500
	logSpoolBytes = 5 << 20
)

// LogShipper is an slog.Handler wrapper that forwards records to the local log
// and buffers them for upload to the server. When the server is unreachable,
// records stay buffered (bounded) and are spooled to disk on shutdown, so a
// reboot during an outage doesn't lose them.
type LogShipper struct {
	client *Client
	minLvl slog.Level
	mu     sync.Mutex
	buf    []model.AgentLogEntry
}

// NewLogShipper creates a shipper. Records below minLevel are not uploaded.
func NewLogShipper(client *Client, minLevel slog.Level) *LogShipper {
	s := &LogShipper{client: client, minLvl: minLevel}
	s.loadSpool()
	return s
}

// Wrap returns a handler that writes to next and buffers for upload.
func (s *LogShipper) Wrap(next slog.Handler) slog.Handler { return &shipHandler{next: next, s: s} }

func (s *LogShipper) add(e model.AgentLogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buf) >= logBufferMax {
		s.buf = s.buf[len(s.buf)-logBufferMax+1:] // keep the most recent records
	}
	s.buf = append(s.buf, e)
}

// Flush uploads buffered records in batches. On failure the remaining records
// stay buffered for the next attempt.
func (s *LogShipper) Flush(ctx context.Context) error {
	for {
		s.mu.Lock()
		n := len(s.buf)
		if n == 0 {
			s.mu.Unlock()
			return nil
		}
		if n > logBatchSize {
			n = logBatchSize
		}
		batch := append([]model.AgentLogEntry(nil), s.buf[:n]...)
		s.mu.Unlock()
		if err := s.client.ShipLogs(ctx, batch); err != nil {
			return err
		}
		s.mu.Lock()
		if len(s.buf) >= n {
			s.buf = s.buf[n:]
		}
		s.mu.Unlock()
	}
}

// Run flushes every 30 seconds until ctx ends, then spools what's left.
func (s *LogShipper) Run(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := s.Flush(fctx); err != nil {
				s.saveSpool()
			}
			cancel()
			return
		case <-t.C:
			s.Flush(ctx)
		}
	}
}

func (s *LogShipper) saveSpool() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buf) == 0 {
		return
	}
	os.MkdirAll(filepath.Dir(logSpoolPath), 0o700)
	f, err := os.OpenFile(logSpoolPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > logSpoolBytes {
		return // cap disk usage during long outages
	}
	enc := json.NewEncoder(f)
	for _, e := range s.buf {
		enc.Encode(e)
	}
	s.buf = nil
}

func (s *LogShipper) loadSpool() {
	f, err := os.Open(logSpoolPath)
	if err != nil {
		return
	}
	defer func() {
		f.Close()
		os.Remove(logSpoolPath)
	}()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var e model.AgentLogEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			s.add(e)
		}
	}
}

type shipHandler struct {
	next  slog.Handler
	s     *LogShipper
	attrs []slog.Attr
}

func (h *shipHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l) || l >= h.s.minLvl
}

func (h *shipHandler) Handle(ctx context.Context, r slog.Record) error {
	var err error
	if h.next.Enabled(ctx, r.Level) {
		err = h.next.Handle(ctx, r)
	}
	if r.Level < h.s.minLvl {
		return err
	}
	e := model.AgentLogEntry{At: r.Time.UTC(), Level: r.Level.String(), Msg: r.Message, Attrs: map[string]any{}}
	add := func(a slog.Attr) {
		v := a.Value.Resolve()
		if a.Key == "component" {
			e.Component = v.String()
			return
		}
		if isSecretKey(a.Key) {
			e.Attrs[a.Key] = "[redacted]"
			return
		}
		if errv, ok := v.Any().(error); ok {
			e.Attrs[a.Key] = errv.Error()
			return
		}
		switch v.Kind() {
		case slog.KindString:
			e.Attrs[a.Key] = v.String()
		case slog.KindInt64:
			e.Attrs[a.Key] = v.Int64()
		case slog.KindBool:
			e.Attrs[a.Key] = v.Bool()
		case slog.KindFloat64:
			e.Attrs[a.Key] = v.Float64()
		case slog.KindDuration:
			e.Attrs[a.Key] = v.Duration().Round(time.Millisecond).String()
		default:
			e.Attrs[a.Key] = fmt.Sprint(v.Any())
		}
	}
	for _, a := range h.attrs {
		add(a)
	}
	r.Attrs(func(a slog.Attr) bool { add(a); return true })
	h.s.add(e)
	return err
}

func (h *shipHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &shipHandler{next: h.next.WithAttrs(attrs), s: h.s, attrs: append(append([]slog.Attr{}, h.attrs...), attrs...)}
}

func (h *shipHandler) WithGroup(name string) slog.Handler {
	return &shipHandler{next: h.next.WithGroup(name), s: h.s, attrs: h.attrs}
}
