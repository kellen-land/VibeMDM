# Enrollment without Apple Business Manager

Apple Business Manager (ABM) is what makes enrollment automatic and devices **supervised**. ABM itself is free, but it
requires a legal organization with a D-U-N-S number. Without it, these are the options and what each one gives you.

| Path | Devices | Supervised? | How |
| --- | --- | --- | --- |
| **User-approved profile enrollment** | Mac | No (User Approved MDM) | Enroll page link → install the profile → approve it in System Settings |
| **Agent installer with `--mdm`** | Mac | No (User Approved MDM) | `curl … | sudo bash` installs the agent, then opens the enrollment profile for approval |
| **Apple Configurator: Prepare** | iPhone, iPad | **Yes** | Cable the device to a Mac running Apple Configurator and point it at VibeMDM's enrollment URL |
| Automated Device Enrollment | Mac, iPhone, iPad | Yes | Requires ABM (not covered here) |

## Macs: what you can and cannot do

Apple Configurator cannot supervise a Mac on its own. Mac supervision only comes from Automated Device Enrollment
through ABM. (Apple Configurator for iPhone can *add* a Mac to ABM, but that still needs an ABM account.)

A Mac enrolled by profile is **User Approved**: the user approved the MDM profile. That unlocks most management:

- configuration profiles, including Privacy Preferences (PPPC), system and kernel extensions, and managed login items
- FileVault recovery key escrow, inventory, restart, lock, erase, remote desktop, and OS update commands
- bootstrap token escrow (macOS 11+)

What needs supervision (ADE) on macOS: blocking profile removal reliably, Activation Lock management, some
restrictions and DDM software-update enforcement features, and zero-touch setup. On a User Approved Mac the user can
remove the enrollment profile; VibeMDM records the CheckOut and the agent keeps reporting until it is removed too.

## iPhone and iPad with Apple Configurator

1. In VibeMDM, open **Enrollment** and create an invitation (or use the default enrollment URL). Copy the
   **Apple Configurator URL**: `https://mdm.example.com/enroll/configurator/<token>`.
2. On a Mac, open Apple Configurator and connect the device by USB.
3. Choose **Prepare → Manual Configuration**. Turn on **Supervise devices**.
4. At **Enroll in MDM Server**, choose **New server…**, give it a name, and paste the Configurator URL. If your server
   uses a certificate macOS doesn't already trust, add its anchor certificate on the next screen.
5. Skip the Apple Business Manager sign-in. Create or choose a **supervision identity** and keep it safe: it is what
   lets that Mac pair with the supervised device again later.
6. Finish. The device is erased, comes back supervised, and enrolls in VibeMDM during setup.

The URL serves the same enrollment profile as the web enroll page. Invitations are checked but not consumed by this
URL, because Configurator downloads the profile once per batch rather than once per device.

> VibeMDM's console and agent focus on Macs. iPhones and iPads enrolled this way get MDM-level management (profiles,
> commands, inventory from DeviceInformation) but no agent features such as scripts and policies.

## Study notes (Apple certification)

- **User Approved MDM** vs **supervised**: approval is per enrollment and unlocks kernel/system extension and PPPC
  payloads on macOS; supervision is a device state granted by ADE (Mac) or ADE/Configurator (iOS).
- The enrollment profile is a `Configuration` profile with a `com.apple.security.scep` payload (device identity) and a
  `com.apple.mdm` payload (`ServerURL`, `CheckInURL`, `Topic`, `IdentityCertificateUUID`, `AccessRights`).
- `AccessRights 8191` grants every MDM right; `SignMessage = true` makes the device sign each request with its identity
  (the `Mdm-Signature` header VibeMDM verifies).
