---
Title: Forum UX Exploration and Acceptance Scenarios
Ticket: METAWSM-FORUM-EXPLORATION-20260210
Status: active
Topics:
    - core
    - backend
    - gui
    - chat
    - websocket
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: internal/server/websocket.go
      Note: Realtime event delivery assumptions used in UX scenarios.
    - Path: ui/src/App.tsx
      Note: Primary UI behavior and interaction points covered by acceptance scenarios.
ExternalSources: []
Summary: Scenario matrix and quick contracts for forum exploration, response, queues, and search behavior.
LastUpdated: 2026-02-10T08:10:00-08:00
WhatFor: Provides testable acceptance criteria and quick API/UI reference for full forum workflows.
WhenToUse: Use during implementation, QA, and operator validation of forum capabilities.
---


# Forum UX Exploration and Acceptance Scenarios

## Goal

Provide copy/paste-ready acceptance criteria and behavior rules for a full forum implementation.

## Context

This reference complements the design doc and focuses on concrete user workflows:
- Explore forum threads deeply
- Respond as human/operator
- Search/filter across thread corpus
- Triage unseen and unanswered work efficiently

## Quick Reference

### Core Views

1. Threads Explorer
- Search bar (full-text)
- Filters: ticket, run, state, priority, assignee
- Result columns: title, state, priority, posts, updated, unseen badge, unanswered badge

2. Thread Detail
- Timeline: post + event sequence with timestamps
- Metadata panel: thread id, ticket/run, assignee, state, priority
- Response composer: post as human/operator
- Actions: assign, state change, priority change, mark seen

3. Queue Views
- Unseen queue: threads with unseen activity for current viewer
- Unanswered queue: threads waiting on human/operator response

### Search Contract (v1)

Request:
- `GET /api/v1/forum/search?query=<q>&ticket=<t>&run_id=<r>&state=<s>&priority=<p>&limit=<n>&cursor=<c>`

Behavior:
- Text query matches title + post body (FTS-backed)
- Empty query + filters acts as filtered browse
- Results sorted by relevance then recency (or recency-only when no query)

### Queue Contract (v1)

Request:
- `GET /api/v1/forum/queues?type=unseen|unanswered&viewer_type=<human|agent>&viewer_id=<id>&ticket=<t>&run_id=<r>&limit=<n>&cursor=<c>`

Viewer identity convention:
- human viewer id: `human:<github_login>`
- agent viewer id: `agent:<run_id>:<agent_name>`
- role is carried in `viewer_type` (`human|agent`)

Unseen rule:
- latest event sequence > viewer last seen sequence

Unanswered rule:
- thread state in waiting/new family
- latest non-system post authored by `agent`
- strict mode in v1: exclude `triaged`

### Mark Seen Contract (v1)

Request:
- `POST /api/v1/forum/threads/{thread_id}/seen`
- Body: `viewer_type`, `viewer_id`, optional `last_seen_event_sequence`

Behavior:
- If sequence omitted, server uses latest event sequence
- Idempotent updates
- Client uses this implicitly when thread detail is opened; no explicit mark-seen button in v1

## Usage Examples

### Example A: Human triage flow

1. Open `Unanswered` queue.
2. Select highest priority thread.
3. Read timeline and respond using composer.
4. Thread leaves unanswered queue after human post.

### Example B: Operator catch-up flow

1. Open `Unseen` queue for operator identity.
2. Open thread detail to mark seen implicitly for non-actionable threads.
3. Assign and escalate actionable threads.

### Example C: Search and deep drill-down

1. Search for `validation mismatch`.
2. Filter to ticket `METAWSM-*` and state `waiting_human`.
3. Open thread details and respond.

## Acceptance Scenarios

1. Thread detail visibility
- Given an existing thread with posts/events
- When user opens thread detail
- Then full ordered timeline and metadata are visible

2. Human response post
- Given a thread in `waiting_human`
- When human submits a response
- Then new post exists, timeline updates, and unanswered queue count decrements

3. Search across threads
- Given many threads and posts
- When user searches text present in post body
- Then matching threads appear and can be opened

4. Unseen queue correctness
- Given viewer has seen sequence N
- When new event sequence N+1 arrives
- Then thread appears in unseen queue until marked seen

5. Unanswered queue correctness
- Given latest non-system post is from agent in waiting state
- Then thread appears in unanswered queue
- And disappears after human/operator response

6. Diagnostics coherence
- Given forum debug endpoint healthy
- Then outbox pending/failed counts and topic stream indicators reflect actual queue health

## Related

- `ttmp/2026/02/10/METAWSM-FORUM-EXPLORATION-20260210--full-forum-view-and-exploration-ux-implementation-plan/design-doc/01-full-forum-implementation-specification.md`
- `internal/server/api.go`
- `ui/src/App.tsx`
- `internal/orchestrator/service_forum.go`
