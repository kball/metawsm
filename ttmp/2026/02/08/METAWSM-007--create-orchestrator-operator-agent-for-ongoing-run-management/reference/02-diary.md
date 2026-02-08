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
    - Path: internal/policy/policy.go
      Note: Added operator policy schema/defaults/validation (commit ca5ed93)
    - Path: internal/policy/policy_test.go
      Note: Added policy tests for operator defaults and invalid settings (commit ca5ed93)
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
