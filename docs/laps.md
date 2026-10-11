# Local administrator passwords (LAPS)

VibeMDM gives every Mac its own random local administrator password, keeps it encrypted on the server, lets an
administrator reveal it with an audited reason, and changes it after anyone views it. It is modelled on the
**jamf binary** method of Jamf Pro's LAPS, and the server offers a Jamf Pro-compatible API so existing Jamf LAPS
scripts work. Console: **Security → Local Admin Passwords**, and **Computer → Security → Local admin password**.

> **Not yet proven on a real Mac.** Password generation, the escrow protocol, retry logic, the server, the
> Jamf-compatible API and the console are tested, and the simulator runs the real agent code against a fake
> account database. The actual `sysadminctl` and `dscl` calls, hiding the account, Secure Token behaviour and
> FileVault interaction need a human on a real Mac (see "What needs a real Mac").

## How it compares with Jamf Pro

Jamf Pro has two LAPS methods. The **MDM method** creates an account in Setup Assistant through a PreStage
(Automated Device Enrollment) and rotates it with the MDM command `SetAutoAdminPassword`. The **jamf binary
method** creates a managed local administrator account after enrollment (Settings → User-Initiated Enrollment →
"Create managed local administrator account") and rotates it at check-in. VibeMDM implements the second one,
because it works without Apple Business Manager.

| Jamf Pro (jamf binary method) | VibeMDM |
| --- | --- |
| Account created by the jamf binary after enrollment, username set in User-Initiated Enrollment, optionally hidden | *Create the account if it is missing* and *Hide the account from the login window* in LAPS settings; the agent creates it at its next check-in |
| Password: 30 characters, upper case letters, numbers and dashes | Default *Easy to type* style: 30 characters, `A–Z` and `2–9` (no look-alikes) in groups of five separated by dashes. *Complex* style is an option |
| **Rotation After Viewing Interval** (`passwordRotationTime`, API default 3600 s) | *Rotate after viewing (minutes)*, default 60. Always on, as in Jamf |
| **Rotation Interval** (`autoRotateEnabled` + `autoRotateExpirationTime`, default off / 90 days) | *Also rotate on a schedule*, default off; *Every (days)*, default 90 |
| Rotation happens at the next check-in (15 minutes by default) | The agent checks at every check-in; *Rotate* and *Set password* wake it through its long-poll, so it usually changes within seconds |
| View password from Inventory → Local User Accounts, with a notice of when it rotates | *Show password* (asks for a reason) on the computer's Security tab and on the LAPS page; the dialog says when it will change |
| Managed Local Administrator Account History | *History* on the computer's LAPS panel (created, changed, viewed, scheduled, set, problem) |
| Audit: who viewed which password, password history | Every password the Mac has had (last 50) is kept sealed with who viewed it and why |
| `set-password` API to choose a password | *More → Set password…* on the computer, or the compatible API |
| Pending rotations (`pending-rotations`) | `GET /api/v1/laps/pending-rotations` and the compatible API |
| Policy *Management Accounts → Rotate Account Password* | *More → Rotate now* on the computer, or *Rotate* on the LAPS page |
| Changing the username affects only re-enrolled Macs; old accounts stay | Changing the account name manages the new account at the next check-in; old accounts are left on the Macs |
| Account can receive a Secure Token; Jamf recommends not using it for FileVault | The agent changes the password with `sysadminctl` and the old password, which keeps Secure Token in step. The same advice applies |
| MDM LAPS (`autoDeployEnabled`) | Not offered: it needs Automated Device Enrollment. The compatible API rejects `autoDeployEnabled: true` |

Differences on purpose: VibeMDM requires a **reason** to reveal a password in the console (Jamf does not), and it
keeps the old password until the Mac confirms the new one (below).

## How a rotation works

1. At each check-in the agent asks `GET /agent/v1/laps`. The server answers whether a rotation is due and why:
   `set` (an admin chose a password), `missing`, `requested`, `used` (the rotation-after-viewing time passed),
   `expired` (scheduled rotation) or `account_changed`.
2. The agent picks the password: the admin-chosen one, or a generated one (`crypto/rand`).
3. It writes the password to a root-only file `/Library/Application Support/VibeMDM/laps-pending.json`.
4. It **stages** the password on the server (`POST /agent/v1/laps/stage`). If this fails the account is left alone.
5. It changes the account: `sysadminctl -resetPasswordFor` with the old password, falling back to
   `dscl . -passwd` when the new password does not authenticate and the account has no Secure Token; or `sysadminctl -addUser … -admin` when the account is missing and creation is allowed. With
   *Hide* on it sets `IsHidden` to 1.
6. It checks the new password with `dscl . -authonly` and reads the account's `GeneratedUID`.
7. It **commits** (`POST /agent/v1/laps/commit`) as applied or failed, then deletes the pending file. Only a
   committed password becomes current; the old one moves to the history.

### Failure modes

| What goes wrong | What happens |
| --- | --- |
| The stage upload fails | Nothing changed on the Mac; the next check-in tries again. |
| The change fails | The agent reports `failed`; the server discards the staged password, keeps the old one and logs a *Problem* event. An admin-chosen password is dropped so the Mac does not retry it forever. |
| The tool exits with an error but the change took effect | The agent trusts `dscl -authonly`, not the exit status, and commits it as applied. |
| The tool exits 0 but changed nothing | `sysadminctl` does this for a wrong old password or a password-policy rejection. The agent decides by `dscl -authonly`; for an account without a Secure Token it then falls back to `dscl . -passwd`. The failure message carries the tool's own output (passwords removed) with a plain explanation: *password policy rejected the new password*, *the current password on record is not the account's password* (use **Set password**), or *refused because of the Secure Token*. |
| A configuration profile sets password rules (for example an mSCP baseline) | Generated passwords never start with a symbol and never contain three repeated or consecutive characters (`aaa`, `abc`, `321`). The Jamf format has no lower case letters: if a profile requires them, use the Complex format. |
| The agent dies or loses the network after changing the account | The pending file survives. At the next cycle the agent tests it against the account and commits applied or failed. If the server forgot the stage (409), it stages again first. |
| A staged password is never confirmed | A Health finding opens after one hour; *Show password* displays the current and the unconfirmed password so you can try both. |
| The account does not exist and creation is off | The agent reports the problem; nothing is created. |

## Secure Token and volume ownership

A Secure Token lets an account unlock FileVault and give tokens to others; on Apple silicon a **volume owner** can
also authorize software updates and change startup security. The agent checks the LAPS account at each check-in
(`sysadminctl -secureTokenStatus`, and `diskutil apfs listUsers /` for volume ownership, which Intel Macs do not have)
and the computer's *Local admin password* panel shows both.

macOS does not let MDM or an agent give an account a Secure Token on its own. There are exactly two ways, and
VibeMDM supports both:

1. **Grant Secure Token…** (computer → Security → Local admin password → *More*). Enter the account name and
   password of an administrator who already has a Secure Token (on Apple silicon, one who is a volume owner, such
   as the person who set up the Mac). VibeMDM seals the credentials, hands them to the agent **once** (then deletes
   them; they expire after 30 minutes if the Mac is offline), and the agent runs
   `sysadminctl -secureTokenOn <LAPS account> -password … -adminUser … -adminPassword …`. On Apple silicon the
   account becomes a volume owner too. The result is checked with `-secureTokenStatus` (sysadminctl exits 0 even
   when it fails) and recorded as a *Secure Token granted* or *Problem* event. Audit: `laps.secure_token_requested`,
   `laps.secure_token_credentials_fetched`, `laps.secure_token_granted` / `_failed`.
2. **Bootstrap token.** If the Mac has escrowed its bootstrap token to VibeMDM, macOS gives a Secure Token (and
   volume ownership) to an account the first time it signs in at the login window. Showing the LAPS password and
   signing in once as that account therefore also works; the agent reports the new state at its next check-in.

Rotation keeps the token: the agent changes the password only with `sysadminctl -resetPasswordFor … -oldPassword`,
and **never falls back to `dscl`** on an account that holds a Secure Token (that would leave FileVault unlock and
the login password out of sync). If the old password is unknown, rotation fails with an explanation instead.

## Showing a password

*Show password* (admin only) asks for a reason, then returns the password with `Cache-Control: no-store`. It is
recorded in the LAPS history and the audit log (`laps.password_viewed`, with reason, IP and fingerprint) and fires
the `LAPSPasswordViewed` webhook. The password then changes after *Rotate after viewing* minutes (if several people
view it, the earliest scheduled time wins). Viewers can see status and history but never a password.

## Console API (admin unless noted)

| Method and path | Purpose |
| --- | --- |
| `GET /api/v1/laps` (viewer) | Settings, per-Mac status and summary. Never contains a password |
| `PUT /api/v1/laps` | Save the settings |
| `GET /api/v1/laps/pending-rotations` (viewer) | Macs with a queued rotation and why |
| `GET /api/v1/computers/{id}/laps` (viewer) | One Mac's status |
| `GET /api/v1/computers/{id}/laps/history` (viewer) | Event history and password history with views (no passwords) |
| `POST /api/v1/computers/{id}/laps/reveal` `{"reason": "…"}` | Reveal (reason required) |
| `POST /api/v1/computers/{id}/laps/rotate` | Rotate now |
| `PUT /api/v1/computers/{id}/laps/password` `{"password": "…"}` | Set a specific password (8–128 characters) |

## Jamf Pro-compatible API

The same paths as Jamf Pro's `local-admin-password` API, under `/api/v2/local-admin-password`. Use a VibeMDM API
token (Settings → API tokens) as `Authorization: Bearer …`; Jamf's `/api/oauth/token` step is not needed.
`{clientManagementId}` is the VibeMDM computer ID; `userSource` is always `JMF`.

| Method and path | Role | Notes |
| --- | --- | --- |
| `GET /settings` | viewer | `passwordRotationTime` and `autoRotateExpirationTime` in seconds |
| `PUT /settings` | admin | Seconds are rounded up to whole minutes and days. `autoDeployEnabled: true` is refused |
| `GET /pending-rotations` | viewer | `{totalCount, results: [{lapsUser, createdDate}]}` |
| `GET /{clientManagementId}/accounts` | viewer | The LAPS account once it has a password |
| `GET /{clientManagementId}/history` | viewer | Events: `CREATED`, `ROTATED`, `VIEWED`, `PENDING`, `SET`, `ERROR` |
| `PUT /{clientManagementId}/set-password` | admin | `{"lapsUserPasswordList": [{"username", "password"}]}` |
| `GET /{clientManagementId}/account/{username}[/{guid}]/password` | admin | Audited with reason "Jamf-compatible API"; starts the rotation-after-viewing timer |
| `GET /{clientManagementId}/account/{username}[/{guid}]/audit` | admin | Every stored password with `audits: [{viewedBy, dateSeen}]` (passwords included, as in Jamf); audited as `laps.history_viewed` |
| `GET /{clientManagementId}/account/{username}[/{guid}]/history` | viewer | Each password's `createdDate`, `dateLastSeen`, `expirationTime`, `rotationStatus` (`COMPLETED`, `VIEWED`, `PENDING`, `ERROR`) |

Agent endpoints (device token): `GET /agent/v1/laps`, `POST /agent/v1/laps/{stage,commit,error}`. The server
includes the current password in `GET /agent/v1/laps` only when a rotation is due, for `sysadminctl`'s
old-password argument, and an admin-chosen password only while it waits to be applied, and only to that Mac's own
token.

Audit actions: `laps.config`, `laps.rotated`, `laps.rotation_failed`, `laps.rotate_requested`, `laps.password_set`,
`laps.password_viewed`, `laps.history_viewed`. Webhooks: `LAPSPasswordRotated`, `LAPSPasswordViewed`. Health check:
`security.laps_not_current`.

## Storage and secrecy

Passwords (current, staged, admin-chosen and the last 50 old ones) are sealed with `VIBEMDM_ENCRYPTION_KEY`
(`internal/secrets`) in `laps_passwords` and `laps_password_history`. Set the key, or they are stored unencrypted
(Health warns). The fingerprint shown in the console is the first 8 hex digits of a SHA-256, so it reveals nothing
about the password. Passwords are never written to server or agent logs, events or the audit log; the agent's log
handler also redacts attributes named like `password`, `token` or `secret`.

## What needs a real Mac

- `sysadminctl -resetPasswordFor … -oldPassword` and `dscl . -passwd` on current macOS, including on accounts that
  hold a **Secure Token** or are FileVault unlockers. Changing such an account's password with `dscl` as root can
  desynchronise it from FileVault, which is why the agent prefers `sysadminctl` with the old password. As Jamf
  advises, use a dedicated LAPS account that is not a FileVault user and rely on the escrowed recovery key instead.
- Account creation with `sysadminctl -addUser`, hiding with `IsHidden`, and reading `GeneratedUID`.
- `sysadminctl -secureTokenOn` and the parsing of `sysadminctl -secureTokenStatus` and `diskutil apfs listUsers /`
  output on current macOS, and volume ownership after a grant on Apple silicon.
- The password is passed as a command-line argument for a moment. Other local users can see it with `ps` during
  that window. The agent runs these commands as root, briefly.

## Sources

- Jamf blog, *How to securely manage local admin passwords with Jamf Pro and LAPS*:
  <https://www.jamf.com/blog/jamf-pro-laps-manage-local-admin-passwords/>
- Jamf technical paper, *Using LAPS in the Jamf Pro API*: <https://learn.jamf.com/r/en-US/technical-paper-laps-current/Using_LAPS>
- Jamf Pro API reference, `PUT /v2/local-admin-password/settings`:
  <https://developer.jamf.com/jamf-pro/reference/put_v2-local-admin-password-settings>
- Response shapes cross-checked with the community Go SDK:
  <https://pkg.go.dev/github.com/deploymenttheory/go-sdk-jamfpro-v2/jamfpro/jamf_pro_api/local_admin_password>
