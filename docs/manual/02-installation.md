# 2. Installation

This chapter deploys VibeMDM with Docker Compose. It covers TLS, every environment variable, the first login,
the at-rest encryption key and its rotation, backups and upgrades.

## 2.1 Requirements

| Requirement | Why |
| --- | --- |
| A Linux host with Docker and Docker Compose v2 | VibeMDM ships as a container image built from `deploy/Dockerfile` |
| A public DNS name (for example `mdm.example.com`) | Macs must reach the server from any network they use |
| Ports 80 and 443 reachable from the internet | Let's Encrypt HTTP-01 validation (port 80) and HTTPS (443) for the Caddy `tls` profile |
| A TLS certificate that macOS trusts | Apple devices refuse MDM servers with self-signed or untrusted certificates |
| Outbound HTTPS from the server to `api.push.apple.com` | Sending APNs pushes (HTTP/2 on 443) |
| Optional outbound HTTPS to `mdmcert.download`, `sofafeed.macadmins.io`, `raw.githubusercontent.com` | Push certificate signing, the macOS version catalog, the super script download (the last one is fetched by Macs, not the server) |
| An Apple Account for the Apple Push Certificates Portal | Creating and renewing the MDM push certificate ([Chapter 3](03-apns-and-certificates.md)) |

> **Study note:** Apple publishes the hosts and ports its devices need in the support article *Use Apple products
> on enterprise networks*. For MDM, the key items are: devices hold a persistent connection to APNs on TCP 5223
> (falling back to 443 on Wi-Fi), APNs uses Apple's 17.0.0.0/8 address block, and the MDM server sends pushes over
> HTTP/2 to `api.push.apple.com` on port 443 (2197 is an alternative). TLS-inspecting proxies that intercept
> Apple traffic commonly break APNs and enrollment.

## 2.2 Deploy with Docker Compose and Caddy

The compose file (`deploy/docker-compose.yml`) defines three services:

| Service | Image | Notes |
| --- | --- | --- |
| `db` | `postgres:17-alpine` | Database `vibemdm`, user `vibemdm`, volume `db-data`, health-checked with `pg_isready` |
| `vibemdm` | `${VIBEMDM_IMAGE:-vibemdm/vibemdm:latest}`, built from `deploy/Dockerfile` | Waits for a healthy `db`; volume `vibemdm-data` at `/data`; publishes `${VIBEMDM_BIND:-127.0.0.1}:${VIBEMDM_PORT:-8080}:8080` |
| `caddy` | `caddy:2-alpine` | Profile `tls` only; ports 80 and 443; mounts `deploy/Caddyfile`; volumes `caddy-data`, `caddy-config` |

### Procedure

1. Clone the repository and change to the deploy directory:

   ```bash
   git clone https://github.com/YOUR-ORG/vibemdm.git
   cd vibemdm/deploy
   cp .env.example .env
   ```

2. Generate secrets:

   ```bash
   openssl rand -base64 48   # VIBEMDM_ENCRYPTION_KEY
   openssl rand -base64 24   # POSTGRES_PASSWORD
   ```

3. Edit `.env`. At minimum set `VIBEMDM_SERVER_URL`, `VIBEMDM_ENCRYPTION_KEY`, `POSTGRES_PASSWORD`, `DOMAIN`
   and `CADDY_EMAIL`. Leave `VIBEMDM_ADMIN_PASSWORD` empty to have one generated, or set a unique value of at
   least 12 characters.
4. Start the stack with TLS:

   ```bash
   docker compose --profile tls up -d --build
   ```

5. Watch the first start:

   ```bash
   docker compose logs -f vibemdm
   ```

   You should see `applied database migrations`, then `VibeMDM listening`. If the admin password was generated,
   it is printed once to the container console (stderr) and saved to `/data/initial-admin-password`.
6. Open `https://mdm.example.com` and sign in ([§2.5](#25-first-login-and-the-forced-password-change)).

### What the Caddyfile does

```caddyfile
{$DOMAIN} {
    encode zstd gzip
    header Strict-Transport-Security "max-age=31536000; includeSubDomains"
    request_body { max_size 8GB }          # package uploads
    reverse_proxy vibemdm:8080 {
        transport http { read_timeout 10m  write_timeout 10m }   # 55 s agent long-polls, uploads
    }
}
```

Caddy gets and renews a Let's Encrypt certificate for `DOMAIN` automatically. The `email` global option uses
`CADDY_EMAIL` for ACME account notices.

### Using your own reverse proxy instead

If you already terminate TLS (nginx, Traefik, HAProxy, a Cloudflare Tunnel), skip the `tls` profile:

1. `docker compose up -d --build` (without `--profile tls`).
2. Set `VIBEMDM_BIND=0.0.0.0` only if the proxy runs on another host. Otherwise keep `127.0.0.1`.
3. Configure the proxy to:
   - forward to port 8080 over HTTP;
   - allow request bodies up to your largest package;
   - allow read/write timeouts of at least 60 s (agents long-poll for 55 s) and support WebSocket upgrades for
     `/api/v1/computers/{id}/shell` and `/agent/v1/shell` (remote shell);
   - set `X-Forwarded-For`, and add the proxy's address or CIDR to `VIBEMDM_TRUSTED_PROXIES`.
4. Use a publicly trusted certificate.

TLS can terminate at the proxy because the MDM channel authenticates devices with the `Mdm-Signature` header,
not with TLS client certificates, so the device identity survives a TLS-terminating proxy.

> **Caution:** a Cloudflare-style proxy that inspects or rewrites traffic can break large uploads, long-polls or
> WebSockets. Test enrollment, a remote shell and a package download after any proxy change.

## 2.3 Environment variables

The server reads `VIBEMDM_*` variables at start-up (`internal/config/config.go`). Compose reads `.env`
and passes a **subset** to the container. A variable that is not listed under `services.vibemdm.environment`
in `docker-compose.yml` does not reach the server; add it there if you need it.

### Server variables

| Variable | Default | Passed by compose? | Description |
| --- | --- | --- | --- |
| `VIBEMDM_DATABASE_URL` | none (required) | yes (built from `POSTGRES_PASSWORD`) | PostgreSQL DSN, e.g. `postgres://vibemdm:secret@db:5432/vibemdm?sslmode=disable`. The server exits with code 2 if it is missing |
| `VIBEMDM_SERVER_URL` | empty | yes (required by compose) | Public base URL, e.g. `https://mdm.example.com`. Trailing `/` is trimmed. Copied into Settings → General on first boot only; after that the stored setting wins |
| `VIBEMDM_LISTEN` | `:8080` | set in the image | Listen address inside the container |
| `VIBEMDM_DATA_DIR` | `/data` | set in the image | Packages, log archives, `initial-admin-password`, temp files |
| `VIBEMDM_AGENT_DIR` | `/app/agents` | set in the image | Prebuilt agent binaries `vibemdm-agent-darwin-arm64` and `-amd64` |
| `VIBEMDM_ADMIN_PASSWORD` | empty | yes | Password for the bootstrap `admin` user, used **only when no users exist**. Empty generates one. Must be 12+ characters and not a placeholder (`change-me-please`, `changeme`, `admin`, `password`, `vibemdm`, `adminadmin`, …) or start-up fails |
| `VIBEMDM_ENCRYPTION_KEY` | empty | yes | Passphrase (16+ characters) for AES-256-GCM encryption of private keys and FileVault recovery keys at rest. See [§2.6](#26-the-encryption-key) |
| `VIBEMDM_TRUSTED_PROXIES` | empty (`.env.example`: `172.16.0.0/12`) | yes (default `172.16.0.0/12`) | Comma-separated IPs/CIDRs allowed to set `X-Forwarded-For`. Without it the client IP is the direct peer |
| `VIBEMDM_ALLOW_HTTP` | `false` | yes | Accept an `http://` server URL for a non-loopback host. Labs only: agents would run commands received without TLS |
| `VIBEMDM_SESSION_TTL_HOURS` | `168` | **no** | Console session lifetime in hours (minimum 1) |
| `VIBEMDM_LOG_DB_LEVEL` | `info` | **no** | Minimum level stored in the searchable `server_logs` table: `debug`, `info`, `warn`, `error` |
| `VIBEMDM_LOG_FORMAT` | `text` | yes | `json` switches stdout logging to JSON |
| `VIBEMDM_DEBUG` | `false` | yes | Debug-level stdout logging; also logs MDM signature-verification errors |
| `VIBEMDM_DUMP_MDM` | `false` | **no** | Dumps every MDM request and response to stdout (very verbose; shows protocol traffic for study) |
| `VIBEMDM_APNS_URL` | empty | **no** | Override the APNs server URL (testing only) |

Booleans accept `1`, `true`, `yes`, `on` and `0`, `false`, `no`, `off`.

### Variables used only by `rotate-encryption-key`

| Variable | Description |
| --- | --- |
| `VIBEMDM_ENCRYPTION_KEY_OLD` | The current key (empty if secrets are stored unencrypted) |
| `VIBEMDM_ENCRYPTION_KEY` | The new key (empty decrypts everything back to plaintext) |
| `VIBEMDM_DATABASE_URL` | The database to update |

### Compose-only variables (`.env`)

| Variable | Default | Description |
| --- | --- | --- |
| `POSTGRES_PASSWORD` | none (required) | Password for the `vibemdm` database user. Compose builds `VIBEMDM_DATABASE_URL` from it |
| `VIBEMDM_BIND` | `127.0.0.1` | Host address the plain-HTTP port is published on. Keep `127.0.0.1` behind Caddy |
| `VIBEMDM_PORT` | `8080` | Host port |
| `VIBEMDM_IMAGE` | `vibemdm/vibemdm:latest` | Image name/tag |
| `VIBEMDM_VERSION` | `dev` | Build argument stamped into the binaries (`vibemdm version`) |
| `DOMAIN` | none (required for `tls`) | Host name Caddy serves and certifies |
| `CADDY_EMAIL` | empty | ACME account e-mail |

### Agent-side variable

| Variable | Where | Description |
| --- | --- | --- |
| `VIBEMDM_ENROLL_TOKEN` | Mac, `vibemdm-agent enroll` | Enrollment token. The installer uses it so the token never appears in `ps` output |

The agent sets `VIBEMDM_SERVER_URL`, `VIBEMDM_COMPUTER_ID`, `VIBEMDM_SCRIPT_NAME`, `VIBEMDM_POLICY_NAME` and
`VIBEMDM_POLICY_ID` for scripts it runs, and `VIBEMDM_SHELL=1` in remote shells. These are outputs, not settings.

## 2.4 Server settings stored in the database

Most behaviour is configured in the console, not the environment. These are stored in the `settings` table and
edited under **Settings → General** (or `PUT /api/v1/settings`, admin only):

| Setting (`json`) | Default | Notes |
| --- | --- | --- |
| `org_name` | `VibeMDM` | Shown in profiles, dialogs and notifications |
| `server_url` | from `VIBEMDM_SERVER_URL` | Must be `https://` unless loopback or `VIBEMDM_ALLOW_HTTP=1` |
| `agent_checkin_interval_seconds` | `300` | 30–86400 |
| `inventory_interval_hours` | `24` | How often agents submit full inventory |
| `enrollment_profile_name` / `enrollment_profile_identifier` | `VibeMDM Enrollment` / `com.vibemdm.enrollment` | Changing the identifier after Macs enrolled breaks *Remove MDM Profile*, which removes by identifier |
| `open_enrollment` | `false` | Lets anyone with the URL download the enrollment profile from `/enroll` without a token |
| `signing_cert_pem` / `signing_key_pem` | empty | Optional identity that signs the enrollment profile ([Chapter 3](03-apns-and-certificates.md)) |
| `remote_shell_enabled` / `remote_shell_idle_timeout_secs` | `false` / `600` | [Chapter 17](17-remote-shell.md) |
| `log_retention_days` / `log_archive_enabled` | `30` / `false` | [Chapter 18](18-logging-and-diagnostics.md) |
| `mcp_enabled` / `mcp_allow_writes` | `false` / `false` | [Chapter 19](19-users-roles-api.md) |
| `filevault_auto_remediate` | `false` | Set through `PUT /api/v1/filevault` ([Chapter 11](11-filevault.md)) |

On first boot the server also generates the **agent enrollment secret** and the **static SCEP challenge**
(24-byte random tokens) if they are empty.

> The `auto_install_agent` field exists in the settings model but no code reads it. It has no effect.

## 2.5 First login and the forced password change

1. Browse to `https://mdm.example.com`.
2. Sign in as `admin` with the password from `.env` or `/data/initial-admin-password`:

   ```bash
   docker compose exec vibemdm cat /data/initial-admin-password
   ```

3. You are sent to a **forced password change** screen. Until you change it, every API call except
   `POST /api/v1/auth/password` returns `403 password change required`. The new password must be at least 8
   characters and different from the current one.
4. Delete the bootstrap file:

   ```bash
   docker compose exec vibemdm rm /data/initial-admin-password
   ```

5. Complete the initial configuration:
   1. **Settings → General**: check the server URL and set the organization name.
   2. **Settings → APNs**: obtain the push certificate ([Chapter 3](03-apns-and-certificates.md)). Macs cannot
      enroll without it.
   3. **Health**: review the findings. A new server typically shows *APNs push certificate missing*.
   4. **Enrollment**: create an invitation ([Chapter 4](04-enrollment.md)).

Changing a password ends every other session for that user and re-issues the current cookie. When an admin resets
another user's password (`POST /api/v1/users/{id}/password`), that user must change it at next login.

## 2.6 The encryption key

With `VIBEMDM_ENCRYPTION_KEY` set, these values are sealed with AES-256-GCM before they reach PostgreSQL
(the stored form starts with `enc:v1:`):

- the SCEP CA private key,
- the APNs push certificate private key and any pending APNs request key,
- the FileVault escrow identity's private key,
- the profile-signing private key,
- every escrowed FileVault recovery key,
- the super authentication password.

Behaviour at start-up:

| Situation | What happens |
| --- | --- |
| Key set, plaintext secrets exist (e.g. you just added a key) | They are sealed automatically; the log says `encrypted previously plaintext secrets at rest` |
| Key not set | A warning is logged and the Health check *Encryption at rest* opens |
| Key wrong or missing while ciphertext exists | **The server refuses to start.** It never regenerates a CA or escrow identity it cannot read |
| Key shorter than 16 characters | Start-up fails |

The key is turned into an AES key with SHA-256, so use a long random value (48 random bytes, base64-encoded, is a
good choice). Some values are **not** encrypted: webhook signing secrets, the static SCEP challenge and the agent
enrollment secret.

> **Back up the key separately from the database.** Without it, the CA, APNs key and every escrowed FileVault
> key are unrecoverable. A database backup without the key is not a working backup.

### Rotating the key

`rotate-encryption-key` re-encrypts every stored secret in **one transaction**. If the old key is wrong, it changes
nothing and exits with code 1.

```bash
cd vibemdm/deploy
docker compose stop vibemdm
docker compose run --rm \
  -e VIBEMDM_ENCRYPTION_KEY_OLD='current-key' \
  -e VIBEMDM_ENCRYPTION_KEY='new-key' \
  vibemdm rotate-encryption-key
# → "Re-encrypted N stored secrets. Start the server with the new VIBEMDM_ENCRYPTION_KEY."
# edit .env: VIBEMDM_ENCRYPTION_KEY=new-key
docker compose up -d
```

To enable encryption on a server that had none, you can just set the key and restart; existing plaintext is sealed
at start-up. To remove encryption, run the rotation with an empty `VIBEMDM_ENCRYPTION_KEY`.

> **Known gap:** the rotation (and the start-up resealing and key check) covers the SCEP CA, pending APNs request,
> escrow identity, push certificates, FileVault keys and the profile-signing key. It does **not** cover the super
> authentication password in `super_config`. After a rotation, re-enter the super account password under
> Software Updates → super (or `PUT /api/v1/updates/super` with `auth_password`), otherwise reading the super
> configuration fails because the old ciphertext cannot be opened with the new key.

## 2.7 Backups

A complete backup has three parts:

| Part | Contains | How |
| --- | --- | --- |
| PostgreSQL | All configuration, inventory, the CA, APNs certificate, escrowed keys (sealed), logs | `pg_dump` |
| `/data` volume | Uploaded packages, log archives | Volume tar |
| `VIBEMDM_ENCRYPTION_KEY` (and `.env`) | The key to every sealed secret | Store in a password manager or vault, **not** next to the dumps |

The Caddy volumes hold only certificates that Caddy can re-obtain; backing them up avoids Let's Encrypt rate limits.

### Procedure

```bash
cd vibemdm/deploy
# 1. Database (custom format, compressed)
docker compose exec -T db pg_dump -U vibemdm -d vibemdm -Fc > vibemdm-$(date +%F).dump

# 2. Data volume (the volume name is <project>_vibemdm-data; check with `docker volume ls`)
docker run --rm -v deploy_vibemdm-data:/data:ro -v "$PWD":/backup alpine \
  tar czf /backup/vibemdm-data-$(date +%F).tgz -C /data .
```

### Restore

```bash
docker compose --profile tls up -d db
docker compose exec -T db pg_restore -U vibemdm -d vibemdm --clean --if-exists < vibemdm-2026-10-01.dump
docker run --rm -v deploy_vibemdm-data:/data -v "$PWD":/backup alpine tar xzf /backup/vibemdm-data-2026-10-01.tgz -C /data
# make sure .env has the same VIBEMDM_ENCRYPTION_KEY as when the dump was taken
docker compose --profile tls up -d
```

After a restore, Macs reconnect by themselves: their identities are still signed by the restored CA, and the
push certificate and topic are unchanged. Commands queued after the backup was taken are lost.

> **Study note:** what really matters for continuity is the **APNs topic** and the **SCEP CA**. If you lose the
> push certificate's private key or let it expire beyond renewal, or if you regenerate the CA, every device must
> re-enroll. That is why Apple insists that you renew the push certificate with the same Apple Account, and why
> MDM vendors guard their CA.

## 2.8 Upgrades and migrations

Migrations are embedded in the binary (`internal/db/migrations/0001_…` to `0016_…`) and applied automatically at
start-up. The server records them in `schema_migrations` and holds a PostgreSQL advisory lock while migrating, so
two replicas cannot migrate at the same time. Migrations are forward-only; there are no down-migrations.

### Procedure

1. Read the release notes and back up ([§2.7](#27-backups)).
2. Pull and rebuild:

   ```bash
   cd vibemdm && git pull
   cd deploy && docker compose --profile tls up -d --build
   ```

3. Check the log for `applied database migrations` and `VibeMDM listening`, then open **Health**.
4. The new image contains new agent binaries. Agents do not update themselves; to roll out a new agent, re-run
   the install command on each Mac (a reinstall on the same Mac is allowed; see
   [takeover protection](04-enrollment.md#47-takeover-protection-and-reset-agent)), or deploy it with a policy.

To roll back, restore the pre-upgrade database dump and run the previous image. Starting an old binary against a
newer schema is not supported.

## 2.9 Liveness and version endpoints

| Endpoint | Auth | Returns |
| --- | --- | --- |
| `GET /healthz` | none | `ok`, or `503 database unavailable` if PostgreSQL does not answer. The image's Docker `HEALTHCHECK` uses it |
| `GET /version` | none | `{"version":"…"}` |
| `GET /api/v1/status` | none | `{"org_name", "version", "time"}` |
| `GET /agent/v1/version` | none | Server version, used by `vibemdm-agent check-connection` |

## 2.10 Running without Docker (development)

```bash
make dev-db
export VIBEMDM_DATABASE_URL='postgres://vibemdm:vibemdm@localhost:5432/vibemdm?sslmode=disable'
export VIBEMDM_SERVER_URL=http://localhost:8080 VIBEMDM_ADMIN_PASSWORD='a-unique-long-password' \
       VIBEMDM_DATA_DIR=./data VIBEMDM_AGENT_DIR=./bin/agents
make agent && go run ./cmd/vibemdm          # API and console on :8080
cd web && npm install && npm run dev        # hot-reloading console on :5173
```

`http://localhost` is allowed because the host is loopback. Real Macs will not enroll against plain HTTP; use the
simulator ([Chapter 21](21-cli-reference.md#213-vibemdm-sim)) for local testing.

Next: [Chapter 3 — APNs and certificates](03-apns-and-certificates.md).
