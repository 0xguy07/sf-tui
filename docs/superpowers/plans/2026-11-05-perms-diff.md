# `perms --diff` — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** `sftui perms A --diff B` prints a per-permission, per-source diff of two users' effective permissions.
**Spec:** `docs/superpowers/specs/2026-11-05-perms-diff-design.md`

## Review Focus
1. A user with zero assignments on one side: every granted permission on the other side becomes an only-A/only-B row; no panic on empty slices. Test in Task 1.
2. Same user both sides (by different identifiers resolving to one Id): exit 2 after resolution. Test in Task 2.
3. `--object` with `--diff`: both reports fetch fields; a field present in one report only → rows. Test in Task 1.
4. Errors from either report are kept and prefixed so the reader knows which side failed. Test in Task 1.
5. Row order is deterministic (name, then perm order). Test in Task 1.

### Task 1: `DiffPermissions` + fixture (`report_bob.json`, hand-authored from `report_account.json`)
### Task 2: `--diff` flag, second resolve, renderer + golden, CLI tests
### Task 3: README line, wiki section, checks, PR
