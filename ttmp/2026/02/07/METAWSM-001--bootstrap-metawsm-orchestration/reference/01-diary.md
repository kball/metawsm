---
Title: Diary
Ticket: METAWSM-001
Status: active
Topics:
    - core
    - cli
    - tui
    - gui
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: README.md
      Note: Operator usage and policy documentation
    - Path: cmd/metawsm/main.go
      Note: Initial CLI with run/status/resume/stop/close/policy-init
    - Path: internal/hsm/hsm.go
      Note: Lifecycle transition guards
    - Path: internal/model/types.go
      Note: Shared state models
    - Path: internal/orchestrator/service.go
      Note: Core orchestration engine and HSM-driven execution
    - Path: internal/policy/policy.go
      Note: Declarative policy defaults and validation
    - Path: internal/store/sqlite.go
      Note: SQLite durable state store and migration bootstrap
    - Path: ttmp/2026/02/07/METAWSM-001--bootstrap-metawsm-orchestration/design-doc/01-core-orchestrator-capabilities.md
      Note: Design decisions driving implementation
ExternalSources: []
Summary: Implementation diary for initial metawsm MVP scaffolding (CLI, policy, HSM, SQLite state store, orchestration engine, tests, and smoke validation).
LastUpdated: 2026-02-07T07:06:30-08:00
WhatFor: ""
WhenToUse: ""
---


# Diary

## Goal

Capture the implementation journey for the initial `metawsm` MVP so future iterations can resume quickly with full context.

## Step 1: Implement Initial MVP Skeleton (CLI + Policy + HSM + SQLite + Engine)

I implemented the first end-to-end code path for `metawsm`: command surface, durable run storage, lifecycle transitions, plan compilation, and command execution. The objective was to move from a docs-only repo to a working binary with enough structure to iterate safely.

This step intentionally prioritizes explicit state transitions and durable persistence over feature breadth. It establishes the architecture and interfaces needed for subsequent iterations (parallel execution, richer health signals, TUI workflows).

### Prompt Context

**User prompt (verbatim):** "OK go ahead and implement, keep a diary as you go"

**Assistant interpretation:** Build the actual `metawsm` code now, and continuously document what changed and why in a structured diary.

**Inferred user intent:** Move from planning docs to executable code while preserving implementation traceability for future collaboration.

**Commit (code):** N/A (working tree changes only in this session)

### What I did
- Created module/runtime scaffolding: `go.mod`, `go.work`, `.gitignore`.
- Added CLI entrypoint with subcommands in `cmd/metawsm/main.go`.
- Added domain model types in `internal/model/types.go`.
- Added declarative policy model/loader/validator/defaults in `internal/policy/policy.go`.
- Added HSM transition guards for run/step/agent state machines in `internal/hsm/hsm.go`.
- Added SQLite-backed durable store implemented via `sqlite3` CLI in `internal/store/sqlite.go`.
- Added orchestration service implementing:
  - plan building,
  - run lifecycle transitions,
  - step execution with retries,
  - tmux agent/session handling,
  - stop/resume/close/status flows,
  - health evaluation from liveness + activity + progress.
- Added unit/integration tests:
  - `internal/hsm/hsm_test.go`
  - `internal/policy/policy_test.go`
  - `internal/store/sqlite_test.go`
  - `internal/orchestrator/service_test.go`
- Rewrote `README.md` with setup/usage/policy docs and added `examples/policy.example.json`.

### Why
- Needed a concrete implementation aligned with ticket decisions (`HSM`, `SQLite`, declarative policy, clean-git close gate).
- Needed durable, inspectable run state to support resume/stop/close behavior.
- Needed command scaffolding and tests to establish a stable development base.

### What worked
- `go test ./... -count=1` passed after implementation and fixes.
- `go build ./...` passed.
- `go run ./cmd/metawsm policy-init` successfully wrote `.metawsm/policy.json`.
- `go run ./cmd/metawsm run --ticket METAWSM-001 --repos metawsm --dry-run` generated and persisted a plan.
- `go run ./cmd/metawsm status --run-id <id>` produced aggregated run/step/agent status.
- `go run ./cmd/metawsm stop --run-id <id>` transitioned run to `stopped` without noisy tmux errors after fix.

### What didn't work
- Initial compile formatting failed due a typo in imports:
  - Command: `gofmt -w .../internal/orchestrator/service.go`
  - Error: `/internal/orchestrator/service.go:11:6: string literal not terminated`
  - Cause: missing quote around `sort` import.
- Initial dry-run flow failed due missing HSM transition:
  - Command: `go run ./cmd/metawsm run --ticket METAWSM-001 --repos metawsm --agent agent --dry-run`
  - Error: `error: illegal run transition planning -> paused`
  - Cause: `planning -> paused` not included in run transition table.
- Early status/stop checks printed tmux socket errors:
  - Error text: `error connecting to /private/tmp/tmux-501/default (Operation not permitted)`
  - Cause: using output-printing shell helper for tmux liveness/kill checks.
  - Fix: switched to quiet tmux helpers (`tmuxHasSession`, `tmuxKillSession`) that suppress expected stderr.

### What I learned
- Even MVP orchestration needs strict transition coverage immediately; missing one edge (`planning -> paused`) breaks user-facing workflows fast.
- Using `sqlite3` CLI is a practical no-dependency bridge for SQLite persistence in a restricted environment.
- Health polling should avoid noisy stderr paths because operator-facing status should stay clean.

### What was tricky to build
- Balancing strict HSM legality with usable CLI behavior (`dry-run`, `resume`, `stop`, `close`) without overcomplicating the first pass.
- Modeling tmux session orchestration while avoiding attach/blocking behavior in a non-interactive CLI.
- Handling workspace path resolution for generated workspace names through WSM config files.

### What warrants a second pair of eyes
- SQLite command construction/escaping in `internal/store/sqlite.go` (SQL safety and correctness on unusual strings).
- HSM transition completeness in `internal/hsm/hsm.go` as more states are introduced.
- Close flow assumptions in `internal/orchestrator/service.go` around workspace naming/path resolution.
- Health evaluation semantics (`last_progress_at` update policy) for long-running agents.

### What should be done in the future
- Implement policy versioned migrations and stronger schema validation.
- Add richer progress-heartbeat ingestion (not only activity-derived updates).
- Implement true parallel step execution for multi-ticket runs.
- Add TUI monitor implementation and operator controls (task #17).

### Code review instructions
- Start at orchestration core: `internal/orchestrator/service.go`.
- Review state guarantees next: `internal/hsm/hsm.go`.
- Review persistence model: `internal/store/sqlite.go`.
- Review CLI contract: `cmd/metawsm/main.go`.
- Validate with:
  - `go test ./... -count=1`
  - `go build ./...`
  - `go run ./cmd/metawsm policy-init`
  - `go run ./cmd/metawsm run --ticket METAWSM-001 --repos metawsm --dry-run`
  - `go run ./cmd/metawsm status --run-id run-stop-test`

### Technical details
- New files:
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/model/types.go`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/policy/policy.go`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/hsm/hsm.go`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite.go`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/hsm/hsm_test.go`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/policy/policy_test.go`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite_test.go`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/examples/policy.example.json`
- Updated files:
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/README.md`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/.gitignore`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/go.mod`
  - `/Users/kball/workspaces/2026-02-07/metawsm/metawsm/go.work`
