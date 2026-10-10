package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/coder/websocket"
	"github.com/creack/pty"

	"strings"

	"github.com/vibemdm/vibemdm/internal/agent"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/shell"
)

// handleShell dials back a shell invite like the real agent. On Linux (the dev
// box) it runs /bin/sh under a PTY so the relay can be exercised end to end.
func (d *device) handleShell(ctx context.Context, inv model.ShellInvite) {
	d.log.Info("sim: opening shell", "session", inv.SessionID)
	wsURL := simToWS(d.server) + "/agent/v1/shell?session=" + inv.SessionID + "&nonce=" + inv.Nonce
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	c, _, err := websocket.Dial(dctx, wsURL, &websocket.DialOptions{HTTPHeader: agent.AuthHeaderFor(d.agentTok, version)})
	cancel()
	if err != nil {
		d.log.Warn("sim shell dial failed", "err", err)
		return
	}
	conn := shell.Adapt(c)
	defer conn.Close("done")
	sh := "/bin/sh"
	if runtime.GOOS == "darwin" {
		sh = "/bin/zsh"
	}
	cmd := exec.Command(sh, "-i")
	cmd.Env = append(cmd.Env, "TERM=xterm-256color", "PS1=sim$ ")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		conn.WriteFrame(ctx, shell.Frame{T: "e", Msg: err.Error()})
		return
	}
	defer ptmx.Close()
	sctx, scancel := context.WithCancel(ctx)
	defer scancel()
	go func() {
		defer scancel()
		buf := make([]byte, 16384)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				conn.WriteFrame(sctx, shell.EncodeData(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		defer scancel()
		for {
			f, err := conn.ReadFrame(sctx)
			if err != nil {
				return
			}
			switch f.T {
			case "d":
				if raw, e := shell.DecodeData(f); e == nil {
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
	conn.WriteFrame(context.Background(), shell.Frame{T: "x", Code: 0})
	_ = fmt.Sprint
}

func simToWS(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasPrefix(base, "https://") {
		return "wss://" + strings.TrimPrefix(base, "https://")
	}
	return "ws://" + strings.TrimPrefix(base, "http://")
}
