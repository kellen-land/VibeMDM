package store

import (
	"context"
	"testing"
	"time"
)

func TestSessionIdleTimeout(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	name := "idle-" + time.Now().Format("150405.000000")
	u, err := s.CreateUser(ctx, name, name, "", "viewer", "x")
	if err != nil {
		t.Fatal(err)
	}
	h := HashToken("session-" + name)
	if err := s.CreateSession(ctx, h, u.ID, time.Hour, "test", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSessionUser(ctx, h, time.Hour); err != nil {
		t.Fatalf("fresh session refused: %v", err)
	}
	s.DB.ExecContext(ctx, `UPDATE sessions SET last_seen_at = now() - interval '2 hours' WHERE token_hash = $1`, h)
	if _, err := s.GetSessionUser(ctx, h, time.Hour); err == nil {
		t.Fatal("session idle for 2 hours accepted with a 1-hour idle timeout")
	}
	if _, err := s.GetSessionUser(ctx, h, 0); err != nil {
		t.Fatalf("idle timeout disabled, session refused: %v", err)
	}
}
