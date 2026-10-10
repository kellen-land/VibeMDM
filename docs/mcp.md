# MCP server for AI agents

VibeMDM includes a [Model Context Protocol](https://modelcontextprotocol.io) server, so AI agents can answer
questions about your fleet and, if you allow it, take a few safe actions. Supported clients include Claude Code,
Claude Desktop, Claude's MCP connector in the API, and other MCP clients. For example, an agent can answer "Which
Macs fail the CIS benchmark and why?", "What health issues are open?" or "Run the Zoom policy on Alice's Mac."

- **Endpoint:** `https://<your server>/mcp` (Streamable HTTP transport, JSON responses).
- **Protocol versions:** 2025-11-25, 2025-06-18 and 2025-03-26.

## Turning it on

1. **Settings → General → MCP server**: enable it. While it is off, `/mcp` returns 404.
2. **Settings → API Tokens**: create a token for the agent. A **viewer** token can only use the read tools.
3. Optional: enable **Allow write tools** to give **admin** tokens the write tools below. Viewer tokens never get them.

Use HTTPS. The Health page raises a warning if MCP is on without HTTPS, and a critical issue when write tools are
also allowed.

## Connecting Claude

**Claude Code**

```bash
claude mcp add --transport http vibemdm https://mdm.example.com/mcp \
  --header "Authorization: Bearer vmdm_…"
```

**Claude Desktop / other clients:** add a remote (HTTP) MCP server with the URL above and the header
`Authorization: Bearer vmdm_…`. Clients that only launch local (stdio) servers can use a bridge such as
`npx mcp-remote https://mdm.example.com/mcp --header "Authorization: Bearer vmdm_…"`.

**Claude API (MCP connector):** pass the server in `mcp_servers` with `"url": "https://mdm.example.com/mcp"` and
`"authorization_token": "vmdm_…"`.

## Tools

Computers can be named by ID, serial number or exact name.

| Tool | Access | What it does |
| --- | --- | --- |
| `fleet_summary` | read | Computers, enrollment, check-ins, pending work, open health issues |
| `search_computers` | read | Find Macs by name, serial, user, model, asset tag, hostname or IP |
| `get_computer` | read | Inventory, security, groups, health issues, compliance scores, super status |
| `list_health_issues` | read | Open health findings grouped by check, with explanations and available fixes |
| `list_policies` / `get_policy_logs` | read | Policies and recent runs (log tails only) |
| `list_profiles` | read | Profiles and deployment counts (no payloads) |
| `compliance_overview` | read | Baseline scores; for one baseline, the most-failing rules and least-compliant Macs |
| `software_update_status` | read | DDM update plans and super deployment status |
| `search_logs` | read | Server or agent logs |
| `recent_mdm_commands` | read | A Mac's recent MDM commands and their status (no payloads) |
| `recent_audit_events` | read | Recent administrative actions |
| `send_push` | write | Wake a Mac with an APNs push |
| `update_inventory` | write | Request fresh inventory (agent or MDM) |
| `run_policy` | write | Run an existing policy on one Mac |
| `acknowledge_health_issue` | write | Acknowledge or snooze a health finding |
| `remediate_health_issue` | write | Apply a runnable fix offered by a health finding |
| `start_compliance_scan` | write | Scan a Mac or a baseline's scope. Remediation is never started from MCP. |

Each tool is described with annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`), so clients can ask
you before running a write tool.

## Security

- **Authentication:** only VibeMDM API tokens are accepted (`Authorization: Bearer vmdm_…`). Browser session
  cookies are rejected, so websites cannot drive the server through a logged-in admin's browser.
- **Origin check:** requests carrying an `Origin` header must come from VibeMDM's own origin. This is the
  DNS-rebinding protection the MCP specification requires.
- **No secrets:** tools never return FileVault keys, the SCEP challenge, the enrollment secret, profile payloads,
  script bodies or raw MDM plists. Logs and policy output are truncated.
- **No dangerous actions:** erase, lock, restart, remote shell, arbitrary commands and script uploads are never
  available over MCP. Write tools act only on objects an admin already configured.
- **Audit and limits:**
  - Every tool call is written to the audit log as `mcp.tool`, with the token name and the arguments, truncated.
  - Each token is limited to 120 requests per minute.
  - Revoke a token under **Settings → API Tokens** to cut off an agent immediately.

Treat AI agents like any other automation: give them a viewer token unless they need to act. Review the audit log,
and remember that text in your fleet data (computer names, log lines) reaches the model as untrusted input.
