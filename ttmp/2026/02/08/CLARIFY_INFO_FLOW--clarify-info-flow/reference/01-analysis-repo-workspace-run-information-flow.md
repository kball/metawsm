---
Title: 'Analysis: repo/workspace/run information flow'
Ticket: CLARIFY_INFO_FLOW
Status: active
Topics:
    - core
    - cli
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: internal/orchestrator/service.go
      Note: Run/doc-repo/workspace planning, context sync, guidance signal handling, and close gates
    - Path: internal/model/types.go
      Note: RunSpec and RunBrief fields that define current information boundaries
    - Path: internal/store/sqlite.go
      Note: Durable run-level state that metawsm uses for cross-workspace visibility
    - Path: internal/policy/policy.go
      Note: Agent profile and policy resolution behavior that influences where context is consumed
    - Path: cmd/metawsm/main.go
      Note: CLI contract for --repos, --doc-repo, bootstrap intake, and operator commands
ExternalSources: []
Summary: "Current-state analysis of information ownership and flow between repo-scoped docmgr, workspace-scoped wsm, and run-scoped metawsm, including the desired workspace-authoritative run model."
LastUpdated: 2026-02-08T06:28:17.900821-08:00
WhatFor: ""
WhenToUse: ""
---

# Analysis: repo/workspace/run information flow

## Goal

Provide a precise current-state map for information ownership and movement across `docmgr`, `wsm`, and `metawsm`, and compare that to the desired model before designing changes.

## Context

The system operates at three scopes with different owners:

- Repo scope (`docmgr`): ticket docs/tasks/changelog under repo-local `ttmp/`.
- Workspace scope (`wsm`): one workspace can include multiple repos and shared execution context.
- Run scope (`metawsm`): one run can span many tickets/workspaces and tracks orchestration state in SQLite.

The core challenge is that these scopes do not align naturally:
- `docmgr` is per repo,
- `wsm` is multi-repo per workspace,
- `metawsm` is multi-workspace/multi-ticket per run.

That forces explicit choices about canonical documentation location, replication, and aggregation.

Desired operating model clarification:
- During active implementation, agents own and update ticket docs in the active workspace.
- Operators primarily review those live workspace docs and guide agents.
- Canonical repo history is established when workspace changes are merged/closed.

## Quick Reference

### Layer model: desired semantics

| Layer | Scope | Owner | Desired source of truth |
|---|---|---|---|
| Repo docs | one repo | `docmgr` | canonical ticket history between runs (post-merge) |
| Workspace runtime | many repos in one workspace | `wsm` + agent process | execution context and working tree state |
| Workspace docs | `<workspace>/<doc_home_repo>/ttmp` | agent + operator loop | active ticket truth during the run |
| Run orchestration | many workspaces/tickets | `metawsm` | operator view of run/agent/step lifecycle |

### Layer model: implemented semantics (today)

| Layer | Current implementation |
|---|---|
| Repo docs | Still repo-scoped. `docmgr` operates relative to one root at a time. |
| Workspace runtime | `wsm` creates/forks workspaces with multiple repos; `metawsm` starts agent in selected doc repo path. |
| Run orchestration | `metawsm` stores cross-workspace state in `.metawsm/metawsm.db` (runs, steps, agents, events, briefs, guidance). |

### Current flow in metawsm

1. Operator starts `run` or `bootstrap`.
2. `metawsm` resolves repos and chooses one `DocRepo` (default first repo, overridable by `--doc-repo`).
3. Plan runs: verify ticket, create/fork/reuse workspace, start tmux sessions.
4. In bootstrap only: ticket docs are copied into `<workspace>/<docRepo>/ttmp/...` before tmux start.
5. Runtime signals are written/read from `<workspace>/.metawsm/*.json`.
6. `status` combines DB state + tmux health + workspace git diffs.
7. `close` merges workspace repos and runs `docmgr ticket close`.

### Desired vs current: gap matrix

| Concern | Desired | Current | Gap |
|---|---|---|---|
| Ticket documentation ownership in multi-repo workspaces | Active run authority in workspace `doc_home_repo`; canonicalized on merge | Implicit single `DocRepo` per run; semantics not explicit | Ownership exists but runtime authority semantics are not formalized |
| Context sync behavior | Consistent across modes | Bootstrap sync exists; non-bootstrap sync does not | Inconsistent behavior by run mode |
| Repo docs vs workspace docs | Explicit run-time authority and handoff contract | One-time copy in bootstrap path | Handoff contract is unclear; drift/confusion risk remains |
| Cross-workspace/run operator visibility | Unified metawsm run ledger | Present in a single DB | No federated/global view across multiple metawsm roots |
| Ticket closure semantics in multi-repo | Clear canonical close/update path | `close` uses one docmgr context | Works but contract is implicit, not explicit |

### Practical implications

- For a cross-repo ticket, docs are effectively hosted in one repo path in workspace during the run.
- Agents can and should update structured ticket docs there as part of implementation flow.
- `metawsm` does have a useful top-level operational truth (run/agent/step state), but it is not yet a full ticket knowledge graph across many repo roots.

## Usage Examples

### Example 1: diagnose where ticket truth lives for a run

```bash
go run ./cmd/metawsm status --ticket CLARIFY_INFO_FLOW
```

Interpretation:
- lifecycle/health truth comes from `metawsm` DB + tmux
- ticket doc truth comes from the run's selected `DocRepo`

### Example 2: inspect docs ownership and workspace context

```bash
docmgr ticket list --ticket CLARIFY_INFO_FLOW
go run ./cmd/metawsm status --ticket CLARIFY_INFO_FLOW
```

Use this to determine:
- where canonical ticket docs are rooted,
- which workspace/repo paths currently carry runtime changes.

### Example 3: identify potential drift risk

For bootstrap runs, if ticket docs are copied into workspace and then parent docs continue changing independently, drift is possible without explicit re-sync.

## Related

- `ttmp/2026/02/08/CLARIFY_INFO_FLOW--clarify-info-flow/design-doc/01-proposal-unify-information-flow-across-docmgr-wsm-and-metawsm.md`
- `ttmp/2026/02/07/METAWSM-004--bootstrap-workspace-context-handoff-and-nested-repo-ambiguity/reference/01-analysis-parent-to-workspace-information-flow-via-docmgr.md`
- `ttmp/2026/02/07/METAWSM-002--first-bootstrap-flow/design-doc/01-minimum-bootstrap-flow.md`
