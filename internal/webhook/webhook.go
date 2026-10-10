// Package webhook delivers event callbacks, mirroring Jamf Pro's webhook
// feature: named endpoints subscribe to server events and receive signed
// JSON POSTs with retries.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
)

// Event names. "*" subscribes to everything.
const (
	ComputerEnrolled            = "ComputerEnrolled"           // MDM enrollment completed
	ComputerUnenrolled          = "ComputerUnenrolled"         // MDM CheckOut
	AgentEnrolled               = "AgentEnrolled"              // agent enrolled
	ComputerCheckIn             = "ComputerCheckIn"            // agent check-in
	ComputerInventoryCompleted  = "ComputerInventoryCompleted" // inventory stored
	PolicyCompleted             = "PolicyCompleted"
	PolicyFailed                = "PolicyFailed"
	MDMCommandCompleted         = "MDMCommandCompleted"
	RestrictedSoftwareViolation = "RestrictedSoftwareViolation"
	ShellSessionOpened          = "ShellSessionOpened"
	FileVaultKeyEscrowed        = "FileVaultKeyEscrowed"
	FileVaultKeyViewed          = "FileVaultKeyViewed"
	HealthIssueOpened           = "HealthIssueOpened"   // a warning/critical health finding opened or escalated
	HealthIssueResolved         = "HealthIssueResolved" // a previously announced finding cleared
)

// AllEvents lists every event name for the UI.
var AllEvents = []string{ComputerEnrolled, ComputerUnenrolled, AgentEnrolled, ComputerCheckIn,
	ComputerInventoryCompleted, PolicyCompleted, PolicyFailed, MDMCommandCompleted, RestrictedSoftwareViolation, ShellSessionOpened,
	FileVaultKeyEscrowed, FileVaultKeyViewed, HealthIssueOpened, HealthIssueResolved}

// Hook is a configured endpoint.
type Hook struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret,omitempty"`
	Enabled   bool      `json:"enabled"`
	Events    []string  `json:"events"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (h Hook) wants(event string) bool {
	for _, e := range h.Events {
		if e == "*" || e == event {
			return true
		}
	}
	return false
}

// Dispatcher fans events out to subscribed hooks in the background.
type Dispatcher struct {
	DB     *sql.DB
	Log    *slog.Logger
	Client *http.Client

	mu       sync.RWMutex
	hooks    []Hook
	loadedAt time.Time
	queue    chan delivery
	once     sync.Once
}

type delivery struct {
	hook    Hook
	event   string
	payload []byte
}

// New creates a dispatcher.
func New(d *sql.DB, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{DB: d, Log: logger, Client: &http.Client{Timeout: 15 * time.Second}, queue: make(chan delivery, 512)}
}

// Start launches the worker pool.
func (w *Dispatcher) Start(ctx context.Context) {
	w.once.Do(func() {
		for i := 0; i < 3; i++ {
			go w.worker(ctx)
		}
	})
}

// Invalidate clears the hook cache (call after hook CRUD).
func (w *Dispatcher) Invalidate() {
	w.mu.Lock()
	w.loadedAt = time.Time{}
	w.mu.Unlock()
}

func (w *Dispatcher) load(ctx context.Context) []Hook {
	w.mu.RLock()
	if time.Since(w.loadedAt) < 30*time.Second {
		defer w.mu.RUnlock()
		return w.hooks
	}
	w.mu.RUnlock()
	rows, err := w.DB.QueryContext(ctx, `SELECT id, name, url, secret, enabled, events, created_at, updated_at FROM webhooks WHERE enabled`)
	if err != nil {
		w.Log.Warn("loading webhooks", "err", err)
		return nil
	}
	defer rows.Close()
	var hooks []Hook
	for rows.Next() {
		var h Hook
		var ev db.JSON[[]string]
		if err := rows.Scan(&h.ID, &h.Name, &h.URL, &h.Secret, &h.Enabled, &ev, &h.CreatedAt, &h.UpdatedAt); err != nil {
			continue
		}
		h.Events = ev.V
		hooks = append(hooks, h)
	}
	w.mu.Lock()
	w.hooks, w.loadedAt = hooks, time.Now()
	w.mu.Unlock()
	return hooks
}

// Envelope is the JSON body delivered to endpoints.
type Envelope struct {
	Event     string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	Server    string    `json:"server"`
	Payload   any       `json:"payload"`
}

// Fire queues an event for delivery to all subscribed hooks. Non-blocking.
func (w *Dispatcher) Fire(event string, payload any) {
	hooks := w.load(context.Background())
	if len(hooks) == 0 {
		return
	}
	body, err := json.Marshal(Envelope{Event: event, Timestamp: time.Now().UTC(), Server: "vibemdm", Payload: payload})
	if err != nil {
		return
	}
	for _, h := range hooks {
		if !h.wants(event) {
			continue
		}
		select {
		case w.queue <- delivery{hook: h, event: event, payload: body}:
		default:
			w.Log.Warn("webhook queue full; dropping event", "event", event, "hook", h.Name)
		}
	}
}

func (w *Dispatcher) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case d := <-w.queue:
			w.deliver(ctx, d)
		}
	}
}

func (w *Dispatcher) deliver(ctx context.Context, d delivery) {
	var lastErr string
	var status int
	attempts := 0
	for attempt := 0; attempt < 3; attempt++ {
		attempts++
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.hook.URL, bytes.NewReader(d.payload))
		if err != nil {
			lastErr = err.Error()
			break
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "vibemdm-webhook")
		req.Header.Set("X-VibeMDM-Event", d.event)
		if d.hook.Secret != "" {
			mac := hmac.New(sha256.New, []byte(d.hook.Secret))
			mac.Write(d.payload)
			req.Header.Set("X-VibeMDM-Signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		}
		resp, err := w.Client.Do(req)
		if err != nil {
			lastErr = err.Error()
		} else {
			status = resp.StatusCode
			resp.Body.Close()
			if status < 300 {
				lastErr = ""
				break
			}
			lastErr = http.StatusText(status)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(5*(attempt+1)) * time.Second):
		}
	}
	w.DB.ExecContext(ctx, `INSERT INTO webhook_deliveries (webhook_id, event, status, error, attempts) VALUES ($1,$2,$3,$4,$5)`,
		d.hook.ID, d.event, status, lastErr, attempts)
	if lastErr != "" {
		w.Log.Warn("webhook delivery failed", "hook", d.hook.Name, "event", d.event, "err", lastErr)
	}
}
