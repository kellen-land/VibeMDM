# 15. Health

The Health engine continuously checks VibeMDM's own configuration and the fleet for problems an administrator would
want to act on. Each problem comes with an explanation and, where possible, a fix: a one-click remediation or a safe
automatic one ("self-healing"). The guiding rule is **visibility without noise**.

Open **Health** in the sidebar. Its badge counts open critical and warning findings. The dashboard has a Health
card, and each computer page shows a strip with that Mac's findings.

## 15.1 How the engine works

```
every 5 min (and 20 s after start) ─► run each enabled check ─► findings (check × object)
                                           │
                       ┌───────────────────┼─────────────────────────┐
                       ▼                   ▼                         ▼
                  new finding        still present              no longer present
                       │                   │                         │
          grace period? ── pending   update last_seen,          2 clean runs in a row
                       │  (hidden)   escalate if worse          (or a manual re-check)
                       ▼                   │                         ▼
                     open ◄────────────────┘                     resolved
                       │
         safe remediation + auto-heal on ─► self-heal (≤ 3 attempts, 1 h × attempts backoff)
```

| Term | Meaning |
| --- | --- |
| **Check** | A detector. It looks at settings, certificates, the database and device state, and returns the problems it sees *now*. It never tracks state itself |
| **Finding** | One problem with one object (computer, policy, profile, app, webhook, update plan, baseline, or the server). Deduplicated by (check, object) |
| **Remediation** | Either *runnable* (the server can do it, e.g. "Send an APNs push") or a *link* to the page where you fix it. Runnable remediations marked **safe** may run automatically |
| **States** | `pending` (in grace period, hidden), `open`, `acknowledged`, `snoozed`, `resolved` |
| **Severities** | `info`, `warning`, `critical` |

## 15.2 Noise controls

| Control | What it does |
| --- | --- |
| Conservative thresholds | Checks flag only conditions worth acting on (e.g. a policy failing on ≥ 3 Macs *and* on half of those it ran on) |
| Grace periods | Some checks hold a new finding as *pending* (hidden, no alerts) until it persists: 10 min (push failures), 15 min (invalid declarations), 30 min (MDM unresponsive, agent silent). If the condition clears first, the finding is deleted without a trace |
| Grouping | The console groups findings by check with an affected count ("40 computers") |
| Auto-resolve | A finding resolves after its condition is absent on **two consecutive runs**; **Re-check now** resolves it at once |
| Flap protection | A finding that returns within **1 hour** of resolving is reopened quietly: no new webhook, history kept |
| Acknowledge | Marks a finding as known: still listed, but out of the headline counts. It reopens if its severity escalates |
| Snooze | Hides a finding for 1 hour to 90 days; it returns if the condition is still present when the snooze ends |
| Info is opt-in | Info findings are listed but filtered out by default and never fire webhooks |
| Disable a check | **Health → Checks**. Disabling closes the check's findings quietly |
| Errors never resolve | If a check errors (e.g. a database timeout), its existing findings are left alone and the error is shown |

Resolved findings are kept for 30 days.

## 15.3 Self-healing

Each check has an **Auto-heal** setting: Default, On or Off. When it is on, the engine runs the first *safe*,
runnable remediation for each open or pending finding:

- at most **3 attempts** per finding, with a backoff of 1 hour × attempts between them;
- each attempt is recorded in the finding's timeline (`auto_heal` / `heal_failed`) and logged;
- snoozed findings are never healed;
- the counter resets when a finding resolves and later reappears.

Healing during the grace period is intentional: a safe fix such as an APNs push often clears the problem before you
ever see it.

Remediations that **change configuration** are never safe (e.g. *Turn remote shell off*, *Disable the webhook*,
*Turn the MCP server off*). They run only when an admin clicks them, and they are audited.

## 15.4 Every check

There are **26** built-in checks (`internal/health/checks.go`).

### Platform

| ID | Title | Severity | Condition | Remediation |
| --- | --- | --- | --- | --- |
| `platform.apns_certificate` | APNs push certificate | warning / critical | Missing (warning); expires in < 30 days; critical < 7 days or expired | Link: renew at the Apple Push Certificates Portal with the same Apple Account, upload in Settings → APNs |
| `platform.server_url` | Server URL | warning / critical | Not set or invalid (critical); not HTTPS (warning) | Link: Settings → General |
| `platform.scep_ca` | SCEP CA certificate | warning / critical | Expires in < 90 days (critical < 14) | Plan re-enrollment ([Chapter 3](03-apns-and-certificates.md)) |
| `platform.signing_certificate` | Profile signing certificate | warning / critical | Stored certificate unparseable, or expires in < 30 days | Link: replace in Settings → General |
| `platform.push_failures` | APNs push failures | warning / critical | Last hour: ≥ 3 pushes and ≥ 50 % failed (critical if none succeeded). Grace 10 min | Link: server logs |
| `platform.webhook_failures` | Failing webhooks | warning | An enabled webhook's last 5 deliveries all failed, the latest within 24 h | Link: review the webhook; *Disable the webhook* (not safe) |
| `platform.agent_binaries` | Agent binaries | warning | No prebuilt agent binaries in `VIBEMDM_AGENT_DIR`; the installer would fail | Link: Enrollment |

### Security

| ID | Title | Severity | Condition | Remediation |
| --- | --- | --- | --- | --- |
| `security.encryption_at_rest` | Encryption at rest | warning | `VIBEMDM_ENCRYPTION_KEY` not set | How to enable it ([§2.6](02-installation.md#26-the-encryption-key)) |
| `security.mcp_transport` | MCP server over plain HTTP | warning / critical | MCP enabled while the server URL is not HTTPS (critical if write tools are allowed) | Link: settings; *Turn the MCP server off* (not safe) |
| `security.remote_shell_transport` | Remote shell over plain HTTP | warning | Remote shell enabled while the server URL is not HTTPS | Link: settings; *Turn remote shell off* (not safe) |
| `security.device_posture` | Security features disabled | warning | SIP or Gatekeeper off on a Mac | Link: computer |
| `security.filevault_key_missing` | FileVault key not escrowed | warning | FileVault on, escrow profile installed > 24 h, no key | *Generate a new key with Escrow Buddy* (**safe**, needs the agent; at most once a day); link |
| `security.laps_not_current` | Local admin password not current | warning | A Mac in the LAPS scope has no password, a failing rotation, an unconfirmed password for over an hour, or a rotation a day overdue | *Rotate now* (**safe**, needs the agent; at most once an hour); links |
| `security.filevault_escrow_profile` | FileVault escrow not configured | warning | Macs with FileVault on and no key are not covered by the escrow profile | Link: FileVault |
| `security.compliance_drift` | Compliance drift | warning | Rules that passed (or were remediated) now fail on a Mac | *Rescan affected Macs* (**safe**); link to the report |
| `security.compliance_scan_failures` | Compliance scans failing | warning | ≥ 3 scans of a baseline failed or expired in 48 h | Link: report |

### Fleet

| ID | Title | Severity | Condition | Remediation |
| --- | --- | --- | --- | --- |
| `fleet.mdm_unresponsive` | MDM channel not responding | warning | Agent online, MDM commands waiting > 2 h, no MDM check-in for 2 h. Grace 30 min | *Send an APNs push* (**safe, auto-heal on by default**) |
| `fleet.agent_silent` | Agent not checking in | warning | MDM is checking in but the agent has been silent for > 3 check-in intervals (min. 2 h). Grace 30 min | Links: computer, agent install command |
| `fleet.agent_missing` | Agent not installed | info | MDM-enrolled > 24 h without the agent | Link: agent install command |
| `fleet.disk_space` | Low disk space | warning / critical | < 10 GB free (critical < 5 GB). macOS updates need roughly 25 GB | Link: computer |

### Configuration

| ID | Title | Severity | Condition | Remediation |
| --- | --- | --- | --- | --- |
| `configuration.profile_failures` | Configuration profile failures | warning | A profile is `failed` on one or more MDM-enrolled Macs (shows the latest error) | *Retry on failed Macs* (**safe**, auto-heal off by default); link |
| `configuration.ddm_invalid` | Invalid declarations | warning | A Mac reports a declaration as invalid (quotes Apple's reason). Grace 15 min | Links: computer's updates tab, update plans |
| `configuration.ddm_not_syncing` | Declarations not syncing | info | Asked to sync > 6 h ago, not fetched | *Ask the Mac to sync declarations* (**safe, auto-heal on by default**) |
| `configuration.policy_failures` | Failing policies | warning | Latest run per Mac failed on ≥ 3 Macs and ≥ 50 % of Macs it ran on (7 days) | Link: logs |
| `configuration.app_install_failures` | Failing app installs | warning | Same rule for Installomator apps (broken label or vendor change) | Link: logs |
| `configuration.policy_never_runs` | Policy can never run | info | Enabled, but no trigger and not in Self Service, or nothing to do | Link: edit policy |
| `configuration.stale_scope_references` | Scope references deleted objects | warning | A policy, profile, app or update plan scope names a deleted group, computer or network segment | Link: edit scope |

## 15.5 Using the Health page

1. **Overview**: headline counts, findings grouped by check (up to 3 samples each), last run time and any check
   errors (`GET /api/v1/health/summary`).
2. Click a group to see the findings; click a finding for its detail, facts, remediations and event timeline
   (`GET /api/v1/health/findings/{id}`).
3. Run a remediation: `POST /api/v1/health/findings/{id}/remediate` `{"remediation_id": "push"}`. The server re-runs
   the check, performs the fix against the current state (with a 2-minute timeout), then re-evaluates. The result is
   recorded as `remediated` or `remediation_failed`.
4. Acknowledge or snooze: `POST /api/v1/health/findings/{id}/state`
   `{"state": "acknowledged" | "snoozed" | "open", "snooze_hours": 24}`.
5. Bulk actions: `POST /api/v1/health/findings/bulk`
   `{"ids": [...], "action": "acknowledge" | "snooze" | "reopen" | "remediate", ...}`.
6. **Checks** tab: `GET /api/v1/health/checks`; `PUT /api/v1/health/checks/{id}` with `{"enabled": false}` or
   `{"auto_heal": true | false | null}` (null = the check's default); `POST /api/v1/health/checks/{id}/run` runs one
   check, `POST /api/v1/health/run` runs all.
7. Recent lifecycle events across all findings: `GET /api/v1/health/events?limit=`.

`GET /api/v1/health/findings` accepts `state`, `severity`, `check`, `object_type` and `object_id`. The default states
are open, acknowledged and snoozed; use `state=resolved` for history. Viewers may read everything; changes require
an admin.

## 15.6 Webhooks

| Event | When |
| --- | --- |
| `HealthIssueOpened` | A **warning or critical** finding opened (after its grace period) or escalated |
| `HealthIssueResolved` | A finding that had been announced resolved |

Payload:

```json
{
  "event": "HealthIssueOpened",
  "timestamp": "2026-10-10T12:00:00Z",
  "server": "vibemdm",
  "payload": {
    "finding_id": "…", "check_id": "fleet.disk_space", "check": "Low disk space",
    "severity": "critical", "summary": "3.1 GB free", "object_type": "computer",
    "object_id": "…", "object_name": "MBP-A-1042"
  }
}
```

Info findings, quiet reopenings and snoozed findings never fire webhooks. Webhook setup is in
[Chapter 19](19-users-roles-api.md#195-webhooks).

## 15.7 Adding a check (developers)

Checks live in `internal/health/checks.go`. A check returns the problems it sees now; the engine handles
persistence, grace periods, resolution, notifications and healing.

```go
r.Add(Check{ID: "fleet.example", Category: "fleet", Title: "Example",
    Description: "What is wrong and why it matters.",
    Grace: 15 * time.Minute, SupportsHeal: true,
    Run: func(ctx context.Context) ([]Finding, error) {
        return []Finding{{ObjectType: "computer", ObjectID: id, ObjectName: name, Severity: Warning,
            Summary: "Short headline", Detail: "How to fix it",
            Remediations: []Remediation{{ID: "fix", Label: "Fix it", Safe: true, Run: fixFn}}}}, nil
    }})
```

Guidelines: return an error rather than an empty list when you cannot tell (errors never resolve findings); keep
`ObjectID` stable; mark a remediation `Safe` only if it is idempotent, cannot lose data and does not change
configuration.

Next: [Chapter 16 — MDM commands and the inspector](16-mdm-commands-and-inspector.md).
