package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
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

	// Ship (optional) uploads agent logs to the server.
	Ship *LogShipper
}

// NewDaemon creates a daemon.
func NewDaemon(cfg *Config, version string, logger *slog.Logger) *Daemon {
	client := NewClient(cfg.ServerURL, cfg.DeviceToken, version)
	return &Daemon{cfg: cfg, client: client, exec: NewExecutor(client, cfg, version, logger), spool: &Spooler{client: client, log: logger},
		log: logger, version: version, checkinNow: make(chan string, 16)}
}

// localTrigger is the message sent over the unix socket.
type localTrigger struct {
	Trigger string `json:"trigger"`
	User    string `json:"user"`
}

// Run executes the daemon until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	d.log.Info("vibemdm agent starting", "version", d.version, "server", d.cfg.ServerURL, "interval", d.cfg.CheckinIntervalSeconds)
	PruneCache()
	var wg sync.WaitGroup

	// Restricted-software enforcement, fed by each check-in's rule set.
	monitor := NewRestrictedMonitor(d.client, d.log)
	d.exec.OnRestrictedRules = monitor.SetRules
	wg.Add(1)
	go func() {
		defer wg.Done()
		monitor.Run(ctx)
	}()

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
				d.log.Debug("task poll failed", "err", err, "retry_in", backoff)
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
				d.runLocked(ctx, func(ctx context.Context) {
					for _, t := range resp.Tasks {
						d.exec.RunTask(ctx, t)
					}
				})
			}
			if resp.Checkin {
				d.log.Info("server requested an immediate check-in")
				d.checkin(ctx, "checkin")
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
		start := time.Now()
		n, err := d.exec.CheckinAndRun(ctx, trigger)
		if err != nil {
			d.log.Warn("check-in failed", "trigger", trigger, "err", err)
			ok = false
			return
		}
		d.log.Debug("check-in complete", "trigger", trigger, "policies", n, "dur", time.Since(start).Round(time.Millisecond))
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

func (d *Daemon) serveSocket(ctx context.Context) {
	os.Remove(SocketPath)
	old := syscall.Umask(0o111)
	ln, err := net.Listen("unix", SocketPath)
	syscall.Umask(old)
	if err != nil {
		d.log.Warn("cannot listen on trigger socket", "err", err)
		return
	}
	os.Chmod(SocketPath, 0o666)
	go func() {
		<-ctx.Done()
		ln.Close()
		os.Remove(SocketPath)
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		go d.handleConn(conn)
	}
}

func (d *Daemon) handleConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	var msg localTrigger
	if err := json.NewDecoder(conn).Decode(&msg); err != nil || msg.Trigger == "" {
		conn.Write([]byte(`{"error":"bad request"}` + "\n"))
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

// SendLocalTrigger asks a running daemon to fire a trigger.
func SendLocalTrigger(trigger, user string) error {
	conn, err := net.DialTimeout("unix", SocketPath, 3*time.Second)
	if err != nil {
		return errors.New("agent daemon is not running")
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := json.NewEncoder(conn).Encode(localTrigger{Trigger: trigger, User: user}); err != nil {
		return err
	}
	var resp map[string]any
	return json.NewDecoder(conn).Decode(&resp)
}
