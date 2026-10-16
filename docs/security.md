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

- **Passwords:** bcrypt (cost 12), 12 to 72 characters whenever a password is set.
  - Placeholder bootstrap passwords such as `change-me-please` are refused at start-up.
  - The bootstrap account, and any account whose password an admin resets, must choose a new password at the
    next login. Until then every API call except `POST /api/v1/auth/password` returns
    `403 password change required`.
- **Sessions:**
  - 256-bit random tokens, stored as SHA-256 hashes.
  - Cookies are HttpOnly, SameSite=Lax, and on HTTPS Secure with the `__Host-` prefix (a sibling subdomain cannot
    set or overwrite them).
  - A session ends after `VIBEMDM_SESSION_IDLE_MINUTES` (default 120) without requests, and after
    `VIBEMDM_SESSION_TTL_HOURS` (default 168) in any case.
  - A password change ends every other session and issues a new cookie. An admin password reset ends all of the
    user's sessions.
- **Login throttling:**
  - Every 6 failures lock the client IP *and* the username for 1, then 5, 15 and 60 minutes.
  - Attempts for the same client or username run one at a time, and at most 8 password checks run at once, so
    parallel requests cannot outrun the lockout. A successful login clears only that username's counter.
  - Changing your own password checks the current one under the same lockout.
  - Every lockout is audited (`auth.lockout`) and fires the `ConsoleLoginLockout` webhook.
  - Unknown usernames cost the same bcrypt work as real ones, so response timing does not reveal valid accounts.
- **CSRF:** cookie-authenticated requests that change state, including login and logout, must come from the
  console's own origin. Cookie requests the browser marks as cross-site (`Sec-Fetch-Site`) are refused for every
  method, because a few GETs have side effects (audited reveals, rotate-after-view). API tokens (`Authorization: Bearer vmdm_…`) are exempt because browsers never send them
  automatically.
- **Headers:**
  - every response carries a strict Content-Security-Policy (no inline scripts, `frame-ancestors 'none'`),
    `X-Frame-Options: DENY`, `nosniff` and `Referrer-Policy: no-referrer`;
  - HSTS is sent when the server URL is HTTPS;
  - API responses are `Cache-Control: no-store`.
- **API tokens:** a token stops working when the user who created it is disabled, and is deleted with that user.
  Roles must be exactly `admin` or `viewer`; anything else is rejected rather than treated as admin, and a user
  update leaves out fields unchanged.
- **Roles:**
  - *Viewers* can read inventory and configuration but cannot change anything.
  - They also do not see secrets embedded in configuration: profile payloads are redacted like MDM command plists
    (any key named like a password, secret, passphrase, token, challenge or license), Application & Custom
    Settings values are redacted the same way, script and extension attribute bodies are hidden, policies lose their
    command and script parameters, and task payloads lose command text and parameters.
  - Secret reveals (FileVault keys, LAPS passwords and their history) are refused if the audit entry cannot be
    written.
  - CSV exports are audited.
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

- **MDM re-enrollment guard:** UDIDs are not secrets either, so an `Authenticate` that presents a different
  identity certificate for a device that is still enrolled is refused with `403` and audited as
  `mdm.enroll_refused`. It is accepted when the device checked out (removed its MDM profile), when its enrollment
  has been silent for 30 days, or after an admin chose **Allow MDM re-enrollment**
  (`POST /api/v1/computers/{id}/allow-mdm-reenroll`, or `/mobile-devices/{id}/…`), for example after an erase.
  *Untested on real hardware:* re-enrolling a Mac after removing its profile relies on macOS sending `CheckOut`
  (the enrollment profile sets `CheckOutWhenRemoved`); an erased Mac needs the admin action or the 30-day window.
- **Revocation:** deleting a computer or mobile device revokes its identity certificate; `/mdm` refuses revoked
  certificates, so a removed device cannot quietly re-enroll with the certificate it still holds.
- **Bounded challenges:** each Apple Configurator download from an invitation link uses up one of its uses, and an
  agent on a Mac that is already MDM-enrolled cannot fetch further enrollment profiles.
- **Certificate requests:** the SCEP CA signs only RSA keys of 2048 bits or more and ECDSA keys on P-256 or larger,
  and copies no DNS or email names from the request into the device certificate.
- **Serial numbers are never proof:** an agent enrollment or MDM `Authenticate` only falls back to matching by
  serial number for a record that has no UDID yet (and, for MDM, no agent).
- `/mdm` request bodies are capped at 16 MB.

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
- **MDM bootstrap token:** the token in the MDM-installed agent profile works once, for its own Mac. If it is
  presented a second time, the server assumes something on that Mac read it and enrolled first: it revokes the
  agent identity issued with it (audited as `agent.bootstrap_reused`), so an admin resets and reinstalls the agent.
- **What a Mac may report about itself:** agent inventory only fills script-type, enabled extension attributes
  (never admin-entered text attributes), and the onboarding welcome form only sets the fields its workflow shows,
  with picker fields limited to their options.
- **Helpers run as root:** the agent runs swiftDialog only when the resolved binary and every folder above it are
  owned by root and not writable by others, and executes that resolved path.
- **Log uploads:** at most a burst of 30 uploads per Mac, then one every 6 seconds (`429` otherwise; the agent keeps
  its buffer). Levels are limited to DEBUG/INFO/WARN/ERROR, attributes to 8 KB, and timestamps to the last 30 days.
- **Self Service portal (127.0.0.1:8571):**
  - Requests must use the portal's own Host name, which defeats DNS rebinding.
  - Requests that run anything need a custom header and a JSON body. Other websites cannot send these without a
    CORS preflight, which the portal never approves.
  - The page sends a Content-Security-Policy that only runs its own script (per-request nonce) and escapes every
    value, including quotes, before it reaches the HTML.

## Remote shell

- **Gates:** the feature is gated by a server toggle (UI and API). It is admin-only, audited, and records a
  transcript.
- **Limits:**
  - one live shell per Mac, enforced by the database;
  - a configurable idle timeout;
  - a hard 4-hour maximum per session.
- **Turning it off** immediately ends every open shell, whether from Settings or the Health fix.
- **Transcripts** record terminal output, not keystrokes; see [remote-shell.md](remote-shell.md).
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
- Deliveries are HMAC-signed (`X-VibeMDM-Signature`). `X-VibeMDM-Signature-V2` also covers the
  `X-VibeMDM-Timestamp` header (HMAC of `timestamp + "." + body`), so receivers can reject old, replayed
  deliveries; prefer it.
- The SOFA catalog fetch backs off for 5 minutes after a failure.

## Known limitations

- The at-rest encryption key is derived with SHA-256 from the passphrase, so use a long random value. Ciphertexts
  are not bound to their row. Webhook signing secrets, the SCEP challenge and the agent enrollment secret are
  stored unencrypted.
- Any enrolled agent can download any package by ID. Packages are treated as non-secret: don't put credentials
  in packages.
- Task output (stdout and stderr) remains visible to viewers. Avoid scripts that print secrets.
- An agent proves its identity with a bearer token, not the Mac's MDM identity certificate. Anyone holding an
  enrollment token (master secret or invitation) and a Mac's UDID can enroll as the agent of an MDM-only Mac that
  has no agent yet. Prefer the MDM-installed agent (single-use bootstrap token) and short-lived invitations.
- LAPS rotation, policy local accounts and super's service-account setup pass passwords to `dscl`, `sysadminctl`
  and super as command-line arguments while they run, where other local users may see them with `ps`
  (untested on real hardware).
- The agent binary is installed in `/usr/local/bin`, which Homebrew makes user-writable on Intel Macs.
- Network segments use the IP address the agent reports, so a compromised Mac can choose its building and
  department.
