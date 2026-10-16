# Installomator

VibeMDM vendors [Installomator](https://github.com/Installomator/Installomator) v10.8 (Apache-2.0, license served at
`/agent/installomator-license`) and uses it in three places:

| Where | What it does |
| --- | --- |
| **Mac apps catalog** | Keep an app installed (and updated weekly, or install once) on every Mac in scope, optionally in Self Service |
| **Policy step** | A policy's *Apps* list runs Installomator labels alongside scripts and packages, with the policy's trigger, frequency and logs |
| **Escrow Buddy remediation** | FileVault key repair installs Escrow Buddy with the `escrowbuddy` label |

Every install verifies the download's code signature against the label's Apple Developer **Team ID** before installing.

## Labels

Over a thousand labels ship with Installomator (`GET /api/v1/apps/labels`). For anything else, define a **custom label**
(`/api/v1/custom-labels`). VibeMDM passes it to Installomator's documented `valuesfromarguments` mechanism:

| Field | Required | Notes |
| --- | --- | --- |
| `key` | yes | lowercase identifier; reference it as `custom:<key>` |
| `name` | yes | app or package name |
| `type` | yes | `dmg`, `pkg`, `zip`, `tbz`, `pkgInDmg`, `pkgInZip`, `appInDmgInZip` |
| `download_url` | yes | plain `https://` URL |
| `expected_team_id` | yes | 10-character Team ID; the install fails if the signature doesn't match |
| `package_id`, `app_name`, `archive_name`, `version_key`, `app_new_version`, `blocking_processes` | no | as in Installomator labels |

`POST /api/v1/apps/installomator-preview` shows the exact command a label and options produce.

## Options

| Option | Values | Installomator variable |
| --- | --- | --- |
| Notify | `success`, `silent`, `all` | `NOTIFY` |
| Blocking process action | `ignore`, `silent_fail`, `prompt_user`, `prompt_user_loop`, `prompt_user_then_kill`, `tell_user`, `tell_user_then_kill`, `kill` | `BLOCKING_PROCESS_ACTION` |
| Reopen | `yes`, `no` | `REOPEN` |
| Ignore App Store apps | `yes`, `no` | `IGNORE_APP_STORE_APPS` |
| Force install | on/off | `INSTALL=force` |

## Why arguments are quoted

Installomator runs `eval` on every `key=value` argument. A crafted value such as
`downloadURL=https://x/$(…)` would otherwise execute as root on every Mac. The server therefore validates each field
(https-only URLs, strict patterns, enum options) **and** single-quotes every value, so `eval` can only ever assign a
literal string. The agent passes the arguments as argv without a shell. Tests in `internal/installomator` run hostile
values through a real shell `eval` to prove they stay literal.
