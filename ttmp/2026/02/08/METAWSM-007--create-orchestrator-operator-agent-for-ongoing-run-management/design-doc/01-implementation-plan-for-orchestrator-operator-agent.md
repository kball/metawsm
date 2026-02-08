---
Title: Implementation plan for orchestrator operator agent
Ticket: METAWSM-007
Status: active
Topics:
    - core
    - cli
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: Add operator command surface and loop wiring
    - Path: cmd/metawsm/main_test.go
      Note: Add operator decision and CLI behavior tests
    - Path: internal/hsm/hsm.go
      Note: Respect allowed run transitions during automated actions
    - Path: internal/orchestrator/service.go
      Note: Reuse restart and stop primitives for automated remediation
    - Path: internal/orchestrator/service_test.go
      Note: Extend run-management tests for stale stop and bounded restarts
    - Path: internal/policy/policy.go
      Note: Add and validate operator policy thresholds
    - Path: internal/store/sqlite.go
      Note: Persist operator retry budget and cooldown state across restarts
    - Path: internal/store/sqlite_test.go
      Note: Verify persisted operator state survives process restarts
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-08T08:36:54.863211-08:00
WhatFor: Plan implementation of a hybrid deterministic plus LLM operator-agent loop that auto-recovers mechanical failures and escalates true decision blockers.
WhenToUse: Before implementing METAWSM-007 changes in CLI/orchestrator watch and run-management paths.
---


# Implementation plan for orchestrator operator agent

## Executive Summary

Implement a new `metawsm operator` command that continuously supervises active runs using a hybrid control model: deterministic safety rules plus an LLM operator-agent for triage and decision support.  
The design reuses existing orchestration primitives (`Status`, `ActiveRuns`, `Restart`, `Stop`, `Guide`) and adds a bounded decision engine, LLM context packaging, and tests, without changing the existing manual `watch` behavior.

## Problem Statement

`metawsm watch` currently detects and reports issues but does not take action. In practice this causes high-noise operator workflows:

- stale historical runs continue appearing as unhealthy and dilute signal;
- dead/stalled sessions require repetitive manual restarts;
- escalation policy is implicit in operator behavior rather than encoded in the product.

Current system baseline:

- `watch` alerts on guidance, terminal states, and unhealthy agents, then prints hints only (`cmd/metawsm/main.go`).
- Recovery primitives already exist and are test-covered (`internal/orchestrator/service.go`): `Restart`, `Stop`, `Guide`, `ActiveRuns`, `Status`.
- Active-run filtering already exists but includes paused/stopping/closing states (`internal/orchestrator/service.go`).
- Run/agent transitions are enforced by HSM (`internal/hsm/hsm.go`), so operator automation should call service APIs, not mutate state directly.

## Proposed Solution

Add an explicit operator loop command with deterministic guardrails and an LLM loop:

1. `metawsm operator` command surface in CLI
- default scope: all active runs (same spirit as `watch --all`);
- optional `--run-id` / `--ticket` scope;
- `--interval`, `--notify-cmd`, `--bell`, and `--dry-run` support.

2. Hybrid decision pipeline per heartbeat/run
- Input: run snapshot (`status`, pending guidance, unhealthy agents, activity ages, run updated time), plus loop memory.
- Pipeline:
  - deterministic pre-checks classify clear safe actions (for example: stale-run cleanup candidates, hard guidance-required states);
  - LLM receives compact operator context and proposes an intent plus rationale;
  - deterministic policy gate validates the proposal against allowed actions.
- Output intent:
  - `escalate_guidance`
  - `auto_restart`
  - `auto_stop_stale`
  - `escalate_blocked`
  - `noop`

3. Bounded automated actions
- Restart only after confirmation threshold (>= 2 unhealthy heartbeats) and within retry budget.
- Stop stale runs only when stale criteria are met and run is not currently progressing.
- Never auto-answer guidance; always escalate explicit question context.
- LLM cannot execute arbitrary commands; it can only select from bounded intents that map to existing service APIs.

4. Escalation contract in command output + optional notify hook
- Emit structured event lines with `event`, `run_id`, `action`, `reason`.
- Reuse notify command env wiring used by `watch`.
- Record escalation summaries in workspace-authoritative ticket docs when docs authority is `workspace_active`:
  - `<workspace>/<doc_home_repo>/ttmp/.../<ticket>/changelog.md`, or
  - `<workspace>/<doc_home_repo>/ttmp/.../<ticket>/reference/02-operator-escalations.md`.

5. Keep `watch` behavior unchanged in this ticket
- `operator` is a higher-autonomy mode.
- `watch` remains a manual observability tool.

6. LLM integration mode flags
- `--llm-mode off|assist|auto` (default `assist` for initial rollout):
  - `off`: deterministic-only.
  - `assist`: LLM proposes + operator confirms/escalates, no autonomous write actions.
  - `auto`: LLM proposals may execute automatically only after deterministic gate approval.
- V1 provider/runtime: Codex CLI.

## Design Decisions

1. Separate `operator` command instead of extending `watch`.
Rationale: preserve backwards-compatible `watch` UX and keep automation opt-in.

2. Reuse service APIs for state transitions and side effects.
Rationale: existing APIs already enforce HSM transitions and persistence rules.

3. Use deterministic policy gate as final authority over LLM proposals.
Rationale: preserves safety and prevents prompt/output drift from causing invalid actions.

4. Persist restart budget and cooldown state in SQLite.
Rationale: operator process must be restartable without losing safety limits or retry history.

5. Require corroboration before restart/escalation.
Rationale: aligns with ticket reference policy and reduces false-positive churn.

6. Add explicit stale-run criteria and make thresholds configurable.
Rationale: stale-run auto-stop is high-impact; defaults should be conservative and auditable.

7. Start LLM rollout in `assist` mode, then enable `auto`.
Rationale: allows prompt and policy tuning with low operational risk.

8. Require runtime evidence before stale-run stop.
Rationale: stale candidates must be confirmed against live session evidence (`tmux has-session`, activity timestamps, pane/log exit markers) to avoid stopping active work.

9. Escalation summaries are written to workspace ticket docs in V1.
Rationale: keeps run decisions auditable where active work is happening and aligns with `workspace_active` doc authority.

## Alternatives Considered

1. Extend `watch` with `--auto`.
Rejected for now: overloads a command currently interpreted as read-only and risks surprising operators.

2. Implement operator logic inside `Service.Status`.
Rejected: mixes data rendering with side-effect orchestration and makes status reads unsafe.

3. Build a background daemon process.
Rejected for now: process lifecycle/supervision complexity is unnecessary for first operator-agent iteration.

4. LLM-only operator (no deterministic gate).
Rejected: too risky for run lifecycle actions; violates escalation and safety boundaries.

## Implementation Plan

1. Add operator policy config + defaults
- Extend `policy.Config` with operator thresholds: unhealthy confirmation count, restart budget, restart cooldown, stale-run age.
- Add LLM operator policy block: mode default, model id, timeout, max tokens, and whether autonomous execution is enabled.
- Set V1 defaults to Codex CLI with `assist` mode enabled by default.
- Validate new fields and populate defaults in `policy.Default()`.

2. Add persistent operator state in store
- Add SQLite-backed fields/table for per-run restart attempts, last restart timestamp, and cooldown window.
- Add store APIs + tests so operator state survives process restart.

3. Add operator command and loop scaffolding
- Add `operator` subcommand to command switch + usage text in `cmd/metawsm/main.go`.
- Implement loop skeleton with selector resolution (`--run-id`/`--ticket`/all), heartbeat ticker, graceful signal handling.
- Add `--llm-mode` flag and context logging hooks.

4. Implement deterministic decision engine as pure functions
- Add parse/evaluation helpers that map run snapshots to actionable decisions.
- Load/store per-run retry and cooldown counters from SQLite.
- Add stale-run verification helpers that inspect runtime evidence before auto-stop.

5. Implement LLM operator adapter (Codex CLI)
- Build compact `OperatorContext` payload from run snapshots/events.
- Define strict LLM response schema (`intent`, `target_run`, `reason`, `confidence`, `needs_human`).
- Parse/validate LLM output; reject invalid or out-of-policy responses.
- Ensure adapter captures stderr/exit status and degrades safely to deterministic-only when unavailable.

6. Wire action execution via deterministic gate
- Merge deterministic and LLM suggestions, with deterministic gate as final authority.
- Execute `service.Restart` / `service.Stop` when resulting intent is auto-action.
- Emit deterministic event output lines and optional notify command payload including `decision_source=rule|llm`.

7. Test coverage
- CLI unit tests for decision classification and command flag validation.
- LLM adapter tests for schema validation, malformed response handling, and policy-gate rejection.
- Service/loop tests for:
  - unhealthy run restarted after corroboration;
  - stale historical run auto-stopped;
  - stale candidate is not stopped when runtime evidence shows active progress/session;
  - guidance-needed always escalates;
  - restart loop is capped and escalates once budget exhausted;
  - restart budget/cooldown survives process restart;
  - `assist` mode never executes autonomous actions;
  - `auto` mode executes only allowlisted actions.

8. Documentation updates
- Update README with `metawsm operator` usage and examples.
- Document hybrid decision policy and `llm-mode` behavior.
- Document escalation-summary write path under `workspace_active` doc authority.
- Add ticket changelog entry and related file mappings in docmgr.

## Open Questions

1. Should stale-run auto-stop apply to `running` status, or only `paused/planning` in V1 once runtime evidence checks are added?

## References

- `ttmp/2026/02/08/METAWSM-007--create-orchestrator-operator-agent-for-ongoing-run-management/reference/01-operator-learnings-for-run-management-and-escalation.md`
- `cmd/metawsm/main.go`
- `cmd/metawsm/main_test.go`
- `internal/orchestrator/service.go`
- `internal/orchestrator/service_test.go`
- `internal/hsm/hsm.go`
- `internal/policy/policy.go`
- `internal/store/sqlite.go`
- `internal/store/sqlite_test.go`
