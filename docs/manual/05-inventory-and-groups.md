# 5. Inventory and groups

Every managed Mac is one **computer** record. Inventory flows into it from two sources, and groups and searches
are built on top of it.

## 5.1 The computer record

Open **Computers** to list Macs (`GET /api/v1/computers?q=…&limit=&offset=`). The search box matches names, serial
numbers, users, models and similar fields. Click a Mac for its detail page, which has tabs for inventory,
management, activity, policies, compliance, updates, logs and (when enabled) the shell.

| Area | Fields (JSON names) |
| --- | --- |
| Identity | `id`, `udid`, `serial_number`, `name`, `hostname`, `local_hostname` |
| Hardware | `model`, `model_name`, `model_identifier`, `product_name`, `architecture`, `cpu`, `cpu_cores`, `memory_mb`, `disk_total_mb`, `disk_free_mb`, `battery_percent` |
| OS and network | `os_version`, `os_build`, `ip_address`, `mac_address`, `uptime_seconds` |
| Users | `last_user`, `last_user_full_name`, local accounts (with admin flag and Secure Token status) |
| Security | `filevault_enabled`, `sip_enabled`, `firewall_enabled`, `gatekeeper_enabled`, `secure_boot_level`, `is_supervised`, `activation_lock_enabled`, `bootstrap_token_escrowed` |
| Management | `mdm_enrolled`, `mdm_enrolled_at`, `mdm_last_checkin`, `mdm_unenrolled_at`, `agent_enrolled`, `agent_enrolled_at`, `agent_last_checkin`, `agent_version`, `last_inventory_at` |
| Raw | `mdm_device_info` (last `DeviceInformation`), `security_info` (last `SecurityInfo`, without the FileVault key blob), `hardware`, `inventory` |
| Admin-editable | `asset_tag`, `site`, `department`, `building`, `room`, `assigned_user`, `assigned_real_name`, `assigned_email`, `assigned_phone`, `assigned_position`, `notes` |

Edit the admin fields with `PATCH /api/v1/computers/{id}` (any subset of the fields above, plus `name`).

> **Renaming:** the agent reports the Mac's own *Computer Name* at every check-in and overwrites `name`. A rename
> in the console therefore lasts only until the next check-in. To rename the Mac itself, run a script such as
> `scutil --set ComputerName "NEW-NAME"; scutil --set LocalHostName NEW-NAME; scutil --set HostName NEW-NAME`, or use an
> onboarding naming template ([Chapter 14](14-white-glove-onboarding.md)).

Other per-computer endpoints:

| Endpoint | Returns |
| --- | --- |
| `GET /api/v1/computers/{id}/inventory` | Apps, profiles, local users and EA values |
| `GET /api/v1/computers/{id}/commands` | MDM commands (`?status=`) |
| `GET /api/v1/computers/{id}/tasks` | Agent tasks |
| `GET /api/v1/computers/{id}/policy-logs` | Policy runs |
| `POST /api/v1/computers/{id}/update-inventory` | Queues MDM inventory commands **and** an agent `inventory` task; reports `mdm_error`/`agent_error` for the channel that is missing |
| `POST /api/v1/computers/{id}/checkin` | Asks the agent to check in now; reports `agent_online` |
| `DELETE /api/v1/computers/{id}` | Deletes the record |
| `GET /api/v1/computers-export.csv` | All computers as CSV |

## 5.2 Inventory sources: MDM vs agent

| Data | From MDM | From the agent |
| --- | --- | --- |
| Device facts | `DeviceInformation` queries: `UDID`, `DeviceName`, `OSVersion`, `BuildVersion`, `SupplementalBuildVersion`, `ModelName`, `Model`, `ProductName`, `SerialNumber`, `DeviceCapacity`, `AvailableDeviceCapacity`, `BatteryLevel`, `IsSupervised`, `IsActivationLockEnabled`, `IsAppleSilicon`, `LocalHostName`, `HostName`, MAC addresses, `OSUpdateSettings`, `SystemIntegrityProtectionEnabled`, `ManagementStatus`, `TimeZone`, `AwaitingConfiguration`, … | `system_profiler SPHardwareDataType -json` (model, CPU, cores, memory), disk, network, IORegistry serial and hardware UUID |
| Security | `SecurityInfo`: `FDE_Enabled`, `FirewallSettings`, `SecureBoot`, SIP, bootstrap-token fields | `fdesetup status`, `csrutil status`, `socketfilterfw --getglobalstate`, `spctl --status` (Gatekeeper), `SPiBridgeDataType` |
| Profiles | `ProfileList` (all profiles, `ManagedOnly = false`) | `profiles` output (discarded when MDM reports profiles, to avoid duplicates) |
| Apps | `InstalledApplicationList` (stored **only if the agent is not installed**) | Walk of application bundles' `Info.plist` (name, version, bundle ID, path) |
| Users | — | `dscl` local accounts with UID, real name, home, admin flag, and `sysadminctl -secureTokenStatus` |
| Extension attributes | — | Script output |
| super status | — | `com.macjutsu.super.plist` ([Chapter 10](10-software-updates.md)) |
| FileVault key | `SecurityInfo` → `FDE_PersonalRecoveryKeyCMS` (escrow) | — |

Both sources write to the same fields (for example `filevault_enabled`); the most recent report wins.

**When inventory is collected:**

- **MDM:** on enrollment, when you click *Update inventory*, and `SecurityInfo` every 4 hours for Macs missing a
  FileVault key.
- **Agent:** on its first check-in (`enrollment_complete`), every `inventory_interval_hours` (default 24), on
  *Update inventory*, from a policy's *Update Inventory* option, onboarding *recon* steps, and
  `sudo vibemdm-agent recon`.
- **Light fields** (last user, host name, computer name, OS version and build, IP address, agent version, uptime)
  are updated at every agent check-in.

> **Known quirk:** `bootstrap_token_escrowed` is set to true when the Mac sends `SetBootstrapToken`, but every
> `SecurityInfo` response also overwrites it from `BootstrapTokenAllowedForAuthentication` (`allowed` → true).
> Those are different facts: one says a token was escrowed, the other says whether the bootstrap token may be used
> for authentication. Treat the field as an indication only; `sudo profiles status -type bootstraptoken` on the
> Mac is authoritative.

> **Study note:** `DeviceInformation`, `SecurityInfo`, `ProfileList`, `InstalledApplicationList` and
> `CertificateList` are the classic MDM inventory queries. With DDM, the **status channel** reports many values
> (OS version, build, model, software update state) without polling. On a Mac you can view the same facts with
> `system_profiler`, `sw_vers`, `fdesetup status`, `csrutil status` (from macOS), `spctl --status` and
> `profiles list`.

## 5.3 Extension attributes

An extension attribute (EA) adds a custom field to every computer.

| Field | Values | Notes |
| --- | --- | --- |
| `name`, `description` | text | Shown in inventory and in criteria as `EA: <name>` |
| `data_type` | `string` (default), `integer`, `date` | Decides which operators smart groups offer |
| `input_type` | `script` (default) or `text` | `script`: the agent runs it during inventory. `text`: an admin types the value |
| `script` | shell script | Runs as root with the [usual script environment](07-policies-scripts-packages.md#73-scripts) |
| `enabled` | bool | Disabled EAs are not run |

### Write a script EA

```bash
#!/bin/bash
# Is the Microsoft Defender daemon running?
if pgrep -x wdavdaemon >/dev/null; then
  echo "<result>Running</result>"
else
  echo "<result>Not running</result>"
fi
```

The value is the text between `<result>` and `</result>`; without the tags, the whole of stdout is used. Keep EAs
fast, because they run on every inventory.

### Procedure

1. **Extension Attributes → New**. Choose the data type and input type, paste the script, save
   (`POST /api/v1/extension-attributes`).
2. Trigger inventory on a test Mac (*Update inventory*, or `sudo vibemdm-agent recon`).
3. Check the value in the computer's inventory, then use it in a smart group.

For `text` EAs, set a value with `POST /api/v1/computers/{id}/ea-values/{ea}` `{"value": "…"}`.

## 5.4 Computer groups

| Type | Membership | Typical use |
| --- | --- | --- |
| **Static** | A fixed list you maintain (`PUT /api/v1/groups/{id}/members` `{"computer_ids": [...]}`) | Pilot rings, VIP Macs, test machines |
| **Smart** | Computed live from criteria at the moment it is evaluated | "macOS older than 15.6", "FileVault off", "has Zoom < 6" |

Smart-group membership is evaluated with SQL whenever it is needed (policy evaluation, scope resolution, the
member list). There is no cache to refresh; a Mac moves in or out as soon as its inventory changes. Within one
check-in, membership is memoized so a group used by many policies is computed once.

### Criteria

Each criterion has `and_or` (`and`/`or`, ignored on the first row), `field`, `operator`, `value`, and optional
`open_paren`/`close_paren` for grouping. `GET /api/v1/groups/criteria-fields` returns every field with its type
and operators, including one field per extension attribute (`ea:<id>`).

| Field type | Operators |
| --- | --- |
| string | `is`, `is_not`, `like`, `not_like`, `matches_regex`, `has_value`, `has_no_value` |
| number | `is`, `is_not`, `greater_than`, `less_than`, `greater_or_equal`, `less_or_equal` |
| bool | `is`, `is_not` |
| version | `is`, `is_not`, `like`, `not_like`, `greater_than`, `less_than`, `greater_or_equal`, `less_or_equal` (compared as versions, so `15.10` > `15.9`) |
| date | `more_than_x_days_ago`, `less_than_x_days_ago`, `before_date`, `after_date`, `has_value`, `has_no_value` |
| group | `member_of`, `not_member_of` |

Available fields: computer name, host name, serial number, UDID, model, model identifier, OS version, OS build,
architecture, processor, cores, RAM, boot drive size and free space, IP address, MAC address, last user, assigned
username, assigned e-mail, department, building, site, asset tag, agent version, FileVault, SIP, firewall,
Gatekeeper, supervised, Activation Lock, MDM enrolled, agent installed, Secure Boot level, last check-in, last
inventory, MDM and agent enrollment dates, record created, application title/version/bundle ID, profile
identifier/name, local user account, computer group, and every EA.

Application, profile and local-user criteria match if **any** item on the Mac matches.

### Worked example: Macs that need attention

1. **Computer Groups → New → Smart**.
2. Add criteria:

   | and/or | ( | Field | Operator | Value | ) |
   | --- | --- | --- | --- | --- | --- |
   | | | Agent Installed | is | true | |
   | and | ( | FileVault Enabled | is | false | |
   | or | | Operating System Version | less_than | 15.6 | |
   | or | | Boot Drive Available (MB) | less_than | 20000 | ) |

3. Click **Preview** (`POST /api/v1/groups/preview` with the criteria) to see who matches before saving.
4. Save. Use the group as a scope target, or as a criterion (`member_of`) in another group.

## 5.5 Advanced searches

An advanced search is a saved smart-group-style query with chosen **display fields**, for reporting rather than
scoping.

| Endpoint | Purpose |
| --- | --- |
| `GET /api/v1/advanced-searches/fields` | Fields available for display and criteria |
| `POST /api/v1/advanced-searches` | `{"name", "criteria": [...], "display_fields": [...]}` |
| `GET /api/v1/advanced-searches/{id}/run` | Run it |
| `GET /api/v1/advanced-searches/{id}/export.csv` | CSV export (cells starting with `= + - @` are neutralized against formula injection) |

Example: "Macs without a FileVault key", displaying name, serial, last user, OS version and last check-in.

## 5.6 Buildings, departments and network segments

These objects form a shared scoping vocabulary (Settings → Network organization).

### Buildings and departments

Named lists (`GET/POST /api/v1/buildings`, `PUT/DELETE /api/v1/buildings/{id}`, the same for `departments`).
A computer's `building` and `department` are text fields set by an admin, by onboarding, or by a network segment.
Each object shows how many computers use it.

### Network segments

A network segment is an inclusive IP range:

| Field | Example |
| --- | --- |
| `name` | `HQ Floor 2` |
| `starting_address` / `ending_address` | `10.20.2.1` – `10.20.2.254` |
| `assign_building` / `assign_department` | `HQ` / empty |

- A computer is matched by its **last reported IP address**, which is the agent's primary-interface address (a
  LAN address, not the public NAT address). Macs without the agent have no IP address and never match.
- When the agent reports an address inside a segment with an *assign* value, the computer's building or department
  is set from it. If several segments match, the narrowest range wins.
- Segments can be scope targets, limitations or exclusions ([Chapter 6](06-scoping.md)).

Next: [Chapter 6 — Scoping](06-scoping.md).
