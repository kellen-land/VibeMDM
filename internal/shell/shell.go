// Package shell implements the audited, admin-only remote root shell: a relay
// that pairs an admin browser WebSocket with the target Mac's agent WebSocket.
//
// The shell is disabled unless the remote_shell_enabled setting is on, is
// restricted to the admin role (enforced by the API layer), and every session
// is recorded (actor, computer, byte counts, and an output transcript).
package shell

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Frame is the JSON message exchanged on both WebSocket legs.
type Frame struct {
	T    string `json:"t"`              // "d" data, "r" resize, "x" exit, "e" error
	B    string `json:"b,omitempty"`    // base64 data for "d"
	Cols int    `json:"c,omitempty"`    // resize cols
	Rows int    `json:"w,omitempty"`    // resize rows
	Code int    `json:"code,omitempty"` // exit code
	Msg  string `json:"msg,omitempty"`  // error message
}

// Conn is the minimal WebSocket surface the relay needs (satisfied by coder/websocket).
type Conn interface {
	ReadFrame(ctx context.Context) (Frame, error)
	WriteFrame(ctx context.Context, f Frame) error
	Close(reason string)
}

// pending is a session awaiting the agent's callback.
type pending struct {
	computerID string
	nonce      string
	agentCh    chan Conn
	done       chan struct{}
}

// Hub coordinates pending sessions between the admin and agent handlers.
type Hub struct {
	mu      sync.Mutex
	pending map[string]*pending
}

// NewHub creates a Hub.
func NewHub() *Hub { return &Hub{pending: map[string]*pending{}} }

// Register creates a pending session the agent can claim with (id, nonce).
func (h *Hub) register(id, computerID, nonce string) *pending {
	p := &pending{computerID: computerID, nonce: nonce, agentCh: make(chan Conn, 1), done: make(chan struct{})}
	h.mu.Lock()
	h.pending[id] = p
	h.mu.Unlock()
	return p
}

func (h *Hub) unregister(id string) {
	h.mu.Lock()
	delete(h.pending, id)
	h.mu.Unlock()
}

// Claim is called by the agent handler. It hands its connection to the waiting
// admin handler and returns a channel closed when the session ends.
func (h *Hub) Claim(id, computerID, nonce string, agent Conn) (<-chan struct{}, error) {
	h.mu.Lock()
	p := h.pending[id]
	h.mu.Unlock()
	if p == nil {
		return nil, errors.New("unknown or expired shell session")
	}
	if p.computerID != computerID || subtleNE(p.nonce, nonce) {
		return nil, errors.New("shell session authorization failed")
	}
	select {
	case p.agentCh <- agent:
		return p.done, nil
	default:
		return nil, errors.New("shell session already claimed")
	}
}

// Result reports how a session ended.
type Result struct {
	Status     string // active->closed|timeout|error
	Reason     string
	Transcript string
	BytesIn    int64 // admin -> agent (keystrokes)
	BytesOut   int64 // agent -> admin (terminal output)
}

// Run registers a session, waits for the agent to connect, then relays until
// either side closes or the idle timeout fires. It always returns a Result for auditing.
func (h *Hub) Run(ctx context.Context, sessionID, computerID, nonce string, admin Conn, connectTimeout, idleTimeout time.Duration, onConnect func()) Result {
	p := h.register(sessionID, computerID, nonce)
	defer h.unregister(sessionID)
	defer close(p.done)

	var agent Conn
	select {
	case agent = <-p.agentCh:
	case <-time.After(connectTimeout):
		admin.WriteFrame(ctx, Frame{T: "e", Msg: "the Mac's agent did not connect (it may be offline)"})
		return Result{Status: "timeout", Reason: "agent did not connect"}
	case <-ctx.Done():
		return Result{Status: "error", Reason: "server shutting down"}
	}
	if onConnect != nil {
		onConnect()
	}

	relayCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer agent.Close("session ended")

	const maxTranscript = 1 << 20 // 1 MiB cap
	var mu sync.Mutex
	transcript := make([]byte, 0, 4096)
	truncated := false
	var bytesIn, bytesOut int64

	// idle timeout: reset on any activity.
	activity := make(chan struct{}, 1)
	bump := func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	}
	go func() {
		t := time.NewTimer(idleTimeout)
		defer t.Stop()
		for {
			select {
			case <-relayCtx.Done():
				return
			case <-activity:
				if !t.Stop() {
					select {
					case <-t.C:
					default:
					}
				}
				t.Reset(idleTimeout)
			case <-t.C:
				admin.WriteFrame(context.Background(), Frame{T: "e", Msg: "session closed after inactivity"})
				cancel()
				return
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	// admin -> agent (keystrokes / resize)
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			f, err := admin.ReadFrame(relayCtx)
			if err != nil {
				return
			}
			bump()
			if f.T == "d" {
				if raw, derr := base64.StdEncoding.DecodeString(f.B); derr == nil {
					bytesIn += int64(len(raw))
				}
			}
			if err := agent.WriteFrame(relayCtx, f); err != nil {
				return
			}
		}
	}()
	// agent -> admin (terminal output), teed into the transcript
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			f, err := agent.ReadFrame(relayCtx)
			if err != nil {
				return
			}
			bump()
			if f.T == "d" {
				if raw, derr := base64.StdEncoding.DecodeString(f.B); derr == nil {
					bytesOut += int64(len(raw))
					mu.Lock()
					if len(transcript) < maxTranscript {
						transcript = append(transcript, raw...)
						if len(transcript) >= maxTranscript {
							truncated = true
						}
					}
					mu.Unlock()
				}
			}
			if err := admin.WriteFrame(relayCtx, f); err != nil {
				return
			}
			if f.T == "x" {
				return
			}
		}
	}()
	wg.Wait()

	reason := "session closed"
	status := "closed"
	if relayCtx.Err() != nil && ctx.Err() != nil {
		reason = "server shutting down"
	}
	mu.Lock()
	tr := string(transcript)
	if truncated {
		tr += "\n…[transcript truncated]"
	}
	mu.Unlock()
	return Result{Status: status, Reason: reason, Transcript: tr, BytesIn: bytesIn, BytesOut: bytesOut}
}

// subtleNE reports whether two strings differ, in constant time.
func subtleNE(a, b string) bool {
	if len(a) != len(b) {
		return true
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v != 0
}

// wsConn adapts a coder/websocket connection to Conn using JSON text frames.
type wsConn struct {
	c *websocket.Conn
}

// Adapt wraps a coder/websocket connection. It raises the read limit so large
// terminal bursts and pastes are not rejected.
func Adapt(c *websocket.Conn) Conn {
	c.SetReadLimit(4 << 20)
	return &wsConn{c: c}
}

func (w *wsConn) ReadFrame(ctx context.Context) (Frame, error) {
	_, data, err := w.c.Read(ctx)
	if err != nil {
		return Frame{}, err
	}
	var f Frame
	if err := json.Unmarshal(data, &f); err != nil {
		return Frame{}, err
	}
	return f, nil
}

func (w *wsConn) WriteFrame(ctx context.Context, f Frame) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return w.c.Write(wctx, websocket.MessageText, b)
}

func (w *wsConn) Close(reason string) {
	if len(reason) > 120 {
		reason = reason[:120]
	}
	w.c.Close(websocket.StatusNormalClosure, reason)
}

// EncodeData builds a data frame from raw bytes.
func EncodeData(b []byte) Frame { return Frame{T: "d", B: base64.StdEncoding.EncodeToString(b)} }

// DecodeData returns the raw bytes of a data frame.
func DecodeData(f Frame) ([]byte, error) { return base64.StdEncoding.DecodeString(f.B) }
