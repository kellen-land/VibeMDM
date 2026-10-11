# Plan: iOS and iPadOS in VibeMDM

Status: planned (tracked in `TODO.md`). Goal: manage iPhone and iPad alongside Macs, with **Mobile Devices fully
separate from Computers, the way Jamf Pro does it**. Remote viewing is out of scope.

## Constraints

- **Test hardware:** an iPhone 13 mini and an iPad (8th generation). Both can be supervised with Apple Configurator
  (which erases them) from a Mac. Check that Apple Configurator on the Mac VM can see a USB-connected device; a
  physical Mac may be needed for Prepare.
- **No Apple Business Manager or Apple School Manager test account** (Apple declined the request). Everything that
  needs one is marked **Blocked: ABM/ASM** below: it can be built against Apple's documentation and the simulator,
  but stays untested and off by default until a real account is available. Never point a test build at the
  district's ASM without permission.
- No agent on iOS (Apple does not allow one): everything goes through MDM and DDM.
- Calm-UI rule: Mobile Devices is its own sidebar area, one primary action per screen, iOS-only options folded away.

## Where we start

- NanoMDM handles iOS check-ins unchanged; MDM commands, profiles, DDM, the command inspector and APNs work for any
  Apple device.
- **Apple Configurator enrollment** (`/enroll/configurator/<token>`) supervises and enrolls iPhone and iPad without
  ABM, so supervised features are testable at home.
- NanoMDM stores each device's **UnlockToken** (`devices.unlock_token`), which `ClearPasscode` needs.
- DDM software-update enforcement works on iOS/iPadOS 17+.
- Missing: iPads currently land in the `computers` table with the Mac UI. No iOS inventory, iOS commands, payload
  editors, app distribution or education features.

## Phase 0 — Mobile Devices as a separate world (Jamf-style)

Jamf Pro keeps two parallel trees: Computers and Mobile Devices each have their own inventory records, smart and
static groups, searches, configuration profiles, apps, extension attributes and enrollment settings. Mirror that:

1. **Data model:** a `mobile_devices` table (and `mobile_device_*` tables for groups, profiles and app assignments),
   separate from `computers`. Route a device by platform at Authenticate/TokenUpdate (`ProductName`, `Model`,
   `OSVersion`): Macs to `computers`, iPhone/iPad (and later Apple TV) to `mobile_devices`. A migration moves any
   iOS rows already in `computers`.
2. **API:** `/api/v1/mobile-devices…`, `/api/v1/mobile-device-groups…`, `/api/v1/mobile-device-profiles…`,
   `/api/v1/mobile-device-apps…`; MDM commands work for either device type. Go and TypeScript types for each.
3. **Console:** a **Mobile Devices** sidebar group (Mobile Devices, Mobile Device Groups, Searches, Configuration
   Profiles, Apps, Enrollment). The device record follows the computer record: *Inventory / Management / History*
   with a side list.
4. **Scoping and Health** for mobile objects use mobile groups only; nothing Mac-only (agent, policies, scripts, LAPS,
   FileVault, compliance) appears there.
5. **Simulator:** `vibemdm-sim -platform iphone|ipad` with iOS answers (supervised or not, passcode, Lost Mode, apps,
   UnlockToken) so every phase is built against it first.

## Phase 1 — Enrollment

| Method | Supervised | Testable now | Notes |
| --- | --- | --- | --- |
| Apple Configurator (exists) | Yes | **Yes** | Move to the mobile enrollment page; document Prepare step by step |
| Profile enrollment in Safari | No | **Yes** | Mobile enrollment page with a QR code to the profile URL; user installs it in Settings |
| Automated Device Enrollment via **NanoDEP** | Yes | **Blocked: ABM/ASM** | Token upload, device sync, enrollment profile (skip panes, support info, Await Configuration), `DeviceConfigured` |
| User Enrollment (BYOD) | No | Blocked (Managed Apple Accounts) | Later, if ever |

## Phase 2 — Inventory

Scheduled `DeviceInformation`, `SecurityInfo`, `InstalledApplicationList`, `ManagedApplicationList`, `ProfileList`,
`CertificateList`. Fields: IMEI/MEID, phone number, carrier and cellular details, battery, storage, supervised,
Activation Lock, Find My, Lost Mode, passcode present/compliant, hardware encryption, last backup. Mobile extension
attributes are server-side (computed from inventory).

## Phase 3 — Commands

Lock (message, phone), **Clear Passcode** (UnlockToken), Erase (Return to Service on 17+), Restart/Shut Down,
**Lost Mode** (enable, `DeviceLocation`, `PlayLostModeSound`, disable), **Settings** (device name, wallpaper,
Bluetooth, data roaming, personal hotspot, time zone), **Activation Lock bypass code** stored sealed and revealed with
an audited reason, `ClearRestrictionsPassword`, `RefreshCellularPlans`, OS updates through DDM plans. Every command
gets an inspector summary and a History entry. Most of these need supervision, which Configurator gives us.

## Phase 4 — Configuration profiles for mobile devices

Separate *Mobile Device Configuration Profiles* (own list and scope). GUI editors: Restrictions, Passcode, Wi-Fi, VPN,
Web Clips, Home Screen Layout, Notifications, Single App Mode / App Lock, Content Filter, Certificates, AirPrint,
Lock Screen message. DDM declarations for iOS 17+ where available.

## Phase 5 — Apps

- **Testable now:** App Store apps by Store ID with `InstallApplication`; without Apps and Books licences the user
  signs in with their own Apple Account to install. Track with `ManagedApplicationList`; remove; Managed App
  Configuration (AppConfig) with the JSON-schema form engine.
- **Blocked: ABM/ASM:** Apps and Books content token, licence sync, device-based licences (no Apple Account needed),
  licence and token-expiry Health checks.

## Phase 6 — Education

- **Partly testable:** Classes defined in VibeMDM and an Education configuration profile (`com.apple.education`) for
  Apple's Classroom app (teacher on the Mac, the iPad as a student device). Remote viewing through Classroom is out of
  scope; this is for class management only.
- **Blocked: ABM/ASM:** roster sync from Apple School Manager, Managed Apple Accounts, **Shared iPad** (needs ADE).

## Phase 7 — Hardening and docs

iOS Health checks (no passcode, not supervised, out-of-date iOS, Lost Mode on, Activation Lock without a bypass code),
store integration tests for the separate tables and scoping, a *Mobile Devices* manual chapter with ACSP/ACITP study
notes (supervision, Configurator, ADE, Apps and Books, Shared iPad, Classroom), and a real-device test pass on the
iPhone 13 mini and iPad (8th generation).

## Out of scope

Remote viewing or screen sharing (AirPlay mirroring on request, Classroom Screen View for helpdesk use, a ReplayKit
companion app). Apple TV, in-house `.ipa` distribution.
