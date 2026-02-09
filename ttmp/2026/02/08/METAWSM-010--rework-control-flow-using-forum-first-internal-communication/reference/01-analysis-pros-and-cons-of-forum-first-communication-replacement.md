---
Title: 'Analysis: pros and cons of forum-first communication replacement'
Ticket: METAWSM-010
Status: active
Topics:
    - core
    - backend
    - cli
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: Operational impact on watch and operator loops
    - Path: internal/orchestrator/service.go
      Note: Tradeoffs from file signals and close gates
    - Path: internal/orchestrator/service_forum.go
      Note: Forum capabilities and current limitations
    - Path: internal/policy/policy.go
      Note: Configured forum transport expectations and constraints
    - Path: internal/store/sqlite_forum.go
      Note: Persistence-level costs and benefits
ExternalSources: []
Summary: Tradeoff analysis for replacing internal file and status-parsing communication with forum-first control flow.
LastUpdated: 2026-02-08T19:12:00-08:00
WhatFor: Evaluate benefits, risks, and operational tradeoffs of forum-first communication migration.
WhenToUse: During prioritization and rollout decisions for METAWSM-010.
---


# Analysis: pros and cons of forum-first communication replacement

## Goal

Provide a clear decision-quality tradeoff analysis for replacing internal communication with the existing forum subsystem.

## Context

Today, metawsm uses file-based bootstrap signals, DB-backed guidance/forum state, and status-text parsing in watch/operator loops. Forum-first replacement would converge these into one canonical communication model.

## Quick Reference

### Pros

| Area | Benefit | Why it matters |
|---|---|---|
| Consistency | Single communication authority (forum tables/events) | Removes split-brain logic between files and DB state |
| Observability | Rich audit history with envelopes (`event_id`, `correlation_id`, `causation_id`) | Easier debugging, replay, and forensic analysis |
| Operator UX | Native queue semantics (state, priority, assignee, SLA aging) | Better triage and escalation workflows than ad hoc file checks |
| Multi-question workflows | Threads/posts naturally support iterative clarification | Better fit than one-shot guidance request file |
| Close/readiness gates | Can derive from structured signals rather than file existence | Reduces file path/root ambiguity and stale file artifacts |
| Automation potential | Structured events can drive future consumers reliably | Enables cleaner docs sync, notifications, and policy-based actions |

### Cons

| Area | Cost/Risk | Why it matters |
|---|---|---|
| Migration complexity | Needs dual-mode compatibility and careful cutover | High risk of regressions in bootstrap lifecycle behavior |
| Semantics design burden | Must define strict control-signal schema for complete/validate | Weak schema risks ambiguous run transitions |
| Runtime coupling | Forum becomes mission-critical for control flow | Outages or schema bugs can block runs if not guarded |
| Data volume and contention | More frequent event/thread writes in SQLite | Could increase lock contention in tight polling loops |
| CLI and prompt churn | Agent/operator instructions and tooling must change | Requires retraining habits and updating docs/playbooks |
| Partial implementation gap | Policy suggests Redis/topic transport, but implementation is SQLite-direct today | Expectation mismatch unless transport roadmap is explicit |

### Neutral but important realities

- Forum-first does not require immediate Redis transport adoption; it can still be a net win using existing SQLite-backed forum primitives.
- Replacing status-text parsing with typed snapshots is a separate but strongly related refactor that should be bundled for maximum reliability gains.
- The biggest risk is not forum mechanics; it is preserving bootstrap-close invariants during migration.

### Net assessment

Forum-first replacement is favorable if executed as a phased migration with compatibility controls. The operational upside (consistency, observability, and triage quality) is substantial, but only if the team invests in:

- explicit control payload contracts;
- dual-read/dual-write migration period;
- strong transition and close-gate regression coverage.

## Usage Examples

### Use this analysis when deciding rollout strategy

- If short-term delivery risk is dominant: keep compatibility mode longer and migrate watch/operator first.
- If long-term maintainability is dominant: prioritize removing file-signal dependencies once forum control semantics are stable.

### Use this analysis when sequencing implementation

1. Define control-signal schema and compatibility mode.
2. Switch service lifecycle reads to forum-first (with fallback).
3. Switch close gates and agent prompts.
4. Remove legacy file signal code.

## Related

- `ttmp/2026/02/08/METAWSM-010--rework-control-flow-using-forum-first-internal-communication/design-doc/01-plan-replace-internal-communication-with-metawsm-008-forum.md`
- `internal/orchestrator/service.go`
- `internal/orchestrator/service_forum.go`
- `cmd/metawsm/main.go`
- `internal/store/sqlite_forum.go`
