# The VibeMDM agent (`vibemdm-agent`)

The agent is a single static binary for macOS (Apple silicon and Intel). It complements the MDM channel the way
the Jamf binary complements Jamf Pro: MDM delivers profiles and management commands; the agent runs scripts,
installs packages, collects detailed inventory, and reacts to triggers.

## Installation

From the **Enrollment** page copy the one-liner (or create an invitation link with its own token):

```bash
curl -fsSL "https://mdm.example.com/agent/install.sh?token=…" | sudo bash
```

The installer downloads the binary for the Mac's architecture to `/usr/local/bin/vibemdm-agent`, runs
`vibemdm-agent enroll`, and installs:

| Path | Purpose |
| --- | --- |
| `/Library/LaunchDaemons/com.vibemdm.agent.plist` | daemon (`vibemdm-agent daemon`), KeepAlive |
| `/Library/LaunchAgents/com.vibemdm.agent.login.plist` | fires the **login** trigger as each user logs in |
| `/Library/Application Support/VibeMDM/agent.json` | server URL + device token (root only) |
| `/Library/Logs/VibeMDM/vibemdm-agent.log` | agent log (rotated; Console → Log Reports), see `vibemdm-agent logs` |

Add `--mdm` to `enroll` (the installer does, unless `install.sh?mdm=0`) and the agent downloads the MDM
enrollment profile and opens *System Settings* so the user can approve it.

Building the binaries yourself: `make agent` places them in `bin/agents/`; the Docker image already includes them.
The agent is not code-signed or notarized by default; the installer clears the quarantine attribute. If you have a
Developer ID, sign and notarize the binaries and place them in the agent directory.

## How it works

- **Check-in** every *N* seconds (Settings → General, default 5 min) with trigger `checkin`. The server returns
  the policies due for this Mac (scope + trigger + frequency), queued tasks, and whether inventory is due.
- **Long-poll** for tasks (`GET /agent/v1/tasks?wait=55`): "Run script now" from the console executes within a second.
  The same channel carries on-demand check-in requests: saving or flushing a policy, or clicking *Check in now*, makes the
  agent evaluate policies immediately instead of waiting for the interval.
- **Resilience**: results that cannot be delivered are spooled under `Application Support/VibeMDM/spool` and replayed;
  check-ins back off (30 s → 8 min) after failures and recover instantly when the long-poll reconnects.
- **Packages** are downloaded concurrently (up to 3 at once) while "before" scripts run, verified by SHA-256, and cached
  by checksum for 14 days so repeated policies install from the local copy.
- **Triggers**: `startup` (daemon start within 10 min of boot), `login` (LaunchAgent → daemon over a local socket),
  `checkin`, `enrollment_complete` (first check-in after enrollment), and **custom** events
  (`sudo vibemdm-agent policy --event my-event`, like `jamf policy -event`).
- **Recon** (`vibemdm-agent recon`): hardware, OS, network, security, apps (Info.plist walk), profiles, local users,
  extension attributes.

## Scripts

Scripts run as root with the familiar parameter convention:

| Variable | Value |
| --- | --- |
| `$1` | mount point (`/`) |
| `$2` | computer name |
| `$3` | console username (empty at login window) |
| `$4` – `$11` | policy/script parameters |

Environment variables: `VIBEMDM_SERVER_URL`, `VIBEMDM_COMPUTER_ID`, `VIBEMDM_SCRIPT_NAME`, `VIBEMDM_POLICY_NAME`.
Output (stdout+stderr, 64 KB) and the exit code are recorded; non-zero marks the step failed.

**Extension attributes** are scripts whose stdout becomes the value. `<result>…</result>` tags are honoured.

## Commands

```
vibemdm-agent enroll --server URL [--token TOKEN] [--mdm] [--allow-insecure-http]
                                       # token can also come from VIBEMDM_ENROLL_TOKEN (keeps it out of ps)
vibemdm-agent status                   # enrollment, connectivity, log level, local components
vibemdm-agent logs [-n N] [-f] [--level L] [--component C] [--grep T]   # show or follow the agent log
sudo vibemdm-agent logs --set-level debug                          # change the log level live
vibemdm-agent policy [--event NAME]    # run policies now
vibemdm-agent trigger NAME             # fire a trigger through the daemon
vibemdm-agent recon                    # submit inventory
vibemdm-agent check-connection
vibemdm-agent selfservice              # open the Self Service portal
vibemdm-agent manage                   # reinstall launchd jobs
vibemdm-agent remove-framework         # unenroll + remove
vibemdm-agent help <command>           # detailed help for one command
```

Man page: `man ./docs/man/vibemdm-agent.8`.

## What else the agent does

Beyond scripts, packages and inventory, the daemon also:

- runs **compliance scans** (mSCP shell checks, optionally the fix scripts) and reports per-rule results;
- shows **onboarding workflows** with swiftDialog, collects the welcome-form answers, renames the Mac and runs the
  configured steps with a live progress list (needs a logged-in user; swiftDialog is installed via Installomator);
- installs **super** (checksum-verified) when the Mac is in the super deployment's scope, and reads super's status
  plist into inventory;
- hosts the local **Self Service** portal and the audited **remote shell** (which a root-owned
  `/Library/Application Support/VibeMDM/disable-remote-shell` file vetoes on the device).

Only the built-in `login`, `checkin` and `network_change` triggers can be fired by unprivileged users; custom
triggers go through a root-only socket (`sudo vibemdm-agent trigger NAME`).

## Uninstall

`sudo vibemdm-agent remove-framework` unenrolls, unloads launchd jobs, and deletes the support directory.
Remove the MDM profile from the console (Management → Remove MDM Profile) or in System Settings.
