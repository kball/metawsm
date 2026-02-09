---
Title: 'Design: durable forum daemon and web API server'
Ticket: METAWSM-011
Status: active
Topics:
    - backend
    - chat
    - gui
    - websocket
    - core
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: docs/how-to-run-forum.md
      Note: Documents current non-daemon forum runtime constraints
    - Path: docs/system-guide.md
      Note: Defines system architecture and operator workflow integration points
    - Path: internal/forumbus/runtime.go
      Note: Current Redis and outbox runtime to daemonize
    - Path: internal/orchestrator/forum_dispatcher.go
      Note: Shows synchronous dispatch and ProcessOnce coupling
    - Path: internal/orchestrator/service.go
      Note: Service lifecycle currently starts forum runtime per command
    - Path: internal/orchestrator/service_forum.go
      Note: Forum command and query APIs to expose via HTTP
    - Path: internal/store/sqlite.go
      Note: Forum and outbox schema backing durable server behavior
    - Path: ttmp/2026/02/08/METAWSM-008--build-metawsm-managed-q-a-forum-for-agent-operator-human-collaboration/design-doc/01-implementation-plan-for-metawsm-q-a-forum.md
      Note: Forum architecture baseline
    - Path: ttmp/2026/02/08/METAWSM-010--rework-control-flow-using-forum-first-internal-communication/design-doc/01-plan-replace-internal-communication-with-metawsm-008-forum.md
      Note: Forum-first control flow migration context
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-09T09:03:08.059286-08:00
WhatFor: ""
WhenToUse: ""
---


# Design: durable forum daemon and web API server

## Executive Summary
METAWSM-008 and METAWSM-010 established a forum-first, Redis-backed control flow, but forum workers currently live inside short-lived CLI command processes. METAWSM-006 (web UI/API) was implemented on an unmerged branch and does not exist on `main`, leaving no durable HTTP surface for forum workflows.

This design introduces a long-running `metawsm serve` process that:
- runs forum bus workers continuously (daemon role),
- exposes a stable HTTP/WebSocket API for forum and run data,
- serves a web UI from the same binary in production,
- preserves existing CLI behavior while enabling API-backed clients.

The result is one durable server process for both operational forum reliability and web interface integration.

## Problem Statement
Current behavior is split across two partial solutions:

1. Forum is implemented, but not daemonized
- `docs/how-to-run-forum.md` explicitly states there is no always-on forum daemon.
- `orchestrator.NewService` starts `forumbus.Runtime`, and most forum actions occur in per-command process lifetimes.
- This makes forum throughput and latency dependent on ad-hoc command activity.

2. Web UI/API was planned and prototyped, but not merged into current `main`
- Historical METAWSM-006 artifacts exist on branch `origin/metawsm-006/metawsm/run-20260208-073222` (`internal/webapi`, `internal/web`, `ui/`, `metawsm web` command).
- Current `main` has no web server command and no first-class forum HTTP API.

3. Known suggestion from prior review
- Stored review feedback for METAWSM-006 in `.metawsm/metawsm.db` includes: “please do not commit compiled assets.”
- The durable design should avoid committing generated frontend assets while still supporting embedded production builds.

Without a durable server, forum-first control flow and future web UX remain operationally fragile and hard to extend.

## Proposed Solution
Create a new server runtime and CLI entrypoint:

### 1) New command: `metawsm serve`
- Starts a long-lived process with configurable `--addr`, `--db`, and optional `--policy`.
- Performs startup checks (DB init/migrations, policy/forum config validation, Redis health).
- Runs until SIGINT/SIGTERM with graceful shutdown.

### 2) Durable forum worker loop
- Reuse `internal/forumbus.Runtime` and existing topic/handler registrations.
- Add a managed worker loop that continuously calls `ProcessOnce` with backoff and jitter.
- Publish health state for API and operator diagnostics:
  - runtime started,
  - Redis reachable,
  - outbox depth/oldest pending age,
  - projection lag indicators.

### 3) HTTP API surface (forum-first)
Build a dedicated package (for example `internal/serverapi`) that wraps an internal service API and exposes:

- `GET /api/v1/health`
- `GET /api/v1/runs` / `GET /api/v1/runs/{run_id}`
- `GET /api/v1/forum/threads` (ticket/run/state/priority/assignee filters)
- `GET /api/v1/forum/threads/{thread_id}`
- `POST /api/v1/forum/threads` (ask/open)
- `POST /api/v1/forum/threads/{thread_id}/posts`
- `POST /api/v1/forum/threads/{thread_id}/assign`
- `POST /api/v1/forum/threads/{thread_id}/state`
- `POST /api/v1/forum/threads/{thread_id}/priority`
- `POST /api/v1/forum/threads/{thread_id}/close`
- `POST /api/v1/forum/control/signal`
- `GET /api/v1/forum/events` (cursor-based polling)
- `GET /api/v1/forum/stream` (WebSocket live updates)

All write endpoints should call existing typed service methods (`ForumOpenThread`, `ForumAnswerThread`, `ForumAppendControlSignal`, etc.) to preserve invariants already enforced in `internal/orchestrator/service_forum.go`.

### 3.5) Internal service separation
- Introduce an internal service API layer (for example `internal/servercore`) with interfaces/DTOs for run snapshots and forum operations.
- Both CLI command handlers and HTTP handlers consume this same internal service API.
- Transport and presentation (CLI text, JSON/WS payloads) stay outside the service API boundary.

### 4) Web UI serving model
- Keep `ui/` as Vite React app.
- Development: Vite dev server proxying `/api` to `metawsm serve`.
- Production: `go generate ./internal/web` builds/copies assets, and `go build -tags embed` embeds static files in binary.
- If assets are missing at runtime, API remains available and root returns actionable error.

### 5) CLI compatibility strategy
- Forum daemon mode is mandatory for forum command handling once `metawsm serve` ships.
- CLI commands continue to exist, but they execute against the same internal service API contracts used by HTTP routes.
- Non-forum CLI operations remain available outside daemon mode.

## Design Decisions
1. Introduce one durable process for both daemon + API responsibilities.
- Rationale: one lifecycle, one config source, one health endpoint.

2. Reuse forum domain/service logic, not duplicate business rules in handlers.
- Rationale: avoids drift between CLI and web behaviors.

3. Keep Redis+outbox architecture unchanged.
- Rationale: forum-first reliability work in METAWSM-008/010 already depends on this transport model.

4. Standardize live updates on WebSocket in V1.
- Rationale: one push protocol keeps implementation and client support focused.

5. Scope server trust to localhost for V1.
- Rationale: local-only operation avoids early auth complexity while satisfying current usage goals.

6. Add internal service separation consumed by both CLI and HTTP.
- Rationale: one business-logic contract avoids drift between command and web surfaces.

7. Make daemon mode mandatory for forum workflows.
- Rationale: removes split runtime behavior and ensures durable processing guarantees.

8. Support stdout logs plus optional structured file sync logging.
- Rationale: enables operational observability without forcing file log persistence by default.

9. Do not commit compiled frontend assets.
- Rationale: aligns with prior review feedback and avoids noisy diffs/artifact churn.

## Alternatives Considered
1. Keep forum workers embedded only in CLI commands.
- Rejected: no always-on delivery/processing guarantee; poor base for web clients.

2. Build separate forum daemon and separate API server binaries.
- Rejected: duplicated configuration/health/lifecycle complexity.

3. Reimplement forum state for web API directly in SQL handlers.
- Rejected: bypasses service invariants and event semantics.

4. Commit generated static assets to repository.
- Rejected: prior review already flagged this; better handled via build/generate flow.

5. Build web UI first, daemon later.
- Rejected: UI would depend on unstable in-process forum runtime behavior.

## Implementation Plan
1. **Server scaffolding + lifecycle**
- add `serve` subcommand and runtime wiring;
- load policy once, initialize store, start forum runtime;
- add graceful shutdown and health probes.

2. **Forum worker durability**
- add continuous bus processing loop with backoff/metrics;
- expose queue depth + lag health diagnostics.

3. **HTTP API endpoints**
- implement run summary/detail endpoints (reuse prior METAWSM-006 shape where useful);
- implement forum query + command endpoints mapped to existing service methods;
- add structured error model and request validation.

4. **Live update channel**
- implement cursor bridge to WebSocket stream;
- emit forum event envelopes + lightweight heartbeat frames.

5. **Web UI integration**
- restore/add `ui/` app for forum inbox + thread detail + run context;
- wire Vite dev proxy to server;
- wire `go generate` + embed flow with non-committed build outputs.

6. **Policy, docs, and tests**
- add policy knobs (`server.enabled`, `server.addr`, logging options, localhost binding expectations);
- update `README.md`, `docs/system-guide.md`, and `docs/how-to-run-forum.md`;
- add tests for server routes, forum command paths, stream behavior, and startup failure modes.

7. **Rollout + migration**
- cut forum flows over to mandatory daemon-backed handling;
- migrate CLI forum commands to the shared internal service API layer;
- document operator runbook for daemon start/stop and diagnostics.

## Resolved Decisions (2026-02-09)
1. Live update channel uses WebSocket in V1.
2. Trust/auth scope is localhost for V1.
3. Build an internal service API consumed by both CLI and HTTP handlers.
4. Daemon mode is mandatory for forum workflows.
5. Logging supports stdout plus optional structured file sync.

## References
- `docs/how-to-run-forum.md`
- `docs/system-guide.md`
- `internal/forumbus/runtime.go`
- `internal/orchestrator/service.go`
- `internal/orchestrator/service_forum.go`
- `internal/orchestrator/forum_dispatcher.go`
- `internal/store/sqlite.go`
- `ttmp/2026/02/08/METAWSM-008--build-metawsm-managed-q-a-forum-for-agent-operator-human-collaboration/design-doc/01-implementation-plan-for-metawsm-q-a-forum.md`
- `ttmp/2026/02/08/METAWSM-010--rework-control-flow-using-forum-first-internal-communication/design-doc/01-plan-replace-internal-communication-with-metawsm-008-forum.md`
- Historical branch artifact: `origin/metawsm-006/metawsm/run-20260208-073222:internal/webapi/api.go`
- Historical branch artifact: `origin/metawsm-006/metawsm/run-20260208-073222:internal/web/spa.go`
