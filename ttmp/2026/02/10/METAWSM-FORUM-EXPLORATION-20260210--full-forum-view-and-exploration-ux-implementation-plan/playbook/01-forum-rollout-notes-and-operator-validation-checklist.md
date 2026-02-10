---
Title: Forum rollout notes and operator validation checklist
Ticket: METAWSM-FORUM-EXPLORATION-20260210
Status: active
Topics:
    - core
    - backend
    - gui
    - chat
    - websocket
DocType: playbook
Intent: long-term
Owners: []
RelatedFiles:
    - Path: internal/server/api.go
      Note: Search/queue/seen API contracts validated in rollout.
    - Path: internal/server/api_test.go
      Note: Coverage for new endpoints and request parsing.
    - Path: internal/store/sqlite_forum.go
      Note: Queue and seen semantics implementation used by rollout validation.
    - Path: ui/src/App.tsx
      Note: Explorer/detail/queue UI behaviors covered by checklist.
ExternalSources: []
Summary: Operator rollout notes and acceptance checks for forum search, queues, seen tracking, and explorer/detail UX.
LastUpdated: 2026-02-10T08:57:00-08:00
WhatFor: Validate and roll out the full forum exploration and response workflow without regressing queue semantics.
WhenToUse: Use after deploys or during release candidates that include forum API/UI changes.
---


# Forum rollout notes and operator validation checklist

## Purpose

Roll out and validate the full forum exploration feature set:
- Search + filter explorer
- Unseen and unanswered queues
- Implicit mark-seen behavior
- Thread timeline and human response composer
- Diagnostics linkage to forum debug health

## Environment Assumptions

- Working directory is the `metawsm` repo root.
- A forum-enabled runtime is running with API server + UI.
- At least one run/ticket has forum activity (threads, posts, state changes).
- Operator has a human viewer identity (`viewer_type=human`, `viewer_id=human:<login>`).

## Commands

```bash
# 1) Backend test gate
go test ./...

# 2) UI typecheck gate
npm --prefix ui run check

# 3) Start or restart metawsm daemon (example)
go run ./cmd/metawsm restart

# 4) Validate search API (replace ticket/run/viewer as needed)
curl -s "http://127.0.0.1:7777/api/v1/forum/search?ticket=METAWSM-011&run_id=run-123&query=validation&viewer_type=human&viewer_id=human:kball&limit=20"

# 5) Validate unseen queue
curl -s "http://127.0.0.1:7777/api/v1/forum/queues?type=unseen&ticket=METAWSM-011&viewer_type=human&viewer_id=human:kball&limit=20"

# 6) Validate unanswered queue
curl -s "http://127.0.0.1:7777/api/v1/forum/queues?type=unanswered&ticket=METAWSM-011&viewer_type=human&viewer_id=human:kball&limit=20"

# 7) Mark a thread seen (idempotent)
curl -s -X POST "http://127.0.0.1:7777/api/v1/forum/threads/<thread-id>/seen" \
  -H "Content-Type: application/json" \
  -d '{"viewer_type":"human","viewer_id":"human:kball","last_seen_event_sequence":0}'
```

## Exit Criteria

1. `go test ./...` passes.
2. `npm --prefix ui run check` passes.
3. `GET /api/v1/forum/search` returns filtered threads and includes badge fields (`is_unseen`, `is_unanswered`).
4. `GET /api/v1/forum/queues?type=unseen` and `type=unanswered` return expected queue subsets.
5. `POST /api/v1/forum/threads/{id}/seen` is idempotent and does not regress sequence values.
6. In UI:
   - Explorer shows queue tabs and counters.
   - Selecting a thread renders timeline and metadata.
   - Reply composer posts human responses.
   - Opening a thread clears unseen state for the active viewer.
   - Diagnostics warning links to debug health panel when bus/outbox is degraded.

## Notes

- Queue semantics are strict by design:
  - `unseen`: latest event sequence exceeds viewer last-seen sequence.
  - `unanswered`: state is `new|waiting_human|waiting_operator` and latest non-system actor is `agent` with no newer human/operator action.
- Viewer identity must remain stable to get meaningful unseen behavior across sessions.
