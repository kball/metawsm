---
Title: 'Fix Plan: event-driven forum stream and human ask flow'
Ticket: METAWSM-FORUM-BUGFIX-20260210
Status: active
Topics:
    - backend
    - chat
    - gui
    - websocket
    - core
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: internal/forumbus/runtime.go
      Note: Redis stream publish/subscribe runtime used as event source
    - Path: internal/server/api.go
      Note: Thread open endpoint contract and stream route wiring
    - Path: internal/server/api_test.go
      Note: Existing websocket stream tests to extend for event-driven behavior
    - Path: internal/server/websocket.go
      Note: Current websocket stream loops on poll_ms and emits heartbeat frames
    - Path: internal/serviceapi/core.go
      Note: Core interface currently exposes polling watch events used by stream handler
    - Path: ui/src/App.tsx
      Note: Frontend refreshes forum data for every websocket frame and lacks open-thread composer
ExternalSources: []
Summary: Design for replacing timer-based stream polling with Redis event fanout and adding human thread creation in forum UI.
LastUpdated: 2026-02-10T11:58:00-08:00
WhatFor: Resolve forum polling load and unblock human-originated question submission from the web UI.
WhenToUse: Use when implementing or reviewing forum stream delivery and human ask/response UX behavior.
---


# Fix Plan: event-driven forum stream and human ask flow

## Executive Summary

Fix two forum regressions with one focused increment:
1. Replace timer-driven websocket polling with event-driven fanout backed by Redis forum events.
2. Add first-class "Ask as Human" flow in the web UI so humans can open new forum threads (not only reply).

The first fix removes avoidable query load and stale-refresh churn. The second fix closes a missing UX path that currently blocks human-originated questions from the forum explorer.

## Problem Statement

### Bug 1: Excessive polling despite websocket usage

Current stream behavior is still poll-based:
- `internal/server/websocket.go` calls `ForumWatchEvents` in a timed loop (`poll_ms`, default 1000ms).
- `ui/src/App.tsx` refreshes full forum data on every websocket frame, including heartbeat frames.
- Each refresh fans out into multiple REST calls (`/forum/search`, two `/forum/queues`, optional `/forum/threads/{id}`, `/forum/debug`).

Result: each open client produces recurring DB/API load even when there are no new forum events.

### Bug 2: Human cannot submit a new question in forum UI

Current UI only offers "Respond as Human" for an existing thread:
- `ui/src/App.tsx` includes `submitReply()` (`POST /api/v1/forum/threads/{id}/posts`).
- There is no "open thread" composer bound to `POST /api/v1/forum/threads`.

Result: humans can reply but cannot ask/open a new question from the forum UI.

## Proposed Solution

### A. Event-driven websocket stream

1. Add a server-side event broker for forum events.
- New component in `internal/server` manages websocket subscribers by `ticket` (and optional `run_id`).
- Broker receives events from Redis forum event topics and fans out only matching events.

2. Stream handshake behavior:
- On websocket connect, optionally send one catch-up frame from `ForumWatchEvents(ticket, cursor, limit)`.
- After catch-up, send live `forum.events` frames from broker fanout.
- Send heartbeat at a long interval (e.g. 20-30s) strictly for connection liveness.

3. UI message handling:
- Parse websocket payload type.
- Ignore `heartbeat` for data refresh.
- Refresh only on `forum.events` with non-empty events.
- Debounce list/detail refresh (e.g. 100-250ms) to coalesce bursty event batches.
- Remove debug refresh from every event frame; keep manual refresh button and optional slow interval refresh.

### B. Human "Ask Question" flow

1. Add new composer panel in `ui/src/App.tsx`:
- Fields: `title`, `body`, `priority`.
- Context fields default from current run/ticket selection (`ticket`, optional `run_id`).
- Actor defaults from viewer controls: `actor_type=human`, `actor_name=<viewerID>`.

2. Submit behavior:
- `POST /api/v1/forum/threads` with validated payload.
- On success: clear form, select new thread, load detail, mark seen for current viewer.

3. Validation and UX:
- Disable "Ask Question" when required fields are empty (`ticket`, `title`, `body`, `viewerID`).
- Inline error if viewer mode is not human (or auto-set to human for this action).
- Keep existing reply composer unchanged.

## Design Decisions

1. Use websocket push with Redis-backed event fanout, not timer-based DB polling.
Rationale: aligns stream behavior with event architecture already used by forum runtime.

2. Keep websocket endpoint contract stable (`/api/v1/forum/stream`).
Rationale: avoids client migration risk while replacing server internals.

3. Refresh UI only on semantic event frames.
Rationale: prevents heartbeat-triggered query storms.

4. Implement human ask in UI using existing `POST /api/v1/forum/threads`.
Rationale: missing UX path, not missing backend primitive.

5. Keep server-side validation strict for thread open fields.
Rationale: avoids silently writing low-quality forum events.

## Alternatives Considered

1. Increase websocket poll interval (e.g. 5-10s) and keep current model.
Rejected: reduces load but still polls and still refreshes on no-op heartbeat frames.

2. Replace websocket with SSE and keep polling in backend.
Rejected: transport swap alone does not solve poll amplification.

3. Ask humans to use CLI (`metawsm forum ask --actor-type human`) instead of UI.
Rejected: does not address reported forum UX bug.

## Implementation Plan

### Phase 1: Stream correctness and load reduction
1. Introduce forum event broker in `internal/server` with ticket-filtered subscriptions.
2. Feed broker from Redis forum event topics and include bounded reconnect/backoff.
3. Update websocket stream handler to send catch-up + live fanout (no tight polling loop).
4. Update stream tests in `internal/server/api_test.go` for event frames and heartbeat semantics.

### Phase 2: UI stream behavior
1. Update websocket `onmessage` in `ui/src/App.tsx` to branch by frame type.
2. Ignore heartbeat for refresh.
3. Debounce event refresh and stop refreshing debug snapshot on every frame.

### Phase 3: Human ask workflow
1. Add ask composer UI and submit handler (`POST /api/v1/forum/threads`).
2. Connect actor identity to viewer context (`human` + `viewerID`).
3. Add UI tests for successful thread creation and required-field validation.

### Phase 4: Validation and rollout
1. Measure API call rate before/after with at least one idle connected client.
2. Manual check: human can open thread, then reply, and thread shows immediately via stream.
3. Update forum docs/playbook with new ask workflow and stream expectations.

## Open Questions

1. Should websocket filter support `run_id` directly in V1 of this fix, or ticket-only first?
2. Should UI apply optimistic thread insertion on successful open, or rely only on event fanout?
3. Do we want a hard max refresh cadence in UI to protect against bursty events?

## References

- `internal/server/websocket.go`
- `internal/server/api.go`
- `internal/server/api_test.go`
- `internal/forumbus/runtime.go`
- `internal/serviceapi/core.go`
- `ui/src/App.tsx`
