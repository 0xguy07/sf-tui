# Non-interactive `sftui why` — design

Status: draft for review · Date: 2026-11-05

## Purpose

Make sf-tui's signature feature, "why did this record change?", usable outside the TUI: in scripts,
CI jobs, `jq` pipelines and AI agents. It also serves as a clean, repeatable demo of the feature.

Success looks like:

- `sftui why <recordId> --org <org>` prints the same investigation the Why tab shows, with no TUI.
- `--format json` output is machine-readable and pipes cleanly into `jq`.
- The command is read-only by construction and cannot reach the write gate.
- It works with no org and no network in tests, via the existing `testdata/why` fixtures.

## Non-goals (first version)

- `perms`, `compare`, `limits` subcommands (a later slice that reuses this dispatcher).
- `--watch`, CSV output, colors, pagination.
- Any write or mutating command.
- A versioned JSON envelope. Output is the existing `Trace` structure, documented as unstable until 1.0.

## Interface

```
sftui                                   # starts the TUI (unchanged)
sftui --version | -v | version          # prints version (only as the first argument)
sftui why <recordId> [flags]

Flags:
  --org, -o <alias|username>   org to query; default: the sf default target org
  --format <text|json>         output format; default: text
  --refresh                    bypass the 24-hour automation cache (like ctrl+r in the TUI)
```

Org resolution order: `--org`, else `sf config get target-org --json`. If neither yields an org, exit 2
with a message telling the user to pass `--org` or run `sf config set target-org=<alias>`.

The record Id is validated with the existing `sf.ValidRecordID` before any network call.

### Output

**stdout carries data only; stderr carries diagnostics.** `sftui why … | jq` must always work.

- `--format json`: the existing `sf.Trace` value, indented, one document.
  Fields: `record`, `object`, `trackedFields`, `saves`, `historyTruncated`, `automation`, `errors`.
- `--format text`: plain text, no ANSI. Sections in order:
  1. Header: record Id, object label and API name.
  2. Saves, newest first: timestamp, who (name, user type badge `AUTO`/`INTEG`/`GUEST` where applicable),
     then each change as `Field: old → new`. A note if history was truncated.
  3. Automation in Salesforce order of execution, one line per item with its type and active state.
  4. For each changed field, the automation that could have written it.
  5. A trailing list of section errors, if any.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Success, including partial results. Failed sections appear in `errors` (JSON) and as a one-line warning on stderr |
| 1 | Runtime failure: record not resolvable, auth failure, `sf` not found |
| 2 | Usage error: bad flags, missing or invalid Id, no org resolvable |

## Components

| File | Responsibility |
|---|---|
| `main.go` | Calls `runCLI(os.Args[1:], os.Stdout, os.Stderr)` first; starts the TUI only if it reports "not handled" |
| `cli.go` | Dispatcher: `runCLI(args, stdout, stderr, deps) (handled bool, code int)`. Knows the subcommand table. Fixes `--version` matching to first argument only |
| `cli_why.go` | Parses `why` flags (stdlib `flag`), resolves the org, validates the Id, calls `sf.BuildTrace`, selects the renderer, maps errors to exit codes |
| `why_text.go` | Pure `renderTraceText(sf.Trace) string`. Independent of the TUI's lipgloss renderer in `panes/why.go` |

`deps` is a small struct holding a factory returning anything with `Query(soql, tooling)` and
`GetJSON(path, v)` (the method set `sf.BuildTrace` already requires), plus the default-org lookup, so
tests inject fakes. Production wiring uses `sf.RESTFor` and `sf config get`. `whyAPI` is unexported but its
methods are exported, so a fake in `package main` satisfies it with no change to `sf/`.

## Data flow

```
args → dispatch → parse flags → resolve org → validate Id
     → BuildTrace(api, org, id, refresh) → render (json | text) → stdout
                                         ↘ warnings / errors → stderr, exit code
```

## Error handling

- Unresolvable record: exit 1, message on stderr, nothing on stdout.
- Partial failure (for example automation loads but history does not): render what exists, list failures in
  `errors`, warn on stderr, exit 0.
- `sf` missing or unauthenticated: exit 1 with the underlying message.
- Unknown subcommand or flag: usage on stderr, exit 2. `sftui` with unrecognised first argument does not
  start the TUI.

## Testing

All tests run offline against fakes and the fixtures in `testdata/why`.

- Flag parsing: defaults, `-o`, `--format`, invalid format, missing Id, invalid Id.
- Org resolution: explicit flag wins; fallback to default org; neither → exit 2.
- Golden files for `text` and `json` output of a known trace, including a truncated-history case and a
  partial-failure case.
- Exit codes for each row of the table above.
- stdout/stderr separation: JSON mode stdout parses as a single JSON document even when warnings occur.
- Dispatcher: no args reports "not handled" (TUI path); `--version` only as the first argument;
  `sftui why -v` is not treated as a version request.
- Cache isolation: `BuildTrace` reads and writes the on-disk automation cache, so tests point the config
  directory at a temp dir and never touch the user's real cache.
- Read-only guarantee: the command's dependencies expose no write capability and do not reference the gate.

## Documentation

- README: a short "Scripting" section with the command and one `jq` example.
- Wiki: a **Scripting and JSON** page with the `Trace` field reference and example one-liners (for example,
  list who changed `Status`, or list active automation touching a field).

## Open decisions (settled for this draft)

- JSON is the bare `Trace`, not a wrapper. Revisit before 1.0.
- Partial results exit 0. A `--strict` flag can be added later if scripts need it.
