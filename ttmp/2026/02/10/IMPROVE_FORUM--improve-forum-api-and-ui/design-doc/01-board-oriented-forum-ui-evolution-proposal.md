---
Title: Board-Oriented Forum UI Evolution Proposal
Ticket: IMPROVE_FORUM
Status: active
Topics:
    - forum
    - ui
    - api
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: internal/server/api.go
      Note: Existing forum API endpoints to be reused and expanded
    - Path: internal/store/sqlite_forum.go
      Note: Queue semantics and state/read model powering board logic
    - Path: ttmp/2026/02/10/IMPROVE_FORUM--improve-forum-api-and-ui/reference/01-current-forum-api-and-ui-analysis.md
      Note: Current-state analysis baseline for this proposal
    - Path: ui/src/App.tsx
      Note: Current forum UI structure to evolve into board-based navigation
ExternalSources: []
Summary: Proposal to evolve Forum Explorer into board-oriented views for in-progress visibility, personal action queues, and completed-work review.
LastUpdated: 2026-02-10T14:26:17-08:00
WhatFor: Define a board-first UI model that improves navigation and operational clarity for active forum workflows.
WhenToUse: Use when planning or implementing the next-generation forum UI and any supporting API changes.
---


# Board-Oriented Forum UI Evolution Proposal

## Executive Summary

Evolve the current single explorer into a board-oriented workspace with three primary boards:

1. `In Progress`: quickly understand active work and current blockers.
2. `Needs Me`: quickly find threads that require my attention.
3. `Recently Completed`: review and navigate recently finished threads.

Each board is viewed through topic areas (ticket first, then run/agent, and later optional custom labels) so exploration is scoped and fast. The detail panel remains event-first, but discovery and prioritization move to board views.

## Problem Statement

The current UI is functional but hard to navigate at growing thread volume:

1. It does not provide an immediate, high-signal summary of active work.
2. It does not center personal action queues (`what needs me now`) as a first-class view.
3. It makes completed-work review possible, but not easy to browse as a coherent feed.

The result is slower triage, more manual filtering, and reduced confidence in what to do next.

## Proposed Solution

### Information Architecture

Adopt a 3-layer structure:

1. Global scope bar
- Run selector
- Ticket selector
- Viewer identity (`viewer_type`, `viewer_id`)

2. Board selector (primary navigation)
- `In Progress`
- `Needs Me`
- `Recently Completed`

3. Topic area selector (secondary navigation/filtering)
- Default facets from existing metadata (ordered):
  - ticket (default)
  - run
  - agent
- Optional in later phase:
  - custom topic labels (user-defined board slices)

### Board Definitions

#### Board A: In Progress

Goal solved: quickly understand work currently in progress.

Sections:
1. `New / Triage`
- state = `new`
2. `Active`
- state in `triaged|waiting_operator|waiting_human`
3. `Awaiting Close`
- state = `answered` (not yet closed)

Card content:
- title, priority, state, assignee, last actor, updated time, unread badge

Top metrics:
- open thread count
- urgent/high open count
- age of oldest open thread

#### Board B: Needs Me

Goal solved: find the threads where I need to be involved.

Sections:
1. `Unseen for Me`
- from queue API `type=unseen` using current viewer identity
2. `Needs Human/Operator Response`
- from queue API `type=unanswered`
3. `Assigned to Me`
- filtered by assignee = viewer identity (inferred automatically from viewer context)

Card actions (inline):
- open thread
- mark seen
- respond
- set state / assign / priority (new UI controls over existing APIs)

#### Board C: Recently Completed

Goal solved: see recently completed work and navigate through it.

Sections:
1. `Recently Closed`
- state = `closed`, sorted by latest event sequence/time

Card affordances:
- “Open timeline”
- “Jump to run”
- “Jump to topic area”

### Topic Areas

Use topic areas as scoped “lanes” inside each board:

Phase 1 topic areas (no schema change):
- ticket (default first view)
- run
- agent

Phase 2 topic areas (optional schema/API extension):
- explicit `topic_area` on thread open/update
- saved board filters (for reusable “boards” beyond built-ins)

### UI Layout Evolution

Keep current strengths (detail timeline + diagnostics), but reorder screen hierarchy:

1. Left rail: Board + topic area navigation
2. Center: board cards/list and board metrics
3. Right pane: thread detail/timeline/composer/actions
4. Collapsed diagnostics drawer by default: keep Forum Debug Health available behind a `System Health` panel

### API and Data Impact

Works immediately with many existing endpoints:
- `/api/v1/forum/search`
- `/api/v1/forum/queues`
- `/api/v1/forum/threads/{id}`
- `/api/v1/forum/threads/{id}/seen`
- `/api/v1/forum/threads/{id}/posts`
- `/api/v1/forum/threads/{id}/assign|state|priority|close`

Likely incremental API additions:
- board summary counts in one call (reduce N+1 queue/search calls)
- optional “completed since” filter or endpoint for efficient recent-complete feeds
- optional saved-view/topic-area resources (phase 2+)

## Design Decisions

1. Keep boards task-oriented, not object-oriented.
- Rationale: users think “what needs attention?” before “which thread?”

2. Keep exactly three primary boards in v1.
- Rationale: aligns directly to your three needs and avoids navigation sprawl.

3. Treat topic areas as a secondary filter layer.
- Rationale: preserves a stable mental model while allowing exploration by domain.
Decision: default ordering is ticket first.

4. Reuse existing queue semantics (`unseen`, `unanswered`) as foundational signals.
- Rationale: semantics already implemented in backend projections and tested behavior.

5. Promote assignment/state/priority controls into UI.
- Rationale: APIs already exist; exposing them improves board actionability immediately.

6. Treat `answered` as still in-progress work.
- Rationale: answered threads are often pending final closure and should remain visible in active operations.

7. Infer "Assigned to Me" from viewer identity.
- Rationale: removes repetitive filtering and keeps the personal-action board immediately useful.

8. Keep diagnostics collapsed by default.
- Rationale: system health remains accessible without competing with daily thread navigation.

## Alternatives Considered

1. Keep current single explorer and add more filters only.
- Rejected: improves filtering but not discoverability or “what now” prioritization.

2. Pure Kanban by thread state only.
- Rejected: state-only lanes do not solve personal action discovery (`Needs Me`) well.

3. Separate application for completed-work analytics.
- Rejected: splits context away from live thread navigation and creates tool switching.

## Implementation Plan

### Phase 1: Board Shell on Existing APIs

1. Add board selector and topic-area chips to UI.
2. Implement `In Progress`, `Needs Me`, and `Recently Completed` views using current `/search` and `/queues`.
3. Preserve existing thread detail and composer behavior.

### Phase 2: Actionability Upgrades

1. Add UI controls for assign/state/priority/close (already supported by API).
2. Add optimistic refresh/update for board cards after mutations.
3. Add keyboard shortcuts for “next needs me” and “respond + mark seen”.

### Phase 3: Data Efficiency and Clarity

1. Add board summary API (counts + key metrics) to reduce multiple round-trips.
2. Add efficient recent-completed feed endpoint or filters.
3. Add clearer card-level “why this is here” explanations (e.g., unseen vs unanswered reason).

### Phase 4: Custom Topic Boards (Optional)

1. Optional thread `topic_area` metadata.
2. Allow pinning favorite boards.

### Success Criteria

1. Time to identify highest-priority in-progress items is materially reduced.
2. Time to find first thread requiring user action is reduced.
3. Completed-thread review and navigation usage increases.

## Open Questions

No blocking open questions at this stage from product direction. Remaining details are implementation-specific.

## References

- `ttmp/2026/02/10/IMPROVE_FORUM--improve-forum-api-and-ui/reference/01-current-forum-api-and-ui-analysis.md`
- `ttmp/2026/02/10/METAWSM-FORUM-EXPLORATION-20260210--full-forum-view-and-exploration-ux-implementation-plan/design-doc/01-full-forum-implementation-specification.md`
- `internal/server/api.go`
- `internal/store/sqlite_forum.go`
- `ui/src/App.tsx`
