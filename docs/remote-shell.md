# Remote shell

VibeMDM can open a live, interactive **root shell** to an enrolled Mac from the web console — the
equivalent of Jamf Remote / SSH, but tunnelled through the agent's existing outbound connection so no
inbound port is opened on the Mac. Because this is the most powerful capability in the product, it is
locked down by default.

## Enabling it

Off until an administrator turns it on, in two equivalent ways:

- **Web UI**: Settings → Access → Remote shell → *Enable remote shell*, admin role only.
- **API**: `PUT /api/v1/settings` with `{"remote_shell_enabled": true}` (optionally
  `{"remote_shell_idle_timeout_secs": 600}`). Toggling it is written to the audit log.

## Using it

On a computer's detail page (agent-enrolled, admin only, feature enabled) open the **Shell** tab. The
browser connects to `/api/v1/computers/{id}/shell`; the server asks that Mac's agent to dial back and the
two are relayed together. A session starts within about a second while the Mac is online. If the agent is
offline the console reports that and no session opens — this is not a way into an offline machine.

## How it is secured and audited

- **Disabled by default**; gated by `remote_shell_enabled`.
- **Admin role only.** Viewers receive HTTP 403.
- **Same-origin only.** The WebSocket upgrade's `Origin` is verified against the server host, blocking
  cross-site WebSocket hijacking; the session cookie (or an API bearer token) still authenticates the caller.
- **One live session per Mac.**
- **Idle timeout** (default 10 minutes, configurable) closes inactive sessions.
- **Full audit.** Every session is recorded in `shell_sessions` with the actor, source IP, start/connect/end
  times, byte counts, and an output transcript (capped at 1 MiB). Opening and closing a session also writes
  `shell.open` / `shell.close` audit-log entries. Review sessions and replay transcripts under
  `GET /api/v1/shell-sessions` and `GET /api/v1/shell-sessions/{id}` (admin only), or in the console's
  Shell sessions view.

## Transport

Both legs speak JSON text frames over WebSocket: `{"t":"d","b":"<base64>"}` for data, `{"t":"r","c","w"}`
for terminal resize, `{"t":"x","code"}` on exit, `{"t":"e","msg"}` for errors. The agent runs the login
shell (`$SHELL` or `/bin/zsh`) under a PTY as root. The server never stores keystrokes separately (they
may be passwords typed at a prompt); the transcript is the terminal output stream. It shows what the shell
echoed, so input typed with echo turned off (`stty -echo`, password prompts) does not appear in it. Treat the
transcript as a record of what the operator saw, not proof of every command. Transcripts are cleaned of NUL bytes
and invalid UTF-8 before they are stored, and a failure to store one is audited as `shell.record_failed`.
