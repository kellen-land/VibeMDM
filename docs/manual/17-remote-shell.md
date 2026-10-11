# 17. Remote shell

VibeMDM can open a live, interactive **root shell** on an enrolled Mac from the web console. It is the equivalent of
SSH or Jamf Remote, but it is tunnelled through the agent's existing outbound connection, so no inbound port is opened
on the Mac. Because it is the most powerful capability in the product, it is locked down by default.

## 17.1 Enable it

Remote shell is **off** until an admin turns it on:

- **Console:** Settings → General → Remote Shell → *Enable remote shell* (admin only).
- **API:** `PUT /api/v1/settings` `{"remote_shell_enabled": true}`, optionally
  `{"remote_shell_idle_timeout_secs": 600}` (minimum 30; default 600).

Turning it on or off is audited (`settings.remote_shell`). **Turning it off immediately ends every open shell.**

The Health check **Remote shell over plain HTTP** warns when the feature is on while the server URL is not HTTPS, and
offers *Turn remote shell off*.

## 17.2 Open a session

1. Open a computer that has the agent and choose the **Shell** tab (visible only to admins when the feature is on).
2. The browser opens a WebSocket to `/api/v1/computers/{id}/shell`.
3. The server checks the preconditions:

   | Check | Failure |
   | --- | --- |
   | Feature enabled | `403 the remote shell is disabled; enable it in Settings` |
   | Caller is an admin (session cookie or admin API token) | `403` |
   | Agent installed | `409 the VibeMDM agent is not installed on this computer` |
   | No other live session for this Mac (`pending` or `active`) | `409 a shell session is already active for this computer` |
   | `Origin` matches the server | The WebSocket upgrade is refused (blocks cross-site WebSocket hijacking) |

4. The server records a `shell_sessions` row (`pending`), writes `shell.open` to the audit log, fires the
   `ShellSessionOpened` webhook and asks the agent (through its long-poll) to dial back to `/agent/v1/shell` with a
   one-time nonce.
5. The agent starts the login shell (`$SHELL`, or `/bin/zsh`) **as root** under a pseudo-terminal, with
   `TERM=xterm-256color` and `VIBEMDM_SHELL=1`, and the server relays the two WebSockets together. The session becomes
   `active`. This takes about a second while the Mac is online.

If the agent is offline, no session opens. This is not a way into an offline Mac.

## 17.3 Limits

| Limit | Value |
| --- | --- |
| Live sessions per Mac | 1 (enforced in the database) |
| Idle timeout | `remote_shell_idle_timeout_secs` (default 10 minutes) |
| Maximum session length | 4 hours, hard limit |
| Transcript size | 1 MiB of output per session |
| Abandoned sessions | Marked `error` ("server restarted or session abandoned") after 1 hour |

## 17.4 Device-side veto

If the root-owned file `/Library/Application Support/VibeMDM/disable-remote-shell` exists on a Mac, that Mac refuses
every shell, whatever the server says. Deploy it with a package or script to Macs that must never accept one (for
example executives' or regulated Macs):

```bash
sudo touch "/Library/Application Support/VibeMDM/disable-remote-shell"
sudo chown root:wheel "/Library/Application Support/VibeMDM/disable-remote-shell"
```

## 17.5 Audit and review

Every session is recorded in `shell_sessions`:

| Field | Meaning |
| --- | --- |
| `actor`, `actor_ip` | Who opened it, from where |
| `status`, `reason` | `pending`, `active`, then how it ended (closed, idle timeout, max duration, feature disabled, error) |
| `started_at`, `connected_at`, `ended_at` | Timeline |
| `bytes_in`, `bytes_out` | Volume each way |
| transcript | The terminal output stream (up to 1 MiB). Keystrokes are not stored separately; the transcript shows what was echoed |

Review sessions in the console's Shell Sessions view, or with `GET /api/v1/shell-sessions` and
`GET /api/v1/shell-sessions/{id}` (transcript), both admin only. Opening and closing a session also writes
`shell.open` / `shell.close` audit entries.

## 17.6 Transport

Both legs speak JSON text frames over WebSocket:

| Frame | Meaning |
| --- | --- |
| `{"t":"d","b":"<base64>"}` | Data (keystrokes from the browser, output from the Mac) |
| `{"t":"r","c":<cols>,"w":<rows>}` | Terminal resize |
| `{"t":"x","code":<n>}` | The shell exited with this code |
| `{"t":"e","msg":"…"}` | Error |

## 17.7 Good practice

- Leave it off by default; turn it on for a support window, then off again.
- Use named admin accounts (not a shared `admin`) so the audit trail means something.
- Prefer scripts and policies for repeatable work; use the shell for diagnosis.
- Never paste secrets into the shell: the transcript stores what the terminal echoes, and admins can replay it.

> **Study note:** Apple's own remote-access options for Macs are **Screen Sharing / Remote Management (ARD)**, which
> MDM can turn on with `EnableRemoteDesktop`, and **Remote Login (SSH)**, which is configured in System Settings →
> General → Sharing or with `sudo systemsetup -setremotelogin on|off`. Security baselines usually require SSH to be off; an
> agent-relayed shell avoids opening an inbound port, but it concentrates power in the MDM, which is why it must be
> strongly authenticated, audited and optional.

Next: [Chapter 18 — Logging and diagnostics](18-logging-and-diagnostics.md).
