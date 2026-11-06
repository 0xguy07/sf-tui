# Non-interactive `sftui compare` and the schema-diff refactor — design

Status: draft for review · Date: 2026-11-05 · Depends on: the `why`/`perms` dispatcher, `cliDeps`, `resolveOrg`, `writeReport`, `prefixWriter`

## Purpose

Answer "did these two orgs drift?" from a script or CI job: which objects exist only on one side,
and for chosen objects, which fields were added, removed or changed. Move the diff logic out of the
Compare pane's render functions into pure, tested functions the TUI and the CLI share.

Success looks like:

- `sftui compare uat prod` prints the object-level diff in a second or two (one `sf sobject list` per org).
- `sftui compare uat prod --object Account` prints the field-level diff for one object.
- `sftui compare uat prod --fields --custom-only --exit-code` is a usable CI step: it describes every
  custom object on both sides through a worker pool and exits 3 when anything differs.
- The Compare tab renders from the same diff structs and shows when a side came from cache.
- Everything is testable offline from fixture describes; the command is read-only by construction.

## Non-goals

- Comparing anything other than sobjects and their fields (validation rules, flows, permission sets,
  layouts, record types). The text output says so in a footer.
- Three-way compares, baseline files, or writing a diff to apply.
- Picklist-value-level detail beyond "picklist values differ" (the TUI's current depth).

## Interface

```
sftui compare <orgA> <orgB> [--object <SObject> | --fields [--custom-only]]
                            [--exit-code] [--refresh] [--format text|json]
```

| Flag | Meaning |
|---|---|
| `--object X` | Field-level diff for one object. Mutually exclusive with `--fields` (both → exit 2) |
| `--fields` | Field-level diff for every object present on both sides |
| `--custom-only` | With `--fields`: only objects whose `Custom` flag is set or whose name ends in `__c`, `__mdt`, `__e`, `__x`, `__b`, `__xo`, `__kav`. Without `--fields` → exit 2 |
| `--exit-code` | Exit 3 when `different` is true |
| `--refresh` | Bypass the 24-hour sobject-list and describe caches on both sides |
| `--format` | `text` (default) or `json` |

Both orgs are positional and required; there is no default-org fallback because a compare needs two.
`A` and `B` are the orgs in the order given; "only in A" means present in the first org only.
Extra positionals exit 2; `--` ends flags.

Exit codes: `0` success (including partial results), `1` runtime failure (either org's sobject list
cannot be loaded, `sf` missing), `2` usage, `3` differences found and `--exit-code` was given.
stdout carries data only; stderr carries diagnostics and `warning: <section>: <message>` lines.

## Data model

New types in `sf/`, all JSON-tagged, no `null` lists.

```
SchemaDiff
  a, b        Side                  {org, sobjects (count), cached (bool)}
  objects     []ObjectState         {name, custom, state}   state ∈ only-a | only-b | both; sorted by name
  fields      []ObjectFieldDiff     one per object compared at field level; sorted by sobject; [] otherwise
  summary     Summary               {onlyA, onlyB, both, objectsCompared, objectsChanged}
  different   bool                  any only-a/only-b object, or any ObjectFieldDiff with a difference
  errors      []SectionError        {section, message}; section is "sobjects-a", "sobjects-b", "describe-a:<X>", "describe-b:<X>"

ObjectFieldDiff
  sobject     string
  onlyA       []FieldBrief          {name, type}   type = the TUI's fieldTypeLabel text
  onlyB       []FieldBrief
  changed     []FieldChange         {name, attrs []string}   attrs = FieldAttrDiff lines, e.g. "length: 80 → 255"
  same        int
  different   bool
```

`fieldTypeLabel`, `fieldAttrDiff`, `sameRefs`, `samePicklist` move from `panes/compare.go` to `sf/`
unchanged in behavior, exported as `FieldTypeLabel` and `FieldAttrDiff`.

## Loading

```
type SchemaLoader interface {
    SObjects(org string, force bool) ([]SObjectSummary, bool /*cached*/, error)
    Describe(org, sobject string, force bool) (Describe, bool /*cached*/, error)
}
```

The existing `loadSObjects` and `loadDescribe` closures are split: plain functions
`fetchSObjects(org, force)` and `fetchDescribe(org, sobject, force)` do the cache check, the `sf`
call and the cache write; the `tea.Cmd` constructors and `DefaultSchemaLoader()` both call them.
`sf` failures go through `sfCommandError` so the real message (stdout JSON) surfaces.

`BuildSchemaDiff(l SchemaLoader, aOrg, bOrg string, opts CompareOptions) (SchemaDiff, error)`:

1. Load both sobject lists (in parallel). Either failing → return the error (exit 1); nothing useful
   exists without them.
2. `DiffSObjects` → `objects`, `summary` counts, `different` if any only-a/only-b.
3. Field level, when `opts.Object != ""` (that one object, even if only on one side — the missing side
   is reported as a `describe-<side>:<X>` error and the diff lists every field as only-the-other-side)
   or `opts.Fields` (every `both` object, filtered by `opts.CustomOnly`): describe each object on both
   sides through a worker pool of `opts.Workers` (default 4) goroutines. A describe failing on one side
   records a section error and skips that object's diff; the rest continues. Results are sorted by
   sobject before return so output is deterministic regardless of completion order.
4. `objectsChanged` counts `ObjectFieldDiff.different`; `different` ORs them in.

`CompareOptions{Object string; Fields, CustomOnly, Refresh bool; Workers int}`.

## Components

| Where | Responsibility |
|---|---|
| `sf/compare_build.go` (new) | `DiffSObjects`, `DiffFields`, `FieldTypeLabel`, `FieldAttrDiff`, `IsCustomName`, `BuildSchemaDiff`, worker pool |
| `sf/compare.go` | Keeps `SchemaLoadedMsg`, `FieldDiffLoadedMsg`, `LoadOrgSchema`, `LoadFieldDiff`; adds `SchemaLoader`, `DefaultSchemaLoader()`; message types gain `Cached` flags |
| `sf/describe.go` | `fetchSObjects` / `fetchDescribe` extracted; `tea.Cmd`s wrap them |
| `panes/compare.go` | `refresh()` renders from `sf.DiffSObjects`; `renderFieldDiff` renders from `sf.DiffFields`; shows `(cached)` next to a side loaded from cache. Diff helpers removed from the pane |
| `cli_compare.go` (new) | Flags, two positional orgs, `--object`/`--fields` exclusivity, `--custom-only` requires `--fields`, calls `d.compare`, exit 3 logic |
| `compare_text.go` (new) | `renderSchemaDiffText(SchemaDiff) string`, plain text |
| `cli.go` | `compare` in the dispatcher and `usage()`; `cliDeps` gains `compare func(aOrg, bOrg string, opts sf.CompareOptions) (sf.SchemaDiff, error)`; `splitPositionals` variant that returns the first two positionals (the existing `splitPositional` returns one; add `splitPositionalN(args, n)` and make the old one call it) |

## Output

**JSON:** the `SchemaDiff`, indented, one document. Unstable until 1.0.

**Text** (exact copy where fixed):
```
Schema compare
  A: <orgA>  (<n> sobjects)[  (cached)]
  B: <orgB>  (<n> sobjects)[  (cached)]

Objects: <onlyA> only in A, <onlyB> only in B, <both> in both

Only in A:                                   ← section omitted when empty
  - <name>[  (custom)]
Only in B:
  + <name>[  (custom)]

Fields: <objectsCompared> objects compared, <objectsChanged> with differences    ← only when field level ran

<sobject>                                    ← one block per ObjectFieldDiff that is different
  - <field>  <type>                          ← only in A
  + <field>  <type>                          ← only in B
  ~ <field>
      <attr line>
  = <same> unchanged

<sobject>: no field-level differences (<same> fields)   ← for --object when identical; omitted under --fields

Errors:                                      ← omitted when none
  <section>: <message>

Objects and fields only. Validation rules, flows, permission sets and layouts are not compared.
```

**TUI:** unchanged layout; the summary header and field-diff header gain `(cached)` after a side's
org name when that side was served from cache.

## Error handling

| Condition | Behaviour |
|---|---|
| Fewer than two orgs, same org twice, `--object` with `--fields`, `--custom-only` without `--fields` | exit 2 |
| Either sobject list fails | exit 1, message names the side |
| A describe fails on one side | `errors` entry, warning on stderr, object skipped, exit 0 (or 3 if other differences and `--exit-code`) |
| No differences | exit 0, text says `Objects: 0 only in A, 0 only in B, N in both` and, at field level, `… 0 with differences` |

## Testing

All offline. Fixtures in `testdata/compare/`: `sobjects_a.json`, `sobjects_b.json` (A has `Legacy__c`
only; B has `NewThing__c` only; `Account`, `Contact`, `Widget__c` in both), `describe_account_a.json`
and `_b.json` (identical), `describe_widget_a.json` and `_b.json` (A has `Old__c`; B has `New__c`;
`Name` length 80 → 255; `Status__c` picklist differs; `Owner` identical).

- `DiffSObjects`: states and sort order; custom flag from either side.
- `DiffFields`: identical → `different=false`, `same` count; Widget → one onlyA, one onlyB, two changed
  with the exact attr lines; `IsCustomName` suffix table.
- `BuildSchemaDiff` with a fake loader: object-level only; `--object` on an object missing from B;
  `--fields` describes exactly the `both` set; `--custom-only` describes only `Widget__c`; a describe
  error on one side → section error and the other objects still diffed; `force` propagates from
  `Refresh`; results sorted regardless of goroutine order (fake loader sleeps randomly); `Workers`
  bound respected (fake counts concurrent calls).
- `renderSchemaDiffText`: golden for the full fixture; identical-orgs case; no ANSI.
- `cli_compare`: positional parsing (two orgs, `--` handling, extra positional), exclusivity errors,
  exit 3 only with `--exit-code`, exit 0 with differences otherwise, JSON single document with warnings,
  text matches renderer, read-only grep.
- `panes`: `Contains` tests that the summary and field views still render the counts and show `(cached)`.

## Documentation

- README: `compare` in the Scripting section with a CI snippet (`--fields --custom-only --exit-code`
  in a GitHub Actions step) and in "What's built".
- Wiki `Scripting-and-JSON`: `compare` section with the `SchemaDiff` field reference and two `jq`
  examples (objects only in prod; fields changed on `Widget__c`).

## Open decisions (settled for this draft)

- Exit 3 is opt-in via `--exit-code`; default exit stays 0 with differences, matching `git diff`.
- Worker pool default 4. `sf` is a Node process; 4 keeps a laptop responsive and is ~4× the TUI's speed.
- `--object` on an object present on one side only is allowed and reported (the TUI allows it too).
