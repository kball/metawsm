---
Title: 'Analysis: current control flow for tickets, agents, and communication'
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
      Note: Watch and operator parsing and direction hints
    - Path: internal/hsm/hsm.go
      Note: Run transition legality used by control flow
    - Path: internal/model/types.go
      Note: Run and guidance state models
    - Path: internal/orchestrator/service.go
      Note: Current run lifecycle and bootstrap signal ingestion
    - Path: internal/store/sqlite.go
      Note: Guidance and forum schema definitions
    - Path: internal/store/sqlite_forum.go
      Note: Forum command and event persistence behavior
ExternalSources: []
Summary: Current-state analysis of metawsm control flow and the communication paths used by tickets, agents, watch, and operator loops.
LastUpdated: 2026-02-08T19:12:00-08:00
WhatFor: Understand how run lifecycle and agent communication work today before replacing communication paths with forum-first flows.
WhenToUse: Before designing or implementing forum-first communication migration.
---


# Analysis: current control flow for tickets, agents, and communication

## Goal

Describe the implemented control flow for run/ticket/agent execution and identify all current communication channels that coordinate agent and operator behavior.

## Context

The codebase currently runs two overlapping communication models:

1. bootstrap file signals in workspace `.metawsm/*.json` files;
2. forum threads/events persisted in SQLite and surfaced through `metawsm forum ...`.

Because both exist, watch/operator guidance logic and close gates currently mix file state, DB state, and status-text parsing.

## Quick Reference

### Run and agent control flow

| Layer | Current behavior | Key code |
|---|---|---|
| Run creation | `Run()` validates inputs/policy, persists run/spec, transitions `created -> planning -> running`, compiles and executes plan steps | `internal/orchestrator/service.go` |
| Plan compilation | Per ticket: verify `docmgr` ticket, create/fork/reuse workspace, optional ticket context sync, tmux start per agent | `internal/orchestrator/service.go` (`buildPlan`) |
| Runtime transitions | HSM guards run/step/agent transitions | `internal/hsm/hsm.go` |
| Agent process runtime | Each agent runs in tmux session; health derived from tmux state + activity/progress timestamps | `internal/orchestrator/service.go` (`executeSingleStep`, `evaluateHealth`) |
| Watch/operator input model | `watch`/`operator` call `status`, then parse plaintext output into a snapshot | `cmd/metawsm/main.go` (`loadWatchSnapshot`, `parseWatchSnapshot`) |

### Communication channels currently used

| Channel | Purpose | Writer | Reader |
|---|---|---|---|
| `.metawsm/guidance-request.json` | agent requests guidance | agent prompt contract | `Status()` via `syncBootstrapSignals()` |
| `.metawsm/guidance-response.json` | operator answer handoff | `metawsm guide` | agent runtime | 
| `.metawsm/implementation-complete.json` | completion signal | agent prompt contract | `Status()` via `syncBootstrapSignals()` |
| `.metawsm/validation-result.json` | bootstrap validation gate | agent prompt contract | `ensureBootstrapCloseChecks()` |
| `guidance_requests` table | normalized pending/answered guidance queue | `syncBootstrapSignals()` and `Guide()` | `Status()`, `Guide()` |
| `forum_*` tables + `forum_events` | forum thread lifecycle + event log | `metawsm forum ...` / `service_forum.go` | `Status()` escalation summary, forum CLI queries |
| status text lines | event classification and direction hints | `Status()` formatter | `watch` / `operator` parser |

### Bootstrap-specific control communication path

1. Agent writes `.metawsm/guidance-request.json` and/or completion/validation files.
2. Operator/watch calls `status` (polling).
3. `Status()` calls `syncBootstrapSignals()`, which reads files, upserts pending guidance rows, and transitions run to `awaiting_guidance` or `completed`.
4. `metawsm guide --answer` writes `.metawsm/guidance-response.json`, removes guidance-request files, marks DB guidance row answered, and transitions `awaiting_guidance -> running`.
5. Close gates require zero pending guidance rows plus passing validation file content.

### Forum control communication path (current)

1. Actor invokes `metawsm forum ask|answer|assign|state|priority|close`.
2. Service validates state transitions and writes directly to SQLite command/read/event tables.
3. `Status()` summarizes forum queue and injects escalation-worthy threads into `Guidance:` lines.
4. `watch`/`operator` treat those lines as guidance-needed alerts.

### Observed coupling and constraints

- Communication is split across filesystem signals, relational tables, and parsed status text.
- `forum.redis.*` and topic prefixes are configured/validated in policy, but forum transport remains in-process direct writes (no Redis/Watermill runtime path yet).
- Bootstrap close correctness still depends on file artifacts, not forum state.
- Agent base prompt still instructs file writes, so agent behavior bypasses forum unless explicitly told otherwise.
- `watch`/`operator` are coupled to status string format (`parseWatchSnapshot`), not a typed snapshot API.

## Usage Examples

### Where to read current run communication state

- `go run ./cmd/metawsm status --run-id <RUN_ID>`
- `go run ./cmd/metawsm forum list --run-id <RUN_ID>`
- `go run ./cmd/metawsm forum watch --ticket <TICKET>`

### Where control decisions are currently enforced

- Run transition legality: `internal/hsm/hsm.go`
- Bootstrap signal ingestion: `internal/orchestrator/service.go` (`syncBootstrapSignals`)
- Guide answer side effects: `internal/orchestrator/service.go` (`Guide`)
- Forum transition invariants: `internal/orchestrator/service_forum.go`
- Watch/operator alert derivation: `cmd/metawsm/main.go` (`parseWatchSnapshot`, `classifyWatchEvent`, `buildOperatorRuleDecision`)

## Related

- `ttmp/2026/02/08/METAWSM-008--build-metawsm-managed-q-a-forum-for-agent-operator-human-collaboration/design-doc/01-implementation-plan-for-metawsm-q-a-forum.md`
- `docs/system-guide.md`
- `README.md`
