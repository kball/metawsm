---
Title: Core orchestrator capabilities
Ticket: METAWSM-001
Status: active
Topics:
    - core
    - cli
    - tui
    - gui
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles: []
ExternalSources: []
Summary: MVP proposal for metawsm as a thin orchestrator over wsm, docmgr, and tmux using HSM + SQLite, with declarative policy and multi-ticket parallel operation.
LastUpdated: 2026-02-07T06:48:52-08:00
WhatFor: ""
WhenToUse: ""
---

# Core orchestrator capabilities

## Executive Summary

`metawsm` should be a thin orchestrator over `wsm`, `docmgr`, and `tmux` that can start, monitor, and close multi-workspace agent work reliably.

The first useful version should focus on:
- repeatable run planning (what workspaces, what ticket, what agents),
- deterministic execution of toolchain steps,
- observable runtime state (what each agent is doing now),
- safe recovery and resume after partial failure.

## Problem Statement

Current multi-agent work is fragmented:
- workspace lifecycle lives in `wsm`,
- ticket/docs lifecycle lives in `docmgr`,
- live session execution lives in `tmux`.

There is no single control plane that can:
- declare intent once ("run N agents on ticket X over these repos"),
- launch and coordinate all required actions,
- track live progress and failures across sessions,
- close the loop back into ticket documentation.

Without that, operations are manual, error-prone, and hard to recover.

## Proposed Solution

Implement `metawsm` around six core pieces:

1. **Run Spec + Plan Compiler**
   - Input: ticket set (one or many), workspace strategy (create/fork/reuse), repo set, agent roles, execution profile.
   - Output: explicit ordered plan of concrete commands and side effects.
   - Include `--dry-run` first-class support.

2. **Hierarchical State Machine (HSM) Runtime**
   - Model lifecycle as nested states:
     - run lifecycle (`created` -> `planning` -> `running` -> `paused|failed|completed`),
     - per-step lifecycle,
     - per-agent lifecycle.
   - Encode transition guards and allowed retries explicitly.
   - Drive execution from state transitions, not ad-hoc conditionals.

3. **State Store (SQLite)**
   - Persist one run record with run id, ticket id, workspace paths, tmux session names, and per-agent state.
   - Persist step status (`pending`, `running`, `done`, `failed`) and timestamps for resume/retry.
   - Use a local SQLite DB for MVP (for example `.metawsm/metawsm.db`) with small normalized tables.

4. **Execution Engine**
   - Executes the compiled plan step-by-step with clear boundaries:
     - `docmgr` setup/ticket/doc actions,
     - `wsm` workspace actions,
     - `tmux` session/pane launches.
   - Stops on hard failure, records failure context, supports scoped retry.
   - Uses HSM transitions as the single source of truth for progression and recovery.

5. **Agent Session Adapter**
   - Normalizes how an agent process is started in tmux (command template, cwd, env vars).
   - Standardizes session naming and pane layout so status queries are predictable.
   - Captures minimal heartbeat data from tmux panes (alive/dead + last output timestamp).

6. **Operator Interfaces**
   - CLI first (`metawsm run/status/resume/stop/close`).
   - TUI second for live run visibility and manual intervention.
   - GUI/web later, backed by the same run state model.

## Design Decisions

1. **Orchestrate, do not replace**
   - Reuse `wsm` and `docmgr` as source-of-truth systems.
   - Avoid duplicating workspace or document business logic in `metawsm`.

2. **Plan then execute**
   - Every action should come from a compiled plan, not ad-hoc command calls.
   - This is required for dry-run, deterministic behavior, and resume support.

3. **State is mandatory**
   - In-memory only orchestration is insufficient once runs span many workspaces/agents.
   - Durable run state is required to recover after crash/restart.

4. **Use HSM for orchestration lifecycle**
   - Lifecycle logic grows quickly with retries, pausing, and partial failure.
   - HSM keeps transitions explicit and prevents illegal state jumps.

5. **Use SQLite for durable state**
   - SQLite gives transactional updates and queryable history without external services.
   - It scales better than ad-hoc JSON files as runs and agents increase.

6. **CLI is the control surface for MVP**
   - CLI gives automation and scriptability immediately.
   - TUI/GUI should consume the same state, not create a separate execution model.

7. **Failure semantics are explicit**
   - Each step declares whether failure is blocking or non-blocking.
   - Recovery commands (`resume`, `retry-step`, `stop`) should be built early.

8. **Support many tickets in parallel**
   - A single orchestrated run may include multiple tickets.
   - Ticket scope is explicit in run spec and persisted state.

9. **Default to `wsm create`**
   - New workspace creation is default behavior.
   - `wsm fork` is opt-in via explicit flag or policy.

10. **Tmux topology is per agent/workspace pair**
   - Each agent/workspace pair gets its own tmux session namespace.
   - Avoids coupling independent agent lifecycles.

11. **Close flow enforces clean git state**
   - Merge/close path requires clean repo state before merge steps start.
   - Non-clean state is a hard stop with actionable diagnostics.

12. **Policy is declarative**
   - Run behavior should be loaded from versioned policy files.
   - Flags override policy only for one-off local exceptions.

## Alternatives Considered

1. **Big monolithic orchestrator that re-implements `wsm` and `docmgr`**
   - Rejected: high scope, duplicated logic, higher divergence risk.

2. **Stateless wrapper scripts only**
   - Rejected: impossible to reliably resume or inspect long-running multi-agent runs.

3. **Flat FSM or ad-hoc branching without hierarchy**
   - Rejected: poor fit for nested run/step/agent lifecycle semantics.

4. **JSON files as durable state**
   - Rejected: weak concurrency guarantees, harder querying, and migration pain.

5. **GUI-first approach**
   - Rejected for MVP: slower to deliver and weaker automation path.

6. **Fully dynamic autonomous scheduling from day one**
   - Rejected for MVP: too much policy complexity before core lifecycle is stable.

## Implementation Plan

1. **Scaffold command surface**
   - Add `metawsm` CLI with `run`, `status`, `resume`, `stop`, `close`.

2. **Define HSM model + SQLite schema**
   - Add hierarchical states and legal transitions for run/step/agent lifecycles.
   - Add SQLite tables for runs, steps, agents, events, and transition history.
   - Persist to `.metawsm/metawsm.db`.

3. **Implement plan compiler**
   - Translate `RunSpec` into explicit steps for `docmgr`, `wsm`, and `tmux`.
   - Add `--dry-run` output for full plan preview.

4. **Implement execution engine on top of HSM**
   - Execute steps with structured logs and failure capture.
   - Add resume-from-last-failed-step behavior using persisted state transitions.

5. **Implement health evaluator**
   - Derive health states from tmux liveness, heartbeat freshness, and progress freshness.
   - Persist health snapshots/events in SQLite for status and auditability.

6. **Implement status reporting**
   - Aggregate SQLite run state + tmux-derived health into concise CLI output.

7. **Add close flow**
   - Integrate `wsm merge` and `docmgr ticket close` with guard checks.
   - Enforce clean git state gate before merge.

8. **Add first TUI view**
   - Read-only run monitor for active runs and agent states.

## Resolved Operator Decisions (2026-02-07)

1. One run can span many tickets in parallel.
2. Default workspace strategy is `wsm create`.
3. Tmux topology is one session namespace per agent/workspace pair.
4. Close flow is gated on clean git state before merge.
5. Policy model is declarative-first and accumulates over time.

## Agent Health Recommendation

Use a three-signal health model with policy-configurable thresholds:

1. **Process Liveness**
   - Signal: tmux pane/session exists and process is alive.
   - Failure mode: `dead` immediately when process exits unexpectedly.

2. **Activity Heartbeat**
   - Signal: last stdout/stderr activity timestamp from pane capture.
   - Default thresholds:
     - `idle` when no output for 5m,
     - `stalled` when no output for 15m.

3. **Progress Heartbeat**
   - Signal: no state transition or no completed step for a window.
   - Default threshold:
     - `stalled` when no progress event for 20m while process is alive.

Derived health states for operator UX:
- `healthy`: alive + recent activity/progress
- `idle`: alive + low activity but within progress window
- `stalled`: alive but exceeded activity/progress thresholds
- `dead`: process exited or pane missing

Recommendation rationale:
- tmux liveness alone is insufficient (hung agents look alive),
- activity alone is noisy (chatter without progress),
- combining liveness + activity + progress is robust for MVP without deep agent instrumentation.

## References

- `AGENT.md` in this repository for current orchestration direction.
- `workspace-manager` README and implementation docs for workspace lifecycle behavior.
- `docmgr` README for ticket/document workflow and root resolution behavior.
