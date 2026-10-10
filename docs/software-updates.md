# Software updates (Declarative Device Management)

Apple's direction is clear: from macOS 26, software updates can only be managed and enforced through **Declarative
Device Management (DDM)**, and the older MDM software-update commands are deprecated. VibeMDM includes a native DDM
server, so update plans are delivered as declarations rather than commands.

## Update plans

| Kind | Declaration | macOS | What it does |
| --- | --- | --- | --- |
| **Enforcement** | `com.apple.configuration.softwareupdate.enforcement.specific` | 14+ | Requires a target version (and optionally build) by a local date and time. macOS shows escalating reminders, downloads and installs, and restarts at the deadline. |
| **Settings** | `com.apple.configuration.softwareupdate.settings` | 15+ | Notifications, major/minor/system deferral days (1–90), automatic download / OS update / security update behavior, Rapid Security Response, standard-user updates. |

Each plan uses the standard scope (targets, limitations, exclusions). When several plans reach the same Mac, the
enforcement plan with the **highest target version** wins (ties: earliest deadline), and the most recently updated
settings plan wins.

The **version catalog** (`GET /api/v1/updates/catalog`) lists the latest macOS versions and builds from the community
[SOFA feed](https://sofa.macadmins.io), cached for six hours.

## How DDM works (the protocol)

1. The server sends the MDM command **`DeclarativeManagement`** with a sync token. That enables DDM on the device, or
   asks it to re-sync.
2. The device sends DeclarativeManagement **check-ins** to `/mdm`:
   - `tokens` returns the `DeclarationsToken`, which changes whenever the declaration set changes.
   - `declaration-items` returns the manifest: identifiers and server tokens for activations, configurations, assets
     and management declarations.
   - `declaration/<kind>/<identifier>` returns each declaration the device doesn't already have.
   - `status` is a JSON report from the device: subscribed status items, plus whether each declaration is **valid**
     and **active**.
3. Activations (`com.apple.activation.simple`) turn configurations on. VibeMDM pairs every configuration with one.

VibeMDM subscribes every Mac to OS version, build, model, and software update status
(`softwareupdate.install-state`, `pending-version`, `failure-reason`, `install-reason`). It re-checks every 30
minutes and after any plan change, then sends a sync only to Macs whose declaration set changed.

`GET /api/v1/computers/{id}/ddm` shows a Mac's declarations exactly as served, the token, what the device last
fetched, and its latest status reports.

## Important: supervision on macOS

Apple's schema lists the software update declarations for macOS as allowed on **`supervised`** enrollments. The schema
doesn't define that term for macOS, and the same label appears on declarations that work on any MDM-enrolled Mac
(passcode settings, status subscriptions). So it most likely means "device enrollment" rather than strictly
"Automated Device Enrollment". **This needs confirming on a real Mac enrolled without Apple Business Manager.** You
don't have to guess: each Mac's status report states whether the enforcement declaration is `valid` and `active`, and
the console shows that per Mac.

## S.U.P.E.R.M.A.N.

[super](https://github.com/Macjutsu/super) (Apache-2.0) adds richer user dialogs, deferrals and scheduling on top of
the update workflow. Version 6.x (beta at the time of writing) supports DDM-based enforcement on Apple silicon running
macOS 15+, while 5.1.x uses local authentication or Jamf-specific MDM workflows. The public documentation covers 5.1
only, so VibeMDM doesn't integrate super yet. Native DDM enforcement works without it, and super can be layered on
once its 6.x DDM behavior is documented.

## Study notes (Apple certification)

- DDM is **device-driven**: the server publishes state, and the device decides when to fetch it and reports back
  asynchronously. MDM commands, by contrast, are server-queued and pulled on push.
- The **status channel** replaces polling commands like `DeviceInformation` for many values.
- Server tokens let the device skip declarations it already has. The DeclarationsToken lets it skip the whole sync.
