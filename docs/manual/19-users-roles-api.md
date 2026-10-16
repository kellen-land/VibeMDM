# 19. Users, roles and the API

## 19.1 Console users and roles

| Role | Can |
| --- | --- |
| **admin** | Everything |
| **viewer** | Read inventory and configuration. Cannot change anything: every non-GET request returns `403 admin role required` |

Viewers also don't see secrets embedded in configuration:

- profile payloads are redacted like MDM command plists;
- script bodies are replaced with `# Visible to administrators only.`;
- agent task payloads lose `command`, `contents`, `parameters`, `env`, `script` and `auth_password`.

Some read endpoints are admin-only: users, API tokens, the audit log, webhooks, shell sessions and transcripts, log
export and archives, diagnostic downloads, the enrollment profile download, and FileVault key reveal. The SCEP
challenge and the agent enrollment secret are returned only to admins.

> Viewers **do** see task output (stdout/stderr). Don't write scripts that print secrets.

### Manage users (Settings → Users, admin)

| Action | API |
| --- | --- |
| List | `GET /api/v1/users` |
| Create | `POST /api/v1/users` `{"username", "display_name", "email", "role": "admin" \| "viewer", "password"}` |
| Update | `PUT /api/v1/users/{id}` `{"display_name", "email", "role", "enabled"}`. You cannot demote or disable yourself |
| Reset a password | `POST /api/v1/users/{id}/password` `{"password": "…"}`. Ends all the user's sessions and forces a change at next login |
| Delete | `DELETE /api/v1/users/{id}`. You cannot delete yourself |

Every signed-in user (viewers too) can change their own password with `POST /api/v1/auth/password`
`{"current_password", "new_password"}`; this works only from a browser session, not with an API token.

Passwords: bcrypt (cost 12), 12 to 72 characters when set (existing shorter passwords keep working until changed). Sessions end after `VIBEMDM_SESSION_IDLE_MINUTES` (default 120) without requests and after `VIBEMDM_SESSION_TTL_HOURS` in any case. Login throttling locks the client
IP and the username after every 6 failures for 1, then 5, 15 and 60 minutes (`429`). Sessions last
`VIBEMDM_SESSION_TTL_HOURS` (default 168 = 7 days).

| Endpoint | Auth |
| --- | --- |
| `POST /api/v1/auth/login` `{"username", "password"}` | Public, same-origin only; sets the session cookie |
| `POST /api/v1/auth/logout` | Same-origin only |
| `GET /api/v1/auth/me` | The current user (for a token: `{"id", "username": "token:<name>", "role", "via": "token"}`) |

## 19.2 API tokens

Create tokens for automation under **Settings → API Tokens** (admin):

```json
POST /api/v1/api-tokens
{ "name": "inventory-export", "role": "viewer", "expires_days": 90 }
→ 201 { "token": "vmdm_…", "api_token": { "id", "name", "token_prefix", "role", "expires_at", … } }
```

- The token is shown **once**; it is stored as a SHA-256 hash with a short prefix for identification.
- `role` defaults to `admin` if it is not `viewer`. Give automation the least privilege it needs.
- `expires_days` is optional; without it the token never expires.
- `last_used_at` is updated on use. Revoke with `DELETE /api/v1/api-tokens/{id}`.
- Tokens are exempt from the CSRF same-origin check, because browsers never send them automatically.

Use a token with the `Authorization` header:

```bash
curl -H "Authorization: Bearer vmdm_…" "https://mdm.example.com/api/v1/computers?q=macbook&limit=20"
```

## 19.3 REST API overview

Base path `/api/v1`. Lists return `{"items": [...], "total": n}` and accept `limit`/`offset`; errors return
`{"error": "..."}` (400 bad input, 401 not authenticated, 403 forbidden or password change required, 404 not found,
409 conflict, 429 rate limited). API responses are `Cache-Control: no-store`.

| Area | Endpoints |
| --- | --- |
| Auth | `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`, `POST /auth/password`, `GET /status` (public) |
| Dashboard | `GET /dashboard` |
| Computers | `GET /computers`, `GET/PATCH/DELETE /computers/{id}`, `GET /computers/{id}/{commands,policy-logs,tasks,inventory,timeline,ddm,filevault,compliance,onboarding,diagnostics}`, `POST /computers/{id}/{commands,tasks,push,update-inventory,checkin,reset-agent,ddm/sync,diagnostics}`, `POST /computers/{id}/filevault/{reveal,remediate}`, `POST /computers/{id}/ea-values/{ea}`, `GET /computers/{id}/shell` (WebSocket), `GET /computers-export.csv` |
| Groups | `GET/POST /groups`, `GET/PUT/DELETE /groups/{id}`, `GET/PUT /groups/{id}/members`, `GET /groups/criteria-fields`, `POST /groups/preview` |
| Searches | `GET/POST /advanced-searches`, `GET /advanced-searches/fields`, `GET/PUT/DELETE /advanced-searches/{id}`, `GET /advanced-searches/{id}/run`, `GET /advanced-searches/{id}/export.csv` |
| Network organization | `GET/POST /buildings`, `PUT/DELETE /buildings/{id}`, same for `/departments` and `/network-segments`, `POST /scope/preview` |
| Scripts | `GET/POST /scripts`, `GET/PUT/DELETE /scripts/{id}`, `POST /scripts/{id}/run` |
| Packages | `GET/POST /packages` (multipart `file`), `GET/PUT/DELETE /packages/{id}`, `GET /packages/{id}/download` |
| Extension attributes | `GET/POST /extension-attributes`, `GET/PUT/DELETE /extension-attributes/{id}` |
| Policies | `GET/POST /policies`, `GET/PUT/DELETE /policies/{id}`, `GET /policies/{id}/logs`, `POST /policies/{id}/{flush,run}`, `GET /policy-logs`, `GET /policy-logs/{id}` |
| Profiles | `GET/POST /profiles`, `GET/PUT/DELETE /profiles/{id}`, `GET /profiles/{id}/deployments`, `POST /profiles/{id}/{deploy,remove}` |
| Custom settings | `GET/POST /custom-schemas`, `GET/PUT/DELETE /custom-schemas/{id}`, `POST /custom-schemas/{id}/{preview,profiles}` |
| Mac Apps | `GET/POST /apps`, `GET /apps/labels`, `GET /apps/logs`, `POST /apps/installomator-preview`, `GET/PUT/DELETE /apps/{id}`, `GET /apps/{id}/logs`, `GET/POST /custom-labels`, `GET/PUT/DELETE /custom-labels/{id}` |
| Restricted software | `GET/POST /restricted-software`, `GET /restricted-software/events`, `GET/PUT/DELETE /restricted-software/{id}` |
| Software updates | `GET/POST /updates/plans`, `GET/PUT/DELETE /updates/plans/{id}`, `GET /updates/overview`, `GET /updates/catalog`, `GET/PUT /updates/super`, `GET /updates/super/status`, `POST /updates/super/install` |
| FileVault | `GET/PUT /filevault`, `POST /filevault/check` |
| Local admin passwords | `GET/PUT /laps`, `GET /laps/pending-rotations`, `GET /computers/{id}/laps`, `GET /computers/{id}/laps/history`, `POST /computers/{id}/laps/{reveal,rotate}`, `PUT /computers/{id}/laps/password`, `POST /computers/{id}/laps/secure-token`; Jamf Pro-compatible routes under `/api/v2/local-admin-password` ([docs/laps.md](../laps.md#jamf-pro-compatible-api)) |
| Compliance | `GET /compliance/catalog`, `GET /compliance/catalog/rules`, `GET /compliance/catalog/rules/{rule}`, `GET/POST /compliance/baselines`, `GET/PUT/DELETE /compliance/baselines/{id}`, `GET /compliance/baselines/{id}/{report,scans,profile,export.csv}`, `GET /compliance/baselines/{id}/computers/{cid}`, `POST /compliance/baselines/{id}/scan` |
| Identity | `GET/PUT /identity/xcreds` |
| Onboarding | `GET/POST /onboarding/workflows`, `GET/PUT/DELETE /onboarding/workflows/{id}`, `GET /onboarding/workflows/{id}/runs`, `POST /onboarding/workflows/{id}/run` |
| Health | `GET /health/summary`, `GET /health/findings`, `GET /health/findings/{id}`, `POST /health/findings/{id}/{state,remediate}`, `POST /health/findings/bulk`, `GET /health/checks`, `PUT /health/checks/{id}`, `POST /health/checks/{id}/run`, `POST /health/run`, `GET /health/events` |
| Activity | `GET /commands`, `GET /commands/search`, `GET/DELETE /commands/{uuid}`, `GET /commands/{uuid}/inspect`, `GET /tasks`, `GET /tasks/{id}`, `POST /tasks/{id}/cancel` |
| Logs | `GET /logs`, `GET /logs/components`, `GET /logs/export`, `GET /logs/archives`, `GET /logs/archives/{name}`, `GET /diagnostics/{id}/download` |
| Remote shell | `GET /shell-sessions`, `GET /shell-sessions/{id}` |
| Enrollment | `GET /enrollment/status`, `GET /enrollment/profile` (admin), `GET/POST /enrollment/invitations`, `PATCH/DELETE /enrollment/invitations/{id}`; automatic agent install: `GET /agent-package`, `PUT /agent-package?version=` (admin, raw signed .pkg), `PUT /agent-package/settings`, `POST /agent-package/sign-csr` (admin), `GET/POST /computers/{id}/agent-install`; public `GET /agent/pkg/manifest.plist`, `GET /agent/pkg/VibeMDM-Agent.pkg` |
| Settings | `GET/PUT /settings`, `POST /settings/rotate-enrollment-secret`, `GET/PUT /settings/apns`, `POST /settings/apns/{request,decrypt}`, `PUT /settings/apns/certificate`, `GET /settings/scep`, `POST /settings/scep/regenerate` |
| Webhooks | `GET/POST /webhooks`, `POST /webhooks/test`, `GET /webhooks/deliveries`, `PUT/DELETE /webhooks/{id}` |
| Admin | `GET/POST /users`, `PUT/DELETE /users/{id}`, `POST /users/{id}/password`, `GET/POST /api-tokens`, `DELETE /api-tokens/{id}`, `GET /audit` |

Outside `/api/v1`: `/mdm` (MDM protocol), `/scep`, `/enroll`, `/enroll/profile`, `/enroll/configurator/{token}`,
`/agent/v1/*` and `/agent/install.sh`, `/agent/download/{platform}`, `/agent/installomator-license`, `/mcp`,
`/healthz`, `/version`.

### Examples

```bash
TOKEN=vmdm_…; S=https://mdm.example.com/api/v1

# Macs with FileVault off, as CSV via a saved search
curl -sH "Authorization: Bearer $TOKEN" "$S/advanced-searches/<id>/export.csv" -o fv-off.csv

# Lock a Mac
curl -sH "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
     -d '{"type":"DeviceLock","pin":"482913","message":"Call IT: 555-0100"}' "$S/computers/<id>/commands"

# Run a policy on a group now
curl -sH "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
     -d '{"group_ids":["<group-id>"]}' "$S/policies/<policy-id>/run"
```

## 19.4 The agent API (for reference)

The agent authenticates with `Authorization: Bearer vmda_…`; an unknown token returns `401 invalid device token;
re-enroll the agent`.

| Endpoint | Purpose |
| --- | --- |
| `POST /agent/v1/enroll` | Enroll with an invitation token or the enrollment secret (public; `409` on refused takeover) |
| `POST /agent/v1/checkin` | Check in with a trigger; returns policies, tasks, app installs, restricted rules, inventory flag |
| `GET /agent/v1/tasks?wait=55` | Long-poll for tasks, check-in requests and shell invitations |
| `POST /agent/v1/tasks/{id}/result`, `POST /agent/v1/policy-logs/{id}` | Results |
| `GET /agent/v1/recon-config`, `POST /agent/v1/inventory` | Inventory |
| `GET /agent/v1/packages/{id}`, `GET /agent/v1/installomator` | Downloads |
| `GET /agent/v1/enrollment-profile` | MDM enrollment profile (for `--mdm`) |
| `POST /agent/v1/app-installs/{id}/result`, `POST /agent/v1/compliance-scans/{id}`, `POST /agent/v1/onboarding/{id}[/user-entry]`, `POST /agent/v1/restricted-events`, `POST /agent/v1/logs`, `PUT /agent/v1/diagnostics/{id}`, `POST /agent/v1/diagnostics/{id}/failed` | Feature results |
| `GET /agent/v1/selfservice`, `POST /agent/v1/selfservice/run` | Self Service |
| `GET /agent/v1/shell` | Remote shell dial-back (WebSocket) |
| `POST /agent/v1/unenroll` | Remove the agent's record |
| `GET /agent/v1/version` | Server version (public) |

## 19.5 Webhooks

Webhooks deliver signed JSON POSTs to your endpoints when events happen (**Settings → Webhooks**, admin).

### Create

```json
POST /api/v1/webhooks
{ "name": "Slack relay", "url": "https://hooks.example.com/vibemdm", "secret": "a-long-random-secret",
  "enabled": true, "events": ["HealthIssueOpened", "HealthIssueResolved", "PolicyFailed"] }
```

An empty `events` list means `["*"]` (everything). `POST /api/v1/webhooks/test` sends a `Test` event to subscribed
hooks; `GET /api/v1/webhooks/deliveries` lists recent deliveries with status, error and attempts.

### Delivery

| Property | Behaviour |
| --- | --- |
| Method / body | `POST`, `Content-Type: application/json`, the envelope below |
| Headers | `User-Agent: vibemdm-webhook`, `X-VibeMDM-Event: <event>`, and `X-VibeMDM-Signature` when a secret is set |
| Signature | Base64 of HMAC-SHA256 over the raw body, keyed with the secret |
| Success | Any status below 300 |
| Retries | 3 attempts in total, waiting 5 s then 10 s |
| Redirects | Never followed |
| Address safety | Loopback, link-local (including 169.254.169.254) and unspecified addresses are refused; private RFC 1918 ranges are allowed |
| Queue | Non-blocking; if the queue is full, the event is dropped and logged |

The Health check **Failing webhooks** opens when an enabled hook's last 5 deliveries failed.

### Envelope

```json
{ "event": "PolicyFailed", "timestamp": "2026-10-10T12:34:56Z", "server": "vibemdm",
  "payload": { "computer_id": "…", "computer": "MBP-A-1042", "log_id": "…", "trigger": "checkin", "status": "failed" } }
```

### Events and payloads

| Event | When | Payload fields |
| --- | --- | --- |
| `ComputerEnrolled` | First MDM `TokenUpdate` | `computer_id`, `computer`, `serial`, `udid`, `os_version` |
| `ComputerUnenrolled` | MDM `CheckOut` | `computer_id`, `computer`, `serial`, `udid` |
| `AgentEnrolled` | Agent enrollment | `computer_id`, `computer`, `serial`, `udid` |
| `ComputerCheckIn` | **Every** agent check-in (high volume) | `computer_id`, `computer`, `serial`, `udid`, `trigger`, `user`, `os_version`, `ip` |
| `ComputerInventoryCompleted` | Agent inventory stored | `computer_id`, `computer`, `serial`, `apps` (count), `os_version` |
| `PolicyCompleted` / `PolicyFailed` | Policy log submitted | `computer_id`, `computer`, `log_id`, `trigger`, `status` |
| `MDMCommandCompleted` | MDM command result (fired twice per result, see [§16.1](16-mdm-commands-and-inspector.md#161-sending-commands)) | `computer_id`, `computer`, `command_uuid`, `request_type`, `status`, `error` |
| `RestrictedSoftwareViolation` | Restricted process found | rule, process, user, whether it was killed, computer |
| `ShellSessionOpened` | Remote shell opened | `computer_id`, `computer`, `actor`, `ip`, `session` |
| `FileVaultKeyEscrowed` | A new PRK escrowed | `computer_id`, `computer`, `serial`, `fingerprint` |
| `FileVaultKeyViewed` | An admin revealed a key | `computer_id`, `computer`, `serial`, `actor`, `reason` |
| `LAPSPasswordRotated` | A new local admin password was applied and escrowed | `computer_id`, `computer`, `serial` |
| `LAPSPasswordViewed` | An admin revealed a local admin password | `computer_id`, `computer`, `serial`, `actor`, `reason` |
| `ConsoleLoginLockout` | Repeated failed logins locked a username or client IP | `kind` (`username`, `ip`, `password_change`), `subject`, `until` |
| `HealthIssueOpened` / `HealthIssueResolved` | [§15.6](15-health.md#156-webhooks) | `finding_id`, `check_id`, `check`, `severity`, `summary`, `object_type`, `object_id`, `object_name` |
| `Test` | `POST /webhooks/test` | `message`, `fired_by`, `at` |

### Verify a signature

```python
import base64, hashlib, hmac

def valid(body: bytes, header: str, secret: str) -> bool:
    mac = hmac.new(secret.encode(), body, hashlib.sha256).digest()
    return hmac.compare_digest(base64.b64encode(mac).decode(), header)
```

Verify against the **raw** request body, before any JSON parsing.

## 19.6 The MCP server for AI agents

VibeMDM includes a [Model Context Protocol](https://modelcontextprotocol.io) server, so AI agents (Claude Code,
Claude Desktop, Claude's MCP connector in the API, other MCP clients) can answer questions about the fleet and, if you
allow it, take a few safe actions.

| Property | Value |
| --- | --- |
| Endpoint | `https://<server>/mcp` |
| Transport | Streamable HTTP, **POST only**, JSON responses (no SSE stream, no sessions) |
| Protocol versions | `2025-11-25`, `2025-06-18`, `2025-03-26` |
| Methods | `initialize`, `ping`, `tools/list`, `tools/call`; notifications are acknowledged with `202`; batching is rejected |

### Turn it on

1. **Settings → AI assistants (MCP)**: enable it (`PUT /api/v1/settings` `{"mcp_enabled": true}`).
   While it is off, `/mcp` returns **404**. The card also shows the endpoint and a ready-made `claude mcp add` snippet.
2. **Settings → API Tokens**: create a token for the agent. A **viewer** token can only use the read tools.
3. Optional: enable **Allow write tools** (`{"mcp_allow_writes": true}`) so **admin** tokens get the write tools.
   Viewer tokens never get them.

Use HTTPS. The Health check *MCP server over plain HTTP* warns when MCP is on without HTTPS, and is critical when write
tools are also allowed. Both toggles are audited (`settings.mcp`, `settings.mcp_writes`).

### Connect a client

**Claude Code:**

```bash
claude mcp add --transport http vibemdm https://mdm.example.com/mcp \
  --header "Authorization: Bearer vmdm_…"
```

**Claude Desktop and other clients:** add a remote (HTTP) MCP server with the URL and the header
`Authorization: Bearer vmdm_…`. Clients that only launch local (stdio) servers can use a bridge such as
`npx mcp-remote https://mdm.example.com/mcp --header "Authorization: Bearer vmdm_…"`.

**Claude API (MCP connector):** pass the server in `mcp_servers` with `"url": "https://mdm.example.com/mcp"` and
`"authorization_token": "vmdm_…"`.

### Tools

Computers can be named by ID, serial number or exact name.

| Tool | Access | Arguments | What it does |
| --- | --- | --- | --- |
| `fleet_summary` | read | — | Computers, enrollment, check-ins, pending work, open health issues |
| `search_computers` | read | `query`, `limit` (≤ 100) | Find Macs by name, serial, user, model, asset tag, host name or IP |
| `get_computer` | read | `computer` | Inventory, security, groups, health issues, compliance scores, super status |
| `list_health_issues` | read | `severity`, `computer`, `include_acknowledged` | Open findings grouped by check, with explanations and fixes |
| `list_policies` | read | — | Policies with triggers, frequency, scope summary and recent outcomes |
| `get_policy_logs` | read | `policy`, `computer`, `status`, `limit` | Recent runs (output truncated) |
| `list_profiles` | read | — | Profiles and deployment counts (no payloads) |
| `compliance_overview` | read | `baseline` | Baseline scores; for one baseline, the most-failing rules and least-compliant Macs |
| `software_update_status` | read | — | DDM update plans and super status |
| `search_logs` | read | `query`, `source`, `min_level`, `computer`, `limit` (≤ 200) | Server or agent logs |
| `recent_mdm_commands` | read | `computer`, `limit` | A Mac's recent commands and status (no payloads) |
| `recent_audit_events` | read | `limit` | Recent administrative actions |
| `send_push` | write | `computer` | Wake a Mac with an APNs push |
| `update_inventory` | write | `computer` | Fresh inventory (agent if installed, otherwise MDM) |
| `run_policy` | write | `policy`, `computer` | Run an existing policy on one Mac |
| `acknowledge_health_issue` | write | `finding_id`, `snooze_hours` (1–2160) | Acknowledge or snooze a finding |
| `remediate_health_issue` | write | `finding_id`, `remediation_id` | Run a runnable remediation offered by a finding |
| `start_compliance_scan` | write | `baseline`, `computer` (omit for the whole scope) | Scan; remediation is never started from MCP |

Each tool carries annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint: false`) so clients
can ask you before running a write tool. Tool calls time out after 60 seconds.

### Security model

| Control | Detail |
| --- | --- |
| Authentication | Only API tokens (`Authorization: Bearer vmdm_…`). Browser session cookies are rejected (**401**), so websites cannot drive the server through a logged-in admin's browser |
| Origin check | Requests with an `Origin` header must come from VibeMDM's own origin (**403**). This is the DNS-rebinding protection the MCP specification requires |
| Authorization | Write tools are hidden from viewer tokens and from everyone unless *Allow write tools* is on; calling a hidden tool returns the JSON-RPC error `unknown tool` |
| No secrets | Tools never return FileVault keys, the SCEP challenge, the enrollment secret, profile payloads, script bodies or raw MDM plists. Logs and policy output are truncated |
| No dangerous actions | Erase, lock, restart, remote shell, arbitrary commands and script uploads are never exposed. Write tools act only on objects an admin already configured. Note that `remediate_health_issue` can run any *runnable* remediation, including the non-safe "turn off" ones (remote shell, MCP, a webhook) |
| Audit | Every tool call is written to the audit log as `mcp.tool` with the token name (`token:<name>`) and the truncated arguments |
| Rate limit | 120 requests per minute per token (**429**) |
| Body size | 1 MB |
| Revocation | Delete the token to cut an agent off immediately |

Treat AI agents like any other automation: give them a viewer token unless they need to act, review the audit log, and
remember that text in your fleet data (computer names, log lines) reaches the model as **untrusted input**.

Next: [Chapter 20 — Security hardening](20-security-hardening.md).
