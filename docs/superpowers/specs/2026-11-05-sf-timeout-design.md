# `sf` subprocess timeout — design

Status: approved in chat 2026-11-05 · Date: 2026-11-05

## Purpose

No `sf` call sf-tui makes has a deadline, so a hung `sf` (network stall, Node startup hang, an
unexpected interactive prompt) hangs the TUI's spinner forever and a CI job until the runner kills it.
Every non-streaming `sf` invocation gets a deadline, surfaced as a clear, distinct error.

## Non-goals

- Per-call budgets (one global deadline is enough to guarantee "never forever").
- Retrying after a timeout.
- `sf apex tail log`, which already runs under a context the Logs tab cancels.

## Behaviour

- Default deadline: **10 minutes** (deploys and full test runs legitimately take minutes; the goal is
  a bound, not a tight one).
- Resolution order: `--timeout <duration>` on `why`, `perms`, `compare` (Go duration syntax: `90s`,
  `2m`) → `SFTUI_TIMEOUT` environment variable (same syntax; applies to the TUI and to CI runners)
  → default. An unparsable value is a usage error for the flag (exit 2) and a startup warning plus
  the default for the env var.
- On deadline: the process is killed, and the error reads
  `<label>: timed out after <d> (set --timeout or SFTUI_TIMEOUT)`. It is a runtime failure: exit 1
  from the CLI, an `ErrMsg` in the TUI.
- Every other failure keeps today's semantics: callers still receive the raw `out` and an
  `*exec.ExitError` so Apex compile errors, test failures and deploy previews that come back on a
  non-zero exit are still parsed.

## Components

| Where | Responsibility |
|---|---|
| `sf/sfcmd.go` (new) | `type TimeoutError struct{ Label string; After time.Duration }`; `SetTimeout(d)`, `Timeout()`, `TimeoutFromEnv() (time.Duration, bool, error)`; `sfCommand(label string, args ...string) *sfRun` returning a thin wrapper with `Output()`, `Run()`, `SetDir(string)`; the wrapper runs under `exec.CommandContext` with the deadline and converts a deadline-exceeded result into `*TimeoutError` (other errors pass through unchanged) |
| `sf/soql.go` | `sfCommandError` returns a `*TimeoutError` unchanged (no re-prefixing) |
| 13 call sites in `sf/` | `exec.Command("sf", …)` → `sfCommand(label, …)`; `rest.go`'s `orgDisplay` and `defaultorg.go`'s `sfConfigGet` included |
| `cli.go`, `cli_why.go`, `cli_perms.go`, `cli_compare.go` | `--timeout` flag parsed in each command and applied via `sf.SetTimeout` before any `sf` call; env fallback applied in `runCLI` |
| `main.go` | Apply `TimeoutFromEnv` before the TUI starts; an invalid value becomes the startup status line warning |

## Testing

All offline, using a fake `sf` shell script placed first on a temp `PATH` (`#!/bin/sh` + `sleep 5`,
or `exit 0` / echo JSON variants):

- `sfCommand(...).Output()` with `SetTimeout(100ms)` against the sleeping fake returns a `*TimeoutError`
  naming the label and duration within well under a second.
- A non-timeout failure still yields an `*exec.ExitError` with `Stderr` populated.
- `TimeoutFromEnv`: unset → `(0, false, nil)`; `90s` → `(90s, true, nil)`; `banana` → error.
- Call-site coverage: `RunQuery`, `fetchSObjects`, `LoadLimits`, `ResolveUser` (via `runSOQLRaw`)
  each produce a `timed out` error under the fake.
- CLI: `--timeout 100ms` is applied (observed via `sf.Timeout()` in a stub), invalid value → exit 2;
  the env fallback.
- `sfCommandError` passes a `*TimeoutError` through unchanged.
