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

The agent logs to `/var/log/vibemdm-agent.log` on the Mac (the LaunchDaemon's stdout and stderr) and ships records
at `info` and above to the server every 30 seconds (`POST /agent/v1/logs`). While the server is unreachable, up to
5,000 records stay in memory; on shutdown they are spooled to
`/Library/Application Support/VibeMDM/spool/logs.ndjson` (capped at 5 MB) and sent after the next start.

On the Mac:

```bash
sudo tail -f /var/log/vibemdm-agent.log
sudo launchctl print system/com.vibemdm.agent | head -40     # is the daemon loaded and running?
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
| Agent log | Last 5 MB of `/var/log/vibemdm-agent.log`, plus the Installomator log |
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
