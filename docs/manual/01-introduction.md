# 1. Introduction

VibeMDM is a self-hosted Apple mobile device management (MDM) server for Macs. It runs as a Docker container, and
its only external dependency is PostgreSQL. It combines three things:

1. **An Apple MDM server.** It is built on the open-source [NanoMDM](https://github.com/micromdm/nanomdm) protocol
   engine. It speaks Apple's MDM check-in and command protocol, includes Declarative Device Management (DDM),
   sends APNs pushes and runs its own SCEP certificate authority.
2. **A macOS agent** (`vibemdm-agent`). It does what MDM cannot: runs scripts as root, installs packages, collects
   detailed inventory, runs Installomator, scans compliance baselines, drives swiftDialog onboarding and relays a
   remote shell.
3. **A web console and REST API.** The console was inspired by Jamf Pro's workflow (policies, smart groups,
   extension attributes, Self Service, scope with exclusions). It adds a health engine, a command inspector, mSCP
   compliance, FileVault escrow, logging, and an MCP server for AI agents. The console follows the operating
   system's light or dark appearance (live, including macOS *Auto*); the button at the bottom of the sidebar
   cycles **System → Light → Dark**, and the choice is remembered per browser.

VibeMDM is an independent implementation. It contains no Jamf code or assets.

## 1.1 What VibeMDM is (and is not)

| VibeMDM does | VibeMDM does not (yet) |
| --- | --- |
| User-approved MDM enrollment of Macs by profile | Automated Device Enrollment (needs Apple Business Manager) |
| Supervised enrollment of iPhone and iPad through Apple Configurator | iOS/iPadOS inventory views, apps or agent features |
| Configuration profiles, MDM commands, DDM software-update declarations | Apps and Books (VPP) app licensing |
| Scripts, packages, policies and Self Service through the agent | Package signing or a notarized agent out of the box |
| FileVault PRK escrow, compliance scanning, health checks | User-channel (per-user) MDM commands and profiles |

## 1.2 Architecture

```
                         ┌──────────────────────────┐
                         │   Apple Push Notification │
                         │   service (APNs)          │
                         └────────▲────────┬─────────┘
               HTTP/2 push        │        │  persistent connection
               (topic + token)    │        │  (TCP 5223 or 443)
                                  │        ▼
┌─────────────────────────────────┴───┐  ┌──────────────────────────────────────────┐
│ vibemdm (one Go binary, port 8080)  │  │ Mac                                       │
│                                     │  │                                           │
│  /mdm        NanoMDM check-in and   │◀─┤  mdmclient ── MDM protocol (HTTPS, plist) │
│              command channel + DDM  │  │    • device identity from SCEP            │
│  /scep       built-in SCEP CA       │◀─┤    • Mdm-Signature on every request       │
│  /enroll     enrollment page and    │  │                                           │
│              profile download       │  │                                           │
│  /agent/v1   agent API (long-poll)  │◀─┤  vibemdm-agent (LaunchDaemon, root)       │
│  /agent/install.sh  installer       │  │    • bearer device token (vmda_…)         │
│  /api/v1     admin REST API         │  │    • scripts, packages, recon, Installo-  │
│  /mcp        MCP server (optional)  │  │      mator, compliance, swiftDialog,      │
│  /           React web console      │  │      Self Service (127.0.0.1:8571), shell │
│  /healthz    liveness (DB ping)     │  └──────────────────────────────────────────┘
│                                     │
│  background: health engine, DDM     │  ┌──────────────┐   ┌──────────────────────┐
│  reconcile, compliance scheduler,   │──│ PostgreSQL   │   │ /data volume          │
│  super reconcile, FileVault sweep,  │  │ NanoMDM +    │   │ packages, log         │
│  log maintenance, webhooks          │  │ VibeMDM data │   │ archives, tmp         │
└──────────────────▲──────────────────┘  └──────────────┘   └──────────────────────┘
                   │ HTTPS (TLS terminated by your reverse proxy)
           ┌───────┴────────┐
           │ Browser / API  │
           │ client / AI    │
           └────────────────┘
```

### Components

| Component | What it is | Where it lives |
| --- | --- | --- |
| `vibemdm` | The server: NanoMDM core, SCEP CA, enrollment profile generator, DDM server, agent API, admin API, MCP server, embedded console | `cmd/vibemdm`, `internal/*` |
| PostgreSQL | All state: NanoMDM's tables (`devices`, `enrollments`, `commands`, `command_results`, `enrollment_queue`, `push_certs`, …) and VibeMDM's (`computers`, `policies`, `config_profiles`, `health_findings`, `compliance_*`, …) | `internal/db/migrations` |
| `/assets` folder | Uploaded packages, the agent package, Mac App icons | Host folder bind mount (`VIBEMDM_ASSETS_PATH`) |
| `/data` volume | Log archives, diagnostics, `initial-admin-password`, temporary files | Docker volume `vibemdm-data` |
| Reverse proxy (yours) | TLS termination, for example Nginx Proxy Manager with a Let's Encrypt certificate | Outside the compose stack; forwards to host port `13031` |
| `vibemdm-agent` | Static Go binary for macOS (arm64 and amd64) | `/usr/local/bin/vibemdm-agent` on the Mac |
| `vibemdm-sim` | A fake-Mac simulator that performs real SCEP and MDM enrollment and acts as an agent | `cmd/vibemdm-sim` |
| `mscp-import` | Developer tool that converts an mSCP checkout into the embedded compliance catalog | `cmd/mscp-import` |

## 1.3 How the pieces talk

VibeMDM uses three channels to reach a Mac. Knowing which one a feature uses is the key to troubleshooting it.

### The MDM channel (Apple's protocol)

The **check-in** protocol carries enrollment life-cycle messages: `Authenticate`, `TokenUpdate`, `CheckOut`,
plus `SetBootstrapToken`/`GetBootstrapToken`, `DeclarativeManagement` and `GetToken`. The Mac sends them to
the `CheckInURL` in its enrollment profile.

The **command** protocol is a pull loop:

1. The server queues a command and sends an APNs push that contains only the device's `PushMagic` string.
2. The Mac connects to the `ServerURL` and reports `Status: Idle`.
3. The server answers with the next queued command (a plist with `CommandUUID` and `Command.RequestType`).
4. The Mac runs it and reports `Acknowledged`, `Error`, `CommandFormatError` or `NotNow` in its next request.
   The server answers with the next command, or with an empty body when the queue is empty.

In VibeMDM both URLs point to `/mdm`. NanoMDM tells check-in messages from command reports by content type
(`application/x-apple-aspen-mdm-checkin` versus `application/x-apple-aspen-mdm`).

Every request is signed with the device identity certificate that SCEP issued (`SignMessage = true`, header
`Mdm-Signature`). VibeMDM verifies the certificate against its CA, and NanoMDM's `certauth` binds each
enrollment to its certificate, so one device cannot impersonate another.

**Features that use it:** profile install and removal, lock, erase, restart, shut down, remote desktop, OS update
commands, MDM inventory (`DeviceInformation`, `SecurityInfo`, `ProfileList`, `InstalledApplicationList`,
`CertificateList`), FileVault key escrow, bootstrap token escrow, DDM.

### The DDM channel (inside MDM)

Declarative Device Management runs over the same `/mdm` endpoint, as `DeclarativeManagement` check-in messages
with an `Endpoint` (`tokens`, `declaration-items`, `declaration/<kind>/<id>`, `status`). The server enables or
re-syncs DDM with the `DeclarativeManagement` MDM command. The device then pulls the declarations it lacks and
reports status asynchronously. VibeMDM uses DDM for status subscriptions and software-update enforcement and
settings ([Chapter 10](10-software-updates.md)).

### The agent channel (VibeMDM's own)

The agent talks HTTPS to `/agent/v1` with a per-device bearer token:

- **Check-in** (`POST /agent/v1/checkin`) every *N* seconds (default 300, ±10 % jitter). The response lists
  the policies due for this trigger, queued tasks, Installomator app installs, restricted-software rules and
  whether inventory is due.
- **Long-poll** (`GET /agent/v1/tasks?wait=55`) keeps a request open for up to 55 seconds. When an admin clicks
  *Run script now* or *Check in now*, the server's notifier answers the waiting request at once. Actions
  therefore take about a second while the Mac is online, with no APNs involved.
- **Results** (`POST /agent/v1/tasks/{id}/result`, `/policy-logs/{id}`, `/inventory`, `/logs`, …). If the server
  is unreachable, results are spooled to disk and replayed later.

**Features that use it:** scripts, packages, policies, Self Service, extension attributes, detailed inventory,
Installomator, compliance scans, super installation, onboarding, diagnostics, log shipping, restricted
software, remote shell.

> **Study note:** MDM is *server-queued and device-pulled*. APNs never carries a command, only a wake-up. If the
> Mac cannot reach APNs (TCP 5223 to Apple's 17.0.0.0/8 network, with a fallback to 443), commands wait in the
> queue until the device checks in for another reason. An agent with its own outbound channel is how products
> such as Jamf (the `jamf` binary) and VibeMDM run scripts. Apple's MDM protocol has no "run this shell script"
> command for macOS.

### Which channel does a console action use?

| Console action | Channel | Typical latency |
| --- | --- | --- |
| Install/remove a profile, lock, erase, restart (MDM) | MDM command + APNs push | seconds |
| Update inventory | Both: MDM inventory commands **and** an agent `inventory` task | seconds |
| Run script, run policy, run command, check in now | Agent long-poll | about 1 s while online |
| Save, enable or flush a policy | Agents in scope are asked to check in (long-poll `checkin` flag) | about 1 s |
| Software update plan | DDM declarations; a `DeclarativeManagement` command triggers the sync | minutes (device-driven) |
| Compliance scan, super install, onboarding, Escrow Buddy | Agent task | about 1 s to queue; run time varies |
| Remote shell | Agent dials back over WebSocket | about 1 s |

## 1.4 The life of a Mac in VibeMDM

1. **Enrollment.** The user installs the enrollment profile. SCEP issues a device identity. The Mac sends
   `Authenticate`, then `TokenUpdate`, and a `computers` row is created and marked MDM-enrolled.
2. **Initial work.** About 2 seconds after the first `TokenUpdate`, VibeMDM queues inventory commands, installs
   every in-scope configuration profile and sends a `DeclarativeManagement` command to enable DDM.
3. **Agent enrollment.** The agent installer enrolls with an invitation token or the enrollment secret. It is
   matched to the same `computers` row by hardware UUID (= MDM UDID) or serial number.
4. **First check-in.** The `enrollment_complete` trigger fires once, inventory is requested, and onboarding
   starts if a workflow is in scope and a real user is logged in.
5. **Steady state.** Periodic check-ins run policies; the health engine, DDM reconciler, compliance scheduler and
   super reconciler run in the background.
6. **Removal.** Removing the MDM profile sends `CheckOut` (the profile sets `CheckOutWhenRemoved = true`).
   VibeMDM records the unenrollment, and the agent keeps reporting until it is removed with
   `vibemdm-agent remove-framework`.

## 1.5 Background jobs

The server runs a one-minute housekeeping ticker. Approximate schedules:

| Job | Frequency | What it does |
| --- | --- | --- |
| Health engine | every 5 min (first run 20 s after start) | Runs all enabled checks ([Chapter 15](15-health.md)) |
| FileVault key sweep | every 10 min | Sends `SecurityInfo` to Macs with FileVault on and no escrowed key, at most every 4 h per Mac |
| DDM reconcile | every 30 min | Sends a sync to Macs whose declaration set changed (macOS 13+, re-asks at most every 6 h) |
| Compliance scheduler | every 10 min | Queues scans that are due per baseline interval |
| super reconcile | every 30 min | Queues super installs on in-scope Macs that lack the pinned version (once a day per Mac) |
| Hourly maintenance | every 60 min | Prunes SCEP challenges, resolved health findings (30 days), command events (90 days); archives and prunes logs |
| Expiry | every minute | Expires stale policy logs, tasks, shell sessions, app installs and sessions |

## 1.6 Glossary

| Term | Meaning |
| --- | --- |
| **ABM / ASM** | Apple Business Manager / Apple School Manager. Apple's portals for device assignment (ADE), Apps and Books, and Managed Apple Accounts |
| **ADE** | Automated Device Enrollment (formerly DEP). Zero-touch, supervised enrollment of devices assigned in ABM/ASM |
| **Activation** | DDM declaration (`com.apple.activation.simple`) that turns configurations on |
| **Activation Lock** | Anti-theft lock tied to an Apple Account. On Macs it needs Apple silicon or a T2 chip |
| **Agent** | `vibemdm-agent`, the LaunchDaemon on each Mac |
| **APNs** | Apple Push Notification service, which wakes devices so they contact the MDM server |
| **Asset (DDM)** | A DDM declaration that carries reference data (credentials, files) for configurations |
| **Bootstrap token** | Token the Mac escrows with MDM (macOS 10.15+). Used to grant Secure Token and, on Apple silicon, to authorize some operations |
| **CheckOut** | Check-in message a Mac sends when the MDM profile is removed |
| **Configuration profile** | `.mobileconfig` plist (`PayloadType = Configuration`) containing payloads |
| **Custom trigger** | A named policy trigger fired with `vibemdm-agent policy --event NAME` |
| **DDM** | Declarative Device Management: device-driven, state-based management with a status channel |
| **Declaration** | DDM object: configuration, asset, activation or management |
| **Extension attribute (EA)** | Custom inventory field, usually filled by an agent script |
| **Escrow Buddy** | Open-source authorization plugin (Netflix, now macadmins) that generates a new FileVault PRK at login |
| **Finding** | One Health problem with one object |
| **Installomator** | Open-source script that downloads, verifies and installs apps from vendor sources |
| **IRK / PRK** | Institutional / personal FileVault recovery key |
| **MCX** | Managed Client for macOS. Managed preferences (`com.apple.ManagedClient.preferences`) |
| **mSCP** | macOS Security Compliance Project (NIST and partners) |
| **NotNow** | Command status: the device is busy or locked; the command stays queued |
| **PPPC / TCC** | Privacy Preferences Policy Control payload / Transparency, Consent and Control |
| **PushMagic** | Random string the device gives in `TokenUpdate`; the only content of an MDM push |
| **SCEP** | Simple Certificate Enrollment Protocol; issues each device's identity certificate |
| **Scope** | Targets, limitations and exclusions that decide which Macs an object applies to |
| **Secure Token** | macOS account attribute that allows an account to unlock FileVault (APFS) |
| **Self Service** | Local portal (`http://127.0.0.1:8571`) where users run policies and install apps |
| **Smart group** | Group whose membership is computed from criteria |
| **SOFA** | Mac Admins' Simple Organized Feed for Apple software updates |
| **super** | S.U.P.E.R.M.A.N., an open-source macOS update workflow by Kevin M. White |
| **Supervision** | Device state that unlocks additional management. Mac: ADE only. iPhone/iPad: ADE or Apple Configurator |
| **swiftDialog** | Open-source dialog tool used for onboarding windows |
| **Topic** | APNs topic in the push certificate, `com.apple.mgmt.External.<UUID>` |
| **UAMDM** | User Approved MDM: the user approved the enrollment profile in System Settings |
| **UDID** | Device identifier. On Macs it is the hardware UUID |
| **XCreds** | Twocanoes' open-source login window that signs in with a cloud identity provider (OIDC) |

## 1.7 Working in the console

Every object (policies, configuration profiles, scripts, groups, searches, extension attributes, Mac Apps, baselines,
update plans, …) is created, edited and deleted the same way, as in Jamf Pro:

- **Opening an object shows it read-only.** The bar at the bottom right holds **History** (who changed it, from the
  audit log; administrators only), **Clone**, **Delete**, object-specific actions such as **Run now**, **Done** (back
  to the list) and **Edit**.
- **Edit** unlocks the whole object, every tab included. The bar then shows **Cancel** and **Save**. There is one
  **Save** for the whole object; it keeps you on the object and returns to view mode. **Cancel** puts back the saved
  values (it asks first when you changed something).
- **Lists** all work the same way: one **New** button, search, a few filters, sortable columns, and a click on a row
  opens the object. Select rows (the checkboxes) for bulk actions such as **Delete**, **Enable** and **Disable**; there
  are no Delete buttons on individual rows.
- **New** on a list page opens an empty object in edit mode; **Save** creates it, **Cancel** returns to the list.
- **Saved:** a successful save turns the Save button green with a check ("Saved"), sweeps a line across the bar and
  briefly outlines the page's cards in green as the object locks back to read-only; a notification confirms it at
  the top right. A failed save leaves you in edit mode with the reason. With *Reduce motion* turned on in the
  operating system, the check and notification appear without animation.
- The first tab of an object has a **Summary** column: status, scope, deployment counts and when it last changed.
- **Delete** asks for a second click (**Delete** again) in the same bar.
- **Clone** opens a new, unsaved copy named "Copy of …"; nothing exists until you save it.
- Leaving a page (or closing the tab) with unsaved changes asks whether to discard them.
- Settings pages work the same way (Edit, then one Save for the page). A device record is edited per section, as in
  Jamf: **Edit** on the *User and Location* or *Extension Attributes* card, then **Save** or **Cancel** on that card.
- Small forms in dialogs (users, webhooks, network segments, …) keep **Cancel** and **Save** at the bottom of the
  dialog.

Viewers see objects read-only and get no Edit, Delete or Clone.

### Keyboard shortcuts

The console uses Jamf Pro's shortcuts. The modifier is **Control** on a Mac (so Command-C, -V and -S keep their usual
meaning; Command-S also saves) and **Alt** on Windows and Linux. Press **?** for the list, or choose
**Keyboard shortcuts** in the account menu.

| Shortcut (Mac / Windows) | Action |
| --- | --- |
| ⌃N / Alt+N | New (on a list page) |
| ⌃E / Alt+E | Edit |
| ⌃S / Alt+S (and ⌘S on a Mac) | Save |
| ⌃C / Alt+C | Cancel |
| ⌃D / Alt+D | Delete (press again to confirm) |
| ⌃⇧N / Alt+Shift+N | Clone |
| ⌃H / Alt+H | History |
| ⌃L / Alt+L | Logs (an object's Logs tab) |
| ⌃V / Alt+V | View (a group's members, a search's results) |
| ⌃B / Alt+B | Back (Done in view mode) |
| ⌃1 / Alt+1 | Dashboard |
| ⌃2 / Alt+2 | Computers |
| ⌃3 / Alt+3 | Mobile Devices |
| ⌃5 / Alt+5 | Settings |
| ⌃9 / Alt+9 | Health (Jamf's Notifications) |
| ⌃⇧L / Alt+Shift+L | Server logs |

In a dialog the shortcuts act on the dialog (⌃S saves it, Esc closes it). While the cursor is in a text field only
Save works, so the field's own Control-key editing (⌃E end of line, ⌃D delete, … on a Mac) is left alone. Jamf's ⌃4
(Users) has no counterpart: VibeMDM keeps no separate user records.

Next: [Chapter 2 — Installation](02-installation.md).
