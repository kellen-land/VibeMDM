# 21. CLI reference

VibeMDM ships four command-line programs. This reference is built from `cmd/*/main.go`. Man pages for the server
and the agent are in [`docs/man/`](../man/) (`man ./docs/man/vibemdm.8`, `man ./docs/man/vibemdm-agent.8`).

| Program | Runs on | Purpose |
| --- | --- | --- |
| [`vibemdm`](#211-vibemdm-server) | The server (Linux container) | The MDM server; key rotation |
| [`vibemdm-agent`](#212-vibemdm-agent) | Each Mac | The management agent |
| [`vibemdm-sim`](#213-vibemdm-sim) | Anywhere | Simulated Macs for testing and study |
| [`mscp-import`](#214-mscp-import) | A developer machine | Regenerates the compliance catalog |

Go's `flag` package is used throughout, so a flag can be written `-flag` or `--flag`, and `-flag=value` or
`-flag value`.

## 21.1 `vibemdm` (server)

```
vibemdm                          run the server (configured by VIBEMDM_* environment variables)
vibemdm version                  print the version
vibemdm rotate-encryption-key    re-encrypt stored secrets from one key to another
vibemdm help                     print usage
```

| Command | Aliases | Behaviour | Exit status |
| --- | --- | --- | --- |
| *(none)* | | Loads the configuration from the environment, connects to PostgreSQL, applies migrations, verifies and reseals secrets, bootstraps the admin user, then serves on `VIBEMDM_LISTEN` until SIGINT/SIGTERM (graceful shutdown, 15 s) | `2` invalid configuration (e.g. `VIBEMDM_DATABASE_URL` missing, bad `VIBEMDM_SESSION_TTL_HOURS` or `VIBEMDM_LOG_DB_LEVEL`); `1` start-up failure (database unreachable, wrong encryption key, placeholder admin password, …) or a server error; `0` clean shutdown |
| `version` | `-version`, `--version` | Prints the version stamped at build time (`dev` for local builds) | `0` |
| `rotate-encryption-key` | | Re-encrypts every stored secret from `VIBEMDM_ENCRYPTION_KEY_OLD` to `VIBEMDM_ENCRYPTION_KEY` in one transaction | `0` success (prints `Re-encrypted N stored secrets…`); `2` a key is invalid (shorter than 16 characters) or `VIBEMDM_DATABASE_URL` missing; `1` database error, or the old key does not open a secret (**nothing is changed**) |
| `help` | `-h`, `--help` | Prints usage, including the list of configuration variables | `0` |

Any other first argument is ignored and the server starts normally.

Environment variables are documented in [§2.3](02-installation.md#23-environment-variables).

Examples:

```bash
# Inside the compose deployment
docker compose exec vibemdm /app/vibemdm version

# Key rotation (server stopped)
docker compose stop vibemdm
docker compose run --rm -e VIBEMDM_ENCRYPTION_KEY_OLD="$OLD" -e VIBEMDM_ENCRYPTION_KEY="$NEW" vibemdm rotate-encryption-key

# Local development
VIBEMDM_DATABASE_URL=postgres://vibemdm:vibemdm@localhost:5432/vibemdm?sslmode=disable \
VIBEMDM_SERVER_URL=http://localhost:8080 VIBEMDM_ADMIN_PASSWORD='a-unique-long-password' \
VIBEMDM_DATA_DIR=./data VIBEMDM_AGENT_DIR=./bin/agents ./bin/vibemdm
```

## 21.2 `vibemdm-agent`

```
vibemdm-agent <command> [options]
vibemdm-agent help <command>        detailed help for one command
```

| Group | Command | Root? | What it does |
| --- | --- | --- | --- |
| Setup | `enroll` | yes | Enroll with a server and install the launchd services |
| | `manage` | yes | (Re)install the LaunchDaemon and login LaunchAgent |
| | `remove-framework` | yes | Unenroll and remove the agent |
| Everyday | `status` | no (more detail with sudo) | Show enrollment, connectivity, logging and local components |
| | `logs` | no to read (admin group); yes for `--set-level` | Show, filter or follow the agent log; change the log level |
| | `policy` | yes | Check in and run policies for a trigger |
| | `trigger <event>` | no for `login`/`checkin`/`network_change`; yes for custom | Ask the running daemon to fire a trigger |
| | `recon` | yes | Collect and submit inventory |
| | `checkin` | yes | One check-in cycle |
| | `check-connection` | in practice yes | Verify the server is reachable with this Mac's credentials |
| | `selfservice` | no | Open the Self Service portal |
| Internal | `daemon` | yes | The long-running agent (started by launchd) |
| | `version` | no | Print the version (`-v`, `--version`) |
| | `help` | no | Usage, or `help <command>` (`-h`, `--help`) |

### Exit status

| Situation | Exit |
| --- | --- |
| Success | `0` |
| A command returned an error (printed as `error: …` on stderr), including "this command must be run as root (use sudo)" | `1` |
| No command, or an unknown command (usage printed) | `2` |
| Invalid flag for `enroll` or `policy` (flag usage printed) | `2` |
| `help <unknown>` | prints `unknown command "…"` and the usage, exits `0` |

### `enroll`

```
vibemdm-agent enroll --server URL [--token TOKEN] [--mdm] [--allow-insecure-http]
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--server URL` | none (required) | Server URL, e.g. `https://mdm.example.com`. Must be `https://` unless `--allow-insecure-http` |
| `--token TOKEN` | `$VIBEMDM_ENROLL_TOKEN` | Enrollment token (invitation or enrollment secret). Prefer the environment variable: arguments are visible to other users in `ps` |
| `--mdm` | `false` | Also download the MDM enrollment profile to `/Users/Shared/VibeMDM-Enrollment-*.mobileconfig` and open it and the Device Management pane for the logged-in user (skipped if the Mac is already MDM-enrolled) |
| `--allow-insecure-http` | `false` | Accept a plain `http://` server (isolated labs only) |

Steps: collects system info → `POST /agent/v1/enroll` (sending the previous device token if `agent.json` exists, so a
reinstall is recognized) → writes `agent.json` (0600) → creates the Downloads and tmp directories (0700) → copies the
binary to `/usr/local/bin/vibemdm-agent` if run from elsewhere → installs and loads the launchd jobs → optionally
opens the MDM profile. Errors: missing server or token, non-HTTPS URL, not root, `enrollment failed: …` (403 invalid
or expired token, 409 an active agent already exists, see [takeover protection](04-enrollment.md#47-takeover-protection-and-reset-agent)).

```bash
sudo VIBEMDM_ENROLL_TOKEN=… vibemdm-agent enroll --server https://mdm.example.com --mdm
```

### `status`

Prints the agent version; whether it is enrolled (and since when), server URL, computer ID, organization and check-in
interval; whether the daemon is running (the local trigger socket exists); whether the server is reachable with the
device token; the log level, the log file and its size, and whether unified logging is built in; and whether
swiftDialog (`/usr/local/bin/dialog`), super (`/usr/local/bin/super`) and the remote-shell veto file are present,
plus the Self Service URL.

`agent.json` is readable only by root, so **without sudo** `status` reports `Enrolled: no (…permission denied)`. Run
`sudo vibemdm-agent status` for the real picture. "Daemon running" is inferred from the socket file and can be stale
after a crash; `sudo launchctl print system/com.vibemdm.agent` is authoritative.

### `logs`

```
vibemdm-agent logs [-n N] [-f] [--level LEVEL] [--component NAME] [--grep TEXT]
vibemdm-agent logs --unified [-f] [--last DURATION] [--component NAME]
vibemdm-agent logs --set-level LEVEL
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-n N` | `50` | Lines to show from the end of `/Library/Logs/VibeMDM/vibemdm-agent.log` (continuing into `.1` if needed) |
| `-f` | `false` | Keep printing new lines; follows the file across rotations |
| `--level LEVEL` | `debug` (all) | Minimum level: `debug`, `info`, `warn`, `error`. Lines without a level (crash output) are always shown |
| `--component NAME` | all | One component: `daemon`, `checkin`, `policy`, `task`, `package`, `apps`, `inventory`, `compliance`, `onboarding`, `shell`, `restricted`, `selfservice`, `spool` |
| `--grep TEXT` | none | Case-insensitive substring |
| `--unified` | `false` | Run `log show` (or `log stream` with `-f`) for subsystem `com.vibemdm.agent` instead of reading the file; prints the command it runs |
| `--last DURATION` | `1h` | With `--unified`: how far back to read |
| `--set-level LEVEL` | none | Save the level to `/Library/Application Support/VibeMDM/logging.json` and apply it to the running daemon at once. Root only |

The format, the components and the unified-logging build requirement are described in
[§18.2](18-logging-and-diagnostics.md#182-agent-logs).

### `policy`

```
vibemdm-agent policy [--event NAME]
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--event NAME` | `checkin` | Trigger to fire: `checkin`, `startup`, `login`, `network_change`, `enrollment_complete`, or any custom name |

Checks in **directly** (not through the daemon) with that trigger, runs every returned policy, task and app install,
and prints `Executed N policies for trigger "NAME"`.

```bash
sudo vibemdm-agent policy --event install_zoom
```

### `trigger <event>`

Sends the event to the running daemon over a Unix socket and returns at once (the daemon queues the check-in).

- As a normal user it uses `/var/run/vibemdm-agent.sock`, which accepts only `login`, `checkin` and `network_change`;
  other triggers fail with `error: this trigger requires root (run with sudo)`.
- As root it uses `/var/run/vibemdm-agent.sock.root`, which accepts any trigger. If the daemon is not running (or
  answers with an error), root runs the check-in directly instead.
- Errors: `usage: vibemdm-agent trigger <event>` without an argument; `agent daemon is not running` (non-root);
  `busy` when the daemon's queue is full.

The login LaunchAgent runs `vibemdm-agent trigger login` in each user session.

### `recon`

Collects full inventory (hardware, security, apps, profiles, local users, extension attributes, super status),
submits it and prints `Inventory submitted.`

### `checkin`

One `checkin` cycle, directly (not through the daemon). Prints nothing on success.

### `check-connection`

Reads `agent.json`, calls the server's version endpoint, and prints `Connected to <url> (server version <v>)`.

### `selfservice`

Opens `http://127.0.0.1:8571` in the default browser. Run as root, it opens the portal in the logged-in user's
session (`launchctl asuser`). Prints `Opened http://127.0.0.1:8571`.

### `manage`

Requires an existing `agent.json`. Rewrites and reloads `/Library/LaunchDaemons/com.vibemdm.agent.plist` and
`/Library/LaunchAgents/com.vibemdm.agent.login.plist` for the current binary. Use it after copying a new agent binary
into place.

### `remove-framework`

Calls `POST /agent/v1/unenroll` (best effort), unloads and deletes the launchd jobs, removes the trigger socket and
deletes `/Library/Application Support/VibeMDM`. Prints `VibeMDM agent removed. The MDM profile (if any) must be removed
separately.` The binary in `/usr/local/bin` is left in place.

### `daemon`

The long-running process started by launchd (`RunAtLoad`, `KeepAlive`, `ThrottleInterval` 15 s). It runs the startup
or check-in trigger, the long-poll loop, periodic check-ins with ±10 % jitter (after failures it retries sooner: 30 s,
doubling up to 8 min, never longer than the normal interval), log shipping, spool replay every 2 minutes, restricted-software monitoring, the Self Service portal and the
trigger sockets. You normally never run it by hand.

### Files

| Path | Purpose |
| --- | --- |
| `/usr/local/bin/vibemdm-agent` | Binary |
| `/Library/Application Support/VibeMDM/agent.json` | Server URL, device token, computer ID (0600) |
| `/Library/Application Support/VibeMDM/{Downloads,tmp,spool}` | Package cache, script temp, undelivered results and logs |
| `/Library/Application Support/VibeMDM/Installomator.sh` | Cached Installomator |
| `/Library/Application Support/VibeMDM/disable-remote-shell` | Optional remote-shell veto |
| `/Library/LaunchDaemons/com.vibemdm.agent.plist`, `/Library/LaunchAgents/com.vibemdm.agent.login.plist` | launchd jobs |
| `/Library/Logs/VibeMDM/vibemdm-agent.log` (`.1` to `.5`) | Agent log, rotated at 10 MB (`root:admin`, 0640) |
| `/Library/Logs/VibeMDM/vibemdm-agent.launchd.log` | The daemon's stdout/stderr (crashes only) |
| `/Library/Application Support/VibeMDM/logging.json` | Saved log level |
| `/var/run/vibemdm-agent.sock`, `/var/run/vibemdm-agent.sock.root` | Trigger sockets |

## 21.3 `vibemdm-sim`

The simulator fakes Macs that speak the **real** protocols: it performs SCEP enrollment against `/scep`, sends signed
`Authenticate` and `TokenUpdate` check-ins, polls `/mdm` for commands (instead of receiving APNs pushes), answers them
with realistic responses, performs DDM syncs, and acts as the agent. It is the main tool for testing and for studying
the protocol without a Mac. It is built into the Docker image (`/app/vibemdm-sim`) and by `make sim`.

```
vibemdm-sim [flags]
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-server` | `http://localhost:8080` | VibeMDM server URL |
| `-n` | `3` | Number of simulated Macs |
| `-token` | empty | Agent enrollment token (invitation or enrollment secret). Empty = no agent simulation |
| `-scep-challenge` | empty | SCEP challenge (Settings → SCEP CA). **Required** unless `-no-mdm` |
| `-topic` | `com.apple.mgmt.External.simulator` | APNs topic reported in `TokenUpdate` |
| `-poll` | `10s` | MDM command poll interval |
| `-checkin` | `30s` | Agent check-in interval |
| `-prefix` | `SIM` | Computer name prefix (`SIM-Mac-01`, …) |
| `-no-mdm` | `false` | Skip MDM enrollment (agent-only Macs) |

**Exit status:** `2` if `-scep-challenge` is missing without `-no-mdm`; otherwise it runs until SIGINT/SIGTERM and
exits `0`. A device whose MDM enrollment fails logs the error and, if `-token` is set, still enrolls as an agent.

**State:** device tokens are saved to `.vibemdm-sim-tokens.json` in the current directory (0600), keyed by server and
UDID, so restarting the simulator is a reinstall, not a takeover. Delete the file (and use **Reset agent**) to start
fresh. Device identities are deterministic per index (`SIM…` serials, `5A0000xx-SIM0-…` UDIDs).

**What the fake Macs do:**

| Area | Behaviour |
| --- | --- |
| Models / OS | Cycle through MacBook Air/Pro, Mac mini, Mac Studio; macOS 15.6.1, 15.5, 14.7.6, 26.0 |
| `DeviceInformation` | Realistic values; `IsSupervised` false, `UserApprovedEnrollment` true |
| `SecurityInfo` | FileVault on for even-numbered Macs; firewall off for every third; a FileVault PRK CMS blob when a key "exists" (every fourth Mac has none until Escrow Buddy remediation runs) |
| `InstallProfile` / `RemoveProfile` | Track installed profiles; return `MCProfileErrorDomain` errors for invalid profiles or unknown identifiers |
| `DeclarativeManagement` | Fetches tokens, the manifest and each declaration, then posts a status report; reports `downloading` and a pending version when below an enforcement target |
| Other commands | Acknowledged; `AvailableOSUpdates` returns macOS 26.0.1 |
| Policies and scripts | Simulated output; any script whose name contains **"fail"** fails |
| Apps | Installomator installs reported as completed |
| Compliance | Simulated results; drift appears on the second scan; some failures "remediated" when remediation is on |
| Onboarding | Fills the form (asset tag `A-1xxx`, first picker option, `sim-<key>` otherwise), applies the naming template, fails steps whose title contains "fail" |
| super | `install_super` records the version and submits inventory |
| Restricted software | The first Mac reports one violation per rule |
| Diagnostics | Uploads a small fake bundle |
| Remote shell | Accepts shell invitations |

Examples:

```bash
# 5 Macs with MDM and agent
bin/vibemdm-sim -server http://localhost:8080 -n 5 \
  -scep-challenge "<Settings → SCEP CA → challenge>" -token "<agent enrollment secret or invitation>"

# 2 agent-only Macs against a lab server, faster check-ins
bin/vibemdm-sim -server http://localhost:8081 -n 2 -no-mdm -token "<secret>" -checkin 10s -prefix LAB
```

Without a real push certificate, upload any certificate with a `UID` attribute as the "push certificate" to unlock
enrollment-profile generation locally; pushes fail, and the simulated devices poll instead.

## 21.4 `mscp-import`

Converts a macOS Security Compliance Project checkout into VibeMDM's embedded catalog. Developers only; the result is
compiled into the server.

```
go run ./cmd/mscp-import -src path/to/macos_security -release release_27.0 [-out internal/compliance/catalog.json.gz]
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-src` | none (required) | Path to a `macos_security` checkout. Uses `src/mscp/data` if present, otherwise the older top-level `rules/` and `baselines/` layout |
| `-release` | none (required) | Release tag recorded in the catalog, e.g. `release_27.0` |
| `-out` | `internal/compliance/catalog.json.gz` | Output file |

Exit status `2` (with flag usage) if `-src` or `-release` is missing. Files containing Apple's "Vendor Description"
copyright content are skipped (that content is not CC BY 4.0).

## 21.5 Make targets

| Target | Does |
| --- | --- |
| `make` / `make all` | `web`, `build`, `agent`, `sim` |
| `make build` (`server`) | `bin/vibemdm` (embeds whatever is in `internal/webui/dist`) |
| `make web` | Builds the React console into `internal/webui/dist` |
| `make agent` | `bin/agents/vibemdm-agent-darwin-arm64` and `-amd64`, cross-compiled without cgo (no unified logging) |
| `make agent-macos` | The same two binaries built on a Mac with cgo, adding unified logging (needs Xcode or the Command Line Tools) |
| `make sim` | `bin/vibemdm-sim` |
| `make test` / `make vet` | Go tests; `go vet` and the web type check |
| `make docker` | Builds the image `vibemdm/vibemdm:<version>` and `:latest` |
| `make up` / `make down` / `make logs` | Compose up (with build), down, follow the server log |
| `make dev-db` | A local PostgreSQL 17 container (user/password `vibemdm`, port 5432) |
| `make clean` | Removes `bin/` and the built console |

Next: [Chapter 22 — Troubleshooting](22-troubleshooting.md).
