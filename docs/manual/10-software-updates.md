# 10. Software updates

VibeMDM offers three ways to manage macOS updates:

| Tool | Mechanism | Best for |
| --- | --- | --- |
| **DDM update plans** (native) | Declarative Device Management declarations | Apple's supported path: a target version by a deadline, plus update settings |
| **super** (S.U.P.E.R.M.A.N.) | Open-source script installed and configured by VibeMDM | Richer user experience: deferral menus, maintenance windows, focus-aware dialogs, scheduled installs |
| **Legacy MDM commands** | `AvailableOSUpdates`, `ScheduleOSUpdate` | Ad-hoc checks and installs on older macOS; deprecated by Apple |

## 10.1 DDM update plans

**Software Updates → Plans** (`/api/v1/updates/plans`). Each plan has the standard scope.

| Kind | Declaration type | macOS | What it does |
| --- | --- | --- | --- |
| `enforcement` | `com.apple.configuration.softwareupdate.enforcement.specific` | 14+ | Requires a target version (and optionally a build) by a local date and time. macOS shows reminders as the deadline approaches, downloads and installs, and restarts at the deadline |
| `settings` | `com.apple.configuration.softwareupdate.settings` | 15+ | Notifications, deferrals, automatic actions, Rapid Security Response behaviour, standard-user updates |

### Plan fields

| Field | Applies to | Validation |
| --- | --- | --- |
| `name`, `enabled`, `scope` | both | |
| `kind` | both | `enforcement` or `settings` |
| `target_os_version` | enforcement | `15.6`, `15.6.1` (regex `^\d+(\.\d+){0,2}$`) |
| `target_build_version` | enforcement | Optional, e.g. `24G84` |
| `target_local_datetime` | enforcement | `YYYY-MM-DDTHH:MM:SS`, interpreted in **the Mac's local time zone** |
| `details_url` | enforcement | Optional `https://` URL shown to the user |
| `settings` | settings | See below |

Settings keys (JSON) and the declaration keys they produce:

| `settings` key | Values | Declaration key |
| --- | --- | --- |
| `notifications` | bool | `Notifications` |
| `allow_standard_user_os_updates` | bool | `AllowStandardUserOSUpdates` |
| `major_deferral_days` | 1–90 | `Deferrals.MajorPeriodInDays` |
| `minor_deferral_days` | 1–90 | `Deferrals.MinorPeriodInDays` |
| `system_deferral_days` | 1–90 | `Deferrals.SystemPeriodInDays` |
| `auto_download` | `Allowed`, `AlwaysOn`, `AlwaysOff` | `AutomaticActions.Download` |
| `auto_install_os_updates` | `Allowed`, `AlwaysOn`, `AlwaysOff` | `AutomaticActions.InstallOSUpdates` |
| `auto_install_security_updates` | `Allowed`, `AlwaysOn`, `AlwaysOff` | `AutomaticActions.InstallSecurityUpdate` |
| `rapid_security_response` | bool | `RapidSecurityResponse.Enable` |
| `rapid_security_response_rollback` | bool | `RapidSecurityResponse.EnableRollback` |

### When several plans reach the same Mac

- **Enforcement:** the plan with the **highest target version** wins; ties go to the earliest deadline.
- **Settings:** the most recently updated plan wins.

### Create an enforcement plan

1. **Software Updates → Plans → New**, kind **Enforcement**.
2. Pick the target version from the catalog (§10.4) or type it, for example `15.6.1`, and optionally the build.
3. Set the deadline, e.g. `2026-11-01T17:00:00` (Mac local time). Give users time: Apple's own guidance is to allow
   several days so the Mac can download and remind.
4. Optionally add a details URL (your change notice).
5. Scope it to a pilot group first. Save.

Saving triggers a DDM reconcile at once; it also runs every 30 minutes. Macs whose declaration set changed get a
`DeclarativeManagement` command and then fetch the new declarations.

## 10.2 How VibeMDM serves DDM

Every MDM-enrolled Mac on macOS 13 or later receives a declaration set built for it:

| Identifier | Type | Purpose |
| --- | --- | --- |
| `com.vibemdm.ddm.status-subscriptions` | `com.apple.configuration.management.status-subscriptions` | Subscribes to status items |
| `com.vibemdm.ddm.activation.status` | `com.apple.activation.simple` | Activates the subscriptions |
| `com.vibemdm.ddm.softwareupdate.enforcement` | enforcement | Only if an enforcement plan applies |
| `com.vibemdm.ddm.activation.softwareupdate.enforcement` | activation | Activates it |
| `com.vibemdm.ddm.softwareupdate.settings` | settings | Only if a settings plan with at least one value applies |
| `com.vibemdm.ddm.activation.softwareupdate.settings` | activation | Activates it |

Status items VibeMDM subscribes to: `device.operating-system.version`, `device.operating-system.build-version`,
`device.model.identifier`, `softwareupdate.install-state`, `softwareupdate.pending-version`,
`softwareupdate.failure-reason`, `softwareupdate.install-reason`.

Each declaration's **server token** is a hash of its type and payload, so it changes exactly when the content
changes. The **declarations token** identifies the whole set.

### The sync, step by step

1. VibeMDM queues the MDM command `DeclarativeManagement` carrying the `SyncTokens` (`DeclarationsToken`,
   `Timestamp`). This enables DDM on the first call and asks for a re-sync afterwards.
2. The Mac sends `DeclarativeManagement` check-ins to `/mdm`:
   - `Endpoint = tokens` → `{"SyncTokens": {"DeclarationsToken": "…", "Timestamp": "…"}}`
   - `Endpoint = declaration-items` → the manifest: `Declarations.{Activations, Configurations, Assets, Management}`,
     each a list of `{Identifier, ServerToken}`
   - `Endpoint = declaration/<kind>/<identifier>` → each declaration it does not have (with the same server token)
   - `Endpoint = status` (with `Data`) → a JSON status report: the subscribed items, plus
     `management.declarations` saying whether each declaration is **valid** and **active**, and any errors.
3. VibeMDM records the served token, the time of the last sync and the latest status.

Per-Mac view: `GET /api/v1/computers/{id}/ddm` shows the declarations exactly as served, the token, what the device
last fetched and its latest status reports. `POST /api/v1/computers/{id}/ddm/sync` forces a sync.

### Status and the overview

`GET /api/v1/updates/overview` (the **Overview** tab) lists every Mac:

| `state` | Meaning |
| --- | --- |
| `up_to_date` | An enforcement plan applies and the Mac's OS version is at or above the target |
| `pending` | An enforcement plan applies and the Mac is below the target |
| `failed` | The Mac reported a `softwareupdate.failure-reason` |
| `not_targeted` | DDM works on the Mac but no enforcement plan applies |
| `no_ddm` | Not MDM-enrolled, or the Mac has never sent a DDM status report |
| `unsupported` | macOS older than 14 (enforcement needs 14+) |

Rows also show `install_state`, `pending_version`, `failure_reason`, `declarations_in_sync` and `last_status_at`.

Related Health checks: **Invalid declarations** (a Mac reports a declaration invalid, with Apple's reason) and
**Declarations not syncing** (asked over 6 hours ago and not fetched; auto-heal re-sends the sync).

## 10.3 Important: supervision and DDM on macOS

Apple's declaration schema lists the macOS software-update declarations as allowed on **supervised** enrollments.
The schema doesn't define "supervised" for macOS, and the same label appears on declarations that work on any
MDM-enrolled Mac (passcode settings, status subscriptions). It most likely means "device enrollment" rather than
strictly "ADE", but **this needs confirming on a real Mac enrolled without ABM**. You don't have to guess: each
Mac's status report says whether the enforcement declaration is `valid` and `active`, and the console shows it per
Mac. If it is invalid, the **Invalid declarations** health finding quotes the reason.

## 10.4 The macOS version catalog (SOFA)

`GET /api/v1/updates/catalog` returns the latest macOS versions and builds from the Mac Admins
[SOFA feed](https://sofa.macadmins.io) (`https://sofafeed.macadmins.io/v1/macos_data_feed.json`). The server caches
it for 6 hours and backs off for 5 minutes after a failed fetch (HTTP 502 while nothing is cached). The plan
editor uses it to pick versions, builds and to see security release notes.

## 10.5 super (S.U.P.E.R.M.A.N.)

[super](https://github.com/Macjutsu/super) by Kevin M. White (Apache-2.0) is a popular open-source update workflow.
VibeMDM deploys it natively:

| Item | Value |
| --- | --- |
| Pinned version | **5.1.2** (6.x, pkg-only for Apple silicon on macOS 15+, was in beta when this was written) |
| Download URL | `https://raw.githubusercontent.com/Macjutsu/super/v5.1.2/super` |
| SHA-256 | `a8ac2a725c7b1402b23e835dc507eb9a37a3d67c20d25abab31b4f6ab221c2d6` |
| Preferences profile | `com.vibemdm.super` (MCX, domain `com.macjutsu.super`), built from the `builtin:com.macjutsu.super` schema |
| Status source | `/Library/Management/super/com.macjutsu.super.plist` (5.x) or `/Library/Preferences/com.macjutsu.super.plist` (6.x) |

The console's **Software Updates → super** tab shows the configuration form, the pinned version and a per-Mac status
table; it uses the API described below.

### Configure super

`PUT /api/v1/updates/super`:

```json
{
  "enabled": true,
  "scope": { "group_ids": ["<pilot>"] },
  "auth_mode": "local",
  "auth_account": "itadmin",
  "auth_password": "…",
  "values": {
    "DeadlineDaysSoft": "3",
    "DeadlineDaysHard": "7",
    "DeferralTimerMenu": "60,240,1440",
    "ScheduleWorkflowActive": "MON:09:00-17:00,TUE:09:00-17:00,WED:09:00-17:00,THU:09:00-17:00,FRI:09:00-17:00",
    "InstallMacOSMajorUpgrades": false,
    "VerboseMode": true
  }
}
```

| Field | Notes |
| --- | --- |
| `enabled`, `scope` | Applies to both the install and the preferences profile |
| `auth_mode` | `none`, `local` or `service` |
| `auth_account` | Required for `local` and `service` |
| `auth_password` | Omit to keep the stored password; `""` clears it. Stored sealed (when the encryption key is set) and **never** put in the profile |
| `values` | Validated against the super schema; empty values are dropped |

Saving regenerates the preferences profile, syncs it to the scope and starts a reconcile.

### Authentication modes

On Apple silicon, installing macOS updates from a script needs the credentials of a **volume owner** (an account
with a Secure Token), or an MDM-driven install authorized by the bootstrap token. super offers several ways to
authenticate; VibeMDM maps them as follows:

| `auth_mode` | What VibeMDM passes to super at install | Use |
| --- | --- | --- |
| `none` | nothing | super falls back to asking the user (see `AuthMDMFailoverToUser`) or to workflows that don't need credentials |
| `local` | `--auth-local-account=<account> --auth-local-password=<password>` | A local admin/volume-owner account that super stores and uses |
| `service` | `--auth-service-account=<account> --auth-service-password=<password>` | super's dedicated service account |

> **Verify before production:** super 5.1's documentation should be checked for the exact semantics of these
> options, especially the `service` mode (super's documented flow for *creating* a service account may need an
> additional admin credential such as `--auth-service-add-via-admin-account`). Test on a pilot Mac and read
> `/Library/Management/super/logs/` after the first run.

> **Security note:** the password reaches the Mac in the `install_super` task, is removed from the task record when
> the task completes, expires or is cancelled, and is redacted from the task output and from viewers. On the Mac it
> is passed to super as a **command-line argument**, so for the few seconds of the install it is visible to other
> local users in `ps`. Use a dedicated account with a unique password.

### Install and status

- **Automatic:** every 30 minutes the super reconciler queues an `install_super` task on each in-scope Mac with the
  agent whose reported super version is not 5.1.2, at most once a day per Mac.
- **On demand:** `POST /api/v1/updates/super/install` `{"computer_ids": [...]}`; an empty list runs the reconcile now.
- The agent downloads the script (up to 16 MB), **verifies the SHA-256** and refuses to run on a mismatch, then runs
  `/bin/bash super [auth options]` as root, which makes super install itself and start its workflow. Then it
  submits inventory.
- `GET /api/v1/updates/super/status` returns `summary` (`targeted`, `current`, `outdated`, `missing`) and a row per
  Mac with its super status: `SuperVersion`, `SuperStatus`, `WorkflowTarget`, `WorkflowScheduledInstall`,
  `NextAutoLaunch`, deadline counters and dates, `LastSuccessfulCheckDate`, downloaded update labels, and which auth
  accounts are configured.
- `GET /api/v1/updates/super` returns the configuration, the stored values, the pinned version, URL, checksum and
  license.

### super preference groups (schema)

| Group | Keys |
| --- | --- |
| What to install | `InstallMacOSMajorUpgrades`, `InstallMacOSMajorVersionTarget`, `InstallNonSystemUpdatesWithoutRestarting`, `InstallRapidSecurityResponses` |
| Workflow | `WorkflowInstallNow`, `WorkflowOnlyDownload`, `WorkflowRequireActiveUser`, `WorkflowRestartWithoutUpdates`, `WorkflowDisableUpdateCheck`, `WorkflowDisableRelaunch` |
| Deferrals | `DeferralTimerDefault`, `DeferralTimerMenu`, `DeferralTimerFocus`, `DeferralTimerError`, `DeferralTimerWorkflowRelaunch` |
| Schedule | `ScheduleWorkflowActive`, `ScheduleDeferredStartDays`, `ScheduleZeroDateManual`, `ScheduledInstallDays`, `ScheduledInstallDate`, `ScheduledInstallUserChoice`, `ScheduledInstallReminder` |
| Deadlines | `DeadlineCountFocus`, `DeadlineCountSoft`, `DeadlineCountHard`, `DeadlineDaysFocus`, `DeadlineDaysSoft`, `DeadlineDaysHard`, `DeadlineDateFocus`, `DeadlineDateSoft`, `DeadlineDateHard` |
| Dialogs | `DialogTimeoutDefault`, `DialogTimeoutUserAuth`, `DialogTimeoutSoftDeadline`, `DisplayIconFile`, `DisplaySilently`, `DisplayHelpMessage` |
| Auth and testing | `AuthMDMFailoverToUser`, `TestMode`, `TestModeTimeout`, `VerboseMode` |

super stores numbers as text, so numeric values are strings in the schema (`"3"`, not `3`). Credentials are never
set by profile.

## 10.6 Legacy MDM software-update commands

Available from a computer's Management tab or `POST /api/v1/computers/{id}/commands`:

| `type` | Fields | Notes |
| --- | --- | --- |
| `AvailableOSUpdates` | — | Lists updates the Mac sees (`ProductKey`, `HumanReadableName`, `Version`, `IsCritical`, …) |
| `ScheduleOSUpdate` | `product_key`, `install_action` | `install_action` such as `Default`, `DownloadOnly`, `InstallASAP`, `NotifyOnly`, `InstallLater`, `InstallForceRestart` |

## 10.7 Which one should I use?

| Situation | Recommendation |
| --- | --- |
| macOS 14+ and you want "everyone on X by date Y" | DDM enforcement plan |
| macOS 15+ and you want deferrals or automatic-install behaviour | DDM settings plan |
| You need maintenance windows, deferral menus, Focus awareness, or user-scheduled installs | super, with or without DDM |
| A one-off "what does this Mac see?" | `AvailableOSUpdates` |
| macOS 13 or earlier | super, or `ScheduleOSUpdate` |

Avoid running two enforcement mechanisms with different targets on the same Mac. If you use super and a DDM
enforcement plan together, give them the same target and make the DDM deadline the backstop (later than super's
hard deadline).

## 10.8 Study notes

> **Study note (DDM concepts):**
> - DDM (introduced with iOS 15 and macOS 13 Ventura) is **device-driven**: the server publishes the desired state
>   and the device pulls it, applies it on its own and reports back. Classic MDM commands are server-queued and
>   pulled on push.
> - **Declaration types:** *configurations* (settings), *assets* (reference data, e.g. credentials or files used by
>   configurations), *activations* (which configurations apply, optionally with predicates), *management*
>   (server capabilities, organization information, properties).
> - The **status channel** lets the device report changes without polling. A server *subscribes* to status items
>   and receives reports when values change. It replaces many `DeviceInformation` queries.
> - **Server tokens** let the device skip declarations it already has. The **DeclarationsToken** lets it skip a
>   whole sync when nothing changed.
> - DDM is enabled and re-synced with the MDM command `DeclarativeManagement`. The DDM traffic itself travels as
>   `DeclarativeManagement` check-in messages.

> **Study note (software update management):**
> - Apple has deprecated the MDM software-update commands and the related restrictions (deferral keys in
>   `com.apple.applicationaccess`, the `com.apple.SoftwareUpdate` payload) in favour of DDM software-update
>   declarations, and has said they will stop working in a future release. Check Apple's current documentation for
>   which macOS version removes them.
> - **Enforcement** declarations (macOS 14+) specify `TargetOSVersion`, optional `TargetBuildVersion`,
>   `TargetLocalDateTime` and optional `DetailsURL`. macOS notifies the user as the deadline nears and installs at the
>   deadline. The exact reminder schedule is defined by Apple and may change between releases.
> - **Deferrals** delay when an update becomes visible (up to 90 days), separately for major, minor and
>   non-OS ("system") updates.
> - On **Apple silicon**, installing an update requires authorization by a volume owner, or by the MDM through the
>   bootstrap token. This is why scripted tools such as super need credentials, and why bootstrap-token escrow
>   matters.
> - Local tools: `softwareupdate --list`, `softwareupdate --download`, `softwareupdate --install --all --restart`
>   (on Apple silicon `--user` and `--stdinpass` provide credentials), `softwareupdate --history`. System Settings →
>   General → Software Update shows deferral and managed state.
> - **Rapid Security Responses** delivered urgent security fixes without a full update (macOS 13+). Apple's naming
>   and mechanism for these background security fixes have evolved since; check current documentation.

Next: [Chapter 11 — FileVault](11-filevault.md).
