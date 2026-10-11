# 6. Scoping

Every object that acts on Macs has a **scope** that decides which Macs it applies to. VibeMDM uses the same scope
model everywhere, modelled on Jamf Pro: **targets**, **limitations** and **exclusions**.

```
in scope  =  (All computers  OR  matches a target)
             AND  matches every limitation category that is set
             AND NOT  matches any exclusion
```

## 6.1 The scope object

```json
{
  "all_computers": false,
  "targets": {
    "computer_ids": [], "group_ids": [], "buildings": [], "departments": [],
    "network_segments": [], "users": []
  },
  "limitations": { "users": [], "network_segments": [] },
  "exclusions": {
    "computer_ids": [], "group_ids": [], "buildings": [], "departments": [],
    "network_segments": [], "users": []
  }
}
```

| Section | Kinds | Logic |
| --- | --- | --- |
| `all_computers` | — | `true` selects every computer as a target |
| `targets` | specific computers, groups (static or smart), buildings, departments, network segments | A Mac is a target if it matches **any** entry |
| `limitations` | users, network segments | Each **category** that is set must match (AND between categories, OR within one) |
| `exclusions` | computers, groups, buildings, departments, network segments, users | A Mac that matches **any** exclusion is out, whatever else is set |

Matching rules:

| Kind | Matches when |
| --- | --- |
| Computer | The computer ID is listed |
| Group | The computer is a member (smart groups are evaluated live) |
| Building / department | The computer's `building` / `department` equals a listed name (case-insensitive) |
| Network segment | The computer's last reported IP address is inside the segment's range |
| User | The computer's `assigned_user` **or** `last_user` equals a listed username (case-insensitive) |

The evaluator checks in this order: exclusions, then limitations, then `all_computers`, then targets. So
**limitations also narrow "All computers"**, and **exclusions always win**.

Users are designed for limitations and exclusions; the evaluator also honours a `users` list in targets, but the
console may not offer it. The API also accepts the older flat shape (`computer_ids`, `group_ids`,
`excluded_computer_ids`, `excluded_group_ids`) and converts it.

### Preview

`POST /api/v1/scope/preview` `{"scope": {...}}` returns `count` and up to 500 matching computers. The console's
scope editors use it. Preview before saving anything destructive.

## 6.2 Where scope is used, and when it is evaluated

Scope is not evaluated at the same moment for every object type. This matters when group membership changes.

| Object | When scope is evaluated | Effect of a Mac entering or leaving scope later |
| --- | --- | --- |
| Policy | At every agent check-in (and at Self Service run) | Applies at the next check-in. Saving or flushing a policy wakes in-scope agents |
| Mac App (Installomator) | At `checkin` and `startup` check-ins | Installs at the next check-in if due |
| Restricted software | At `checkin` and `startup` check-ins | Rules update at the next check-in |
| Compliance baseline scans | Every 10 minutes by the scheduler | Scans start when due |
| DDM update plan | Every 30 minutes by the DDM reconciler (and on save) | Declarations change at the next reconcile; the Mac is asked to sync |
| super deployment (install) | Every 30 minutes by the super reconciler | Installs when the Mac is reconciled |
| Onboarding workflow | At each agent check-in until the Mac has a run | Starts when a real user is logged in |
| **Configuration profile** (including the generated FileVault, compliance, super and XCreds profiles) | **Only** when the profile is saved, when you click **Deploy**, and when a Mac first enrolls in MDM | **Not automatic.** A Mac that joins the scope later (for example because a smart group now matches it) does not get the profile, and a Mac that leaves scope keeps it, until the profile is saved or deployed again |

> **Operational rule for profiles:** after you change the membership of a group that a profile is scoped to, open
> the profile and click **Deploy** (`POST /api/v1/profiles/{id}/deploy`). VibeMDM then installs it where it is
> missing and removes it where it is now out of scope. `{"reinstall": true}` resends it everywhere.

## 6.3 Worked examples

### Example 1: everyone except the pilot ring

*Goal:* deploy a policy to all Macs except a static group "Pilot".

```json
{ "all_computers": true,
  "targets": {}, "limitations": {},
  "exclusions": { "group_ids": ["<pilot-group-id>"] } }
```

### Example 2: a smart group, but only on the office network

*Goal:* install a printer only on "Finance Macs" (a smart group on `department is Finance`) when they are on the
HQ LAN.

```json
{ "targets":     { "group_ids": ["<finance-smart-group>"] },
  "limitations": { "network_segments": ["<hq-lan-segment>"] },
  "exclusions":  {} }
```

A Finance Mac at home has a non-HQ IP address and is out of scope until it comes back. Policies re-evaluate at
every check-in, so this kind of location-aware scope works well for **policies**. For a **profile** it would not
follow the Mac around, because profile scope is only evaluated on save or deploy (§6.2).

### Example 3: limit to named users

*Goal:* a Self Service policy only for two developers, on whichever Mac they use.

```json
{ "all_computers": true,
  "limitations": { "users": ["alice", "bob"] } }
```

A Mac is in scope when its assigned user or last logged-in user is `alice` or `bob`.

### Example 4: buildings with an exclusion

*Goal:* everything in buildings "HQ" and "Annex", except loaner Macs (static group) and one kiosk.

```json
{ "targets":    { "buildings": ["HQ", "Annex"] },
  "exclusions": { "group_ids": ["<loaners>"], "computer_ids": ["<kiosk-id>"] } }
```

### Example 5: why is this Mac not in scope?

Work through the order the evaluator uses:

1. **Exclusions.** Is the Mac in an excluded group, building, department or segment, or is its assigned or last
   user excluded? If so, it is out.
2. **Limitations.** If user limitations are set, is `assigned_user` or `last_user` one of them? If segment
   limitations are set, is `ip_address` inside one? Macs without the agent have no IP address and fail every
   segment limitation.
3. **Targets.** Is `all_computers` on, or does the Mac match at least one target?
4. For profiles: has the profile been deployed since the Mac became eligible?

Use **Preview** with the exact scope, and check the computer's inventory fields (`building`, `department`,
`last_user`, `ip_address`, group memberships).

## 6.4 Deleted objects in a scope

If a group, computer or network segment named in a scope is deleted, the scope keeps the dangling ID, which
simply matches nothing. The Health check **Scope references deleted objects** lists every policy, profile, app
and update plan affected so you can clean them up ([Chapter 15](15-health.md)).

> **Study note:** Apple's own tools don't define a scope model. Scoping is an MDM-vendor concept. The exam
> concepts behind it are **device groups** and **assignment**: in ABM/ASM, devices are assigned to an MDM server
> and apps or books are assigned to users or devices; within the MDM, profiles and apps are assigned to groups.
> Know the difference between **device-channel** assignments (apply to the whole Mac, installed in the System
> domain) and **user-channel** assignments (apply to one user, require the user's MDM channel). VibeMDM uses only
> the device channel.

Next: [Chapter 7 — Policies, scripts and packages](07-policies-scripts-packages.md).
