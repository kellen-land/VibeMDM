package shell

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeConn is an in-memory Conn pair for testing the relay.
type fakeConn struct {
	in     chan Frame
	out    chan Frame
	closed bool
	mu     sync.Mutex
}

func newPair() (*fakeConn, *fakeConn) {
	a2b := make(chan Frame, 64)
	b2a := make(chan Frame, 64)
	return &fakeConn{in: b2a, out: a2b}, &fakeConn{in: a2b, out: b2a}
}

func (f *fakeConn) ReadFrame(ctx context.Context) (Frame, error) {
	select {
	case fr, ok := <-f.in:
		if !ok {
			return Frame{}, errors.New("closed")
		}
		return fr, nil
	case <-ctx.Done():
		return Frame{}, ctx.Err()
	}
}

func (f *fakeConn) WriteFrame(ctx context.Context, fr Frame) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errors.New("closed")
	}
	select {
	case f.out <- fr:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *fakeConn) Close(string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.out)
	}
}

func TestRelayPairsAndRecordsTranscript(t *testing.T) {
	hub := NewHub()
	adminSrv, adminCli := newPair()
	agentSrv, agentCli := newPair()

	var res Result
	done := make(chan struct{})
	go func() {
		res = hub.Run(context.Background(), "s1", "c1", "secret", adminSrv, 2*time.Second, time.Minute, nil)
		close(done)
	}()

	// Agent claims the session.
	time.Sleep(50 * time.Millisecond)
	if _, err := hub.Claim("s1", "c1", "secret", agentSrv); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Admin sends a keystroke; it must reach the agent.
	rctx, rcancel := context.WithTimeout(context.Background(), time.Second)
	defer rcancel()
	adminCli.WriteFrame(context.Background(), EncodeData([]byte("ls\n")))
	if f, err := agentCli.ReadFrame(rctx); err != nil {
		t.Fatalf("agent never received keystroke: %v", err)
	} else if b, _ := DecodeData(f); string(b) != "ls\n" {
		t.Fatalf("agent got %q", b)
	}
	// Agent sends output; it must reach the admin and be recorded.
	agentCli.WriteFrame(context.Background(), EncodeData([]byte("file1 file2\n")))
	if f, err := adminCli.ReadFrame(rctx); err != nil {
		t.Fatalf("admin never received output: %v", err)
	} else if b, _ := DecodeData(f); string(b) != "file1 file2\n" {
		t.Fatalf("admin got %q", b)
	}
	// Agent exits → session ends.
	agentCli.WriteFrame(context.Background(), Frame{T: "x", Code: 0})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not end on exit")
	}
	if res.BytesIn != 3 || res.BytesOut != 12 {
		t.Errorf("byte counts in=%d out=%d", res.BytesIn, res.BytesOut)
	}
	if res.Transcript != "file1 file2\n" {
		t.Errorf("transcript = %q", res.Transcript)
	}
}

func TestClaimRejectsBadNonce(t *testing.T) {
	hub := NewHub()
	admin, _ := newPair()
	go hub.Run(context.Background(), "s2", "c1", "right", admin, time.Second, time.Minute, nil)
	time.Sleep(50 * time.Millisecond)
	agent, _ := newPair()
	if _, err := hub.Claim("s2", "c1", "wrong", agent); err == nil {
		t.Fatal("expected nonce mismatch to be rejected")
	}
	if _, err := hub.Claim("s2", "other", "right", agent); err == nil {
		t.Fatal("expected computer mismatch to be rejected")
	}
}

func TestConnectTimeout(t *testing.T) {
	hub := NewHub()
	admin, _ := newPair()
	res := hub.Run(context.Background(), "s3", "c1", "n", admin, 100*time.Millisecond, time.Minute, nil)
	if res.Status != "timeout" {
		t.Fatalf("status = %q, want timeout", res.Status)
	}
}

var _ = base64.StdEncoding

func TestExitCodeReachesTheAdminAndTheSessionClosesCleanly(t *testing.T) {
	hub := NewHub()
	adminSrv, adminCli := newPair()
	agentSrv, agentCli := newPair()
	done := make(chan struct{})
	go func() {
		hub.Run(context.Background(), "s2", "c1", "n", adminSrv, 2*time.Second, time.Minute, nil)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	if _, err := hub.Claim("s2", "c1", "n", agentSrv); err != nil {
		t.Fatal(err)
	}
	agentCli.WriteFrame(context.Background(), Frame{T: "x", Code: 3})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	f, err := adminCli.ReadFrame(ctx)
	if err != nil || f.T != "x" || f.Code != 3 {
		t.Fatalf("admin got %+v %v, want exit 3", f, err)
	}
	if _, err := adminCli.ReadFrame(ctx); err == nil || err == context.DeadlineExceeded {
		t.Fatalf("admin connection should be closed by the server after the exit, got %v", err)
	}
	<-done
}

func TestAgentVanishingIsReported(t *testing.T) {
	hub := NewHub()
	adminSrv, adminCli := newPair()
	agentSrv, agentCli := newPair()
	go hub.Run(context.Background(), "s3", "c1", "n", adminSrv, 2*time.Second, time.Minute, nil)
	time.Sleep(50 * time.Millisecond)
	if _, err := hub.Claim("s3", "c1", "n", agentSrv); err != nil {
		t.Fatal(err)
	}
	agentCli.Close("gone") // no exit frame
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	f, err := adminCli.ReadFrame(ctx)
	if err != nil || f.T != "e" {
		t.Fatalf("admin got %+v %v, want an error frame", f, err)
	}
}
