package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/shell"
	"github.com/vibemdm/vibemdm/internal/webhook"
)

// shellWS is the admin-side endpoint for an interactive remote shell. It is
// admin-only, gated by the remote_shell_enabled setting, limited to one live
// session per computer, and fully audited (actor, IP, byte counts, transcript).
func (h *Handler) shellWS(w http.ResponseWriter, r *http.Request) {
	p := auth.FromContext(r.Context())
	settings, err := h.st.GetSettings(r.Context())
	if err != nil {
		writeErr(w, 500, "settings unavailable")
		return
	}
	if !settings.RemoteShellEnabled {
		writeErr(w, 403, "the remote shell is disabled; enable it in Settings")
		return
	}
	comp, err := h.st.GetComputer(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "computer")
		return
	}
	if !comp.AgentEnrolled {
		writeErr(w, 409, "the VibeMDM agent is not installed on this computer")
		return
	}
	if n, _ := h.st.ActiveShellCount(r.Context(), comp.ID); n > 0 {
		writeErr(w, 409, "a shell session is already active for this computer")
		return
	}

	// All preconditions passed — upgrade. coder/websocket verifies the Origin
	// against the Host by default, which blocks cross-site WebSocket hijacking.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
	if err != nil {
		return
	}
	ip := auth.ClientIP(r)
	sessionID, err := h.st.CreateShellSession(r.Context(), comp.ID, p.Username, ip)
	if err != nil {
		if strings.Contains(err.Error(), "shell_sessions_one_live_idx") {
			c.Close(websocket.StatusPolicyViolation, "a shell session is already active for this computer")
			return
		}
		c.Close(websocket.StatusInternalError, "audit record failed")
		return
	}
	h.st.Audit(r.Context(), p.Username, "shell.open", "computer", comp.ID, map[string]any{"session": sessionID, "ip": ip})
	h.hooks.Fire(webhook.ShellSessionOpened, map[string]any{"computer_id": comp.ID, "computer": comp.Name, "actor": p.Username, "ip": ip, "session": sessionID})
	h.log.Info("remote shell opened", "computer", comp.Name, "actor", p.Username, "session", sessionID, "ip", ip)

	nonce := auth.RandomToken(24)
	// Background context so the relay is not cancelled when the HTTP request
	// context ends on upgrade; it ends when either WebSocket closes.
	ctx := context.Background()
	h.notifier.RequestShell(comp.ID, model.ShellInvite{SessionID: sessionID, Nonce: nonce})

	idle := time.Duration(settings.RemoteShellIdleTimeoutSecs) * time.Second
	if idle <= 0 {
		idle = 10 * time.Minute
	}
	res := h.shellHub.Run(ctx, sessionID, comp.ID, nonce, shell.Adapt(c), 20*time.Second, idle, func() {
		h.st.MarkShellConnected(ctx, sessionID)
		h.log.Info("remote shell connected", "session", sessionID)
	})
	if err := h.st.CloseShellSession(ctx, sessionID, res.Status, res.Reason, res.Transcript, res.BytesIn, res.BytesOut); err != nil {
		h.log.Error("recording shell session transcript failed", "err", err, "session", sessionID)
		h.st.Audit(ctx, "system", "shell.record_failed", "computer", comp.ID, map[string]any{"session": sessionID, "error": err.Error()})
	}
	h.st.Audit(ctx, p.Username, "shell.close", "computer", comp.ID, map[string]any{"session": sessionID, "status": res.Status, "bytes_out": res.BytesOut})
	h.log.Info("remote shell closed", "session", sessionID, "status", res.Status, "bytes_in", res.BytesIn, "bytes_out", res.BytesOut)
	c.Close(websocket.StatusNormalClosure, "closed")
}

func (h *Handler) listShellSessions(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.st.ListShellSessions(r.Context(), r.URL.Query().Get("computer_id"), qInt(r, "limit", 50), qInt(r, "offset", 0))
	if err != nil {
		h.fail(w, err, "shell sessions")
		return
	}
	writeJSON(w, 200, listResp[model.ShellSession]{Items: items, Total: total})
}

func (h *Handler) getShellTranscript(w http.ResponseWriter, r *http.Request) {
	ss, transcript, err := h.st.GetShellTranscript(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err, "shell session")
		return
	}
	writeJSON(w, 200, map[string]any{"session": ss, "transcript": transcript})
}
