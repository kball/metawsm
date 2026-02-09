---
Title: 'Plan: implement Redis Watermill forum control flow and migration'
Ticket: METAWSM-008
Status: active
Topics:
    - core
    - chat
    - backend
    - websocket
    - gui
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: CLI forum commands, watch/operator snapshot ingestion, and control command UX
    - Path: internal/model/forum.go
      Note: Command/event envelopes and topic registry used by transport and handlers
    - Path: internal/orchestrator/service.go
      Note: Current bootstrap file-signal control flow and close checks to be migrated
    - Path: internal/orchestrator/service_forum.go
      Note: Current direct SQLite command handling path to be split into dispatcher + handlers
    - Path: internal/policy/policy.go
      Note: Forum Redis/topic/mode policy surface and rollout flags
    - Path: internal/store/sqlite.go
      Note: Forum schema, projection tracking, and idempotency primitives
    - Path: internal/store/sqlite_forum.go
      Note: Command persistence and projection updates requiring event-driven refactor
ExternalSources: []
Summary: >
  Detailed implementation and migration plan to move forum control flow from direct in-process SQLite writes
  to Redis/Watermill command-event transport, then cut run/bootstrap control gates and watch/operator logic over
  to forum-native signaling.
LastUpdated: 2026-02-09T05:53:32.573325-08:00
WhatFor: Build the missing Redis/Watermill runtime path and safely migrate metawsm control flow to use it as the canonical communication model.
WhenToUse: Before implementing the Redis/Watermill forum runtime and before switching bootstrap/run control gates to forum-first mode.
---

# Plan: implement Redis Watermill forum control flow and migration

## Executive Summary

METAWSM-008 implemented forum commands, state transitions, and projections, but it did not implement the planned Redis/Watermill runtime flow. Command execution currently goes directly from service methods to SQLite writes (`service_forum.go` -> `sqlite_forum.go`), while `policy.forum.redis.*` and topic prefixes are configuration-only.

This plan completes the missing architecture in two tracks:

1. implement a real Watermill + Redis command/event pipeline with idempotent consumers and replay/recovery support;
2. migrate run/bootstrap control flow from file signals + status-text parsing to forum-native control signals with a full cutover.

The migration is a full cutover: replace `direct` with `forum_first` in one migration with no compatibility bridge, no soak window, and no kill-switch fallback.

## Problem Statement

Current behavior has three mismatches:

1. transport mismatch:
- design/docs describe Watermill + Redis command/event flow;
- implementation performs direct SQLite writes for command handling and projection updates.

2. control-flow split brain:
- bootstrap guidance/completion/validation gates still depend on `.metawsm/*.json` files (`syncBootstrapSignals`, `Guide`, `ensureBootstrapCloseChecks`);
- forum data is only supplemental for status/escalation rendering.

3. operator/watch coupling:
- watch/operator parse rendered status text, not a typed communication snapshot;
- this blocks reliable event-driven behavior and makes cutover brittle.

Without closing this gap, we keep policy/config drift, mixed semantics, and weak replay/outage behavior for the communication path.

## Proposed Solution

### 1. Add a forum transport runtime layer

Introduce a `forumbus` runtime package with:
- Watermill router startup/shutdown lifecycle;
- Redis stream pub/sub transport initialization from `policy.forum.redis`;
- command subscribers (`forum.commands.*`) and event/integration publishers (`forum.events.*`, `forum.integration.*`);
- middleware stack for correlation propagation, retries, dead-letter routing, structured logs, and idempotency keys.

Service methods publish commands through a dispatcher interface, not directly to store methods.

### 2. Split command handling from projection updates

Refactor current store writes into explicit roles:
- command handlers:
  validate invariants, mutate command-side state, append `forum_events` audit rows;
- projection consumers:
  consume domain events and update `forum_thread_views`, `forum_thread_stats`.

Retain at-least-once semantics with projection idempotency tracked via `forum_projection_events`.

### 3. Implement reliable publish/consume semantics

Use a transactional outbox table for command and integration events:
- command handler DB transaction writes state + event + outbox row atomically;
- publisher worker drains outbox to Redis and marks rows sent;
- replay/recovery re-drives unsent outbox rows after crashes.

This prevents lost events when DB commit succeeds but Redis publish fails.

### 4. Define forum-native control signal contract

Standardize control events/payloads on forum primitives for:
- guidance requested;
- guidance answered;
- implementation complete;
- validation result.

Add payload schema versioning for control messages and strict validation rules.

### 5. Migrate bootstrap/run gates to forum control state

Replace file-dependent logic with forum-derived logic:
- `syncBootstrapSignals`: forum-first derivation of pending guidance and completion status;
- `Guide`: answer via forum command path with no file output path;
- `ensureBootstrapCloseChecks`: validate completion/validation forum signals instead of file existence.

### 6. Replace status parsing with typed snapshot consumption

Add service-level typed snapshot API for watch/operator:
- unresolved guidance/control threads;
- escalation candidates;
- per-run forum queue counters;
- unhealthy agents and blockers.

Keep `status` as presentation-only text rendering of the typed snapshot.

### 7. Execute a single cutover

Use one migration release to switch all control flow to forum-first. Do not introduce runtime compatibility modes or rollback toggles.

## Design Decisions

1. Keep SQLite as command/read state authority during transport migration.
Rationale: minimizes storage blast radius; transport and semantic migration are already high risk.

2. Introduce outbox before enabling Redis command path.
Rationale: ensures crash-safe publish semantics and deterministic replay.

3. No runtime compatibility mode and no kill-switch fallback.
Rationale: this is in-progress software and migration simplicity is prioritized over back-compat complexity.

4. Keep status text for humans, but stop making it machine input.
Rationale: preserves UX while removing parser fragility from operator automation.

5. Enforce exactly one control thread per `(run_id, agent_name)`.
Rationale: removes ambiguity for completion/validation state derivation and close-gate checks.

6. Remove `metawsm guide` after forum-first cutover.
Rationale: keep a single command surface (`metawsm forum ...`) and avoid parallel operator workflows.

7. Cut over without canary soak.
Rationale: this environment does not require prolonged staged rollout before enabling the new path.

## Alternatives Considered

1. Stay on direct SQLite forever and ignore Redis/Watermill plan.
Rejected: leaves architecture/documentation drift and blocks event-driven fan-out/replay goals.

2. Canary/soak rollout with compatibility toggles.
Rejected: adds migration scaffolding and operational complexity not needed for in-progress software.

3. Move all persistence to Redis streams.
Rejected: unnecessary data model churn; SQLite already holds authoritative workflow state and query models.

## Implementation Plan

### Phase 0: Baseline and control contracts

Deliverables:
- document current/target data flows and mode behavior table;
- define control payload schemas (`guidance_request`, `guidance_answer`, `completion`, `validation`) with versioned fields;
- define SLOs: publish success, consumer lag, replay duration.

Acceptance:
- contract docs committed;
- schema validation tests added for control payloads.

### Phase 1: Transport foundation (no behavior change)

Deliverables:
- add Watermill + Redis dependencies;
- add `internal/forumbus` package (router, publisher, subscriber, middleware);
- add policy validation for Redis URL/stream/group/consumer and control mode;
- add health endpoint/check method for forum bus readiness.

Acceptance:
- process starts with bus enabled/disabled cleanly;
- no command-path behavior change yet.

### Phase 2: Command dispatcher + outbox

Deliverables:
- add command dispatcher interface with implementations:
  - `DirectForumDispatcher` (current path);
  - `BusForumDispatcher` (publish to command topics);
- add outbox schema and worker;
- refactor `service_forum.go` to call dispatcher abstraction.

Acceptance:
- in `direct` mode behavior unchanged;
- outbox publish retries + dead-letter behavior tested.

### Phase 3: Async command handlers and projectors

Deliverables:
- implement Watermill command consumers for open/add-post/assign/state/priority/close;
- refactor command-side DB updates into handler units;
- implement projection consumers from `forum.events.*` into view/stats tables;
- implement idempotency with `event_id` + `forum_projection_events`.

Acceptance:
- duplicate command/event delivery is safe;
- replay from stored events rebuilds projections deterministically.

### Phase 4: Forum control signal cutover (no bridge)

Deliverables:
- add helper commands/API methods for control signaling through forum;
- update `Guide` to publish answer control event and remove legacy guidance-response file output;
- remove legacy file-signal ingestion paths from runtime control decisions.

Acceptance:
- run lifecycle advances using only forum control signals for guidance/answer/completion/validation.

### Phase 5: Bootstrap and close-gate migration

Deliverables:
- refactor `syncBootstrapSignals` to read forum control state first;
- refactor `ensureBootstrapCloseChecks` to forum validation/completion rules;
- remove file fallback logic entirely.

Acceptance:
- `forum_first` runs complete/close without `.metawsm/*.json` dependencies;
- no code path in run-control logic depends on legacy signal files.

### Phase 6: Watch/operator typed snapshot migration

Deliverables:
- add typed snapshot API in service;
- switch `watch` and `operator` to typed snapshot instead of status parsing;
- keep status output as rendering of snapshot data.

Acceptance:
- operator decisions/hints match prior expected outcomes;
- parser-coupling tests removed/replaced with typed snapshot tests.

### Phase 7: Full cutover release

Deliverables:
- switch all runs to forum-first control flow in one release;
- remove runtime mode switches and rollback toggles from policy/runtime wiring;
- add metrics/log dashboards:
  publish failure rate, command processing lag, projection lag, DLQ counts;
- run incident drills: Redis outage, consumer restart, replay recovery.

Acceptance:
- forum-first is the only active control-flow path;
- no runtime flag exists for legacy-mode execution.

### Phase 8: Legacy removal

Deliverables:
- remove legacy file-signal writes/reads and related docs;
- remove compatibility shims and dead code paths;
- simplify guide/bootstrap logic around forum-only semantics.

Acceptance:
- no production code depends on `.metawsm/guidance-*.json`, `.metawsm/implementation-complete.json`, `.metawsm/validation-result.json`.

## Rollout and Migration Checklist

1. Pre-migration
- verify Redis availability and persistence settings;
- create dashboards/alerts for bus health and lag.

2. Data migration
- backfill unresolved legacy guidance rows/files into forum control threads;
- optionally replay recent `forum_events` into Redis for warm caches/subscribers.

3. Cutover progression
- `direct` (baseline) -> `forum_first` (all runs).

4. Failure handling
- no rollback path; handle failures by forward-fix and replay/recovery mechanisms.

## Validation Plan

1. Unit tests
- command envelope/schema validation;
- control payload validation;
- dispatcher and control-gate routing logic for the single forum-first path.

2. Integration tests
- Redis unavailable at startup and mid-run;
- duplicate command delivery;
- projection lag then catch-up;
- outbox replay after crash.

3. End-to-end tests
- agent asks guidance, operator answers, run resumes;
- completion + validation forum signals unlock close;
- watch/operator consume typed snapshot and produce correct hints/actions.

## Resolved Decisions (2026-02-09)

1. Use exactly one control thread per `(run_id, agent_name)`.
2. Remove `metawsm guide` in favor of `metawsm forum ...` commands.
3. Use no soak duration; execute direct full cutover.
4. Ship no kill switch and no compatibility mode; migrate to forum-first only.

## References

- `ttmp/2026/02/08/METAWSM-008--build-metawsm-managed-q-a-forum-for-agent-operator-human-collaboration/design-doc/01-implementation-plan-for-metawsm-q-a-forum.md`
- `ttmp/2026/02/08/METAWSM-010--rework-control-flow-using-forum-first-internal-communication/design-doc/01-plan-replace-internal-communication-with-metawsm-008-forum.md`
- `ttmp/2026/02/08/METAWSM-010--rework-control-flow-using-forum-first-internal-communication/reference/01-analysis-current-control-flow-for-tickets-agents-and-communication.md`
- `internal/orchestrator/service.go`
- `internal/orchestrator/service_forum.go`
- `internal/store/sqlite.go`
- `internal/store/sqlite_forum.go`
