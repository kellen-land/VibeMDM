# Mobile devices (iPhone and iPad)

VibeMDM keeps iPhones and iPads apart from Macs, the way Jamf Pro keeps **Computers** and **Mobile Devices** apart:
their own records, groups, searches and API. The console's sidebar has a **Mobile Devices** section (Mobile Devices,
Mobile Device Groups, Searches). Plan and remaining work: [plans/ios-ipados.md](plans/ios-ipados.md).

There is no agent on iOS and iPadOS: everything goes through MDM.

## Enrolling an iPhone or iPad

**Mobile Devices → Enrollment** shows both ways that work without Apple Business Manager:

| Method | Supervised | How |
| --- | --- | --- |
| Apple Configurator | Yes | On a Mac: Prepare → Manual Configuration → Supervise devices → Enroll in MDM Server → new server with the Configurator URL. **Erases the device.** Unlocks Lost Mode, the Activation Lock bypass code, silent app installs and most restrictions; the user cannot remove management |
| Safari | No | Scan the QR code or open `/enroll?device=mobile`, download the profile, then Settings → Profile Downloaded → Install. The user can remove it |

The public `/enroll` page shows iPhone and iPad steps to iOS Safari (and to `?device=mobile`, because iPadOS Safari
identifies as a Mac by default) and links there from the Mac steps. With open enrollment off, *Create a one-time
link* makes a single-use invitation valid for 7 days and shows it as a QR code (`GET /api/v1/qr?text=` renders an
SVG). The enrollment profile is the same for Macs and mobile devices.

## How a device becomes a mobile device

At MDM `Authenticate`, a device whose `ProductName` or `Model` starts with `iPhone`, `iPad`, `iPod` or `AppleTV` is
stored in `mobile_devices` instead of `computers`. `TokenUpdate`, `CheckOut` and command results are routed the same
way. Migration `0021` moves any iPhones and iPads that earlier versions filed under computers, including their MDM
command history.

Webhooks: `MobileDeviceEnrolled`, `MobileDeviceUnenrolled`; `MDMCommandCompleted` carries `mobile_device_id`.

## Inventory

There is no agent, so the server asks. Right after enrollment, and then whenever the last report is older than the
inventory interval (Settings → General, default 24 h; an offline device is asked again at most every 6 hours),
VibeMDM sends `DeviceInformation` (iOS queries), `SecurityInfo`, `ProfileList`, `InstalledApplicationList` and
`ManagedApplicationList`. The record shows identity, hardware (capacity, battery, MAC addresses), cellular details
(phone number, IMEI, carrier), security (passcode set and compliant, data protection, supervised, Activation Lock,
Find My, Lost Mode, last iCloud backup), apps with their managed state, installed profiles and groups.

## Extension attributes

There is no agent on iOS, so mobile extension attributes are entered by administrators: define them under **Settings →
Mobile device attributes** (text field or pop-up menu; string, integer or date) and fill them in on the device record
(*Inventory → Extension attributes*). They appear as `EA: …` fields in mobile groups and searches.
API: `GET/POST /api/v1/mobile-extension-attributes`, `PUT/DELETE /api/v1/mobile-extension-attributes/{id}`,
`PUT /api/v1/mobile-devices/{id}/ea-values/{ea}` `{"value": "…"}`.

## Groups and searches

Mobile device groups (smart or static) and searches use the same criteria builder as computers, with mobile fields
(device type, model, OS version, capacity, battery, phone number, IMEI, carrier, supervised, Shared iPad, passcode,
Lost Mode, Activation Lock, Find My, apps, profiles, user and location). Computer-only fields are rejected.

## Commands

*Management → Actions* on the device record. Administrators only; each command is queued through MDM, recorded in the
audit log (`mobile_device.command`) and shown under *History → Commands*. Commands marked **S** need a supervised
device (Apple Configurator or, later, Automated Device Enrollment); on an unsupervised device the row is greyed out
and the API answers 409.

| Action | MDM command | Notes |
| --- | --- | --- |
| Lock | `DeviceLock` | Optional message and phone number on the Lock Screen. |
| Lost Mode (S) | `EnableLostMode`, `DisableLostMode` | Needs a message or a phone number. While it is on: **Locate** (`DeviceLocation`, coordinates and accuracy shown with a map link) and **Play sound** (`PlayLostModeSound`). Location is only available in Lost Mode. |
| Rename (S) | `Settings` → `DeviceName` | |
| Clear passcode | `ClearPasscode` | Uses the UnlockToken the device sent at enrollment; refused if there is none. |
| Restart, Shut down (S) | `RestartDevice`, `ShutDownDevice` | |
| Erase | `EraseDevice` | You type the device name or serial number to confirm. Option to keep the eSIM data plan. The device leaves management. |
| Device settings | `Settings` | Bluetooth (S), data roaming, Personal Hotspot, time zone (S). |
| Refresh cellular plans | `RefreshCellularPlans` | Cellular devices only; needs the carrier's https eSIM server URL. |

After Lost Mode, Settings, Lock or Clear passcode, VibeMDM asks for `DeviceInformation` or `SecurityInfo` again so the
record catches up. Not yet available: wallpaper, Return to Service, OS updates and `ClearRestrictionsPassword`.

## Configuration profiles

*Mobile Devices → Configuration Profiles* is separate from computer profiles (its own list, scope and deployments), as
in Jamf Pro. **New profile** opens the editor: *General* (name, category, the description users see in Settings),
then **Add payload** to pick from the built-in editors:

The editors cover the payloads Jamf Pro offers for mobile devices (44 in all): General (Lock Screen Message, AirPrint,
AirPlay, Font, Notifications, Domains), Security (Passcode, Restrictions with every Apple key, Content Filter, Single App
Mode, Certificate Transparency and Revocation), Network (Wi-Fi including WPA2/WPA3 Enterprise, Ethernet, VPN, Per-App
VPN, Global HTTP Proxy, DNS Settings, DNS Proxy, Relay, Cellular, APN, Cellular Private Network, Network Usage Rules),
Certificates (Certificate, PEM, root, identity, SCEP, ACME), Accounts (Mail, Exchange ActiveSync, Google, LDAP, Calendar,
Contacts, Subscribed Calendars), Single Sign-On (SSO Extensions, Kerberos, legacy SSO), Home Screen Layout, Web Clips,
Skip Setup Items and Education. See [§8.7](manual/08-configuration-profiles.md#87-payload-editors-jamf-pro-payloads)
for how the editors are built.

Settings you leave *Not set* are omitted, so the device keeps Apple's default. Payloads that Apple allows more than
once (Wi-Fi, VPN, certificates, accounts, Web Clips, …) can be added several times. **Show profile XML** previews the generated `.mobileconfig`. **Upload** instead keeps a `.mobileconfig`
made elsewhere (Apple Configurator, iMazing Profile Editor) as XML.

Scope uses *Targets* (all mobile devices, devices, mobile device groups, buildings, departments) and *Exclusions*.
Saving installs the profile on devices in scope (`InstallProfile`) and removes it from devices that left
(`RemoveProfile`); changing the payload re-sends it everywhere. *Deployments* shows each device's status; **Sync scope**
re-checks group membership and **Redeploy** sends it again. A newly enrolled device receives every profile scoped to
it. Payloads marked **S** need a supervised device: an unsupervised one rejects the whole profile, and the device
record lists it under *Profiles → Waiting or failed* with the device's error. Passwords, shared secrets and identity
files are hidden from viewers.

Not yet available: DDM declarations for iOS 17+, enterprise Wi-Fi (802.1X), certificate references in VPN and Wi-Fi,
and per-profile removal from single devices. **Untested on real hardware:** every payload above.

## Activation Lock bypass code

For a supervised device, VibeMDM asks for `ActivationLockBypassCode` after inventory (at most once a day until it has
one), seals the code with the server's encryption key and removes it from the stored raw command result. The
*Security* section shows **Show code**: an administrator gives a reason, the reveal is audited
(`mobile_device.activation_lock_bypass_viewed`) and the code appears once with a copy button. Use it in the
Activation Lock screen after an erase (leave the Apple Account blank, enter the code as the password). A device may
not return a code (for example if Activation Lock was already turned on by the user before supervision); **Collect
now** asks again.

## API

| Method and path | Purpose |
| --- | --- |
| `GET /api/v1/mobile-devices` | List (`q`, `group_id`, `sort`, `order`, `limit`, `offset`, `ids`) |
| `GET/PATCH/DELETE /api/v1/mobile-devices/{id}` | Record with apps, profiles and groups; edit user and location fields; delete the record |
| `GET /api/v1/mobile-devices/{id}/commands` | MDM command history (also `GET /api/v1/commands?mobile_device_id=`) |
| `POST /api/v1/mobile-devices/{id}/update-inventory` | Ask for inventory now |
| `POST /api/v1/mobile-devices/{id}/actions` | Send a command: `{"action": "lock" \| "clear_passcode" \| "erase" \| "restart" \| "shutdown" \| "enable_lost_mode" \| "disable_lost_mode" \| "locate" \| "play_sound" \| "settings" \| "refresh_cellular_plans" \| "collect_bypass_code", …}` (`message`, `phone`, `footnote`, `confirm_name`, `preserve_data_plan`, `settings: [{"item", "device_name", "enabled", "time_zone"}]`, `esim_server_url`) |
| `GET /api/v1/mobile-device-payloads` | Payload editors (type, title, category, JSON schema) |
| `GET/POST /api/v1/mobile-device-profiles`, `GET/PUT/DELETE /api/v1/mobile-device-profiles/{id}` | Mobile profiles: `{"name", "category", "description", "scope", "payloads": [{"type", "values"}]}` or `"payload": "<xml>"` for an upload |
| `POST /api/v1/mobile-device-profiles/preview` | Build the XML without saving (`payload_errors` per payload) |
| `GET /api/v1/mobile-device-profiles/{id}/deployments`, `POST …/deploy` `{"reinstall": bool}` | Status per device; sync or redeploy |
| `GET /api/v1/mobile-devices/{id}/profile-deployments` | One device's VibeMDM profiles |
| `POST /api/v1/mobile-scope/preview` | `{"scope": …}` → devices in scope |
| `POST /api/v1/mobile-devices/{id}/activation-lock-bypass/reveal` | `{"reason": "…"}` → `{"code": "…"}` (audited, not cached) |
| `GET/POST /api/v1/mobile-device-groups`, `GET/PUT/DELETE /api/v1/mobile-device-groups/{id}` | Groups |
| `GET/PUT /api/v1/mobile-device-groups/{id}/members` | Members (`{"mobile_device_ids": [...]}` for static groups) |
| `GET /api/v1/mobile-device-groups/criteria-fields`, `POST /api/v1/mobile-device-groups/preview` | Criteria fields and preview |
| `GET/POST /api/v1/mobile-device-searches`, `GET/PUT/DELETE /api/v1/mobile-device-searches/{id}` | Saved searches |
| `GET /api/v1/mobile-device-searches/fields`, `GET /api/v1/mobile-device-searches/{id}/run`, `…/export.csv` | Columns, results, CSV |

## Testing without a device

`vibemdm-sim -platform iphone` or `-platform ipad` enrolls simulated iPhones or iPads (MDM only; odd-numbered devices
are supervised, every third has no passcode). The simulator answers every command above, including Lost Mode with a
location and a fake bypass code. **Untested on real hardware:** all commands, Lost Mode location, the bypass code and
its use after an erase. Real-device testing on an iPhone 13 mini and an iPad (8th generation) is still to do.
