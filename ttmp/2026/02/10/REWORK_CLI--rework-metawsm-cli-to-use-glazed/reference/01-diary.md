---
Title: Diary
Ticket: REWORK_CLI
Status: active
Topics:
    - cli
    - core
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: |-
        Added testable usage matrix source of truth
        Added usageCommandLines and usageText for testable CLI baseline
    - Path: cmd/metawsm/main_test.go
      Note: |-
        Added usage matrix coverage test
        Added usage matrix regression test
    - Path: ttmp/2026/02/10/REWORK_CLI--rework-metawsm-cli-to-use-glazed/reference/01-cli-baseline-matrix.md
      Note: |-
        Recorded parser and command baseline metrics
        Recorded command family and parser metrics baseline
ExternalSources: []
Summary: Implementation diary for REWORK_CLI migration execution
LastUpdated: 2026-02-10T09:20:00-08:00
WhatFor: Track implementation steps, tradeoffs, failures, and validation for CLI migration.
WhenToUse: During ticket execution and handoff/review.
---


# Diary

## Goal

Capture implementation progress for `REWORK_CLI` with exact code/test/documentation changes and commit checkpoints.

## Step 1: Establish CLI baseline matrix and usage guardrails

I started by confirming that the ticket already had an actionable phased task list, so no additional task decomposition was needed. Then I created baseline documentation and test guardrails before touching parser architecture.

The goal of this step was to lock in current command/usage expectations so later glazed migration commits can be validated against a known-good command matrix.

### Prompt Context

**User prompt (verbatim):** "Use docmgr to pick up ticket REWORK_CLI. Read the plan, break it into tasks if it isn't already, then implement. Keep a diary and commit as you go."

**Assistant interpretation:** Pick up the `REWORK_CLI` ticket in docmgr, execute the migration plan in task order, keep a detailed diary, and make incremental commits.

**Inferred user intent:** Execute the CLI migration with disciplined documentation and small, reviewable checkpoints.

**Commit (code):** de9f9d6eaa5427c8db4a0b7afc83818e53816da6 — "metawsm: baseline CLI usage matrix for migration parity"

### What I did

- Confirmed `REWORK_CLI` in `metawsm/ttmp` already contains an 8-step task list.
- Added reference docs:
  - `reference/01-diary.md`
  - `reference/01-cli-baseline-matrix.md`
- Added a usage matrix source of truth in code via `usageCommandLines` and `usageText()`.
- Updated `printUsage()` to print from `usageText()`.
- Added `TestUsageTextIncludesExpectedCommandMatrix` to lock baseline usage coverage.
- Verified baseline tests with `go test ./cmd/metawsm -count=1`.

### Why

- Migration safety requires preserving command behavior and help surface while changing parser architecture.
- A testable usage matrix reduces drift risk while command registration is migrated to glazed/cobra.

### What worked

- Existing parser behavior tests already passed.
- Usage output refactor (`usageText`) allowed direct test coverage with no behavior change.
- Ticket documentation now contains explicit baseline metrics for future parity checks.

### What didn't work

- Initial ticket scaffold under the workspace root (`ttmp/.../REWORK_CLI--rework-cli-to-use-glazed`) contained no plan details; the actual plan was under `metawsm/ttmp/...`.
- Command used during investigation:
  - `docmgr doc list --ticket REWORK_CLI` (workspace root) -> no docs found.
- Resolution: switched to the `metawsm` subproject docmgr root and used `metawsm/ttmp/...` ticket artifacts.

### What I learned

- The repository has multiple docmgr roots; ticket lookup must match the active subproject (`metawsm/.ttmp.yaml`) to avoid empty scaffolds.
- Turning usage text into data (`usageCommandLines`) provides a simple parity anchor for parser migration.

### What was tricky to build

- Maintaining exact existing help text while making it testable required refactoring print output without changing line content/order.

### What warrants a second pair of eyes

- Confirm that the usage-line list remains intentionally manual and complete as command surface evolves.

### What should be done in the future

- Continue with step 2: add glazed dependency and root command scaffolding.

### Code review instructions

- Start with `cmd/metawsm/main.go` (`usageCommandLines`, `usageText`, `printUsage`).
- Then review `cmd/metawsm/main_test.go` (`TestUsageTextIncludesExpectedCommandMatrix`).
- Validate with:
  - `go test ./cmd/metawsm -count=1`

### Technical details

- Baseline parser metrics recorded in `reference/01-cli-baseline-matrix.md`:
  - `31` `flag.NewFlagSet` callsites
  - `185` flag definition callsites
  - `21` usage command lines
