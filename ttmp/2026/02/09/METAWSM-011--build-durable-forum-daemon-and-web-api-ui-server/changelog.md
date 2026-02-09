# Changelog

## 2026-02-09

- Initial workspace created


## 2026-02-09

Created METAWSM-011 by combining forum runtime constraints with the unmerged web UI/API plan into a durable metawsm serve daemon+API+web architecture document, including phased rollout and non-committed frontend artifact policy.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/09/METAWSM-011--build-durable-forum-daemon-and-web-api-ui-server/design-doc/01-design-durable-forum-daemon-and-web-api-server.md — Primary design for durable daemon plus web API and UI
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/09/METAWSM-011--build-durable-forum-daemon-and-web-api-ui-server/tasks.md — Execution backlog for implementation phases


## 2026-02-09

Resolved prior open questions for METAWSM-011: WebSocket-only live updates, localhost trust scope, internal shared service API for CLI+HTTP, mandatory daemon mode for forum workflows, and optional structured file sync logging.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/09/METAWSM-011--build-durable-forum-daemon-and-web-api-ui-server/design-doc/01-design-durable-forum-daemon-and-web-api-server.md — Records resolved architectural decisions and updated rollout constraints
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/09/METAWSM-011--build-durable-forum-daemon-and-web-api-ui-server/tasks.md — Updated implementation backlog to reflect the resolved decisions

## 2026-02-09

Step 1: implemented serve daemon command + durable forum worker loop with health/outbox metrics (commit b553d46).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/server/runtime.go — Introduces daemon runtime and /api/v1/health
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/server/worker.go — Adds continuous ProcessOnce loop with metrics
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/09/METAWSM-011--build-durable-forum-daemon-and-web-api-ui-server/reference/01-diary.md — Step 1 implementation diary


## 2026-02-09

Step 2: added shared internal service API layer and refactored forum CLI + daemon runtime to consume it (commit b251e84).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Forum commands switched to new serviceapi
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/serviceapi/core.go — Core service abstraction used by CLI and server
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/09/METAWSM-011--build-durable-forum-daemon-and-web-api-ui-server/reference/01-diary.md — Step 2 diary details


## 2026-02-09

Step 3: implemented run/forum HTTP API routes and WebSocket forum event stream on the daemon runtime (commit 15b13fe).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/server/api.go — Primary HTTP route and request/response handling
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/server/websocket.go — Native WebSocket stream implementation for forum events
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/09/METAWSM-011--build-durable-forum-daemon-and-web-api-ui-server/reference/01-diary.md — Step 3 implementation notes


## 2026-02-09

Step 4: added web UI/embed pipeline, daemon-backed remote service API client, and mandatory daemon mode for forum CLI workflows (commit ed279e1).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/serviceapi/remote.go — Remote forum/run client implementation over daemon HTTP API
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/web/spa.go — Serves SPA alongside API routes
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/09/METAWSM-011--build-durable-forum-daemon-and-web-api-ui-server/reference/01-diary.md — Step 4 implementation details
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ui/src/App.tsx — Initial web dashboard for run/thread visibility


## 2026-02-09

Step 5: added daemon API and websocket regression tests for route behavior and failure modes (commit 4dc9edc).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/server/api_test.go — Route and websocket regression coverage
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/09/METAWSM-011--build-durable-forum-daemon-and-web-api-ui-server/reference/01-diary.md — Step 5 diary details


## 2026-02-09

Step 6: finalized daemon-mode rollout docs/runbooks and marked Task 9 complete (commit e1ac9a2).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/README.md — Updated command surface and daemon/UI quick start
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/docs/how-to-run-forum.md — Daemon-first forum operations runbook
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/docs/system-guide.md — System architecture updated for daemon/shared service API
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/09/METAWSM-011--build-durable-forum-daemon-and-web-api-ui-server/reference/01-diary.md — Step 6 diary details
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/09/METAWSM-011--build-durable-forum-daemon-and-web-api-ui-server/tasks.md — Task checklist now fully complete

