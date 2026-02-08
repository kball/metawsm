# Changelog

## 2026-02-08

- Initial workspace created


## 2026-02-08

Step 1: Added run-level doc topology contract, generalized seed planning, persisted doc sync state/revision, and status freshness warnings (commit d66e1ed8defc1b75614ecc4930c30652745f0d5c).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Implements seed-mode planning and status output
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite.go — Adds doc_sync_states persistence


## 2026-02-08

Step 2: Added close-time hard gates for missing/unsynced workspace doc state and doc-home cleanliness (commit 6d5505261c97fad6dcb0244080f2a5e963f33ea4).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Adds ensureWorkspaceDocCloseChecks close gating
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Adds/updates close-gate tests


## 2026-02-08

Step 3: Added docmgr API endpoint config, federation client, workspace-first dedupe, refresh action, and global docs view via metawsm docs (commit ecd9617eb6e3b995ce206422899c0c0c40bbf6e4).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Adds metawsm docs command for aggregation and refresh
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/docfederation/client.go — Queries workspace/status+tickets and refresh endpoints
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/docfederation/merge.go — Implements workspace-first merge and dedupe key rules


## 2026-02-08

Step 4: Added Phase 5 test coverage (stale warning + federation merge/selection), updated operator docs, and added workspace-authoritative federation playbook (commit 86c3654c2b6668cf5ed751e3f15f8a6423d5bccf).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/README.md — Operator command workflow updates
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/examples/policy.example.json — docs.api endpoint examples
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/docfederation/client_test.go — Federation dedupe/refresh coverage
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Stale warning-only coverage

