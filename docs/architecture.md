# Architecture

## Components

- **`vibemdm`** – one Go binary: NanoMDM core (check-ins, command queue, APNs), SCEP CA, enrollment profile
  generator, agent API, admin API, embedded React console. Stateless apart from PostgreSQL and the data volume.
- **PostgreSQL** – the NanoMDM schema (`devices`, `enrollments`, `commands`, `command_results`, `enrollment_queue`,
  `push_certs`, …) plus VibeMDM tables (`computers`, `policies`, `policy_logs`, `scripts`, `packages`,
  `config_profiles`, `profile_deployments`, `computer_groups`, `agent_tasks`, `extension_attributes`, …).
  Migrations are embedded and applied at start-up.
- **Data volume** (`/data`) – uploaded packages.
- **`vibemdm-agent`** – macOS daemon (see agent.md).

## Request flow

1. A Mac installs the enrollment profile → SCEP (`/scep`) issues an identity certificate from the built-in CA
   (challenge password from settings) → the Mac sends `Authenticate` and `TokenUpdate` to `/mdm`.
2. `internal/mdmsvc` wraps the NanoMDM service: it creates/updates the `computers` row, marks MDM enrollment,
   queues the initial inventory commands and scoped profiles, and interprets command results
   (`DeviceInformation`, `SecurityInfo`, `ProfileList`, `InstalledApplicationList`, `InstallProfile`, `RemoveProfile`).
3. The agent enrolls with an invitation token or the enrollment secret and is matched to the same `computers` row
   via hardware UUID (= MDM UDID) or serial number. Each check-in evaluates policies: enabled → trigger matches →
   in scope (static/smart groups, exclusions) → frequency satisfied (`policy_logs`). A *pending* log row is created
   when a policy is handed out; the agent completes it with step output.
4. Console actions become MDM commands (`mdm_commands_meta` records intent) or agent tasks (`agent_tasks`), and the
   long-poll notifier wakes the agent immediately.

## Security notes

- Console sessions: bcrypt passwords, HttpOnly cookies, same-origin check on mutations, login rate limiting,
  roles (admin/viewer), API tokens (`vmdm_…`, hashed at rest), audit log.
- Agent: per-device bearer tokens (hashed at rest), enrollment tokens are consumable invitations or a rotatable secret.
- MDM: identity certificates are verified against the SCEP CA; NanoMDM's `certauth` binds each enrollment to its
  certificate so devices cannot impersonate one another.
- **Secrets at rest.** With `VIBEMDM_ENCRYPTION_KEY` set, the SCEP CA key, APNs key, APNs request key, FileVault
  escrow key, profile-signing key and FileVault recovery keys are encrypted with AES-256-GCM before they reach
  PostgreSQL. Existing plaintext values are sealed automatically at start-up.
- **Fail-safe start-up.** The server opens every stored ciphertext before doing anything else and refuses to start if
  the key is wrong or missing. A CA that exists but cannot be read is never replaced; first-boot CA creation is
  insert-only. Back up the key: without it, stored secrets are unrecoverable.
- **Key rotation.** Stop the server, then run
  `VIBEMDM_ENCRYPTION_KEY_OLD=… VIBEMDM_ENCRYPTION_KEY=… vibemdm rotate-encryption-key`. It re-encrypts everything in
  one transaction and changes nothing if the old key is wrong.
- TLS is terminated by Caddy (or your proxy). Apple requires a publicly trusted certificate.
- See [security.md](security.md) for the full security model, the hardening checklist and known limitations.

## Not (yet) included

Automated Device Enrollment (ABM/ASM), Declarative Device Management, iOS/iPadOS inventory views, Self Service
app, package signing. The data model and NanoMDM already support DDM/ADE plumbing, so these are natural next steps.

## Performance and on-demand behaviour

VibeMDM is designed so that console actions take effect in about a second, not at the next scheduled check-in:

| Action | Path | Latency |
| --- | --- | --- |
| Run script / command / policy now, update inventory | agent task → long-poll wake | ~1 s while the Mac is online |
| Save, enable or flush a policy | every agent in scope is asked to check in immediately (`checkin` flag on the long-poll) | ~1 s |
| "Check in now" on a computer | same mechanism, single Mac | ~1 s |
| MDM commands (lock, profiles, restart…) | APNs push → device connects | seconds |
| Periodic check-in | configurable (default 5 min), ±10 % jitter, faster retry after failures | background |

Agent-side: inventory collectors run concurrently (hardware, disk, security, apps, profiles, users, extension attributes
with bounded parallelism); packages are prefetched concurrently while "before" scripts run and cached by checksum so
re-runs never download twice; failed result submissions are spooled to disk and replayed when the server returns;
HTTP keep-alive/HTTP2 connections are reused.

Server-side: policy evaluation memoizes smart-group membership per check-in; inventory child rows (apps, profiles,
users, extension attributes) are written with single `jsonb_to_recordset` statements; the long-poll holds no database
connection while waiting; package downloads use `sendfile` with range support.
