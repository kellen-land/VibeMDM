# Local administrator passwords (LAPS)

VibeMDM can give every Mac its own random local administrator password, rotate it on a schedule and after each
use, and keep it encrypted on the server. An administrator can reveal it with a reason, which is audited.
Console: **Security → Local Admin Passwords**.

> **Not yet proven on a real Mac.** The password generation, escrow protocol, retry logic and the whole server and
> console are tested, and the simulator runs the real agent code against a fake account database. The actual
> `sysadminctl` and `dscl` calls, Secure Token behaviour and FileVault interaction need a human on a real Mac
> (see "What needs a real Mac").

## How it works

1. You turn LAPS on and scope it (**Settings** on the page). Defaults: account `vibeadmin`, 20 characters, change
   every 30 days, change 60 minutes after a password is viewed.
2. At each check-in the agent asks `GET /agent/v1/laps`. The server answers whether a rotation is due and why:
   `missing`, `expired`, `used` (a view grace period ended), `requested` (admin clicked Rotate) or `account_changed`.
3. To rotate, the agent generates the password locally (`crypto/rand`, every character class, no ambiguous or
   shell-special characters), then:
   1. writes it to a root-only file `/Library/Application Support/VibeMDM/laps-pending.json`;
   2. **stages** it on the server (`POST /agent/v1/laps/stage`); if this fails the account is left alone;
   3. changes the account (`sysadminctl -resetPasswordFor` with the old password, falling back to
      `dscl . -passwd`; or `sysadminctl -addUser` when the account is missing and creation is allowed);
   4. checks the new password with `dscl . -authonly`;
   5. **commits** (`POST /agent/v1/laps/commit`) as applied or failed, then deletes the pending file.
4. Only a committed password becomes the current one. The server never discards the old password until the new one
   is confirmed.

### Failure modes

| What goes wrong | What happens |
| --- | --- |
| The stage upload fails | Nothing changed on the Mac; the next check-in tries again. |
| The change fails | The agent reports `failed`; the server discards the staged password and keeps the old one. The Mac shows *Needs attention* with the error. |
| The tool exits with an error but the change took effect | The agent trusts `dscl -authonly`, not the exit status, and commits it as applied. |
| The agent dies or loses the network after changing the account | The pending file survives. At the next cycle the agent tests it against the account and commits applied or failed. If the server forgot the stage (409), it stages again first. |
| A staged password is never confirmed | A Health finding opens after one hour; *Show password* displays the current and the unconfirmed password so you can try both. |
| The account does not exist and creation is off | The agent reports the problem; nothing is created. |

## Showing a password

*Show password* (admin only) asks for a reason, then returns the password with `Cache-Control: no-store`. It is
recorded in the audit log (`laps.password_viewed`, with reason, IP and fingerprint) and fires the
`LAPSPasswordViewed` webhook. If **Change the password after someone views it** is on, the rotation is scheduled
for the configured number of minutes later. Viewers can see status but never a password.

## Rotating now

*Rotate* (admin) sets a flag and queues a `laps_rotate` task; the agent rotates at once. The same remediation is
offered by the Health check **Local admin password not current** (`security.laps_not_current`).

## API (admin unless noted)

| Method and path | Purpose |
| --- | --- |
| `GET /api/v1/laps` (viewer) | Policy, per-Mac status and summary. Never contains a password |
| `PUT /api/v1/laps` | Save the policy |
| `GET /api/v1/computers/{id}/laps` (viewer) | One Mac's status |
| `POST /api/v1/computers/{id}/laps/reveal` `{"reason": "…"}` | Reveal (reason required) |
| `POST /api/v1/computers/{id}/laps/rotate` | Rotate now |

Agent endpoints (device token): `GET /agent/v1/laps`, `POST /agent/v1/laps/{stage,commit,error}`. The server
includes the current password in `GET /agent/v1/laps` only when a rotation is due, for `sysadminctl`'s
old-password argument, and only to that Mac's own token.

Audit actions: `laps.config`, `laps.rotated`, `laps.rotation_failed`, `laps.rotate_requested`,
`laps.password_viewed`. Webhooks: `LAPSPasswordRotated`, `LAPSPasswordViewed`.

## Storage and secrecy

Passwords are sealed with `VIBEMDM_ENCRYPTION_KEY` (`internal/secrets`) in `laps_passwords`. Set the key, or they
are stored unencrypted (Health warns). The fingerprint shown in the console is the first 8 hex digits of a SHA-256,
so it reveals nothing about the password. Passwords are never written to server or agent logs; the agent's log
handler also redacts attributes named like `password`, `token` or `secret`.

## What needs a real Mac

- `sysadminctl -resetPasswordFor … -oldPassword` and `dscl . -passwd` on current macOS, including on accounts that
  hold a **Secure Token** or are FileVault unlockers. Changing such an account's password with `dscl` as root can
  desynchronise it from FileVault, which is why the agent prefers `sysadminctl` with the old password. Test with
  your own account layout before scoping all Macs; consider a dedicated LAPS account that is not a FileVault user.
- Account creation with `sysadminctl -addUser` (hidden from the login window is not configured).
- The password is passed as a command-line argument for a moment. Other local users can see it with `ps` during
  that window. The agent runs these commands as root, briefly.
