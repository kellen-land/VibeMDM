# FileVault recovery key escrow

VibeMDM escrows each Mac's FileVault **personal recovery key (PRK)** using Apple's native MDM mechanism, and
repairs Macs that are missing a key with [Escrow Buddy](https://github.com/macadmins/escrow-buddy) (Apache-2.0).

## How escrow works (the protocol)

1. VibeMDM generates an **escrow encryption certificate** on first boot (stored in the database; its private key is
   sealed at rest when `VIBEMDM_ENCRYPTION_KEY` is set).
2. The built-in **FileVault Recovery Key Escrow** profile carries that certificate plus a
   `com.apple.security.FDERecoveryKeyEscrow` payload. With it installed, macOS encrypts the PRK to the certificate.
3. When the server sends the **SecurityInfo** MDM command, the Mac returns the encrypted key as
   `FDE_PersonalRecoveryKeyCMS` (a CMS/PKCS#7 enveloped blob).
4. The server decrypts it with the escrow private key, stores it sealed, and never writes the blob into inventory.

macOS only escrows keys generated **after** the escrow profile is installed. Macs encrypted earlier show as
*missing key* until a new key is generated.

## Missing keys: Escrow Buddy remediation

Escrow Buddy is an authorization plugin that generates a new PRK at the next login by a FileVault-enabled user,
without prompting for a password. The `filevault_regenerate` agent task:

1. Checks FileVault is on (`fdesetup status`).
2. Installs Escrow Buddy with Installomator (`escrowbuddy` label, Team ID `T4SK8ZXCXG`) if the plugin bundle is
   missing **or** its `Escrow Buddy:Invoke,privileged` mechanism is gone from `system.login.console`. macOS updates
   can reset the authorization database, so a missing mechanism forces a reinstall.
3. Sets `GenerateNewKey = true` in `/Library/Preferences/com.netflix.Escrow-Buddy`.

After the user logs in, the new key is escrowed. The server re-queries **SecurityInfo** every 4 hours for Macs that
still lack a key, so it is collected without anyone clicking anything.

With **auto-remediation** on, the agent's check-in queues this task (at most once a day) for any Mac that has FileVault
on, the escrow profile installed, and no escrowed key.

## Console and API

| Action | API |
| --- | --- |
| Fleet overview and built-in profile state | `GET /api/v1/filevault` |
| Turn escrow / enforcement profiles on, scope them, toggle auto-remediation | `PUT /api/v1/filevault` |
| Ask every Mac missing a key for SecurityInfo now | `POST /api/v1/filevault/check` |
| Escrow status for one Mac (never the key) | `GET /api/v1/computers/{id}/filevault` |
| Reveal a key (admin only, audited, fires `FileVaultKeyViewed`) | `POST /api/v1/computers/{id}/filevault/reveal` `{"reason": "…"}` |
| Reveal the keys a Mac reported before its current one (admin only, audited) | `POST /api/v1/computers/{id}/filevault/history/reveal` `{"reason": "…"}` |
| Run Escrow Buddy remediation on one Mac | `POST /api/v1/computers/{id}/filevault/remediate` |

A newly reported key never destroys the previous one: the last 10 replaced keys are kept (sealed like the current
key) so an unexpected report cannot lose a key that still unlocks the disk.

The optional **FileVault Enforcement** profile (`com.apple.MCX.FileVault2`) turns FileVault on with a personal
recovery key, deferred to the next login, so new keys are escrowed from the start.

## Study notes (Apple certification)

- PRK vs. institutional recovery key (IRK): this flow escrows the *personal* key. IRKs are a legacy certificate-based
  mechanism and are not used here.
- The escrow profile must be installed through MDM (device channel) for `FDE_PersonalRecoveryKeyCMS` to appear.
- `fdesetup changerecovery -personal` is the manual equivalent of what Escrow Buddy automates at login.
