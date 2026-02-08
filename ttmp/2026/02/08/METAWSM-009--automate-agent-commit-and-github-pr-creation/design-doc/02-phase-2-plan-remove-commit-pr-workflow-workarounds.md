---
Title: 'Phase 2 plan: remove commit/PR workflow workarounds'
Ticket: METAWSM-009
Status: active
Topics:
    - cli
    - core
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: Commit/PR command UX, preflight diagnostics, and actor defaults
    - Path: internal/orchestrator/service.go
      Note: Branch prep semantics, commit orchestration, and PR execution ordering
    - Path: internal/store/sqlite.go
      Note: SQLite busy-timeout and retry behavior for concurrent command safety
    - Path: internal/orchestrator/service_test.go
      Note: Regression coverage for dirty-tree rebase flow and lock contention
    - Path: ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/tasks.md
      Note: Phase 2 implementation backlog for native workaround removal
ExternalSources: []
Summary: "Phase 2 hardening plan to remove manual branch prep, lock-handling, and attribution workarounds observed in real commit/PR execution."
LastUpdated: 2026-02-08T13:07:37.584643-08:00
WhatFor: "Define the native behavior needed so operators do not need manual git stash/rebase or retry workarounds to complete commit/PR flows."
WhenToUse: "Use when implementing or reviewing robustness improvements for metawsm commit/pr workflows after V1 rollout."
---

# Phase 2 plan: remove commit/PR workflow workarounds

## Executive Summary

Reopen METAWSM-009 for a hardening phase that makes commit/PR workflows work natively in the scenarios where we had to apply manual operator workarounds.

The primary gaps observed in real use were:
1. `metawsm commit` failing when workspace diffs were based on an older branch tip and checkout-to-base would overwrite local changes.
2. SQLite `database is locked` errors when commands touched run state concurrently.
3. Missing actor attribution fallback (`actor=unknown`) even when local auth context existed.
4. Insufficient preflight diagnostics to explain exactly how to resolve these edge cases.

## Problem Statement

V1 successfully added commit and PR automation, but the execution model still assumes a happy path:
1. Branch prep assumes we can always reset to `origin/<base>` before committing.
2. Store writes assume single-command access without lock contention.
3. Actor identity assumes explicit input rather than deterministic default resolution.

In practice, operators can reach a state where implementation diffs are valid but command flow fails and requires manual intervention (stash/rebase/retry).

## Proposed Solution

Implement Phase 2 robustness improvements in four workstreams.

### 1) Native branch-prep rebasing in `commit`

Add a deterministic "promote workspace diff onto target base" flow:
1. Snapshot dirty tree (tracked + untracked) to an internal patch/stash representation.
2. Move branch tip to configured base (`origin/<base>`).
3. Reapply snapshot.
4. Stop with a structured conflict report if reapply fails.
5. Continue with staged commit once conflict-free.

This replaces manual stash/reset/pop workarounds.

### 2) Store contention hardening

Add native lock handling so transient parallel usage does not fail the command:
1. Configure SQLite busy timeout at connection init.
2. Add bounded retry/backoff around write-heavy metadata updates.
3. Return typed "operation in progress" errors for overlapping commit/pr mutations on the same run.

### 3) Deterministic actor attribution

When `--actor` is omitted:
1. Resolve actor from `gh auth status` account first.
2. Fallback to git identity.
3. Persist the resolved actor in commit/PR metadata and status output.

### 4) Preflight diagnostics and remediation

Add `commit`/`pr` preflight checks that explain:
1. base divergence and planned rebase behavior,
2. lock contention status,
3. auth/actor resolution source,
4. exact remediation when automatic handling is not possible.

## Design Decisions

1. Automatic branch-diff promotion should be default behavior.
Reason: this is the most common operator pain and should not require manual git choreography.

2. Lock contention should be retried automatically with bounded limits.
Reason: transient contention is expected in CLI usage and should not look like data corruption.

3. Actor metadata should be best-effort auto-populated.
Reason: audit fields lose value when they frequently show `unknown`.

4. Preserve existing explicit flags (`--actor`, `--dry-run`) and keep outputs deterministic.
Reason: compatibility and operator trust.

## Alternatives Considered

1. Keep current behavior and document manual stash/rebase playbook.
Rejected: still requires operator-level git recovery for routine flows.

2. Forbid commit when branch tip is stale relative to base.
Rejected: blocks useful work and pushes complexity to users.

3. Add global process mutex only, no SQLite retry behavior.
Rejected: does not protect against cross-process contention or race windows.

## Implementation Plan

### Phase 2A: Branch prep robustness
1. Introduce a reusable workspace snapshot/apply helper for dirty trees.
2. Replace hard reset flow in commit with snapshot-reset-reapply semantics.
3. Add tests for stale-base dirty-tree commit success and conflict reporting.

### Phase 2B: Locking and retries
1. Add SQLite busy timeout configuration.
2. Add bounded retry wrapper around commit/pr metadata write paths.
3. Add tests that intentionally create concurrent writers and verify graceful retry/failure.

### Phase 2C: Actor defaults and diagnostics
1. Implement actor resolution chain (`flag -> gh account -> git identity`).
2. Persist resolved actor in events and stored commit/PR records.
3. Extend dry-run/preflight output to include base-drift and lock/actor diagnostics.

### Phase 2D: Operator playbook and rollout
1. Update playbook with "native handling" behavior and conflict fallback.
2. Add e2e scenario covering stale-base workspace to commit+PR without manual git intervention.

## Open Questions

1. Should automatic branch-diff promotion be opt-out (flag) or always-on?
2. Should concurrent run mutations queue or fail-fast with retry hints after timeout?
3. What retry budget is acceptable before we surface lock contention to operators?

## References

- `ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/design-doc/01-plan-teach-agents-to-commit-and-open-github-prs.md`
- `ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/playbook/01-operator-and-agent-commit-pr-workflow.md`
- `ttmp/2026/02/08/METAWSM-006--build-a-go-backend-plus-web-frontend-for-metawsm/reference/01-bootstrap-brief-run-20260208-073222.md`
