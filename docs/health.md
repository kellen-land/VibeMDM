# Health: misconfiguration detection and self-healing

VibeMDM continuously checks its own configuration and the fleet for problems an administrator would want
to act on. For each problem it explains what is wrong and why it matters. Where it can, it offers a fix:
a one-click remediation, or a safe automatic one ("self-healing").

The guiding rule is **visibility without noise**. A finding should be something worth your attention.
Brief blips, duplicates and resolved conditions stay out of the way.

Open **Health** in the console sidebar. The badge counts open critical and warning findings.

## How it works

```
every 5 min ─► run each enabled check ─► findings (check × object)
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
         safe remediation + auto-heal on ─► self-heal (≤3 attempts, 1 h backoff)
```

- **Checks** are detectors that look at server settings, certificates, the database and device state.
  There are 22 built-in checks in four categories: *platform*, *security*, *fleet* and *configuration*.
- A **finding** is one problem with one object, such as a computer, policy, profile, app, webhook,
  update plan, or the server itself. Findings are deduplicated by (check, object). The console groups
  them by check, so 40 Macs with low disk space show up as one row reading "40 computers".
- A **remediation** is either *runnable*, meaning the server can do it ("Send an APNs push", "Retry on
  failed Macs"), or a *link* to the page where you fix it. Remediations marked **safe** may run
  automatically when auto-heal is on for that check.

## Noise controls

| Control | What it does |
| --- | --- |
| Conservative thresholds | Checks only flag conditions worth acting on. Examples: a policy fails on ≥3 Macs *and* on half of those it ran on; disk below 10 GB; MDM commands waiting over 2 h while the agent is online. |
| Grace periods | Some checks hold a new finding as *pending* (hidden, no alerts) until it has persisted. Examples: 10 min for push failures, 30 min for an unresponsive MDM channel. If the condition clears first, the finding is deleted and leaves no trace. |
| Grouping and dedupe | One finding per (check, object). The console groups them by check with an affected count. |
| Auto-resolve | A finding resolves after its condition is absent on **two consecutive runs**, so a single missed scan never closes an issue. **Re-check now** resolves it immediately. |
| Flap protection | A finding that returns within an hour of resolving is reopened **quietly**: no new webhook, and its history is kept. |
| Acknowledge | Marks a finding as known. It stays listed but drops out of the headline counts. If its severity escalates, it reopens. |
| Snooze | Hides a finding for 1 hour to 90 days. When the snooze expires and the condition is still present, it comes back. |
| Info is opt-in | Info-level findings (e.g. "MDM enrolled without the agent") are listed but filtered out by default. They never fire webhooks. |
| Disable a check | **Health → Checks**. Disabling closes the check's findings quietly. |
| Failed checks never resolve findings | If a check errors (e.g. a database timeout), its existing findings are left alone and the error shows on the Health page. |

## Self-healing

Each check has an **Auto-heal** setting: Default, On or Off. When it is on, the engine runs the first *safe*,
runnable remediation for each open or pending finding:

- at most **3 attempts** per finding, with a backoff of 1 h × attempts between them;
- every attempt is recorded in the finding's timeline (`auto_heal` / `heal_failed`) and logged;
- snoozed findings are never healed;
- the attempt counter resets when a finding resolves and later reappears.

Healing during the grace period is intentional. A safe fix such as an APNs push often clears the
problem before you ever see it.

| Check | Safe remediation | Auto-heal default |
| --- | --- | --- |
| MDM channel not responding | Send an APNs push | **On** |
| Declarations not syncing | Re-send the DDM sync | **On** |
| Configuration profile failures | Retry InstallProfile on failed Macs | Off |
| FileVault key not escrowed | Queue Escrow Buddy (agent), at most once a day | Off (see also FileVault auto-remediate) |

Remediations that change configuration are never safe. Examples are *Turn remote shell off* and
*Disable the webhook*. These run only when an administrator clicks them, and they are audited.

## Built-in checks

| Category | Check | Severity | Condition |
| --- | --- | --- | --- |
| platform | APNs push certificate | warning / critical | missing; expires in < 30 days (critical < 7 or expired) |
| platform | Server URL | warning / critical | not set or invalid (critical); not HTTPS |
| platform | SCEP CA certificate | warning / critical | expires in < 90 days (critical < 14) |
| platform | Profile signing certificate | warning / critical | unparseable; expires in < 30 days |
| platform | APNs push failures | warning / critical | ≥ 3 and ≥ 50 % of pushes failed in the last hour (critical if none succeeded) |
| platform | Failing webhooks | warning | last 5 deliveries failed, the latest within 24 h |
| security | Encryption at rest | warning | `VIBEMDM_ENCRYPTION_KEY` not set |
| security | Remote shell over plain HTTP | warning | remote shell on while the server URL is not HTTPS |
| security | Security features disabled | warning | SIP or Gatekeeper off |
| security | FileVault key not escrowed | warning | FileVault on, escrow profile installed > 24 h, no key |
| security | FileVault escrow not configured | warning | Macs with FileVault on and no key are not covered by the escrow profile |
| fleet | MDM channel not responding | warning | agent online, MDM commands waiting > 2 h, no MDM check-in for 2 h |
| fleet | Agent not checking in | warning | MDM active but the agent silent for > 3 check-in intervals (min. 2 h) |
| fleet | Agent not installed | info | MDM enrolled > 24 h without the agent |
| fleet | Low disk space | warning / critical | < 10 GB free (critical < 5 GB) |
| configuration | Configuration profile failures | warning | a profile failed to install on one or more Macs |
| configuration | Invalid declarations | warning | a Mac reports a DDM declaration as invalid (shows Apple's reason) |
| configuration | Declarations not syncing | info | asked to sync > 6 h ago, not fetched |
| configuration | Failing policies | warning | latest run per Mac failed on ≥ 3 Macs and ≥ 50 % (7 days) |
| configuration | Failing app installs | warning | same rule for Installomator apps (broken label or vendor change) |
| configuration | Policy can never run | info | enabled, but no trigger and not in Self Service, or nothing to do |
| configuration | Scope references deleted objects | warning | a policy/profile/app/update plan scope names a deleted group, computer or network segment |

## Notifications

Two webhook events (see **Settings → Webhooks**):

- `HealthIssueOpened`: a **warning or critical** finding opened (after its grace period) or escalated.
- `HealthIssueResolved`: a finding that was announced has resolved.

Payload: `{finding_id, check_id, check, severity, summary, object_type, object_id, object_name}`. Info
findings, quiet reopenings (flapping) and snoozed findings never fire webhooks.

## API

All endpoints are under `/api/v1`. Viewers may read; writes require an admin.

| Endpoint | Purpose |
| --- | --- |
| `GET /health/summary` | headline counts, groups by check (≤ 3 samples each), last run, check errors |
| `GET /health/findings?state=&severity=&check=&object_type=&object_id=` | list findings (default states: open, acknowledged, snoozed; `state=resolved` for history) |
| `GET /health/findings/{id}` | one finding with its event timeline |
| `POST /health/findings/{id}/state` | `{"state": "acknowledged" \| "snoozed" \| "open", "snooze_hours": 24}` |
| `POST /health/findings/{id}/remediate` | `{"remediation_id": "push"}`. Re-runs the check, performs the fix against current state, then re-evaluates. |
| `POST /health/findings/bulk` | `{"ids": [...], "action": "acknowledge" \| "snooze" \| "reopen" \| "remediate", ...}` |
| `GET /health/checks` / `PUT /health/checks/{id}` | list checks; `{"enabled": false}`, `{"auto_heal": true \| false \| null}` (null = default) |
| `POST /health/checks/{id}/run` / `POST /health/run` | run one check or all checks now |
| `GET /health/events?limit=` | recent lifecycle events across all findings |

## Adding a check

Checks live in `internal/health/checks.go`. A check is a function that returns the problems it sees *now*.
The engine (`internal/health/engine.go`) handles persistence, grace periods, resolution, notifications and
healing, so a detector never tracks state itself:

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

Guidelines:

- Return an error rather than an empty list when you cannot tell; errors never resolve findings.
- Keep `ObjectID` stable, so the same problem maps to the same finding.
- Mark a remediation `Safe` only if it is idempotent, cannot lose data and does not change configuration.

Planned next sources: mSCP compliance drift (TODO #8) feeds in as another check.
