# `sftui perms <userA> --diff <userB>` — design

Status: approved in chat 2026-11-05 · Depends on: `perms`, `BuildPermissionsReport`

## Purpose

Answer "what can A do that B can't, and the other way round, and where does each come from?"
from the same org, for access reviews and "make B like A" tickets.

## Interface

```
sftui perms <userA> --diff <userB> [--org] [--object X] [--format text|json] [--timeout <d>]
```

Both users resolve with the existing rules (Id, username, exact name; ambiguity → exit 2). The same
user on both sides → exit 2. No exit-code gate: this is an investigation view. Exit 0/1/2 as `perms`.

## Data model (`sf/`)

```
PermissionsDiff
  a, b          UserBrief
  assignments   {onlyA []string, onlyB []string}          source labels
  overrides     []OverrideDiff   {name: modifyAllData|viewAllData, a []string, b []string}  only when they differ
  objects       []GrantDiff      one per (sobject, perm) where the sides differ in grant or in sources
  fields        []GrantDiff      same for (field, perm) when --object was given
  object        string           the --object value, "" otherwise
  different     bool
  errors        []SectionError   union of both reports' errors, prefixed "a:" / "b:"

GrantDiff
  name     string        sobject or field name
  perm     string        read|create|edit|delete|viewAll|modifyAll
  a, b     []string      sources on each side; [] when not granted
  sameGrant bool         both granted (sources differ) — "same access, different reason"
```

Rows are sorted by name then by the canonical permission order. An object present in only one
report (the other user has no row for it) yields rows for each granted permission with the other
side `[]`. Objects where both sides have identical grants produce no rows.

Pure: `DiffPermissions(a, b PermissionsReport) PermissionsDiff`.

## Output

Text (exact copy where fixed):
```
Permissions diff
  A: <nameA>  (<usernameA>)
  B: <nameB>  (<usernameB>)

Assignments only A:                        ← omitted when empty
  <label>
Assignments only B:
  <label>

System overrides:                          ← omitted when none differ
  Modify All Data   A[<src|->]  B[<src|->]

Object permissions (<n> differences):      ← "no differences" line when n == 0
  <sobject padded 30>  <perm padded 9>  A[<src|->]  B[<src|->][  (same grant, different source)]

Fields on <object> (<n> differences):      ← only with --object
  ...

Object and field permissions only. Sharing rules, roles and record ownership are not evaluated.
```
JSON: the `PermissionsDiff`, one document.

## Components

| Where | Responsibility |
|---|---|
| `sf/permissions_diff.go` | types, `DiffPermissions` |
| `cli_perms.go` | `--diff` flag; resolves B, builds both reports (same org/object), renders via `perms_diff_text.go` |
| `perms_diff_text.go` | `renderPermissionsDiffText` |

## Testing

Second fixture report `testdata/perms/report_bob.json`: Bob has the profile only plus "Reports
Viewer", no Modify All Data, Account read via the profile only, no Case rows, Industry read via the
profile only. Tests: assignments split; overrides diff (modifyAllData differs, viewAllData same →
one row); objects: Account create/edit only-A, Account read same grant same source → no row, Case
rows only-A; fields with --object; identical users → `different:false`, no rows; sort order; text
golden; CLI: `--diff` same user → 2, ambiguous B → 2 listing candidates, JSON round-trip, text
matches renderer, read-only grep.
