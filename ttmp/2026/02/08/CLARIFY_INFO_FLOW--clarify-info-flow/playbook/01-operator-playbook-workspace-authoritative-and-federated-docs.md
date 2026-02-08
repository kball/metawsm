---
Title: 'Operator playbook: workspace-authoritative and federated docs'
Ticket: CLARIFY_INFO_FLOW
Status: active
Topics:
    - core
    - cli
DocType: playbook
Intent: long-term
Owners: []
RelatedFiles:
    - Path: README.md
      Note: Operator-facing workflow and command documentation
    - Path: cmd/metawsm/main.go
      Note: docs command and topology flags
    - Path: internal/docfederation/client.go
      Note: Federation endpoint query and refresh behavior
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-08T07:19:16.394395-08:00
WhatFor: ""
WhenToUse: ""
---


# Operator playbook: workspace-authoritative and federated docs

## Purpose

Run tickets with workspace-authoritative docs, verify doc freshness/sync state, and use federated docmgr APIs for cross-workspace/repo operator visibility.

## Environment Assumptions

- `metawsm`, `docmgr`, and `wsm` are installed and on `PATH`.
- `.metawsm/policy.json` includes `docs.api.workspace_endpoints` and `docs.api.repo_endpoints`.
- `docmgr api serve` is running for each configured endpoint base URL.
- Target ticket exists in docmgr.

## Commands

```bash
# 1) Start run with explicit doc-home ownership and seeding
go run ./cmd/metawsm run \
  --ticket CLARIFY_INFO_FLOW \
  --repos metawsm,workspace-manager \
  --doc-home-repo metawsm \
  --doc-authority-mode workspace_active \
  --doc-seed-mode copy_from_repo_on_start

# 2) Verify run docs topology + freshness in status
go run ./cmd/metawsm status --ticket CLARIFY_INFO_FLOW

# 3) Inspect federated docs view (workspace-first, repo fallback)
go run ./cmd/metawsm docs --policy .metawsm/policy.json

# 4) Trigger refresh on selected endpoint(s) when needed
go run ./cmd/metawsm docs --policy .metawsm/policy.json --refresh --endpoint workspace-metawsm

# 5) Complete operator loop
go run ./cmd/metawsm iterate --ticket CLARIFY_INFO_FLOW --feedback "Address remaining review comments."
go run ./cmd/metawsm merge --ticket CLARIFY_INFO_FLOW --dry-run
go run ./cmd/metawsm merge --ticket CLARIFY_INFO_FLOW
go run ./cmd/metawsm close --ticket CLARIFY_INFO_FLOW
```

## Exit Criteria

- `status` output includes Docs topology (`home_repo`, `authority`, `seed_mode`) and no sync failures.
- `metawsm docs` returns endpoint health and aggregated ticket list with links.
- `close` succeeds without doc sync or workspace ticket-doc gate failures.
- Ticket docs/changelog/tasks are updated and committed in the workspace doc-home repo before close.

## Notes

- Close is hard-blocked for missing/unsynced workspace ticket-doc state.
- Stale freshness is warning-only in status output by default.
- `--doc-repo` remains supported as an alias for `--doc-home-repo`.
