# 8. Configuration profiles

Configuration profiles are Apple's standard way to configure a device. VibeMDM delivers them over MDM with
`InstallProfile` and `RemoveProfile`, tracks their status per Mac, builds profiles with payload editors for every
payload Jamf Pro offers ([§8.7](#87-payload-editors-jamf-pro-payloads)), keeps uploaded `.mobileconfig` files, and builds
managed-preference profiles from JSON schemas ("Application & Custom Settings").

## 8.1 Profile anatomy

A profile is an XML property list:

```xml
<plist version="1.0"><dict>
  <key>PayloadType</key>        <string>Configuration</string>
  <key>PayloadVersion</key>     <integer>1</integer>
  <key>PayloadIdentifier</key>  <string>com.example.wifi</string>
  <key>PayloadUUID</key>        <string>6F1C…</string>
  <key>PayloadDisplayName</key> <string>Office Wi-Fi</string>
  <key>PayloadScope</key>       <string>System</string>
  <key>PayloadContent</key>
  <array>
    <dict>
      <key>PayloadType</key>       <string>com.apple.wifi.managed</string>
      <key>PayloadIdentifier</key> <string>com.example.wifi.payload</string>
      <key>PayloadUUID</key>       <string>A0B2…</string>
      <key>PayloadVersion</key>    <integer>1</integer>
      <key>SSID_STR</key>          <string>Office</string>
      <!-- payload-specific keys -->
    </dict>
  </array>
</dict></plist>
```

> **Study note (payload basics):**
> - The outer dictionary has `PayloadType = Configuration`. Each item in `PayloadContent` is a **payload** with its
>   own `PayloadType`, `PayloadIdentifier`, `PayloadUUID` and `PayloadVersion`.
> - **`PayloadIdentifier`** identifies the profile. Installing a profile with the same identifier **replaces** the
>   existing one. That is how updates work.
> - **`PayloadUUID`** should change when the content changes. Some tools rely on it to detect a new version.
> - **`PayloadScope`** (`System` or `User`) matters for manual installs. Over MDM, the **channel** decides: the
>   device channel installs system-wide, the user channel for one user.
> - Profiles installed by MDM can be removed only by the MDM, or indirectly by removing the MDM enrollment
>   profile, which removes everything that MDM installed. `PayloadRemovalDisallowed` matters for manually
>   installed profiles; on iOS and iPadOS it is honoured only on supervised devices. Making the **enrollment**
>   profile itself non-removable on a Mac requires ADE.
> - Profiles can be signed (PKCS#7) so the installer shows them as *Verified*. Profiles delivered over MDM do not
>   need a signature.

## 8.2 Upload and deploy a profile

1. Build the profile in a tool such as **iMazing Profile Editor**, **ProfileCreator** or Apple Configurator, and
   export it **unsigned** as XML. A signed profile is a PKCS#7 blob and fails the "valid property list" check.
2. **Configuration Profiles → New**: upload or paste the `.mobileconfig`, set a category and the scope, save.
   API: `POST /api/v1/profiles` `{"payload": "<?xml …", "category": "Network", "scope": {...}}`.
3. VibeMDM normalizes the profile:
   - rejects anything that is not a property list with `PayloadType = Configuration`;
   - takes the identifier from `PayloadIdentifier` (or writes the identifier you gave into the plist);
   - fills a missing `PayloadUUID`;
   - takes the name and description from `PayloadDisplayName` and `PayloadDescription` if you left them empty;
   - records `level = user` if `PayloadScope` is `User`; otherwise sets `level = device` and `PayloadScope = System`.
4. On save, VibeMDM syncs the profile to its scope in the background: `InstallProfile` to every in-scope
   MDM-enrolled Mac that does not have it, `RemoveProfile` to Macs that have it but are out of scope.

> VibeMDM sends every profile over the **device channel**, including profiles marked `level = user`. It has no
> user-channel delivery. For user-level settings, use device-level payloads or MCX managed preferences, which apply
> to all users.

### Updating a profile

Editing the payload (`PUT /api/v1/profiles/{id}`) re-installs the profile on every Mac in scope; the identifier is
kept, so macOS replaces the old version. Changing only the name or scope does not re-send it to Macs that already
have it.

### Deployment status

`GET /api/v1/profiles/{id}/deployments` lists the profile's status on each Mac, and the profile list shows counts.

| Status | Meaning |
| --- | --- |
| `pending` | `InstallProfile` queued; waiting for the device's answer |
| `installed` | The device acknowledged it, or a later `ProfileList` showed it present |
| `failed` | The device returned `Error` (the error chain is stored), or the command could not be queued |
| `removing` | `RemoveProfile` queued |
| `removed` | The device acknowledged the removal |

### Actions

| Action | API |
| --- | --- |
| Re-sync to scope (install missing, remove out-of-scope) | `POST /api/v1/profiles/{id}/deploy` `{"reinstall": false}` |
| Re-send to every Mac in scope | `POST /api/v1/profiles/{id}/deploy` `{"reinstall": true}` |
| Remove from specific Macs | `POST /api/v1/profiles/{id}/remove` `{"computer_ids": [...]}` |
| Install on one Mac outside the scope logic | `POST /api/v1/computers/{id}/commands` `{"type": "InstallProfile", "profile_id": "…"}` |
| Delete | `DELETE /api/v1/profiles/{id}` (sends `RemoveProfile` to every Mac that has it, then deletes) |

Remember that profile scope is evaluated only on save, deploy and enrollment ([§6.2](06-scoping.md#62-where-scope-is-used-and-when-it-is-evaluated)).

The Health check **Configuration profile failures** groups failures per profile and offers **Retry on failed
Macs**. The command inspector shows the device's exact error ([Chapter 16](16-mdm-commands-and-inspector.md)).

### Profiles VibeMDM generates

Some features generate and own a profile. Edit them in their feature, not in the profile editor; your changes
would be overwritten.

| Identifier | Owner |
| --- | --- |
| `com.vibemdm.filevault.escrow`, `com.vibemdm.filevault.enforce` | FileVault ([Chapter 11](11-filevault.md)) |
| `com.vibemdm.compliance.<baseline-id>` | Compliance ([Chapter 12](12-compliance.md)) |
| `com.vibemdm.super` | Software Updates → super ([Chapter 10](10-software-updates.md)) |
| `com.vibemdm.xcreds` | Identity → XCreds ([Chapter 13](13-identity-xcreds.md)) |
| `com.vibemdm.app.<domain>.<random>` | Application & Custom Settings (below) |

Viewers see profile payloads redacted.

## 8.3 Application & Custom Settings (JSON schemas)

Many Mac apps are configured with **managed preferences**: keys in a preference domain such as
`com.google.Chrome`. Jamf Pro popularized "external application" JSON schemas that describe those keys so an admin
gets a form instead of hand-written XML. VibeMDM accepts the same schema format.

### Built-in schemas

| ID | Domain | Purpose |
| --- | --- | --- |
| `builtin:com.macjutsu.super` | `com.macjutsu.super` | super 5.1 settings (used by Software Updates → super) |
| `builtin:com.twocanoes.xcreds` | `com.twocanoes.xcreds` | XCreds settings (used by Identity → XCreds) |

VibeMDM's built-in schemas were written from each project's public documentation; they are not copied from Jamf's
or anyone else's schema library.

### Upload a custom schema

1. Get a schema: from the app vendor, the community *Jamf JSON schema* collections, or write your own.
2. **Configuration Profiles → Application & Custom Settings → Upload schema**.
   API: `POST /api/v1/custom-schemas` `{"name": "Google Chrome", "preference_domain": "com.google.Chrome", "schema": {…}}`.
   `schema` may be a JSON object or a string containing JSON (as pasted from a file).
3. If you leave `preference_domain` empty, VibeMDM uses the schema's `__preferencedomain`; if `name` is empty, it uses
   the schema `title`.

Checks on upload: the schema must be valid JSON, at most 2 MB, have `properties`, nest no deeper than 32 levels,
and every `pattern` must compile. The domain must be reverse-DNS (`^[A-Za-z0-9][A-Za-z0-9_-]*(\.[A-Za-z0-9_-]+)+$`)
and must not start with `com.apple.ManagedClient`.

Other endpoints: `GET /api/v1/custom-schemas` (built-in and custom, with the number of profiles using each),
`GET/PUT/DELETE /api/v1/custom-schemas/{id}` (built-ins cannot be deleted).

### Supported schema keywords

| Keyword | Support |
| --- | --- |
| `title`, `description` | Labels and help text |
| `type` | `string`, `integer`, `number`, `boolean`, `array`, `object` (a single type or a list) |
| `properties`, `items` | Nested objects and arrays |
| `enum`, `default` | Allowed values; defaults pre-fill the form |
| `minimum`, `maximum` | Numbers |
| `minLength`, `maxLength`, `pattern` | Strings (pattern is a Go regular expression) |
| `minItems`, `maxItems` | Arrays |
| `required` | Required keys of an object |
| `property_order` / `propertyOrder` | Field order |
| `links` | Documentation links shown next to a field (`rel`, `href`) |
| `anyOf` / `oneOf` | Alternatives, including Jamf's "Not Configured" idiom (an alternative of `type: null`) |
| `options` | `enum_titles`, `infoText`, `inputAttributes.placeholder`, `remove_empty_properties` |
| `__preferencedomain`, `__version` | Header fields |
| **`format: "data"`** (VibeMDM extension) | A base64 string that is emitted as plist `<data>` (for example a license file) |

### Validation rules

- Keys the schema does not define are **rejected**.
- Keys with empty values (null, `""`, `[]`, `{}`) are **dropped**, so "empty means not managed".
- Values are coerced to plist types: integers to `<integer>`, numbers to `<real>`, booleans, strings, arrays,
  dictionaries; `format: data` strings to `<data>`.
- For `anyOf`/`oneOf`, the first non-null alternative that accepts the value is used.
- Errors come back as a list of `{path, message}`, e.g. `{"path": "ExtensionSettings.*.installation_mode", "message": "is not one of the allowed values"}`.

### Create a profile from a schema

1. Open the schema and fill in the form. **Preview** shows the validated values and the generated XML.
   API: `POST /api/v1/custom-schemas/{id}/preview` `{"values": {...}}`.
2. Save with a name, category (default `Applications`) and scope.
   API: `POST /api/v1/custom-schemas/{id}/profiles` `{"name": "Chrome policy", "scope": {...}, "values": {...}}`.
   To update an existing schema profile, include `"profile_id"`.
3. The profile gets the identifier `com.vibemdm.app.<domain>.<8 random hex>` and is synced like any other profile.
   It remembers its schema (`schema_source`: `custom:<id>` or `builtin:<domain>`) and values (`schema_values`), so it
   opens in the form editor again.

## 8.4 The MCX payload format

Schema profiles use the **`com.apple.ManagedClient.preferences`** payload in the MCX "Forced" form, the same shape
Jamf's Application & Custom Settings payload produces:

```xml
<dict>
  <key>PayloadType</key>       <string>com.apple.ManagedClient.preferences</string>
  <key>PayloadIdentifier</key> <string>com.vibemdm.app.com.google.Chrome.1a2b3c4d.mcx</string>
  <key>PayloadUUID</key>       <string>…</string>
  <key>PayloadVersion</key>    <integer>1</integer>
  <key>PayloadContent</key>
  <dict>
    <key>com.google.Chrome</key>
    <dict>
      <key>Forced</key>
      <array>
        <dict>
          <key>mcx_preference_settings</key>
          <dict>
            <key>HomepageLocation</key> <string>https://intranet.example.com</string>
            <key>BrowserSignin</key>    <integer>2</integer>
          </dict>
        </dict>
      </array>
    </dict>
  </dict>
</dict>
```

VibeMDM derives the UUIDs from the identifier (SHA-1 name-based UUIDs), so an unchanged profile renders
byte-for-byte identically.

> **Study note (MCX and managed preferences):**
> - MCX ("Managed Client for OS X") is the legacy Open Directory policy system. Its preference-management part
>   lives on as the `com.apple.ManagedClient.preferences` payload. Inside, each domain has a list of management
>   frequencies: **`Forced`** (always enforced), **`Set-Once`** (applied once, user may change) and **`Often`**
>   (re-applied at each login). VibeMDM always uses `Forced`.
> - A simpler alternative for many apps is a payload whose `PayloadType` is the app's own domain
>   (`PayloadType = com.google.Chrome`), with the keys at the top level. Both end up as managed preferences.
> - Managed preferences appear under `/Library/Managed Preferences/` (system-wide) and
>   `/Library/Managed Preferences/<user>/`. Read them with `defaults read "/Library/Managed Preferences/com.google.Chrome"`,
>   or check a key's state with `CFPreferencesAppValueIsForced` in code.
> - Managed (forced) values take precedence over the user's own `defaults` values.

## 8.5 Study notes: common payload types

> **Study note (payload types to know):**
>
> | Payload type | Purpose | Notes |
> | --- | --- | --- |
> | `com.apple.mdm` | MDM enrollment | Only in the enrollment profile |
> | `com.apple.security.scep` / `.acme` / `.pkcs12` / `.root` / `.pkcs1` | Certificates and identities | [Chapter 3](03-apns-and-certificates.md) |
> | `com.apple.wifi.managed` | Wi-Fi | SSID, security type, EAP settings |
> | `com.apple.vpn.managed` | VPN | Built-in IKEv2/IPsec, or app-based VPN providers |
> | `com.apple.applicationaccess` | Restrictions | Many keys need supervision on iOS; fewer apply to macOS |
> | `com.apple.TCC.configuration-profile-policy` | **PPPC** (privacy) | Needs UAMDM or ADE |
> | `com.apple.system-extension-policy` | Allowed system extensions (by team ID / bundle ID) | Needs UAMDM |
> | `com.apple.syspolicy.kernel-extension-policy` | Allowed kernel extensions (legacy) | Needs UAMDM; on Apple silicon also needs reduced security |
> | `com.apple.servicemanagement` | **Managed login items / background tasks** (macOS 13+) | Stops users from disabling a vendor's background items |
> | `com.apple.extensiblesso` | Single sign-on extensions, including **Platform SSO** and the Kerberos extension | [Chapter 13](13-identity-xcreds.md) |
> | `com.apple.MCX.FileVault2`, `com.apple.security.FDERecoveryKeyEscrow` | FileVault enablement and key escrow | [Chapter 11](11-filevault.md) |
> | `com.apple.security.firewall` | Application firewall | Enable, block all incoming, stealth mode |
> | `com.apple.systempolicy.control` | Gatekeeper settings | Allowed sources |
> | `com.apple.loginwindow`, `com.apple.screensaver` | Login window and screen saver | |
> | `com.apple.notificationsettings` | Notification permissions per app | |
> | `com.apple.SoftwareUpdate` | Legacy software update settings | Being replaced by DDM ([Chapter 10](10-software-updates.md)) |
> | `com.apple.ManagedClient.preferences` | Custom managed preferences (MCX) | §8.4 |

> **Study note (PPPC / TCC):** Transparency, Consent and Control (TCC) protects privacy-sensitive resources:
> Full Disk Access (`SystemPolicyAllFiles`), Accessibility, Camera, Microphone, Screen Recording (`ScreenCapture`),
> Input Monitoring (`ListenEvent`), Files and Folders, Automation (Apple Events), and more. The PPPC payload can
> pre-approve many services for an app identified by **bundle ID or path plus its code requirement**
> (`codesign -dr - /Applications/App.app` prints it). Key limits:
> - **Camera and Microphone cannot be granted** by profile, only denied. The user must approve them.
> - **Screen Recording and Input Monitoring** cannot be silently granted; since macOS 11 a profile can set
>   `Authorization = AllowStandardUserToSetSystemService` so a **standard** user can approve them.
> - PPPC payloads only work on UAMDM or ADE enrollments.
> - `tccutil reset <Service> <bundleID>` resets a user's decisions for testing.

## 8.6 Inspect profiles on a Mac

```bash
sudo profiles list                      # installed profiles (all levels)
sudo profiles show -output stdout-xml   # full content
profiles status -type enrollment        # MDM enrollment state
```

In **System Settings → General → Device Management** the user sees each profile and its payloads.
MDM-installed profiles show no *Remove* button.

## 8.7 Payload editors (Jamf Pro payloads)

**Configuration Profiles → New profile** builds a profile the way Jamf Pro does: a *General* section (name, category,
description and **Level**: computer or user), then **Add payload** for each kind of setting. Mobile Device
Configuration Profiles use the same editor ([Mobile devices](../mobile-devices.md)). **Upload** keeps a
`.mobileconfig` made elsewhere instead.

### What is covered

The payload list follows Jamf Pro's Computer and Mobile Device Configuration Profiles: 83 editors for macOS and 44 for
iOS/iPadOS. Where one Jamf payload writes several Apple payload types, VibeMDM shows one editor per type, named after
Jamf's tab; the editor header shows the Jamf payload it corresponds to.

| Jamf Pro payload | VibeMDM editors (Apple payload type) |
| --- | --- |
| Security & Privacy | General (`com.apple.preference.security`), FileVault (`com.apple.MCX.FileVault2`, FileVault options, recovery key escrow), Firewall (`com.apple.security.firewall`), Gatekeeper (`com.apple.systempolicy.control`, `.managed`) |
| Restrictions | Restrictions (`com.apple.applicationaccess`), Applications (`.new`), System Settings (`com.apple.systempreferences`), Media (`com.apple.systemuiserver`, disc burning), Menu bar items, Accounts, Users & Groups, Game Center, App Store, App extensions, Desktop |
| Network | Wi-Fi (`com.apple.wifi.managed`) and Ethernet: one editor with an **Interface** menu (any, first/second/third active, first/second/third) that picks Apple's per-interface payload type |
| Login Window | Login Window (`com.apple.loginwindow`), Login Window options (`loginwindow`), Screen Saver |
| Parental Controls | Content filter, Time limits |

Also on macOS: Passcode, VPN and Per-App VPN (with app mapping), Proxies, DNS Settings, DNS Proxy, Relay, Content
Caching, Certificate, SCEP, ACME, AD Certificate, certificate and identity preferences, Directory, Dock, Energy
Saver, Time Machine, Time Server, Finder, Accessibility, Printing, AirPrint, AirPlay, Font, Notifications,
Associated Domains, Domains, Identification, Privacy Preferences Policy Control, Approved Kernel Extensions, System
Extensions, Managed Login Items, Login Items, Mobility, Smart Card, Content Filter, Certificate Transparency,
Single Sign-On Extensions (including Kerberos and Platform SSO), Mail, Exchange Web Services, LDAP, Calendar, Contacts,
Skip Setup Items, System Migration, File Provider, Xsan and Education.

Left out on purpose: Jamf's macOS **Software Update** payload (Jamf removed it in 11.31 because Apple dropped it in
macOS 27; use [Software Updates](10-software-updates.md)), and payloads Apple has retired (Lights Out Management,
FileVault recovery key redirection for macOS 10.12 and earlier, macOS Server accounts, login scripts, legacy
Dictionary/Dictation/ShareKit restrictions). Application & Custom Settings stays its own page ([§8.3](#83-application--custom-settings-json-schemas)).

### How the editors are made

`cmd/profile-schemas` generates every editor from Apple's [device-management](https://github.com/apple/device-management)
repository, so each one has every key Apple documents for the platform, with its type, allowed values, range, default
and description. Keys Apple removed on a platform are left out. To pick up a newer Apple release:

```bash
git clone --depth 1 https://github.com/apple/device-management /tmp/device-management
go run ./cmd/profile-schemas -src /tmp/device-management
```

The catalog (which payloads, their Jamf names and categories) is in `cmd/profile-schemas/catalog.go`; hand-written
titles and descriptions are in `cmd/profile-schemas/curated/`. The output is `internal/schema/payloads/<platform>/`.

### Using the editor

- Every setting starts **Not set** and is left out of the profile, so the device keeps Apple's default. (Jamf writes
  every Restrictions key, so an unchecked box there means *false*; in VibeMDM it means *not managed*.)
- Small notes next to a setting: **Supervised** (only works on supervised devices), the first OS version that supports
  it when that is newer than iOS 16 / macOS 13, **Deprecated**, and **User level only** / **Computer level only**.
- Long forms open on **Show only configured** once something is set; big groups (Enterprise/EAP settings, IKEv2
  parameters, …) stay folded until you open them.
- **Certificates:** fields that refer to a certificate (a Wi-Fi or Ethernet network's identity, a VPN's certificate, an
  Exchange account's S/MIME certificate, the FileVault escrow certificate) are menus listing the Certificate, SCEP and ACME
  payloads in the same profile, as in Jamf. The reference must point to a certificate payload in the profile.
- **Level:** payloads that Apple allows only at computer level (Privacy Preferences Policy Control, kernel and system
  extensions, …) can't be added to a user-level profile; the server refuses the combination too.
- **Secrets** (passwords, shared secrets, SCEP challenges, `.p12` files and their passwords) are password fields and are
  hidden from viewers, both in the form values and in the XML.

### Payload variables

Text settings may contain Jamf Pro's payload variables. They are filled in for each device when the profile is sent:

| Variable | Mac | iPhone / iPad |
| --- | --- | --- |
| `$COMPUTERNAME`, `$DEVICENAME` | Computer name | Device name |
| `$SERIALNUMBER`, `$UDID` | ✓ | ✓ |
| `$MACADDRESS` | Primary MAC address | Wi-Fi MAC address |
| `$ASSETTAG` | – | Asset tag |
| `$USERNAME` | Assigned user, else the last user | Assigned user |
| `$FULLNAME`, `$REALNAME` | Assigned user's name, else the last user's | Assigned user's name |
| `$EMAIL`, `$PHONE` | From User and Location | Assigned email; the device's phone number |
| `$POSITION`, `$SITENAME` | From User and Location | – |
| `$DEPARTMENTNAME`, `$BUILDINGNAME`, `$ROOM` | ✓ | ✓ |

Unknown values become empty, as in Jamf Pro. Values are XML-escaped. A profile already installed keeps the values it
was sent with: after renaming a device or changing its user, use **Redeploy** on the profile's Deployments tab.

> **Untested on real hardware:** the generated payloads follow Apple's schema and build valid profiles (unit tests and
> the throwaway-server checks cover certificate references, Ethernet interfaces, kernel extension maps, user-level
> scope and redaction), but they have not yet been installed on a real Mac, iPhone or iPad. Test new payload types
> on one device first.

### API

| Endpoint | Purpose |
| --- | --- |
| `GET /api/v1/profile-payloads?platform=macos\|ios` | The editors (id, Apple payload type, title, category, level, notes and JSON schema), gzipped |
| `POST /api/v1/profiles` / `PUT /api/v1/profiles/{id}` | `{"name", "category", "description", "level": "device"\|"user", "scope", "payloads": [{"type", "uuid", "values"}]}` builds the profile; `"payload": "<xml>"` uploads one instead |
| `POST /api/v1/profiles/preview` | Build the XML without saving (`payload_errors` per payload) |

Next: [Chapter 9 — Mac Apps and Installomator](09-apps-installomator.md).
