---
Title: Diary
Ticket: METAWSM-010
Status: active
Topics:
    - core
    - backend
    - cli
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: internal/forumbus/runtime.go
      Note: Implements forum bus runtime lifecycle
    - Path: internal/forumbus/runtime_test.go
      Note: Validates runtime publish/process behavior
    - Path: internal/model/forum_bus.go
      Note: Defines outbox message and status model
    - Path: internal/model/forum_control.go
      Note: Defines versioned forum control payload schema and validation
    - Path: internal/orchestrator/service_forum.go
      Note: Adds control signal append flow and one-thread-per-run-agent control thread enforcement
    - Path: internal/orchestrator/service_test.go
      Note: Adds service-level test for control thread reuse
    - Path: internal/store/sqlite.go
      Note: Adds forum_control_threads mapping table for store-level enforcement
    - Path: internal/store/sqlite_forum.go
      Note: Implements control thread mapping read/write queries
    - Path: internal/store/sqlite_test.go
      Note: Adds store-level mapping tests
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-09T06:24:19.957975-08:00
WhatFor: ""
WhenToUse: ""
---



# Diary

## Goal

Track implementation progress for METAWSM-010 as task-list-ordered, commit-by-commit diary entries with validation output and review guidance.

## Step 1: Implement control payload schema and single control-thread mapping

I started with the two prerequisite tasks from the new backlog: define explicit, versioned forum control payload contracts and enforce one control thread per `(run_id, agent_name)`. This was done before transport cutover so later lifecycle logic (`syncBootstrapSignals`, close gates, and guide removal) has a stable control substrate.

The implementation added a typed control payload model, store-level control-thread mapping table/queries, and service-level control signal append flow that reuses a deterministic control thread. This gives a concrete place to record future guidance/completion/validation events without filesystem signals.

### Prompt Context

**User prompt (verbatim):** "ok yes. follow the task list, implement, keep a diary and commit as you go."

**Assistant interpretation:** Implement METAWSM-010 in task order, keep the diary current, and make incremental commits after each substantial slice.

**Inferred user intent:** Drive the full forum-first migration from planning docs into real code with traceable progress and disciplined commit history.

**Commit (code):** dcd11d1a95b0ed1a0946ac2d397ff5d15ebd8367 — "Add forum control payload schema and single control-thread mapping"

### What I did
- Added `internal/model/forum_control.go`:
  - `ForumControlType` (`guidance_request|guidance_answer|completion|validation`)
  - `ForumControlPayloadV1` with `schema_version` and validation rules
  - `ForumControlThread` mapping model
- Added model validation tests in `internal/model/forum_control_test.go`.
- Added `forum_control_threads` schema table in `internal/store/sqlite.go` with:
  - `PRIMARY KEY (run_id, agent_name)`
  - `UNIQUE(thread_id)`
- Added store mapping methods in `internal/store/sqlite_forum.go`:
  - `UpsertForumControlThread`
  - `GetForumControlThread`
  - `ListForumControlThreads`
- Added service control-signal flow in `internal/orchestrator/service_forum.go`:
  - `ForumControlSignalOptions`
  - `ForumAppendControlSignal`
  - deterministic control thread ID generation and `ensureForumControlThread` helper
- Added tests:
  - `internal/store/sqlite_test.go`: control-thread mapping upsert/lookup behavior
  - `internal/orchestrator/service_test.go`: repeated control signals reuse one thread for same run+agent
- Formatted code:
  - `gofmt -w internal/model/forum_control.go internal/model/forum_control_test.go internal/store/sqlite.go internal/store/sqlite_forum.go internal/store/sqlite_test.go internal/orchestrator/service_forum.go internal/orchestrator/service_test.go`
- Validated tests:
  - `go test ./internal/model ./internal/store ./internal/orchestrator -count=1`

### Why
- `T1` and `T2` are foundational for the rest of the migration. Without versioned control payloads and a single control thread identity, forum-first lifecycle derivation remains ambiguous.

### What worked
- New control payload contracts validate correctly.
- Store mapping enforces one control thread per `(run_id, agent_name)` at data layer.
- Service control-signal API reuses the same control thread across multiple signal posts.
- Focused package tests passed.

### What didn't work
- N/A in this step (no command failures encountered).

### What I learned
- A dedicated control-thread mapping table provides clear one-thread enforcement without requiring invasive forum table migrations.

### What was tricky to build
- Keeping control-thread enforcement at both service and store layers while preserving existing forum thread APIs and tests.

### What warrants a second pair of eyes
- `internal/orchestrator/service_forum.go` control thread ID normalization and deterministic ID strategy.
- `internal/store/sqlite_forum.go` upsert behavior in `UpsertForumControlThread` under concurrent writers.

### What should be done in the future
- Implement `T3/T4` next: transport runtime and outbox so control signals can move to bus-backed execution.

### Code review instructions
- Start here:
  - `internal/model/forum_control.go`
  - `internal/orchestrator/service_forum.go`
  - `internal/store/sqlite_forum.go`
  - `internal/store/sqlite.go`
- Validate with:
  - `go test ./internal/model ./internal/store ./internal/orchestrator -count=1`

### Technical details
- Task status updates:
  - `METAWSM-010` `T1`, `T2` checked complete.
- Deterministic control thread ID format:
  - `fctrl-<sanitized-run-id>-<sanitized-agent-name>`

## Step 2: Add forum bus runtime and durable outbox foundation

I implemented the transport foundation needed for bus-backed command flow by adding a dedicated `forumbus` runtime package and outbox persistence primitives in SQLite. This establishes publish/process mechanics and health checks without yet switching forum commands over to bus dispatch.

The scope here was intentionally foundational: outbox lifecycle APIs, runtime handler registry, publish/consume loop, and tests proving enqueue -> claim -> handle -> sent status transition.

### Prompt Context

**User prompt (verbatim):** "ok yes. follow the task list, implement, keep a diary and commit as you go."

**Assistant interpretation:** Continue implementing the next tasks in order with incremental commits and documented diary/changelog updates.

**Inferred user intent:** Keep momentum on the full migration by landing reusable infrastructure pieces before command-path cutover.

**Commit (code):** 22391b48b2ebb81845129d5f7c541b854a6f05cd — "Add forum bus runtime and durable outbox primitives"

### What I did
- Added `internal/model/forum_bus.go` with outbox status/message model types.
- Added `forum_outbox` table + index in `internal/store/sqlite.go`.
- Added outbox store APIs in `internal/store/sqlite_forum.go`:
  - `EnqueueForumOutbox`
  - `ClaimForumOutboxPending`
  - `MarkForumOutboxSent`
  - `MarkForumOutboxFailed`
  - `ListForumOutboxByStatus`
- Added runtime package `internal/forumbus/runtime.go`:
  - start/stop and health checks
  - topic handler registration
  - outbox-backed `Publish`
  - `ProcessOnce` dispatch loop
- Wired service bootstrap in `internal/orchestrator/service.go`:
  - instantiate and start bus runtime during `NewService`
- Added service bus helpers in `internal/orchestrator/service_forum.go`:
  - `ForumBusHealth`
  - `ProcessForumBusOnce`
- Added tests:
  - `internal/forumbus/runtime_test.go`
  - outbox lifecycle coverage in `internal/store/sqlite_test.go`
- Validation commands:
  - `go test ./internal/model ./internal/store ./internal/forumbus ./internal/orchestrator -count=1`

### Why
- `T3` and `T4` require concrete transport plumbing before command entrypoints can be switched to bus dispatch.

### What worked
- Runtime can publish and process command messages via outbox and topic handlers.
- Outbox status transitions and claim semantics work under tests.
- Existing orchestrator/store tests still pass with runtime initialization in `NewService`.

### What didn't work
- N/A in this step (no command failures encountered).

### What I learned
- Keeping runtime processing explicit (`ProcessOnce`) is a low-risk way to integrate bus mechanics before introducing background worker behavior.

### What was tricky to build
- Outbox claim semantics needed deterministic row selection and status transition to avoid duplicate handling in future concurrent processors.

### What warrants a second pair of eyes
- `internal/store/sqlite_forum.go` claim/update strategy in `ClaimForumOutboxPending`.
- `internal/forumbus/runtime.go` failure handling and retry behavior assumptions.

### What should be done in the future
- Implement `T5/T6`: route forum command entrypoints through the bus and register command-topic consumers.

### Code review instructions
- Start here:
  - `internal/forumbus/runtime.go`
  - `internal/store/sqlite_forum.go`
  - `internal/store/sqlite.go`
  - `internal/orchestrator/service.go`
- Validate with:
  - `go test ./internal/model ./internal/store ./internal/forumbus ./internal/orchestrator -count=1`

### Technical details
- Task status updates:
  - `METAWSM-010` `T3`, `T4` checked complete.
