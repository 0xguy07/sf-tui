# `sf` subprocess timeout — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every non-streaming `sf` call runs under a deadline (default 10m, `--timeout` / `SFTUI_TIMEOUT`) and reports a distinct `TimeoutError`.

**Architecture:** `sf/sfcmd.go` wraps `exec.CommandContext` in `sfCommand(label, args...)`; call sites swap `exec.Command("sf", …)` for it and keep their existing output/error handling. CLI commands parse `--timeout`; `runCLI`/`main` apply the env fallback.

**Spec:** `docs/superpowers/specs/2026-11-05-sf-timeout-design.md`

## Global Constraints
- No new dependencies. Non-timeout error semantics unchanged (raw `out` + `*exec.ExitError`). `gofmt`/`vet`/`test -race` clean.

## Review Focus
1. A call site that sets `cmd.Dir` (`metadata.go`): the wrapper must support it. Test in Task 2 (compile) and a `SetDir` unit test in Task 1.
2. The TUI with `SFTUI_TIMEOUT=banana`: starts with a warning, not a crash. Test in Task 3.
3. `--timeout 0` or negative: rejected as usage (exit 2). Test in Task 3.
4. A timeout during `BuildSchemaDiff`'s worker pool: the error is attributed to the object and the rest proceeds. Covered by existing section-error handling; test in Task 2.
5. `sfCommandError` on a `*TimeoutError`: no double prefix. Test in Task 1.

### Task 1: `sf/sfcmd.go` + tests (fake `sf` on PATH)
Produces: `TimeoutError`, `SetTimeout`, `Timeout`, `TimeoutFromEnv`, `sfCommand`, `(*sfRun).Output/Run/SetDir`; `sfCommandError` passthrough. Tests: `TestSfCommand_Timeout`, `TestSfCommand_ExitErrorPreserved`, `TestSfCommand_SetDir`, `TestTimeoutFromEnv`, `TestSFCommandError_TimeoutPassthrough`. Commit.

### Task 2: Swap the 13 call sites
Each site: `sfCommand(<label>, args...)`; labels are the `sf` subcommand (`sf org list`, `sf apex run`, …). Tests: `TestRunQuery_Timeout`, `TestFetchSObjects_Timeout`, `TestLoadLimits_Timeout`, `TestResolveUser_Timeout` under the sleeping fake with `SetTimeout(100ms)`; `TestBuildSchemaDiff_DescribeTimeoutIsSectionError`. Commit.

### Task 3: CLI `--timeout` and env fallback; TUI startup
`--timeout` on why/perms/compare (shared `applyTimeout(flagValue string, stderr) int`), `runCLI` applies env before dispatch, `main.go` applies env and sets a startup warning on parse error. Tests: `TestTimeoutFlag_Applied`, `TestTimeoutFlag_Invalid` (exit 2, incl. `0` and `-5s`), `TestTimeoutEnv_Applied`, `TestTimeoutEnv_InvalidWarns`. README: one paragraph under Scripting; usage() lines. Commit.

### Task 4: Finish
Checks, review package, push, PR.
