# sf-tui

A terminal UI for Salesforce developers. Write SOQL with live autocomplete, run anonymous Apex, tail debug logs, and run tests with coverage — all keyboard-driven, all orgs, no `--json | jq` gymnastics.

It's a daily driver for CLI-comfortable admins and architects too: a permission explorer that answers "what can this user actually see — and which perm set grants it?", a live API-limits dashboard, a fuzzy object/field browser, and field-level schema compare between two orgs.

Runs on top of the official `sf` CLI and inherits your existing auths for free — if you live in `sf` but want something richer than a flat command line, this is for you. Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Requirements

- `sf` CLI on your PATH (`sf --version`)
- At least one org authenticated (`sf org login web`)

## Install

### macOS / Linux (Homebrew)

```
brew install 0xguy07/tap/sftui
```

### Windows (Scoop)

```
scoop bucket add 0xguy07 https://github.com/0xguy07/scoop-bucket
scoop install sftui
```

### Manual

Grab the right archive for your platform from the [latest release](https://github.com/0xguy07/sf-tui/releases/latest), extract `sftui` (or `sftui.exe` on Windows), and put it somewhere on your `PATH`.

### From source

Requires Go 1.24+.

```
go install github.com/0xguy07/sf-tui@latest
```

This installs a binary named `sf-tui` into `$(go env GOPATH)/bin` — make sure that directory is on your `PATH`. (The packaged Homebrew/Scoop/release builds name it `sftui`; rename or symlink if you want them to match.)

## Run

```
sftui
```

## Tabs

| Key      | Tab              | What it does |
|----------|------------------|--------------|
| `alt+1`  | **Query**        | Write SOQL, run against the selected org, scroll results |
| `alt+2`  | **Objects**      | Browse every SObject in the org and inspect fields/types/picklists. `ctrl+r` refreshes the schema cache |
| `alt+3`  | **Logs**         | Live-tail Apex debug logs |
| `alt+4`  | **Apex**         | Anonymous Apex scratchpad — write code, run with `ctrl+r`, see the debug log inline |
| `alt+5`  | **Limits**       | API limits with horizontal usage bars, sorted by % used. `ctrl+r` refreshes |
| `alt+6`  | **Permissions**  | Pick a user, see effective object permissions and which perm sets / profile grant them |
| `alt+7`  | **Tests**        | Apex test runner — multi-select classes with `space`, run with `ctrl+r`, see pass/fail + coverage with uncovered line numbers |
| `alt+8`  | **Meta**         | Deploy preview for the current project — see what would deploy/delete/conflict, dry-run with `ctrl+r`, real deploy with `ctrl+d` (asks first) |
| `alt+9`  | **Compare**      | Schema diff between two orgs — pick org A, `ctrl+r`, pick org B, `ctrl+r` again. `enter` on a row → field-level diff. `c` to clear |

## Keybindings

| Key                 | Action |
|---------------------|--------|
| `tab` / `shift+tab` | Switch pane within the current tab |
| `alt+1` … `alt+9`   | Switch tab |
| `ctrl+k`            | Command palette — fuzzy-pick any action |
| `ctrl+o`            | Open in org — selected record (Query), object in Setup (Objects), or Setup home |
| `ctrl+t`            | Toggle Tooling API for queries (Query tab) |
| `ctrl+i`            | Toggle Apex log inspector (Logs tab) |
| `space`             | Toggle test class selection (Tests tab) |
| `↑ ↓ / j k`         | Navigate list / table |
| `← → / h l`         | Page through list / table |
| `g / home`          | Jump to top (lists, tables, viewports) |
| `G / end`           | Jump to bottom (lists, tables, viewports) |
| `?`                 | Help overlay — every keybinding by section |
| `/`                 | Filter current list |
| `ctrl+r`            | Run query (Query) / execute Apex (Apex) / refresh schema cache (Objects) / run tests (Tests) / dry-run (Meta) / load side (Compare) |
| `ctrl+d`            | Real, non-dry-run deploy (Meta tab) — confirms with `y/n` |
| `ctrl+space`        | Force-trigger autocomplete |
| `tab` / `enter`     | Accept current autocomplete suggestion (when popup is open) |
| `esc`               | Close autocomplete / overlay |
| `ctrl+s`            | Save current query (Query tab) / save record (editor) |
| `ctrl+e`            | Export current query to `~/sf-tui-queries/*.soql` |
| `ctrl+y`            | Copy results as TSV to clipboard |
| `ctrl+x`            | Write results as CSV to `~/sf-tui-queries/*.csv` |
| `enter` (results)   | Open record editor for the selected row |
| `ctrl+p`            | Open saved queries / history picker |
| `ctrl+l`            | Start/stop log tail (Logs tab) |
| `ctrl+c` / `q`      | Quit |

## Why sf-tui?

- **SOQL autocomplete** — type `SELECT Id FROM Acc…` and get live suggestions. After `FROM Account`, field names autocomplete in `SELECT`/`WHERE`/`ORDER BY`. Dotted relationship walks (`Account.Owner.Email`) work too. Built from a describe cache keyed by org that **persists to disk**, so launches and lookups stay instant after the first fetch — `ctrl+r` on the Objects tab refreshes when schema changes.
- **Object Explorer** — fuzzy-search every SObject in your org, see fields, types, picklist values, required flags, and relationships without leaving the keyboard.
- **SOQL that remembers** — per-org query history and named saved queries, fuzzy-picked with `ctrl+p`. Export any query to a `.soql` file with `ctrl+e`.
- **Inline record editor** — press `enter` on any query result to edit its fields and `ctrl+s` to write back to the org, without leaving the table.
- **Live log tail** — watch Apex logs stream in real time.
- **Apex scratchpad** — write anonymous Apex in a real editor pane, hit `ctrl+r`, get the compile/runtime status and full debug log in the output pane. No more `sf apex run --file /tmp/foo.apex` round-trips.
- **Limits dashboard** — every API limit in the org as a colored usage bar (green / yellow / red), sorted by % used so the things to worry about float to the top.
- **Command palette (`ctrl+k`)** — fuzzy-pick any action: switch tab, run, open in org, save query, copy as TSV, …
- **Open in org (`ctrl+o`)** — one keystroke jumps the browser to the selected record (Query tab) or the sobject in Setup → Object Manager (Objects tab).
- **Permission Explorer** — type a user, see every object permission they have (R/C/E/D/View All/Modify All) and *which* perm set or profile grants each. The question every admin gets every week, answered without leaving the keyboard.
- **Apex log inspector (`ctrl+i`)** — flips the Logs tab from raw text to a structured, indented tree of code units / methods / SOQL / DEBUG events with timing, exceptions highlighted.
- **Apex test runner** — list test classes, multi-select with `space`, run with `ctrl+r`. See pass/fail and per-class line coverage with red/green bars. Coverage below 75% turns red so the bad apple is impossible to miss.
- **Tooling API support (`ctrl+t`)** — flip the Query tab into Tooling API mode to query `ApexClass`, `FlowDefinition`, `CustomField`, and friends. Header shows a yellow `TOOLING` badge so you don't run a regular query against the wrong API.
- **Org Compare with field-level diff** — sobject-level diff first, then `enter` on any row drills into a field-by-field comparison: only-in-A, only-in-B, and changed (with the specific attribute that moved — type, length, required, formula, picklist values).
- **Real deploy from the Meta tab (`ctrl+d`)** — once you've reviewed the preview, `ctrl+d` runs a real deploy. A red `REAL DEPLOY to <org> — y/n` confirmation makes sure you can't fire it by accident.
- **Multi-org native** — switch orgs with one keypress; each tab scopes to the selected org.
- **Single binary** — ~6 MB, no Electron, no browser, no JVM. Works over SSH.

## What's built

- [x] SOQL autocomplete using the describe cache
- [x] Record editor — edit rows from a query result and save back
- [x] Export results (TSV to clipboard, CSV to file)
- [x] Anonymous Apex scratchpad (`ctrl+r` to execute)
- [x] Command palette (`ctrl+k`)
- [x] Open in org (`ctrl+o`)
- [x] Limits dashboard
- [x] Permission Explorer (user → effective object perms + sources)
- [x] Apex log inspector (structured tree view)
- [x] Apex test runner with coverage
- [x] Tooling API support
- [x] Field-level permissions in Permission Explorer (`f` in Perms tab)
- [x] Uncovered-line drill-down in test runner
- [x] Metadata deploy preview + dry-run
- [x] Org compare (sobject-level schema diff between two orgs)
- [x] Field-level diff per sobject in Org Compare
- [x] Real (non-dry-run) deploy from the Meta tab

## Configuration

Saved queries, history, and the on-disk schema cache live at:

```
~/Library/Application Support/sf-tui/        (macOS)
~/.config/sf-tui/                            (Linux)
%AppData%\sf-tui\                            (Windows)
```

The schema cache (per-org sobject lists and describes) lives under `cache/` there and is served for 24 hours before a refetch; `ctrl+r` on the Objects tab forces a refresh. Safe to delete at any time — it just rebuilds on next use.

Exported queries (`ctrl+e`) and CSVs (`ctrl+x`) are written to `~/sf-tui-queries/`.

## License

MIT
