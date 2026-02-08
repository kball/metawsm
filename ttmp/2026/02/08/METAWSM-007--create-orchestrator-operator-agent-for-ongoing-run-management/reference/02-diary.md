---
Title: Diary
Ticket: METAWSM-007
Status: active
Topics:
    - core
    - cli
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: examples/policy.example.json
      Note: Documented operator policy block and Codex assist defaults (commit ca5ed93)
    - Path: internal/model/types.go
      Note: Added OperatorRunState model persisted by store methods (commit 1aa8086)
    - Path: internal/policy/policy.go
      Note: Added operator policy schema/defaults/validation (commit ca5ed93)
    - Path: internal/policy/policy_test.go
      Note: Added policy tests for operator defaults and invalid settings (commit ca5ed93)
    - Path: internal/store/sqlite.go
      Note: Added operator_run_states table and CRUD helpers (commit 1aa8086)
    - Path: internal/store/sqlite_test.go
      Note: Added persistence test across store reopen (commit 1aa8086)
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-08T09:01:08.218858-08:00
WhatFor: Implementation diary for METAWSM-007 hybrid deterministic+LLM operator work.
WhenToUse: Use to review what changed, why, failures encountered, and how to validate each commit.
---



# Diary

## Goal

Capture the implementation journey for METAWSM-007, step by step, with exact commands, failures, validation results, and reviewer guidance.

## Step 1: Add Operator Policy Defaults and Validation

This step established the policy contract for the new operator loop before touching runtime logic. I added an explicit `operator` policy block with thresholds and LLM settings, then validated defaults so the operator can start in `assist` mode with Codex CLI.

This unlocked predictable configuration behavior for later phases (store-backed budgets, operator command loop, and LLM adapter), and gave us concrete schema-level guardrails early.

### Prompt Context

**User prompt (verbatim):** "ok implement. Keep a diary along the way. Commit as you go"

**Assistant interpretation:** Implement the ticket plan end-to-end, document progress in a structured diary, and create incremental commits instead of one large batch.

**Inferred user intent:** Execute the operator feature with strong traceability and recoverability so progress can be reviewed and resumed safely at any point.

**Commit (code):** `ca5ed93` — "Add operator policy defaults and validation"

### What I did
- Added `operator` config schema in `internal/policy/policy.go`:
- thresholds: `unhealthy_confirmations`, `restart_budget`, `restart_cooldown_seconds`, `stale_run_age_seconds`
- LLM block: `mode`, `command`, `model`, `timeout_seconds`, `max_tokens`
- Set defaults for V1:
- `mode=assist`
- `command=codex`
- conservative restart/stale thresholds
- Added policy validation rules for all new fields in `Validate`.
- Added tests in `internal/policy/policy_test.go`:
- default values assert (`codex`, `assist`)
- invalid LLM mode rejection
- missing LLM command rejection
- invalid restart budget rejection
- Updated `examples/policy.example.json` with operator config block.
- Ran formatting and focused tests.

### Why
- Operator behavior must be deterministic and configurable before runtime/autonomous behavior is introduced.
- Defaulting to Codex CLI + assist mode matches agreed V1 scope and lowers rollout risk.

### What worked
- Policy schema/defaults compiled cleanly.
- New validation rules and tests passed.
- Example policy now documents the operator block.

### What didn't work
- Initial test command failed due sandbox restriction writing to default Go build cache:
- Command: `go test ./internal/policy -count=1`
- Error: `open /Users/kball/Library/Caches/go-build/...: operation not permitted`
- Fix: reran with local cache overrides:
- `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`
- First commit attempt failed once with stale git lock error:
- Error: `Unable to create .../index.lock: File exists`
- Retried commit after lock cleared; commit succeeded.

### What I learned
- In this sandbox, Go tests should consistently use workspace/tmp cache overrides.
- Commit tooling can transiently fail on stale worktree locks; retry is usually sufficient.

### What was tricky to build
- Getting config validation strict enough to prevent bad operator runtime settings without over-constraining optional fields like model id.

### What warrants a second pair of eyes
- Validation boundaries for operator thresholds and token/time defaults in `internal/policy/policy.go`.
- Whether current defaults are too strict/lenient for real multi-run workloads.

### What should be done in the future
- Add policy docs/comments for expected operational ranges of each operator threshold.

### Code review instructions
- Start in `internal/policy/policy.go`:
- `Config.Operator` shape
- defaults in `Default()`
- checks in `Validate()`
- Then review `internal/policy/policy_test.go` new tests for invalid modes/command/budget.
- Validate with:
- `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`

### Technical details
- Example policy block added in `examples/policy.example.json`:
```json
"operator": {
  "unhealthy_confirmations": 2,
  "restart_budget": 3,
  "restart_cooldown_seconds": 60,
  "stale_run_age_seconds": 3600,
  "llm": {
    "mode": "assist",
    "command": "codex",
    "model": "",
    "timeout_seconds": 30,
    "max_tokens": 400
  }
}
```

## Step 2: Persist Operator Retry State in SQLite

This step implemented the persistence layer for operator retry/cooldown state so behavior remains consistent across process restarts. I added a first-class model plus store table/methods for `restart_attempts` and restart timing fields.

This unlocks restart-safe enforcement of retry budgets and cooldown windows in the upcoming operator loop, which was a hard requirement for V1.

### Prompt Context

**User prompt (verbatim):** "ok implement. Keep a diary along the way. Commit as you go"

**Assistant interpretation:** Continue implementing the planned operator system with incremental commits and keep documenting each step in the diary.

**Inferred user intent:** Build the feature in auditable, resumable increments without losing operational state across restarts.

**Commit (code):** `1aa8086` — "Persist operator restart budget state in sqlite"

### What I did
- Added `model.OperatorRunState` in `internal/model/types.go`.
- Extended SQLite schema in `internal/store/sqlite.go` with `operator_run_states`.
- Added store methods:
- `UpsertOperatorRunState`
- `GetOperatorRunState`
- Added persistence test in `internal/store/sqlite_test.go`:
- write operator state
- reopen store
- verify state round-trips with expected timestamps and attempts
- Ran formatting and focused tests.

### Why
- Retry/cooldown counters must survive process restarts so safety behavior is deterministic and restartable in production use.

### What worked
- Schema migration pattern (`CREATE TABLE IF NOT EXISTS`) integrated cleanly.
- Round-trip test validates durability across store reopen.
- `internal/store` tests pass with local Go cache overrides.

### What didn't work
- No new functional blockers in this step.
- Continued requirement: run tests with `GOCACHE`/`GOMODCACHE` overrides in sandbox.

### What I learned
- Existing store patterns were easy to extend for operator-specific state without introducing a new migration subsystem.

### What was tricky to build
- Time-field fidelity in tests (`RFC3339` parse/format and equality) required second-level truncation to avoid flaky comparisons.

### What warrants a second pair of eyes
- Whether `OperatorRunState` should also track additional counters (for example consecutive unhealthy observations) now or later.
- Long-term schema evolution strategy once operator state grows.

### What should be done in the future
- Add a targeted reset/delete API for operator state during terminal run transitions if we decide to prune stale rows automatically.

### Code review instructions
- Start with `internal/store/sqlite.go`:
- schema addition for `operator_run_states`
- new `UpsertOperatorRunState`/`GetOperatorRunState` methods
- Then check `internal/store/sqlite_test.go`:
- `TestOperatorRunStatePersistsAcrossStoreReopen`
- Validate with:
- `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/store -count=1`

### Technical details
- Stored columns:
- `run_id` (PK)
- `restart_attempts` (integer)
- `last_restart_at` (RFC3339 string)
- `cooldown_until` (RFC3339 string)
- `updated_at` (RFC3339 string)
