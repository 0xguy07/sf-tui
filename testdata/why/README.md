# Why-tab fixtures

REST responses used by `sf/why_test.go`. No live org is contacted in tests.

Provenance:

| Source | Files |
|--------|-------|
| **Captured** from a Salesforce sandbox (API v67.0), then sanitized. Ids, names, usernames, and all record values were replaced. The response shapes are unchanged. | `entitydefinition_*.json`, `describe_casehistory.json`, `tracked_case.json`, `users.json`, `triggers_account.json` (shape), `flows_account.json` (shape, including the inactive managed-flow row) |
| **Synthesized** from the documented Tooling / Metadata API shapes. **Not yet verified against a live org** (Step 0 open item). | `flow_*.json`, `pb_*.json`, `validationrules_account.json`, `workflowrule*_*.json`, `workflowfieldupdate*_*.json` |
| Synthesized to exercise the Account acceptance scenario | `history_account.json`, `tracked_account.json`, `record_account.json`, `globaldescribe.json`, remaining `describe_*.json` |

Replace a synthesized file with a sanitized capture when you verify it against a dev org.
