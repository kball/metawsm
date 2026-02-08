---
Title: 'Proposal: unify information flow across docmgr, wsm, and metawsm'
Ticket: CLARIFY_INFO_FLOW
Status: active
Topics:
    - core
    - cli
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: internal/orchestrator/service.go
      Note: Primary execution flow where doc-host selection, context sync, and close behavior are implemented
    - Path: internal/model/types.go
      Note: RunSpec and related models where ticket-topology metadata should be represented
    - Path: internal/store/sqlite.go
      Note: Persistence layer for run-level topology and sync state
    - Path: internal/policy/policy.go
      Note: Policy defaults and validation surface for documentation topology choices
    - Path: cmd/metawsm/main.go
      Note: CLI flags and operator-facing contracts that must expose topology explicitly
ExternalSources: []
Summary: "Proposal for explicit ticket information topology, including canonical doc ownership, sync behavior, and federated docmgr API aggregation for metawsm."
LastUpdated: 2026-02-08T06:28:17.900859-08:00
WhatFor: ""
WhenToUse: ""
---

# Proposal: unify information flow across docmgr, wsm, and metawsm

## Executive Summary

Define an explicit information-topology contract for each run:

1. Every ticket has one canonical documentation home repo (`doc_home_repo`).
2. During an active run, the workspace copy in `<workspace>/<doc_home_repo>/ttmp` is the writable source of truth for agents.
3. `metawsm` remains the run-level operational source of truth for lifecycle and health.
4. On merge/close, workspace doc changes become canonical repo history through normal workspace merge flow.
5. Sync/freshness behavior is explicit, versioned, and visible in status output.

This keeps `docmgr` repo-scoped semantics intact while making multi-repo workspaces and multi-workspace runs predictable.

## Problem Statement

Today, multi-repo and multi-workspace orchestration works operationally, but information ownership is implicit:

- A single `DocRepo` is selected per run, but not modeled as a first-class ticket topology rule.
- Bootstrap performs context sync; standard runs do not.
- Workspace ticket docs can drift from parent-repo ticket docs.
- Operator visibility is strong for execution state but weaker for documentation sync state.

Without explicit topology, behavior is hard to reason about when tickets span multiple repos and runs.

Additionally, metawsm needs queryable ticket state across active repos/workspaces. `docmgr` exposes this via HTTP API, but each API instance is rooted to a single docs root. A federation pattern is required to build a live cross-repo ticket view.

Current proposal drafts also risked an inversion where parent-repo docs were treated as runtime authority. The intended operator model is the opposite: agents should own and continuously update ticket docs in the active workspace while implementing.

## Proposed Solution

### A. Make ticket information topology explicit

Add first-class run metadata (run-level for v1):
- `doc_home_repo`: canonical repo for ticket docs for this run.
- `doc_authority_mode`: `workspace_active`.
- `doc_seed_mode`: `none | copy_from_repo_on_start`.
- `doc_freshness_revision`: monotonic timestamp/hash for last successful seed/refresh.

### B. Define source-of-truth contract

- Active run ticket truth: workspace `doc_home_repo` copy (agent-writable, operator-reviewed).
- Between runs / after close: canonical truth in merged repo history for `doc_home_repo`.
- Run execution truth: `metawsm` SQLite (`runs/steps/agents/events/briefs/guidance`).

### C. Unify workspace doc seeding behavior across run modes

Move ticket context initialization from bootstrap-only behavior to a generalized step policy:
- include seed/sync step whenever `doc_seed_mode=copy_from_repo_on_start`,
- support standard and bootstrap runs consistently,
- record seed/sync status and revision in DB.

### D. Surface topology and doc authority in status

Extend status output with:
- canonical doc home repo per ticket,
- active authority mode (`workspace_active`),
- last seed/sync revision and freshness,
- warnings when workspace doc state is unavailable or stale.

### E. Keep close semantics canonical

Close path should preserve workspace-authoritative run behavior and canonicalize on merge:
- merge workspaces as today,
- ensure ticket docs/tasks/changelog updates done by agents are committed in workspace doc home repo before close,
- close/update ticket docs in canonical doc home context after merge,
- fail with actionable message if workspace doc state is missing/stale beyond policy.

### F. (Optional later) global run aggregation

For org-level visibility across multiple metawsm roots, add an aggregator mode later (`metawsm status --all-roots`) that reads multiple DB roots. This is out-of-scope for the first topology contract.

### G. Add federated docmgr API aggregation for metawsm

Use `docmgr` as the per-ticket content authority and expose it to metawsm by federation:

- Run one `docmgr api serve` instance per active workspace doc root (at least for each `<workspace>/<doc_home_repo>/ttmp`) so operators can review what agents are actively updating.
- Optionally run baseline repo-level docmgr APIs for non-active context.
- In metawsm, add a doc federation client that queries all configured workspace/repo API endpoints and merges results into a single operator view.
- Use canonical ownership (`doc_home_repo`) to de-duplicate ticket data:
  - for active runs, workspace endpoint for `doc_home_repo` wins,
  - repo-level endpoints are fallback/context when workspace endpoints are absent.
- Track API freshness (`indexedAt`) and refresh policy so "live state" is explicit, not assumed.

This makes docmgr browseable/queryable by metawsm without changing docmgr's repo-rooted model.

## Design Decisions

1. Keep one canonical doc home repo per ticket per run.
Reason: avoids split-brain ticket history while preserving docmgr's repo-root model.

2. Treat active workspace docs as authoritative during a run.
Reason: that is where agents work and where operators review/guide ongoing execution.

3. Generalize sync beyond bootstrap mode.
Reason: run mode should not change fundamental information correctness.

4. Persist sync metadata in metawsm DB.
Reason: operators need auditable, queryable sync state in the same control plane as lifecycle.

5. Preserve existing tool boundaries.
Reason: do not reimplement docmgr/wsm internals; orchestrate them with explicit contracts.

6. Federate docmgr APIs rather than centralizing docs into one new store.
Reason: keeps docmgr as source of ticket knowledge while giving metawsm a cross-repo read model.

## Resolved Decisions (2026-02-08)

1. Seeding is one-way only in v1.
- Use repo -> workspace seeding on start.
- Do not implement workspace -> repo push-back sync beyond normal merge/close.

2. Topology granularity is run-level in v1.
- No per-ticket topology overrides for now.

3. Minimum global aggregation interface is lightweight.
- Show a high-level cross-repo ticket view.
- Provide links to specific docmgr web interfaces for drill-down.

4. Refresh is triggerable by metawsm when needed.
- metawsm may call docmgr `/api/v1/index/refresh` when operator requests freshness.
- Freshness metadata (`indexedAt`) should still be displayed to operators.

5. Close-gate defaults are asymmetric in v1.
- Hard block close on unsynced/missing workspace ticket-doc state.
- Warn (do not block) on stale docmgr index freshness by default.
- Add optional strict mode later if stale freshness should block close.

## Alternatives Considered

1. Run docmgr independently in every repo for every ticket.
Rejected: duplicates ticket state and creates reconciliation burden.

2. Keep current implicit first-repo behavior.
Rejected: simple but ambiguous; drift and ownership confusion persist.

3. Replace docmgr with a metawsm-native ticket store.
Rejected: high scope and duplicates mature capabilities already provided by docmgr.

4. Keep parent-repo docs as runtime authority and treat workspace docs as read-only mirrors.
Rejected: inverts actual operating workflow where agents update ticket state in workspace during implementation.

5. Build a new metawsm-native ticket API and bypass docmgr API.
Rejected: duplicates ticket/document query concerns already implemented by docmgr.

## Implementation Plan

1. Model updates:
- Extend `RunSpec` and persistence schema for topology/sync metadata.

2. CLI/policy contract:
- Add explicit flags and defaults for `doc_home_repo`, `doc_authority_mode`, `doc_seed_mode`.
- Validate that `doc_home_repo` is one of `--repos`.

3. Planner changes:
- Emit a generalized `ticket_context_sync` step when seeding is enabled for both run modes.

4. Executor changes:
- Record seed/sync revision/status in DB on success/failure.
- Keep one-way copy semantics only (repo -> workspace on start for v1).

5. Status/UI changes:
- Display topology, authority mode, and doc freshness sections.
- Warn when workspace docmgr index freshness is stale.

6. Close-flow enforcement:
- Hard-fail close when workspace ticket-doc authority is missing or unsynced (for example seed/sync failed or required ticket docs absent).
- Ensure agent-authored workspace docs are present/committed before close.
- Ensure doc close/update operations use canonical doc home context after merge.
- Keep stale-index freshness as warning-only by default (non-blocking).

7. docmgr API federation:
- Add workspace/repo docmgr API endpoint configuration in metawsm policy/runtime config.
- Implement metawsm aggregation client that queries workspace-first then repo fallback `/api/v1/*` endpoints and normalizes responses.
- Add merge rules keyed by `ticket` + `doc_home_repo` + run activity to avoid duplicate/conflicting ticket records.
- Implement a minimum global view that lists tickets and links to per-repo/per-workspace docmgr web UIs.
- Add explicit refresh action in metawsm that calls `/api/v1/index/refresh` on selected endpoints.
- Expose federation health in status/web surfaces (endpoint reachable, last indexedAt, stale).

8. Tests:
- Unit tests for topology validation.
- Plan tests for mode-independent sync step emission.
- Integration tests for canonical close behavior and sync-status reporting.
- Integration tests for multi-endpoint federation merge and de-dup behavior.

9. Migration:
- Backward compatible defaults map existing `DocRepo` to `doc_home_repo`.
- Existing runs continue functioning without new flags.
- Federation can be opt-in at first; single-repo behavior remains default.

## Open Questions

None currently.

## References

- `ttmp/2026/02/08/CLARIFY_INFO_FLOW--clarify-info-flow/reference/01-analysis-repo-workspace-run-information-flow.md`
- `ttmp/2026/02/07/METAWSM-004--bootstrap-workspace-context-handoff-and-nested-repo-ambiguity/reference/01-analysis-parent-to-workspace-information-flow-via-docmgr.md`
- `internal/orchestrator/service.go`
- `internal/model/types.go`
- `internal/store/sqlite.go`
