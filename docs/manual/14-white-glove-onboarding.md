# 14. White-glove onboarding

Onboarding workflows give a new Mac a guided first-run experience, similar to Jamf Setup Manager or DEPNotify. When
a Mac in scope first has a real user logged in, the agent shows a [swiftDialog](https://github.com/swiftDialog/swiftDialog)
(MIT) window that:

1. collects details in a **welcome form** (computer name, asset tag, department, …);
2. names the Mac from a **naming template**;
3. runs **steps** (install apps, run policies and scripts, update inventory, wait) with a live progress list;
4. finishes with a message and an optional **restart, log out or shut down**.

**Onboarding** in the sidebar lists the workflows; each workflow page has the editor (with a live preview of the
dialog) and a **Runs** tab, and every computer page has an **Onboarding** tab with that Mac's runs. This chapter
describes the objects and behaviour through the API, which the console uses.

## 14.1 Workflows

| Field | JSON | Notes |
| --- | --- | --- |
| Name | `name` | Required |
| Enabled | `enabled` | |
| Priority | `priority` | Lower numbers win. Workflows are ordered by `priority`, then name |
| Scope | `scope` | [Chapter 6](06-scoping.md) |
| Content | `config` | Below |

`config`:

| Field | Notes |
| --- | --- |
| `title` | Window title (default `Setting up your Mac`) |
| `message` | Text shown in the window (swiftDialog supports Markdown) |
| `icon` | A URL, a file path, or an SF Symbol (`SF=laptopcomputer`) |
| `banner` | Optional banner image URL |
| `blur_screen` | Blur everything behind the window |
| `fields` | Welcome-form fields (§14.2) |
| `computer_name_template` | e.g. `MAC-{serial}` or `{asset_tag}-{user}` (§14.3) |
| `steps` | Progress steps (§14.4) |
| `final_action` | `none`, `restart`, `logout` or `shutdown` |
| `final_message` | Shown at the end (default `Your Mac is ready.`) |

API: `GET/POST /api/v1/onboarding/workflows`, `GET/PUT/DELETE /api/v1/onboarding/workflows/{id}`.

A workflow must have at least one field or one step.

## 14.2 Fields

| Field property | Notes |
| --- | --- |
| `key` | One of `computer_name`, `asset_tag`, `building`, `department`, `room`, `assigned_user`, `assigned_real_name`, `assigned_email`, `assigned_position`. Each key may appear once |
| `label` | Defaults to a friendly name for the key |
| `required` | The user can't continue without a value |
| `placeholder` | Prompt text inside the field |
| `options` | A list of values: shows a picker instead of a text field |
| `regex`, `regex_error` | Validation pattern and message (text fields only) |

swiftDialog separates field options with commas, so **commas are not allowed** in labels, placeholders, options or
regex error messages (the server rejects them). A regex that contains a comma is ignored by the agent.

The values the user enters are written to the computer record: `computer_name` → name, and the others to the
matching inventory fields (asset tag, building, department, room, assigned user, full name, e-mail, position).
Values longer than 256 characters and unknown keys are dropped.

## 14.3 Naming template

If the form has no `computer_name` field (or it was left empty), the template is used:

| Placeholder | Replaced with |
| --- | --- |
| `{serial}` | The Mac's serial number |
| `{asset_tag}` | The asset tag entered in the form |
| `{user}` | The `assigned_user` entered, otherwise the logged-in user's short name |
| `{model}` | A short model code: `MBP` (MacBook Pro), `MBA` (MacBook Air), `MINI` (Mac mini), `IMAC`, `MP` (Mac Pro), `MAC` (others) |

The agent sets `ComputerName` to the result, and `LocalHostName` and `HostName` to a sanitized version (letters,
digits and hyphens only, at most 63 characters), with `scutil --set`. Because the Mac itself is renamed, the name
survives later check-ins.

Example: template `{model}-{asset_tag}` with asset tag `A-1042` on a MacBook Pro gives `MBP-A-1042`.

## 14.4 Steps

| `type` | Required fields | What it does |
| --- | --- | --- |
| `app` | `app_id` (a Mac Apps catalog entry) | Installs the app with Installomator, using the app's label and options |
| `policy` | `policy_id` | Runs the policy (trigger `onboarding`; a policy log is created) |
| `script` | `script_id` | Runs a library script (no parameters) |
| `recon` | — | Submits full inventory |
| `wait` | `seconds` (1–3600) | Waits, for example for a profile to arrive |

Every step has a `title` (required, no commas), optional `subtitle` and `icon`, and `continue_on_error`. When a step
fails and `continue_on_error` is false, the remaining steps are marked **skipped** and the run fails.

Example workflow:

```json
{
  "name": "Standard laptop",
  "enabled": true,
  "priority": 10,
  "scope": { "group_ids": ["<new-laptops>"] },
  "config": {
    "title": "Welcome to Example Corp",
    "message": "We're setting up your Mac. This takes about 15 minutes.",
    "icon": "SF=laptopcomputer",
    "fields": [
      { "key": "asset_tag", "label": "Asset tag", "required": true, "regex": "^A-[0-9]{4}$", "regex_error": "Use the format A-1234" },
      { "key": "department", "label": "Department", "required": true, "options": ["Engineering", "Finance", "Sales"] }
    ],
    "computer_name_template": "{model}-{asset_tag}",
    "steps": [
      { "type": "app", "title": "Google Chrome", "app_id": "<chrome-app-id>" },
      { "type": "app", "title": "Slack", "app_id": "<slack-app-id>", "continue_on_error": true },
      { "type": "policy", "title": "Printers and Dock", "policy_id": "<policy-id>" },
      { "type": "recon", "title": "Updating inventory" }
    ],
    "final_action": "restart",
    "final_message": "All done! Your Mac will restart."
  }
}
```

## 14.5 When a workflow runs

At every agent check-in, the server looks for onboarding to start:

1. If the console user is empty, `root`, `_mbsetupuser` or `loginwindow`, nothing happens. swiftDialog needs a
   **logged-in user**.
2. It walks the **enabled** workflows in priority order and stops at the **first one in scope**.
3. If that workflow has never run on this Mac, it creates a run (status `queued`) and an `onboarding` agent task,
   then wakes the agent. If it already ran, nothing happens, and lower-priority workflows are not considered.

So only one workflow applies to a Mac: the highest-priority one in scope.

### On the Mac

1. If a user is logged in and `/usr/local/bin/dialog` is missing, the agent installs swiftDialog with Installomator
   (`swiftdialog` label, silent). If swiftDialog can't be installed, the run continues **without a window**.
2. **Welcome form**: a swiftDialog window with the fields (`--textfield`, `--selecttitle`/`--selectvalues`,
   `--json`), the title, message, icon, banner and blur. It waits up to 30 minutes for **Continue**.
3. The naming template is applied (§14.3) and the entered values are sent to the server (run status `running`;
   the computer record is updated).
4. **Progress window**: a second swiftDialog window with one list row per step and a progress bar, driven through
   the command file `/var/tmp/vibemdm-onboarding.log`. Each row shows *Working…*, then *Done*, *Failed* or
   *Skipped*.
5. At the end, the window shows the final message (or "Some items could not be completed. IT has been notified."
   on failure) and enables the button (**Restart**, **Log Out**, **Shut Down** or **Done**). It waits up to 10 minutes
   for the user to click it.
6. The agent submits the result, then performs the final action, only if the run completed: `shutdown -r +1`,
   `shutdown -h +1`, or logs the user out.

The whole run has a 3-hour limit. With no user logged in (for example a manual run on a Mac at the login window),
the steps run silently without windows.

> "IT has been notified" refers to the run being recorded as failed in VibeMDM. There is no onboarding-specific
> webhook or Health check yet; watch the runs list.

## 14.6 What admins see

| Endpoint | Shows |
| --- | --- |
| `GET /api/v1/onboarding/workflows/{id}/runs?computer_id=&limit=` | Runs of a workflow, newest first |
| `GET /api/v1/computers/{id}/onboarding` | Runs on one Mac |

Each run has `status` (`queued`, `running`, `completed`, `failed`), `user_entry` (the values collected), `steps`
(title, type, status `success`/`failed`/`skipped`, detail, duration), the agent's `log`, and timestamps. The audit
log records `onboarding.user_entry` and `onboarding.run`.

## 14.7 Running it again

A Mac runs each workflow once. To run it again, for example after fixing a broken step:

```json
POST /api/v1/onboarding/workflows/{id}/run
{ "computer_ids": ["<computer-id>"], "force": true }
```

`force: true` resets the previous run first; without it, a Mac that already has a run is skipped with an error. The
request queues the run immediately (the logged-in-user check does not apply), so make sure someone is at the Mac
if the workflow has fields.

## 14.8 Requirements and tips

- **A logged-in user** is required for any window. Automatic starts wait for one.
- The agent must be installed. Onboarding is not available on MDM-only Macs.
- Put long installs late, and use `continue_on_error` for nice-to-have apps.
- Use a `wait` step if a later step depends on a configuration profile that arrives over MDM.
- Test with the simulator: `vibemdm-sim` fakes the form (an asset tag `A-1xxx`, the first option of each picker),
  applies the naming template, and fails any step whose title contains "fail"
  ([Chapter 21](21-cli-reference.md#213-vibemdm-sim)).

> **Study note:** Apple's own first-run tools are **Setup Assistant** and, with ADE, **Await Configuration**
> (`AwaitingConfiguration` in `DeviceInformation`; the MDM sends `DeviceConfigured` to release the device) and
> **enrollment customization** (a web view during Setup Assistant). Third-party onboarding tools (swiftDialog,
> DEPNotify, Setup Manager) run after the user reaches the desktop, or at the login window, and are driven by an
> agent. Without ADE, VibeMDM can only start after a user logs in.

Next: [Chapter 15 — Health](15-health.md).
