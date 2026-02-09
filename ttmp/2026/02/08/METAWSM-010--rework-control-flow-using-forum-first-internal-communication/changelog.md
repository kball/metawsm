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


## 2026-02-09

Step 3: Refactored forum command entrypoints to bus-backed dispatch and registered command topic handlers (commit d78deff127187700d4b1424721c0c66ade1b8c34).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/forum_dispatcher.go — Dispatcher abstraction and publish/process semantics
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_forum.go — Forum command methods now dispatch to bus topics


## 2026-02-09

Step 4: Migrated runtime lifecycle to forum-only control signals, removed metawsm guide CLI, and removed legacy file-signal runtime paths (commit 7d5712a2d4b3edb1934363a82e95d5503e145d4f).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Replaces guide command surface with forum signal command
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/model/types.go — Removes obsolete file-signal payload types
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Guide/syncBootstrapSignals/close checks now forum-first
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_forum_control.go — Introduces control-state derivation from forum control posts


## 2026-02-09

Step 5: Added forum.events projection consumers with idempotent forum_projection_events markers and introduced typed RunSnapshot API for watch/operator (commit f589f30d952f32ef3ab4020f66ebf0e3f062b8d0).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Removes runtime dependence on status-text parsing for watch snapshots
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_forum.go — Publishes command-side events to forum.events topics and registers projection consumers
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_snapshot.go — Adds typed run snapshot API consumed by watch/operator
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite_forum.go — Adds projection marker idempotency and thread-view rebuild


## 2026-02-09

Step 6: Updated README/system-guide to forum-first signaling, added outage/replay/e2e lifecycle tests, and executed cutover checklist verification (commit 99d39da2dc7adf6a8fd64f6db5632fd841985e6e).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/README.md — Forum-first command docs and control-signal examples
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/docs/system-guide.md — System guide updated to forum-only signaling contract
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/forumbus/runtime_test.go — Redis outage and outbox replay recovery tests
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Forum-only lifecycle e2e test
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite_test.go — Projection replay catch-up test

