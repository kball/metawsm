# metawsm System Guide

## What This System Does

`metawsm` orchestrates coding-agent execution across ticketed work that can span multiple repositories and workspaces.

It composes three external systems:
- `docmgr` for ticket/document lifecycle (`ttmp/`)
- `wsm` for workspace lifecycle (create/fork/reuse/merge/delete)
- `tmux` for long-running agent sessions

Operator command surface:
- `run`, `bootstrap`
- `status`, `tui`
- `guide`, `resume`, `stop`, `restart`, `iterate`
- `forum` (`ask`, `answer`, `assign`, `state`, `priority`, `close`, `list`, `thread`, `watch`)
- `merge`, `close`, `cleanup`
- `docs` (federated docmgr API aggregation + optional refresh)
- `policy-init`

## Core Architecture

### 1) Policy-Driven Control Plane

Runtime behavior is configured by `.metawsm/policy.json` (or defaults).

Key policy areas:
- workspace strategy/base branch/branch prefix
- tmux session pattern
- retry and health thresholds
- agent profiles and runner configuration
- docs topology defaults:
- `docs.authority_mode` (`workspace_active`)
- `docs.seed_mode` (`none|copy_from_repo_on_start`)
- `docs.stale_warning_seconds`
- docs API federation endpoints:
- `docs.api.workspace_endpoints[]`
- `docs.api.repo_endpoints[]`
- `docs.api.request_timeout_seconds`
- forum transport and defaults:
- `forum.topics.command_prefix|event_prefix|integration_prefix`
- `forum.redis.url|stream|group|consumer`
- `forum.sla.escalation_minutes`
- `forum.docs_sync.enabled`

### 2) Run-Level Documentation Topology

Each run carries explicit docs topology in persisted `RunSpec`:
- `doc_home_repo`
- `doc_authority_mode`
- `doc_seed_mode`
- `doc_freshness_revision`

Current contract:
- during active run, workspace copy in `doc_home_repo` is authoritative (`workspace_active`)
- on close/merge, canonical ticket history is established through normal repo merge flow

Backward compatibility:
- `--doc-home-repo` is canonical
- `--doc-repo` is retained as a legacy alias

### 3) Durable State in SQLite

`metawsm` persists orchestration state in `.metawsm/metawsm.db`.

Primary entities:
- runs, run tickets, steps, agents, events
- bootstrap run briefs
- guidance requests
- forum command-side + projection state:
- `forum_threads`, `forum_posts`, `forum_assignments`, `forum_state_transitions`
- `forum_events`, `forum_thread_views`, `forum_thread_stats`
- doc sync state (`doc_sync_states`) with per-ticket/workspace seed status + revision

This enables deterministic status rendering, restart/resume behavior, and close-time safety checks.

### 4) Plan Compilation and Execution

For each ticket, planning emits ordered steps:
- verify ticket in `docmgr`
- provision workspace via `wsm`
- optionally seed docs (`ticket_context_sync`) when `doc_seed_mode=copy_from_repo_on_start`
- start tmux session per `agent/workspace`

Important: seeding is mode-independent now (available in both `run` and `bootstrap`), controlled by seed mode.

### 5) Status and Freshness Visibility

`status` includes a dedicated Docs section:
- `home_repo`
- `authority`
- `seed_mode`
- `freshness_revision`
- per-ticket/workspace sync entries

Freshness behavior is asymmetric by design:
- stale docmgr index freshness is warning-only
- missing/unsynced workspace ticket-doc state is enforced at close (hard-fail)

### 6) Close Gates

Close path requires:
- clean workspace git state
- bootstrap validation contracts (if bootstrap run)
- synced workspace ticket-doc state for seeded runs
- presence of ticket docs under workspace doc-home repo
- clean doc-home repo state before canonical close actions

If these invariants fail, close is blocked with actionable errors.

### 7) Federation and Global Docs View

`metawsm docs` queries multiple `docmgr` API endpoints (`/api/v1/workspace/status`, `/api/v1/workspace/tickets`) and supports refresh (`POST /api/v1/index/refresh`).

Merge behavior:
- workspace endpoints preferred over repo endpoints
- repo endpoints are fallback/context
- dedupe key uses ticket + doc-home-repo + active-context semantics

Output includes:
- endpoint health/freshness
- high-level federated ticket list
- source endpoint links for drill-down

## Bootstrap Signaling Contract

For bootstrap workflows, agent/operator signaling remains file-based per workspace:
- `.metawsm/guidance-request.json`
- `.metawsm/guidance-response.json`
- `.metawsm/implementation-complete.json`
- `.metawsm/validation-result.json`

`status` ingests these files and transitions run state through HSM rules.

## Operator Workflow (Current)

1. Configure policy (`policy-init` + docs endpoint config).
2. Start run with explicit docs topology (`--doc-home-repo`, optional seed mode override).
3. Monitor with `status` / `tui`.
4. Use `docs` for federated docs visibility and optional endpoint refresh.
5. Iterate/restart as needed after review.
6. Merge completed workspace work.
7. Close run (close gates enforce doc-sync correctness).
8. Cleanup sessions/workspaces.

## Key Code Areas

- CLI commands: `cmd/metawsm/main.go`
- Orchestration runtime/planner/close gates: `internal/orchestrator/service.go`
- Models (`RunSpec`, doc sync types): `internal/model/types.go`
- Policy schema/validation: `internal/policy/policy.go`
- Persistence layer: `internal/store/sqlite.go`
- Federation client/merge: `internal/docfederation/client.go`, `internal/docfederation/merge.go`
- HSM transitions: `internal/hsm/hsm.go`

## Notes

- `README.md` is the primary quick-start command reference.
- Ticket history, design docs, and playbooks live under `ttmp/` and are managed with `docmgr`.
