---
Title: 'Plan: replace internal communication with METAWSM-008 forum'
Ticket: METAWSM-010
Status: active
Topics:
    - core
    - backend
    - cli
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: CLI
    - Path: internal/model/forum.go
      Note: Forum envelope and control payload contracts
    - Path: internal/orchestrator/service.go
      Note: Migration touchpoints for guide
    - Path: internal/orchestrator/service_forum.go
      Note: Forum-first command handlers and state transition semantics
    - Path: internal/policy/policy.go
      Note: Policy flags and full-cutover mode configuration
    - Path: internal/store/sqlite_forum.go
      Note: Canonical forum storage and event model
ExternalSources: []
Summary: Phased migration plan to move run/agent internal communication from bootstrap file signals and status parsing to a forum-first control flow.
LastUpdated: 2026-02-08T19:12:00-08:00
WhatFor: Define how metawsm can adopt forum-first communication without breaking existing run safety and operator workflows.
WhenToUse: When implementing or sequencing the control-flow rework described in METAWSM-010.
---


# Plan: replace internal communication with METAWSM-008 forum

## Executive Summary

Replace file-signal and status-text-based internal communication with a forum-first protocol centered on the existing METAWSM-008 forum tables/events, using a full cutover instead of dual-write/dual-read compatibility bridging.

## Problem Statement

Current control flow mixes three mechanisms:

1. workspace files for bootstrap guidance/completion/validation;
2. SQLite forum threads/events for discussion workflows;
3. status text parsing for watch/operator decisions.

This creates split-brain communication semantics and extra failure modes (missing files, ambiguous precedence, parser drift).

## Proposed Solution

Adopt a single internal communication bus model based on forum entities and events.

### A. Define forum-native control signals

Use forum as the canonical communication protocol for:

- guidance request (`agent -> operator/human`);
- guidance answer (`operator/human -> agent`);
- completion declaration (`agent -> system/operator`);
- validation result (`agent -> system/operator`);
- operator escalation/triage states.

Recommended v1 representation:

- Reuse existing thread/post/state primitives.
- Add deterministic conventions for control-thread titles/state and payload schema.
- Add helper commands for agents/operators so they do not need to handcraft low-level forum semantics.

### B. Make forum the source of truth for run gates

Replace file checks in bootstrap lifecycle with forum-derived checks:

- `awaiting_guidance` is derived from unresolved control threads in waiting states.
- `completed` requires completion+validation forum signals per agent/run.
- close checks validate forum control state instead of `.metawsm/*.json` files.

### C. Move watch/operator to structured snapshot, not status parsing

Expose a typed run snapshot API from orchestrator service that includes:

- unresolved guidance/control threads;
- unhealthy agents;
- queue summaries.

Update `watch` and `operator` to consume this typed snapshot directly, keeping `status` as human rendering only.

### D. Align agent contract and policy

Update default agent prompt/profile from file instructions to forum instructions. Agents should use explicit commands for ask/complete/validate signaling.

## Design Decisions

1. Forum-first but migration-safe.
Rationale: avoid breaking live workflows while converging on one communication model.

2. Keep SQLite forum backend as immediate authority.
Rationale: already implemented and tested; Redis transport can remain a future enhancement.

3. Separate transport ambition from control-flow replacement.
Rationale: replacing communication semantics is already high risk; avoid coupling to new runtime infrastructure in the same cutover.

4. Add explicit control-signal schema versioning inside forum payloads.
Rationale: safeguards future evolution of completion/validation semantics.

## Alternatives Considered

1. Keep dual model indefinitely (files + forum).
Rejected: preserves ambiguity and parser/file coupling.

2. Replace with non-forum custom state tables for guidance/completion.
Rejected: duplicates communication primitives and bypasses existing METAWSM-008 investment.

3. Full Watermill/Redis cutover before semantics migration.
Rejected for first migration: increases blast radius and deployment complexity.

## Implementation Plan

1. Phase 0: control protocol spec and acceptance criteria
- define forum control-thread conventions (thread classification, required payload fields);
- define acceptance criteria for `awaiting_guidance`, `completed`, and close gates under forum-first mode;
- document single-cutover behavior and cutover checklist.

2. Phase 1: control-signal cutover (no bridge)
- add helper commands for forum control signals (ask/answer/complete/validate);
- when `metawsm guide` answers guidance, write forum answer/state only;
- remove legacy file ingestion from runtime control logic.

3. Phase 2: forum-first read path in service lifecycle
- refactor `syncBootstrapSignals()` to read unresolved/resolved forum control threads first;
- derive run status transitions from forum state;
- remove file fallback from this path.

4. Phase 3: close-gate migration
- replace `ensureBootstrapCloseChecks()` file requirements with forum validation/completion requirements;
- ensure per-agent completion and done-criteria equivalence are represented in forum payload schema;
- keep strict validation errors and actionable failure messages.

5. Phase 4: watch/operator refactor
- add typed snapshot method on orchestrator service;
- stop parsing rendered status text in `watch`/`operator`;
- directly classify guidance-needed/escalation events from structured forum+agent data.

6. Phase 5: agent/policy contract switch
- update default agent base prompt from `.metawsm/*.json` signaling to forum commands;
- update docs (`README.md`, `docs/system-guide.md`) and operational playbooks;
- include migration notes for existing tickets/runs;
- remove `metawsm guide` command path from docs and CLI surface.

7. Phase 6: deprecation and cleanup
- remove legacy file-signal reads/writes immediately after cutover release hardening;
- simplify code paths (`Guide`, `syncBootstrapSignals`, iteration reset logic);
- remove stale docs that describe file-signal contracts.

8. Validation and rollout controls
- add unit/integration tests for forum-driven transitions and close gates;
- add regression tests for operator alerts and watch hints under forum-first mode;
- execute a single full cutover to forum-first with no compatibility mode and no canary soak window.

## Open Questions

1. Should completion and validation be separate control thread types or one typed payload stream in a single per-agent control thread?
2. Should forum-first mode be bootstrap-only first, or cover both `run` and `bootstrap` simultaneously?
3. Is policy-level `forum.enabled` expected to disable only UI/commands, or also control-flow dependencies once forum-first becomes mandatory?

## Resolved Decisions (2026-02-09)

1. Use exactly one control thread per `(run_id, agent_name)`.
2. Remove `metawsm guide` as part of the migration.
3. Use no soak duration; execute direct full cutover.
4. Ship no kill switch and no compatibility mode.

## References

- `internal/orchestrator/service.go`
- `internal/orchestrator/service_forum.go`
- `internal/store/sqlite.go`
- `internal/store/sqlite_forum.go`
- `cmd/metawsm/main.go`
- `internal/policy/policy.go`
- `internal/model/forum.go`
- `ttmp/2026/02/08/METAWSM-008--build-metawsm-managed-q-a-forum-for-agent-operator-human-collaboration/design-doc/01-implementation-plan-for-metawsm-q-a-forum.md`
