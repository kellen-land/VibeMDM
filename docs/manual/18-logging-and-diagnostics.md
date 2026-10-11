# 18. Logging and diagnostics

VibeMDM keeps two searchable log streams in PostgreSQL (server and agent), writes server logs to stdout for
`docker compose logs`, and can collect on-demand diagnostic bundles from individual Macs.

## 18.1 Server logs

Every server log line goes to **stdout** (text, or JSON with `VIBEMDM_LOG_FORMAT=json`) **and**, at or above
`VIBEMDM_LOG_DB_LEVEL` (default `info`), into the `server_logs` table. Database writes are batched every second and
never block request handling. If the database can't keep up, records are dropped and counted, and the count is
logged hourly (`log records dropped because the database could not keep up`).

| Component | What it logs |
| --- | --- |
| `mdm` | VibeMDM's MDM hooks: authenticate, token update, check-out, command results |
| `nanomdm` | The protocol engine (check-in and command handlers, certificate verification, storage, push) |
| `agent` | Agent API: enrollments, refused takeovers, check-ins (debug) |
| `api` | Admin API errors |
| `scep` | Certificate issuance |
| `policy` | Policy evaluation |
| `commander` | Command queueing and push results |
| `ddm` | Declarative management |
| `compliance`, `super`, `onboarding`, `health`, `mcp`, `webhook` | Their features |

HTTP access lines (`msg=http`) carry `method`, `path`, `status`, `dur` and `remote`. Successful `/api/…`,
`/assets/…` and agent long-poll requests are logged at debug level; everything else, and every 4xx, at info; 5xx at
warn. **Query strings are never logged**, and the Apple Configurator URL is logged as
`/enroll/configurator/[redacted]`.

Useful stdout commands:

```bash
docker compose logs -f vibemdm
docker compose logs vibemdm | grep 'component=nanomdm'
```

Set `VIBEMDM_DEBUG=true` for debug output (including MDM signature-verification errors) and
`VIBEMDM_DUMP_MDM=true` to print every MDM request and response (very verbose; useful for studying the protocol).
Both need adding to `docker-compose.yml` if your compose file does not pass them ([§2.3](02-installation.md#23-environment-variables)).

## 18.2 Agent logs

The agent writes every record to three places:

| Where | What | Read it with |
| --- | --- | --- |
| `/Library/Logs/VibeMDM/vibemdm-agent.log` | One line per record. Rotated at 10 MB, with five old copies kept (`.1` to `.5`, 60 MB at most). Owner `root:admin`, mode 0640 | `vibemdm-agent logs`, Console.app → **Log Reports** → *VibeMDM*, `tail -f` |
| macOS unified logging | Subsystem `com.vibemdm.agent`, one **category** per component (below) | `vibemdm-agent logs --unified`, Console.app's live stream, `log show` / `log stream` |
| The server | Records at `info` and above, every 30 seconds (`POST /agent/v1/logs`) | **Logs** → agent, each computer's **Logs** tab ([§18.3](#183-searching-and-live-tail)) |

While the server is unreachable, up to 5,000 records stay in memory. On shutdown they are spooled to
`/Library/Application Support/VibeMDM/spool/logs.ndjson` (capped at 5 MB) and sent after the next start.

The daemon's own stdout and stderr (launchd) go to `/Library/Logs/VibeMDM/vibemdm-agent.launchd.log`. It stays
empty unless the agent crashes, so a Go panic trace lands there. Agents before this release logged to
`/var/log/vibemdm-agent.log`. That file is left in place, and diagnostic bundles still collect it.

### Reading the log file

```
2026-10-11T13:54:11.043Z INFO  [policy] policy started policy="Onboard tools" policy_id=481f… trigger=manual log_id=a360… scripts=1 packages=0 apps=0 command=true inventory=true
2026-10-11T13:54:11.056Z INFO  [policy] policy step finished policy="Onboard tools" step=script name="Set up dock" status=completed exit=0 dur=4ms output=done
2026-10-11T13:54:11.062Z WARN  [policy] policy step failed policy="Onboard tools" step=command name="Execute Command" status=failed exit=2 dur=5ms output="cmd ok | ls: cannot access '/nonexistent-dir': No such file or directory"
2026-10-11T13:54:11.081Z WARN  [policy] policy finished policy="Onboard tools" status=failed steps=3 dur=38ms
```

Each line has a timestamp, a level, a `[component]` and a message, followed by `key=value` attributes. Values with
spaces are quoted, and newlines are escaped, so every record stays on one line and `grep` works. `output=` holds
the last line of a step's output (the last three lines when it failed). With the log level at `debug`, a
`policy step output` record also carries up to the last 4 KB.

| Component | Records |
| --- | --- |
| `daemon` | Start-up (version, server, computer ID, log level, whether unified logging is built in), stop, connection lost and restored, tasks received, local triggers, log level changes |
| `checkin` | `check-in accepted` (how many policies, tasks and app installs the server returned) and `check-in complete` with duration; failures with the error. A routine check-in with nothing to do is `debug`, so the default log is not one line every five minutes |
| `policy` | `policy started`, one `policy step finished` / `policy step failed` per script, package, app, command and inventory step (exit code, duration, output summary), `policy finished`, scheduled restarts |
| `task` | `task started` and `task finished` for every task from the console (task ID, type, status, exit code, duration, output summary). Task payloads are never logged |
| `package` | Downloads (size, duration), cache hits (`debug`), checksum mismatches |
| `apps` | Installomator installs: label, status, duration, output summary |
| `inventory` | Collection and upload times, number of apps and extension attributes, upload failures |
| `compliance` | Scan start (rules, remediation on or off) and result counts with duration |
| `onboarding` | Every line of the onboarding run log, plus start and finish |
| `shell` | Remote shell sessions opened and ended (exit code, duration), refusals |
| `restricted`, `selfservice`, `spool` | Restricted-software kills, the Self Service portal, replay of undelivered results |

### The `logs` command

```bash
vibemdm-agent logs                          # last 50 lines
vibemdm-agent logs -n 200 --component policy
vibemdm-agent logs -f --level warn          # follow warnings and errors (Ctrl-C to stop)
vibemdm-agent logs --grep "Install Zoom"
vibemdm-agent logs --unified --last 2h      # the same records from unified logging
sudo vibemdm-agent logs --set-level debug   # more detail, applied to the running daemon at once
sudo vibemdm-agent logs --set-level info    # back to normal
```

Administrators can read the file without `sudo` (group `admin`). `--set-level` saves the level in
`/Library/Application Support/VibeMDM/logging.json`, so it survives restarts, and tells the running daemon over its
root-only socket. `sudo vibemdm-agent status` shows the current level, the log file's size and whether unified
logging is available. Debug records stay on the Mac; the server only receives `info` and above. Commands run by
hand as root (`policy`, `checkin`, `recon`) print their records to the terminal and append them to the same file.

### Unified logging needs a Mac-built agent

Writing to unified logging uses Apple's `os_log` C API, which Go can only call through cgo. The agent that `make
agent` and the Docker image build is cross-compiled on Linux with `CGO_ENABLED=0`. It writes to the log file and
the server only, and `status` reports `Unified logging: no`. To get unified logging, build the agent on a Mac with
Xcode or the Command Line Tools installed:

```bash
make agent-macos    # both architectures, CGO_ENABLED=1, into bin/agents/
```

Then copy the two binaries into the server's agent directory (`VIBEMDM_AGENT_DIR`). Installs and re-installs from
then on use them. Records are written with `%{public}`, so Console shows the text instead of `<private>`. `info`
records are stored as the `default` type, because unified logging keeps `info` only in memory. Warnings are
`default` records starting with `WARN`, since `os_log` has no warning type, and errors use the `error` type.

```bash
log show --last 1h --info --debug --predicate 'subsystem == "com.vibemdm.agent"'
log stream --level debug --predicate 'subsystem == "com.vibemdm.agent" AND category == "policy"'
```

> **Untested on real hardware.** The file log, rotation, the `logs` command, live level changes and log shipping
> were verified by running the agent against a test server on Linux. The `os_log` code path, how Console.app
> lists the file under Log Reports, and the `root:admin` 0640 permissions on macOS still need checking on a Mac.

### What is never logged

Device tokens, enrollment tokens, the super service-account password, SCEP challenges and FileVault keys never reach
the logger. As a safety net, any attribute whose name contains `token`, `password`, `passwd`, `secret`,
`passphrase`, `challenge`, `recovery_key` or `credential` is written as `[redacted]` in all three destinations.
Script parameters and task payloads are not logged. Step output summaries are the same output the server already
stores with the policy log or task result.

### Other useful commands on the Mac

```bash
sudo launchctl print system/com.vibemdm.agent | head -40     # is the daemon loaded and running?
cat /Library/Logs/VibeMDM/vibemdm-agent.launchd.log           # crash output, if any
```

## 18.3 Searching and live tail

**Logs** in the sidebar (server and agent tabs) and each computer's **Logs** tab use `GET /api/v1/logs`, with a
2-second live tail.

| Parameter | Meaning |
| --- | --- |
| `source` | `server` or `agent` |
| `level` | Minimum level: `debug`, `info`, `warn`, `error` |
| `component` | Exact component (`GET /api/v1/logs/components?source=` lists them) |
| `computer_id` | Agent logs for one Mac |
| `q` | Case-insensitive substring over the message and attributes (trigram-indexed when the `pg_trgm` extension is available) |
| `from`, `to` | RFC 3339 time range |
| `before_id` | Page back (results newest first) |
| `since_id` | Live tail (results oldest first, newer than the ID) |

Examples:

```
GET /api/v1/logs?source=server&component=nanomdm&level=warn
GET /api/v1/logs?source=agent&computer_id=<id>&q=Installomator
GET /api/v1/logs?source=server&q=push&from=2026-10-10T08:00:00Z
```

## 18.4 Export, retention and archives

| Feature | Details |
| --- | --- |
| **Export** | `GET /api/v1/logs/export` (admin, audited) with the same filters: matching entries as gzipped NDJSON |
| **Retention** | Entries older than `log_retention_days` (default 30, range 1–3650, Settings → General) are pruned hourly |
| **Archives** | With `log_archive_enabled`, the previous UTC day is written to `<data>/log-archive/YYYY-MM-DD-{server,agent}.ndjson.gz` before pruning. List and download with `GET /api/v1/logs/archives` and `GET /api/v1/logs/archives/{name}` (admin) |

Include `/data/log-archive` in your backups if you need long-term logs.

## 18.5 Diagnostic bundles

`POST /api/v1/computers/{id}/diagnostics` (admin) queues a `collect_diagnostics` task. The agent builds a gzipped
tarball and uploads it (`PUT /agent/v1/diagnostics/{id}`):

| File | Contents |
| --- | --- |
| `agent.log`, `agent.log.1`, `agent-launchd.log`, `agent-legacy.log` | The last 5 MB of the agent log and of its previous rotation, the launchd output (crashes), and the pre-upgrade `/var/log/vibemdm-agent.log` |
| `agent-unified.ndjson` | The agent's unified log for the last two hours (only from Mac-built agents, [§18.2](#182-agent-logs)) |
| `installomator.log` | Last 2 MB of `/var/log/Installomator.log` |
| `mdmclient.ndjson` | The **mdmclient** unified log for the last two hours (`log show --style ndjson --last 2h --predicate 'process == "mdmclient" OR subsystem == "com.apple.ManagedClient"'`). This is the device's side of every MDM conversation and the best place to see why a profile or command failed |
| `profiles-enrollment.txt` | `profiles status -type enrollment` |
| `profiles-list.txt` | `profiles list` |
| `sw_vers.txt` | `sw_vers` |
| `system_profiler.json` | Hardware and software overview |
| Others | The agent's launchd state, disk usage, and `agent.json` with the device token redacted |

`GET /api/v1/computers/{id}/diagnostics` lists bundles (status `requested`, `uploaded`, `failed`, with size and
SHA-256). `GET /api/v1/diagnostics/{id}/download` downloads one (admin, audited).

## 18.6 Audit log

Separate from operational logs, the **audit log** (Settings → Audit log, `GET /api/v1/audit`, admin) records who did
what: logins, password changes, settings changes, MDM commands, agent tasks, profile and policy changes, FileVault key
views, shell sessions, raw-payload reveals, refused agent enrollments, MCP tool calls, health remediations, and more.
Each entry has `at`, `actor`, `action` (for example `mdm.command`, `filevault.key_viewed`, `agent.enroll_refused`,
`mcp.tool`), `target_type`, `target_id` and `details`.

## 18.7 Study notes: logs on the Mac

> **Study note:**
> - macOS uses the **unified logging system**. `log show` reads stored logs; `log stream` follows them live;
>   `log collect` creates a `.logarchive` for later analysis in **Console**.
> - Watch MDM live while you send a command:
>   `log stream --info --predicate 'process == "mdmclient" OR subsystem == "com.apple.ManagedClient"'`.
> - Useful predicates: `subsystem == "com.apple.ManagedClient"` (profiles, MCX), `process == "softwareupdated"`
>   (updates), `process == "syspolicyd"` (Gatekeeper), `subsystem == "com.apple.TCC"` (privacy prompts).
> - `sudo profiles renew -type enrollment` re-runs enrollment checks (on ADE Macs it re-fetches the enrollment
>   configuration); `profiles status -type enrollment` shows whether the Mac is enrolled and whether enrollment was
>   user approved.
> - **System Information** (`system_profiler`) and **sysdiagnose** (`sudo sysdiagnose`) collect broad diagnostics
>   for Apple support cases.

Next: [Chapter 19 — Users, roles and the API](19-users-roles-api.md).
