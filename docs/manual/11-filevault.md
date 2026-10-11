# 11. FileVault

VibeMDM escrows each Mac's FileVault **personal recovery key (PRK)** with Apple's native MDM mechanism, repairs
Macs that are missing a key with [Escrow Buddy](https://github.com/macadmins/escrow-buddy) (Apache-2.0), and lets
admins reveal a key with a reason that is audited.

## 11.1 How escrow works

1. On first boot VibeMDM creates an **escrow identity** (certificate + private key) and stores it in the database
   (the key sealed at rest when `VIBEMDM_ENCRYPTION_KEY` is set). It is created only when none exists; if it exists
   but cannot be read, the server refuses to start, because replacing it would make every escrowed key
   undecryptable.
2. The built-in **FileVault Recovery Key Escrow** profile (`com.vibemdm.filevault.escrow`) contains:
   - a `com.apple.security.pkcs1` payload with the escrow certificate;
   - a `com.apple.security.FDERecoveryKeyEscrow` payload with `Location` (your organization name, shown to the user
     as where the key is kept) and `EncryptCertPayloadUUID` pointing at the certificate.
3. With the profile installed, macOS encrypts the PRK to the certificate when a PRK is generated.
4. When the server sends the MDM command **`SecurityInfo`**, the Mac returns the encrypted key as
   `FDE_PersonalRecoveryKeyCMS` (a CMS/PKCS#7 enveloped blob).
5. VibeMDM decrypts it with the escrow private key, stores the PRK sealed in `filevault_keys`, records a display
   fingerprint (`…-ABCD`, the last block), writes the audit entry `filevault.key_escrowed` and fires the
   `FileVaultKeyEscrowed` webhook. The blob is removed before `SecurityInfo` is saved to inventory.

macOS escrows only keys generated **after** the escrow profile is installed. Macs encrypted earlier show as
**missing key** until a new key is generated (§11.3).

The server asks for `SecurityInfo` every 4 hours (sweep every 10 minutes) for every Mac with FileVault on and no
escrowed key, so keys are collected without anyone clicking anything.

## 11.2 Turn it on

**FileVault** in the sidebar (`GET /api/v1/filevault`) shows the fleet summary (`escrowed`, `missing_key`,
`not_encrypted`, `unknown`), a row per Mac, the escrow certificate and the state of both built-in profiles.

1. Enable the **escrow profile** and scope it (usually *All computers*).
2. Optionally enable the **enforcement profile** to turn FileVault on (§11.4).
3. Optionally enable **auto-remediation** (§11.3).

API (admin):

```json
PUT /api/v1/filevault
{
  "escrow":  { "enabled": true, "scope": { "all_computers": true } },
  "enforce": { "enabled": true, "scope": { "group_ids": ["<laptops>"] },
               "options": { "DeferAtLogout": false, "DeferDontAskAtLogout": false,
                            "MaxBypassAttempts": 0, "ShowRecoveryKey": false } },
  "auto_remediate": true
}
```

Each section is optional. Turning a profile off keeps its record but empties its scope, which removes it from
every Mac. Like all profiles, the built-in ones are synced when saved; deploy again after changing the membership
of a group they are scoped to ([§6.2](06-scoping.md#62-where-scope-is-used-and-when-it-is-evaluated)).

Other endpoints:

| Action | API |
| --- | --- |
| Ask every Mac that is missing a key for `SecurityInfo` now | `POST /api/v1/filevault/check` (admin) |
| Escrow status for one Mac (never the key) | `GET /api/v1/computers/{id}/filevault` → `escrowed`, `fingerprint`, `escrowed_at`, `rotated_count`, `source`, `escrow_profile_status` |
| Reveal a key | `POST /api/v1/computers/{id}/filevault/reveal` `{"reason": "…"}` (admin) |
| Run Escrow Buddy remediation on one Mac | `POST /api/v1/computers/{id}/filevault/remediate` (admin) |

`rotated_count` counts how many times a different key was escrowed for the Mac.

## 11.3 Missing keys: Escrow Buddy remediation

Escrow Buddy is an authorization plugin that generates a new PRK at the next login by a FileVault-enabled user,
without asking for a password (it uses the password the user just typed at login). The new key is then escrowed
by the escrow profile.

The `filevault_regenerate` agent task:

1. Checks that FileVault is on (`fdesetup status`). If not, it fails with *FileVault is not enabled on this Mac;
   nothing to escrow*.
2. Installs Escrow Buddy with Installomator (label `escrowbuddy`, Team ID `T4SK8ZXCXG`) if the plugin bundle is
   missing **or** its `Escrow Buddy:Invoke,privileged` mechanism is missing from the `system.login.console`
   authorization right. macOS updates can reset the authorization database, so a missing mechanism forces a
   reinstall.
3. Sets `GenerateNewKey = true` in `/Library/Preferences/com.netflix.Escrow-Buddy`.

After the user's next login, the new key exists; the 4-hourly `SecurityInfo` sweep collects it.

| Trigger | When |
| --- | --- |
| Manual | Computer → FileVault → **Remediate** (`POST …/filevault/remediate`) |
| Auto-remediation (`filevault_auto_remediate`) | At an agent check-in, at most once a day per Mac, when FileVault is on, the escrow profile is `installed` and no key is escrowed |
| Health self-heal | The *FileVault key not escrowed* finding offers **Generate a new key with Escrow Buddy** (safe; auto-heal off by default) |

## 11.4 The enforcement profile

`com.vibemdm.filevault.enforce` contains a `com.apple.MCX.FileVault2` payload:

| Key | Value | Meaning |
| --- | --- | --- |
| `Enable` | `On` | Turn FileVault on |
| `Defer` | `true` | Enable at the next login (or logout), using the user's password, instead of needing credentials in the profile |
| `UseRecoveryKey` | `true` | Create a personal recovery key (which the escrow profile captures) |
| `ShowRecoveryKey` | option | Show the PRK to the user |
| `DeferForceAtUserLoginMaxBypassAttempts` | option `MaxBypassAttempts` | How many times the user may skip at login: `0` forces it at the next login, `-1` allows unlimited skips |
| `DeferDontAskAtUserLogout` | option | Don't prompt at logout. If `DeferAtLogout` is false, VibeMDM always sets it to `true`, so the prompt happens at login |

Deploy the escrow profile **before or together with** the enforcement profile, so the first PRK is escrowed.

## 11.5 Revealing a key

1. Open the computer's **FileVault** panel and choose **Reveal recovery key**.
2. Enter a reason (for example a ticket number). The API does not enforce a non-empty reason, so make it a
   team rule.
3. The key is returned to the admin (with `Cache-Control: no-store`). The audit log records
   `filevault.key_viewed` with the actor, reason, client IP and key fingerprint, and the `FileVaultKeyViewed`
   webhook fires with `computer_id`, `computer`, `serial`, `actor`, `reason`.

Viewers can never reveal keys, and MCP tools never return them.

> **Practice:** after revealing a key to a user, generate a new one (Escrow Buddy remediation, or
> `sudo fdesetup changerecovery -personal` on the Mac), so the disclosed key stops working.

## 11.6 Health checks

| Check | Condition |
| --- | --- |
| **FileVault key not escrowed** | FileVault on, escrow profile installed for over 24 hours, no key |
| **FileVault escrow not configured** | Macs have FileVault on but the escrow profile is not deployed to them |
| **Encryption at rest** | Escrowed keys are stored unencrypted because `VIBEMDM_ENCRYPTION_KEY` is not set |

## 11.7 Study notes: FileVault and Secure Token

> **Study note (FileVault basics):**
> - FileVault encrypts the startup volume. On Macs with Apple silicon or a T2 chip, internal storage is always
>   hardware-encrypted; turning FileVault on adds protection by requiring a user's password (or recovery key) to
>   unlock the volume encryption key at startup. Enabling it on such Macs is near-instant.
> - Who can unlock: users with a **Secure Token** (and on Apple silicon, the **volume owners**), plus the recovery
>   key.
> - Tools: `fdesetup status`, `sudo fdesetup list` (enabled users), `sudo fdesetup enable`,
>   `sudo fdesetup changerecovery -personal`, `sudo fdesetup validaterecovery`, `sudo fdesetup add -usertoadd <user>`.

> **Study note (PRK vs IRK):**
> - A **personal recovery key (PRK)** is unique to each Mac (format `XXXX-XXXX-XXXX-XXXX-XXXX-XXXX`). It is the
>   recommended method for organizations, escrowed to MDM with `FDERecoveryKeyEscrow` and returned in
>   `SecurityInfo` as `FDE_PersonalRecoveryKeyCMS`.
> - An **institutional recovery key (IRK)** is one keychain-based key (`FileVaultMaster.keychain`) shared by many
>   Macs. It is legacy; Apple recommends PRKs and documents limitations for IRKs on newer Macs and macOS versions.
>   VibeMDM does not use IRKs. Check the *Apple Platform Deployment* guide for the current support statement.
> - An **iCloud/Apple Account** recovery option exists for personal Macs and is not appropriate for organizations.
> - MDM also has a `RotateFileVaultKey` command for rotating keys; VibeMDM does not use it and relies on Escrow Buddy.
> - The escrow profile must be installed through MDM (the device channel) for `FDE_PersonalRecoveryKeyCMS` to appear.

> **Study note (Secure Token, volume ownership, bootstrap token):**
> - **Secure Token** is an account attribute that lets the account unlock FileVault and grant Secure Token to
>   others. The first administrator created in Setup Assistant gets it automatically. Check with
>   `sysadminctl -secureTokenStatus <user>` (VibeMDM shows it per local account in inventory). Grant with
>   `sysadminctl -secureTokenOn <user> -password - -adminUser <admin> -adminPassword -`.
> - On **Apple silicon**, **volume owners** (users with a Secure Token, usually) are the accounts allowed to change
>   startup security settings and authorize software updates.
> - The **bootstrap token** (macOS 10.15+, escrowed to MDM) lets macOS give a Secure Token to accounts that log in
>   later, such as mobile or newly created accounts, so they can unlock FileVault. Without it, accounts created by
>   scripts or by tools such as XCreds may end up without a Secure Token. See [Chapter 4](04-enrollment.md#410-study-notes-supervision-user-approved-mdm-activation-lock-bootstrap-token).

## 11.6 Local administrator passwords (LAPS)

The same escrow pattern also manages a local administrator account's password: a random password per Mac, rotated
on a schedule and after each view, sealed on the server, and revealed to admins with an audited reason. It lives
under **Security → Local Admin Passwords** and is documented in [docs/laps.md](../laps.md), including the
stage → apply → verify → commit protocol that keeps the Mac and the server in agreement when something fails.

Next: [Chapter 12 — Compliance](12-compliance.md).
