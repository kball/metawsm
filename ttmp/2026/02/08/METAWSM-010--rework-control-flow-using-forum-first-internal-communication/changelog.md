# Changelog

## 2026-02-08

- Initial workspace created


## 2026-02-09

Updated forum-first control-flow migration plan to remove compatibility bridge/dual-write and use full cutover semantics.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-010--rework-control-flow-using-forum-first-internal-communication/design-doc/01-plan-replace-internal-communication-with-metawsm-008-forum.md — Aligned phases and rollout modes with full migration stance


## 2026-02-09

Aligned forum-first migration plan with resolved decisions: one control thread, remove metawsm guide, no soak/canary, no kill-switch or compatibility mode.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-010--rework-control-flow-using-forum-first-internal-communication/design-doc/01-plan-replace-internal-communication-with-metawsm-008-forum.md — Updated implementation and rollout controls to hard cutover semantics


## 2026-02-09

Replaced placeholder tasks with a concrete 17-task full-cutover backlog (forum-first only, one control thread per run+agent, no back-compat path).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-010--rework-control-flow-using-forum-first-internal-communication/tasks.md — Defines implementation-ordered work items for Redis/Watermill and control-flow migration


## 2026-02-09

Step 1: Implemented versioned forum control payload schema and one-thread-per-(run,agent) control mapping (commit dcd11d1a95b0ed1a0946ac2d397ff5d15ebd8367).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/model/forum_control.go — New control payload contract and validation
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_forum.go — New control signal append API and deterministic control thread enforcement
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite_forum.go — Control thread mapping persistence and lookup


## 2026-02-09

Step 2: Added forum bus runtime package and durable SQLite outbox primitives for publish/process flow (commit 22391b48b2ebb81845129d5f7c541b854a6f05cd).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/forumbus/runtime.go — Adds runtime lifecycle
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite.go — Adds forum_outbox schema table and index
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite_forum.go — Adds outbox enqueue/claim/sent/failed APIs

