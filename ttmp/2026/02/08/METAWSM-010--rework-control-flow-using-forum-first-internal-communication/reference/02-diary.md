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
    - Path: README.md
      Note: Updated public command and control-flow docs to forum-first signaling
    - Path: cmd/metawsm/main.go
      Note: |-
        Removed guide command and added forum signal subcommand
        watch/operator now consume typed snapshot instead of status parsing
    - Path: docs/system-guide.md
      Note: Replaced file-signal contract with forum signaling contract
    - Path: internal/forumbus/runtime.go
      Note: Implements forum bus runtime lifecycle
    - Path: internal/forumbus/runtime_test.go
      Note: |-
        Validates runtime publish/process behavior
        Added redis outage and outbox replay recovery coverage
    - Path: internal/model/forum_bus.go
      Note: Defines outbox message and status model
    - Path: internal/model/forum_control.go
      Note: Defines versioned forum control payload schema and validation
    - Path: internal/model/types.go
      Note: Removed legacy file-signal payload structs
    - Path: internal/orchestrator/forum_dispatcher.go
      Note: |-
        Adds bus-backed dispatcher abstraction for forum command publishing
        Drains outbox chain to process command + event topics
    - Path: internal/orchestrator/service.go
      Note: Forum-first Guide/syncBootstrapSignals/close-check migration
    - Path: internal/orchestrator/service_forum.go
      Note: |-
        Adds control signal append flow and one-thread-per-run-agent control thread enforcement
        Publishes forum events and registers projection consumers
    - Path: internal/orchestrator/service_forum_control.go
      Note: Derives per-agent forum control lifecycle state
    - Path: internal/orchestrator/service_snapshot.go
      Note: Typed run snapshot API for watch/operator
    - Path: internal/orchestrator/service_test.go
      Note: |-
        Adds service-level test for control thread reuse
        Updated tests for forum-first lifecycle behavior
        Added forum-only lifecycle e2e coverage including resume and close
    - Path: internal/store/sqlite.go
      Note: Adds forum_control_threads mapping table for store-level enforcement
    - Path: internal/store/sqlite_forum.go
      Note: |-
        Implements control thread mapping read/write queries
        Projection idempotency and refresh via forum_projection_events
    - Path: internal/store/sqlite_test.go
      Note: |-
        Adds store-level mapping tests
        Validates projection idempotency markers
        Added projection replay catch-up coverage
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-09T07:05:13-08:00
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

## Step 3: Switch forum command entrypoints to bus-backed dispatcher

With the runtime/outbox foundation in place, I switched forum command entrypoints to dispatch through command topics and registered handlers that execute command-side mutations. This replaces direct service-to-store forum command writes with publish/process semantics while preserving current CLI behavior.

The service now constructs typed forum commands, dispatches to topic handlers, and reads resulting thread state back from projections. This keeps the API behavior stable while moving command execution through the same bus path.

### Prompt Context

**User prompt (verbatim):** "ok yes. follow the task list, implement, keep a diary and commit as you go."

**Assistant interpretation:** Continue implementing the next backlog tasks in order and keep progress documented with incremental commits.

**Inferred user intent:** Move from infrastructure setup to actual command-path cutover, not just scaffolding.

**Commit (code):** d78deff127187700d4b1424721c0c66ade1b8c34 — "Route forum commands through bus dispatcher and topic handlers"

### What I did
- Added dispatcher abstraction in `internal/orchestrator/forum_dispatcher.go`.
- Extended service wiring in `internal/orchestrator/service.go`:
  - forum topic registry capture from policy
  - dispatcher assignment
  - forum bus handler registration during `NewService`
- Added bus command handler registration and dispatch helper in `internal/orchestrator/service_forum.go`.
- Refactored forum command entrypoints to bus dispatch:
  - `ForumOpenThread`
  - `ForumAddPost`
  - `ForumAnswerThread`
  - `ForumAssignThread`
  - `ForumChangeState`
  - `ForumSetPriority`
  - `ForumCloseThread`
  - `ForumAppendControlSignal`
- Validation command:
  - `go test ./internal/orchestrator ./internal/forumbus ./internal/store ./internal/model -count=1`

### Why
- `T5` and `T6` required moving command entrypoints off direct store writes into bus-backed command publication and topic consumer handling.

### What worked
- Forum command APIs still return expected thread results.
- Command execution now traverses outbox-backed topic handlers.
- Existing test suites passed after cutover.

### What didn't work
- N/A in this step (no command failures encountered).

### What I learned
- Registering handlers during `NewService` allows immediate cutover without introducing extra runtime bootstrap commands.

### What was tricky to build
- Preserving existing method-level invariants while replacing direct mutation calls with asynchronous dispatch + post-dispatch reads.

### What warrants a second pair of eyes
- `internal/orchestrator/service_forum.go` handler registration and dispatch flow, especially `ForumAnswerThread` sequencing.
- `internal/orchestrator/forum_dispatcher.go` process-once semantics under bursty command loads.

### What should be done in the future
- Implement `T7` event-projection consumer shape and then proceed to lifecycle migration tasks (`T8+`).

### Code review instructions
- Start here:
  - `internal/orchestrator/forum_dispatcher.go`
  - `internal/orchestrator/service_forum.go`
  - `internal/orchestrator/service.go`
- Validate with:
  - `go test ./internal/orchestrator ./internal/forumbus ./internal/store ./internal/model -count=1`

### Technical details
- Task status updates:
  - `METAWSM-010` `T5`, `T6` checked complete.

## Step 4: Cut runtime lifecycle over to forum-first control signals and remove guide CLI

I migrated the runtime lifecycle path from mixed filesystem+forum behavior to forum-first control-state derivation. `Guide`, bootstrap signal sync, and bootstrap close checks now consume forum control posts only, and the CLI guidance surface now routes through `metawsm forum signal` instead of a dedicated `metawsm guide` command.

This step also removed remaining runtime file-signal readers/writers and related helper code paths in `service.go`, making control-flow state single-sourced to forum control threads. The behavior change was validated with full test-suite execution.

### Prompt Context

**User prompt (verbatim):** "ok yes. follow the task list, implement, keep a diary and commit as you go."

**Assistant interpretation:** Continue executing the task backlog in order, commit each significant slice, and keep diary/changelog/task bookkeeping current.

**Inferred user intent:** Complete a full migration to forum-first control flow without compatibility paths, with auditable incremental progress.

**Commit (code):** 7d5712a2d4b3edb1934363a82e95d5503e145d4f — "Migrate runtime guidance and close checks to forum control signals"

### What I did
- Added `internal/orchestrator/service_forum_control.go`:
  - control payload parser (`parseForumControlPayload`)
  - per-agent control state derivation (`forumControlStatesForRun`)
- Refactored `Guide` in `internal/orchestrator/service.go`:
  - reads pending guidance from forum control state
  - answers by appending `guidance_answer` control signal
  - removed legacy `.metawsm/guidance-response.json` writing and guidance table flow
- Refactored `syncBootstrapSignals()` to forum-only control-state derivation.
- Refactored `ensureBootstrapCloseChecks()` to require forum completion+validation control signals.
- Updated status output guidance section to render pending forum control guidance requests.
- Removed legacy file-signal helper code and deletion behavior from runtime:
  - guidance request/response readers
  - completion/validation readers
  - signal file cleanup during iterate feedback recording
- Removed top-level `metawsm guide` command and usage text in `cmd/metawsm/main.go`.
- Added `metawsm forum signal` subcommand to post typed control signals.
- Updated watch direction hints to use forum signal commands.
- Removed obsolete payload structs from `internal/model/types.go`:
  - `GuidanceRequestPayload`
  - `GuidanceResponsePayload`
  - `CompletionSignalPayload`
- Updated orchestrator tests to forum-first behavior.
- Validation command:
  - `go test ./... -count=1`

### Why
- `T8`, `T9`, `T10`, `T12`, and `T13` are all part of the same runtime cutover boundary. Landing them together avoids partial dual-path behavior.

### What worked
- Runtime transitions now follow forum control semantics end-to-end for guidance/completion/validation.
- CLI guidance surface is consolidated under forum commands.
- Full repository tests passed after refactor.

### What didn't work
- Initial command context was wrong when resuming this step:
  - `git status --short` returned `fatal: not a git repository (or any of the parent directories): .git`
  - resolved by switching to `/Users/kball/workspaces/2026-02-07/metawsm/metawsm` as working directory.

### What I learned
- Centralizing lifecycle state in forum control threads simplifies close gating and status rendering because control semantics are explicit and versioned.

### What was tricky to build
- Preserving run-status transition correctness while replacing synchronous guidance-table checks with derived forum control state.

### What warrants a second pair of eyes
- `internal/orchestrator/service_forum_control.go` payload parsing and state-reduction logic over ordered posts.
- `internal/orchestrator/service.go` run-status transition paths in `syncBootstrapSignals` and `Guide`.
- `cmd/metawsm/main.go` `forum signal` payload validation and actor-type normalization.

### What should be done in the future
- Implement `T7` projection consumers and `T11` typed snapshot API migration for watch/operator.

### Code review instructions
- Start here:
  - `internal/orchestrator/service_forum_control.go`
  - `internal/orchestrator/service.go`
  - `cmd/metawsm/main.go`
  - `internal/orchestrator/service_test.go`
- Validate with:
  - `go test ./... -count=1`

### Technical details
- Task status updates:
  - `METAWSM-010` `T8`, `T9`, `T10`, `T12`, `T13` checked complete.

## Step 5: Add forum event projection consumers and typed watch/operator snapshots

I implemented the remaining control-flow plumbing so forum command handling now emits `forum.events.*` messages and applies projection updates through dedicated consumers. The projection application is now explicitly idempotent via `forum_projection_events`, which gives a durable replay guard independent of command dedupe.

In the same slice, I added a typed run snapshot API and moved watch/operator loading off status-text parsing. `watch` and `operator` now read structured snapshot data from the orchestrator service, while `status` stays as human-oriented rendering.

### Prompt Context

**User prompt (verbatim):** "ok yes. follow the task list, implement, keep a diary and commit as you go."

**Assistant interpretation:** Continue completing the remaining task backlog in implementation order with focused commits and updated diary/changelog/task tracking.

**Inferred user intent:** Finish the migration with concrete transport/projection behavior and remove brittle parsing paths from automation loops.

**Commit (code):** f589f30d952f32ef3ab4020f66ebf0e3f062b8d0 — "Add forum event projections and typed run snapshot API"

### What I did
- Updated forum command bus handlers in `internal/orchestrator/service_forum.go`:
  - after each command mutation, fetches persisted forum event by event ID
  - publishes it to mapped `forum.events.*` topics
  - registers event-topic handlers that apply projections
- Added event topic mapping helper and event publish helper in service layer.
- Updated dispatcher in `internal/orchestrator/forum_dispatcher.go`:
  - drains `ProcessOnce` in a bounded loop so command + event chains process within one dispatch cycle.
- Added projection APIs in `internal/store/sqlite_forum.go`:
  - `GetForumEvent`
  - `ApplyForumEventProjections`
  - projection idempotency helpers keyed by `(projection_name, event_id)` in `forum_projection_events`
  - deterministic thread-view rebuild (`refreshForumThreadView`) and stats refresh path
- Added typed snapshot API in `internal/orchestrator/service_snapshot.go`:
  - `RunSnapshot` with pending guidance, unhealthy agents, diff flag, PR counts, review feedback counts
- Migrated watch/operator load path in `cmd/metawsm/main.go`:
  - `loadWatchSnapshot` now consumes `service.RunSnapshot(...)`
  - no runtime dependency on parsing `service.Status(...)` text
- Added/updated tests:
  - `internal/store/sqlite_test.go`: projection idempotency + projection marker assertions
  - `internal/orchestrator/service_test.go`: command+event outbox flow assertion and typed snapshot guidance coverage
- Validation commands:
  - `go test ./internal/store ./internal/orchestrator ./internal/forumbus -count=1`
  - `go test ./cmd/metawsm ./internal/orchestrator ./internal/store ./internal/forumbus -count=1`
  - `go test ./... -count=1`

### Why
- `T7` required real event-driven projection consumers with idempotent replay protection.
- `T11` required eliminating status-text parsing from machine decision paths (`watch`/`operator`) in favor of structured service data.

### What worked
- Command handlers now publish event-topic messages and projection consumers apply successfully.
- `forum_projection_events` records projection application and blocks duplicate re-apply.
- Watch/operator behavior still passes existing tests after migration to typed snapshot loading.
- Full repository tests passed.

### What didn't work
- `docmgr task list --ticket METAWSM-010` immediately after `task check` still displayed stale checkboxes for `T7`/`T11`; direct `tasks.md` inspection confirmed both were updated to checked state.

### What I learned
- Rebuilding projection rows from command-side source tables is a practical idempotent strategy that avoids brittle event-payload-specific mutation logic.

### What was tricky to build
- Preserving synchronous command UX while introducing command->event->projection chains required bounded dispatcher draining to ensure follow-on event work gets processed in the same command call flow.

### What warrants a second pair of eyes
- `internal/orchestrator/service_forum.go` command handler sequencing and event publish error semantics.
- `internal/store/sqlite_forum.go` projection marker usage and rebuild SQL (`refreshForumThreadView`).
- `internal/orchestrator/service_snapshot.go` parity with prior watch/operator signal semantics.

### What should be done in the future
- Complete `T14` docs refresh and the remaining resilience/e2e coverage (`T15`, `T16`), then perform cutover checklist execution (`T17`).

### Code review instructions
- Start here:
  - `internal/orchestrator/service_forum.go`
  - `internal/store/sqlite_forum.go`
  - `internal/orchestrator/service_snapshot.go`
  - `cmd/metawsm/main.go`
  - `internal/orchestrator/forum_dispatcher.go`
- Validate with:
  - `go test ./... -count=1`

### Technical details
- Task status updates:
  - `METAWSM-010` `T7`, `T11` checked complete.

## Step 6: Refresh forum-first docs, add resilience/e2e coverage, and execute cutover checklist

I finished the remaining migration tasks by updating the user/system docs to forum-first control flow and adding explicit resilience/e2e tests for outage/replay/idempotency/lifecycle behavior. This closes the implementation backlog with runnable evidence that control signaling and watch/operator flows are no longer tied to file signals or status-text parsing.

I also executed the cutover checklist in-repo: verified no runtime `guide` command path, verified no non-test file-signal readers/writers remain, verified watch/operator runtime path uses typed `RunSnapshot`, and re-ran the full test suite. That completed the final cutover task with code-level evidence.

### Prompt Context

**User prompt (verbatim):** "ok yes. follow the task list, implement, keep a diary and commit as you go."

**Assistant interpretation:** Continue implementing the remaining backlog items and keep commit-by-commit documentation updated.

**Inferred user intent:** Fully complete the migration plan, including docs/test hardening and final cutover readiness checks.

**Commit (code):** 99d39da2dc7adf6a8fd64f6db5632fd841985e6e — "Update forum-first docs and add resilience lifecycle tests"

### What I did
- Updated docs for forum-first-only control flow:
  - `README.md`: removed `guide` command references and file-signal section; added `forum signal` guidance and control-signal examples
  - `docs/system-guide.md`: updated command surface and signaling contract to forum-only model
- Added `T15` resilience tests:
  - `internal/forumbus/runtime_test.go`:
    - redis unavailable behavior test (`Healthy`/`ProcessOnce` failure)
    - outbox replay recovery test (failed message replay after handler registration)
  - `internal/store/sqlite_test.go`:
    - projection lag catch-up test by deleting `forum_thread_views` row and replaying event projection
- Added `T16` e2e lifecycle test:
  - `internal/orchestrator/service_test.go`:
    - forum-only lifecycle ask -> answer -> resume -> completion -> validation -> close
    - verifies one control thread per `(run_id, agent_name)` across lifecycle signals
- Executed cutover checklist evidence (`T17`):
  - `rg -n "case \"guide\"|metawsm guide|guideCommand\\(" cmd internal -g "*.go" -g '!**/*_test.go'` (no matches)
  - `rg -n "guidance-request\\.json|guidance-response\\.json|implementation-complete\\.json|validation-result\\.json" cmd internal -g "*.go" -g '!**/*_test.go'` (no matches)
  - `rg -n "RunSnapshot\\(|loadWatchSnapshot\\(" cmd/metawsm/main.go internal/orchestrator -g "*.go"` (typed snapshot path confirmed)
  - `go test ./... -count=1`

### Why
- `T14` required public/operator docs to match the new no-compat runtime behavior.
- `T15` and `T16` required proving the transport and lifecycle behavior under replay/outage and full forum-only execution.
- `T17` required explicit cutover verification and removal of runtime migration ambiguity.

### What worked
- Docs now consistently describe forum-first control flow and commands.
- Resilience tests passed for redis-unavailable and outbox replay recovery scenarios.
- E2E lifecycle test passed for forum-only guidance/completion/validation close flow.
- Full repository tests passed.

### What didn't work
- `docmgr task list --ticket METAWSM-010` had stale rendered output immediately after earlier `task check`; direct task file inspection and subsequent checks confirmed expected task state.

### What I learned
- Explicit replay-oriented tests (projection rebuild + outbox failed->sent recovery) are critical for confidence once command and projection handling are decoupled by bus semantics.

### What was tricky to build
- Building an e2e lifecycle test that includes `resume` while preserving realistic bootstrap close prerequisites (workspace config, doc sync state, run brief done criteria).

### What warrants a second pair of eyes
- `internal/forumbus/runtime_test.go` assumptions around simulated redis unavailability (config-level health guard vs live network faults).
- `internal/orchestrator/service_test.go` e2e lifecycle test setup fidelity for production-like run topologies.
- `README.md` and `docs/system-guide.md` command examples for operator playbook consistency.

### What should be done in the future
- N/A

### Code review instructions
- Start here:
  - `README.md`
  - `docs/system-guide.md`
  - `internal/forumbus/runtime_test.go`
  - `internal/store/sqlite_test.go`
  - `internal/orchestrator/service_test.go`
- Validate with:
  - `go test ./... -count=1`

### Technical details
- Task status updates:
  - `METAWSM-010` `T14`, `T15`, `T16`, `T17` checked complete.
