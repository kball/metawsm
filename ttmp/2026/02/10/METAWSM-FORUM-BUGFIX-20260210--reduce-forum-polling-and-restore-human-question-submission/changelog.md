# Changelog

## 2026-02-10

- Initial workspace created


## 2026-02-10

Created initial design to replace timer-based forum stream polling with Redis-driven websocket fanout and to add a human ask composer in forum UI.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/server/websocket.go — Current stream polling implementation analyzed for load behavior
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/10/METAWSM-FORUM-BUGFIX-20260210--reduce-forum-polling-and-restore-human-question-submission/design-doc/01-fix-plan-event-driven-forum-stream-and-human-ask-flow.md — Primary design artifact for both reported forum bugs
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ui/src/App.tsx — Current UI behavior analyzed for refresh and missing human question submission path


## 2026-02-10

Step 1: Added server-side forum event broker with ticket/run filtered fanout and tests (commit 9b46c10cb406de50d95f8cc03e4aca8c5010dad0).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/server/forum_event_broker.go — New broker primitive for non-blocking ticket/run scoped event fanout
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/server/forum_event_broker_test.go — Unit tests validating broker matching and delivery behavior
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/10/METAWSM-FORUM-BUGFIX-20260210--reduce-forum-polling-and-restore-human-question-submission/reference/01-diary.md — Diary entry for step 1 implementation and validation

