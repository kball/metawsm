---
Title: Current Forum API and UI Analysis
Ticket: IMPROVE_FORUM
Status: active
Topics:
    - forum
    - ui
    - api
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: internal/orchestrator/service_forum.go
      Note: Forum command orchestration
    - Path: internal/server/api.go
      Note: HTTP forum API route registration and request handlers
    - Path: internal/server/forum_event_broker.go
      Note: Live event fanout behavior and per-subscriber filtering
    - Path: internal/server/websocket.go
      Note: WebSocket stream protocol including catch-up and heartbeat frames
    - Path: internal/serviceapi/remote.go
      Note: Remote API client methods mapping forum operations to endpoints
    - Path: internal/store/sqlite_forum.go
      Note: Queue/search semantics and read-state persistence
    - Path: ui/src/App.tsx
      Note: Current Forum Explorer UI areas
ExternalSources: []
Summary: Current-state analysis of forum HTTP/WebSocket APIs, forum data semantics, and the existing Forum Explorer UI areas and flows.
LastUpdated: 2026-02-10T13:37:35-08:00
WhatFor: Understand how the forum currently works end-to-end before planning IMPROVE_FORUM changes.
WhenToUse: Use when scoping API/UI improvements, debugging forum behavior, or mapping operator/human guidance workflows.
---


# Current Forum API and UI Analysis

## Goal

Document the current forum API surface and Forum Explorer UI so IMPROVE_FORUM can prioritize changes against actual behavior, not assumptions.

## Context

The forum is daemon-backed (`metawsm serve`) and exposed via HTTP + WebSocket APIs. CLI commands and the React UI both call the same API layer. Forum state is event-driven (commands -> events -> projections), with queue semantics (`unseen`, `unanswered`) derived from projected thread/event/read models.

## Quick Reference

### 1) Forum API Surface (Current)

Registered routes are in `internal/server/api.go` and `internal/server/websocket.go`.

| Area | Endpoint(s) | Purpose | Used by current UI? |
|---|---|---|---|
| Runs | `GET /api/v1/runs`, `GET /api/v1/runs/{run_id}` | Run selection and run snapshot lookup | `GET /runs` yes, `GET /runs/{id}` no |
| Thread collection | `GET /api/v1/forum/threads`, `POST /api/v1/forum/threads` | List/open threads | open yes, list no |
| Thread detail/actions | `GET /api/v1/forum/threads/{thread_id}` and `POST .../posts|assign|state|priority|close|seen` | Inspect thread and mutate thread state | detail/posts/seen yes; assign/state/priority/close no |
| Search/queues | `GET /api/v1/forum/search`, `GET /api/v1/forum/queues` | Search and queue-focused lists | yes |
| Control signal | `POST /api/v1/forum/control/signal` | Append typed lifecycle/control payloads | no (CLI/automation path) |
| Event polling | `GET /api/v1/forum/events` | Cursor-based event watch | no |
| Stats/debug | `GET /api/v1/forum/stats`, `GET /api/v1/forum/debug` | Aggregates and runtime health | debug yes, stats no |
| Live stream | `GET /api/v1/forum/stream` (WebSocket upgrade) | Catch-up + live forum event frames + heartbeat | yes |

### 2) Major UI Areas (Forum Explorer)

The UI is a single-page React app (`ui/src/App.tsx`) with four major panels:

1. Runs panel
- Lists runs from `/api/v1/runs`.
- Selecting a run sets `selectedRunID`, and the first run ticket becomes the active ticket context.
- Enables scoping all subsequent forum operations to a run/ticket.

2. Threads Explorer panel
- Queue tabs: `All`, `Unseen`, `Unanswered`.
- Filters: query text, state, priority, assignee.
- Viewer identity controls: `viewer_type`, `viewer_id`.
- Uses `/api/v1/forum/search` for `All` and `/api/v1/forum/queues` for queue tabs.
- Enables triage and backlog slicing, not just raw listing.

3. Thread Detail panel
- `Ask as Human` composer opens a thread (`POST /api/v1/forum/threads`).
- Thread timeline view merges event stream + matched posts from `GET /api/v1/forum/threads/{id}`.
- `Respond as Human` composer posts replies (`POST /posts`).
- Seen marker is written with `POST /seen`.
- Enables question/answer workflows and event-by-event reconstruction per thread.

4. Forum Debug Health panel
- Polls `GET /api/v1/forum/debug` every 15s.
- Shows bus health, outbox backlog, topic subscription state, and stream lag-like indicators.
- Enables runtime diagnostics when list/search freshness appears stale.

### 3) Current Flow Map

Flow A: Initial load and run scoping
1. UI loads and calls `/api/v1/runs`.
2. First run auto-selected if none selected.
3. Ticket from selected run scopes thread/search/queue calls.

Flow B: Queue triage and search
1. User picks tab (`all|unseen|unanswered`) and filter controls.
2. UI calls `/forum/search` (all) or `/forum/queues` (queue tabs), plus two queue calls for counters.
3. User selects thread from list.

Flow C: Thread inspection and read tracking
1. UI calls `/forum/threads/{id}` for detail.
2. UI immediately posts `/forum/threads/{id}/seen`.
3. Queue badges update as seen state changes.

Flow D: Human asks new question
1. `Ask as Human` submits `POST /forum/threads`.
2. UI refreshes list/detail and marks created thread seen.
3. New thread appears in `All`, potentially queue tabs based on derived flags.

Flow E: Human replies
1. `Respond as Human` submits `POST /forum/threads/{id}/posts`.
2. UI refreshes list/detail and seen marker.
3. Thread ordering and queue flags may change.

Flow F: Live updates
1. UI opens WebSocket `/api/v1/forum/stream?ticket=...` (and run filter where applicable).
2. Server sends catch-up `forum.events` frame, then live `forum.events` and `heartbeat`.
3. UI debounces refresh (~150ms) and reloads list/detail.

### 4) Derived Queue Semantics (Important)

Queue calculations are SQL-derived in `internal/store/sqlite_forum.go`, not hardcoded in UI:

- `is_unseen`: last thread event sequence is greater than viewer’s last seen sequence.
- `is_unanswered`: state in `new|waiting_human|waiting_operator` AND agent has posted more recently than human/operator, with last non-system actor = agent.

This is why viewer identity (`viewer_type`, `viewer_id`) materially changes queue output.

### 5) API Capabilities vs Current UI Coverage

Available in API but not exposed in current web UI controls:
- Assign thread (`POST /assign`)
- Change state (`POST /state`)
- Change priority (`POST /priority`)
- Close thread (`POST /close`)
- Submit control signals (`POST /forum/control/signal`)
- Poll raw events (`GET /forum/events`)
- Read stats (`GET /forum/stats`)
- Direct list endpoint (`GET /forum/threads`)

Interpretation: the current UI is optimized for exploration + human ask/reply + diagnostics, but not full lifecycle/thread administration.

## Usage Examples

### Example: planning IMPROVE_FORUM scope

Use this document to separate work into:
1. UI enhancements over already-available APIs (assign/state/priority/close controls).
2. New API work where no capability exists.
3. Queue UX updates that must preserve SQL-derived unseen/unanswered semantics.

### Example: debugging “thread not leaving unanswered”

Check:
1. Last non-system actor type in thread events.
2. Whether a human/operator post happened after latest agent post.
3. Thread state is still in `new|waiting_human|waiting_operator`.

## Related

- `docs/how-to-run-forum.md`
- `README.md` (API summary + serve/UI run instructions)
