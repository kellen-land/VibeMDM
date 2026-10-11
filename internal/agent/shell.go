package agent

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/creack/pty"

	"github.com/vibemdm/vibemdm/internal/shell"
)

// runShell dials the server's agent shell endpoint for a session invite and
// bridges a PTY-backed root shell to it. The server enforces authorization,
// auditing and the remote_shell_enabled gate; here we just honour an invite we
// received on our authenticated poll connection.
// ShellOptOutFile, when present and owned by root, makes this Mac refuse remote shells
// regardless of server settings (a local, device-side veto).
const ShellOptOutFile = "/Library/Application Support/VibeMDM/disable-remote-shell"

func shellOptedOut() bool {
	fi, err := os.Stat(ShellOptOutFile)
	if err != nil {
		return false
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Uid != 0 {
		return false // only root may create the veto
	}
	return true
}

func (d *Daemon) runShell(ctx context.Context, invite shellInvite) {
	log := d.log.With("component", "shell", "session", invite.SessionID)
	if shellOptedOut() {
		log.Warn("refusing remote shell: disabled on this Mac", "file", ShellOptOutFile)
		return
	}
	log.Info("opening remote shell session")
	start := time.Now()
	wsURL := toWS(d.cfg.ServerURL) + "/agent/v1/shell?session=" + invite.SessionID + "&nonce=" + invite.Nonce
	dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	c, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPHeader: authHeader(d.cfg.DeviceToken, d.version),
	})
	cancel()
	if err != nil {
		log.Warn("remote shell dial failed", "err", err)
		return
	}
	conn := shell.Adapt(c)
	defer conn.Close("shell ended")

	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		shellPath = "/bin/zsh"
	}
	cmd := exec.Command(shellPath, "-il")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "VIBEMDM_SHELL=1")
	cmd.Dir = "/var/root"
	if _, err := os.Stat(cmd.Dir); err != nil {
		cmd.Dir = "/"
	}
	ptmx, err := pty.Start(cmd)
	if err != nil {
		conn.WriteFrame(ctx, shell.Frame{T: "e", Msg: "failed to start shell: " + err.Error()})
		return
	}
	defer func() { _ = ptmx.Close() }()
	pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80})

	sctx, scancel := context.WithCancel(ctx)
	defer scancel()

	// PTY output -> server
	go func() {
		defer scancel()
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if werr := conn.WriteFrame(sctx, shell.EncodeData(buf[:n])); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	// server -> PTY (keystrokes / resize)
	go func() {
		defer scancel()
		for {
			f, err := conn.ReadFrame(sctx)
			if err != nil {
				return
			}
			switch f.T {
			case "d":
				raw, derr := shell.DecodeData(f)
				if derr == nil {
					ptmx.Write(raw)
				}
			case "r":
				if f.Cols > 0 && f.Rows > 0 {
					pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(f.Rows), Cols: uint16(f.Cols)})
				}
			case "x":
				return
			}
		}
	}()

	<-sctx.Done()
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
	cmd.Wait()
	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	conn.WriteFrame(context.Background(), shell.Frame{T: "x", Code: code})
	log.Info("remote shell session ended", "exit", code, "dur", time.Since(start))
}

type shellInvite struct {
	SessionID string
	Nonce     string
}
