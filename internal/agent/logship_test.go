package agent

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestLogShipperBuffersAndRetries(t *testing.T) {
	var fail atomic.Bool
	var got atomic.Int64
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "down", 503)
			return
		}
		got.Add(1)
		w.Write([]byte(`{"stored":1}`))
	}))
	defer srv.Close()
	s := &LogShipper{client: NewClient(srv.URL, "tok", "test"), minLvl: slog.LevelInfo}
	log := slog.New(s.Wrap(slog.NewTextHandler(&bytes.Buffer{}, nil))).With("component", "daemon")
	log.Info("one")
	log.Debug("not shipped")
	log.Warn("two", "k", "v")
	if len(s.buf) != 2 || s.buf[0].Component != "daemon" || s.buf[1].Attrs["k"] != "v" {
		t.Fatalf("buffer = %+v", s.buf)
	}
	if err := s.Flush(context.Background()); err == nil {
		t.Fatal("expected flush to fail while server is down")
	}
	if len(s.buf) != 2 {
		t.Fatal("records must be kept after a failed flush")
	}
	fail.Store(false)
	if err := s.Flush(context.Background()); err != nil || len(s.buf) != 0 || got.Load() != 1 {
		t.Fatalf("flush after recovery: err=%v buf=%d posts=%d", err, len(s.buf), got.Load())
	}
	// Bounded buffer keeps the most recent records.
	for i := 0; i < logBufferMax+10; i++ {
		log.Info("x")
	}
	if len(s.buf) != logBufferMax {
		t.Fatalf("buffer len = %d, want %d", len(s.buf), logBufferMax)
	}
}
