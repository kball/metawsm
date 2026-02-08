---
Title: Implementation plan for metawsm Q&A forum
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
      Note: Add forum command group and operator integration points for question routing
    - Path: internal/model/types.go
      Note: Extend domain model with forum thread/question/answer entities
    - Path: internal/store/sqlite.go
      Note: Persist forum questions, answers, states, assignment, and audit fields
    - Path: internal/store/sqlite_test.go
      Note: Validate forum persistence and state transition semantics
    - Path: internal/orchestrator/service.go
      Note: Bridge run/agent lifecycle events into forum question workflow
    - Path: docs/system-guide.md
      Note: Document operator/human response workflow and operational expectations
ExternalSources: []
Summary: >
  Event-first implementation plan for a metawsm-managed Q&A forum using Watermill as the
  messaging backbone (Redis transport) for command handling, fan-out, projections, and escalation automation,
  while persisting operational forum state in SQLite.
LastUpdated: 2026-02-08T12:50:00-08:00
WhatFor: Define Watermill (Redis) event topology, SQLite state model, APIs, UX, and phased rollout for an agent-to-operator/human question forum.
WhenToUse: Before implementing METAWSM-008 event bus, storage, CLI/API, and UI changes.
---

# Implementation plan for metawsm Q&A forum

## Executive Summary

Build a first-class Q&A forum inside metawsm as a full event-driven subsystem using Watermill.
Commands from agents/operators/humans are converted into durable domain events, then projected into query models for CLI/TUI/GUI and automation.

The system should provide:
- durable question threads tied to run/ticket/agent context;
- explicit ownership and SLA states (`new`, `triaged`, `waiting_operator`, `waiting_human`, `answered`, `closed`);
- consistent fan-out to operator loop, notifications, and docs integration;
- full auditability via immutable event history with idempotent consumers.

## Problem Statement

Current guidance flows are optimized for single blocking prompts, not ongoing multi-question collaboration:
- agents cannot easily ask several questions and track response status over time;
- operator/human responders do not have a shared queue with priorities and ownership;
- important decision history is fragmented across terminal output, files, and docs.

Additionally, current watch/operator behavior is mostly polling and text-snapshot driven, which is harder to extend for multi-consumer workflows (triage queue, SLA escalations, unread indicators, and downstream sync).

This causes slowdowns in active runs, fragile integrations, and weak traceability of technical decisions.

## Proposed Solution

Add a metawsm forum subsystem with Watermill as the event backbone.

1. Event contracts (source of truth)
- Define versioned domain events with envelope metadata:
  - `event_id`, `event_type`, `event_version`, `occurred_at`
  - `thread_id`, `run_id`, `ticket`, `agent_name`
  - `actor_type` (`agent|operator|human|system`)
  - `correlation_id`, `causation_id`
- Core event types:
  - `forum.thread.opened`
  - `forum.post.added`
  - `forum.assigned`
  - `forum.state.changed`
  - `forum.priority.changed`
  - `forum.sla.escalation_requested`
  - `forum.thread.closed`

2. Command bus + handlers
- Intake commands from CLI/operator/agent adapters:
  - `OpenThread`, `AddPost`, `AssignThread`, `ChangeState`, `SetPriority`, `CloseThread`
- Publish commands to Watermill command topics.
- Command handlers validate invariants, perform transactional writes, and emit domain events.

3. Watermill topology
- Use Watermill router with named handlers and middleware (retry, poison queue handling, correlation IDs, structured logging).
- Use Redis-backed Watermill Pub/Sub from day one for command/event transport durability and decoupled consumers.
- Split topics by concern:
  - commands: `forum.commands.*`
  - domain events: `forum.events.*`
  - integration notifications: `forum.integration.*`

4. Persistence + projection/read models
- Command-side state tables in SQLite:
  - `forum_threads`, `forum_posts`, `forum_state_transitions`, `forum_assignments`
- Event durability:
  - Redis stream/topic retention for transport durability and replay window
  - explicit domain event audit table (`forum_events`) for forensic/history queries
- Read-side projection tables:
  - `forum_thread_views` (list/filter queue)
  - `forum_thread_stats` (unread/escalation/SLA counters)
  - optional `forum_agent_inbox` view
- Consumers are idempotent and safe for at-least-once delivery.
- Events describe changes and trigger consumers; SQLite persists authoritative operational state and query models.

5. Interaction surfaces
- CLI:
  - `metawsm forum ask`
  - `metawsm forum list`
  - `metawsm forum answer`
  - `metawsm forum assign`
  - `metawsm forum close`
- Operator loop:
  - subscribes to `forum.events.*` and `forum.integration.*`;
  - performs triage/escalation policy actions;
  - can draft/propose answers via command publication.
- TUI/GUI:
  - read from projection APIs for inbox/thread detail;
  - optionally subscribe to event streams for live updates.

6. Workflow integration
- Agents open/update threads through command adapters with automatic run/ticket metadata.
- Operator/human actions follow the same command path, preserving attribution.
- Documentation sync is an independent subscriber that appends decision summaries to ticket docs when enabled.

## Design Decisions

1. Use Watermill events as the first-class backbone, not side effects.
Rationale: simplifies fan-out to operator automation, notifications, and projections without coupling command code to each consumer.

2. Use Redis as Watermill transport from day one.
Rationale: avoids dual migration later and enables first-class async fan-out semantics immediately.

3. Model forum as thread/post primitives with explicit state transitions.
Rationale: supports clarifications while enabling deterministic command validation.

4. Accept at-least-once delivery and require idempotent consumers.
Rationale: practical reliability model for Watermill + Redis while avoiding brittle exactly-once assumptions.

5. Separate command-side writes from read-side projections.
Rationale: keeps writes strict and auditable while enabling fast filtered inbox queries for UI/operator paths.

6. Support both operator and human responders with the same command path.
Rationale: avoids dual systems and keeps attribution and authorization checks consistent.

7. Run all forum command/event flows through Watermill from day one.
Rationale: enforces one integration model and prevents mixed direct-call/event-call drift.

8. Set default SLA thresholds to 30 minutes initially.
Rationale: provides an actionable baseline for escalation tuning before collecting usage metrics.

9. Scope forum threads across runs for the same ticket.
Rationale: preserves decision continuity and reduces repeated context capture between successive runs.

10. Keep docs-sync enabled by default for answered/closed threads.
Rationale: maximizes automatic audit capture while the workflow matures.

11. Start with lower operator autonomy and mandatory human review gates.
Rationale: prioritize correctness and trust while gathering signal for future automation increases.

## Alternatives Considered

1. CRUD-only forum service (no event bus).
Rejected: command handlers become tightly coupled to every downstream behavior (alerts, projections, doc sync), raising implementation and testing complexity.

2. In-memory Go channel pub/sub.
Rejected: no durability/replay and weak failure recovery for operator workflows.

3. SQLite transport for Watermill in V1.
Rejected: does not match decided deployment direction and would force transport migration later.

## Implementation Plan

1. Phase 1: event and schema foundations
- add Watermill dependency and shared topic registry;
- add Redis-backed Watermill pub/sub configuration and connection lifecycle;
- define command and event envelopes with schema versioning;
- add forum command-side and read-side tables;
- add migration for domain event audit table (`forum_events`).
Acceptance: event contracts are fixed and test fixtures cover all core forum command/event paths.

2. Phase 2: command bus and handlers
- add `metawsm forum` command group publishing to `forum.commands.*`;
- implement command handlers to enforce invariants and emit `forum.events.*`;
- add retry/DLQ/idempotency middleware and consumer scaffolding.
Acceptance: open/assign/answer/close commands produce expected events and deterministic state.

3. Phase 3: projections and query APIs
- implement projection consumers for queue/thread views;
- add `ListQuestions(filter)` and `GetThread(thread_id)` backed by projections;
- add `WatchQuestions(cursor)` over event stream cursor.
Acceptance: CLI/operator/UI can query by ticket/run/state/priority/assignee with low-latency reads.

4. Phase 4: operator loop and agent adapters
- subscribe operator loop to escalation topics and SLA timers;
- add agent submission adapter with automatic metadata enrichment;
- expose policy knobs for auto-triage, escalation thresholds, and optional auto-draft behavior.
Acceptance: operator automation reacts from events instead of status-text polling for forum workflows.

5. Phase 5: GUI/TUI and notifications
- add forum inbox/thread panels backed by projection APIs;
- add optional live updates using event subscriptions;
- wire notify hooks for high-priority and SLA-breach events.
Acceptance: human operators can triage and answer end-to-end from visual surface with live queue updates.

6. Phase 6: docs and operational hardening
- update system docs and operator playbook;
- implement docs-sync subscriber for decision summary append behavior (default-on with policy override);
- add smoke tests and failure-mode handling (Redis unavailable, DB lock, malformed event payloads, duplicate deliveries, projection lag).
Acceptance: production workflow is documented, replay-safe, test-covered, and recoverable.

## Open Questions

1. Resolved: all forum commands/events run through Watermill asynchronously from day one.
2. Resolved: Watermill transport is Redis (not SQLite).
3. Resolved: default SLA escalation threshold is 30 minutes for both `new` and `waiting_human` (tunable by policy later).
4. Resolved: thread identity spans runs for the same ticket (cross-run thread scope).
5. Resolved: docs-sync subscriber is enabled by default for answered/closed threads.
6. Resolved: start with lower operator autonomy and mandatory human review gates by default.

## References

- `README.md`
- `docs/system-guide.md`
- `cmd/metawsm/main.go`
- `internal/orchestrator/service.go`
- `internal/store/sqlite.go`
- `https://watermill.io/docs/getting-started/`
- `https://watermill.io/advanced/cqrs/`
- `https://watermill.io/pubsubs/redisstream/`
- `ttmp/2026/02/08/METAWSM-007--create-orchestrator-operator-agent-for-ongoing-run-management/design-doc/01-implementation-plan-for-orchestrator-operator-agent.md`
