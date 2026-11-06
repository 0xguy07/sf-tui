# `sftui compare` and the schema-diff refactor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `sftui compare <orgA> <orgB> [--object X | --fields [--custom-only]] [--exit-code] [--refresh]` prints an object-level and optional field-level schema diff as text or JSON, exits 3 on drift when asked, and the Compare tab renders from the same diff code.

**Architecture:** Pure diff functions in `sf/compare_build.go` operate on `SObjectSummary` and `Describe` values. A `SchemaLoader` interface is the only I/O seam; `fetchSObjects`/`fetchDescribe` are split out of the existing `tea.Cmd` closures so the TUI and the loader share them. `BuildSchemaDiff` runs describes through a bounded worker pool and sorts results. The CLI reuses the dispatcher, `writeReport`, `prefixWriter` and a generalised positional splitter.

**Tech Stack:** Go 1.24, stdlib (`sync`, `sort`).

**Spec:** `docs/superpowers/specs/2026-11-05-compare-cli-design.md`

## Global Constraints

- No new module dependencies.
- stdout data only; stderr diagnostics; `warning: <section>: <message>` lines unprefixed.
- Exit codes `0` success incl. partial, `1` runtime, `2` usage, `3` differences with `--exit-code`.
- Read-only: `cliDeps` gains only `compare`; no new file references `gate`.
- No `null` lists in JSON: every slice in `SchemaDiff`/`ObjectFieldDiff` is initialised.
- Diff behaviour of the TUI is unchanged (same attributes compared, same buckets); only `(cached)` markers are added.
- `sf` failures surface through `sfCommandError`.
- `gofmt -l .` empty; `go vet ./...`, `go test ./...` pass; tests never touch the real config dir.

## Review Focus

1. **Same org given twice** (`sftui compare prod prod`): exit 2 with a clear message, no `sf` calls. Test in Task 5.
2. **An object present on one side only with `--object`**: diff lists every field as only-that-side and records a `describe-<side>:<X>` error for the missing side, exit 0. Test in Task 3.
3. **`--fields` when the two orgs share no objects**: `Fields: 0 objects compared`, no describes issued, exit 0. Test in Task 3.
4. **Describe results arriving out of order from the worker pool**: `fields` sorted by sobject every time. Test in Task 3 (fake loader with random sleeps).
5. **`--exit-code` with differences only in `errors`** (a describe failed, nothing else differs): exit 0, because a failure is not drift. Test in Task 5.

---

### Task 1: Split fetchers out of the `tea.Cmd`s; `SchemaLoader`

**Files:**
- Modify: `sf/describe.go:75-170` (`loadSObjects`, `loadDescribe`)
- Modify: `sf/compare.go` (add `SchemaLoader`, `DefaultSchemaLoader`; `SchemaLoadedMsg`/`FieldDiffLoadedMsg` gain `Cached` fields)
- Create: `sf/schemaloader_test.go`

**Interfaces:**
- Produces:
  ```go
  func fetchSObjects(org string, force bool) ([]SObjectSummary, bool /*cached*/, time.Time, error)
  func fetchDescribe(org, sobject string, force bool) (Describe, bool, time.Time, error)
  type SchemaLoader interface {
      SObjects(org string, force bool) ([]SObjectSummary, bool, error)
      Describe(org, sobject string, force bool) (Describe, bool, error)
  }
  func DefaultSchemaLoader() SchemaLoader           // sfSchemaLoader{} over the fetchers
  // SchemaLoadedMsg gains Cached bool; FieldDiffLoadedMsg gains ACached, BCached bool
  ```
  The `exec.Command` calls inside the fetchers use `sfCommandError(out, err)` on failure. `loadSObjects`/`loadDescribe` become one-line wrappers mapping the fetcher result to the existing messages (`Cached`, `CachedAt` preserved). `LoadOrgSchema`/`LoadFieldDiff` pass the cached flags through.

- [ ] **Step 1: Failing tests** — the fetchers shell out, so test only the pure wrapper plumbing and the error path:
  ```go
  func TestFetchSObjects_SfMissing(t *testing.T)   // t.Setenv("PATH", t.TempDir()); HOME/XDG_CONFIG_HOME → temp dir; fetchSObjects("x", true) returns err containing "executable file not found"
  func TestFetchDescribe_SfMissing(t *testing.T)   // same for fetchDescribe("x","Account",true)
  func TestDefaultSchemaLoader_Implements(t *testing.T) // var _ SchemaLoader = DefaultSchemaLoader()
  ```
- [ ] **Step 2: Run, confirm fail** (`undefined: fetchSObjects`).
- [ ] **Step 3: Implement.** Keep the two-format `sf sobject list` parsing exactly as it is today.
- [ ] **Step 4: `go build ./... && go test ./...`** — the TUI compiles and existing tests pass.
- [ ] **Step 5: Commit** — `git commit -m "Split fetchSObjects/fetchDescribe out of the tea.Cmds; add SchemaLoader"`

---

### Task 2: Pure diff functions and types

**Files:**
- Create: `sf/compare_build.go`, `sf/compare_build_test.go`
- Create: `testdata/compare/sobjects_a.json`, `sobjects_b.json`, `describe_account_a.json`, `describe_account_b.json`, `describe_widget_a.json`, `describe_widget_b.json`
- Modify: `panes/compare.go` (delete `fieldTypeLabel`, `fieldAttrDiff`, `sameRefs`, `samePicklist`; call the `sf` versions)
- Modify: `panes/compare_test.go` → does not exist; the existing `panes/helpers_test.go` tests `sameRefs`/`samePicklist`/`diffGlyph` — move the first two to `sf/compare_build_test.go` and keep `diffGlyph`'s.

**Interfaces:**
- Produces:
  ```go
  const (StateOnlyA = "only-a"; StateOnlyB = "only-b"; StateBoth = "both")
  type Side struct{ Org string `json:"org"`; SObjects int `json:"sobjects"`; Cached bool `json:"cached"` }
  type ObjectState struct{ Name string `json:"name"`; Custom bool `json:"custom"`; State string `json:"state"` }
  type FieldBrief struct{ Name string `json:"name"`; Type string `json:"type"` }
  type FieldChange struct{ Name string `json:"name"`; Attrs []string `json:"attrs"` }
  type ObjectFieldDiff struct{ SObject string `json:"sobject"`; OnlyA, OnlyB []FieldBrief `json:"onlyA"/"onlyB"`; Changed []FieldChange `json:"changed"`; Same int `json:"same"`; Different bool `json:"different"` }
  type Summary struct{ OnlyA, OnlyB, Both, ObjectsCompared, ObjectsChanged int }   // json: onlyA, onlyB, both, objectsCompared, objectsChanged
  type SchemaDiff struct{ A, B Side; Objects []ObjectState; Fields []ObjectFieldDiff; Summary Summary; Different bool; Errors []SectionError }  // json: a, b, objects, fields, summary, different, errors
  func DiffSObjects(a, b []SObjectSummary) ([]ObjectState, Summary)   // Summary has OnlyA/OnlyB/Both filled
  func DiffFields(sobject string, a, b *Describe) ObjectFieldDiff      // nil side → every field of the other side is only-that-side
  func FieldTypeLabel(f Field) string                                  // moved verbatim
  func FieldAttrDiff(a, b Field) []string                              // moved verbatim
  func IsCustomName(name string) bool                                  // suffix table from the spec
  ```

- [ ] **Step 1: Fixtures.** `sobjects_*.json` in the wrapped `sobjectListResult` shape (`{"status":0,"result":{"sobjects":[{name,label,custom,queryable,keyPrefix}…]}}`): A = Account, Contact, Legacy__c, Widget__c; B = Account, Contact, NewThing__c, Widget__c (custom flag true on `__c` names). `describe_*.json` in `describeResult` shape with `fields`: Account A/B identical (Id, Name string(255) nillable=false, Industry picklist with 2 active values); Widget A: Id, Name string(80), Old__c string(40), Status__c picklist [New, Open], Owner reference→[User]; Widget B: Id, Name string(255), New__c double, Status__c picklist [New, Open, Closed], Owner reference→[User].

- [ ] **Step 2: Failing tests**
  ```go
  func TestDiffSObjects(t *testing.T)          // states: Account both, Contact both, Legacy__c only-a (custom), NewThing__c only-b (custom), Widget__c both; sorted; Summary{OnlyA:1, OnlyB:1, Both:3}
  func TestDiffFields_Identical(t *testing.T)  // Account → Different=false, Same==3, empty non-nil slices
  func TestDiffFields_Widget(t *testing.T)     // OnlyA==[{Old__c,"string(40) [custom]"}], OnlyB==[{New__c,"double [custom]"}], Changed names == [Name, Status__c]; Name attrs == ["length: 80 → 255"]; Status__c attrs == ["picklist values differ"]; Same==2 (Id, Owner); Different
  func TestDiffFields_OneSideNil(t *testing.T) // b=nil → OnlyA has all of A's fields, OnlyB empty, Different
  func TestIsCustomName(t *testing.T)          // Widget__c, X__mdt, E__e, Z__x true; Account, Foo__History false
  func TestSameRefsAndPicklist(t *testing.T)   // moved cases
  ```
  (`FieldTypeLabel` tags order is `required,custom,read-only` as today; Widget fields have `Updateable: true`, `Nillable: true`, `Custom: true`.)

- [ ] **Step 3: Run, confirm fail.** - [ ] **Step 4: Implement**; update `panes/compare.go` to call `sf.FieldTypeLabel`/`sf.FieldAttrDiff` and delete the moved helpers; move the two helper tests. - [ ] **Step 5: `go test ./...` green; `go build ./...`.**

- [ ] **Step 6: Commit** — `git commit -m "Add pure schema diff functions; move field diff helpers to sf"`

---

### Task 3: `BuildSchemaDiff` with the worker pool

**Files:**
- Modify: `sf/compare_build.go`; Create: `sf/compare_report_test.go`

**Interfaces:**
- Produces:
  ```go
  type CompareOptions struct{ Object string; Fields, CustomOnly, Refresh bool; Workers int }
  func BuildSchemaDiff(l SchemaLoader, aOrg, bOrg string, opts CompareOptions) (SchemaDiff, error)
  ```
  Algorithm: load A and B sobject lists concurrently (two goroutines); either error → `return diff, fmt.Errorf("sobjects-%s: %w", side, err)`. Fill `A`/`B` sides, `Objects`, `Summary`. Target set: `opts.Object` → `[opts.Object]`; `opts.Fields` → every `StateBoth` name, filtered by `IsCustomName(name) || summary.Custom` when `CustomOnly`. Pool: `Workers` (≤0 → 4) goroutines pull names from a channel; each calls `l.Describe` for A then B (skipping a side where the object is absent per `Objects`, recording `describe-<side>:<X>` with message `not present in <org>`), appends to a mutex-guarded slice. After `wg.Wait()`, sort `Fields` by `SObject` and `Errors` by `Section`. `Different` = any only-a/only-b or any `Fields[i].Different`. `Summary.ObjectsCompared = len(Fields)`, `ObjectsChanged` = count of Different.

- [ ] **Step 1: Failing tests** with `fakeSchemaLoader{t; sobjects map[org]file; describes map[org+"/"+sobject]file; errs map[string]error; sleepMax time.Duration; maxConcurrent *int32 (atomic)}`:
  ```go
  func TestBuildSchemaDiff_ObjectsOnly(t *testing.T)       // no describes issued; Summary{1,1,3,0,0}; Different true; Fields empty non-nil
  func TestBuildSchemaDiff_Object(t *testing.T)            // Object "Widget__c" → one ObjectFieldDiff, ObjectsCompared 1, ObjectsChanged 1
  func TestBuildSchemaDiff_ObjectMissingOnB(t *testing.T)  // Object "Legacy__c": A described, B skipped; Errors == [{"describe-b:Legacy__c","not present in borg"}]; Fields[0].OnlyA has all fields; err nil
  func TestBuildSchemaDiff_FieldsDescribesBothSet(t *testing.T) // Fields → describes exactly Account, Contact, Widget__c on both orgs (fake records calls)
  func TestBuildSchemaDiff_CustomOnly(t *testing.T)        // only Widget__c described
  func TestBuildSchemaDiff_NoCommonObjects(t *testing.T)   // sobjects_b replaced by a list with no overlap → ObjectsCompared 0, no describe calls
  func TestBuildSchemaDiff_DescribeErrorOneSide(t *testing.T) // errs["borg/Contact"]=boom → Errors has describe-b:Contact; Fields has Account and Widget__c; err nil
  func TestBuildSchemaDiff_SortedUnderConcurrency(t *testing.T) // sleepMax=20ms, Workers 4, run 5 times: Fields names always [Account Contact Widget__c]
  func TestBuildSchemaDiff_WorkersBound(t *testing.T)      // Workers 2, sleepMax 10ms → maxConcurrent ≤ 2
  func TestBuildSchemaDiff_RefreshForces(t *testing.T)     // Refresh → fake saw force=true on every call
  func TestBuildSchemaDiff_SObjectsFail(t *testing.T)      // errs["aorg"]=boom → err non-nil containing "sobjects-a"
  ```
- [ ] **Step 2: Run, confirm fail.** - [ ] **Step 3: Implement.** - [ ] **Step 4: `go test -race ./sf` green** (race detector on, because of the pool).
- [ ] **Step 5: Commit** — `git commit -m "Add BuildSchemaDiff with a bounded describe worker pool"`

---

### Task 4: Rewire the Compare pane

**Files:**
- Modify: `panes/compare.go` (`refresh`, `renderFieldDiff`, `SetSchema` gains a `cached bool`, `SetFieldDiff` reads `ACached/BCached`)
- Modify: `main.go:769-782` (pass `msg.Cached` into `SetSchema`)
- Create: `panes/compare_test.go`

**Interfaces:** `func (c *Compare) SetSchema(side, org string, objs []sf.SObjectSummary, cached bool)`; `refresh()` builds its items from `sf.DiffSObjects`; `renderFieldDiff(msg sf.FieldDiffLoadedMsg, aOrg, bOrg string)` renders `sf.DiffFields(msg.SObject, msg.A, msg.B)`; both print `(cached)` after an org name when that side was cached.

- [ ] **Step 1: Failing tests**
  ```go
  func TestCompareRefresh_Counts(t *testing.T)      // SetSchema A/B from fixtures → Viewport content contains "only in A:  1", "only in B:  1", "in both:    3"; list has 5 items
  func TestCompareRefresh_CachedMarker(t *testing.T) // cached=true on A → content contains "(cached)"
  func TestRenderFieldDiff_Widget(t *testing.T)     // contains "- Old__c", "+ New__c", "~ Name", "length: 80 → 255"
  ```
- [ ] **Step 2: Run, confirm fail.** - [ ] **Step 3: Implement**; remove the pane's own diff loop. - [ ] **Step 4: `go test ./...`; `go build ./...`; `timeout 4 go run . </dev/null` exits without panic.**
- [ ] **Step 5: Commit** — `git commit -m "Compare tab: render from shared diff functions; show cached sides"`

---

### Task 5: Text renderer and the `compare` command

**Files:**
- Create: `compare_text.go`, `compare_text_test.go`, `cli_compare.go`, `cli_compare_test.go`
- Create: `testdata/compare/diff_full.json` (captured from `BuildSchemaDiff` with `Fields: true` via a throwaway `DUMP_COMPARE=1` test, then deleted), `testdata/compare/compare_full.txt` (golden)
- Modify: `cli.go` (`cliDeps.compare`, `prodDeps`, `usage`, dispatcher), `cli_why.go` (`splitPositional` → `splitPositionalN`)

**Interfaces:**
- Produces:
  ```go
  func renderSchemaDiffText(d sf.SchemaDiff) string
  func runCompare(args []string, stdout, stderr io.Writer, d cliDeps) int
  func splitPositionalN(args []string, n int) (positionals []string, rest []string)   // first n non-flag args; extras after "--" in rest
  func splitPositional(args []string) (string, []string)                           // = first of splitPositionalN(args, 1)
  // cliDeps gains: compare func(aOrg, bOrg string, opts sf.CompareOptions) (sf.SchemaDiff, error)
  ```
  Flags: `--object`, `--fields`, `--custom-only`, `--exit-code`, `--refresh`, `--format`. `takesValue` gains nothing new (`--object` already there). Validation order: parse → NArg>0 → exactly two orgs else `compare: need two orgs` (2) → same org (2) → `--object`+`--fields` (2) → `--custom-only` without `--fields` (2) → format (2) → `d.compare` error (1) → `writeReport` → if `opts.ExitCode && diff.Different` return 3.

- [ ] **Step 1: Capture `diff_full.json`** with the Task 3 fake loader, `CompareOptions{Fields: true}`, orgs `aorg`/`borg`, `A.Cached=true` set by the fake.
- [ ] **Step 2: Failing tests**
  ```go
  // compare_text_test.go
  func TestRenderSchemaDiffText_Golden, _NoDifferences (A==B lists, Fields empty → "Objects: 0 only in A, 0 only in B, 3 in both"), _NoANSI, _ErrorsSection
  // cli_compare_test.go  (stub compare returns the fixture; records opts)
  func TestCompare_TwoOrgsRequired            // one org → 2, "need two orgs"
  func TestCompare_SameOrgRejected            // prod prod → 2
  func TestCompare_OrgsBeforeOrAfterFlags     // {"a","b","--fields"} and {"--fields","a","b"} both call compare("a","b",{Fields:true})
  func TestCompare_ObjectAndFieldsExclusive   // → 2
  func TestCompare_CustomOnlyNeedsFields      // → 2
  func TestCompare_ExitCodeOnDrift            // --exit-code with Different → 3; without → 0
  func TestCompare_ExitCodeErrorsOnlyIsZero   // Different=false, Errors non-empty, --exit-code → 0 (Review Focus 5)
  func TestCompare_RefreshAndWorkersDefault   // --refresh → opts.Refresh; opts.Workers == 4
  func TestCompare_RuntimeErrorExit1          // compare err → 1, stdout empty
  func TestCompare_JSONSingleDocWithWarnings, _TextMatchesRenderer, _JSONRoundTrips
  func TestSplitPositionalN                   // {"a","b","--x"} n=2 → [a b],[--x]; {"a","b","c"} n=2 → [a b],["--","c"]
  func TestRunCLI_CompareDispatch             // runCLI({"compare"}) handled, 2, usage mentions "sftui compare"
  ```
- [ ] **Step 3: Run, confirm fail.** - [ ] **Step 4: Implement; generate golden; inspect it against the spec's text block.** - [ ] **Step 5: `gofmt -l .; go vet ./...; go test -race -count=1 ./...`.** - [ ] **Step 6: Smoke:** `go run . compare` → 2; `go run . compare a b` → 1 with the `sf` message, empty stdout.
- [ ] **Step 7: Commit** — `git commit -m "Add non-interactive 'sftui compare' command"`

---

### Task 6: Documentation

- [ ] **Step 1: README** — Scripting section: `compare` synopsis, one paragraph on object vs field level and `--exit-code`, and a GitHub Actions snippet:
  ```yaml
  - run: sftui compare uat prod --fields --custom-only --exit-code
  ```
  "What's built": `sftui compare <orgA> <orgB>` and "Compare tab shows which sides came from cache".
- [ ] **Step 2: Wiki `Scripting-and-JSON.md`** — `## compare` section: synopsis, flag table, exit code 3, `SchemaDiff` field reference, two `jq` examples validated against `diff_full.json` (objects only in B: `jq -r '.objects[] | select(.state=="only-b") | .name'`; changed fields on Widget__c: `jq -r '.fields[] | select(.sobject=="Widget__c") | .changed[] | "\(.name): \(.attrs|join("; "))"'`). Update the intro ("Three commands…"), Home and Tab Guide lines. Link check. Push.
- [ ] **Step 3: Commit both repos.**

---

### Task 7: Finish the branch

- [ ] `gofmt -l .`, `go vet ./...`, `go test -race -count=1 ./...` clean; `grep -n "gate" cli_compare.go compare_text.go sf/compare_build.go` prints nothing.
- [ ] Push to `feat/compare-cli` (plain push; the branch already holds the spec), mark PR #13 ready, retitle `Add non-interactive 'sftui compare'; shared schema diff for the Compare tab`.
