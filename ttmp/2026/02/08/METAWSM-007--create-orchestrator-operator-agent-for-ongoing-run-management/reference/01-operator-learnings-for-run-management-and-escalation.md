---
Title: Operator learnings for run management and escalation
Ticket: METAWSM-007
Status: active
Topics:
    - core
    - cli
DocType: reference
Intent: long-term
Owners: []
RelatedFiles: []
ExternalSources: []
Summary: "Operational learnings and escalation policy for keeping metawsm agents moving while minimizing noisy operator interrupts."
LastUpdated: 2026-02-08T08:29:34.632551-08:00
WhatFor: "Run triage, automated recovery behavior, and human escalation boundaries."
WhenToUse: "When building an orchestrator operator-agent that supervises active metawsm runs."
---

# Operator learnings for run management and escalation

## Goal

Keep agent runs moving automatically and escalate to a human only for decisions or constraints that cannot be safely resolved by automation.

## Context

Observed during live run management:

- Many unhealthy alerts came from stale historical runs, not current work.
- A useful watcher must triage and act, not just repeat state.
- `running + unhealthy` does not always mean permanent failure; quick re-check is required.
- Human attention should be reserved for real decision points (scope, policy, environment, ownership), not routine recovery.

## Quick Reference

### Operating policy (authoritative)

1. Prefer automatic remediation for mechanical failures:
   - dead tmux session
   - stale run left in old state
   - transient DB lock/read errors
2. Escalate only when remediation is blocked or would change product intent.
3. Keep active run set small; stop/archive stale runs so alerts remain high-signal.
4. Require corroboration before escalation:
   - at least 2 consecutive unhealthy heartbeats, or
   - explicit guidance request, or
   - terminal run state (`failed`, `stopped`, `completed`, `closed`).

### Escalation decision matrix

| Signal | Auto action | Human escalation? |
|---|---|---|
| Dead agent session in active run | `metawsm restart --run-id <RUN_ID>` | No |
| Stale paused/running historical run | `metawsm stop --run-id <RUN_ID>` | No |
| Planning run cannot be stopped due FSM rule | Apply FSM fix, then stop | No |
| Guidance requested (`awaiting_guidance` or `Guidance:` section) | Wait for operator answer | Yes |
| Environment/network/tooling blocked (cannot complete done criteria) | Record blocker + keep run consistent | Yes |
| Scope or architecture change needed beyond ticket intent | Pause and request direction | Yes |
| Repeated restart loop without progress | Pause run and summarize failure evidence | Yes |

### Human-escalation contract (strict)

Escalate only when one of these is true:

1. A product/priority decision is needed.
2. A policy/safety decision is needed.
3. External capability is missing (credentials, network, package registry, infra).
4. Work clearly requires a new ticket (scope expansion, cross-team dependency, non-incidental feature request).

Do **not** escalate for:

- stale historical runs,
- one-off dead sessions,
- recoverable restart/cleanup actions,
- transient watcher/status inconsistencies that self-heal on retry.

### Recommended operator-agent loop

```text
Loop every N seconds:
1) Collect active runs (`metawsm watch --all` semantics).
2) Suppress stale/noise runs by stopping non-current dead runs.
3) For each active run:
   a) If guidance needed -> escalate to human with exact question + context.
   b) If unhealthy and recoverable -> restart once (bounded retries).
   c) If still unhealthy after retry budget -> escalate with evidence.
   d) If done -> notify operator for merge/close flow.
4) Log actions and rationale to ticket docs (diary/changelog).
```

### Command reference

```bash
# Inspect one run deeply
metawsm status --run-id <RUN_ID>

# Watch all active runs
metawsm watch --all --interval 15 --bell=false

# Recover unhealthy run
metawsm restart --run-id <RUN_ID>

# Quarantine stale run
metawsm stop --run-id <RUN_ID>

# Human guidance path
metawsm guide --run-id <RUN_ID> --answer "<decision>"
```

## Usage Examples

### Example A: noisy unhealthy alerts from old runs

1. `watch --all` shows multiple dead runs from prior dates.
2. Stop stale runs first.
3. Re-run watch.
4. Only current ticket run remains in active triage set.

### Example B: active ticket run goes unhealthy

1. `watch` reports dead session for current run.
2. Restart run once.
3. Re-check status/heartbeat.
4. If stable: no human escalation.
5. If still failing after bounded retries: escalate with concrete reason and command history.

### Example C: true human-needed case

1. Agent can compile/test Go but cannot complete frontend build due environment/network restrictions.
2. Agent records blocker and validation state in ticket docs.
3. Escalate to human because this requires environment access decision and/or follow-up ticketing.

## Related

- `ttmp/2026/02/08/METAWSM-007--create-orchestrator-operator-agent-for-ongoing-run-management/index.md`
- `ttmp/2026/02/08/METAWSM-006--build-a-go-backend-plus-web-frontend-for-metawsm/reference/01-diary.md`
