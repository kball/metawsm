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
    - Path: cmd/metawsm/glazed_low_risk_commands.go
      Note: Migrated policy-init/docs/serve into glazed BareCommand implementations
    - Path: cmd/metawsm/main.go
      Note: |-
        Added testable usage matrix source of truth
        Added usageCommandLines and usageText for testable CLI baseline
        Replaced top-level switch dispatch with executeCLI
    - Path: cmd/metawsm/main_test.go
      Note: |-
        Added usage matrix coverage test
        Added usage matrix regression test
        Added root command registration coverage
    - Path: cmd/metawsm/root_command.go
      Note: Added cobra/glazed root registration with legacy passthrough adapters
    - Path: go.mod
      Note: Added glazed and cobra dependencies
    - Path: go.sum
      Note: Recorded migration dependency checksums
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

## Step 2: Add glazed root scaffolding and migrate low-risk commands

I replaced the top-level switch-based command router with a cobra root command builder, then migrated `policy-init`, `docs`, and `serve` to explicit glazed `BareCommand` implementations. For the remaining command families, I added legacy passthrough cobra commands so runtime behavior stays intact while the migration proceeds incrementally.

This step establishes the new command registration architecture and proves mixed-mode migration works: glazed-managed commands can coexist with unchanged legacy handlers.

### Prompt Context

**User prompt (verbatim):** "Use docmgr to pick up ticket REWORK_CLI. Read the plan, break it into tasks if it isn't already, then implement. Keep a diary and commit as you go."

**Assistant interpretation:** Continue executing the migration plan with small, committed milestones while maintaining ticket docs.

**Inferred user intent:** Move real CLI implementation forward now, not just planning, while preserving reviewability.

**Commit (code):** 16bf5a39d8a53faa5d63342b2b74d5f1928bf115 — "metawsm: add glazed root scaffolding and migrate docs/serve/policy-init"

### What I did

- Added new root command scaffolding in `cmd/metawsm/root_command.go`:
  - `executeCLI(...)` + `newRootCommand()`
  - glazed command registration helper (`buildGlazedCobraCommand`)
  - legacy passthrough wrappers for unmigrated commands
- Added migrated low-risk glazed commands in `cmd/metawsm/glazed_low_risk_commands.go`:
  - `policy-init`
  - `docs`
  - `serve`
- Updated `main()` in `cmd/metawsm/main.go` to execute the new root command.
- Added root registration test coverage in `cmd/metawsm/main_test.go`.
- Added dependencies required by migration (`go.mod`, `go.sum`, `go.work.sum`).

### Why

- The new root scaffolding is required before migrating the remaining command families.
- Low-risk commands provide a safe first migration slice to validate the glazed/cobra integration.
- Legacy passthrough prevents a disruptive big-bang rewrite.

### What worked

- `go test ./cmd/metawsm -count=1` passed after dependency and command wiring updates.
- `go run ./cmd/metawsm docs --help` and `go run ./cmd/metawsm serve --help` now show glazed-generated flag help.
- Root-level usage (`go run ./cmd/metawsm help`) still prints the established usage matrix.

### What didn't work

- First compile attempt failed due missing module requirements:
  - Command: `go test ./cmd/metawsm -count=1`
  - Error excerpts:
    - `no required module provides package github.com/go-go-golems/glazed/pkg/cli`
    - `no required module provides package github.com/spf13/cobra`
- Resolution:
  - `go get github.com/go-go-golems/glazed@v0.7.3`
  - `go get github.com/spf13/cobra@v1.10.1`

### What I learned

- Mixed registration (migrated glazed commands + passthrough legacy commands) provides a low-risk path for command-by-command migration.
- Root help behavior needs explicit handling to avoid losing subcommand-specific `--help` output.

### What was tricky to build

- Balancing compatibility and progress: root-level help needed to keep the existing top-level usage output while preserving command-specific help for migrated subcommands.

### What warrants a second pair of eyes

- Review whether passthrough wrappers should continue to use `DisableFlagParsing` or if early partial parameter-layer adoption is preferable for non-migrated commands.

### What should be done in the future

- Continue with Step 4 migration: run-selector command family with shared selector parameters/layers.

### Code review instructions

- Start with `cmd/metawsm/root_command.go` for registration strategy.
- Review `cmd/metawsm/glazed_low_risk_commands.go` for migrated command implementations.
- Confirm entrypoint switch in `cmd/metawsm/main.go` now routes through `executeCLI`.
- Validate with:
  - `go test ./cmd/metawsm -count=1`
  - `go run ./cmd/metawsm docs --help`
  - `go run ./cmd/metawsm serve --help`

### Technical details

- Migrated commands currently use `cmds.BareCommand` and keep existing output contract.
- Remaining commands are registered as passthrough wrappers to existing functions, preserving behavior until each family is migrated.
