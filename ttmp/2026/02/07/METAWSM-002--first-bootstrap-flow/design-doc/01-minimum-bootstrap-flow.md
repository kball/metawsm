---
Title: Minimum bootstrap flow
Ticket: METAWSM-002
Status: active
Topics:
    - core
    - cli
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: AGENT.md
      Note: Project orchestration direction and source-of-truth integration constraints
    - Path: internal/orchestrator/service.go
      Note: Current plan/execute flow analyzed to scope minimum incremental changes
    - Path: ttmp/2026/02/07/METAWSM-001--bootstrap-metawsm-orchestration/design-doc/01-core-orchestrator-capabilities.md
      Note: Original architecture decisions reused by this minimum bootstrap design
    - Path: ttmp/2026/02/07/METAWSM-001--bootstrap-metawsm-orchestration/reference/01-diary.md
      Note: Implementation baseline reviewed to identify completed vs missing capabilities
ExternalSources: []
Summary: Minimum design for interactive intake, guidance loop, and merge-ready completion using existing metawsm foundations
LastUpdated: 2026-02-07T07:26:37.38819-08:00
WhatFor: ""
WhenToUse: ""
---



# Minimum bootstrap flow

## Executive Summary

Define the smallest reliable flow that starts from just `--ticket`, gathers enough intent from the user, creates the code workspace, runs implementation, pauses to ask for guidance when blocked, and finishes through merge/close.

The implementation should reuse what `METAWSM-001` already built (run planning/execution, HSM transitions, SQLite durability, `wsm`/`docmgr`/`tmux` integration, close gates) and add only missing capabilities for interactive bootstrap and guidance-driven continuation.

## Problem Statement

Current `metawsm` can execute planned steps, but it does not yet support the operator loop requested for first-use ticket execution:
- It requires pre-specified `--repos` and optional `--agent`; there is no guided intake from a ticket.
- It starts tmux sessions, then marks the run complete without an implementation guidance loop.
- It has no explicit "needs guidance" state/command for asking the user targeted follow-up questions mid-run.
- It does not capture a structured "what are we building?" brief before code execution.

From `METAWSM-001`, the following foundation already exists and should be reused:
- CLI + service surfaces for `run/status/resume/stop/close/tui`.
- Persisted run/step/agent/event state in `.metawsm/metawsm.db`.
- Plan compiler that verifies ticket existence, creates/forks workspaces, and starts tmux sessions.
- Clean-git gate + `wsm merge` + `docmgr ticket close` close flow.

## Proposed Solution

Add a new operator-first command:

`metawsm bootstrap --ticket <ID> --repos <repo1,repo2,...> [--agent ...]`

with four phases.

### Phase 1: Intake (question loop until sufficient)

Collect and persist a minimum "Run Brief" before workspace creation:
- Goal: what should be built/changed.
- Scope: areas/files expected to change.
- Done criteria: tests/checks and acceptance criteria.
- Constraints: non-goals, risky areas, approvals needed.
- Merge intent: branch/merge expectations if non-default.

Input requirement:
- `--repos` is mandatory for v1 bootstrap and is validated before intake starts.

Completeness gate:
- If any field is missing or ambiguous, ask another focused question.
- Exit intake only when all required fields are concrete and actionable.

Artifacts:
- Save structured brief in SQLite tied to `run_id`.
- Write/update a ticket reference doc (for operator/agent shared context).

### Phase 2: Bootstrap setup

After intake passes:
- Ensure docs ticket exists; if missing, auto-create with default bootstrap metadata.
- Build workspace name from ticket + run id (reuse current strategy).
- Create workspace (`wsm create` by default) and start tmux agent session.
- Pass Run Brief into agent startup prompt/context.

### Phase 3: Execution + guidance loop

Run until one of: complete, failed, or needs user guidance.

Minimum mechanism:
- Agent emits a guidance request by writing JSON to a workspace sentinel path:
  - `<workspace>/.metawsm/guidance-request.json`
  - payload includes `run_id`, `agent`, `question`, and optional `context`.
- Orchestrator transitions run into a paused guidance-needed state.
- Operator responds with a new command:
  - `metawsm guide --run-id <RUN_ID> --answer "<text>"`
- Resume execution from paused point with response appended to context.

This loop repeats until completion criteria are met.

### Phase 4: Merge-ready close

When agent reports completion:
- Run required validation commands from Run Brief.
- Enforce existing clean-git gate.
- Reuse existing `close` flow (`wsm merge` + `docmgr ticket close`).

## Design Decisions

1. Bootstrap is a new command, not an overloaded `run` flag set.
Reason: keeps existing deterministic `run` behavior stable and adds an explicit operator UX for intake.

2. Start with single-ticket flow only.
Reason: this is the minimum path to validate intake and guidance loops; multi-ticket can layer later.

3. Persist intake Q/A in SQLite and ticket docs.
Reason: recoverability and auditability require durable context across resume/stop/restart.

4. Guidance is explicit run state, not inferred from inactivity.
Reason: inactivity heuristics are noisy; an explicit guidance signal is reliable and testable.

5. Guidance signaling uses a per-workspace file sentinel.
Reason: it avoids fragile stdout parsing and can be implemented with minimal changes to current tmux/session model.

6. Bootstrap auto-creates missing tickets.
Reason: reduces operator friction and matches expected first-use behavior.

7. `--repos` is mandatory in v1 bootstrap.
Reason: repository inference is ambiguous today; explicit input keeps behavior deterministic.

8. Reuse existing close gates and command integrations.
Reason: `METAWSM-001` already implemented merge safety and doc closure; duplicate logic adds risk.

## Alternatives Considered

1. Keep current `run` flow and require users to pre-fill everything manually.
Rejected: does not satisfy "ask sufficient questions until it knows what to build."

2. Build full autonomous multi-agent planning first.
Rejected: too large for first bootstrap; increases complexity before proving operator loop.

3. Use only TUI/manual operator observation for guidance.
Rejected: no explicit machine-readable pause/question/answer path.

## Implementation Plan

1. Add `bootstrap` command surface and intake prompt loop with completeness checks.
2. Extend model/store schema for Run Brief + Q/A transcript linked to run.
3. Add ticket reference document generation/update for intake outputs.
4. Auto-create ticket workspace when `--ticket` does not already exist.
5. Extend planner/executor to include an agent work step that can signal guidance-needed via sentinel file.
6. Add guidance-request watcher/transition logic for paused-guidance state.
7. Add `guide` command and HSM transitions for paused-guidance -> running.
8. Add completion validation hook (tests/checks) before close.
9. Add integration tests for:
   - mandatory repos validation,
   - auto-create ticket behavior,
   - intake completeness loop,
   - guidance pause/resume via sentinel,
   - successful merge-ready completion path.
10. Add operator playbook documenting bootstrap workflow and failure recovery.

## Open Questions

None for MVP scope as of 2026-02-07.

## References

- `ttmp/2026/02/07/METAWSM-001--bootstrap-metawsm-orchestration/reference/01-diary.md`
- `ttmp/2026/02/07/METAWSM-001--bootstrap-metawsm-orchestration/design-doc/01-core-orchestrator-capabilities.md`
- `internal/orchestrator/service.go`
- `cmd/metawsm/main.go`
