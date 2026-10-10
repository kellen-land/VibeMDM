# VibeMDM

**VibeMDM** is a self-hosted Apple MDM for Macs, shipped as a Docker container. It pairs the
[NanoMDM](https://github.com/micromdm/nanomdm) protocol engine with everything a small fleet admin
expects from a commercial product: a web console, inventory, smart groups, policies, a script library,
configuration profiles, a package distribution point, and a lightweight macOS agent (`vibemdm-agent`)
that runs scripts and policies on the Mac.

```
┌──────────────┐  APNs push   ┌────────────────────────────────────────────┐
│  Apple APNs  │◀────────────▶│  vibemdm (Go)                               │
└──────┬───────┘              │  ├─ /mdm       NanoMDM check-in + commands  │
       │ wake                 │  ├─ /scep      built-in SCEP CA             │
┌──────▼───────┐  MDM (TLS)   │  ├─ /enroll    user-initiated enrollment    │
│  Mac         │─────────────▶│  ├─ /agent/v1  agent API (long-poll)        │
│  ├ MDM client│              │  ├─ /api/v1    admin REST API               │
│  └ agent     │─────────────▶│  └─ /          React web console            │
└──────────────┘  agent API   └────────────────────┬───────────────────────┘
                                                   │
                                            ┌──────▼──────┐
                                            │ PostgreSQL  │
                                            └─────────────┘
```

## Features

| Area | What you get |
| --- | --- |
| **Enrollment** | SCEP-issued device identities, enrollment profile generator (optionally signed), user-initiated enrollment page with invitation links, one-line `curl \| sudo bash` agent installer |
| **Inventory** | Hardware, OS, security posture (FileVault, SIP, firewall, Gatekeeper, Secure Boot), installed apps, profiles, local users, extension attributes — from both the MDM channel and the agent |
| **Computer groups** | Static groups and **smart groups** with a Jamf-style criteria builder (and/or, parentheses, 45+ fields incl. app title/version, group membership, extension attributes) |
| **Policies** | Triggers (startup, login, check-in, enrollment complete, custom events), execution frequency, scope with exclusions, script payloads with `$4`–`$11` parameters, packages, "execute command", inventory update, restart, user messages, per-computer logs with step output, flush logs, run now |
| **Scripts** | Script library with parameter labels, priority, run-on-demand against computers or groups with live results |
| **Configuration profiles** | Upload `.mobileconfig`, scope it, VibeMDM installs/removes via `InstallProfile`/`RemoveProfile` and tracks per-device status |
| **Packages** | Upload `.pkg` / `.dmg` / `.zip` to the built-in distribution point; the agent verifies SHA-256 and installs |
| **Management commands** | Update inventory, lock (PIN), restart, shut down, erase, remote desktop on/off, OS updates, remove MDM profile, raw command plist |
| **Agent** | Single static Go binary; LaunchDaemon with long-poll tasks (near-instant script execution), login trigger, recon, `<result>` extension attributes |
| **Remote shell** | Live interactive root shell to an online Mac from the browser (xterm.js), relayed through the agent — off by default, admin-only, same-origin, one session per Mac, idle timeout, full transcript audit |
| **Console** | React + Tailwind UI (dark-first, teal), dashboard, activity log, audit log, users/roles (admin/viewer), API tokens |

Jamf Pro's workflow was the inspiration (policies, smart groups, scripts, extension attributes, Self Service
terminology). VibeMDM is an independent implementation and contains no Jamf code or assets.

## Quick start (Docker)

Requirements: a public DNS name pointing at the host, ports 80/443 reachable (for Let's Encrypt), Docker.

```bash
git clone https://github.com/YOUR-ORG/vibemdm.git && cd vibemdm/deploy
cp .env.example .env            # set VIBEMDM_SERVER_URL, VIBEMDM_ADMIN_PASSWORD, POSTGRES_PASSWORD, DOMAIN, CADDY_EMAIL
docker compose --profile tls up -d --build
```

Open `https://mdm.example.com`, sign in as `admin`, then:

1. **Settings → APNs** – obtain an Apple push certificate (see [docs/apns.md](docs/apns.md)). Without it Macs cannot enroll.
2. **Enrollment** – create an invitation link, or copy the install command.
3. On a Mac: open the enrollment link, install the profile, approve it in *System Settings → Device Management*, and run the agent install command in Terminal.

Already have TLS termination (nginx, Traefik, Cloudflare Tunnel)? Skip the `tls` profile and proxy to port 8080. Apple requires a
certificate that macOS trusts; self-signed certificates do not work for MDM.

## Repository layout

```
cmd/vibemdm         server binary        internal/mdmsvc    NanoMDM wrapper: inventory + command results
cmd/vibemdm-agent   macOS agent          internal/scepsrv   SCEP server / CA
cmd/vibemdm-sim     fake-Mac simulator   internal/policy    policy evaluation for the agent
cmd/mscp-import     mSCP catalog import  internal/agentapi  agent API + long-poll notifier
internal/api        admin REST API       internal/enroll    enrollment profile generator
internal/store      SQL (PostgreSQL)     internal/health    misconfiguration detection + self-healing
internal/db         migrations           internal/compliance mSCP baselines, scans, enforcement profiles
internal/ddm        declarative mgmt     internal/schema    JSON-schema (Application & Custom Settings) profiles
internal/superwf    super deployment     internal/onboarding white-glove onboarding (swiftDialog)
internal/mcp        MCP server for AI    web/               React console (Vite, Tailwind)
deploy/             Dockerfile, compose, Caddyfile           docs/manual/  administrator manual
```

## Development

```bash
make dev-db                      # local PostgreSQL in Docker (user/pass vibemdm)
export VIBEMDM_DATABASE_URL='postgres://vibemdm:vibemdm@localhost:5432/vibemdm?sslmode=disable'
export VIBEMDM_SERVER_URL=http://localhost:8080 VIBEMDM_ADMIN_PASSWORD=adminadmin1 VIBEMDM_DATA_DIR=./data VIBEMDM_AGENT_DIR=./bin/agents
make agent && go run ./cmd/vibemdm          # API on :8080
cd web && npm install && npm run dev        # console on :5173 (proxies to :8080)
```

No Mac handy? Point the simulator at the server — it performs real SCEP + MDM enrollment and acts as the agent:

```bash
go run ./cmd/vibemdm-sim -server http://localhost:8080 -n 5 \
    -scep-challenge "$(Settings → SCEP CA → challenge)" -token "$(Settings → General → agent enrollment secret)"
```

Upload any certificate that has a `UID` attribute as the "push certificate" to unlock enrollment-profile generation
locally (pushes will fail, the simulated devices poll instead). Add `-no-mdm` for agent-only Macs. The simulator also
fakes compliance scans (with drift on the second scan), super installs and onboarding runs, so every console page
has data to show.

## Documentation

- [docs/apns.md](docs/apns.md) – getting an Apple MDM push certificate
- [docs/agent.md](docs/agent.md) – agent architecture, triggers, script parameters, uninstall
- [docs/architecture.md](docs/architecture.md) – how the pieces fit together, data model, security notes
- [docs/api.md](docs/api.md) – REST API overview and tokens
- [docs/enrollment.md](docs/enrollment.md) – enrollment options without Apple Business Manager (Mac, iPhone, iPad)
- [docs/man/](docs/man/) – man pages for `vibemdm` and `vibemdm-agent` (`man ./docs/man/vibemdm-agent.8`)
- [docs/mcp.md](docs/mcp.md) – MCP server for AI agents (Claude Code, Claude Desktop, Claude API)
- [docs/security.md](docs/security.md) – security model, production hardening checklist, known limitations
- [docs/health.md](docs/health.md) – misconfiguration detection, remediations and self-healing
- [docs/inspector.md](docs/inspector.md) – MDM command inspector: timelines, summaries, redaction
- [docs/software-updates.md](docs/software-updates.md) – native DDM, update enforcement and settings plans
- [docs/logging.md](docs/logging.md) – searchable server/agent logs, live tail, archives, diagnostic bundles
- [docs/installomator.md](docs/installomator.md) – Mac Apps catalog, Installomator policy steps, custom labels
- [docs/filevault.md](docs/filevault.md) – FileVault recovery key escrow and Escrow Buddy remediation
- [docs/remote-shell.md](docs/remote-shell.md) – the interactive remote root shell and how it is secured

## License

MIT. Includes NanoMDM (MIT, © micromdm) as a library and its PostgreSQL schema verbatim.
