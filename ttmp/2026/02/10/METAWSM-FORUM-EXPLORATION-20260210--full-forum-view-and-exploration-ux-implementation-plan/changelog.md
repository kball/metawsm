# Changelog

## 2026-02-10

- Initial workspace created


## 2026-02-10

Created comprehensive forum implementation and UX exploration specification, including queue semantics for unseen/unanswered, API contracts, and phased rollout plan.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/10/METAWSM-FORUM-EXPLORATION-20260210--full-forum-view-and-exploration-ux-implementation-plan/design-doc/01-full-forum-implementation-specification.md — Core implementation plan and architecture decisions.
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/10/METAWSM-FORUM-EXPLORATION-20260210--full-forum-view-and-exploration-ux-implementation-plan/reference/01-forum-ux-exploration-and-acceptance-scenarios.md — Acceptance criteria and quick-reference behavior matrix.


## 2026-02-10

Resolved forum open questions: role-qualified viewer identity model, implicit mark-seen, session-only filters, and strict unanswered queue semantics.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/10/METAWSM-FORUM-EXPLORATION-20260210--full-forum-view-and-exploration-ux-implementation-plan/design-doc/01-full-forum-implementation-specification.md — Captured resolved decisions and updated open questions.
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/10/METAWSM-FORUM-EXPLORATION-20260210--full-forum-view-and-exploration-ux-implementation-plan/reference/01-forum-ux-exploration-and-acceptance-scenarios.md — Aligned queue/search/mark-seen acceptance rules with resolved decisions.


## 2026-02-10

Implemented full forum exploration stack: seen/unanswered data model, store queue/search APIs, service orchestration methods, new HTTP endpoints (/search, /queues, /threads/{id}/seen), API/store tests, and UI explorer/detail/queue workflows with diagnostics linkage.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/model/forum.go — New viewer/queue/search/seen domain types and thread badge fields.
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_forum.go — Service methods for queue/search/seen and thread timeline detail.
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/server/api.go — New API routes and request parsing for search/queues/seen.
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/server/api_test.go — API coverage for new routes and mark-seen behavior.
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite.go — Schema additions for forum_thread_reads and forum_thread_queue_view.
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite_forum.go — Store methods for search, queue listing, seen writes, and queue projection refresh.
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite_test.go — Store coverage for unseen/unanswered semantics and seen idempotency.
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/10/METAWSM-FORUM-EXPLORATION-20260210--full-forum-view-and-exploration-ux-implementation-plan/playbook/01-forum-rollout-notes-and-operator-validation-checklist.md — Rollout and operator acceptance checklist for feature validation.
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ui/src/App.tsx — Explorer/detail UI with queue tabs, timeline, composer, and implicit mark-seen.
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ui/src/styles.css — Layout/styling updates for full forum exploration UX.
