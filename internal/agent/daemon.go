package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

// Daemon is the long-running agent process.
//
// Design goals: react to the server within a second (long-poll), keep periodic
// check-ins cheap and jittered, never lose results (disk spool), and recover
// from server/network outages with bounded backoff.
type Daemon struct {
	cfg     *Config
	client  *Client
	exec    *Executor
	spool   *Spooler
	log     *slog.Logger
	version string
	mu      sync.Mutex // serializes policy/task execution

	checkinNow chan string // trigger names requested out of band

	ssJobs *ssJobs         // Self Service runs and history
	ssApp  *selfServiceApp // the Self Service app in /Applications

	// Ship (optional) uploads agent logs to the server.
	Ship *LogShipper
	// LogLevel (optional) is the live on-device log level; root can change it
	// through the trigger socket (vibemdm-agent logs --set-level).
	LogLevel *slog.LevelVar
}

// NewDaemon creates a daemon.
func NewDaemon(cfg *Config, version string, logger *slog.Logger) *Daemon {
	client := NewClient(cfg.ServerURL, cfg.DeviceToken, version)
	ex := NewExecutor(client, cfg, version, logger)
	return &Daemon{cfg: cfg, client: client, exec: ex,
		spool: &Spooler{client: client, log: logger.With("component", "spool")},
		log:   logger.With("component", "daemon"), version: version, checkinNow: make(chan string, 16),
		ssApp: &selfServiceApp{appsDir: "/Applications", statePath: selfServiceState, version: version, log: logger,
			ensureDlg: func(ctx context.Context) error { return ex.ensureDialog(ctx, nil) }}}
}

// localTrigger is the message sent over the unix socket.
type localTrigger struct {
	Trigger  string `json:"trigger,omitempty"`
	User     string `json:"user,omitempty"`
	LogLevel string `json:"log_level,omitempty"` // root socket only
}

// Run executes the daemon until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	lvl := "info"
	if d.LogLevel != nil {
		lvl = LevelName(d.LogLevel.Level())
	}
	d.log.Info("vibemdm agent starting", "version", d.version, "server", d.cfg.ServerURL, "computer_id", d.cfg.ComputerID,
		"interval", d.cfg.CheckinIntervalSeconds, "log_level", lvl, "unified_logging", unifiedAvailable)
	PruneCache()
	var wg sync.WaitGroup

	// Restricted-software enforcement, fed by each check-in's rule set.
	monitor := NewRestrictedMonitor(d.client, d.log.With("component", "restricted"))
	d.exec.OnRestrictedRules = monitor.SetRules
	wg.Add(1)
	go func() {
		defer wg.Done()
		monitor.Run(ctx)
	}()

	// The Self Service app follows the configuration each check-in brings.
	d.exec.OnSelfService = func(c model.AgentSelfService) { d.ssApp.Apply(ctx, c) }

	// Local Self Service portal.
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.serveSelfService(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		d.serveSocket(ctx)
	}()

	// Startup trigger if we just booted, else a regular check-in.
	initial := "checkin"
	if up := Uptime(ctx); up > 0 && up < 10*time.Minute {
		initial = "startup"
	}
	d.checkin(ctx, initial)
	if initial == "startup" {
		d.checkin(ctx, "checkin")
	}

	// Long-poll for tasks and on-demand check-in requests.
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoff := time.Second
		for ctx.Err() == nil {
			resp, err := d.client.PollTasks(ctx, 55)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				if IsUnauthorized(err) {
					d.log.Error("device token rejected by server; re-enroll the agent", "err", err)
					sleepCtx(ctx, 5*time.Minute)
					continue
				}
				// Warn once when the connection drops, then stay quiet while retrying.
				if backoff == time.Second {
					d.log.Warn("server connection lost; retrying with backoff", "err", err)
				} else {
					d.log.Debug("task poll failed", "err", err, "retry_in", backoff)
				}
				sleepCtx(ctx, backoff)
				if backoff < 2*time.Minute {
					backoff *= 2
				}
				continue
			}
			if backoff > time.Second {
				d.log.Info("server connection restored")
				backoff = time.Second
				d.spool.Flush(ctx)
				if d.Ship != nil {
					go d.Ship.Flush(ctx)
				}
			}
			if len(resp.Tasks) > 0 {
				d.log.Info("tasks received", "count", len(resp.Tasks))
				d.runLocked(ctx, func(ctx context.Context) {
					for _, t := range resp.Tasks {
						d.exec.RunTask(ctx, t)
					}
				})
			}
			if resp.Checkin {
				d.log.Info("server requested an immediate check-in")
				d.runLocked(ctx, func(ctx context.Context) { d.exec.CheckinAndRunRequested(ctx) })
			}
			for _, inv := range resp.Shells {
				go d.runShell(ctx, shellInvite{SessionID: inv.SessionID, Nonce: inv.Nonce})
			}
		}
	}()

	// Periodic check-in with jitter; retry sooner after a failure.
	wg.Add(1)
	go func() {
		defer wg.Done()
		failures := 0
		for ctx.Err() == nil {
			interval := time.Duration(d.cfg.CheckinIntervalSeconds) * time.Second
			if failures > 0 {
				interval = min(time.Duration(30<<uint(min(failures-1, 4)))*time.Second, interval)
			}
			// ±10% jitter so a fleet never checks in as one burst.
			jitter := time.Duration(rand.Int64N(int64(interval)/5)) - interval/10
			select {
			case <-ctx.Done():
				return
			case trig := <-d.checkinNow:
				if d.checkin(ctx, trig) {
					failures = 0
				} else {
					failures++
				}
			case <-time.After(interval + jitter):
				if d.checkin(ctx, "checkin") {
					failures = 0
				} else {
					failures++
				}
			}
		}
	}()

	// Log shipping.
	if d.Ship != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Ship.Run(ctx)
		}()
	}

	// Spool retry.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for sleepCtx(ctx, 2*time.Minute) {
			d.spool.Flush(ctx)
		}
	}()

	<-ctx.Done()
	d.log.Info("vibemdm agent stopping")
	wg.Wait()
	return nil
}

// checkin performs a check-in for trigger under the execution lock. Returns success.
func (d *Daemon) checkin(ctx context.Context, trigger string) bool {
	ok := true
	d.runLocked(ctx, func(ctx context.Context) {
		// CheckinAndRun logs the result.
		if _, err := d.exec.CheckinAndRun(ctx, trigger); err != nil {
			ok = false
		}
	})
	return ok
}

func (d *Daemon) runLocked(ctx context.Context, fn func(context.Context)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	fn(ctx)
}

func sleepCtx(ctx context.Context, dur time.Duration) bool {
	t := time.NewTimer(dur)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// publicTriggers may be fired by any local user (the per-user LaunchAgent sends
// "login"); everything else, including custom triggers, needs the root socket.
var publicTriggers = map[string]bool{"login": true, "network_change": true, "checkin": true}

// RootSocketPath accepts any trigger and is only accessible to root.
const RootSocketPath = SocketPath + ".root"

func (d *Daemon) serveSocket(ctx context.Context) {
	go d.listenTrigger(ctx, RootSocketPath, 0o600, false)
	d.listenTrigger(ctx, SocketPath, 0o666, true)
}

func (d *Daemon) listenTrigger(ctx context.Context, path string, mode os.FileMode, restricted bool) {
	os.Remove(path)
	old := syscall.Umask(0o177)
	ln, err := net.Listen("unix", path)
	syscall.Umask(old)
	if err != nil {
		d.log.Warn("cannot listen on trigger socket", "path", path, "err", err)
		return
	}
	os.Chmod(path, mode)
	go func() {
		<-ctx.Done()
		ln.Close()
		os.Remove(path)
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		go d.handleConn(conn, restricted)
	}
}

func (d *Daemon) handleConn(conn net.Conn, restricted bool) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	var msg localTrigger
	if err := json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&msg); err != nil || (msg.Trigger == "" && msg.LogLevel == "") {
		conn.Write([]byte(`{"error":"bad request"}` + "\n"))
		return
	}
	if msg.LogLevel != "" {
		d.setLogLevel(conn, msg.LogLevel, restricted)
		return
	}
	if restricted && !publicTriggers[msg.Trigger] {
		d.log.Warn("refused local trigger from the unprivileged socket", "trigger", msg.Trigger)
		conn.Write([]byte(`{"error":"this trigger requires root (run with sudo)"}` + "\n"))
		return
	}
	d.log.Info("local trigger received", "trigger", msg.Trigger, "user", msg.User)
	select {
	case d.checkinNow <- msg.Trigger:
		conn.Write([]byte(`{"ok":true}` + "\n"))
	default:
		conn.Write([]byte(`{"error":"busy"}` + "\n"))
	}
}

func (d *Daemon) setLogLevel(conn net.Conn, level string, restricted bool) {
	l, err := ParseLogLevel(level)
	switch {
	case restricted:
		conn.Write([]byte(`{"error":"changing the log level requires root (run with sudo)"}` + "\n"))
	case err != nil:
		json.NewEncoder(conn).Encode(map[string]string{"error": err.Error()})
	case d.LogLevel == nil:
		conn.Write([]byte(`{"error":"this daemon does not support live log level changes"}` + "\n"))
	default:
		d.LogLevel.Set(l)
		d.log.Warn("log level changed", "level", LevelName(l)) // warn so the change shows at any level
		conn.Write([]byte(`{"ok":true}` + "\n"))
	}
}

// SendLocalLogLevel asks the running daemon to switch its log level now (root only).
func SendLocalLogLevel(level string) error {
	return sendLocal(RootSocketPath, localTrigger{LogLevel: level})
}

// SendLocalTrigger asks a running daemon to fire a trigger.
func SendLocalTrigger(trigger, user string) error {
	path := SocketPath
	if os.Geteuid() == 0 {
		path = RootSocketPath
	}
	return sendLocal(path, localTrigger{Trigger: trigger, User: user})
}

func sendLocal(path string, msg localTrigger) error {
	conn, err := net.DialTimeout("unix", path, 3*time.Second)
	if err != nil {
		return errors.New("agent daemon is not running")
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		return err
	}
	var resp map[string]any
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return err
	}
	if msg, _ := resp["error"].(string); msg != "" {
		return errors.New(msg)
	}
	return nil
}
