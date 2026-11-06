# Non-interactive `sftui perms` and the permissions refactor — design

Status: draft for review · Date: 2026-11-05 · Depends on: `sftui why` (dispatcher, `cliDeps`, exit-code conventions)

## Purpose

Answer "what can this user actually do, and which permission set or profile grants it?" from a script,
CI job, or agent, with per-permission precision. Along the way, move the permissions logic out of
Bubble Tea command closures into pure, tested functions so the TUI's Perms tab and the CLI share one
implementation, and so the TUI gains the same precision.

Success looks like:

- `sftui perms <user> --org <org>` prints, for every object, each of Read/Create/Edit/Delete/
  View All/Modify All with the sources that grant it.
- `--object Account` adds field-level Read/Edit with sources.
- A user with **Modify All Data** or **View All Data** is reported as such, because those override
  object permissions. The TUI did not know this before.
- The Perms tab shows per-permission sources and the overrides line, using the same code.
- Everything is testable offline from JSON fixtures; the command is read-only by construction.

## Non-goals

- Sharing rules, role hierarchy, record ownership, or any record-level access. The report covers
  object and field permissions only, and the text output says so in a footer.
- Muting permission sets inside permission set groups (their effect is already reflected in the
  `ObjectPermissions` rows Salesforce materialises; we do not explain the muting).
- Listing users (`sftui users`), filtering to granted-only, CSV output.
- Session settings, login hours, IP ranges, and other profile-level restrictions.

## Interface

```
sftui perms <user> [--org, -o <alias|username>] [--object <SObject>] [--format text|json]
```

`<user>` is resolved in this order:

| Looks like | Rule |
|---|---|
| 15 or 18 chars starting with `005` | User Id, exact |
| contains `@` | `Username`, exact, case-insensitive |
| anything else | `Name`, exact, case-insensitive; must match exactly one user |

Inactive users are included. Zero matches → exit 2, `perms: no user matches "<ident>"`. More than one
name match → exit 2 with each candidate on its own stderr line as `<Name>  <Username>  <Id>` and the
hint `pass the username or Id`.

Org resolution, `--format` handling, stdout/stderr separation, and exit codes are identical to
`sftui why`: 0 success including partial results, 1 runtime failure, 2 usage. The `<user>` argument may
appear before or after flags; `--` ends flags; extra positionals exit 2.

## Data model

New types in `sf/`, all JSON-tagged. Source labels are the permission set label, or
`Profile: <name>` for the profile-owned permission set.

```
PermissionsReport
  user         UserBrief                     id, name, username, isActive, profile
  assignments  []PermSetAssignment           permSetId, label, isProfile, profileName, groupLabel
  overrides    Overrides                     modifyAllData []source, viewAllData []source
  objects      []ObjectPerm                  sorted by sobject
  fields       []FieldPerm                   only when --object was given; sorted by field
  object       string                        the --object value, "" otherwise
  errors       []SectionError                sections that failed; the rest of the report stands

ObjectPerm (existing type, extended; old fields keep their names and JSON tags)
  sobject, read, create, edit, delete, viewAll, modifyAll   bool   (unchanged)
  sources  []string                                                 (unchanged: any grant)
  grants   map[string][]string   keys: read, create, edit, delete, viewAll, modifyAll → sources

FieldPerm (existing type, extended)
  field, read, edit  (unchanged)   sources []string (unchanged)
  grants map[string][]string       keys: read, edit → sources

PermSetAssignment (existing type, extended)
  groupLabel string   the PermissionSetGroup.MasterLabel when assigned through a group
```

`grants` is the per-permission answer. `sources` stays so `panes/permissions.go` and any JSON consumer
of the TUI's message types keep working. `Overrides` comes from `PermissionSet.PermissionsModifyAllData`
and `PermissionsViewAllData` on each assigned permission set (profile-owned ones included).

## Queries

All through the existing `sf data query --json` path (`runSOQLRaw`), not the REST client, to match
the TUI. Five queries, each its own fixture:

1. **User:** `SELECT Id, Name, Username, IsActive, Profile.Name FROM User WHERE <Id = | Username = | Name => '<escaped>'`
2. **Assignments:** `SELECT PermissionSetId, PermissionSet.Label, PermissionSet.IsOwnedByProfile, PermissionSet.Profile.Name, PermissionSet.PermissionsModifyAllData, PermissionSet.PermissionsViewAllData, PermissionSetGroupId, PermissionSetGroup.MasterLabel FROM PermissionSetAssignment WHERE AssigneeId = '<id>'`
3. **Object perms:** unchanged from today (`ObjectPermissions WHERE ParentId IN (...)`).
4. **Field perms** (only with `--object`): unchanged from today (`FieldPermissions WHERE SobjectType = '<obj>' AND ParentId IN (...)`).

If `PermissionSetGroup.MasterLabel` is not queryable in an org (older API), the assignments query is
retried without the two group columns and `groupLabel` stays empty; this is a section note, not an error.

## Components

| Where | Responsibility |
|---|---|
| `sf/permissions.go` | Keeps the types and the two existing `tea.Cmd` constructors (`LoadPermissions`, `LoadFieldPermissions`), which become thin wrappers around the builders below. `LoadUsers` unchanged. |
| `sf/permissions_build.go` (new) | Pure builders, no I/O: `buildAssignments(records) ([]PermSetAssignment, Overrides)`, `buildObjectPerms(records, labels) []ObjectPerm`, `buildFieldPerms(records, labels) []FieldPerm`, plus `BuildPermissionsReport(q soqlRunner, org string, user UserBrief, object string) PermissionsReport` which runs queries 2–4 and assembles the report, recording a `SectionError` for any query that fails instead of aborting. |
| `sf/permissions_user.go` (new) | `ResolveUser(q soqlRunner, org, ident string) (UserBrief, error)` with the rules above; `ErrAmbiguousUser{Candidates []UserBrief}` as a typed error. |
| `sf/soql.go` (new, tiny) | `type soqlRunner interface { SOQL(org, soql string, tooling bool) ([]byte, error) }` and `sfRunner{}` implementing it over `runSOQLRaw`. Tests implement it with a map from a query substring to a fixture file, the same routing idea as `fakeAPI` in `why_test.go`. |
| `cli_perms.go` (new, `package main`) | Flag parsing (shares `splitPositional`), org resolution, calls `d.resolveUser` then `d.perms`, renders, maps errors to exit codes. |
| `perms_text.go` (new) | `renderPermissionsText(PermissionsReport) string`, plain text, no ANSI. |
| `cli.go` | Adds `perms` to the dispatcher and `usage()`; `cliDeps` gains `resolveUser func(org, ident string) (sf.UserBrief, error)` and `perms func(org string, user sf.UserBrief, object string) (sf.PermissionsReport, error)`. |
| `panes/permissions.go` | `renderPermDetail` shows per-permission sources and an overrides line (see below). No layout change elsewhere. |

The TUI path: `LoadPermissions` calls `BuildPermissionsReport(sfRunner{}, org, user, "")` and returns the
existing `PermissionsLoadedMsg`, now carrying the report's `Assignments`, `Objects`, and a new
`Overrides` field. `LoadFieldPermissions` calls the same with `object` set and returns
`FieldPermissionsLoadedMsg` from `report.Fields`. Behaviour the TUI had is preserved; it gains data.

## Output

**JSON:** the `PermissionsReport`, indented, one document. Unstable until 1.0.

**Text** (exact copy where fixed):
```
<Name>  (<Username>)  [inactive]           ← "[inactive]" only when !isActive
Profile: <profile>

Assignments (<n>):
  <label>[  via group <groupLabel>]
  ...

System overrides:                           ← section omitted when neither list is non-empty
  Modify All Data: <sources, comma-joined>  ← implies edit/delete/view on every object
  View All Data: <sources>

Object permissions (<n> objects):
  <sobject padded to 30>  R[<src>] C[<src>] E[<src>] D[<src>] VA[<src>] MA[<src>]
  ...                                       ← <src> = sources joined by "+", or "-" when not granted

Fields on <object> (<n>):                   ← only with --object
  <field padded to 30>  R[<src>] E[<src>]
  (no FieldPermissions rows; fields follow profile defaults)   ← when n == 0

Errors:                                     ← omitted when none
  <section>: <message>

Object and field permissions only. Sharing rules, roles and record ownership are not evaluated.
```

**TUI Perms tab:** the object row's `sources` column becomes the per-permission form
`R:Profile: Sales  C,E:Sales Ops` (permission letters grouped by identical source set, dim), and a
`System overrides` section appears above the object table when either list is non-empty. The FLS
section gets the same treatment.

## Error handling

| Condition | Behaviour |
|---|---|
| User not found / ambiguous | exit 2, message on stderr, stdout empty |
| `sf` missing, auth failure, user query fails | exit 1 |
| Assignments query fails | exit 1 (nothing useful can be reported without it) |
| Object perms or field perms query fails | report the rest, add a `SectionError`, warn on stderr, exit 0 |
| No assignments at all | valid report with empty sections; text says `Assignments (0):` |

## Testing

All offline. New fixtures in `testdata/perms/`: `user_by_id.json`, `user_by_username.json`,
`users_name_ambiguous.json`, `users_none.json`, `assignments.json` (a profile-owned set, two
standalone sets, one set assigned via a group, one set with `PermissionsModifyAllData = true`),
`objectperms.json` (Account granted R by the profile and C/E by a set, Case granted by two sets, an
object with a row but no grants), `fieldperms.json`, `fieldperms_empty.json`.

- Builder tests: per-permission `grants` map correctness, dedupe of a source that grants several
  permissions, `sources` backward-compatible value, overrides collected from the right sets,
  group label carried.
- `ResolveUser`: Id, username (case-insensitive), unique name, ambiguous name (typed error lists
  candidates), none, SOQL escaping of a name containing `'`.
- `BuildPermissionsReport`: full report from fixtures; object-perms query failure yields a partial
  report with one `SectionError`.
- `renderPermissionsText`: golden for the full fixture; no-overrides case omits the section; no ANSI.
- `cli_perms`: argument order, trimming, `--object`, ambiguous user → exit 2 listing candidates,
  JSON stdout is one document even with warnings, exit codes per table, read-only grep.
- Existing TUI tests (none for this pane today) — a `renderPermDetail` golden is added so the pane
  change is pinned.

## Documentation

- README: add `perms` to the Scripting section and the "What's built" list.
- Wiki `Scripting-and-JSON`: `perms` synopsis, user resolution rules, `PermissionsReport` field
  reference, and two `jq` examples (objects a user can delete; every source granting Edit on Account).

## Open decisions (settled for this draft)

- Per-permission `grants` is additive beside `sources`; nothing is removed from the TUI's types.
- Overrides are reported, not applied: `Account.edit` stays `[-]` for a Modify-All-Data user whose
  sets grant nothing on Account; the overrides section is where the reader learns they can edit anyway.
  Applying them would hide which sets grant what, which is the question the tool exists to answer.
