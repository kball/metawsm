---
Title: Diary
Ticket: METAWSM-011
Status: active
Topics:
    - backend
    - chat
    - gui
    - websocket
    - core
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: |-
        Adds serve CLI command and runtime flags (commit b553d46)
        Forum CLI commands now consume serviceapi.Core (commit b251e84)
    - Path: internal/orchestrator/service_forum.go
      Note: Exposes forum outbox stats for runtime health (commit b553d46)
    - Path: internal/server/runtime.go
      Note: |-
        Daemon runtime lifecycle and health endpoint (commit b553d46)
        Runtime now depends on serviceapi.Core abstraction (commit b251e84)
    - Path: internal/server/worker.go
      Note: |-
        Durable forum worker loop and metrics snapshots (commit b553d46)
        Worker loop now consumes serviceapi.Core (commit b251e84)
    - Path: internal/serviceapi/core.go
      Note: Introduces shared Core interface and LocalCore adapter (commit b251e84)
    - Path: internal/store/sqlite_forum.go
      Note: Adds forum outbox count and oldest timestamp queries (commit b553d46)
    - Path: internal/store/sqlite_test.go
      Note: Regression coverage for outbox stats helpers (commit b553d46)
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-09T09:28:18.302287-08:00
WhatFor: ""
WhenToUse: ""
---



# Diary

## Goal

Capture task-by-task implementation progress for METAWSM-011, including exact commits, validation commands, and operator-relevant review guidance.

## Step 1: Add `metawsm serve` and durable forum worker loop

I implemented Tasks 1 and 2 from METAWSM-011 by introducing a long-running server runtime and a managed forum worker loop. This creates a durable process shape (`metawsm serve`) with graceful shutdown handling and health visibility, rather than relying on short-lived CLI process lifetimes.

The implementation also added outbox metrics plumbing from SQLite store up through orchestrator service APIs, so worker health and queue lag can be reported immediately from `/api/v1/health` while the API surface is still being expanded in later tasks.

### Prompt Context

**User prompt (verbatim):** "OK yeah implement task by task, keep a diary and commit as you go. Continue until complete."

**Assistant interpretation:** Implement the METAWSM-011 backlog sequentially, committing each completed slice and maintaining detailed diary/changelog records.

**Inferred user intent:** Ship the full durable daemon + web server architecture end-to-end with clear progress traceability and reviewable incremental commits.

**Commit (code):** b553d46c0aa8b2d0b14724286105bd5beeb9795c — "Add serve daemon command and durable forum worker runtime"

### What I did
- Added `metawsm serve` CLI command and usage wiring in `cmd/metawsm/main.go`.
- Added `internal/server/runtime.go` with:
  - runtime lifecycle orchestration,
  - graceful shutdown,
  - base HTTP mux and `/api/v1/health` endpoint.
- Added `internal/server/worker.go` with durable forum worker loop:
  - periodic `ProcessForumBusOnce`,
  - bus health checks,
  - outbox lag/status snapshots,
  - periodic operational log summaries.
- Added outbox stats model (`ForumOutboxStats`) in `internal/model/forum_bus.go`.
- Added outbox stat queries in store layer:
  - `CountForumOutboxByStatus`,
  - `OldestForumOutboxCreatedAt`.
- Added orchestrator API:
  - `Service.ForumOutboxStats()`,
  - `Service.Shutdown()` for controlled forum bus stop.
- Extended store tests in `internal/store/sqlite_test.go` for the new outbox stats helpers.
- Ran formatting and focused tests:
  - `gofmt -w ...`
  - `go test ./internal/store -count=1`
  - `go test ./internal/orchestrator -count=1`
  - `go test ./cmd/metawsm -count=1`

### Why
- Task 1 requires a durable process entrypoint with clean lifecycle semantics.
- Task 2 requires continuous forum worker execution with operational visibility (health + backlog lag), which needs explicit outbox metrics and worker state exposure.

### What worked
- All focused package tests passed after implementation and formatting.
- Worker loop, bus health checks, and health endpoint wiring compiled and integrated cleanly with existing orchestrator APIs.
- Task checklist updated for tasks 1 and 2.

### What didn't work
- N/A in this step; no blocking runtime/test failures were encountered.

### What I learned
- Existing forum outbox/store APIs were close to sufficient; adding lightweight count/oldest queries provided enough signal for daemon observability without major schema changes.

### What was tricky to build
- Keeping worker state snapshots concurrency-safe while mixing bus health, outbox metrics, and processing counters.
- Ensuring server shutdown order is deterministic: HTTP shutdown, worker cancellation, then forum bus teardown.

### What warrants a second pair of eyes
- Whether `ProcessForumBusOnce` cadence defaults (`500ms`, batch `100`) are appropriate under high event throughput.
- Whether health endpoint should remain `503` when Redis is unavailable versus returning `200` with a degraded payload for some operator environments.

### What should be done in the future
- Implement shared internal service API abstraction (Task 3) and route both CLI + HTTP through it.
- Expand HTTP API and WebSocket live stream support (Tasks 4 and 5).

### Code review instructions
- Start with server lifecycle and worker loop:
  - `internal/server/runtime.go`
  - `internal/server/worker.go`
- Check orchestration/stats plumbing:
  - `internal/orchestrator/service_forum.go`
  - `internal/store/sqlite_forum.go`
  - `internal/model/forum_bus.go`
- Validate with:
  - `go test ./internal/store -count=1`
  - `go test ./internal/orchestrator -count=1`
  - `go test ./cmd/metawsm -count=1`

### Technical details
- New command:
  - `metawsm serve --addr :3001 --db .metawsm/metawsm.db`
- New health endpoint:
  - `GET /api/v1/health`
- Worker snapshot now includes:
  - bus health/error,
  - pending/processing/failed outbox counts,
  - oldest pending outbox age,
  - processing totals and consecutive error count.

## Step 2: Add shared internal service API for CLI + server

I implemented Task 3 by introducing a dedicated internal service API package (`internal/serviceapi`) and routing both forum CLI commands and daemon runtime/worker to consume it. This creates a single application-facing contract that is independent from transport surfaces and prepares the HTTP layer to use the same methods next.

I intentionally kept the implementation as a thin adapter over `orchestrator.Service` so behavior remains stable while the abstraction boundary is introduced. This keeps risk low while enabling future HTTP and WebSocket handlers to share the same service core.

### Prompt Context

**User prompt (verbatim):** "OK yeah implement task by task, keep a diary and commit as you go. Continue until complete."

**Assistant interpretation:** Continue backlog implementation sequentially with small validated commits and matching ticket documentation updates.

**Inferred user intent:** Ensure architectural decisions are reflected in real code structure (not only design docs), specifically the shared internal service API requirement.

**Commit (code):** b251e84923a4efdc5ddbb3aec3a6af31bc8b2082 — "Add shared service API layer for CLI and server consumers"

### What I did
- Added `internal/serviceapi/core.go`:
  - `Core` interface defining forum + runtime-facing operations,
  - `LocalCore` adapter backed by `orchestrator.Service`,
  - shared option/type aliases for forum operations.
- Refactored daemon runtime and worker to depend on `serviceapi.Core` rather than direct `orchestrator.Service` concrete type.
- Refactored all forum CLI subcommands in `cmd/metawsm/main.go` to use `newForumCore()` and `serviceapi` option types.
- Added explicit `defer core.Shutdown()` for forum command invocations.
- Ran formatting and focused validation:
  - `gofmt -w cmd/metawsm/main.go internal/server/runtime.go internal/server/worker.go internal/serviceapi/core.go`
  - `go test ./internal/server -count=1`
  - `go test ./cmd/metawsm -count=1`
  - `go test ./internal/orchestrator -count=1`

### Why
- Task 3 explicitly requires one internal service API consumed by both CLI and HTTP.
- This abstraction avoids business-logic drift as HTTP endpoints are added in the next tasks.

### What worked
- Forum CLI still compiles and tests pass after moving through `serviceapi`.
- Runtime/worker dependencies now target an interface boundary suitable for reuse and testing.
- Task checklist updated with Task 3 completed.

### What didn't work
- N/A in this step; no implementation blockers or test failures occurred.

### What I learned
- Most forum-facing behavior was already encapsulated in orchestrator methods, so introducing `serviceapi` was mainly a dependency-direction and ownership change rather than domain rewrites.

### What was tricky to build
- Ensuring refactor breadth was complete across all forum subcommands so no hidden direct orchestrator dependency remained in CLI paths.
- Keeping shutdown behavior explicit after introducing interface-based construction.

### What warrants a second pair of eyes
- Interface surface size in `serviceapi.Core` (verify it is right-sized and not prematurely broad).
- Whether `LocalCore` should eventually include run-list/detail methods or remain forum-focused with separate query interfaces.

### What should be done in the future
- Implement HTTP endpoints using `serviceapi.Core` directly (Task 4).
- Add WebSocket streaming endpoint on top of the same service layer (Task 5).

### Code review instructions
- Start with abstraction boundary:
  - `internal/serviceapi/core.go`
- Verify consuming integrations:
  - `internal/server/runtime.go`
  - `internal/server/worker.go`
  - `cmd/metawsm/main.go` (forum command paths)
- Validate with:
  - `go test ./cmd/metawsm -count=1`
  - `go test ./internal/orchestrator -count=1`
  - `go test ./internal/server -count=1`

### Technical details
- New constructor:
  - `serviceapi.NewLocalCore(dbPath)`
- Shared CLI helper:
  - `newForumCore(dbPath)` now returns `serviceapi.Core`.
- Shared shutdown path:
  - `core.Shutdown()` used by CLI forum commands and server runtime cleanup.
