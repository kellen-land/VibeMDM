# 7. Policies, scripts and packages

Policies are VibeMDM's main automation object. A policy says **when** (triggers and frequency), **where** (scope)
and **what** (scripts, packages, Installomator apps, a command, inventory, restart, messages). Policies run
through the agent, not through MDM.

## 7.1 The policy object

| Field | JSON | Notes |
| --- | --- | --- |
| Name, category | `name`, `category` | |
| Enabled | `enabled` | Disabled policies never run (not even in Self Service) |
| Triggers | `triggers` | `startup`, `login`, `checkin`, `enrollment_complete`, `network_change` (booleans) and `custom` (a name) |
| Frequency | `frequency` | `ongoing`, `once_per_computer` (default), `once_per_day`, `once_per_week`, `once_per_month` |
| Scope | `scope` | [Chapter 6](06-scoping.md) |
| Scripts | `scripts[]` | `script_id`, `priority` (`before` / `after`), `parameters` (up to 8 values for `$4`–`$11`) |
| Packages | `packages[]` | `package_id`, `action` (`install`) |
| Apps | `apps[]` | `label` (an Installomator label or `custom:<key>`), `options` ([Chapter 9](09-apps-installomator.md)) |
| Execute command | `run_command` | One shell command line, run with `/bin/bash -c` as root |
| Update inventory | `update_inventory` | Full recon at the end |
| Restart | `reboot` | `enabled`, `message`, `delay_minutes` (0 = now) |
| User messages | `user_interaction` | `start_message`, `complete_message` (macOS notifications) |
| Self Service | `self_service` | `enabled`, `display_name`, `description`, `button_text` |

API: `GET/POST /api/v1/policies`, `GET/PUT/DELETE /api/v1/policies/{id}`.

## 7.2 Triggers and frequency

### Triggers

| Trigger | Fires when |
| --- | --- |
| `startup` | The daemon starts and the Mac has been up less than 10 minutes (then a normal `checkin` follows immediately) |
| `login` | A user logs in: the per-user LaunchAgent runs `vibemdm-agent trigger login`, which signals the daemon over its local socket |
| `checkin` | Every periodic check-in (default every 300 s, ±10 % jitter), every on-demand check-in, `vibemdm-agent checkin` |
| `enrollment_complete` | Once per computer, on the first regular check-in after agent enrollment (inventory is also requested) |
| `network_change` | When something sends the `network_change` trigger to the daemon (see the note below) |
| custom (`custom: "install_zoom"`) | `sudo vibemdm-agent policy --event install_zoom`, a `custom_trigger` task, or a script that calls the agent |

> **Note:** the agent accepts `network_change` on its local socket and policies can select it, but the current
> agent has **no built-in network watcher** that fires it. Fire it yourself, for example from a LaunchDaemon with
> `WatchPaths` on `/Library/Preferences/SystemConfiguration`, running `vibemdm-agent trigger network_change`.

On each check-in the server returns every policy that is **enabled**, has a **matching trigger**, is **in scope**
and whose **frequency is satisfied**. A *pending* policy-log row is created when a policy is handed out; the agent
completes it with each step's output.

### Frequency

| Frequency | Runs again when |
| --- | --- |
| `ongoing` | Every time the trigger fires (a run already pending or running in the last hour is not duplicated) |
| `once_per_computer` | Never, after one **completed** run |
| `once_per_day` | No completed run in the last 1 day |
| `once_per_week` | No completed run in the last 7 days |
| `once_per_month` | No completed run in the last 30 days |

**Failed runs do not count**, so a failed once-per-computer policy is retried at the next matching trigger. A run
that stays *pending* or *running* blocks duplicates for one hour; stale runs are then marked expired.

### Flush and run now

| Action | API | Effect |
| --- | --- | --- |
| Flush logs | `POST /api/v1/policies/{id}/flush` (optionally `{"computer_id": "…"}`) | Deletes the policy's run history (for one Mac or all), so once-per-X policies run again. In-scope agents are asked to check in |
| Run now | `POST /api/v1/policies/{id}/run` `{"computer_ids": [...], "group_ids": [...], "scoped": false}` | Queues a `run_policy` task with trigger `manual`. `scoped: true` targets everything in the policy's scope. Frequency is not checked |
| Save / enable | `PUT /api/v1/policies/{id}` | In-scope agents are asked to check in within about a second |

## 7.3 Scripts

The script library (**Scripts**) stores shell scripts.

| Field | Notes |
| --- | --- |
| `name`, `category`, `info`, `notes` | Descriptive |
| `priority` | Default position (`before`/`after`) when added to a policy |
| `parameter_labels` | Up to 8 labels shown for `$4`–`$11` in policy and run dialogs |
| `os_requirements` | Informational |
| `contents` | The script. If it starts with `#!`, it is executed directly (so the shebang chooses the interpreter: `#!/bin/zsh`, `#!/usr/bin/python3`, …). Otherwise it runs with `/bin/bash` |

### Execution environment

Scripts run **as root**, from a private 0700 directory under `/Library/Application Support/VibeMDM/tmp`, with the
working directory `/` and a 2-hour timeout.

| Positional parameter | Value |
| --- | --- |
| `$1` | Target mount point, always `/` |
| `$2` | Computer name |
| `$3` | Console (logged-in) username; empty at the login window |
| `$4` – `$11` | Policy or run parameters, in order (empty strings if unset) |

| Environment variable | Value |
| --- | --- |
| `VIBEMDM_SERVER_URL` | Server URL |
| `VIBEMDM_COMPUTER_ID` | This computer's ID in VibeMDM |
| `VIBEMDM_SCRIPT_NAME` | Script name |
| `VIBEMDM_POLICY_NAME`, `VIBEMDM_POLICY_ID` | Set when run from a policy |

stdout and stderr (up to 64 KB, then truncated) and the exit code are recorded. A **non-zero exit marks the step
failed**, which marks the policy run failed.

### Example: a parameterized script

```bash
#!/bin/zsh
# $4 = dock position (left|bottom|right), $5 = autohide (true|false)
user="$3"
[[ -z "$user" ]] && { echo "No user logged in"; exit 0; }
uid=$(id -u "$user")
launchctl asuser "$uid" sudo -u "$user" defaults write com.apple.dock orientation -string "${4:-bottom}"
launchctl asuser "$uid" sudo -u "$user" defaults write com.apple.dock autohide -bool "${5:-false}"
launchctl asuser "$uid" sudo -u "$user" killall Dock
```

Set the labels to `Dock position` and `Autohide` so that `$4` and `$5` are self-explanatory in the policy editor.

### Run a script on demand

`POST /api/v1/scripts/{id}/run` `{"computer_ids": [...], "group_ids": [...], "parameters": ["left", "true"]}`
queues a `run_script` task on each Mac. Results appear live in the run dialog and under **Activity**.

> **Security note:** viewers do not see script bodies, but they **do** see task output. Never echo secrets.

## 7.4 Packages

**Packages** is VibeMDM's built-in distribution point.

1. **Packages → Upload**: choose a `.pkg`, `.mpkg`, `.dmg` or `.zip` (`POST /api/v1/packages`, multipart field
   `file`). The server stores it in `/data/packages` and records its size and SHA-256.
2. Add it to a policy's **Packages** list.

How the agent installs a package:

| Type | Action |
| --- | --- |
| `.pkg`, `.mpkg` | `installer -verboseR -pkg <file> -target /` |
| `.dmg` | Mounted read-only and hidden (`hdiutil attach -nobrowse`); every `.pkg` inside is installed and every `.app` is copied to `/Applications` with `ditto`; then detached |
| `.zip` | Expanded with `ditto -x -k`; `.pkg` files installed, `.app` bundles copied to `/Applications` |

Downloads start as soon as the policy starts (up to 3 at once) while the `before` scripts run. Each file is
verified against its SHA-256 and cached by checksum for 14 days, so a re-run installs from the local copy.

> Any enrolled agent can download any package by ID. Treat packages as non-secret: never embed credentials in them.

> **Study note:** macOS installer packages are flat `.pkg` files (xar archives) with a payload and optional
> pre/postinstall scripts. For distribution outside the Mac App Store, Apple expects packages and apps to be signed
> with a **Developer ID** and **notarized**, so Gatekeeper accepts them. MDM itself can install packages with
> `InstallEnterpriseApplication` (a signed distribution package referenced by a manifest URL), which VibeMDM exposes
> as an MDM command, but the agent route is the one VibeMDM uses for policies. `pkgutil --pkgs` and
> `pkgutil --files <id>` show installed package receipts.

## 7.5 Execution order of a policy

1. `start_message` notification (if set and a user is logged in).
2. Package downloads begin in the background.
3. `before` scripts, in order.
4. Packages, in order (after their downloads finish).
5. Apps (Installomator), in order.
6. **Local Accounts** actions, in order (7.5.1).
7. **Execute Command**.
8. `after` scripts, in order.
9. **Update Inventory** (if enabled).
10. The log is submitted; if submission fails it is spooled and replayed.
11. `complete_message` notification.
12. Restart (if enabled): a notification with the restart message, then `shutdown -r now` or `shutdown -r +N`.

A failed step does not stop later steps; it marks the run failed. Notifications use `osascript display
notification` in the user's session.

Policy logs: `GET /api/v1/policies/{id}/logs`, `GET /api/v1/policy-logs`, `GET /api/v1/policy-logs/{id}`, and the
computer's policy tab. Each log has the trigger, status, timestamps, a text log and per-step details (type,
name, status, exit code, output, duration). Webhooks `PolicyCompleted` and `PolicyFailed` fire when a run is
submitted.

### 7.5.1 Local Accounts

The *Local Accounts* tab of a policy is Jamf Pro's Local Accounts payload. Each action names a user and runs in
order:

| Action | What the agent does |
| --- | --- |
| **Create Account** | `sysadminctl -addUser` with the full name, password, optional hint and home directory (default `/Users/<username>`), as a standard user or, with *Allow user to administer computer*, an administrator; then `createhomedir`. An account that already exists is left unchanged, so the policy can run repeatedly. *Enable user for FileVault* grants a Secure Token with `sysadminctl -secureTokenOn` through the **LAPS account**, which must hold a Secure Token itself (Jamf Pro uses its management account the same way); without one, a Mac with an escrowed bootstrap token grants the token at the user's first login-window sign-in. |
| **Reset Password** | Without a Secure Token: `dscl . -passwd`. With a Secure Token: `sysadminctl -resetPasswordFor … -adminUser <LAPS account>`, so FileVault keeps working; if the LAPS account has no Secure Token the action fails with that explanation instead of using dscl. The user's login keychain keeps the old password until they update it. |
| **Delete Account** | `sysadminctl -deleteUser`, keeping the home folder unless *Delete home folder* is on. Never deletes the logged-in user or the only account with a Secure Token. Deleting an account that does not exist succeeds as a no-op. |
| **Disable Account for FileVault** | `fdesetup remove -user`. Never removes the last FileVault user; does nothing when FileVault is off. |

The LAPS account cannot be targeted (it is managed under Security → LAPS). Passwords are sealed with
`VIBEMDM_ENCRYPTION_KEY` (set it, or they are stored as given), never returned by the API (`password_set` says one
is stored; send an entry without `password` to keep it) and never part of the policy the agent receives or of
stored tasks: during the run the agent fetches them from `POST /agent/v1/policy-logs/{log_id}/account-secrets`,
which answers only while that computer's run is open and is audited (`policy.account_passwords_sent`). Command
output is scrubbed of passwords before it is logged.

API: `accounts` on `POST/PUT /api/v1/policies`:
`[{"action": "create" | "reset_password" | "delete" | "disable_filevault", "username", "full_name", "password",
"password_hint", "home_directory", "admin", "enable_filevault", "delete_home"}]`.

Agents report the optional features they support at each check-in (`capabilities`; Local Accounts is
`policy_accounts`). A policy with Local Accounts actions is never sent to an older agent, which would ignore them and
report success: **Run now** answers with an error, and a scheduled run is logged as *failed* with the reason (at most
once a day). Update the agent and the policy runs at the next check-in.

**Untested on a real Mac:** every action above (the simulator only fetches the passwords and reports the steps).

## 7.6 Self Service

Self Service lets users run approved policies and install apps themselves.

- The agent daemon serves a local portal at **`http://127.0.0.1:8571`**. Users open it with
  `vibemdm-agent selfservice`, a bookmark, or a Dock link you deploy.
- It lists every **enabled** policy with `self_service.enabled` that is in scope for the Mac, plus every Mac App
  with Self Service enabled. Each item shows its display name, description and button text.
- Clicking the button asks the server (`POST /agent/v1/selfservice/run`). The server re-checks enabled and scope,
  creates a pending log with trigger `self_service` and returns the policy or app, which the agent runs at once.
  **Frequency is not applied** to Self Service runs.
- The portal accepts only requests to its own host name (DNS-rebinding protection), and requests that run something
  must send the custom header `X-VibeMDM-SelfService` and a JSON body, which other websites cannot send without a
  CORS preflight that the portal never approves.

To make a policy Self Service only, enable Self Service and select no triggers. (The Health check *Policy can
never run* flags policies with no trigger that are not in Self Service.)

## 7.7 Restricted software

Restricted software rules stop named processes, like Jamf's feature of the same name.

| Field | Notes |
| --- | --- |
| `name` | Rule name |
| `process_name` | Process to match |
| `match_type` | `exact`, `contains` or `regex` |
| `kill_process` | Kill the process when found |
| `notify_user`, `display_message` | Tell the user why |
| `enabled`, `scope` | As usual |

The agent receives the rules for its Mac at each `checkin`/`startup` check-in and checks the process list
(`ps -axo pid=,user=,comm=`) every 20 seconds. Violations are reported (`GET /api/v1/restricted-software/events`)
and fire the `RestrictedSoftwareViolation` webhook. This is a deterrent, not a security boundary: a process can
run for up to 20 seconds, and renaming a binary defeats `exact` matching.

## 7.8 The agent's local triggers

The daemon listens on two Unix sockets:

| Socket | Permissions | Accepts |
| --- | --- | --- |
| `/var/run/vibemdm-agent.sock` | world-writable | Only `login`, `checkin` and `network_change` |
| `/var/run/vibemdm-agent.sock.root` | root only (0600) | Any trigger, including custom ones |

`vibemdm-agent trigger <event>` sends to the root socket when run as root and to the public socket otherwise. When
run as root and the daemon is down, it runs the check-in directly. `sudo vibemdm-agent policy --event NAME`
checks in directly (without the daemon) and prints `Executed N policies for trigger "NAME"`.

Use custom triggers to chain work: a policy's *after* script can run
`/usr/local/bin/vibemdm-agent policy --event step2` to start the next policy.

## 7.9 Agent tasks (one-off actions)

`POST /api/v1/computers/{id}/tasks` queues a one-off task for a Mac's agent (the agent must be installed):

| `type` | Extra fields | Does |
| --- | --- | --- |
| `run_script` | `script_id`, `parameters` (8) | Runs a library script |
| `run_command` | `command` | Runs a shell command as root |
| `run_policy` | `policy_id` | Runs a policy now (trigger `manual`) |
| `inventory` | — | Full recon |
| `custom_trigger` | `trigger` | Fires a custom trigger |
| `notify` | `message` | Shows a notification |
| `restart`, `shutdown` | `message` | Notification, then restart or shut down in 1 minute |
| `logout_user` | — | Logs out the console user (`launchctl bootout gui/<uid>`) |
| `filevault_regenerate` | — | Escrow Buddy remediation ([Chapter 11](11-filevault.md)) |

Other task types are created by features: `compliance_scan`, `install_super`, `onboarding`,
`collect_diagnostics`. Tasks are listed under **Activity** (`GET /api/v1/tasks`) and can be cancelled before
delivery (`POST /api/v1/tasks/{id}/cancel`).

Next: [Chapter 8 — Configuration profiles](08-configuration-profiles.md).
