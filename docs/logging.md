# Logging and diagnostics

VibeMDM keeps two searchable log streams in PostgreSQL, plus on-demand diagnostic bundles from individual Macs.

## Server logs

Every server log line goes to standard output (for `docker compose logs`) **and**, at or above
`VIBEMDM_LOG_DB_LEVEL` (default `info`), into the `server_logs` table. Writes are batched every second and never block
request handling. If the database can't keep up, records are dropped and counted, and the count is reported in the log.

Components you'll see: `mdm` (VibeMDM's MDM hooks), `nanomdm` (protocol engine), `agent` (agent API), `api`, `scep`,
`policy`, `commander`, `webhook`. HTTP access lines carry `method`, `path`, `status`, `dur` and `remote`.

## Agent logs

The agent logs to `/var/log/vibemdm-agent.log` on the Mac and ships records at `info` and above to the server every 30
seconds. While the server is unreachable, up to 5,000 records stay in memory; on shutdown they are spooled to
`/Library/Application Support/VibeMDM/spool/logs.ndjson` (capped at 5 MB) and sent after the next start.

## Searching and tailing

`GET /api/v1/logs?source=server|agent` accepts:

| Parameter | Meaning |
| --- | --- |
| `level` | minimum level: `debug`, `info`, `warn`, `error` |
| `component` | exact component (`GET /api/v1/logs/components?source=` lists them) |
| `computer_id` | agent logs for one Mac |
| `q` | case-insensitive substring over the message and attributes (trigram-indexed when `pg_trgm` is available) |
| `from`, `to` | RFC 3339 time range |
| `before_id` | page back (results newest first) |
| `since_id` | live tail (results oldest first, newer than the id) |

The console's **Logs** page and each computer's **Logs** tab use these, with a 2-second live tail.

## Export, retention and archives

- **Export** (`GET /api/v1/logs/export`, admin, audited): matching entries as gzipped NDJSON.
- **Retention**: entries older than `log_retention_days` (default 30, Settings → General) are pruned hourly.
- **Archives**: with `log_archive_enabled`, the previous UTC day is written to
  `<data>/log-archive/YYYY-MM-DD-{server,agent}.ndjson.gz` before pruning. List and download them under
  `GET /api/v1/logs/archives`.

## Diagnostic bundles

`POST /api/v1/computers/{id}/diagnostics` (admin) asks the agent for a gzipped tarball containing:

- the agent log (last 5 MB) and Installomator log
- the **mdmclient** unified log for the last two hours (`log show --predicate 'process == "mdmclient" OR subsystem ==
  "com.apple.ManagedClient"'`). This is the device's side of every MDM conversation, and the best place to see why a
  profile or command failed.
- `profiles status -type enrollment`, `profiles list`, `sw_vers`, hardware/software summary, the agent's launchd state,
  disk usage, and the agent configuration with the device token redacted

Download with `GET /api/v1/diagnostics/{id}/download` (admin, audited).

## Study notes (Apple certification)

- `log stream --predicate 'process == "mdmclient"' --info` on a Mac shows MDM traffic live while you send a command.
- `sudo profiles renew -type enrollment` re-runs enrollment checks; `profiles status -type enrollment` shows whether
  the Mac is MDM enrolled and whether enrollment was user approved.
