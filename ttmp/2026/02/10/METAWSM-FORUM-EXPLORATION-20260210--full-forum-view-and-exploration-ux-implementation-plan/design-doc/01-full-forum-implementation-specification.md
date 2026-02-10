---
Title: Full Forum Implementation Specification
Ticket: METAWSM-FORUM-EXPLORATION-20260210
Status: active
Topics:
    - core
    - backend
    - gui
    - chat
    - websocket
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: internal/orchestrator/service_forum.go
      Note: Service entry points and behavior constraints for queue/search/detail operations.
    - Path: internal/server/api.go
      Note: API surface and request/response wiring for explorer and queue endpoints.
    - Path: internal/store/sqlite_forum.go
      Note: Data model
ExternalSources: []
Summary: End-to-end architecture and delivery plan for a complete forum exploration and response experience.
LastUpdated: 2026-02-10T08:10:00-08:00
WhatFor: Defines product scope, data model, APIs, and UI delivery strategy for full forum workflows.
WhenToUse: Use when implementing forum features, reviewing scope, or planning rollout milestones.
---


# Full Forum Implementation Specification

## Executive Summary

Build a complete forum experience in metawsm with:
- Thread detail and timeline views
- Human response workflows directly in UI
- Full-text search and structured filters across threads
- Operator views for unseen and unanswered work queues
- Explicit read-state tracking and answerability semantics

The implementation extends existing forum primitives (`forum_threads`, `forum_posts`, `forum_events`, control signals, outbox bus) rather than replacing them.

## Problem Statement

Current forum capabilities are functional but incomplete for day-to-day exploration and operations:
- Users can list threads/events but cannot deeply explore a thread in the UI.
- Human responses are available by CLI/API, but the web experience does not provide first-class response composition.
- Search across all threads is limited.
- There is no robust concept of unseen and unanswered queues per operator/human user.
- Exploration UX and queue semantics are fragmented.

This blocks practical usage as ticket volume and thread volume grow.

## Proposed Solution

### 1. Domain Extensions

Add explicit read-state and queue semantics.

- New table: `forum_thread_reads`
  - `thread_id`, `viewer_type`, `viewer_id`, `last_seen_event_sequence`, `updated_at`
  - `viewer_type` values: `human|agent`
  - canonical `viewer_id` scheme:
    - human viewer: `human:<github_login>`
    - agent viewer: `agent:<run_id>:<agent_name>`
  - this allows agent and human read state to remain separate even when both use the same GitHub login credentials underneath
- Optional projection table: `forum_thread_queue_view`
  - denormalized fields for queue filtering (`is_unseen`, `is_unanswered`, `last_actor_type`, `last_event_sequence`)

Definitions:
- `unseen` for viewer V:
  - latest thread event sequence > `last_seen_event_sequence` for V
- `unanswered` for human/operator queue:
  - thread state is `new|waiting_human|waiting_operator`
  - last non-system actor is `agent`
  - no later post from `human|operator`
  - strict mode: do not include `triaged`

### 2. Backend/Service Capabilities

Add service methods:
- `ForumMarkThreadSeen(thread_id, viewer_type, viewer_id, last_seen_sequence)`
- `ForumSearchThreads(query, filters, viewer_context, limit, cursor)`
- `ForumListQueue(queue_type=unseen|unanswered, viewer_context, filters)`

Queue API supports:
- ticket/run filters
- priority and state filters
- assignee filters
- pagination

### 3. API Endpoints

Add/extend endpoints:
- `GET /api/v1/forum/search`
  - query, ticket/run filters, state/priority filters, cursor/limit
- `GET /api/v1/forum/queues`
  - `type=unseen|unanswered`, viewer metadata, standard filters
- `POST /api/v1/forum/threads/{id}/seen`
  - marks thread/event sequence as seen
  - called implicitly by the client when thread detail is opened (not a user-visible explicit action in v1)
- Existing endpoints continue as source of truth:
  - `GET /api/v1/forum/threads/{id}`
  - `POST /api/v1/forum/threads/{id}/posts` for human response

### 4. UI/UX

Add dedicated forum screens:
- `Threads Explorer`
  - searchable list, chips for state/priority/ticket/run
  - quick badges for unseen/unanswered counts
- `Thread Detail`
  - full timeline (posts + state changes + assignments + control events)
  - response composer (human/operator)
  - implicit mark-seen on open (no explicit mark-seen button in v1)
- `Queue Views`
  - `Unseen` tab
  - `Unanswered` tab
  - bulk-safe actions (mark seen, assign, set state)
  - filter state is session-only in v1 (no server-side saved views)

### 5. Diagnostics Integration

Keep diagnostics visible in explorer context:
- link thread operations to `/api/v1/forum/debug` health indicators
- show warning banners when bus unhealthy or outbox backlog grows
- keep stream debug panel for operator troubleshooting

## Design Decisions

### Decision 1: Incremental extension over rewrite
Use existing forum/outbox/event infrastructure. This minimizes migration risk and preserves tested code paths.

### Decision 2: Read-state is per viewer
Unseen cannot be globally inferred. Add explicit viewer-scoped state for correctness.

### Decision 3: Unanswered is a derived queue
Do not manually label unanswered; compute from latest events/state to avoid drift.

### Decision 4: API-first with shared contracts
CLI and UI consume the same queue/search/detail semantics through API contracts.

### Decision 5: Viewer identity is role-qualified, not auth-qualified
Read-state identity is keyed by forum actor role and runtime identity, not only by GitHub credential identity.
- human UI principal: `viewer_type=human`, `viewer_id=human:<github_login>`
- agent principal: `viewer_type=agent`, `viewer_id=agent:<run_id>:<agent_name>`

### Decision 6: Mark-seen is implicit in v1
Thread read state is updated automatically when a thread detail view is opened.

### Decision 7: Filters are session-only in v1
Explorer and queue filter choices live in client session state only.

### Decision 8: Unanswered queue remains strict
Only `new|waiting_human|waiting_operator` states with latest non-system agent post are included.

## Alternatives Considered

1. Keep only event stream + client-side derivation
- Rejected: expensive and inconsistent across clients; difficult pagination semantics.

2. Build a separate search datastore
- Deferred: overkill at current scale; SQLite FTS + projections are sufficient initially.

3. Treat unanswered as manual state transitions only
- Rejected: drifts from actual conversation state and increases operator burden.

## Implementation Plan

### Phase 1: Data and Service Foundations
1. Add `forum_thread_reads` schema and store APIs.
2. Add queue derivation and search helpers in store/service.
3. Add tests for unseen/unanswered semantics.

### Phase 2: API Surface
1. Add `/forum/search`, `/forum/queues`, `/threads/{id}/seen`.
2. Add request/response types in `internal/server/api.go`.
3. Add server tests for filtering, pagination, and queue correctness.

### Phase 3: UI Delivery
1. Replace single-pane thread list with explorer + detail split.
2. Add response composer in thread detail.
3. Add unseen/unanswered tabs and counters.
4. Integrate websocket refresh with optimistic seen-state updates.

### Phase 4: Hardening and Rollout
1. Add metrics and warning thresholds (outbox lag, unhealthy bus, stale queues).
2. Add migration notes and operator playbook.
3. Roll out behind a feature flag if needed, then default on.

## Open Questions

1. Should we include a secondary `viewer_device_id` in read-state to support per-device unread tracking later?
2. Do we want an optional "mark unread" action in v1.1 for operator triage workflows?

## References

- `ttmp/2026/02/10/METAWSM-FORUM-EXPLORATION-20260210--full-forum-view-and-exploration-ux-implementation-plan/reference/01-forum-ux-exploration-and-acceptance-scenarios.md`
- `internal/orchestrator/service_forum.go`
- `internal/store/sqlite_forum.go`
- `internal/server/api.go`
- `ui/src/App.tsx`
