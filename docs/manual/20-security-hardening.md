# 20. Security hardening

An MDM server can run code as root on every Mac it manages. Treat it as one of your most sensitive systems. This
chapter summarizes VibeMDM's security model ([`docs/security.md`](../security.md) has the full write-up) and gives
an operational checklist.

## 20.1 Production checklist

| # | Do this | Why | How to verify |
| --- | --- | --- | --- |
| 1 | Put VibeMDM behind TLS (Caddy `tls` profile) and set `VIBEMDM_SERVER_URL=https://…` | macOS requires HTTPS for MDM; agents refuse plain-HTTP servers | Health: no *Server URL* finding |
| 2 | Keep `VIBEMDM_BIND=127.0.0.1` | The plain-HTTP port is reachable only through the proxy | `ss -ltnp` on the host shows 8080 on 127.0.0.1 |
| 3 | Set `VIBEMDM_ENCRYPTION_KEY` (48 random bytes) and back it up separately | Private keys and FileVault recovery keys are sealed at rest | Health: no *Encryption at rest* finding |
| 4 | Set `VIBEMDM_TRUSTED_PROXIES` to your proxy's address/CIDR | Client IPs for rate limiting and audit come from `X-Forwarded-For` only from a trusted proxy | Audit entries show real client IPs |
| 5 | Change the bootstrap admin password (enforced) and delete `/data/initial-admin-password` | The generated password sits in that file until removed | `docker compose exec vibemdm ls /data` |
| 6 | Create named admin accounts; give read-only staff the **viewer** role | Accountability and least privilege | Settings → Users |
| 7 | Use invitations (single-use or limited, expiring) instead of the master enrollment secret | Limits exposure if a link leaks | Enrollment page |
| 8 | Leave **Open enrollment** off | Otherwise anyone who can reach `/enroll` can download a profile | Settings → General |
| 9 | Leave **Remote shell** off unless needed; consider the device-side veto file on sensitive Macs | With it on, an admin can open a root shell on any Mac with the agent | Settings → General |
| 10 | Leave **MCP** off unless needed; use viewer tokens; keep *Allow write tools* off unless required | AI agents act with the token's rights | Settings → General, Settings → API Tokens |
| 11 | Give API tokens an expiry and the viewer role where possible; revoke unused ones | Long-lived admin tokens are attractive targets | Settings → API Tokens (`last_used_at`) |
| 12 | Sign webhook deliveries with a secret and verify the signature | Receivers can authenticate VibeMDM | Webhook config |
| 13 | Back up the database, `/data` and the key; test a restore | Recovery from loss or ransomware | [§2.7](02-installation.md#27-backups) |
| 14 | Renew the APNs certificate early, with the same Apple Account | Losing the topic means re-enrolling every Mac | Health: *APNs push certificate* |
| 15 | Keep the host, Docker, PostgreSQL and VibeMDM up to date | Patch vulnerabilities | Release notes |
| 16 | Sign and notarize the agent binaries if you have a Developer ID | Avoids clearing quarantine and proves origin | `codesign -dv /usr/local/bin/vibemdm-agent` |
| 17 | Review the audit log and Health regularly | Detect misuse and drift | Settings → Audit log, Health |

## 20.2 Console security

| Area | Controls |
| --- | --- |
| Passwords | bcrypt cost 12; minimum 8 characters (12 for the bootstrap password); placeholder bootstrap passwords refused at start-up |
| Forced change | The bootstrap account and any account an admin resets must change the password at next login; until then every API call except `POST /api/v1/auth/password` returns `403 password change required` |
| Sessions | 256-bit random tokens stored as SHA-256 hashes; cookies HttpOnly, SameSite=Lax, Secure on HTTPS; a password change ends all other sessions and re-issues the cookie |
| Login throttling | Every 6 failures lock the client IP and the username for 1, then 5, 15 and 60 minutes; unknown usernames cost the same bcrypt work as real ones |
| CSRF | Cookie-authenticated state-changing requests (including login and logout) must come from the console's own origin; API tokens are exempt |
| Headers | Strict Content-Security-Policy (no inline scripts, `frame-ancestors 'none'`), `X-Frame-Options: DENY`, `nosniff`, `Referrer-Policy: no-referrer`, HSTS on HTTPS, `Cache-Control: no-store` on the API |
| Roles | Viewers read only; secrets in configuration are redacted for them ([§19.1](19-users-roles-api.md#191-console-users-and-roles)) |
| Logs | Query strings never logged; Configurator token redacted; webhook errors omit URLs; the bootstrap password never reaches the searchable log table |

## 20.3 Enrollment security

| Area | Controls |
| --- | --- |
| SCEP challenges | Fresh, single-use challenge per downloaded profile (2 h user, 1 h agent, 7 days admin); Configurator 24 h reusable; static challenge admin-only |
| Invitations | Each profile download and each agent enrollment uses one; expiry and disable supported; tokens hashed |
| Installer | Token passed via environment variable, not argv; server URL taken from settings, never from the Host header; refuses to run without a server URL |
| Takeover protection | A new agent enrollment can replace an existing computer's agent only with the current device token, after **Reset agent**, or after 30 days of silence; refusals return 409 and are audited as `agent.enroll_refused` |
| MDM identity | Device certificates verified against the SCEP CA; NanoMDM `certauth` binds each enrollment to its certificate; every request signed (`Mdm-Signature`) |

## 20.4 Agent security

| Area | Controls |
| --- | --- |
| Transport | TLS with normal certificate verification; `enroll` requires `https://` unless `--allow-insecure-http`; the server refuses an `http://` server URL unless loopback or `VIBEMDM_ALLOW_HTTP=1` |
| Local files | `agent.json` mode 0600; scripts run from a private 0700 directory; packages verified by SHA-256; the enrollment profile saved with an exclusive create in `/Users/Shared` (root never writes into a user's home) |
| Trigger sockets | `/var/run/vibemdm-agent.sock` (world-writable) accepts only `login`, `checkin`, `network_change`; custom triggers need the root-only `.sock.root` |
| Self Service portal | `127.0.0.1:8571`, Host-header check (anti DNS rebinding), custom header + JSON body for actions (no CORS preflight approval) |
| Installomator | Validated fields and single-quoted arguments; Team ID pinning ([§9.6](09-apps-installomator.md#96-why-arguments-are-single-quoted)) |
| super | Pinned version, SHA-256 verified before running |

## 20.5 Remote shell, outbound requests, MCP

- **Remote shell:** off by default; admin-only; same-origin WebSocket; one live session per Mac; idle timeout; 4-hour
  maximum; full transcript; turning the feature off ends all sessions; per-Mac veto file
  ([Chapter 17](17-remote-shell.md)).
- **Webhooks:** no redirects; loopback, link-local (including cloud metadata) and unspecified addresses refused; HMAC
  signatures. The SOFA fetch backs off for 5 minutes after a failure.
- **MCP:** API tokens only, Origin check, write tools opt-in and admin-only, no secrets, audited, rate-limited
  ([§19.6](19-users-roles-api.md#196-the-mcp-server-for-ai-agents)).

## 20.6 Known limitations

| Limitation | Mitigation |
| --- | --- |
| The at-rest key is derived with SHA-256 from the passphrase; ciphertexts are not bound to their row | Use a long random key |
| Webhook secrets, the static SCEP challenge and the enrollment secret are stored unencrypted | Protect database access and backups |
| Key rotation does not cover the super authentication password | Re-enter it after a rotation ([§2.6](02-installation.md#26-the-encryption-key)) |
| Any enrolled agent can download any package by ID | Never put credentials in packages |
| Task output is visible to viewers | Don't print secrets in scripts |
| The super password is passed to super as a command-line argument on the Mac | Use a dedicated account; the exposure lasts only for the install |
| Google client secrets and other values in profiles are readable by local admins on the Mac | Use `settingsOverrideScriptPath` or provider options that avoid secrets in profiles |
| User Approved (non-supervised) Macs: users can remove the MDM profile | Detect with a smart group (*MDM enrolled* false, *Agent installed* true) and follow up |
| Device identities expire after 3 years with no automatic renewal | Plan re-enrollment ([§3.2](03-apns-and-certificates.md#32-the-scep-ca-and-device-identities)) |

## 20.7 Study notes: macOS platform security

> **Study note (Gatekeeper and notarization):** Gatekeeper checks apps that carry the `com.apple.quarantine`
> attribute (set by browsers, Mail, AirDrop and most download tools) before their first launch: they must be signed
> with a Developer ID and **notarized** by Apple (macOS 10.15+), or come from the App Store. `spctl --status` shows
> whether assessments are enabled; `spctl -a -vv <app>` assesses one app. Control-clicking *Open*, or the Privacy &
> Security pane, lets a user override a block (the override flow changed in macOS 15). MDM can set allowed sources
> with `com.apple.systempolicy.control`.

> **Study note (System Integrity Protection):** SIP (macOS 10.11+) prevents even root from modifying protected system
> locations, loading unsigned kernel code and attaching to protected processes. It can only be changed from
> **recoveryOS** with `csrutil` (`csrutil status` reports it from macOS). On macOS 11+ the system volume is also a
> cryptographically **Signed System Volume (SSV)**, mounted read-only.

> **Study note (XProtect and friends):** **XProtect** is macOS's built-in signature-based anti-malware, checking
> files at launch and when they change. **XProtect Remediator** (macOS 12.3+) scans periodically and removes known
> malware. The legacy **MRT** (Malware Removal Tool) preceded it. Their definitions update in the background
> independently of macOS updates, when *Install Security Responses and system files* is on.

> **Study note (Secure Boot and startup security):** on Apple silicon, the boot chain is verified from the Boot ROM;
> **Startup Security Utility** in recoveryOS sets *Full Security*, *Reduced Security* (needed for some kernel
> extensions) or *Permissive Security*. On T2 Macs, Secure Boot has Full/Medium/No Security and an external-boot
> setting. `SecurityInfo` reports `SecureBoot` (VibeMDM shows `secure_boot_level`). Only a **volume owner** can lower
> startup security on Apple silicon.

> **Study note (TCC and PPPC):** see [§8.5](08-configuration-profiles.md#85-study-notes-common-payload-types). Privacy
> protections are enforced by TCC; MDM can pre-approve some services through PPPC on UAMDM or ADE enrollments, but
> Camera and Microphone always need the user, and Screen Recording / Input Monitoring can at most be delegated to
> standard users.

> **Study note (Activation Lock, Recovery Lock, firmware password):** Activation Lock (Apple silicon/T2) ties the Mac
> to an Apple Account; Recovery Lock (Apple silicon, set by MDM with `SetRecoveryLock`) protects recoveryOS; the
> firmware password is the Intel-era equivalent. Know which applies to which hardware.

Next: [Chapter 21 — CLI reference](21-cli-reference.md).
