# Security model and hardening

This page describes how VibeMDM protects the console, the agent channel and enrollment. It also lists the
settings an administrator should check before going to production. The [architecture notes](architecture.md)
cover encryption at rest and key rotation.

## Production checklist

| Do this | Why |
| --- | --- |
| Put VibeMDM behind a TLS-terminating reverse proxy (for example Nginx Proxy Manager) and set `VIBEMDM_SERVER_URL=https://…` | macOS requires HTTPS for MDM. The agent refuses plain-http servers unless told otherwise. |
| Firewall the published plain-http port (`13031` in `docker-compose.yml`) so only the proxy can reach it | The port is published on every host interface; anyone who reaches it bypasses TLS. |
| Set `VIBEMDM_ENCRYPTION_KEY` and back it up | Private keys and FileVault recovery keys are sealed at rest. |
| Set `VIBEMDM_TRUSTED_PROXIES` to your proxy's address/CIDR | Client IPs (rate limiting, audit) come from `X-Forwarded-For` only when it is set by a trusted proxy. |
| Change the bootstrap admin password at first login (enforced) | The generated password exists in `/data/initial-admin-password` until you change it. Delete the file afterwards. |
| Use invitations (single-use, expiring) rather than the master enrollment secret | Invitations limit exposure if a link leaks. |
| Leave Remote Shell off unless you need it | When it is on, an admin can open a root shell on any Mac with the agent. |

The Health page flags several of these automatically: no HTTPS, no encryption key, remote shell over plain HTTP.

## Console

- **Passwords:** bcrypt (cost 12), with a minimum of 8 characters (12 for the bootstrap password).
  - Placeholder bootstrap passwords such as `change-me-please` are refused at start-up.
  - The bootstrap account, and any account whose password an admin resets, must choose a new password at the
    next login. Until then every API call except `POST /api/v1/auth/password` returns
    `403 password change required`.
- **Sessions:**
  - 256-bit random tokens, stored as SHA-256 hashes.
  - Cookies are HttpOnly, SameSite=Lax, and Secure on HTTPS.
  - A password change ends every other session and issues a new cookie. An admin password reset ends all of the
    user's sessions.
- **Login throttling:**
  - Every 6 failures lock the client IP *and* the username for 1, then 5, 15 and 60 minutes.
  - Unknown usernames cost the same bcrypt work as real ones, so response timing does not reveal valid accounts.
- **CSRF:** cookie-authenticated requests that change state, including login and logout, must come from the
  console's own origin. API tokens (`Authorization: Bearer vmdm_…`) are exempt because browsers never send them
  automatically.
- **Headers:**
  - every response carries a strict Content-Security-Policy (no inline scripts, `frame-ancestors 'none'`),
    `X-Frame-Options: DENY`, `nosniff` and `Referrer-Policy: no-referrer`;
  - HSTS is sent when the server URL is HTTPS;
  - API responses are `Cache-Control: no-store`.
- **Roles:**
  - *Viewers* can read inventory and configuration but cannot change anything.
  - They also do not see secrets embedded in configuration: profile payloads are redacted like MDM command plists,
    script bodies are hidden, and task payloads lose command text and parameters.
  - Enrollment profile downloads, the SCEP challenge, the agent enrollment secret, FileVault keys, raw MDM payloads
    and shell transcripts are admin-only.
- **Logs:**
  - The request log never records query strings.
  - The Apple Configurator URL, which embeds a token, is logged as `/enroll/configurator/[redacted]`.
  - Webhook errors omit the URL.
  - The bootstrap password is never written to the searchable log table.
  - Upgrading purges any such rows written by older versions.

## Enrollment

- **SCEP challenges:**
  - Every enrollment profile carries a fresh SCEP challenge minted for that download: single use, valid 2 hours for
    user downloads, 1 hour for the agent, and 7 days for admin downloads.
  - Apple Configurator profiles get a challenge that can be reused for 24 hours, because one profile prepares a
    whole batch of devices.
  - The static challenge under **Settings → SCEP** still works for scripted enrollments and is visible only to
    admins.
- **Invitations:** each profile download from an invitation link uses up one of its uses. The public `/enroll`
  page never shows the master enrollment secret; the agent install step appears only for invitation links.
- **Agent install:** `install.sh` passes the token to the agent through an environment variable, not the command
  line, where other local users could see it with `ps`. The script refuses to run until a server URL is configured.
- **Takeover protection:** UDIDs and serial numbers are not secrets, so a new agent enrollment may replace an
  existing computer's agent only in three cases:
  - it presents that agent's current device token (a reinstall on the same Mac); or
  - an admin clicked **Reset agent** on the computer (`POST /api/v1/computers/{id}/reset-agent`); or
  - the old agent has been silent for 30 days.

  Refused attempts return `409` and are audited as `agent.enroll_refused`.

## Agent

- **Transport:**
  - Every request uses TLS with normal certificate verification.
  - `vibemdm-agent enroll` requires an `https://` server unless `--allow-insecure-http` is given.
  - The server refuses an `http://` server URL unless the host is loopback or `VIBEMDM_ALLOW_HTTP=1`.
  - Over plain HTTP, anyone on the network could inject tasks that run as root.
- **Local files:**
  - `agent.json` is written with mode 0600.
  - Scripts run from a private 0700 directory.
  - Packages are verified by SHA-256.
  - The MDM enrollment profile is saved with an exclusive create in `/Users/Shared`. Root never writes into a
    user's home folder.
- **Local trigger sockets:**
  - `/var/run/vibemdm-agent.sock` is world-writable but accepts only the built-in `login`, `checkin` and
    `network_change` triggers.
  - Custom triggers need the root-only `/var/run/vibemdm-agent.sock.root`. `sudo vibemdm-agent policy -trigger …`
    uses it automatically.
- **Self Service portal (127.0.0.1:8571):**
  - Requests must use the portal's own Host name, which defeats DNS rebinding.
  - Requests that run anything need a custom header and a JSON body. Other websites cannot send these without a
    CORS preflight, which the portal never approves.

## Remote shell

- **Gates:** the feature is gated by a server toggle (UI and API). It is admin-only, audited, and records a
  transcript.
- **Limits:**
  - one live shell per Mac, enforced by the database;
  - a configurable idle timeout;
  - a hard 4-hour maximum per session.
- **Turning it off** immediately ends every open shell.
- **Device-side veto:** if the root-owned file `/Library/Application Support/VibeMDM/disable-remote-shell`
  exists, the Mac refuses all shells regardless of server settings.

## Build and deployment

- The Compose file builds the image locally under a name no registry can serve (`vibemdm-local`), so
  `docker compose up` can never pull a look-alike image. Set `VIBEMDM_IMAGE` to run a published image.
- The container runs as an unprivileged user with every Linux capability dropped and `no-new-privileges`.
- `.dockerignore` keeps `.env`, keys, certificates and local notes out of the build context.
- `make dev-db` publishes PostgreSQL on `127.0.0.1` only.
- CI runs with a read-only token unless a job needs more, every third-party action is pinned to a commit SHA
  (Dependabot proposes updates), and each release carries a `SHA256SUMS` file for the agent binaries.
- The client IP for rate limiting and audit comes from `X-Forwarded-For` via a trusted proxy; `X-Real-IP` is ignored.
- Terminal hyperlinks in the remote shell and transcripts open only for `http(s)` addresses, after a prompt that
  shows the real address.

## Outbound requests

- Webhooks never follow redirects.
- They refuse loopback, link-local (including cloud metadata at 169.254.169.254) and unspecified addresses.
- Private RFC 1918 networks stay allowed, because self-hosted relays commonly live there.
- Deliveries are HMAC-signed (`X-VibeMDM-Signature`).
- The SOFA catalog fetch backs off for 5 minutes after a failure.

## Known limitations

- The at-rest encryption key is derived with SHA-256 from the passphrase, so use a long random value. Ciphertexts
  are not bound to their row. Webhook signing secrets, the SCEP challenge and the agent enrollment secret are
  stored unencrypted.
- Any enrolled agent can download any package by ID. Packages are treated as non-secret: don't put credentials
  in packages.
- Task output (stdout and stderr) remains visible to viewers. Avoid scripts that print secrets.
