package logsink

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestHandlerTeesAndExtractsComponent(t *testing.T) {
	var buf bytes.Buffer
	sink := New(nil, slog.LevelInfo)
	h := Wrap(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}), sink)
	log := slog.New(h).With("component", "api")
	log.Info("hello", "user", "alice", "n", 3, "err", errors.New("boom"))
	log.Debug("noisy") // below sink level: forwarded, not stored
	if !bytes.Contains(buf.Bytes(), []byte("hello")) || !bytes.Contains(buf.Bytes(), []byte("noisy")) {
		t.Fatalf("next handler missed records: %s", buf.String())
	}
	if len(sink.ch) != 1 {
		t.Fatalf("sink got %d records, want 1", len(sink.ch))
	}
	e := <-sink.ch
	if e.Component != "api" || e.Msg != "hello" || e.Level != "INFO" {
		t.Fatalf("bad entry: %+v", e)
	}
	if e.Attrs["user"] != "alice" || e.Attrs["n"] != int64(3) || e.Attrs["err"] != "boom" {
		t.Fatalf("bad attrs: %+v", e.Attrs)
	}
	if _, ok := e.Attrs["component"]; ok {
		t.Fatal("component should be a column, not an attr")
	}
}

func TestDropsWhenFull(t *testing.T) {
	sink := &Sink{ch: make(chan entry, 1), minLvl: slog.LevelInfo}
	h := Wrap(slog.NewTextHandler(&bytes.Buffer{}, nil), sink)
	for i := 0; i < 3; i++ {
		h.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "x", 0))
	}
	if sink.Dropped() != 2 {
		t.Fatalf("dropped = %d, want 2", sink.Dropped())
	}
}
