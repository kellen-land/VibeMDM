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

## API

| Method and path | Purpose |
| --- | --- |
| `GET /api/v1/mobile-devices` | List (`q`, `group_id`, `sort`, `order`, `limit`, `offset`, `ids`) |
| `GET/PATCH/DELETE /api/v1/mobile-devices/{id}` | Record with apps, profiles and groups; edit user and location fields; delete the record |
| `GET /api/v1/mobile-devices/{id}/commands` | MDM command history (also `GET /api/v1/commands?mobile_device_id=`) |
| `POST /api/v1/mobile-devices/{id}/update-inventory` | Ask for inventory now |
| `GET/POST /api/v1/mobile-device-groups`, `GET/PUT/DELETE /api/v1/mobile-device-groups/{id}` | Groups |
| `GET/PUT /api/v1/mobile-device-groups/{id}/members` | Members (`{"mobile_device_ids": [...]}` for static groups) |
| `GET /api/v1/mobile-device-groups/criteria-fields`, `POST /api/v1/mobile-device-groups/preview` | Criteria fields and preview |
| `GET/POST /api/v1/mobile-device-searches`, `GET/PUT/DELETE /api/v1/mobile-device-searches/{id}` | Saved searches |
| `GET /api/v1/mobile-device-searches/fields`, `GET /api/v1/mobile-device-searches/{id}/run`, `…/export.csv` | Columns, results, CSV |

## Testing without a device

`vibemdm-sim -platform iphone` or `-platform ipad` enrolls simulated iPhones or iPads (MDM only; odd-numbered devices
are supervised, every third has no passcode). Real-device testing on an iPhone 13 mini and an iPad (8th generation)
is still to do.
