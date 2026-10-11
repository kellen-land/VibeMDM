# 16. MDM commands and the command inspector

MDM commands are Apple's server-to-device instructions. VibeMDM queues them in NanoMDM's queue, wakes the Mac with an
APNs push, records each step in the **command inspector**, and interprets the results into inventory and
deployment status.

## 16.1 Sending commands

From a computer's **Management** tab, or `POST /api/v1/computers/{id}/commands` with a `type` and its fields. The Mac
must be MDM-enrolled (otherwise HTTP 409). Every command is audited as `mdm.command`. The response includes the
`command_uuid` and any `push_error`.

| `type` | Fields | Notes |
| --- | --- | --- |
| `DeviceInformation` | — | Inventory queries (the list VibeMDM asks for is in [§5.2](05-inventory-and-groups.md#52-inventory-sources-mdm-vs-agent)) |
| `SecurityInfo` | — | FileVault, firewall, Secure Boot, SIP, bootstrap token; escrowed FileVault key |
| `ProfileList` | — | All installed profiles (`ManagedOnly = false`) |
| `InstalledApplicationList` | — | Installed apps (`ManagedAppsOnly = false`) |
| `CertificateList` | — | Installed certificates |
| `DeviceLock` | `pin` (**6 digits, required**), `message`, `phone` | Locks the Mac; the PIN unlocks it at the lock screen. Write the PIN down before sending |
| `EraseDevice` | `pin` (6 digits, optional) | Erases the Mac. On Macs that support *Erase All Content and Settings* the behaviour differs (see the study note). `DeviceInformation` reports `PINRequiredForEraseDevice` and `PINRequiredForDeviceLock` |
| `RestartDevice` | — | Restarts |
| `ShutDownDevice` | — | Shuts down |
| `EnableRemoteDesktop` / `DisableRemoteDesktop` | — | Turns Screen Sharing / Remote Management on or off |
| `InstallProfile` | `profile_id` | Installs a library profile (tracked as a deployment) |
| `RemoveProfile` | `profile_id` or `identifier` | Removes a library profile, or any profile by identifier |
| `RemoveMDMProfile` | — | Removes the enrollment profile (by `enrollment_profile_identifier`), which unenrolls the Mac |
| `InstallEnterpriseApplication` | `manifest_url` | Installs a signed distribution package from a manifest |
| `AvailableOSUpdates` | — | Lists available updates |
| `ScheduleOSUpdate` | `product_key`, `install_action` | Legacy update install ([Chapter 10](10-software-updates.md#106-legacy-mdm-software-update-commands)) |
| `SetRecoveryLock` | `current_password`, `new_password` | Sets or changes the Recovery Lock password (Apple silicon) |
| `DeviceConfigured` | — | Releases a device held in Await Configuration (only meaningful with ADE) |
| `Raw` | `plist` | Any command: a full command plist (`CommandUUID` is generated if missing) |

Other paths that queue commands: profile sync (`InstallProfile` / `RemoveProfile`), *Update inventory* (four
inventory commands), the FileVault sweep (`SecurityInfo`), DDM sync (`DeclarativeManagement`), Health remediations.
Each command has a **kind** recorded in `mdm_commands_meta` (`inventory`, `management`, `raw`, `profile_install`,
`ddm_sync`, `filevault_check`, …) and who created it.

### A Raw command example

```xml
<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
  <key>Command</key>
  <dict>
    <key>RequestType</key><string>DeviceInformation</string>
    <key>Queries</key><array><string>OSUpdateSettings</string><string>MDMOptions</string></array>
  </dict>
</dict></plist>
```

### Command lists and cancelling

| Endpoint | Purpose |
| --- | --- |
| `GET /api/v1/commands?computer_id=&status=&limit=&offset=` | All commands (status `Pending`, `Acknowledged`, `Error`, `NotNow`, `CommandFormatError`) |
| `GET /api/v1/commands/{uuid}` | One command with its request and result plists (redacted unless `?reveal=1`, admin) |
| `DELETE /api/v1/commands/{uuid}` | Cancels (deactivates) a command that has not been delivered; recorded as `cancelled` and audited |
| `GET /api/v1/computers/{id}/commands` | One Mac's commands |

Results are interpreted as they arrive: `DeviceInformation`, `SecurityInfo`, `ProfileList` and
`InstalledApplicationList` update inventory; `InstallProfile` and `RemoveProfile` update deployment status. The
`MDMCommandCompleted` webhook fires for each result.

> **Known quirk:** `MDMCommandCompleted` is currently fired **twice** for each command result (once with the
> `error` field, once without). Deduplicate on `command_uuid` in your receiver.

## 16.2 The command inspector

The inspector follows every command from queueing until the device answers, and explains each step in plain
language. Open it from any command (**Activity → Commands → a command**, route `/commands/{uuid}`).

| Step | Event | What happened |
| --- | --- | --- |
| 1 | `queued` | The server stored the command in the device's queue (NanoMDM `enrollment_queue`) |
| 2 | `push_sent` / `push_failed` | The server sent an APNs push containing only the device's *PushMagic*: "come and check in". `push_sent` records the APNs ID; `push_failed` the error |
| 3 | `delivered` | The device connected to `/mdm` (reporting `Idle` or a previous result) and the server answered with this command's plist |
| 4 | `acknowledged` / `error` / `not_now` | The device reported the result in its next request. `NotNow` means "busy or locked, try again later"; the command stays queued |
| — | `cancelled` | An admin cancelled the command before delivery |

A device keeps connecting until its queue is empty: each response carries the next command, and an empty response
ends the session. That is why several `delivered`/`acknowledged` pairs often appear within a second.

| Endpoint | Returns |
| --- | --- |
| `GET /api/v1/commands/{uuid}/inspect` | A one-line summary of what the command asks for, a summary of the result, the timeline, and the request/response plists |
| `GET /api/v1/computers/{id}/timeline` | Every command event for one Mac, newest first |
| `GET /api/v1/commands/search?q=&computer_id=&limit=` | Searches command and result plists |

Timeline events are kept for 90 days.

### Redaction

Sensitive values are redacted by default everywhere: `PIN`, `Password`, `NewPassword`, `CurrentPassword`,
`Payload` (profile contents), `FDE_PersonalRecoveryKeyCMS`, `PersonalRecoveryKey`, `RecoveryKey`, `BootstrapToken`,
`UnlockToken`, `Token`, `PushMagic`, `ChallengePassword`, `Challenge`, `Passcode`, `ActivationLockBypassCode`,
`EscrowKey`, `RotateRecoveryKey`, `SecurityToken`, `Identity`, `Data`, `Certificate`, `VPNPassword`, `ProxyPassword`,
`SharedSecret`, `AuthPassword`, `PrivateKey`, `Secret`. Placeholders note the original size, for example
`[redacted 6 bytes]`.

Admins can request raw payloads with `?reveal=1`; every reveal is audited as `mdm.command_reveal`. Viewers always
get redacted payloads. Search matches the redacted text, so it cannot be used to confirm a secret value.

## 16.3 Study notes: an InstallProfile round trip on the wire

This walk-through follows one profile install through every message. You can watch the real thing with
`VIBEMDM_DUMP_MDM=1` on the server, the inspector timeline, and `log stream --info --predicate 'process ==
"mdmclient"'` on the Mac.

> **Study note (step 0: enrollment already happened).** At enrollment the Mac sent two check-in messages (HTTP PUT
> to the `CheckInURL`, `Content-Type: application/x-apple-aspen-mdm-checkin`), each signed with its SCEP identity in
> the `Mdm-Signature` header:
>
> ```xml
> <dict>
>   <key>MessageType</key>  <string>Authenticate</string>
>   <key>Topic</key>        <string>com.apple.mgmt.External.4f2c…</string>
>   <key>UDID</key>         <string>5A000001-…</string>
>   <key>SerialNumber</key> <string>C02…</string>
>   <key>Model</key>        <string>Mac15,6</string>
>   <key>OSVersion</key>    <string>15.6.1</string>
> </dict>
> ```
>
> ```xml
> <dict>
>   <key>MessageType</key> <string>TokenUpdate</string>
>   <key>Topic</key>       <string>com.apple.mgmt.External.4f2c…</string>
>   <key>UDID</key>        <string>5A000001-…</string>
>   <key>Token</key>       <data>…32-byte APNs device token…</data>
>   <key>PushMagic</key>   <string>9E1C…</string>
> </dict>
> ```
>
> The server now knows how to push to the device (token + topic) and what to put in the push (PushMagic).
> `Authenticate` alone does not complete enrollment; `TokenUpdate` does. A Mac sends `TokenUpdate` again whenever its
> push token changes.

> **Study note (step 1: queue).** An admin installs a profile. VibeMDM builds the command and stores it in the queue
> (inspector: `queued`):
>
> ```xml
> <dict>
>   <key>CommandUUID</key> <string>0c6e3d0a-…</string>
>   <key>Command</key>
>   <dict>
>     <key>RequestType</key> <string>InstallProfile</string>
>     <key>Payload</key>     <data>PD94bWwg…(the .mobileconfig, base64)…</data>
>   </dict>
> </dict>
> ```

> **Study note (step 2: push).** VibeMDM sends an HTTP/2 request to APNs (`POST /3/device/<token>` on
> `api.push.apple.com`, authenticated with the push certificate, `apns-topic` set to the topic) with this body:
>
> ```json
> {"mdm": "9E1C…"}
> ```
>
> That's all: no command, no data. APNs delivers it over the Mac's persistent connection (inspector: `push_sent`).

> **Study note (step 3: the device asks for work).** `mdmclient` wakes up and sends its status to the `ServerURL`
> (`Content-Type: application/x-apple-aspen-mdm`):
>
> ```xml
> <dict>
>   <key>Status</key> <string>Idle</string>
>   <key>UDID</key>   <string>5A000001-…</string>
> </dict>
> ```
>
> The server answers with the next command in the queue: the plist from step 1 (inspector: `delivered`).

> **Study note (step 4: the result).** The Mac installs the profile and reports, again to the `ServerURL`:
>
> ```xml
> <dict>
>   <key>Status</key>      <string>Acknowledged</string>
>   <key>CommandUUID</key> <string>0c6e3d0a-…</string>
>   <key>UDID</key>        <string>5A000001-…</string>
> </dict>
> ```
>
> The server records the result (inspector: `acknowledged`; deployment status `installed`) and answers with the next
> command, or with an **empty body** when the queue is empty, which ends the conversation.
>
> If the profile was invalid, the Mac would answer instead:
>
> ```xml
> <dict>
>   <key>Status</key>      <string>Error</string>
>   <key>CommandUUID</key> <string>0c6e3d0a-…</string>
>   <key>ErrorChain</key>
>   <array><dict>
>     <key>ErrorCode</key>            <integer>4000</integer>
>     <key>ErrorDomain</key>          <string>MCProfileErrorDomain</string>
>     <key>LocalizedDescription</key> <string>The profile is not valid.</string>
>   </dict></array>
> </dict>
> ```
>
> (Error codes and domains come from the device; the example is the one VibeMDM's simulator returns.)

> **Study note (statuses).** A device answers each command with exactly one of: `Acknowledged` (done), `Error`
> (failed, with an `ErrorChain`), `CommandFormatError` (the plist was malformed), or `NotNow` (it can't run the command
> now, for example because the device is locked or busy; it will check in again later, and the command stays queued).
> `Idle` is not a result; it means "I have nothing to report, give me work".

> **Study note (command facts to know):**
> - `DeviceLock` on a Mac requires a 6-digit PIN; keep it, because it is the only way to unlock the Mac.
> - `EraseDevice`: on Macs with Apple silicon or a T2 chip running a recent macOS, the MDM erase can use **Erase All
>   Content and Settings**, which is fast and may not need a PIN; older Macs need a PIN and do a full erase. Check
>   Apple's documentation for the exact requirements of your macOS version.
> - `SetRecoveryLock` (Apple silicon) sets a password that protects recoveryOS, similar to a firmware password on Intel
>   Macs.
> - `EnableRemoteDesktop` turns on Remote Management (ARD) on macOS 10.14.4 and later.
> - `DeviceConfigured` is used with ADE's **Await Configuration** to release the device from Setup Assistant.
> - User-channel commands (to a user's own MDM channel) exist on macOS for user-level profiles and settings; VibeMDM
>   uses only the device channel.

Next: [Chapter 17 — Remote shell](17-remote-shell.md).
