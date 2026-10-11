# 23. Study guide

This chapter condenses the manual into a revision plan for Apple's two IT certifications and turns VibeMDM into a
lab. It maps each exam topic area to the chapters that teach it and to a hands-on exercise ("Try this in VibeMDM").
Exercises marked **simulator** need only the server and `vibemdm-sim` ([§21.3](21-cli-reference.md#213-vibemdm-sim));
those marked **real Mac** need a Mac you can enroll and, ideally, erase.

| Certification | Exam | What it covers | Best chapters |
| --- | --- | --- | --- |
| **Apple Certified Support Professional (ACSP)** | *Apple Device Support* | Supporting Macs, iPhones and iPads: setup, accounts, file systems, data, networking, security and privacy, apps, troubleshooting | 1, 3, 5, 7, 8, 11, 18, 20, 22 |
| **Apple Certified IT Professional (ACITP)** | *Apple Deployment and Management* | Deploying and managing Apple devices at scale: Apple Business Manager / Apple School Manager, MDM, enrollment, profiles, DDM, apps, updates, identity, security | 1, 3, 4, 6, 8, 10, 11, 12, 13, 14, 16 |

Apple publishes an exam preparation guide for each exam with the current objectives. Objectives change between
releases of macOS and iOS; this chapter follows the general areas of the Apple Training courses and does not claim to
mirror a particular edition. Where this manual says "check Apple's documentation", do so before the exam.

> **Study note (how the exams differ):** ACSP tests you as the person at the desk with the user: you need to know
> *where* a setting is, *what* a tool shows and *why* a Mac behaves as it does. ACITP tests you as the administrator
> of a fleet: you need to know *which* Apple service or protocol provides a capability, *what it requires* (ABM,
> supervision, UAMDM, a Managed Apple Account, a particular OS version) and *in which order* things happen.
> VibeMDM is a good lab for both because it exposes the protocol and the device behaviour, but remember what it
> cannot show you: Automated Device Enrollment, Apps and Books, Managed Apple Accounts and Shared iPad all need
> ABM/ASM.

## 23.1 How to use this guide

1. Read the chapter listed for a topic, skipping the procedures and reading the **Study notes**.
2. Do the lab. Watching the real protocol messages makes the facts stick.
3. Fill in the gaps from Apple's own references (§23.6). The exams are written from those, not from any third-party
   manual.
4. Use the fact tables in §23.5 for a final pass.

## 23.2 ACSP: Apple Device Support

| Topic area | What to know | Where in this manual | Try this in VibeMDM |
| --- | --- | --- | --- |
| **macOS basics and setup** | Setup Assistant and its panes; System Settings layout; Apple silicon vs Intel differences (recoveryOS, Startup Security Utility, volume owners, no Target Disk Mode); macOS version numbering and the yearly release cycle; Rapid Security Responses and background security updates | [§1.6](01-introduction.md#16-glossary) glossary, [§10.8](10-software-updates.md#108-study-notes), [§20.7](20-security-hardening.md#207-study-notes-macos-platform-security), [§22.13](22-troubleshooting.md#2213-study-notes-diagnosing-a-mac) | **Real Mac:** run `sw_vers`, `system_profiler SPHardwareDataType` and compare with what VibeMDM's inventory shows after `sudo vibemdm-agent recon` (Lab 9) |
| **User accounts** | Account types: administrator, standard, sharing-only, guest, the hidden `root`; **Secure Token** and **volume ownership**; password reset paths (another admin, Apple Account, FileVault recovery key, recoveryOS `resetpassword`); login keychain and the password-change rules; fast user switching; `sysadminctl` and `dscl` | [§11.7](11-filevault.md#117-study-notes-filevault-and-secure-token), [§4.10](04-enrollment.md#410-study-notes-supervision-user-approved-mdm-activation-lock-bootstrap-token) (bootstrap token), [§5.2](05-inventory-and-groups.md#52-inventory-sources-mdm-vs-agent) (local users in inventory) | **Real Mac:** Lab 10 (Secure Token). In the computer's inventory, find the local accounts list with the admin flag and Secure Token status that the agent collected with `dscl` and `sysadminctl -secureTokenStatus` |
| **File systems and storage (APFS)** | APFS containers, volumes and volume groups (System and Data), space sharing, snapshots, the Signed System Volume; Disk Utility and First Aid; permissions (POSIX, ACLs, the `everyone` entry); FileVault's relationship to APFS | [§11.7](11-filevault.md#117-study-notes-filevault-and-secure-token), [§20.7](20-security-hardening.md#207-study-notes-macos-platform-security) (SSV) | **Real Mac:** `diskutil apfs list` and `diskutil list`; note which volume FileVault encrypts and compare with `fdesetup status` and the `filevault_enabled` field in VibeMDM |
| **Data management and backup** | Time Machine (local snapshots, destinations, exclusions, restore), iCloud Drive and Desktop & Documents sync, Migration Assistant, the difference between a backup and sync, what MDM can and cannot restore | No VibeMDM feature; see the study notes in [§2.7](02-installation.md#27-backups) for the server-side analogy | **Real Mac:** `tmutil listlocalsnapshots /` and `tmutil destinationinfo`; think about which parts of a managed Mac a reinstall recreates (profiles, apps, agent) and which only a backup covers (user data) |
| **Networking** | TCP/IP, DHCP, DNS, network locations and service order, Wi-Fi payloads, VPN types, proxies, the application firewall, Private Relay and Private Wi-Fi Address; the hosts and ports Apple devices need (APNs on TCP 5223 with 443 fallback, 17.0.0.0/8) | [§2.1](02-installation.md#21-requirements) study note, [§22.3](22-troubleshooting.md#223-apns-and-pushes) | **Real Mac:** with a command queued in VibeMDM, run `nettop -m tcp` or `lsof -i :5223` to see `apsd`'s connection to Apple, then watch the command arrive in the inspector (Lab 1) |
| **Security and privacy** | Gatekeeper and notarization, XProtect and XProtect Remediator, SIP, FileVault and recovery keys, TCC/PPPC (which services the user must grant), Find My and Activation Lock, Lockdown Mode, passwords and passkeys, the firewall, Keychain and iCloud Keychain, certificate trust | [§20.7](20-security-hardening.md#207-study-notes-macos-platform-security), [§8.5](08-configuration-profiles.md#85-study-notes-common-payload-types) (PPPC), [§3.4](03-apns-and-certificates.md#34-the-tls-certificate) (trust), [§9.6](09-apps-installomator.md#96-why-arguments-are-single-quoted) (Team IDs), [§11.7](11-filevault.md#117-study-notes-filevault-and-secure-token) | **Simulator:** Lab 6 (FileVault escrow). **Real Mac:** deploy a PPPC profile for an app you use and observe which services it can grant (Full Disk Access) and which it cannot (Camera); then `tccutil reset All <bundleID>` and watch the prompts return |
| **Apps and processes** | App Store vs Developer ID vs unsigned apps; installer packages and receipts (`pkgutil`); login items and background items (`com.apple.servicemanagement`); `launchd` daemons vs agents; Activity Monitor; force quit; Rosetta 2 | [§7.4](07-policies-scripts-packages.md#74-packages), [§8.5](08-configuration-profiles.md#85-study-notes-common-payload-types), [§21.2](21-cli-reference.md#212-vibemdm-agent) (the agent's LaunchDaemon and LaunchAgent) | **Real Mac:** `sudo launchctl print system/com.vibemdm.agent` and `launchctl print gui/$(id -u)/com.vibemdm.agent.login`. Explain why the first runs as root and the second in the user's session |
| **Troubleshooting** | Apple's troubleshooting method; Safe Mode, recoveryOS, Apple Diagnostics; Console and the unified log (`log show`, `log stream`, predicates); `sysdiagnose`; startup sequence and the login process; reading crash reports; when to reinstall | [Chapter 22](22-troubleshooting.md), [§18.7](18-logging-and-diagnostics.md#187-study-notes-logs-on-the-mac) | **Real Mac:** Lab 11 (logs and diagnostics). **Simulator:** break the server on purpose and fix it from Health (Lab 12) |

## 23.3 ACITP: Apple Deployment and Management

| Topic area | What to know | Where in this manual | Try this in VibeMDM |
| --- | --- | --- | --- |
| **Apple Business Manager / Apple School Manager** | What they are; roles; adding devices (reseller, Apple Configurator for iPhone, purchase); assigning devices to an MDM server; **Managed Apple Accounts** and federation (Entra ID, Google Workspace, custom IdPs); **Apps and Books** (VPP) licences and location-based assignment; Shared iPad and Classroom (ASM) | [§1.1](01-introduction.md#11-what-vibemdm-is-and-is-not) (what VibeMDM lacks), [§1.6](01-introduction.md#16-glossary), [§4.1](04-enrollment.md#41-enrollment-options-without-apple-business-manager), [§6.4](06-scoping.md#64-deleted-objects-in-a-scope) study note | None directly; VibeMDM has no ABM integration. Read the *Apple Platform Deployment* chapters on ABM and note, for each VibeMDM feature, which ABM capability would improve it (ADE for enrollment, Apps and Books for apps) |
| **The MDM protocol** | APNs and the push certificate (topic, yearly renewal, same Apple Account); the check-in protocol (`Authenticate`, `TokenUpdate`, `CheckOut`, bootstrap token messages); the command protocol (`Idle`, `Acknowledged`, `Error`, `NotNow`, `CommandFormatError`); device vs user channel; SCEP/ACME identities and `Mdm-Signature`; what an MDM server must host (HTTPS with a trusted certificate) | [§1.3](01-introduction.md#13-how-the-pieces-talk), [Chapter 3](03-apns-and-certificates.md), [§16.3](16-mdm-commands-and-inspector.md#163-study-notes-an-installprofile-round-trip-on-the-wire) | **Simulator:** Lab 1 (watch an enrollment and a command on the wire), Lab 2 (push certificate) |
| **Enrollment types** | Automated Device Enrollment (ABM, Setup Assistant, supervision, non-removable profile, Await Configuration, enrollment customization); Device Enrollment by profile (User Approved MDM on the Mac); account-driven Device Enrollment and **User Enrollment** with Managed Apple Accounts (service discovery, data separation, limited commands); Apple Configurator for iPhone/iPad (supervision, anchor certificates); what supervision unlocks on each platform | [Chapter 4](04-enrollment.md), especially [§4.1](04-enrollment.md#41-enrollment-options-without-apple-business-manager) and [§4.10](04-enrollment.md#410-study-notes-supervision-user-approved-mdm-activation-lock-bootstrap-token) | **Real Mac:** Lab 9 (UAMDM). If you have a spare iPhone or iPad, supervise and enroll it with Apple Configurator ([§4.6](04-enrollment.md#46-iphone-and-ipad-with-apple-configurator)) and compare `IsSupervised` with the Mac's |
| **Configuration profiles and payloads** | Profile anatomy (`PayloadIdentifier`, `PayloadUUID`, scope); the common payloads and what each needs (PPPC and extension policies need UAMDM; many restrictions need supervision on iOS); managed preferences (MCX vs typed payloads); signing; removal rules; precedence when several profiles set the same key | [Chapter 8](08-configuration-profiles.md) | **Simulator:** Lab 3 (install a profile, read an error). **Real Mac:** build a managed-preference profile from a JSON schema and verify it with `defaults read /Library/Managed\ Preferences/<domain>` |
| **Declarative Device Management** | Why DDM exists (device-driven, status channel); declaration types (configuration, asset, activation, management); predicates; server tokens and the declarations token; the `DeclarativeManagement` command and check-in endpoints; software-update enforcement and settings declarations; what still needs classic MDM | [§1.3](01-introduction.md#13-how-the-pieces-talk), [§10.2](10-software-updates.md#102-how-vibemdm-serves-ddm), [§10.8](10-software-updates.md#108-study-notes) | **Simulator:** Lab 5 (enforcement plan; watch `tokens`, `declaration-items`, `declaration/…` and `status` requests) |
| **App distribution** | Apps and Books (device vs user assignment, Managed Apple Accounts not required for device assignment); Managed apps and `InstallApplication`; `InstallEnterpriseApplication` and manifests; in-house apps; Developer ID signing and notarization; installer packages; third-party tools such as Installomator; Managed Open In and data separation on iOS | [Chapter 7](07-policies-scripts-packages.md), [Chapter 9](09-apps-installomator.md), [§16.1](16-mdm-commands-and-inspector.md#161-sending-commands) | **Simulator:** Lab 4 (policy with a package and an Installomator app). **Real Mac:** `codesign -dv --verbose=4` and `spctl -a -vv` on an app Installomator installed; find the Team ID that the label pins |
| **Software updates** | DDM enforcement (target version, deadline, reminders) and settings (deferrals up to 90 days, automatic actions); the deprecated MDM commands and restriction keys; Apple silicon authorization (volume owner or bootstrap token); Rapid Security Responses and their successors; `softwareupdate` CLI; how deferrals affect what the user sees | [Chapter 10](10-software-updates.md) | **Simulator:** Lab 5. **Real Mac:** create a settings plan with a 7-day minor deferral and check System Settings → General → Software Update before and after |
| **FileVault, Secure Token, bootstrap token** | Enabling FileVault by profile (`Defer`), personal vs institutional recovery keys, escrow with `FDERecoveryKeyEscrow` and `SecurityInfo`, key rotation, Secure Token and who gets it, volume owners on Apple silicon, the bootstrap token's roles (Secure Token for new accounts, kernel extensions and updates on Apple silicon) | [Chapter 11](11-filevault.md), [§4.10](04-enrollment.md#410-study-notes-supervision-user-approved-mdm-activation-lock-bootstrap-token) | **Simulator:** Lab 6. **Real Mac:** Lab 10 |
| **Identity and SSO** | Extensible SSO (`com.apple.extensiblesso`) with redirect and credential extensions; the **Kerberos SSO extension** versus binding to Active Directory; **Platform SSO** (registration, password vs Secure Enclave vs smart card methods, what each macOS version added); third-party login windows such as XCreds; Managed Apple Accounts with federation; certificate-based authentication (SCEP, ACME, smart cards) | [Chapter 13](13-identity-xcreds.md), [§3.2](03-apns-and-certificates.md#32-the-scep-ca-and-device-identities) | **Real Mac (optional, erasable):** configure XCreds against a test tenant ([§13.5](13-identity-xcreds.md#135-testing-advice)) and note which account properties (Secure Token, admin) the created account has. Compare with what Platform SSO would do, from Apple's documentation |
| **Compliance and security baselines** | Security frameworks (NIST 800-53, CIS, DISA STIG, CMMC) and the macOS Security Compliance Project; enforcing with profiles vs auditing with scripts; drift; organization-defined values; documenting exemptions; built-in protections that baselines check | [Chapter 12](12-compliance.md), [§20.7](20-security-hardening.md#207-study-notes-macos-platform-security) | **Simulator:** Lab 7 (baseline, drift on the second scan). **Real Mac:** scan a CIS Level 1 baseline without remediation and read the failing rules' discussions |
| **Inventory, reporting and automation** | What `DeviceInformation`, `SecurityInfo`, `ProfileList` and `InstalledApplicationList` return; the DDM status channel as the modern alternative; smart groups and scoping as vendor concepts; APIs and webhooks | [Chapter 5](05-inventory-and-groups.md), [Chapter 6](06-scoping.md), [Chapter 19](19-users-roles-api.md) | **Simulator:** Lab 4 (smart group). Build a saved search and export it as CSV |
| **Device commands and lifecycle** | Lock, erase (Erase All Content and Settings vs full erase), restart, Remote Desktop, Recovery Lock, Activation Lock bypass (supervised only), Lost Mode (iOS, supervised), returning devices to ABM, unenrollment and what the device keeps | [Chapter 16](16-mdm-commands-and-inspector.md), [§4.8](04-enrollment.md#48-unenrolling) | **Simulator:** send `DeviceLock` and `EraseDevice` to a simulated Mac and read the redacted PIN in the inspector; reveal it with `?reveal=1` and find the `mdm.command_reveal` audit entry |
| **Onboarding and the first-run experience** | Setup Assistant panes and skipping them with ADE, Await Configuration and `DeviceConfigured`, enrollment customization, third-party onboarding tools and why they need an agent | [Chapter 14](14-white-glove-onboarding.md) | **Simulator:** Lab 8 |

## 23.4 Labs

Each lab lists what you need, the steps and what to look for. The simulator labs assume a lab server on
`http://localhost:8080` set up as in [§2.10](02-installation.md#210-running-without-docker-development) or the Docker
deployment, with any certificate that has a `UID` attribute uploaded as the "push certificate" so enrollment profiles
can be generated ([§21.3](21-cli-reference.md#213-vibemdm-sim)). Copy the **static SCEP challenge** from Settings →
SCEP CA and the **agent enrollment secret** from Settings → General (both admin-only).

### Lab 1 (simulator): an enrollment and a command on the wire

*Exam links: MDM protocol, APNs, SCEP, check-in vs command channel.*

1. Start the server with `VIBEMDM_DUMP_MDM=1` so every MDM request and response is printed
   ([§2.3](02-installation.md#23-environment-variables)).
2. Enroll one simulated Mac:

   ```bash
   bin/vibemdm-sim -server http://localhost:8080 -n 1 -scep-challenge "<static challenge>" -token "<enrollment secret>"
   ```

3. In the server output, find in order: `GetCACert` and `PKIOperation` on `/scep`; the `Authenticate` check-in
   (topic, UDID, serial); `TokenUpdate` (token and `PushMagic`); the first `Idle` report. Match each to the
   description in [§16.3](16-mdm-commands-and-inspector.md#163-study-notes-an-installprofile-round-trip-on-the-wire).
4. In the console, open the computer, send **DeviceInformation** from the Management tab, and open the command in
   Activity → Commands. The inspector shows `queued`, `push_failed` (the simulator has no APNs), then `delivered` and
   `acknowledged` when the simulator's next poll picks it up (`-poll`, default 10 s).
5. Questions to answer: Why did the command arrive although the push failed? What would happen on a real Mac? Which
   message completed the enrollment, `Authenticate` or `TokenUpdate`?

### Lab 2 (simulator): the push certificate

*Exam links: APNs certificate lifecycle, topic, renewal.*

1. Open Settings → APNs (`GET /api/v1/settings/apns`) and note the certificate's `topic`, `not_after` and the Health
   finding `platform.apns_certificate` if it is close to expiry.
2. Download an enrollment profile (Enrollment page) and open it in a text editor. Find `Topic` in the `com.apple.mdm`
   payload and the `Challenge` in the SCEP payload. Confirm the topic equals the certificate's `UID`.
3. Explain, in one sentence each: why devices stop responding when the topic changes; why renewing with the same
   Apple Account keeps the topic; why the profile cannot be generated without the certificate.

### Lab 3 (simulator): install a profile and read an error

*Exam links: profile anatomy, `PayloadIdentifier`, MDM error responses, redaction.*

1. Create a simple profile (for example a `com.apple.screensaver` payload with `idleTime` 600) in a profile editor,
   export it unsigned, and upload it in Configuration Profiles with scope **All computers**.
2. Watch the deployment reach `installed` on the simulated Macs (`GET /api/v1/profiles/{id}/deployments`). The
   simulator tracks installed identifiers.
3. Send `RemoveProfile` with a made-up `identifier` from the Management tab. The simulator answers `Error` with
   `MCProfileErrorDomain` code 4001 ("not installed"). Read the `ErrorChain` in the inspector.
4. Edit the profile's payload (change `idleTime`) and save. Confirm that VibeMDM re-installed it under the **same**
   identifier, and explain why that replaces rather than duplicates the profile.
5. Open the `InstallProfile` command: the `Payload` is `[redacted … bytes]`. Reveal it with `?reveal=1` and find the
   `mdm.command_reveal` entry in the audit log.

### Lab 4 (simulator): smart group, scope and a policy

*Exam links: inventory queries, device groups and assignment, app distribution, agent vs MDM.*

1. Run five simulated Macs with the agent (`-n 5 -token …`). The simulator cycles through macOS 15.6.1, 15.5, 14.7.6
   and 26.0 and turns FileVault on for even-numbered Macs.
2. Create a smart group **"macOS below 15.6"** with *Operating System Version* `less_than` `15.6`. Preview it: which
   simulated Macs match, and why is the comparison "15.10 > 15.9" important?
3. Create a policy scoped to that group with trigger `checkin`, a script named **"Report OS"** (`sw_vers`) and an
   Installomator app (`googlechrome`). Save it: in-scope agents are woken through the long-poll and the policy runs
   within seconds. Read the logs.
4. Add a second script called **"This will fail"**: the simulator fails any script whose name contains "fail". Watch
   the policy log turn `failed` and, after three Macs fail, the Health finding `configuration.policy_failures` appear
   (allow for the 5-minute engine interval, or **Run all checks**).
5. Exclude one Mac in the scope's exclusions and re-run. Explain the evaluation order (exclusions, limitations,
   targets).

### Lab 5 (simulator): a DDM enforcement plan

*Exam links: DDM declaration types, status channel, server tokens, enforcement declarations.*

1. Create an **Enforcement** plan targeting `26.0.1` with a deadline a day ahead, scoped to all computers
   ([§10.1](10-software-updates.md#101-ddm-update-plans)). Saving triggers a reconcile and a `DeclarativeManagement`
   command to each Mac on macOS 13 or later.
2. Watch the server output (or the `ddm` log component) for the simulator's check-ins: `tokens`,
   `declaration-items`, one `declaration/…` fetch per declaration, then `status`.
3. Open a computer's **Updates** tab (`GET /api/v1/computers/{id}/ddm`): the declarations as served, the token, and the
   status report. Simulated Macs below the target report `install-state: downloading` and a pending version; the
   Overview shows them `pending`, and a Mac already on 26.0 `up_to_date`.
4. Change the target version and save. Only the enforcement declaration's server token changes; the status
   subscription's does not. Explain why the Mac re-fetches only one declaration.
5. Stop the simulator and wait: after 6 hours the Health finding `configuration.ddm_not_syncing` would open. Read its
   remediation and explain what `DeclarativeManagement` with the current token does.

### Lab 6 (simulator): FileVault escrow and Escrow Buddy

*Exam links: FileVault, personal recovery keys, escrow payload, `SecurityInfo`, Secure Token.*

1. In **FileVault**, enable the escrow profile for all computers. Run `POST /api/v1/filevault/check`.
2. Simulated Macs with FileVault on return an `FDE_PersonalRecoveryKeyCMS` blob in `SecurityInfo`; VibeMDM decrypts
   it and shows a fingerprint. Every fourth Mac has **no** key until remediation. Find them in the FileVault page.
3. Run **Remediate** on one of them (`POST /api/v1/computers/{id}/filevault/remediate`). The simulator "generates"
   a new key; the next `SecurityInfo` escrows it. Check `rotated_count`.
4. Reveal a key with a reason, then find `filevault.key_viewed` in the audit log and the `FileVaultKeyViewed` webhook
   (set up a webhook first, [§19.5](19-users-roles-api.md#195-webhooks)).
5. Explain the difference between a PRK and an IRK, and why the escrow profile must be installed *before* the key is
   generated.

### Lab 7 (simulator): a compliance baseline and drift

*Exam links: security baselines, enforce vs audit, drift, exemptions.*

1. Create a baseline from `cis_lvl1` with a 1-hour scan interval, scoped to all computers, without remediation.
   Exempt one rule with a reason.
2. **Scan now**. The simulator returns a mix of pass and fail results. Read the report
   (`GET /api/v1/compliance/baselines/{id}/report`) and one Mac's detail.
3. Scan again: the simulator introduces **drift** on the second scan. Watch `drifted` rise and the Health finding
   `security.compliance_drift` appear with its *Rescan affected Macs* remediation.
4. Turn on `deploy_profile` and preview the generated enforcement profile
   (`GET /api/v1/compliance/baselines/{id}/profile`). Which rules does it cover, and why can't a profile cover all of
   them?
5. Export the CSV and explain the `previous_status` column.

### Lab 8 (simulator): onboarding

*Exam links: first-run experience, Setup Assistant vs third-party onboarding, why an agent is needed.*

1. Create a workflow with an `asset_tag` text field (regex `^A-[0-9]{4}$`), a `department` picker, the template
   `{model}-{asset_tag}`, two app steps and a `recon` step. Try adding a comma to a label and read the error.
2. Scope it to all computers and start the simulator with the agent: the simulator fills the form (asset tag
   `A-1xxx`, the first picker option), so each Mac is renamed (for example `MBP-A-1001`) and a run appears under
   **Onboarding → Runs** and the computer's **Onboarding** tab.
3. Add a step titled **"Fail on purpose"** and re-run with `force: true` on one Mac. Observe `failed` and `skipped`
   statuses and that no final action is performed.
4. Explain what a real Mac would show at each stage, and why the server waits for a console user before starting.

### Lab 9 (real Mac): User Approved MDM enrollment

*Exam links: enrollment types, UAMDM, supervision, bootstrap token, what MDM can see.*

1. Create an invitation and enroll a Mac with the one-line installer and `--mdm`
   ([§4.5](04-enrollment.md#45-the-agent-installer)). Approve the profile in System Settings → General → Device
   Management.
2. On the Mac: `profiles status -type enrollment` (expect `MDM enrollment: Yes (User Approved)`),
   `sudo profiles list`, `sudo profiles status -type bootstraptoken`.
3. In VibeMDM, compare `is_supervised` (false), `bootstrap_token_escrowed` and the `DeviceInformation` fields with
   what the Mac reports. Read the known quirk about `bootstrap_token_escrowed` in
   [§5.2](05-inventory-and-groups.md#52-inventory-sources-mdm-vs-agent).
4. Deploy a PPPC profile granting an app Full Disk Access. Confirm it installs (UAMDM) and that the app shows as
   allowed in System Settings → Privacy & Security.
5. Remove the MDM profile from the Mac as the user. Watch `CheckOut` arrive (`ComputerUnenrolled` webhook) and note
   that the agent still checks in. Explain why ADE would have prevented the removal.

### Lab 10 (real Mac): Secure Token, volume ownership and FileVault

*Exam links: user accounts, Secure Token, FileVault enablement by profile.*

1. `sysadminctl -secureTokenStatus <your admin>` and, on Apple silicon, `diskutil apfs listUsers /` to see volume
   owners (uncertain detail: the exact output changes between macOS versions; read it rather than memorize it).
2. Create a standard user with `sysadminctl -addUser` (no Secure Token). Check its status.
3. Enable the VibeMDM **enforcement profile** with `Defer` and `MaxBypassAttempts` 0 scoped to this Mac
   ([§11.4](11-filevault.md#114-the-enforcement-profile)). Log out and in as a Secure Token user: FileVault turns on
   and, with the escrow profile in place, the PRK is escrowed within the next `SecurityInfo` sweep (or send one).
4. Explain which users can unlock the disk at startup, and what the bootstrap token would do for a new account created
   by a login-window tool such as XCreds.

### Lab 11 (real Mac): logs and diagnostics

*Exam links: unified logging, troubleshooting method, MDM client diagnosis.*

1. Run `log stream --info --predicate 'process == "mdmclient" OR subsystem == "com.apple.ManagedClient"'` while
   you send a `ProfileList` command. Identify the push arrival, the connection to `/mdm` and the response.
2. Request a diagnostic bundle (`POST /api/v1/computers/{id}/diagnostics`) and open `mdmclient.ndjson` from it.
3. Stop the agent (`sudo launchctl bootout system/com.vibemdm.agent`) and wait two hours, or lower
   `agent_checkin_interval_seconds`: the Health finding `fleet.agent_silent` appears. Follow its advice
   (`sudo launchctl kickstart -k system/com.vibemdm.agent`; or `sudo vibemdm-agent manage` after a bootout) and
   watch it resolve.

### Lab 12 (simulator): break the server and fix it from Health

*Exam links: certificates and trust, security posture, operational hygiene.*

1. Start the server without `VIBEMDM_ENCRYPTION_KEY`: Health opens `security.encryption_at_rest`. Add the key and
   restart: the log says the plaintext secrets were sealed, and the finding resolves after two runs or a re-check.
2. In a lab with `VIBEMDM_ALLOW_HTTP=1`, turn on the remote shell and MCP: `security.remote_shell_transport` and
   `security.mcp_transport` open. Note which remediations are marked safe and which are not, and why.
3. Delete a group that a policy's scope uses: `configuration.stale_scope_references` opens. Fix the scope.
4. Explain the grace period, auto-resolve and flap protection in your own words ([§15.2](15-health.md#152-noise-controls)).

### Lab 13 (simulator): an AI agent with least privilege

*Exam links: automation, API tokens, least privilege (not an exam objective, but good practice).*

1. Enable MCP, create a **viewer** token and connect Claude Code with `claude mcp add` ([§19.6](19-users-roles-api.md#196-the-mcp-server-for-ai-agents)).
2. Ask it which Macs fail the baseline and why, and what health issues are open. Then ask it to run a policy: it gets
   `unknown tool`. Explain the two switches that control write access.
3. Read the `mcp.tool` audit entries.

## 23.5 Quick facts

Use these for a last pass. Each has been covered in a study note.

| Fact | Where |
| --- | --- |
| An APNs push for MDM contains only `{"mdm": "<PushMagic>"}`; commands are pulled by the device | [§1.3](01-introduction.md#13-how-the-pieces-talk), [§16.3](16-mdm-commands-and-inspector.md#163-study-notes-an-installprofile-round-trip-on-the-wire) |
| Devices connect to APNs on TCP 5223 (fallback 443 on Wi-Fi) within 17.0.0.0/8; servers push to `api.push.apple.com:443` (or 2197) | [§2.1](02-installation.md#21-requirements) |
| The push certificate lasts one year; **renew** (same Apple Account) to keep the topic; re-creating changes the topic and forces re-enrollment | [§3.1](03-apns-and-certificates.md#31-the-apns-push-certificate) |
| `Authenticate` then `TokenUpdate` completes enrollment; `CheckOut` reports removal when `CheckOutWhenRemoved` is set | [§1.3](01-introduction.md#13-how-the-pieces-talk) |
| Command results: `Acknowledged`, `Error` (with `ErrorChain`), `CommandFormatError`, `NotNow`; `Idle` is a request for work | [§16.3](16-mdm-commands-and-inspector.md#163-study-notes-an-installprofile-round-trip-on-the-wire) |
| SCEP: the private key never leaves the device; the shared secret is the challenge; ACME is the newer alternative with attestation | [§3.2](03-apns-and-certificates.md#32-the-scep-ca-and-device-identities) |
| Enrollment methods: ADE (ABM, supervised, Setup Assistant), Device Enrollment by profile (UAMDM on Mac), account-driven Device Enrollment, User Enrollment (BYOD, Managed Apple Account, limited visibility) | [§4.1](04-enrollment.md#41-enrollment-options-without-apple-business-manager) |
| A Mac is supervised **only** through ADE; iPhone and iPad also through Apple Configurator (which erases the device) | [§4.10](04-enrollment.md#410-study-notes-supervision-user-approved-mdm-activation-lock-bootstrap-token) |
| UAMDM (macOS 10.13.4+) is needed for PPPC, kernel and system extension policies and bootstrap-token escrow; scripts cannot create it | [§4.10](04-enrollment.md#410-study-notes-supervision-user-approved-mdm-activation-lock-bootstrap-token) |
| Activation Lock on a Mac needs Apple silicon or T2; MDM can get a bypass code only for supervised devices | [§4.10](04-enrollment.md#410-study-notes-supervision-user-approved-mdm-activation-lock-bootstrap-token) |
| Bootstrap token (10.15+): escrowed to MDM, grants Secure Token to later accounts, authorizes some Apple silicon operations | [§4.10](04-enrollment.md#410-study-notes-supervision-user-approved-mdm-activation-lock-bootstrap-token), [§11.7](11-filevault.md#117-study-notes-filevault-and-secure-token) |
| `PayloadIdentifier` identifies a profile; installing the same identifier replaces it; MDM-installed profiles are removable only by MDM | [§8.1](08-configuration-profiles.md#81-profile-anatomy) |
| MCX managed preferences: `com.apple.ManagedClient.preferences` → domain → `Forced` / `Set-Once` / `Often`; applied values appear in `/Library/Managed Preferences` | [§8.4](08-configuration-profiles.md#84-the-mcx-payload-format) |
| PPPC can grant most TCC services but **never Camera or Microphone**; Screen Recording and Input Monitoring can only be delegated to standard users | [§8.5](08-configuration-profiles.md#85-study-notes-common-payload-types) |
| DDM: configurations, assets, activations, management; status subscriptions replace polling; server tokens skip unchanged declarations | [§10.8](10-software-updates.md#108-study-notes) |
| Software-update enforcement declarations need macOS 14+, settings declarations 15+; the MDM update commands are deprecated | [§10.1](10-software-updates.md#101-ddm-update-plans), [§10.8](10-software-updates.md#108-study-notes) |
| On Apple silicon an update must be authorized by a volume owner or by MDM through the bootstrap token | [§10.8](10-software-updates.md#108-study-notes) |
| FileVault: PRK per Mac (recommended, escrowed via `FDERecoveryKeyEscrow` and returned in `SecurityInfo`), IRK is legacy; the escrow profile must be installed before the key is generated | [§11.7](11-filevault.md#117-study-notes-filevault-and-secure-token) |
| Secure Token: first Setup Assistant admin gets it; check with `sysadminctl -secureTokenStatus`; volume owners on Apple silicon | [§11.7](11-filevault.md#117-study-notes-filevault-and-secure-token) |
| Gatekeeper checks quarantined downloads for Developer ID signature and notarization; `spctl --status`; root scripts' downloads are usually not quarantined | [§9.6](09-apps-installomator.md#96-why-arguments-are-single-quoted), [§20.7](20-security-hardening.md#207-study-notes-macos-platform-security) |
| SIP can be changed only from recoveryOS (`csrutil`); the system volume is signed and read-only (SSV, macOS 11+) | [§20.7](20-security-hardening.md#207-study-notes-macos-platform-security) |
| XProtect (signatures at launch), XProtect Remediator (periodic scans, 12.3+), MRT (legacy) | [§20.7](20-security-hardening.md#207-study-notes-macos-platform-security) |
| Extensible SSO payload: redirect extensions (web SSO, Platform SSO) and credential extensions (Kerberos); Platform SSO methods: password, Secure Enclave key, smart card | [§13.6](13-identity-xcreds.md#136-study-notes-platform-sso-xcreds-and-the-kerberos-sso-extension) |
| Security frameworks: NIST 800-53 (Low/Moderate/High), 800-171 (CUI), CMMC, CIS Level 1/2, DISA STIG; mSCP maps them to macOS rules with ODVs | [§12.6](12-compliance.md#126-study-notes-security-baselines) |
| A profile enforces; a script remediates once; a scan audits; document exemptions | [§12.6](12-compliance.md#126-study-notes-security-baselines) |
| Unified log: `log show`, `log stream`, `log collect`; MDM lives in `process == "mdmclient"` and `subsystem == "com.apple.ManagedClient"` | [§18.7](18-logging-and-diagnostics.md#187-study-notes-logs-on-the-mac) |
| Remote access on a Mac: Screen Sharing / Remote Management (`EnableRemoteDesktop`) and Remote Login (SSH); baselines usually require SSH off | [§17.7](17-remote-shell.md#177-good-practice) |
| Apple's first-run tools: Setup Assistant, Await Configuration (`DeviceConfigured`), enrollment customization; third-party onboarding needs an agent and a logged-in user | [§14.8](14-white-glove-onboarding.md#148-requirements-and-tips) |

## 23.6 Apple's official references

Read these from Apple directly; they are the source the exams are written from. Titles only, because Apple moves
pages. Start at `support.apple.com` for the guides and `developer.apple.com` for the protocol documentation.

| Reference | Use it for |
| --- | --- |
| **Apple Platform Deployment** (guide) | ABM/ASM, enrollment types, supervision, Managed Apple Accounts, profiles and payload reference, DDM overview, software updates, FileVault and Secure Token, Platform SSO, app distribution. The primary ACITP text |
| **Apple Platform Security** (guide) | Secure Boot, Secure Enclave, FileVault and data protection, Gatekeeper, notarization, XProtect, SIP and the SSV, Activation Lock, Recovery Lock, TCC. The primary security text for both exams |
| **Device Management** documentation on developer.apple.com | The MDM protocol reference (check-in and command messages, error codes, `DeviceInformation` queries), the profile and payload keys, the DDM declaration schema and status items, the `com.apple.mdm` payload |
| **macOS User Guide** and **macOS support articles** | Where settings live, how Time Machine, iCloud, Migration Assistant, Safe Mode and recoveryOS work; the ACSP's everyday material |
| **Use Apple products on enterprise networks** (support article) | The hosts and ports for APNs, software updates, ABM, iCloud and other services |
| **Apple Training: "Apple Device Support"** (course) | The ACSP curriculum, with its exam preparation guide |
| **Apple Training: "Apple Deployment and Management"** (course) | The ACITP curriculum, with its exam preparation guide |
| **Apple Business Manager User Guide** / **Apple School Manager User Guide** | Roles, device assignment, Apps and Books, federation, Shared iPad (ASM) |
| **Apple Configurator User Guide** (Mac and iPhone) | Supervising and enrolling iPhone and iPad; adding Macs to ABM with Apple Configurator for iPhone |
| **Platform SSO** and **Managed Apple Accounts** sections of Apple Platform Deployment | Identity topics that change with each release |

Community references used by this manual (not Apple, not exam sources): the macOS Security Compliance Project
(NIST), the Mac Admins SOFA feed, Installomator, swiftDialog, Escrow Buddy, super and XCreds, each linked from its
chapter.

## 23.7 Before the exam

- The exams test Apple's current releases. Re-check anything this manual marks as **uncertain** or **version
  dependent** (bootstrap token uses, Erase All Content and Settings requirements, Platform SSO features, Rapid
  Security Response naming, the exact Setup Assistant panes, DDM availability on non-supervised Macs) against the
  current guides.
- Know the vocabulary precisely: *supervised* vs *user approved*, *device channel* vs *user channel*, *Device
  Enrollment* vs *User Enrollment*, *personal* vs *institutional* recovery key, *configuration* vs *activation*
  declaration, *Managed Apple Account* vs personal Apple Account.
- Be able to explain any VibeMDM behaviour you observed in the labs in Apple's terms: not "the inspector showed
  push_failed", but "APNs could not deliver the wake-up, so the device never polled for the queued command".

Back to the [table of contents](README.md).
