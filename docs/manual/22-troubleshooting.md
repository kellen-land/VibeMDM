# 22. Troubleshooting

This chapter is organized by area. Each table lists a **symptom**, its **likely cause**, **how to confirm** it (the
exact Health check, log component, agent command or API call) and the **fix**. Health check IDs are the ones in
`internal/health/checks.go` ([§15.4](15-health.md#154-every-check)); log components are the `component=` values
described in [§18.1](18-logging-and-diagnostics.md#181-server-logs).

## 22.1 First steps for any problem

Work out **which channel** the broken feature uses ([§1.3](01-introduction.md#13-how-the-pieces-talk)). Profiles,
MDM commands, DDM and FileVault escrow use the MDM channel and need APNs. Scripts, policies, Self Service, compliance,
super, onboarding and the remote shell use the agent channel and need the daemon on the Mac to be running. Then:

| Step | Where | What you learn |
| --- | --- | --- |
| 1. Open **Health** | Sidebar → Health (`GET /api/v1/health/summary`) | Most misconfigurations and fleet problems already have a finding with an explanation and a fix. Click **Re-check now** if you just changed something |
| 2. Open the computer | Computers → the Mac | `mdm_last_checkin` vs `agent_last_checkin` tells you which channel is dead. The Health strip shows that Mac's findings |
| 3. Look at the command | Activity → Commands → the command (`GET /api/v1/commands/{uuid}/inspect`) | The inspector timeline shows whether the problem is the push (`push_failed`), the device never connecting (no `delivered`), or the device refusing (`error`, `not_now`) |
| 4. Read the server log | Logs → Server, filter by component (`GET /api/v1/logs?source=server&component=…&level=warn`) | `scep`, `mdm`, `nanomdm`, `agent`, `commander`, `ddm`, `health`, `mcp`, `api` |
| 5. On the Mac | `sudo vibemdm-agent status`, `sudo tail -f /var/log/vibemdm-agent.log`, `profiles status -type enrollment`, `log stream --info --predicate 'process == "mdmclient"'` | Whether the agent is enrolled and running, whether the Mac is MDM-enrolled, and the device's side of every MDM conversation |
| 6. Collect a diagnostic bundle | Computer → Diagnostics (`POST /api/v1/computers/{id}/diagnostics`) | The `mdmclient.ndjson` file has the last two hours of the Mac's MDM log; `profiles-*.txt`, `launchd-agent.txt` and `agent-config.json` cover the rest ([§18.5](18-logging-and-diagnostics.md#185-diagnostic-bundles)) |

What `sudo vibemdm-agent status` prints and what each line means:

| Line | Healthy value | If not |
| --- | --- | --- |
| `Enrolled:` | `yes (since …)` | `no (…permission denied)`: you forgot `sudo`. `no (open …agent.json: no such file…)`: the agent was never enrolled or was removed; run the install command |
| `Server:` / `Computer ID:` | Your HTTPS URL and the ID shown in the console | A different URL means the Mac was enrolled against another server (or a lab instance) |
| `Daemon running:` | `yes` | `no`: the LaunchDaemon is unloaded or crashed (§22.5). Inferred from the socket file, so confirm with `sudo launchctl print system/com.vibemdm.agent` |
| `Server reachable:` | `yes (server <version>)` | `no (…)`: DNS, TLS, proxy or firewall between the Mac and the server; or `server returned 401`, which means the device token was revoked (**Reset agent** was used) |
| `swiftDialog:` / `super:` | `yes` where you use onboarding or super | `no`: see §22.8 and §22.7 |
| `Remote shell veto:` | `no` | `yes`: the Mac refuses remote shells by design ([§17.4](17-remote-shell.md#174-device-side-veto)) |

> **Study note (troubleshooting method):** Apple's support training teaches a structured method rather than
> guesswork: gather information (what changed, which user, which network, is it reproducible), verify the problem
> yourself, try quick fixes (restart, log out and in, test as another user, Safe Mode), run diagnostics (Console,
> Activity Monitor, Apple Diagnostics, `sysdiagnose`), escalate or research, repair, verify the fix and **document
> it**. The names of the steps differ between editions of the course, so learn the idea rather than a list. The
> tables in this chapter follow the same pattern: symptom, hypothesis, a way to confirm it before you change
> anything, then the fix.

## 22.2 Enrollment

### MDM enrollment (the profile)

| Symptom | Likely cause | How to confirm | Fix |
| --- | --- | --- | --- |
| The enrollment page or profile download returns `503 enrollment is not fully configured: no APNs push certificate has been uploaded` | No push certificate: the profile must contain the APNs topic | `GET /api/v1/enrollment/status` → `issues`; Health `platform.apns_certificate` (*APNs push certificate*) says *missing* | Obtain and upload the push certificate ([§3.1](03-apns-and-certificates.md#31-the-apns-push-certificate)) |
| `GET /enroll/profile?token=…` returns `403 invalid or expired enrollment token` | The invitation expired, was disabled, reached `max_uses`, or the token was mistyped. Remember that each profile download **and** each agent enrollment consumes one use | `GET /api/v1/enrollment/invitations` and look at the invitation's enabled flag, expiry and use count ([§4.3](04-enrollment.md#43-invitations-and-the-enrollment-secret)) | Create a new invitation with a larger `max_uses`, or re-enable it (`PATCH /api/v1/enrollment/invitations/{id}` `{"enabled": true}`) |
| Safari shows a certificate warning, or macOS says the profile could not be downloaded or the server certificate is invalid | The TLS certificate is self-signed, expired or from a private CA the Mac does not trust; or the server URL is `http://` | Open `https://mdm.example.com` in Safari on the Mac (no padlock = no enrollment). Health `platform.server_url` (*Server URL*) warns when the URL is not HTTPS | Use a publicly trusted certificate (for example Let's Encrypt in Nginx Proxy Manager). A private CA must be installed and trusted on the Mac **before** enrollment ([§3.4](03-apns-and-certificates.md#34-the-tls-certificate)) |
| The profile installs for a moment, then macOS reports that the profile failed to install; the `mdmclient` log mentions SCEP | The one-time SCEP challenge in that profile was already used (the file was installed twice or shared), or it expired (2 h for a user download, 1 h for the agent's copy, 7 days for an admin download) | Server log, component `scep`: `scep pkioperation failed` or `scep: refusing to sign CSR` with `invalid SCEP challenge`. On the Mac: `log show --last 15m --predicate 'process == "mdmclient"'` | Download a **fresh** profile and install that one. Never reuse a `.mobileconfig` file across Macs (except the Configurator profile, whose challenge is reusable for 24 h) |
| Same symptom right after you regenerated the SCEP CA | `POST /api/v1/settings/scep/regenerate` replaced the CA and the static challenge; every old profile and every existing device identity is now invalid | Settings → SCEP CA shows a new validity start date; the audit log has `scep.regenerate` | Every Mac must re-enroll with a new profile ([§3.2](03-apns-and-certificates.md#32-the-scep-ca-and-device-identities)) |
| The Mac appears in **Computers** but is not marked MDM-enrolled, or never appears at all | `Authenticate` arrived but `TokenUpdate` did not. The Mac obtains its APNs device token before sending `TokenUpdate`, so a Mac that cannot reach APNs (TCP 5223 or 443 to `17.0.0.0/8`) never completes enrollment | Server log, component `mdm`: an `Authenticate` line without a following token update for that UDID. On the Mac: `profiles status -type enrollment` shows `MDM enrollment: No`, and `log stream --predicate 'process == "mdmclient"'` shows push-token errors | Fix the Mac's network path to APNs (proxy, firewall, captive portal). Remove the half-installed profile in System Settings → Device Management and install a fresh one |
| Enrollment "succeeds" but no profiles arrive and Health later opens `fleet.agent_missing` | Only the agent is missing: that is expected for MDM-only Macs | Computer → `agent_enrolled` is false | Run the agent install command ([§4.5](04-enrollment.md#45-the-agent-installer)) |
| The user cannot find where to approve the profile | On macOS 13 and later it is **System Settings → General → Device Management**; on older versions **System Preferences → Profiles**. A downloaded profile waits there until the user clicks **Install** | `profiles status -type enrollment` shows `MDM enrollment: Yes (User Approved)` once approved | Point the user at the pane; the `--mdm` installer opens it automatically |
| Apple Configurator cannot use the MDM server URL | The URL must be the invitation's `configurator_url` (`/enroll/configurator/<token>`), the token must be valid, and a private TLS CA must be added as an anchor in **Prepare** | Server log shows `/enroll/configurator/[redacted]` with `403` (bad token) or `503` (not configured) | Create a new invitation and paste the full Configurator URL; add the anchor certificate ([§4.6](04-enrollment.md#46-iphone-and-ipad-with-apple-configurator)) |

### Agent enrollment (the installer)

| Symptom | Likely cause | How to confirm | Fix |
| --- | --- | --- | --- |
| `curl … /agent/install.sh?token=…` prints `{"error":"the server URL is not configured (Settings → General)"}` (HTTP 503) | The server URL setting is empty, so the installer refuses to guess from the Host header | `GET /api/v1/settings` → `server_url` | Set the URL in Settings → General |
| The installer prints `{"error":"invalid or expired enrollment token"}` (HTTP 403), or `vibemdm-agent enroll` fails with `enrollment failed: server returned 403: invalid or expired enrollment token` | Invitation exhausted, expired or disabled; or the enrollment secret was rotated | `GET /api/v1/enrollment/invitations`; the audit log shows `settings.rotate_enrollment_secret` if someone rotated the secret | New invitation, or the current secret from Settings → General (admin) |
| The installer fails with `{"error":"agent binary not available on this server (build with \`make agent\`)"}` (HTTP 404) | The image or `VIBEMDM_AGENT_DIR` has no `vibemdm-agent-darwin-arm64` / `-amd64` | Health `platform.agent_binaries` (*Agent binaries*); `GET /api/v1/enrollment/status` → `agent_binaries` | Rebuild the image (`make agent` runs as part of the Docker build) or copy the binaries into `VIBEMDM_AGENT_DIR` |
| `enrollment failed: server returned 409: …` ("already has an active agent") | **Takeover protection**: another agent record for this serial number or UDID is still active and you did not present its device token (fresh macOS install, `agent.json` deleted, a cloned disk image, or two Macs reporting the same identifiers) | Audit log entry `agent.enroll_refused` with the serial, UDID and IP; server log, component `agent`: `agent enroll refused: computer already has an active agent` | On the computer page click **Reset agent** (`POST /api/v1/computers/{id}/reset-agent`), then run the installer again. If the old agent has been silent for 30 days the next enrollment is accepted anyway ([§4.7](04-enrollment.md#47-takeover-protection-and-reset-agent)) |
| The agent appears as a **second** computer record | The agent's hardware UUID or serial did not match the MDM record (common on virtual machines and on Macs whose MDM record was created from a different UDID) | Compare `udid` and `serial_number` on both records (`GET /api/v1/computers?q=<serial>`) | Unenroll and delete the duplicate (`DELETE /api/v1/computers/{id}`), then enroll MDM first and the agent second (`--mdm` does this in one step) |
| `enroll` fails with `server URL must use https://` | Lab server on plain HTTP | — | For an isolated lab only: `--allow-insecure-http` on the Mac and `VIBEMDM_ALLOW_HTTP=1` on the server ([§2.3](02-installation.md#23-environment-variables)) |
| `enroll --mdm` enrolled the agent but no profile window opened | The Mac is already MDM-enrolled (the step is skipped), or nobody is logged in at the console | `enroll` prints `Enrolled. Computer ID …`; `profiles status -type enrollment` | Open `/Users/Shared/VibeMDM-Enrollment-*.mobileconfig` by hand, or send the user the invitation's `enroll_url` |

## 22.3 APNs and pushes

| Symptom | Likely cause | How to confirm | Fix |
| --- | --- | --- | --- |
| Commands stay `Pending`; the inspector shows `push_failed` | The push certificate expired, the server cannot reach `api.push.apple.com:443`, or there is no certificate for the device's **topic** | Health `platform.push_failures` (*APNs push failures*) quotes the last error; `platform.apns_certificate` shows expiry; `GET /api/v1/settings/apns` lists each certificate's `topic`, `not_after` and `expired`; the inspector's `push_failed` event carries the error text | Renew the certificate **with the same Apple Account** and upload it ([§3.1](03-apns-and-certificates.md#31-the-apns-push-certificate)); allow outbound HTTPS to Apple from the server (and from your proxy, if the container egresses through one) |
| Pushes fail only for Macs enrolled before a certain date | **Topic mismatch**: a *new* certificate was created instead of renewing, so it has a different `com.apple.mgmt.External.<UUID>` topic than the one in those Macs' enrollment profiles | Compare the topic in `GET /api/v1/settings/apns` with the topic in the device's `TokenUpdate` (inspector or `mdm` log). Macs enrolled after the upload work; older ones do not | If you still have the old certificate and key, upload them too (several topics can coexist). Otherwise every old Mac must re-enroll. Use **Renew** in the portal next time |
| `push_sent` is recorded but the Mac never connects (`delivered` never appears) | The Mac is off, asleep or cannot reach APNs (TCP 5223 or 443 to `17.0.0.0/8`), or its push token is stale | Health `fleet.mdm_unresponsive` (*MDM channel not responding*): agent online, MDM commands waiting more than 2 h. Its facts show `mdm_last_checkin`. On the Mac: `log stream --predicate 'process == "mdmclient" OR process == "apsd"'` | Fix the network path (many guest and hotel networks block 5223). The finding's **Send an APNs push** remediation is safe and on by default. As a last resort, remove the MDM profile and re-enroll |
| A Mac answers pushes but a command reports `NotNow` repeatedly | The device is locked, in Setup Assistant, or the command needs a logged-in user | Inspector: `not_now` events; the command stays queued | Wait; macOS retries when the condition clears. `NotNow` is normal, not an error ([§16.2](16-mdm-commands-and-inspector.md#162-the-command-inspector)) |
| The simulator's Macs never receive pushes | Expected: `vibemdm-sim` has no APNs connection and **polls** `/mdm` every `-poll` interval instead | Inspector shows `push_failed` for simulated devices and `delivered` a few seconds later anyway | None. Use a real Mac to test APNs |
| **Generate request** (Path A) fails | `mdmcert.download` is unreachable from the server, or the e-mail address was rejected | `POST /api/v1/settings/apns/request` returns `submit_error`; audit log `apns.request` with `submitted: false` | Allow outbound HTTPS to `mdmcert.download`, or submit the CSR there manually and paste the response into **Decrypt** |
| Upload fails with `certificate has no UID (topic); is this an Apple MDM push certificate?` | The file is not an MDM push certificate (for example a TLS certificate) | — | Download the push certificate from the Apple Push Certificates Portal. (In a lab with the simulator, any certificate that has a `UID` attribute will do, as [§21.3](21-cli-reference.md#213-vibemdm-sim) explains) |

> **Study note:** when an MDM "stops working for every device at once", the push certificate is the first suspect:
> it expires yearly. When it stops working "for all devices enrolled before day X", suspect a changed topic. When it
> stops working "for one device", suspect that device's network path to APNs or a stale push token. The exam expects
> you to know that APNs carries only a wake-up and that renewing (not re-creating) the certificate preserves the
> topic.

## 22.4 Configuration profiles and DDM

| Symptom | Likely cause | How to confirm | Fix |
| --- | --- | --- | --- |
| Upload is rejected as not a valid property list or missing `PayloadType = Configuration` | The file is **signed** (a PKCS#7 blob), binary, or not a profile | Open the file in a text editor: a signed profile does not start with `<?xml` | Export the profile **unsigned** as XML from your editor ([§8.2](08-configuration-profiles.md#82-upload-and-deploy-a-profile)). VibeMDM signs only the enrollment profile |
| Deployment status `failed`, with an `MCProfileErrorDomain` error | The Mac rejected the payload: a key with the wrong type, a payload that this macOS version does not support, a PPPC or system-extension payload on a Mac that is not User Approved, or a duplicate `PayloadUUID` | Health `configuration.profile_failures` (*Configuration profile failures*) shows the most recent error per profile; the inspector's `error` event has the full `ErrorChain`; on the Mac: `log show --last 1h --predicate 'subsystem == "com.apple.ManagedClient"'` and `profiles status -type enrollment` | Fix the payload (compare with Apple's *Device Management* profile reference), then **Retry on failed Macs** or **Deploy**. For PPPC and extension policies the enrollment must be User Approved ([§4.10](04-enrollment.md#410-study-notes-supervision-user-approved-mdm-activation-lock-bootstrap-token)) |
| A Mac that is in scope never got the profile (no deployment row) | Profile scope is evaluated **only** on save, on **Deploy** and at first MDM enrollment. The Mac joined the smart group later | `GET /api/v1/profiles/{id}/deployments` lists no row for the Mac; `POST /api/v1/scope/preview` with the profile's scope includes it | Click **Deploy** (`POST /api/v1/profiles/{id}/deploy`) after group membership changes ([§6.2](06-scoping.md#62-where-scope-is-used-and-when-it-is-evaluated)) |
| Status stuck at `pending` | The `InstallProfile` command has not been delivered: APNs (§22.3) or `NotNow` | Inspector for the command | Fix APNs or wait |
| The profile is `installed` but the app ignores the managed preference | Wrong preference domain, wrong value type (`"3"` instead of `3`), the key is not one the app honours, or the app only reads it at launch | On the Mac: `defaults read "/Library/Managed Preferences/<domain>"` shows what macOS actually applied; **Preview** in the schema editor shows the XML | Correct the schema values; use `format: data` for binary values; quit and relaunch the app ([§8.3](08-configuration-profiles.md#83-application--custom-settings-json-schemas), [§8.4](08-configuration-profiles.md#84-the-mcx-payload-format)) |
| A vendor says "use our PayloadType" but you built an MCX profile (or the other way round) | Both forms end up as managed preferences, but some apps document only one. The typed form puts the keys at the top level of a payload whose `PayloadType` is the app's domain; the MCX form wraps them in `com.apple.ManagedClient.preferences` → domain → `Forced` → `mcx_preference_settings` | `defaults read` shows the same result for both; if a key is missing, you probably put it under the wrong domain | Build the typed payload in an editor and upload it as an ordinary profile, or fix the domain in the schema profile |
| A `User`-scoped profile does nothing for the user | VibeMDM sends every profile over the **device channel**; it has no user channel | `level = user` in the profile list | Use a device-level payload or a managed preference that applies to all users ([§8.2](08-configuration-profiles.md#82-upload-and-deploy-a-profile)) |
| The user cannot remove a profile | Profiles installed by MDM can only be removed by MDM | System Settings shows no **Remove** button | `POST /api/v1/profiles/{id}/remove` `{"computer_ids": [...]}`, or change the scope and **Deploy** |
| Health: **Invalid declarations** (`configuration.ddm_invalid`) | The Mac rejected a DDM declaration: an enforcement declaration on macOS 13 (needs 14+), a settings declaration on macOS 14 (needs 15+), or a requirement the schema calls "supervised" ([§10.3](10-software-updates.md#103-important-supervision-and-ddm-on-macos)) | The finding quotes Apple's reason per declaration; `GET /api/v1/computers/{id}/ddm` shows the status report with `valid: false` | Scope the plan with a smart group on **Operating System Version**; confirm the behaviour on a real Mac enrolled without ABM |
| Health: **Declarations not syncing** (`configuration.ddm_not_syncing`) | The Mac was asked to sync more than 6 hours ago and has not fetched the manifest: offline, or pushes not arriving | `GET /api/v1/computers/{id}/ddm` → the served token differs from what the device fetched (`declarations_in_sync` false in `GET /api/v1/updates/overview`) | Auto-heal re-sends the sync; otherwise `POST /api/v1/computers/{id}/ddm/sync` and check APNs (§22.3) |

## 22.5 Agent, policies and Self Service

| Symptom | Likely cause | How to confirm | Fix |
| --- | --- | --- | --- |
| Health: **Agent not checking in** (`fleet.agent_silent`): MDM is alive but the agent has been silent for more than 3 check-in intervals (at least 2 h) | The LaunchDaemon is unloaded or crashing, the Mac is asleep with MDM waking on push, the agent's token was revoked, or the agent cannot reach the server | On the Mac: `sudo vibemdm-agent status`; `sudo launchctl print system/com.vibemdm.agent`; `/var/log/vibemdm-agent.log` for `device token rejected by server; re-enroll the agent` or `check-in failed` | `sudo launchctl kickstart -k system/com.vibemdm.agent` to restart it; `sudo vibemdm-agent manage` to rewrite the launchd jobs; if the token was revoked, re-run the install command |
| `Daemon running: no` although the plist exists | launchd unloaded the job (for example after the binary was replaced) or the socket file is stale | `sudo launchctl print system/com.vibemdm.agent` says `could not find service` | `sudo vibemdm-agent manage` (requires `agent.json`), then `sudo launchctl kickstart -k system/com.vibemdm.agent` |
| `Server reachable: no (server returned 401: invalid device token; re-enroll the agent)` | **Reset agent** was used, or the record was deleted on the server | Audit log `agent.reset` on that computer; `/var/log/vibemdm-agent.log` repeats the 401 | Re-run the install command. The old `agent.json` is replaced |
| `Server reachable: no (… x509: certificate …)` or `… no such host` | TLS trust or DNS on the Mac (private CA, captive portal, split DNS) | `curl -v https://mdm.example.com/version` on the Mac | Fix the certificate chain or DNS; the agent uses the system trust store |
| **Run script now** / **Check in now** does nothing for a minute or more | The long-poll (`GET /agent/v1/tasks?wait=55`) is not connected: the daemon is down, or a proxy closes idle connections early | `POST /api/v1/computers/{id}/checkin` returns `agent_online: false`; server log (debug) shows no poll from that Mac | Restart the daemon; raise proxy read timeouts to at least 60 s ([§2.2.1](02-installation.md#221-nginx-proxy-manager)) |
| A policy never runs on a Mac | Not in scope, trigger does not match, frequency already satisfied, the policy is disabled, or the Mac has no agent | Policy → **Logs** (`GET /api/v1/policies/{id}/logs`); `POST /api/v1/scope/preview` with the policy's scope; Health `configuration.policy_never_runs` (*Policy can never run*) | Fix scope or trigger; **Flush** the logs for that Mac (`POST /api/v1/policies/{id}/flush` `{"computer_id": "…"}`) so a once-per-X policy runs again; or **Run now** |
| A policy ran but a script step failed | Non-zero exit; `$3` is empty at the login window; the script expected bash but has a `#!/bin/zsh` shebang (or the reverse); a `defaults write` ran as root instead of the user | The policy log shows each step's exit code and output ([§7.3](07-policies-scripts-packages.md#73-scripts)); Health `configuration.policy_failures` (*Failing policies*) opens at ≥ 3 Macs and ≥ 50 % failing | Fix the script; use `launchctl asuser` for per-user settings; guard against no console user |
| A package step fails | The download's SHA-256 did not match, the `.pkg` requires a newer macOS, or `installer` rejected it | The step output contains `installer` messages; on the Mac `/var/log/install.log` | Re-upload the package; check `installer -pkg … -target /` by hand on a test Mac |
| `network_change` policies never fire | The agent accepts the trigger but has **no built-in network watcher** | — | Fire it yourself (a LaunchDaemon with `WatchPaths`, or a script) as described in [§7.2](07-policies-scripts-packages.md#72-triggers-and-frequency) |
| `vibemdm-agent trigger install_zoom` prints `error: this trigger requires root (run with sudo)` | Custom triggers are accepted only on the root-only socket | — | `sudo vibemdm-agent trigger install_zoom`, or `sudo vibemdm-agent policy --event install_zoom` |
| `vibemdm-agent trigger login` prints `agent daemon is not running` | The public socket `/var/run/vibemdm-agent.sock` is missing | `sudo vibemdm-agent status` → `Daemon running: no` | Restart the daemon (above) |
| The Self Service portal returns **403 forbidden** | The portal only answers requests whose `Host` is `127.0.0.1:8571` or `localhost:8571`, and only runs items when the request carries `X-VibeMDM-SelfService: 1` with a JSON body. Opening it by the Mac's host name, through a browser extension that rewrites headers, or from another site, is refused by design | The browser's address bar shows something other than `http://127.0.0.1:8571` or `http://localhost:8571` | Open it with `vibemdm-agent selfservice` or the exact loopback URL ([§7.6](07-policies-scripts-packages.md#76-self-service)) |
| The Self Service portal does not load at all | The daemon is not running (it serves the portal) | `sudo vibemdm-agent status` | Restart the daemon |
| The portal is empty | No **enabled** policy with `self_service.enabled`, in scope for this Mac, and no Mac App with Self Service enabled | `GET /api/v1/policies` and the scope preview | Enable Self Service on the policy and check the scope. Frequency is not applied to Self Service runs, but `enabled` is |
| Inventory is stale although the agent checks in | Full inventory runs every `inventory_interval_hours` (default 24); only the light fields update at every check-in. A slow extension attribute can also time out | Computer → `last_inventory_at`; the agent log around `inventory` | `sudo vibemdm-agent recon`, or **Update inventory**; make EA scripts fast ([§5.3](05-inventory-and-groups.md#53-extension-attributes)) |
| Results are missing after a server outage | The agent spooled them | `/Library/Application Support/VibeMDM/spool/` on the Mac is not empty | Nothing: the spool is replayed every 2 minutes once the server is back |

## 22.6 Compliance

| Symptom | Likely cause | How to confirm | Fix |
| --- | --- | --- | --- |
| Scans show `expired` with `no result from the agent within 24 hours` | The agent was offline, the daemon is down, or the scan exceeded the agent's 45-minute limit | Health `security.compliance_scan_failures` (*Compliance scans failing*) after 3 such scans in 48 h; `GET /api/v1/compliance/baselines/{id}/scans?computer_id=…` | Fix the agent (§22.5), then **Scan now** |
| A scan is `failed` | The agent returned an error for the whole scan (for example the task payload could not be parsed, or the agent version is too old for a new rule format) | The scan row's `error`; the `compliance_scan` task output under **Activity** | Update the agent; rescan |
| Individual rules report `error` | The check script timed out (60 s), a command it needs is missing on that macOS version, or the expected type does not match | `GET /api/v1/compliance/baselines/{id}/computers/{cid}` shows the rule's `detail` and actual value | Exempt the rule with a reason, or report the rule upstream to mSCP if its check is wrong for your macOS version |
| A rule fails again after remediation ("the fix did not stick") | A fix script sets the value **once**; the user, another tool or a macOS update changed it back; or the rule is one that only a configuration profile can hold | The rule's `previous_status` was `pass`/`remediated` and is now `fail`: it has **drifted**. Health `security.compliance_drift` (*Compliance drift*) lists the rules; `GET …/baselines/{id}/profile` shows whether the enforcement profile is deployed and which rules it covers | Turn on `deploy_profile` so settings with profile content are **enforced**, and **Deploy** the generated profile after scope changes; keep `auto_remediate` for the rest ([§12.4](12-compliance.md#124-the-enforcement-profile)) |
| The enforcement profile is `failed` on some Macs | One merged payload is invalid for that macOS version | Same diagnosis as a profile failure (§22.4); the inspector shows the `ErrorChain` | Exempt the rule whose payload is rejected (the profile is regenerated), then **Deploy** |
| Many rules are `not_applicable` or `manual` | Expected: rules limited to another architecture or macOS version, and rules tagged `manual`, `inherent`, `permanent`, `n_a` or `supplemental`, are never scanned | The per-computer report shows the state and reason | None |
| Saving the baseline fails on an exclusion | A **reason** is required for every exempted rule | `400` error naming the rule | Add the reason |
| Saving fails on an ODV override | The value does not match the rule's ODV type or limits | `400` error naming the rule | Use a value of the right type within the rule's range |
| Some Macs are never scanned | No agent on them, or they are out of scope | The report's rows show `not_scanned`; the scheduler skips Macs without the agent | Install the agent; check the scope |

## 22.7 Software updates

### super

| Symptom | Likely cause | How to confirm | Fix |
| --- | --- | --- | --- |
| `GET /api/v1/updates/super/status` lists a Mac as `missing` | The reconciler has not run yet (every 30 min; at most one install per Mac per day), the Mac has no agent, or super is `enabled: false` | `GET /api/v1/updates/super` → `enabled`; the Mac's **Activity** tab for an `install_super` task; `sudo vibemdm-agent status` → `super: no` | `POST /api/v1/updates/super/install` `{"computer_ids": ["…"]}` to install now, or an empty list to reconcile the whole scope |
| A Mac stays `outdated` | It reports a super version other than the pinned 5.1.2 (for example a copy installed by hand); the reconciler re-queues an install once a day | The status row's `SuperVersion` | Let the reconciler run, or install on demand |
| The `install_super` task fails with `super checksum mismatch (got …, want …); refusing to run` | The download was altered (a TLS-inspecting proxy, a captive portal) or GitHub served something unexpected | Task output under **Activity** | Fix the network path from the Mac to `raw.githubusercontent.com`; VibeMDM never runs an unverified script |
| The task fails with `downloading super: …` | The Mac cannot reach `raw.githubusercontent.com` | Same | Allow the host, or mirror the script (that requires changing the pinned URL and checksum in the code) |
| super is installed but never updates the Mac | Authentication: on Apple silicon super needs a volume owner's credentials (`auth_mode` `local` or `service`) or an MDM bootstrap-token path; or the schedule and deferral settings are holding it | The status row's `SuperStatus`, `WorkflowTarget`, `NextAutoLaunch` and deadline fields; on the Mac `/Library/Management/super/logs/` | Set `auth_mode` and a dedicated account ([§10.5](10-software-updates.md#105-super-superman)); re-check the preference values |
| super's preferences did not change after you edited them | The `com.vibemdm.super` profile is scoped like super itself but, like every profile, is re-evaluated only on save or **Deploy** | Configuration Profiles → `com.vibemdm.super` → deployments | Save the super configuration again or **Deploy** the profile |
| After rotating `VIBEMDM_ENCRYPTION_KEY`, reading the super configuration fails | The super password is not covered by `rotate-encryption-key` | Server log, component `super` | Re-enter `auth_password` ([§2.6](02-installation.md#26-the-encryption-key)) |

### DDM update plans

| Symptom | Likely cause | How to confirm | Fix |
| --- | --- | --- | --- |
| Overview state `no_ddm` | The Mac has never sent a DDM status report: macOS 12 or older, not MDM-enrolled, or the first `DeclarativeManagement` command was never delivered | `GET /api/v1/updates/overview`; `GET /api/v1/computers/{id}/ddm` is empty | `POST /api/v1/computers/{id}/ddm/sync`; check APNs (§22.3); Health `configuration.ddm_not_syncing` |
| State `unsupported` | Enforcement needs macOS 14 or later | The Mac's `os_version` | Use super or a legacy command for that Mac ([§10.7](10-software-updates.md#107-which-one-should-i-use)) |
| State `pending` for a long time | The deadline has not passed; the Mac is downloading; the user keeps deferring; or the update needs free disk space | The row's `install_state`, `pending_version`; Health `fleet.disk_space` (*Low disk space*): macOS updates need roughly 25 GB; on the Mac `softwareupdate --list` and System Settings → General → Software Update | Wait for the deadline; free disk space; check that the plan with the **highest target version** is the one you expect ([§10.1](10-software-updates.md#101-ddm-update-plans)) |
| State `failed` | The Mac reported `softwareupdate.failure-reason` | The row's `failure_reason`; `GET /api/v1/computers/{id}/ddm` | Act on the reason (disk space, battery, network); the Mac retries on its own |
| The plan's declaration is **invalid** on the Mac | See §22.4 (**Invalid declarations**) | | |
| The version picker is empty / `GET /api/v1/updates/catalog` returns 502 | The SOFA feed could not be fetched and nothing is cached yet (5-minute back-off) | Server log, component `api` | Allow outbound HTTPS to `sofafeed.macadmins.io`, or type the version by hand |
| `ScheduleOSUpdate` does nothing | Legacy command; on Apple silicon it needs the bootstrap token to authorize the install, and Apple has deprecated it | Inspector result; `bootstrap_token_escrowed` on the computer | Prefer a DDM plan or super |

## 22.8 Onboarding

| Symptom | Likely cause | How to confirm | Fix |
| --- | --- | --- | --- |
| No swiftDialog window appears on a new Mac | The server starts onboarding only when a **real user is logged in at the console** (not the login window, `root` or `_mbsetupuser`); the workflow is not in scope or disabled; the Mac already has a run; or the Mac has no agent | `GET /api/v1/computers/{id}/onboarding` (no run = never started); the workflow's **Runs** tab; `POST /api/v1/scope/preview` with the workflow's scope; `sudo vibemdm-agent status` | Log a user in and wait for the next check-in, or force it: `POST /api/v1/onboarding/workflows/{id}/run` `{"computer_ids": ["…"], "force": true}` ([§14.7](14-white-glove-onboarding.md#147-running-it-again)) |
| A run exists and is `completed`, but nobody saw a window | **swiftDialog was not installed** (`/usr/local/bin/dialog` missing) and Installomator could not install it, or no user was logged in when the run started (a forced run at the login window) | The run's `log` contains `swiftDialog unavailable (…); continuing without a window` or `no user logged in; running steps without a window`; `sudo vibemdm-agent status` → `swiftDialog: no` | Install swiftDialog (a Mac App with the `swiftdialog` label, or by hand), make sure the Mac can reach GitHub, and re-run with `force` while a user is logged in |
| Saving the workflow fails with `commas are not allowed in labels, placeholders or options` or `a title without commas is required` | swiftDialog uses commas to separate field options and list items, so VibeMDM rejects them | The `400` error names the field or step | Remove the commas (use a dash or "and") |
| A field's regex is not enforced | A regex that contains a comma is **ignored** by the agent for the same reason | The field's `regex` | Rewrite the pattern without commas (`[0-9]{4}` instead of `[0-9]{4,4}`) |
| Later steps show as `skipped` | An earlier step failed and `continue_on_error` was false | The run's `steps` | Fix the failing step, or set `continue_on_error` on nice-to-have steps |
| The entered values are not on the computer record | Unknown field keys or values longer than 256 characters are dropped | The run's `user_entry` vs the computer record | Use the fixed set of keys ([§14.2](14-white-glove-onboarding.md#142-fields)) |
| The Mac did not restart or log out at the end | The final action runs **only if the run completed** | The run's `status` | Fix the failed step |
| The run stays `queued` | The agent is offline or not installed | `sudo vibemdm-agent status` | Restart the daemon or install the agent |
| The run stays `running` | The user has not clicked **Continue** (30-minute wait) or the final button (10 minutes); a step is still installing; the whole run is limited to 3 hours | The run's `steps` and `log` | Wait, or look at the step that is in progress |
| The computer name did not change | The form had a `computer_name` field and the user typed a name (it wins over the template), or the template produced an empty string | The run's `user_entry` and the computer's `name` | Adjust the template or remove the field ([§14.3](14-white-glove-onboarding.md#143-naming-template)) |
| The simulator fails a step | By design: the simulator fails any step whose title contains "fail" | — | Rename the step |

## 22.9 MCP server

Every response from `/mcp` other than a JSON-RPC result is one of these:

| Status / message | Cause | Confirm | Fix |
| --- | --- | --- | --- |
| **404** (plain *404 page not found*) | The MCP server is **disabled**. `/mcp` is not served at all while `mcp_enabled` is false | `GET /api/v1/settings` → `mcp_enabled` | Enable it in Settings → General → *MCP server for AI agents* (`PUT /api/v1/settings` `{"mcp_enabled": true}`) |
| **401** `an API token is required (Authorization: Bearer vmdm_…)` | No `Authorization` header, a browser session cookie instead of a token, a revoked or expired token, or a token typed with a mistake | Settings → API Tokens: does the token exist and has `last_used_at` moved? | Create a token and send it as `Authorization: Bearer vmdm_…` ([§19.2](19-users-roles-api.md#192-api-tokens)) |
| **403** `cross-origin MCP requests are not allowed` | The request carried an `Origin` header that does not match the server's host (a browser-based client, a proxy that rewrites `Host`, or a tunnel on a different name) | Server log, component `mcp` | Use a client that sends no `Origin` (Claude Code, Claude Desktop, `mcp-remote`) or one on the same origin; make the proxy pass the original `Host` |
| **403** `password change required` | Not an MCP error: it is returned by the **admin API** to a browser session in the forced-password-change state. MCP accepts tokens only, so if you see it you are calling `/api/v1/…` with a cookie | — | Finish the password change (§22.10) |
| **405** `use POST` | The client opened an SSE stream (`GET`) or tried to delete a session (`DELETE`). VibeMDM's transport is Streamable HTTP with JSON responses only | — | Configure the client for Streamable HTTP without SSE; Claude Code's `--transport http` is correct |
| **429** `rate limit exceeded; slow down` | More than 120 requests per minute from one token | Audit log `mcp.tool` entries | Slow the agent down, or give separate agents separate tokens |
| **400** `batching is not supported` | The client sent a JSON-RPC batch array | — | Send one request per call (the current MCP specification no longer requires batching) |
| JSON-RPC error `unknown tool` for `send_push`, `run_policy`, … | Write tools are hidden from viewer tokens and from everyone while **Allow write tools** is off | `GET /api/v1/settings` → `mcp_allow_writes`; the token's role in Settings → API Tokens | Turn on write tools and use an **admin** token, if you really want the agent to act ([§19.6](19-users-roles-api.md#196-the-mcp-server-for-ai-agents)) |
| Health: **MCP server over plain HTTP** (`security.mcp_transport`) | MCP is on while the server URL is not HTTPS (critical if writes are allowed) | The finding | Serve HTTPS, or turn MCP off (the finding offers *Turn the MCP server off*) |
| A tool call times out | Tool calls are limited to 60 seconds; a `search_logs` or `compliance_overview` on a very large fleet can exceed it | Server log, component `mcp` | Narrow the query (`computer`, `limit`, `baseline`) |

## 22.10 Logins, sessions and tokens

| Symptom | Likely cause | How to confirm | Fix |
| --- | --- | --- | --- |
| Login returns **429** `too many failed attempts; try again later` | Six failed attempts locked the **client IP** and the **username** for 1 minute, then 5, 15 and 60 minutes on repeated bursts | Server log, component `api`: `login throttled` with the username and IP | Wait out the lockout (the server has no unlock command; a restart clears the in-memory counters). Check the password manager entry the user is retrying |
| Everyone is locked out at once, and the log shows the same IP for all users | The server sees your reverse proxy's address for every client because `VIBEMDM_TRUSTED_PROXIES` does not include the proxy | Audit entries all carry the proxy's IP | Add the proxy's IP or CIDR to `VIBEMDM_TRUSTED_PROXIES` and restart ([§2.3](02-installation.md#23-environment-variables)) |
| Every call returns **403** `password change required` | The account is in the forced-change state: the bootstrap `admin` on first login, or any user whose password an admin reset | `GET /api/v1/auth/me` works; everything else is 403 | Change the password in the console's forced-change screen, or `POST /api/v1/auth/password` `{"current_password", "new_password"}` (browser session only) |
| A viewer sees **403** `admin role required` on every change | Viewers are read-only by design | `GET /api/v1/auth/me` → `role` | Ask an admin, or change the role (`PUT /api/v1/users/{id}` `{"role": "admin"}`) |
| Login succeeds but the console immediately shows the login page again | The session cookie is `Secure` and the console was opened over plain HTTP, or an upstream proxy strips cookies | Browser developer tools → cookies | Use `https://`; for a loopback lab the cookie is not marked Secure |
| Login fails with a CSRF or same-origin error | The console is served from one host name and the API is called from another (a proxy rewrite, or a bookmark to the IP address) | Browser developer tools → the login request's `Origin` vs the page URL | Use the server URL consistently; make the proxy pass `Host` |
| Sessions expire too soon or too late | `VIBEMDM_SESSION_TTL_HOURS` (default 168) | — | Set it in the environment (compose does not pass it by default; add it to `docker-compose.yml`) |
| The only admin forgot the password | `VIBEMDM_ADMIN_PASSWORD` is used only when **no users exist**, so it cannot reset an existing account | — | Another admin resets it (`POST /api/v1/users/{id}/password`). If there is no other admin, restore a backup from when you knew the password, or edit the `admin_users` row in PostgreSQL with a bcrypt hash you generate yourself (there is no CLI for this) |
| An API token stopped working | It expired (`expires_at`) or was revoked | Settings → API Tokens | Create a new one |

## 22.11 Health findings that will not go away, and check errors

| Symptom | Cause | Fix |
| --- | --- | --- |
| A finding is still open after you fixed the problem | Findings resolve after **two consecutive clean runs** (the engine runs every 5 minutes), or at once with **Re-check now** | Click **Re-check now** on the finding or run the check (`POST /api/v1/health/checks/{id}/run`) |
| A finding you fixed came back within the hour, without a webhook | Flap protection reopened it quietly: the condition really did return | Treat it as current; check the facts on the finding |
| The Health page shows a **check error** and its findings never resolve | The check hit a database error or timeout. Errors never resolve findings, so stale findings are kept rather than guessed away | Server log, component `health`; `GET /api/v1/health/summary` → errors. Fix the database (disk, connections), then **Run all checks** (`POST /api/v1/health/run`) |
| **Encryption at rest** (`security.encryption_at_rest`) will not clear | `VIBEMDM_ENCRYPTION_KEY` is still unset | Set it and restart ([§2.6](02-installation.md#26-the-encryption-key)) |
| **Server URL** (`platform.server_url`) is critical | The URL is empty or unparseable; warning if it is `http://` | Settings → General |
| **SCEP CA certificate** (`platform.scep_ca`) warns | The CA (10-year lifetime) is within 90 days of expiry (critical at 14) | Plan a re-enrollment campaign; the CA cannot be renewed in place ([§3.2](03-apns-and-certificates.md#32-the-scep-ca-and-device-identities)) |
| **Profile signing certificate** (`platform.signing_certificate`) warns | The stored signing certificate expires within 30 days or cannot be parsed | Replace or clear it in Settings → General ([§3.3](03-apns-and-certificates.md#33-profile-signing)) |
| **Failing webhooks** (`platform.webhook_failures`) | The last five deliveries to an enabled webhook failed | `GET /api/v1/webhooks/deliveries` shows the status and error; fix the receiver or disable the hook |
| **Security features disabled** (`security.device_posture`) | SIP or Gatekeeper is off on a Mac | SIP can only be re-enabled from recoveryOS (`csrutil enable`); Gatekeeper can be enforced with a `com.apple.systempolicy.control` payload |
| **FileVault key not escrowed** (`security.filevault_key_missing`) | FileVault was on before the escrow profile arrived, so no PRK was ever encrypted to the escrow certificate | **Generate a new key with Escrow Buddy** (needs the agent), or `sudo fdesetup changerecovery -personal` on the Mac ([§11.3](11-filevault.md#113-missing-keys-escrow-buddy-remediation)) |
| **FileVault escrow not configured** (`security.filevault_escrow_profile`) | Macs with FileVault on are outside the escrow profile's scope | Widen the scope in **FileVault** and save ([§11.2](11-filevault.md#112-turn-it-on)) |
| **Scope references deleted objects** (`configuration.stale_scope_references`) | A policy, profile, app or plan names a deleted group, computer or segment | Open the object's scope and remove the dangling entry |
| **Failing app installs** (`configuration.app_install_failures`) | An Installomator label is broken or the vendor changed its download; Team ID mismatch | `GET /api/v1/apps/{id}/logs` shows Installomator's output; update Installomator (a new VibeMDM release) or use a custom label ([§9.5](09-apps-installomator.md#95-custom-labels)) |

The remaining checks (`fleet.mdm_unresponsive`, `fleet.agent_silent`, `fleet.agent_missing`, `fleet.disk_space`,
`platform.apns_certificate`, `platform.push_failures`, `platform.agent_binaries`, `security.mcp_transport`,
`security.remote_shell_transport`, `security.compliance_drift`, `security.compliance_scan_failures`,
`configuration.profile_failures`, `configuration.ddm_invalid`, `configuration.ddm_not_syncing`,
`configuration.policy_failures`, `configuration.policy_never_runs`) are covered in the sections above.

## 22.12 The server itself

| Symptom | Cause | Confirm | Fix |
| --- | --- | --- | --- |
| The container exits with status **2** at start | Invalid configuration: `VIBEMDM_DATABASE_URL` missing, or a bad `VIBEMDM_SESSION_TTL_HOURS` / `VIBEMDM_LOG_DB_LEVEL` | `docker compose logs vibemdm` prints the variable name | Fix `.env` / `docker-compose.yml` ([§2.3](02-installation.md#23-environment-variables)) |
| The container exits with status **1** and the log says the database is unreachable | PostgreSQL is not ready or the password changed | `docker compose ps`; `docker compose logs db` | Compose waits for the `db` health check; if you changed `POSTGRES_PASSWORD` after the first start, the database still has the old one |
| Exit 1 with a message about the encryption key or a secret that cannot be opened | `VIBEMDM_ENCRYPTION_KEY` is wrong or missing while sealed values exist. The server refuses to start rather than regenerate the CA or escrow identity | The log line names the table | Restore the correct key. If it is lost, the CA, APNs key and escrowed FileVault keys are unrecoverable ([§2.6](02-installation.md#26-the-encryption-key)) |
| Exit 1 with a message about the admin password | `VIBEMDM_ADMIN_PASSWORD` is shorter than 12 characters or a placeholder (`change-me-please`, `changeme`, `admin`, `password`, `vibemdm`, `adminadmin`, …) | — | Set a unique password or leave it empty to have one generated |
| `GET /healthz` returns `503 database unavailable` | PostgreSQL is down or out of connections | `docker compose logs db` | Restart or repair the database |
| Two servers started against one database and one hangs at start | Migrations hold a PostgreSQL advisory lock; the second waits | The log of the second instance | Expected; it continues when the first finishes. Do not run two different versions |
| Uploads of large packages fail through the proxy | The proxy's body-size limit or timeout | Proxy log | Raise the limit to match the server's 8 GB and 10-minute timeouts ([§2.2.1](02-installation.md#221-nginx-proxy-manager)) |
| Logs search is slow or `log records dropped because the database could not keep up` appears | The database cannot absorb the log volume; `VIBEMDM_LOG_DB_LEVEL=debug` is far too chatty for production | Server log (hourly summary of dropped records) | Set the level back to `info`; lower `log_retention_days`; check that the `pg_trgm` extension is available for search ([§18.4](18-logging-and-diagnostics.md#184-export-retention-and-archives)) |

## 22.13 Study notes: diagnosing a Mac

> **Study note (startup and recovery options):** on **Apple silicon**, hold the power button at startup for the
> **Options** screen (recoveryOS). From there: **Disk Utility** (First Aid), **Reinstall macOS**, **Startup Security
> Utility** (security policy per volume), **Terminal** (`csrutil`, `resetpassword`), **Share Disk** (replaces Target
> Disk Mode), and **Safe Mode** by holding Shift when selecting the startup disk. On Intel Macs the keys are
> Command-R (recovery), Shift (Safe Mode), D (Apple Diagnostics), Option (Startup Manager) and T (Target Disk Mode).
> Safe Mode loads only required kernel extensions, checks the startup disk, clears some caches and disables login
> items: it is the classic quick fix for third-party software conflicts.

> **Study note (tools you should recognize):** **Console** (unified log and crash reports), **Activity Monitor**
> (CPU, memory, energy, disk, network per process), **System Information** / `system_profiler`, **Disk Utility** and
> `diskutil`, **Keychain Access**, **Network Utility** is gone (use `ping`, `traceroute`, `nc`, `dig`, `networkQuality`),
> `sysdiagnose` for Apple support, `profiles` for MDM state, `log stream` / `log show` with predicates, `fdesetup`,
> `sysadminctl`, `softwareupdate`, `spctl`, `csrutil`, `tccutil`, `launchctl`. Know roughly what each shows and
> whether it needs `sudo`.

> **Study note (MDM-specific diagnosis on a Mac):** `profiles status -type enrollment` (enrolled? user approved?
> ADE?), `sudo profiles list` and `sudo profiles show` (what is installed), `sudo profiles renew -type enrollment`
> (re-run enrollment checks; on an ADE Mac re-fetch its assignment), the `mdmclient` process and the
> `com.apple.ManagedClient` subsystem in the unified log, `sudo profiles status -type bootstraptoken`, and
> `system_profiler SPConfigurationProfileDataType`. A Mac that cannot reach APNs will still enroll partially and will
> never receive commands; Apple documents the required hosts and ports in *Use Apple products on enterprise
> networks*.

Next: [Chapter 23 — Study guide](23-study-guide.md).
