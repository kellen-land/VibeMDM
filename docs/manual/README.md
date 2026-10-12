# VibeMDM Administrator Manual

This manual covers how to install, run and troubleshoot a self-hosted VibeMDM server. VibeMDM is a Go server that
embeds the NanoMDM protocol engine, ships with a macOS agent, and has a React web console. The manual also
teaches the Apple device-management concepts underneath each feature, so you can use VibeMDM as a lab while you
prepare for the Apple certifications:

- **Apple Certified Support Professional (ACSP)**, exam *Apple Device Support*
- **Apple Certified IT Professional (ACITP)**, exam *Apple Deployment and Management*

## How to read this manual

- **Procedures** are numbered steps that use the real console names and API paths from the code.
- **Tables** list settings, fields, endpoints, checks and flags.
- **Study notes** are blockquotes that start with **Study note:**. They explain the Apple concept behind a feature
  in exam terms. They are kept apart from the procedures, so you can skip them while operating the server or read
  only them while you revise.
- Where Apple's behaviour is not certain, or depends on the macOS version, the text says so. Check Apple's
  current documentation before you rely on a detail in production or in an exam.
- The web console is still being developed. When a page may change, the manual describes it in general terms
  (for example "Software Updates → super tab") and gives the API endpoint, which is stable.

## Table of contents

| # | Chapter | What it covers |
| --- | --- | --- |
| 1 | [Introduction](01-introduction.md) | What VibeMDM is, the architecture, how the MDM, agent and DDM channels work, and a glossary |
| 2 | [Installation](02-installation.md) | Docker Compose, TLS with Nginx Proxy Manager, every environment variable, first login, the encryption key, backups, upgrades |
| 3 | [APNs and certificates](03-apns-and-certificates.md) | Push certificate lifecycle, the SCEP CA, profile signing, one-time SCEP challenges |
| 4 | [Enrollment](04-enrollment.md) | User-approved enrollment, invitations, the agent installer, Apple Configurator, supervision, takeover protection |
| 5 | [Inventory and groups](05-inventory-and-groups.md) | Computers, inventory sources, extension attributes, smart and static groups, searches, network organization |
| 6 | [Scoping](06-scoping.md) | Targets, limitations and exclusions, with worked examples |
| 7 | [Policies, scripts and packages](07-policies-scripts-packages.md) | Triggers, frequencies, `$4`–`$11`, packages, Self Service, custom and local triggers |
| 8 | [Configuration profiles](08-configuration-profiles.md) | Profiles, payload editors for every Jamf Pro payload, payload variables, Application & Custom Settings (JSON schemas), MCX |
| 9 | [Mac Apps and Installomator](09-apps-installomator.md) | App catalog, labels, options, custom labels, argument quoting |
| 10 | [Software updates](10-software-updates.md) | DDM enforcement and settings plans, status, SOFA, super |
| 11 | [FileVault](11-filevault.md) | Escrow, Escrow Buddy remediation, audited key reveal, Secure Token |
| 12 | [Compliance](12-compliance.md) | mSCP baselines, tailoring, scans, remediation, reports, drift |
| 13 | [Identity and XCreds](13-identity-xcreds.md) | XCreds for Entra ID, Google and Okta; Platform SSO compared |
| 14 | [White-glove onboarding](14-white-glove-onboarding.md) | swiftDialog workflows, fields, naming templates, steps |
| 15 | [Health](15-health.md) | Misconfiguration detection, noise controls, every check, self-healing, webhooks |
| 16 | [MDM commands and the inspector](16-mdm-commands-and-inspector.md) | Commands, the inspector timeline, redaction, a wire-level walk-through |
| 17 | [Remote shell](17-remote-shell.md) | The browser root shell and how it is secured |
| 18 | [Logging and diagnostics](18-logging-and-diagnostics.md) | Server and agent logs, search, live tail, archives, diagnostic bundles |
| 19 | [Users, roles and the API](19-users-roles-api.md) | Roles, API tokens, the REST API, webhooks, the MCP server for AI agents |
| 20 | [Security hardening](20-security-hardening.md) | The security model and an operational checklist |
| 21 | [CLI reference](21-cli-reference.md) | Every command and flag of `vibemdm`, `vibemdm-agent`, `vibemdm-sim` and `mscp-import` |
| 22 | [Troubleshooting](22-troubleshooting.md) | Symptom → cause → how to confirm → fix tables for enrollment, APNs, profiles and DDM, the agent, compliance, updates, onboarding, MCP, logins, Health and the server |
| 23 | [Study guide](23-study-guide.md) | ACSP and ACITP topic areas mapped to chapters, thirteen labs with the simulator and a real Mac, quick facts, Apple's official references |

## Conventions

| Convention | Meaning |
| --- | --- |
| `mdm.example.com` | Your server's public host name |
| `/api/v1/...` | The admin REST API. Lists return `{"items": [...], "total": n}`; errors return `{"error": "..."}` |
| `vmdm_…` | An API token (console → Settings → API Tokens) |
| `vmda_…` | An agent device token (stored on the Mac in `agent.json`) |
| *Admin* / *Viewer* | Console roles. Viewers may read. Every non-GET request needs an admin |
| `sudo` | Agent commands on the Mac generally need root |

## Related documents

The feature notes in [`docs/`](../) are shorter and engineering-oriented. This manual expands on them; where the
two disagree, the manual follows the current code. The man pages are in [`docs/man/`](../man/).
