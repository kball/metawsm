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

