// Package logsink tees structured server logs into PostgreSQL so they can be
// searched and tailed from the console. Writes are batched and asynchronous:
// logging never blocks request handling, and if the database is slow or down
// records are dropped (and counted) rather than piling up in memory.
package logsink

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vibemdm/vibemdm/internal/logfmt"
)

type entry struct {
	At        time.Time      `json:"at"`
	Level     string         `json:"level"`
	Component string         `json:"component"`
	Msg       string         `json:"msg"`
	Attrs     map[string]any `json:"attrs"`
}

// Sink batches log entries into the server_logs table.
type Sink struct {
	db      *sql.DB
	ch      chan entry
	minLvl  slog.Level
	dropped atomic.Int64
	once    sync.Once
}

// New creates a sink. Records below minLevel are not stored.
func New(db *sql.DB, minLevel slog.Level) *Sink {
	return &Sink{db: db, ch: make(chan entry, 8192), minLvl: minLevel}
}

// Dropped returns how many records were discarded because the buffer was full.
func (s *Sink) Dropped() int64 { return s.dropped.Load() }

// Start runs the batch writer until ctx ends (then flushes what's buffered).
func (s *Sink) Start(ctx context.Context, onErr func(error)) {
	s.once.Do(func() { go s.run(ctx, onErr) })
}

func (s *Sink) run(ctx context.Context, onErr func(error)) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	batch := make([]entry, 0, 500)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		b, _ := json.Marshal(batch)
		fctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := s.db.ExecContext(fctx, `INSERT INTO server_logs (at, level, component, msg, attrs)
			SELECT at, level, COALESCE(component,''), msg, COALESCE(attrs,'{}'::jsonb)
			FROM jsonb_to_recordset($1::jsonb) AS x(at timestamptz, level text, component text, msg text, attrs jsonb)`, string(b))
		cancel()
		if err != nil && onErr != nil {
			onErr(err)
		}
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case e := <-s.ch:
					batch = append(batch, e)
				default:
					flush()
					return
				}
			}
		case e := <-s.ch:
			batch = append(batch, e)
			if len(batch) >= 500 {
				flush()
			}
		case <-t.C:
			flush()
		}
	}
}

// Handler wraps an slog.Handler, forwarding every record to it and teeing
// records at or above the sink's level into the database.
type Handler struct {
	next  slog.Handler
	sink  *Sink
	attrs []slog.Attr
	group string
}

// Wrap returns a handler that tees into sink.
func Wrap(next slog.Handler, sink *Sink) *Handler { return &Handler{next: next, sink: sink} }

func (h *Handler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l) || l >= h.sink.minLvl
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	var err error
	if h.next.Enabled(ctx, r.Level) {
		err = h.next.Handle(ctx, r)
	}
	if r.Level < h.sink.minLvl {
		return err
	}
	e := entry{At: r.Time, Level: r.Level.String(), Msg: r.Message, Attrs: map[string]any{}}
	add := func(a slog.Attr) {
		a.Value = a.Value.Resolve()
		if a.Key == "component" {
			e.Component = a.Value.String()
			return
		}
		key := a.Key
		if h.group != "" {
			key = h.group + "." + key
		}
		if logfmt.IsSecretKey(a.Key) {
			e.Attrs[key] = "[redacted]"
			return
		}
		e.Attrs[key] = attrValue(a.Value)
	}
	for _, a := range h.attrs {
		add(a)
	}
	r.Attrs(func(a slog.Attr) bool { add(a); return true })
	_, hasErr := e.Attrs["err"]
	e.Msg = logfmt.Readable(r.Level, e.Component, e.Msg, hasErr)
	select {
	case h.sink.ch <- e:
	default:
		h.sink.dropped.Add(1)
	}
	return err
}

func attrValue(v slog.Value) any {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return v.Int64()
	case slog.KindUint64:
		return v.Uint64()
	case slog.KindFloat64:
		return v.Float64()
	case slog.KindBool:
		return v.Bool()
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindGroup:
		m := map[string]any{}
		for _, a := range v.Group() {
			m[a.Key] = attrValue(a.Value.Resolve())
		}
		return m
	default:
		if err, ok := v.Any().(error); ok {
			return err.Error()
		}
		return fmt.Sprint(v.Any())
	}
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	n := *h
	n.next = h.next.WithAttrs(attrs)
	n.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &n
}

func (h *Handler) WithGroup(name string) slog.Handler {
	n := *h
	n.next = h.next.WithGroup(name)
	if n.group != "" {
		n.group += "." + name
	} else {
		n.group = name
	}
	return &n
}
