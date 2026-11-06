# Security Policy

sf-tui works with live Salesforce orgs, so security reports are taken seriously.

## Supported versions

Fixes are made on `main` and released as a new version. Only the latest release is supported.

| Version | Supported |
|---|---|
| Latest release | Yes |
| Older releases | No; please upgrade |

## Reporting a vulnerability

**Please do not open a public issue for a security problem.**

Report it privately through GitHub: go to the repository's **Security** tab and choose
**Report a vulnerability** ([direct link](https://github.com/0xguy07/sf-tui/security/advisories/new)).

Please include:

- What you found and why it matters
- The sf-tui version (or commit) and your OS
- Steps to reproduce, ideally against a scratch org or developer org
- Any suggested fix

Do not include real org usernames, access tokens, or customer record data in a report.

## What to expect

This is a small project maintained by one person, so these are good-faith targets, not guarantees:

- Acknowledgement within about 7 days
- An initial assessment within about 14 days
- A fix or mitigation, and a coordinated disclosure with credit if you want it, as soon as is practical

## In scope

- **Write-gate bypass:** any way to save a record, run anonymous Apex, or deploy to a locked or protected org without the unlock and confirmation steps described in the [wiki](https://github.com/0xguy07/sf-tui/wiki/Safety-and-the-Write-Gate)
- **Misclassification** that makes a production org look like a sandbox or developer org
- **Credential exposure:** access tokens or instance URLs written to disk, logs, or the clipboard
- **Injection:** user-controlled input reaching a shell command or a SOQL string unescaped
- **Data at rest:** record data or field history written to the cache or config directory (these are meant never to be stored)
- **Unsafe file handling** in exports (`~/sf-tui-queries/`) or the on-disk cache

## Out of scope

- Vulnerabilities in the Salesforce platform or the `sf` CLI itself; report those to Salesforce
- Actions you take after deliberately unlocking a production org
- Issues that require an attacker to already have access to your user account or your unlocked terminal session
- Findings from automated scanners without a demonstrated impact

## How sf-tui handles credentials

sf-tui has no login flow and stores no credentials. For direct API calls it asks the `sf` CLI for the org's access token (`sf org display --json`) and keeps it in memory only. Your org logins remain managed by `sf`.
