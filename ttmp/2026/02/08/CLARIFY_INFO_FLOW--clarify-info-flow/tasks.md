# Tasks

## TODO


- [x] [Phase 1] Add run-level topology fields to model/store: doc_home_repo, doc_authority_mode=workspace_active, doc_seed_mode, doc_freshness_revision.
- [x] [Phase 1] Add/validate CLI and policy contract for run-level topology defaults and constraints (doc_home_repo must be in --repos).
- [x] [Phase 2] Generalize ticket_context_sync planning so seeding is available in both run and bootstrap modes.
- [x] [Phase 2] Implement one-way seeding only (repo -> workspace on start) and persist seed status/revision in metawsm DB.
- [x] [Phase 2] Update status/TUI output with doc authority + freshness, including warning-only stale index messaging.
- [x] [Phase 3] Implement close enforcement: hard-fail on missing/unsynced workspace ticket-doc state.
- [x] [Phase 3] Implement close behavior: ensure agent-authored workspace docs are committed/merged before canonical doc close actions.
- [ ] [Phase 4] Add docmgr API endpoint configuration for workspace and repo roots (workspace-first, repo fallback).
- [ ] [Phase 4] Build metawsm federation client to query multiple docmgr /api/v1 endpoints and normalize results.
- [ ] [Phase 4] Implement ticket merge/dedupe rules keyed by ticket + doc_home_repo + active run context.
- [ ] [Phase 4] Add metawsm-triggered refresh action calling /api/v1/index/refresh on selected docmgr endpoints.
- [ ] [Phase 4] Add minimum global aggregation view: high-level ticket list + links to per-workspace/per-repo docmgr web UIs.
- [ ] [Phase 5] Add unit/integration tests for topology validation, seeding flow, and close-gate behavior (unsynced hard-fail, stale warning-only).
- [ ] [Phase 5] Add integration tests for multi-endpoint federation, workspace-first precedence, dedupe, and refresh action behavior.
- [ ] [Phase 5] Update operator docs/playbooks with workspace-authoritative ticket workflow and federation usage.
