# `sftui limits` — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** `sftui limits [--warn N]` prints API limits as text/JSON and exits 3 when any limit is at or above N %.
**Spec:** `docs/superpowers/specs/2026-11-05-limits-cli-design.md`

## Global Constraints
No new deps; no null lists; stdout data only; exit 0/1/2/3; read-only; gofmt/vet/race clean.

## Review Focus
1. `--warn 100` with a limit at exactly 100 % → breached (≥). Test in Task 1.
2. `--warn` given but the org returns zero limits → `0 of 0 limits …`, exit 0. Test in Task 2.
3. `remaining > max` (Salesforce does report this for some limits) → `used` clamps to 0, `pct` 0. Test in Task 1.
4. `--warn 0` / `101` / `abc` → exit 2. Test in Task 2.
5. Sort is stable for equal `pct` (by name) so output is deterministic. Test in Task 1.

### Task 1: `sf` types, parser, builder, HumanInt (+ fixture)
Tests: `TestParseLimits`, `TestBuildLimitsReport_SortAndPct`, `TestBuildLimitsReport_Breached` (thresholds 100, 92, 93, 10, nil), `TestBuildLimitsReport_ClampsUsed`, `TestHumanInt` (moved from panes). `LoadLimits` wraps `ParseLimits`; `panes/limits.go` uses `sf.HumanInt`. Commit.

### Task 2: renderer + command
Tests: `TestRenderLimitsText_Golden` (warn 90), `_NoWarn` (no footer/marker), `_NoANSI`, `TestLimits_WarnValidation`, `TestLimits_ExitCodeOnBreach`, `TestLimits_NoWarnExit0`, `TestLimits_EmptyLimits`, `TestLimits_JSONSingleDoc`, `TestLimits_DefaultOrgFallback`, `TestRunCLI_LimitsDispatch`. Smoke. Commit.

### Task 3: docs + finish
README Scripting paragraph + cron example; wiki section with field reference and two jq examples; checks; review package; push; PR.
