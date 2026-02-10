---
Title: Diary
Ticket: METAWSM-FORUM-BUGFIX-20260210
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
    - Path: internal/forumbus/runtime.go
      Note: Added bus observer registration and notification during redis message consumption (commit 05d88afe3febf2fe85a3bdbfb775ca38ae891f8f)
    - Path: internal/orchestrator/service_forum.go
      Note: Exposed SubscribeForumEvents callback bridge for decoded ForumEvent payloads (commit 05d88afe3febf2fe85a3bdbfb775ca38ae891f8f)
    - Path: internal/server/forum_event_broker.go
      Note: Introduces ticket/run filtered broker used for websocket fanout (commit 9b46c10cb406de50d95f8cc03e4aca8c5010dad0)
    - Path: internal/server/forum_event_broker_test.go
      Note: Adds broker behavioral tests for filtering
    - Path: internal/server/runtime.go
      Note: Added runtime event pump wiring from live forum subscriber into broker (commit 05d88afe3febf2fe85a3bdbfb775ca38ae891f8f)
    - Path: internal/server/websocket.go
      Note: Reworked stream endpoint to catch-up plus broker-driven live event push (commit 05d88afe3febf2fe85a3bdbfb775ca38ae891f8f)
ExternalSources: []
Summary: Implementation diary for sequential task delivery and validation evidence for METAWSM-FORUM-BUGFIX-20260210.
LastUpdated: 2026-02-10T12:56:00-08:00
WhatFor: Capture what changed, why, test results, risks, and commit traceability for each completed task.
WhenToUse: Use to review implementation history, reproduce verification, and continue work task-by-task.
---



# Diary

## Goal

Track incremental implementation of `METAWSM-FORUM-BUGFIX-20260210`, including code changes, test evidence, failures, and review guidance per task.

## Step 1: Add Server-Side Event Broker Primitive

This step introduces a dedicated in-memory broker in the server layer for websocket fanout. It provides ticket/run filtering and isolates fanout logic from websocket transport concerns so the stream rework can focus on data source wiring and frame semantics.

The goal for this step was to complete Task 1 only: create the broker primitive plus tests. No stream endpoint behavior was changed yet.

### Prompt Context

**User prompt (verbatim):** "Go through and implement task by task, keeping a diary and commiting as you go"

**Assistant interpretation:** Implement each ticket task sequentially, committing incrementally and recording detailed diary entries while work progresses.

**Inferred user intent:** Ensure the forum bugfix work is delivered in auditable slices with clear progress tracking and reproducible validation.

**Commit (code):** `9b46c10cb406de50d95f8cc03e4aca8c5010dad0` — "Add forum event broker with ticket/run fanout"

### What I did
- Added `internal/server/forum_event_broker.go`:
- Implemented `ForumEventBroker` with `Subscribe(ticket, runID)`, `Publish(event)`, and `Close()`.
- Added non-blocking delivery (`tryPublishEvent`) with stale-message drop behavior for slow subscribers.
- Added ticket/run filter matching for subscriber scoping.
- Added `internal/server/forum_event_broker_test.go`:
- Coverage for ticket/run filters.
- Coverage for unsubscribe channel closure behavior.
- Coverage for stale-message drop semantics with single-slot buffers.
- Ran:
- `gofmt -w internal/server/forum_event_broker.go internal/server/forum_event_broker_test.go`
- `go test ./internal/server -count=1`

### Why
- Websocket fanout needs a reusable server-side primitive before stream handler migration.
- Filtered subscriptions are required to avoid sending unrelated forum events to clients.

### What worked
- Broker fanout and filtering logic behaved as expected in unit tests.
- Non-blocking publish approach prevented fanout backpressure from blocking sender paths.
- Server package tests passed after adding the new test file.

### What didn't work
- `docmgr doc list --ticket METAWSM-FORUM-BUGFIX-20260210` did not display the newly created reference diary document even though the file exists on disk.
- Command/output:
- `docmgr doc list --ticket METAWSM-FORUM-BUGFIX-20260210`
- Output showed only the design doc.
- No implementation impact; continued by editing the diary file directly at the known path.

### What I learned
- A small broker abstraction keeps websocket stream changes localized and easier to test.
- Dropping stale buffered events is preferable to blocking when clients are slow, because fresh forum state is more valuable than guaranteed delivery of every intermediate frame for this UI.

### What was tricky to build
- Preserving non-blocking behavior while still preferring latest-event delivery required explicit stale-drain logic.
- Unsubscribe and close paths needed careful channel lifecycle handling to avoid panics from double-close.

### What warrants a second pair of eyes
- Whether stale-drop semantics are acceptable for all future consumers (currently optimized for UI freshness).
- Memory/performance behavior with high subscriber counts (current implementation snapshots subscribers per publish).

### What should be done in the future
- Wire the broker to actual forum event ingress from Redis-driven runtime in the next step.

### Code review instructions
- Start at `internal/server/forum_event_broker.go`:
- Review `Subscribe`, `Publish`, `tryPublishEvent`, and filter matching.
- Validate with:
- `go test ./internal/server -count=1`
- Inspect tests in `internal/server/forum_event_broker_test.go` for expected semantics.

### Technical details
- Broker API:
- `Subscribe(ticket, runID)` returns `<-chan model.ForumEvent` and `unsubscribe func()`.
- `Publish(event)` fans out to matching subscribers.
- `Close()` closes all subscriber channels and clears registry.
- Task progress:
- Marked Task 1 complete in `tasks.md` via `docmgr task check --ticket METAWSM-FORUM-BUGFIX-20260210 --id 1`.

## Step 2: Rework Stream Endpoint To Catch-Up Plus Live Push

This step replaced the websocket stream’s timer polling loop with catch-up + live push semantics. The stream now fetches an initial event window and then listens for broker fanout, emitting heartbeats only for liveness.

To source live events from Redis processing, I added a bus observer path through `forumbus -> orchestrator service -> local serviceapi -> server runtime broker`.

### Prompt Context

**User prompt (verbatim):** "Go through and implement task by task, keeping a diary and commiting as you go"

**Assistant interpretation:** Continue sequential implementation, commit each task, and keep detailed diary entries.

**Inferred user intent:** Deliver the forum bugfix as incremental, reviewable slices with clear artifact history.

**Commit (code):** `05d88afe3febf2fe85a3bdbfb775ca38ae891f8f` — "Rework forum stream to use live broker fanout"

### What I did
- Added Redis-consumption observer support in `internal/forumbus/runtime.go`:
- `RegisterObserver(topicPrefix, observer)`.
- Observer notification on successful handler execution in `consumeRedisMessages`.
- Added observer test in `internal/forumbus/runtime_test.go`.
- Added orchestrator hook in `internal/orchestrator/service_forum.go`:
- `SubscribeForumEvents(callback)` decodes event payloads and forwards `model.ForumEvent`.
- Added optional subscriber capability in `internal/serviceapi/core.go`:
- `LiveForumEventSubscriber` interface.
- `LocalCore.SubscribeForumEvents(...)`.
- Wired server runtime pump in `internal/server/runtime.go`:
- Starts/stops forum event pump during runtime lifecycle.
- Publishes incoming live events into `Runtime.eventBroker`.
- Reworked websocket stream behavior in `internal/server/websocket.go`:
- Removed `poll_ms` loop behavior.
- Initial catch-up via `ForumWatchEvents(ticket, cursor, limit)`.
- Live broker subscription filtered by `ticket` and optional `run_id`.
- Heartbeat frames every 25 seconds.
- Updated `internal/server/api_test.go` test runtime helper to initialize event broker.
- Ran:
- `gofmt -w internal/forumbus/runtime.go internal/forumbus/runtime_test.go internal/orchestrator/service_forum.go internal/serviceapi/core.go internal/server/runtime.go internal/server/websocket.go internal/server/api_test.go`
- `go test ./internal/forumbus ./internal/orchestrator ./internal/server ./internal/serviceapi -count=1`

### Why
- The user-reported issue was polling-heavy behavior despite websocket usage.
- Live fanout from Redis event processing removes recurring watch queries in idle periods.

### What worked
- Observer callback chain delivered events from bus processing into the server broker.
- Stream endpoint now emits event frames based on real incoming events instead of periodic polling.
- Package tests passed for all touched backend areas.

### What didn't work
- No functional blockers in this step.

### What I learned
- The existing outbox/Redis processing path is a strong insertion point for event fanout without introducing another Redis client in the server package.
- Keeping event subscription capability as an optional interface avoids broad `Core` interface churn.

### What was tricky to build
- Ensuring observer callbacks do not break bus processing required defensive recovery in observer dispatch.
- Runtime lifecycle needed careful stop ordering so observer unsubscription and broker close happen during shutdown.

### What warrants a second pair of eyes
- Observer callback panic handling strategy (currently recover-and-continue silently).
- Stream behavior when no live event subscriber is available (heartbeat-only mode).

### What should be done in the future
- Tighten websocket tests to explicitly validate heartbeat cadence and broker-driven live frames (Task 3).

### Code review instructions
- Start at `internal/server/websocket.go` for stream protocol behavior.
- Then review `internal/server/runtime.go` for event pump lifecycle.
- Follow event source path in `internal/orchestrator/service_forum.go` and `internal/forumbus/runtime.go`.
- Validate with:
- `go test ./internal/forumbus ./internal/orchestrator ./internal/server ./internal/serviceapi -count=1`

### Technical details
- Stream path:
- Catch-up: one `ForumWatchEvents` call.
- Live: `ForumEventBroker.Subscribe(ticket, runID)`.
- Liveness: heartbeat frame every 25s.
- Event source path:
- `forumbus.Runtime.consumeRedisMessages` -> `notifyObservers` -> `Service.SubscribeForumEvents` -> `LocalCore.SubscribeForumEvents` -> `server.Runtime.startEventPump`.
- Task progress:
- Marked Task 2 complete via `docmgr task check --ticket METAWSM-FORUM-BUGFIX-20260210 --id 2`.
