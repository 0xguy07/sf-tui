# Non-interactive `sftui limits` — design

Status: approved in chat 2026-11-05 · Depends on: the dispatcher, `writeReport`, `--timeout`

## Purpose

Make the Limits tab's answer, "which API limits are close to the ceiling?", usable from cron and CI:
print every limit with its usage, and optionally fail when any limit is at or above a threshold.

## Interface

```
sftui limits [--org <alias|username>] [--warn <percent>] [--format text|json] [--timeout <d>]
```

- `--warn N` (integer 1–100): exit **3** when any limit's `pct` ≥ N. Without `--warn`, exit 0 on success.
- Org resolution, stdout/stderr rules, exit codes 0/1/2 as the other commands. `--warn` outside 1–100
  or non-integer → exit 2.

## Data model (`sf/`)

```
LimitsReport
  org      string
  limits   []LimitUsage   {name, max, remaining, used, pct}   sorted by pct desc, then name
  warn     *int           the threshold when --warn was given, else absent
  breached []string       names with pct ≥ warn; [] otherwise (never null)
```

`pct` is `used / max × 100` rounded to one decimal; `max ≤ 0` → `pct 0` (as the TUI's `Limit.Pct`).

Pure: `ParseLimits(out []byte) ([]Limit, error)` split out of `LoadLimits` (which keeps calling it),
`BuildLimitsReport(limits []Limit, org string, warn *int) LimitsReport`.

## Output

Text (exact copy where fixed):
```
API limits for <org>  (<n> limits)[  warn at <W>%]

  <name padded 40>  <used> / <max>   <pct>%[  ⚠]
  ...

<k> of <n> limits at or above <W>%            ← only with --warn
```
`used`/`max` use the TUI's `humanInt` formatting (10.0K, 1.5M), moved to `sf` as `HumanInt`.

JSON: the `LimitsReport`, one document.

## Components

| Where | Responsibility |
|---|---|
| `sf/limits.go` | `LimitUsage`, `LimitsReport`, `ParseLimits`, `BuildLimitsReport`, `HumanInt`; `LoadLimits` becomes a wrapper |
| `panes/limits.go` | `humanInt` → `sf.HumanInt` |
| `cli_limits.go`, `limits_text.go` | flags, `--warn` validation, exit 3, text renderer |
| `cli.go` | `cliDeps.limits func(org string) ([]sf.Limit, error)`; dispatcher + usage |

## Testing

Fixture `testdata/limits/limits.json` in `sf limits api display --json` shape with five limits:
one at 100 % (max 100, remaining 0), one at 92.5 %, one at 10 %, one with `max 0`, one with
`remaining > max` (used clamps to 0). Tests: parse, sort order, pct rounding and clamping, breached
set at several thresholds, text golden (with and without `--warn`), `--warn` validation matrix,
exit 3 only when breached and `--warn` given, JSON round-trip, read-only grep, `LoadLimits` still
returns the same message.
