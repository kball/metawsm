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
    - Path: internal/server/api_test.go
      Note: Added websocket stream tests for catch-up
    - Path: internal/server/forum_event_broker.go
      Note: Introduces ticket/run filtered broker used for websocket fanout (commit 9b46c10cb406de50d95f8cc03e4aca8c5010dad0)
    - Path: internal/server/forum_event_broker_test.go
      Note: Adds broker behavioral tests for filtering
    - Path: internal/server/runtime.go
      Note: |-
        Added runtime event pump wiring from live forum subscriber into broker (commit 05d88afe3febf2fe85a3bdbfb775ca38ae891f8f)
        Added configurable stream heartbeat interval used by endpoint and tests (commit 071a080f8287a70c06d17a34b37f7ff51b457695)
    - Path: internal/server/websocket.go
      Note: Reworked stream endpoint to catch-up plus broker-driven live event push (commit 05d88afe3febf2fe85a3bdbfb775ca38ae891f8f)
    - Path: ui/src/App.tsx
      Note: |-
        Websocket onmessage now ignores heartbeat/no-event frames and debounces refreshes (commit afa77ed1d2bfbb0cc285cf168d62378bf6fe3e52)
        Separated debug refresh cadence from filter-driven data refresh and added 15s interval (commit 1a710380748f52260c553b46b35895ba428a9f85)
        Added ask-question composer UI controls and submit gating state (commit 5bfc0ad96c0d3b805e86101c86a93113e1ef3337)
        Ask composer submit now posts to /api/v1/forum/threads and selects created thread (commit e41f9f2040bdf3e7d0d7072c14c2cbe64cf97cf0)
ExternalSources: []
Summary: Implementation diary for sequential task delivery and validation evidence for METAWSM-FORUM-BUGFIX-20260210.
LastUpdated: 2026-02-10T13:05:00-08:00
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

## Step 3: Add Websocket Tests For Event and Heartbeat Semantics

This step expanded websocket endpoint coverage to explicitly assert the new stream model behaviors: catch-up event frames, heartbeat when idle, and broker-fed live event frames. The previous test only validated that some frame arrived.

I also added a configurable stream heartbeat interval in runtime state so endpoint tests can validate heartbeat behavior without waiting 25 seconds.

### Prompt Context

**User prompt (verbatim):** "Go through and implement task by task, keeping a diary and commiting as you go"

**Assistant interpretation:** Continue through the ticket checklist in order, committing each completed item and recording implementation details.

**Inferred user intent:** Ensure each task is concretely implemented and independently verifiable.

**Commit (code):** `071a080f8287a70c06d17a34b37f7ff51b457695` — "Expand forum websocket tests for catchup and heartbeat"

### What I did
- Replaced the old generic stream test in `internal/server/api_test.go` with focused tests:
- `TestHandleForumStreamSendsCatchUpFrame`
- `TestHandleForumStreamSendsHeartbeatWhenIdle`
- `TestHandleForumStreamSendsLiveBrokerEventFrame`
- Added websocket test helpers:
- `openTestWebSocket(...)`
- `readWebSocketJSONFrame(...)`
- Updated test runtime factory to set `streamBeat` to `50ms`.
- Extended runtime options/state in `internal/server/runtime.go`:
- Added `Options.StreamHeartbeat`
- Added `Runtime.streamBeat` and `streamHeartbeatInterval()`.
- Updated stream code in `internal/server/websocket.go` to consume runtime heartbeat interval.
- Ran:
- `gofmt -w internal/server/api_test.go internal/server/runtime.go internal/server/websocket.go`
- `go test ./internal/server -count=1`

### Why
- Task 3 required explicit event/heartbeat websocket behavior tests.
- Heartbeat interval needed to be configurable to make heartbeat tests practical and deterministic.

### What worked
- All three new endpoint tests passed.
- Heartbeat test executes quickly with the test interval.
- Live broker publish test validates stream receives events without polling loops.

### What didn't work
- No blocking issues in this step.

### What I learned
- Endpoint-level websocket tests are much easier to maintain with reusable handshake/frame helpers.
- Runtime-configured heartbeat intervals are useful for both testing and future tuning.

### What was tricky to build
- Ensuring live event publish occurs after websocket subscribe required a short delayed goroutine in the test.
- Frame decoding in tests needed explicit JSON parsing to avoid brittle string matching.

### What warrants a second pair of eyes
- Test timing assumptions around the delayed broker publish (`25ms`) under high CI load.
- Whether to expose heartbeat interval as a user-facing CLI flag in future iterations.

### What should be done in the future
- Add assertions on frame payload content structure (`events[*].envelope`) if schema stability becomes critical.

### Code review instructions
- Start in `internal/server/api_test.go` at the three new stream tests and helper functions.
- Review heartbeat interval plumbing in `internal/server/runtime.go`.
- Confirm stream interval use in `internal/server/websocket.go`.
- Validate with:
- `go test ./internal/server -count=1`

### Technical details
- New stream test scenarios:
- Catch-up only (events from `ForumWatchEvents`).
- Idle heartbeat only (no catch-up, no live events).
- Live push only (broker publish after subscribe).
- Task progress:
- Marked Task 3 complete via `docmgr task check --ticket METAWSM-FORUM-BUGFIX-20260210 --id 3`.

## Step 4: Debounce UI Refresh and Ignore Heartbeats

This step updated frontend websocket handling so refresh work is triggered only by non-empty `forum.events` frames. Heartbeat frames are ignored, and event-driven refreshes are debounced to collapse bursts.

The change is intentionally narrow to satisfy Task 4 without mixing in debug-panel refresh policy changes (Task 5).

### Prompt Context

**User prompt (verbatim):** "Go through and implement task by task, keeping a diary and commiting as you go"

**Assistant interpretation:** Continue sequential execution of checklist items with isolated commits and diary updates.

**Inferred user intent:** Reduce noisy refresh behavior in measurable, auditable increments.

**Commit (code):** `afa77ed1d2bfbb0cc285cf168d62378bf6fe3e52` — "Debounce websocket refresh and ignore heartbeat frames"

### What I did
- Updated `ui/src/App.tsx`:
- Added websocket frame parser (`parseForumStreamFrame`).
- Ignored frames that are not `type === "forum.events"` or have empty `events`.
- Added `useRef` timer-based debounce (`150ms`) around refresh calls.
- Used `selectedThreadRef` to avoid websocket reconnect churn when thread selection changes.
- Removed per-frame debug refresh from websocket handler (debug refresh remains elsewhere).
- Ran:
- `npm --prefix ui run check`

### Why
- Heartbeat-triggered refreshes were causing unnecessary API churn.
- Event bursts can trigger multiple back-to-back refreshes without debounce.

### What worked
- TypeScript check passed after changes.
- Message-path logic now explicitly filters out heartbeat/no-op frames.
- Debounced refresh preserves responsiveness while reducing request bursts.

### What didn't work
- No blocking issues in this step.

### What I learned
- Using refs for selected thread and timer control avoids stale closure behavior and unnecessary websocket reconnections.

### What was tricky to build
- Balancing effect dependencies to keep socket lifecycle stable while still refreshing the current thread detail correctly.

### What warrants a second pair of eyes
- Debounce interval choice (`150ms`) may need tuning under heavy event rates.
- Potential interaction with future optimistic updates in the UI list/detail panels.

### What should be done in the future
- Task 5: reduce automatic debug refresh cadence in non-stream paths.

### Code review instructions
- Review websocket effect and frame parsing in `ui/src/App.tsx`.
- Focus on `socket.onmessage` flow and `parseForumStreamFrame`.
- Validate with:
- `npm --prefix ui run check`

### Technical details
- UI stream gating now requires:
- `frame.type === "forum.events"`
- `Array.isArray(frame.events) && frame.events.length > 0`
- Debounce implementation:
- `window.setTimeout(..., 150)` with cancel-on-replace and cleanup-on-unmount.
- Task progress:
- Marked Task 4 complete via `docmgr task check --ticket METAWSM-FORUM-BUGFIX-20260210 --id 4`.

## Step 5: Reduce Automatic Debug Refresh Frequency

This step reduced debug snapshot refresh frequency by separating debug updates from the high-frequency thread/filter refresh path. Debug data now refreshes on run/ticket changes and a slower periodic timer.

This keeps diagnostics available while avoiding extra requests during routine search/filter interactions.

### Prompt Context

**User prompt (verbatim):** "Go through and implement task by task, keeping a diary and commiting as you go"

**Assistant interpretation:** Continue sequential delivery and commit each completed task with diary documentation.

**Inferred user intent:** Reduce load sources incrementally while preserving observability.

**Commit (code):** `1a710380748f52260c553b46b35895ba428a9f85` — "Throttle forum debug refresh to run-level cadence"

### What I did
- Updated `ui/src/App.tsx` effects:
- Kept `refreshForumData()` tied to filter/view dependencies.
- Moved `refreshDebug(selectedTicket, selectedRunID)` into its own effect scoped to run/ticket changes.
- Added interval-based debug refresh every `15s`.
- Removed debug refresh coupling to queue/search/viewer filter changes.
- Ran:
- `npm --prefix ui run check`

### Why
- Task 5 required lowering debug refresh frequency.
- Debug health does not need to refresh on every UI filter edit.

### What worked
- TypeScript check passed.
- Debug refresh path is now lower-frequency and scoped.
- Manual refresh button behavior remains unchanged.

### What didn't work
- No blockers in this step.

### What I learned
- Separating data concerns into dedicated effects makes refresh cadence tuning straightforward.

### What was tricky to build
- Ensuring existing forum data refresh behavior remained unchanged while extracting debug refresh into a separate cadence.

### What warrants a second pair of eyes
- The `15s` interval may be too slow or too fast depending on operational preference.

### What should be done in the future
- Consider making debug interval configurable from settings or policy if operators need tuning.

### Code review instructions
- Review effect split in `ui/src/App.tsx` around `refreshForumData` and `refreshDebug`.
- Validate with:
- `npm --prefix ui run check`

### Technical details
- Debug refresh cadence:
- Immediate on `[selectedTicket, selectedRunID]` change.
- Interval refresh every `15000ms`.
- Task progress:
- Marked Task 5 complete via `docmgr task check --ticket METAWSM-FORUM-BUGFIX-20260210 --id 5`.

## Step 6: Add Ask-Question Composer UI

This step introduced a visible "Ask as Human" composer in the forum detail panel. It includes title/body/priority inputs and submit button state controls.

This was intentionally scoped to UI surface and local state only, with network submission wiring deferred to Task 7.

### Prompt Context

**User prompt (verbatim):** "Go through and implement task by task, keeping a diary and commiting as you go"

**Assistant interpretation:** Keep implementing each checklist item as a focused, committed slice.

**Inferred user intent:** Make progress visibly and safely by separating UI and API behavior changes.

**Commit (code):** `5bfc0ad96c0d3b805e86101c86a93113e1ef3337` — "Add Ask as Human composer UI in forum detail panel"

### What I did
- Updated `ui/src/App.tsx`:
- Added state for ask form: `questionTitle`, `questionBody`, `questionPriority`, `savingQuestion`.
- Added `canSubmitQuestion` gating.
- Added `submitQuestion()` placeholder function.
- Added "Ask as Human" composer UI block with title/priority/body controls and submit button.
- Ran:
- `npm --prefix ui run check`

### Why
- Task 6 specifically required adding the composer UI for human-originated questions.
- Splitting UI scaffolding from API integration keeps change review focused.

### What worked
- New composer renders with expected fields and button state logic.
- TypeScript check passed.

### What didn't work
- Submission intentionally not wired yet; function sets a temporary error message.

### What I learned
- The existing detail panel layout can host both ask and reply composers without structural refactors.

### What was tricky to build
- Ensuring new form state and button gating are ready for API wiring while keeping this step isolated.

### What warrants a second pair of eyes
- Placement of the ask composer in detail panel may be revisited for UX flow once end-to-end behavior is wired.

### What should be done in the future
- Complete Task 7 by wiring `submitQuestion()` to `POST /api/v1/forum/threads` and thread selection updates.

### Code review instructions
- Review ask composer state and rendering in `ui/src/App.tsx`.
- Validate with:
- `npm --prefix ui run check`

### Technical details
- Composer fields:
- `title` (required)
- `priority` (`urgent|high|normal|low`)
- `body` (required)
- Submit gating:
- Requires selected ticket, viewer ID, non-empty title/body, and not currently submitting.
- Task progress:
- Marked Task 6 complete via `docmgr task check --ticket METAWSM-FORUM-BUGFIX-20260210 --id 6`.

## Step 7: Wire Ask Composer To Thread Creation API

This step connected the ask composer to `POST /api/v1/forum/threads` and completed the human-originated question flow. Successful submissions now create a thread, refresh list/detail, and focus the new thread.

I kept actor identity tied to the viewer input (`actor_name=<viewerID>`) with fixed `actor_type=human` for this composer.

### Prompt Context

**User prompt (verbatim):** "Go through and implement task by task, keeping a diary and commiting as you go"

**Assistant interpretation:** Continue executing each task in order and record implementation outcomes per commit.

**Inferred user intent:** Fully restore human question submission in the forum UI, not only visually but functionally.

**Commit (code):** `e41f9f2040bdf3e7d0d7072c14c2cbe64cf97cf0` — "Wire human ask composer to forum thread create API"

### What I did
- Updated `ui/src/App.tsx`:
- Implemented `submitQuestion()` network flow:
- `POST /api/v1/forum/threads`
- Payload includes: `ticket`, `run_id`, `title`, `body`, `priority`, `actor_type: "human"`, `actor_name: viewerID`.
- On success:
- Clears ask fields.
- Resets priority to `normal`.
- Sets queue tab to `all`.
- Refreshes forum data.
- Selects and loads created thread detail.
- Marks created thread seen.
- Added `viewerType === "human"` requirement in `canSubmitQuestion`.
- Added inline UI hint when viewer type is not human.
- Ran:
- `npm --prefix ui run check`

### Why
- Task 7 required real API submission using viewer-backed identity.
- Human submission bug remains unresolved without this wiring.

### What worked
- TypeScript check passed.
- Ask composer now performs end-to-end thread creation workflow on success.
- Created thread becomes selected in UI when the API returns thread ID.

### What didn't work
- No blockers in this step.

### What I learned
- Reusing existing thread refresh/detail/seen functions minimized additional state complexity.

### What was tricky to build
- Keeping button gating and viewer role constraints consistent while preserving a straightforward UX.

### What warrants a second pair of eyes
- Whether forcing `actor_type="human"` in this composer is always desired, even if viewer mode changes in future UX.
- Potential duplicate refresh work from both manual refresh and stream-driven updates right after create.

### What should be done in the future
- Add UI tests for the ask flow (Task 8) to lock behavior.

### Code review instructions
- Review `submitQuestion()` and `canSubmitQuestion` in `ui/src/App.tsx`.
- Validate with:
- `npm --prefix ui run check`

### Technical details
- Ask submit payload keys:
- `ticket`, `run_id`, `title`, `body`, `priority`, `actor_type`, `actor_name`
- Post-success sequence:
- `refreshForumData` -> `setSelectedThreadID` -> `refreshThreadDetail` -> `markThreadSeen`
- Task progress:
- Marked Task 7 complete via `docmgr task check --ticket METAWSM-FORUM-BUGFIX-20260210 --id 7`.
