# 4. Enrollment

Enrollment makes a device managed. In VibeMDM a Mac is usually enrolled **twice**, into two separate channels
that VibeMDM merges into one computer record:

| Channel | What enrolls it | Gives you |
| --- | --- | --- |
| **MDM** | The enrollment profile (SCEP + `com.apple.mdm` payloads), approved by the user | Profiles, MDM commands, DDM, FileVault escrow, MDM inventory |
| **Agent** | `vibemdm-agent enroll` (normally through the one-line installer) | Scripts, packages, policies, Self Service, compliance, onboarding, remote shell, detailed inventory |

The agent is matched to the MDM record by hardware UUID (which is the Mac's MDM UDID) or, failing that, serial
number.

## 4.1 Enrollment options without Apple Business Manager

| Path | Devices | Supervised? | How |
| --- | --- | --- | --- |
| User-approved profile enrollment | Mac | No (User Approved MDM) | Enrollment page link → install the profile → approve it in System Settings |
| Agent installer with `--mdm` | Mac | No (User Approved MDM) | `curl … \| sudo bash` installs the agent, then opens the enrollment profile for approval |
| Apple Configurator: Prepare | iPhone, iPad | **Yes** | Cable the device to a Mac running Apple Configurator and point it at VibeMDM's Configurator URL |
| Automated Device Enrollment | Mac, iPhone, iPad | Yes | Requires ABM/ASM; not supported by this deployment |

> **Study note (enrollment types):** Apple defines these enrollment methods. Know what each gives you.
>
> | Method | Ownership model | How it starts | Supervision | Notes |
> | --- | --- | --- | --- | --- |
> | **Automated Device Enrollment (ADE)** | Organization-owned | Device is assigned to an MDM server in ABM/ASM; enrollment happens in Setup Assistant | Yes | Zero-touch. Enrollment can be made mandatory and the MDM profile non-removable. Supports Await Configuration and skipping Setup Assistant panes |
> | **Device Enrollment** (profile-based, manual) | Organization-owned or shared | User installs a profile (web, e-mail, Configurator) | macOS: no. iOS: only via Configurator | On macOS this becomes *User Approved MDM* when the user approves it in System Settings |
> | **Account-driven Device Enrollment** | Organization-owned | User signs in with a Managed Apple Account in Settings/System Settings | No | Newer (iOS 17, macOS 14 era). Uses service discovery on the organization's domain |
> | **User Enrollment** (account-driven) | Personal (BYOD) | User signs in with a Managed Apple Account | No | Privacy-preserving: the MDM cannot see personal data or the device serial number, and gets a limited command set |
>
> VibeMDM implements Device Enrollment. ADE and the account-driven methods need ABM/ASM and a Managed Apple
> Account infrastructure.

## 4.2 Before you enroll anything

| Check | Where | Why |
| --- | --- | --- |
| Server URL is `https://…` | Settings → General | Macs refuse plain-HTTP MDM servers |
| APNs push certificate uploaded | Settings → Push certificate | The enrollment profile needs the topic |
| Agent binaries present | Enrollment page status (`GET /api/v1/enrollment/status`), Health → *Agent binaries* | The installer downloads `vibemdm-agent-darwin-{arm64,amd64}` from the server |
| TLS certificate trusted by macOS | Browse to the URL in Safari on a Mac | Enrollment fails with an untrusted certificate |

`GET /api/v1/enrollment/status` returns the configuration state and a list of `issues` in plain language.

## 4.3 Invitations and the enrollment secret

Two kinds of token authorize enrollment:

| Token | Scope | Recommended use |
| --- | --- | --- |
| **Invitation** | Per link, with optional `max_uses` and expiry; can be disabled | Everyday enrollment: one per batch, per department or per user |
| **Agent enrollment secret** | Master secret, never expires; admin-only | Scripted deployments; rotate it if it leaks |

### Create an invitation

1. Open **Enrollment** and choose **New invitation**.
2. Enter a name, an optional maximum number of uses and an optional expiry in days.
   API: `POST /api/v1/enrollment/invitations` `{"name": "Finance laptops", "max_uses": 20, "expires_days": 14}`.
3. Copy the three URLs from the response. **The token is shown only once**; it is stored hashed.

| Field in the response | Example | Use |
| --- | --- | --- |
| `enroll_url` | `https://mdm.example.com/enroll?token=…` | Send to the user. Opens the enrollment page |
| `install_command` | `curl -fsSL "https://mdm.example.com/agent/install.sh?token=…" \| sudo bash` | Run in Terminal on the Mac |
| `configurator_url` | `https://mdm.example.com/enroll/configurator/…` | Apple Configurator's MDM server URL (iPhone/iPad) |

Manage invitations with `GET /api/v1/enrollment/invitations`, `PATCH …/{id}` `{"enabled": false}` and
`DELETE …/{id}`.

### How uses are counted

| Action | Consumes a use? |
| --- | --- |
| Opening `/enroll?token=…` (the page) | No (checked only) |
| Downloading the profile from `/enroll/profile?token=…` | **Yes** |
| Agent enrollment with the invitation token (`POST /agent/v1/enroll`) | **Yes** |
| Fetching `install.sh?token=…` | No (checked only, so the script fails fast on a bad token) |
| Apple Configurator URL | No (checked only, because Configurator downloads once per batch) |
| Agent `--mdm` profile download | No (uses the agent's device token, not the invitation) |

A Mac that downloads the profile from the page **and** then installs the agent uses **two** uses. The installer
route (`install.sh` with `--mdm`) uses one. Set `max_uses` accordingly.

### Open enrollment

With `open_enrollment` on (Settings → Enrollment), anyone who can reach `/enroll` can download an enrollment profile
without a token. The agent still needs a token. Leave it off unless the server is reachable only from a trusted
network.

### Rotate the enrollment secret

`POST /api/v1/settings/rotate-enrollment-secret` (admin) issues a new secret. Existing agents are unaffected
(they use their own device tokens); scripts that embed the old secret stop working.

## 4.4 Enroll a Mac: user-approved profile enrollment

1. Send the user the invitation's `enroll_url`.
2. On the Mac, the user opens it in Safari and clicks the download button. The profile
   (`VibeMDM Enrollment`, identifier `com.vibemdm.enrollment` by default) is downloaded.
3. The user opens **System Settings → General → Device Management** (on older macOS: System Preferences →
   Profiles), selects the downloaded profile, clicks **Install** and authenticates with an administrator account.
4. macOS generates the device key, gets an identity from `/scep`, then sends `Authenticate` and `TokenUpdate` to
   `/mdm`. The computer appears in **Computers** as MDM-enrolled.
5. About two seconds later VibeMDM queues `DeviceInformation`, `SecurityInfo`, `ProfileList` and
   `InstalledApplicationList`, installs every in-scope profile and sends `DeclarativeManagement`.
6. The enrollment page also shows the agent install command for invitation links. Run it next
   ([§4.5](#45-the-agent-installer)).

What the enrollment profile contains:

| Payload | Key settings |
| --- | --- |
| `com.apple.security.scep` | `URL` = `<server>/scep`, `Challenge` (one-time), `Keysize` 2048, `KeyType` RSA, `Key Usage` 5, `Retries` 3, `RetryDelay` 10 |
| `com.apple.mdm` | `ServerURL` and `CheckInURL` = `<server>/mdm`, `Topic`, `IdentityCertificateUUID`, `AccessRights` 8191, `SignMessage` true, `CheckOutWhenRemoved` true, `ServerCapabilities` (`com.apple.mdm.per-user-connections`, `com.apple.mdm.bootstraptoken`, `com.apple.mdm.token`) |
| Top level | `PayloadType` Configuration, `PayloadScope` System, `PayloadRemovalDisallowed` false; optionally signed |

> **Study note:** `AccessRights` is a bit mask; 8191 grants every right (inspect profiles, install/remove
> profiles, device lock and passcode removal, erase, query device information and network information,
> inspect/install/remove provisioning profiles, inspect applications, restrictions, security queries, settings,
> app management). `ServerCapabilities` with `com.apple.mdm.bootstraptoken` tells macOS that the server can
> escrow a **bootstrap token**. After a user with a Secure Token logs in, macOS sends it with `SetBootstrapToken`,
> and VibeMDM records `bootstrap_token_escrowed` on the computer.

## 4.5 The agent installer

From the **Enrollment** page (or an invitation) copy the install command and run it in Terminal on the Mac:

```bash
curl -fsSL "https://mdm.example.com/agent/install.sh?token=…" | sudo bash
```

The server generates the script on the fly. It:

1. refuses to run unless it is root;
2. detects the architecture (`arm64` → `darwin-arm64`, `x86_64` → `darwin-amd64`);
3. downloads the binary from `/agent/download/<platform>` to `/usr/local/bin/vibemdm-agent` and removes the
   `com.apple.quarantine` attribute;
4. runs `VIBEMDM_ENROLL_TOKEN=… vibemdm-agent enroll --server <server URL> --mdm`. The token goes through an
   environment variable, not the command line, so other users cannot see it with `ps`.

The script uses the **server URL from settings**, never the request's Host header. It refuses to run until a server
URL is configured (HTTP 503). Add `&mdm=0` to the URL to skip the MDM step. If the server URL is not HTTPS (a lab),
the script adds `--allow-insecure-http`.

`vibemdm-agent enroll` then:

1. collects the serial number, hardware UUID, host name, computer name, OS version and architecture;
2. calls `POST /agent/v1/enroll` with the token (and the previous device token, if a config already exists);
3. writes `/Library/Application Support/VibeMDM/agent.json` (mode 0600: server URL, `vmda_…` device token,
   computer ID, org name, check-in interval);
4. copies itself to `/usr/local/bin/vibemdm-agent` if needed and installs and loads:
   - `/Library/LaunchDaemons/com.vibemdm.agent.plist` (`vibemdm-agent daemon`, `RunAtLoad`, `KeepAlive`, output
     to `/Library/Logs/VibeMDM/vibemdm-agent.launchd.log`, which only catches crashes; the agent writes its own
     log to `/Library/Logs/VibeMDM/vibemdm-agent.log`, see [§18.2](18-logging-and-diagnostics.md#182-agent-logs));
   - `/Library/LaunchAgents/com.vibemdm.agent.login.plist` (`vibemdm-agent trigger login` in each Aqua session);
5. with `--mdm`, if the Mac is not yet MDM-enrolled, downloads the enrollment profile with its device token, saves
   it with an exclusive create as `/Users/Shared/VibeMDM-Enrollment-*.mobileconfig`, opens it for the logged-in
   user and opens the Device Management pane. The user still has to approve it.

The agent is not code-signed or notarized by default. If you have a Developer ID, sign and notarize the binaries
and place them in `VIBEMDM_AGENT_DIR`.

## 4.6 Automatic agent install through MDM

Like Jamf Pro after user-initiated enrollment, VibeMDM can install the agent itself as soon as a Mac finishes MDM
enrollment, so nobody runs a Terminal command. Turn it on under **Enrollment → Install the agent automatically**.

**One-time setup: build the signed package.** macOS installs packages sent with `InstallEnterpriseApplication` only
if they are signed by a certificate it trusts. On any Mac (the build Mac does not need to be enrolled):

```bash
VIBEMDM_URL=https://mdm.example.com VIBEMDM_API_TOKEN=<admin API token> ./deploy/macos/build-agent-pkg.sh
```

The script downloads both agent binaries from your server, builds a product archive (`pkgbuild` + `productbuild`)
whose postinstall copies the right binary to `/usr/local/bin/vibemdm-agent` and runs
`vibemdm-agent enroll --from-mdm`, creates a key and a certificate request on that Mac, has VibeMDM's
**package-signing CA** sign it (`POST /api/v1/agent-package/sign-csr`; the private key never leaves the Mac), signs
with `productsign` from a throwaway keychain, and uploads the result (`PUT /api/v1/agent-package?version=…`). The
server refuses unsigned packages. Set `SIGN_IDENTITY` to sign with your own Developer ID Installer certificate
instead. Re-run the script after upgrading VibeMDM.

**What happens at enrollment** (and when you click **Install agent** on a computer, or the Health finding
*Agent not installed* offers *Install the agent through MDM*):

1. VibeMDM sends a temporary **bootstrap profile** (`com.vibemdm.agent.bootstrap`) with the package-signing root
   (`com.apple.security.root`) and managed preferences for `com.vibemdm.agent`: the server URL and a **single-use
   token** that expires in 24 hours and only works for that Mac's UDID or serial number.
2. It sends `InstallEnterpriseApplication` pointing at `/agent/pkg/manifest.plist` (chunked MD5s of the package).
3. macOS downloads `/agent/pkg/VibeMDM-Agent.pkg`, checks the signature against the root it now trusts and installs.
4. The postinstall enrolls the agent with the token (log: `/Library/Logs/VibeMDM/install.log`).
5. When the agent enrolls, VibeMDM removes the bootstrap profile, so the Mac trusts the package-signing root only for
   the minutes the install takes.

The package-signing CA can only issue code-signing certificates: its path length is 0 and its name constraints
permit only the DNS name `invalid.`, so it cannot vouch for a web server even while a Mac trusts it. It is created
on first use, sealed with `VIBEMDM_ENCRYPTION_KEY` and never replaced automatically.

> **Untested on a real Mac.** The simulator exercises the whole protocol (profile, manifest, MD5 chunks, signature
> presence, single-use token, profile removal), but whether macOS accepts a package signed by a root delivered this
> way, `productsign` with a non-Apple certificate, and the postinstall timing must be confirmed on hardware.
> Jamf Pro uses the same mechanism (packages signed by its built-in CA, trusted through the MDM profile).

> **Study note:** `InstallEnterpriseApplication` takes a `ManifestURL` (or an inline `Manifest`) in the same format
> as iOS enterprise app manifests: `items[0].assets[0]` with `kind` `software-package`, `url`, and `md5-size` plus
> `md5s` for chunked integrity, and `metadata` with `bundle-identifier` and `bundle-version`. The package must be a
> signed product archive (`productbuild`), not a bare component package. Automated Device Enrollment can also run
> such packages during Setup Assistant (`InstallApplication` with `PrestageEnrollment` in Jamf terms).

## 4.7 iPhone and iPad with Apple Configurator

VibeMDM's console and agent focus on Macs. iPhones and iPads enrolled this way get MDM-level management
(profiles, commands, `DeviceInformation` inventory) but none of the agent features.

1. In **Enrollment**, create an invitation (or use the default enrollment URL) and copy the **Apple Configurator
   URL**: `https://mdm.example.com/enroll/configurator/<token>`. The token is in the path because Configurator
   stores the URL verbatim; the server log redacts it as `/enroll/configurator/[redacted]`.
2. On a Mac, open **Apple Configurator** and connect the device with a cable.
3. Choose **Prepare → Manual Configuration** and turn on **Supervise devices**.
4. At **Enroll in MDM Server**, choose **New server…**, give it a name and paste the Configurator URL. If your TLS
   certificate is not publicly trusted, add its anchor certificate on the next screen.
5. Skip the Apple Business Manager sign-in. Create or choose a **supervision identity** and keep it safe: it is
   what lets that Mac pair with the supervised device again later.
6. Finish. The device is **erased**, comes back supervised and enrolls in VibeMDM during Setup Assistant.

The profile served at this URL embeds a SCEP challenge that can be reused for 24 hours, because one profile
prepares a whole batch.

## 4.8 Takeover protection and Reset agent

UDIDs and serial numbers are not secrets. Without protection, anyone with an enrollment token could enroll a fake
agent with a victim Mac's serial number and receive its tasks. VibeMDM therefore lets a new agent enrollment
replace an existing computer's agent only when:

1. it presents that agent's **current device token** (a reinstall on the same Mac: `enroll` reads the old
   `agent.json` and sends it as `previous_device_token`); or
2. an admin clicked **Reset agent** on the computer (`POST /api/v1/computers/{id}/reset-agent`), which revokes
   the old token at once; or
3. the old agent has been silent for **30 days**.

A refused attempt returns **HTTP 409** and is audited as `agent.enroll_refused`.

**When to use Reset agent:**

- after erasing and reinstalling macOS on a Mac (the old `agent.json` is gone);
- when moving a Mac to a new user and reinstalling the agent from scratch;
- when an agent's token may have been copied.

After a reset, the old agent's requests return 401 and it logs `device token rejected by server; re-enroll the
agent`. Re-run the install command.

## 4.9 Unenrolling

| Goal | How |
| --- | --- |
| Remove MDM management | Computer → Management → **Remove MDM Profile** (`POST /api/v1/computers/{id}/commands` `{"type": "RemoveMDMProfile"}`), which sends `RemoveProfile` for the enrollment profile identifier. Or the user removes it in System Settings. The Mac sends `CheckOut` and VibeMDM marks it unenrolled |
| Remove the agent | On the Mac: `sudo vibemdm-agent remove-framework` (unenrolls on the server, unloads launchd jobs, deletes the support directory) |
| Forget the computer | `DELETE /api/v1/computers/{id}`. Do this only after unenrolling; a still-enrolled Mac or agent will recreate a record |

On a User Approved Mac, the user **can** remove the enrollment profile. VibeMDM records the `CheckOut`
(`ComputerUnenrolled` webhook) and the agent keeps reporting until it is removed too, so you can detect and
re-enroll such Macs (for example with a smart group *MDM enrolled is false* and *Agent enrolled is true*).

## 4.10 Verify enrollment on the Mac

```bash
profiles status -type enrollment
# Enrolled via DEP: No
# MDM enrollment: Yes (User Approved)

sudo profiles show -type enrollment          # shows the enrollment (ADE) configuration, if any
sudo profiles list                           # installed profiles
sudo vibemdm-agent check-connection          # agent → server reachability with the device token
log stream --info --predicate 'process == "mdmclient"'   # live MDM traffic
```

## 4.11 Study notes: supervision, User Approved MDM, Activation Lock, bootstrap token

> **Study note (User Approved MDM):** on macOS 10.13.4 and later, an MDM enrollment is *user approved* if the user
> approved it in the Profiles/Device Management pane, or if it came from ADE (which is implicitly approved). UAMDM
> is required for, among other things: kernel extension and system extension policies, PPPC (privacy) payloads,
> and bootstrap-token escrow. Profiles installed by a script or the `profiles` command cannot achieve user
> approval; this is deliberate, so that malware cannot silently grant itself MDM rights.

> **Study note (supervision):** supervision is a device state that gives the organization more control.
> - **iPhone and iPad** become supervised through ADE or through Apple Configurator's *Prepare* (which erases the
>   device). Supervision unlocks many restrictions, silent app installs, Single App Mode, and more.
> - **Mac** becomes supervised **only through ADE**: Macs enrolled by ADE on macOS 11 or later are supervised.
>   Apple Configurator for Mac cannot supervise a Mac by itself. *Apple Configurator for iPhone* can add a Mac to
>   ABM/ASM (so that it can then use ADE), but that still needs an ABM/ASM account.
> - What needs supervision on macOS includes reliable non-removable MDM, Activation Lock management, several
>   restrictions, and some DDM software update features. A User Approved Mac still gets most management:
>   profiles including PPPC and system extensions, FileVault escrow, inventory, lock, erase, remote desktop and OS
>   update commands, and bootstrap-token escrow.
> - The `DeviceInformation` query `IsSupervised` reports the state; VibeMDM shows it as `is_supervised`.

> **Study note (Activation Lock):** Activation Lock ties a device to the user's Apple Account through Find My. On a
> Mac it requires Apple silicon or a T2 Security Chip (macOS 10.15 or later). For organization-owned devices, the
> MDM can obtain an **Activation Lock bypass code** only for **supervised** devices, and can allow or disallow
> Activation Lock with a restriction. ABM/ASM can also manage Activation Lock for organization-owned devices.
> Because VibeMDM's Macs are not supervised, it cannot manage Activation Lock; `IsActivationLockEnabled` from
> `DeviceInformation` is reported only for visibility (`activation_lock_enabled`).

> **Study note (bootstrap token):** introduced in macOS 10.15, the bootstrap token is generated when a user with a
> Secure Token logs in on an MDM-enrolled Mac whose server advertises `com.apple.mdm.bootstraptoken`. It is escrowed
> to the MDM (`SetBootstrapToken`) and used by macOS (`GetBootstrapToken`) to: grant a Secure Token to other
> accounts that log in (for example mobile or newly created accounts); and on Apple silicon, to authorize
> kernel-extension loading and software updates without a user's password in some flows. Commands:
> `sudo profiles status -type bootstraptoken` and `sudo profiles install -type bootstraptoken`. The details of which
> operations use it have changed across macOS versions; check the *Apple Platform Deployment* guide for your
> version.
>
> Two different things are easy to confuse. **Escrowed** means the Mac sent `SetBootstrapToken` and the server holds
> the token (VibeMDM shows *Bootstrap token escrowed*, from what NanoMDM stored). `SecurityInfo`'s
> **`BootstrapTokenAllowedForAuthentication`** (`allowed`, `disallowed`, `not supported`) is instead whether the
> Secure Enclave on an Apple silicon Mac lets the bootstrap token authorize software updates and kernel extensions.
> It is set automatically for Automated Device Enrollment and can be changed in recoveryOS; `disallowed` is normal
> for a user-approved enrollment and says nothing about escrow. With user-approved enrollment the automatic escrow at
> login is not always reliable; `sudo profiles install -type bootstraptoken` escrows it on demand.

Next: [Chapter 5 — Inventory and groups](05-inventory-and-groups.md).
