# 12. Compliance (mSCP)

VibeMDM integrates the **macOS Security Compliance Project** (mSCP), the NIST-led project that turns security
frameworks into concrete macOS rules with check scripts, fix scripts and configuration-profile content. VibeMDM
embeds an mSCP catalog, lets you tailor and scope **baselines**, scans Macs through the agent, optionally
remediates, generates an **enforcement profile**, and reports results with drift tracking.

## 12.1 The embedded catalog

| Item | Value |
| --- | --- |
| Source | macOS Security Compliance Project (`usnistgov/macos_security`) |
| Release | `release_27.0` (platform 27.0) |
| Rules | 350 |
| License | CC BY 4.0, see `internal/compliance/NOTICE.md` |

Benchmarks (baselines) shipped in this catalog, with their rule counts:

| Key | Benchmark | Rules |
| --- | --- | --- |
| `800-53r5_low` / `_moderate` / `_high` | NIST SP 800-53 rev. 5 Low / Moderate / High | 177 / 222 / 231 |
| `800-171` | NIST SP 800-171 | 214 |
| `cis_lvl1`, `cis_lvl2` | CIS Apple macOS Benchmark Level 1 / Level 2 | 96 / 117 |
| `cisv8` | CIS Controls v8 | 180 |
| `cmmc_lvl1`, `cmmc_lvl2` | Cybersecurity Maturity Model Certification Level 1 / 2 | 92 / 212 |
| `cnssi-1253_low` / `_moderate` / `_high` | CNSSI 1253 | 245 / 253 / 264 |
| `disa_stig` | DISA STIG | 162 |
| `hicp_lp` | Probably HICP (Health Industry Cybersecurity Practices), large practices; the catalog only gives the key, so check the mSCP documentation | 146 |
| `all_rules` | Every rule | 322 |

Browse it with:

| Endpoint | Returns |
| --- | --- |
| `GET /api/v1/compliance/catalog` | Source, release, license and each benchmark with its rule count, number of scannable rules and number with profile content |
| `GET /api/v1/compliance/catalog/rules?benchmark=cis_lvl1` | Rules in a benchmark (`&all=1` for every rule, `&scripts=1` to include check/fix scripts) |
| `GET /api/v1/compliance/catalog/rules/{rule}?benchmark=…` | One rule with discussion, references, tags, severity, platforms, scripts |

**Attribution:** mSCP includes contributions from the US Government, the public and Apple, and is licensed under
CC BY 4.0. VibeMDM converted the rule and baseline YAML to a compact JSON catalog, keeping only the fields it needs.
Files containing "Vendor Description content Copyright … Apple Inc." are excluded by the importer, because that
content is not licensed under CC BY 4.0. VibeMDM is not affiliated with or endorsed by NIST, NASA, DISA, Los Alamos
National Laboratory, CIS or Apple. Developers regenerate the catalog with `cmd/mscp-import`
([Chapter 21](21-cli-reference.md#214-mscp-import)).

## 12.2 Baselines

A **baseline** is a benchmark that you tailored and scoped.

| Field | Notes |
| --- | --- |
| `name`, `description` | |
| `benchmark` | A catalog key such as `cis_lvl1` |
| `enabled`, `scope` | Disabled baselines are not scanned and their profile is withdrawn |
| `extra_rules` | Rule IDs added on top of the benchmark (must exist in the catalog) |
| `exclusions` | `{"rule_id": "reason"}`. Exempted rules are not scanned or enforced. **A reason is required** |
| `odv_overrides` | `{"rule_id": value}`. Organization-defined values, validated against the rule's ODV type and limits |
| `scan_interval_hours` | 1–720, default 24 |
| `auto_remediate` | Scheduled scans also run fix scripts for failing rules |
| `deploy_profile` | Generate and deploy the enforcement profile |

API: `GET/POST /api/v1/compliance/baselines`, `GET/PUT/DELETE /api/v1/compliance/baselines/{id}`.

### Create and tailor a baseline

1. **Compliance → New baseline**. Pick a benchmark, for example `cis_lvl1`.
2. Review the rules. For each rule you can see its severity, whether it has a check, a fix and profile content,
   and why it can't be scanned (if so).
3. **Exempt** rules that don't fit, with a reason (for example `os_airdrop_disable`: "AirDrop is approved for
   design staff").
4. **Set ODVs** where your policy differs from the benchmark's value (for example the screen-saver idle time or
   the password minimum length).
5. Add **extra rules** from other benchmarks if needed.
6. Choose the scan interval, auto-remediation and whether to deploy the enforcement profile.
7. Scope it to a pilot group and save.

### How rules are resolved per Mac

For each Mac, VibeMDM builds the rule list for its **macOS major version and CPU architecture**:

| Rule state | Why | Scanned? |
| --- | --- | --- |
| `excluded` | Listed in `exclusions` | No |
| `manual` | Tagged `inherent`, `permanent`, `n_a`/`not_applicable` or `supplemental`; or has no automated check; or is tagged `manual` | No |
| `not_applicable` | Not defined for this macOS version, or limited to the other architecture (`arm64` / `i386` tags) | No |
| scannable | Has a check script and an expected result for this version | Yes |

Version-specific check, result and fix overrides are used where mSCP defines them. ODVs are substituted into the
check, the fix and the expected result (`$ODV`), using your override, else the benchmark's value, else
`recommended`.

## 12.3 Scans

Scans run through the agent as a `compliance_scan` task.

1. The server sends each scannable rule's `check` script, its `expected_type` (`string`, `integer` or `base64`) and
   expected value, and, if remediation was requested, its `fix` script.
2. The agent runs each check the way mSCP's own script does: **`/bin/zsh -c`** as root, with a 60-second timeout
   per check (5 minutes per fix, 45 minutes per scan).
3. It compares the output with the expected value:

   | Result | Meaning |
   | --- | --- |
   | `pass` | Output matches |
   | `fail` | Output differs |
   | `error` | The check could not run (timeout, error) |
   | `remediated` | It failed, the fix ran, and the check then passed |

4. It posts the results (`POST /agent/v1/compliance-scans/{id}`). The server ignores results for rules it did not
   ask about.

| How scans start | Trigger label |
| --- | --- |
| The scheduler (every 10 minutes) queues a scan when the Mac's last scan of that baseline is older than `scan_interval_hours`. It uses the baseline's `auto_remediate` | `scheduled` |
| **Scan now**: `POST /api/v1/compliance/baselines/{id}/scan` `{"computer_ids": [...], "remediate": false}`. An empty list scans the whole scope; omitting `remediate` uses the baseline setting | `manual` / `manual_remediate` |
| Health self-heal (*Rescan affected Macs*) | `health` |
| MCP `start_compliance_scan` (never with remediation) | `mcp` |

Scans need the agent. Macs without it are skipped with a reason. Scans that never report back expire (checked
hourly). Scan history: `GET /api/v1/compliance/baselines/{id}/scans?computer_id=&limit=`.

> **Caution with remediation:** mSCP fix scripts change system settings as root. Some rules (password policy, SSH,
> Bluetooth, sharing services) can lock users out of workflows. Run a scan without remediation first, review the
> failing rules, exempt what doesn't fit, test remediation on a pilot group, and prefer the **enforcement profile**
> for settings that a profile can manage. A profile keeps the setting enforced; a fix script sets it once.

## 12.4 The enforcement profile

With `deploy_profile` on, VibeMDM merges the configuration-profile content of every non-exempt rule into one
profile, `com.vibemdm.compliance.<baseline-id>` ("Compliance: <name>"):

- payloads of the same type are merged key by key;
- managed preferences (`com.apple.ManagedClient.preferences`) are emitted per domain in MCX `Forced` form;
- `$ODV` placeholders are replaced, keeping numeric types.

Preview it with `GET /api/v1/compliance/baselines/{id}/profile` (payload, the rule IDs it covers, and whether it
is deployed). It is scoped like the baseline and withdrawn when the baseline is disabled or `deploy_profile` is
turned off. Edit the baseline, not the profile.

## 12.5 Reports

| Endpoint | Shows |
| --- | --- |
| `GET /api/v1/compliance/baselines/{id}/report` | Summary (`targeted`, `scanned`, `compliant`, average `score`, `failing_rules`, `drifted`, `remediated`), one row per Mac, one row per rule sorted by failures |
| `GET /api/v1/compliance/baselines/{id}/computers/{cid}` | One Mac against the baseline: every rule with status `pass`, `fail`, `error`, `remediated`, `not_scanned`, `excluded`, `manual` or `not_applicable`, the actual and expected values, and whether a fix exists |
| `GET /api/v1/computers/{id}/compliance` | Every baseline for one Mac |
| `GET /api/v1/compliance/baselines/{id}/export.csv` | Latest result of every rule on every Mac |

A Mac is **compliant** when its latest results have no failures and no errors. The baseline **score** is the
average of per-Mac scores.

CSV columns: `computer`, `rule_id`, `title`, `severity`, `status`, `previous_status`, `actual`, `expected`, `detail`,
`checked_at`. Cells that start with `=`, `+`, `-`, `@`, tab or carriage return are prefixed with `'` to neutralize
spreadsheet formula injection.

### Drift

Every result keeps its **previous status**. A rule that was `pass` or `remediated` and is now `fail` has
**drifted**: someone changed a setting or a profile was removed. Drift is counted in the report and feeds two
Health checks:

| Check | Condition | Remediation |
| --- | --- | --- |
| **Compliance drift** | Rule results regressed on one or more Macs (lists recent rules) | *Rescan affected Macs* (safe; uses the baseline's auto-remediate setting) |
| **Compliance scans failing** | At least 3 scans of a baseline failed or expired in 2 days | Link to the report |

## 12.6 Study notes: security baselines

> **Study note (frameworks):**
> - **NIST SP 800-53** is the US federal catalog of security and privacy controls, with Low/Moderate/High
>   baselines. **NIST SP 800-171** protects Controlled Unclassified Information in non-federal systems. **CMMC**
>   builds on 800-171 for US defence contractors.
> - **CIS Benchmarks** are consensus configuration guides; Level 1 is practical and low-impact, Level 2 is stricter.
>   **CIS Controls v8** is a higher-level set of safeguards.
> - **DISA STIGs** are US Department of Defense configuration requirements.
> - **mSCP** maps these frameworks to individual macOS rules. Each rule has an ID (for example
>   `os_gatekeeper_enable`, `system_settings_firewall_enable`, `pwpolicy_minimum_length_enforce`), a check, an
>   expected result, often a fix and/or configuration-profile content, references to the frameworks, and
>   **organization-defined values (ODVs)** where the framework leaves the value to the organization.

> **Study note (enforcement vs auditing):** a configuration profile **enforces** a setting continuously, and the user
> usually cannot change it. A script **remediates** once and the setting can drift. Auditing (a scan) tells you
> the current state. Mature programs combine a profile for what profiles can manage, scripts for the rest, and
> regular scans to detect drift. Exemptions must be documented with a reason, which VibeMDM enforces.

> **Study note (built-in macOS protections often in baselines):** Gatekeeper (`spctl --status`), System Integrity
> Protection (`csrutil status`), the application firewall (`/usr/libexec/ApplicationFirewall/socketfilterfw
> --getglobalstate`), FileVault (`fdesetup status`), automatic security updates (XProtect and other background
> security data), screen-saver password and idle time, disabling remote login (SSH) and sharing services, and
> password policy (`pwpolicy`). See [Chapter 20](20-security-hardening.md#207-study-notes-macos-platform-security)
> for Gatekeeper, SIP and XProtect.

Next: [Chapter 13 — Identity and XCreds](13-identity-xcreds.md).
