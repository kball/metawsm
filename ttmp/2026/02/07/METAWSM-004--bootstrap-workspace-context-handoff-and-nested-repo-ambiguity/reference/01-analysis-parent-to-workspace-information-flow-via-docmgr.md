---
Title: 'Analysis: Parent-to-workspace information flow via docmgr'
Ticket: METAWSM-004
Status: active
Topics:
    - core
    - cli
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: Bootstrap intake and docmgr ticket/brief creation in parent process
    - Path: internal/orchestrator/service.go
      Note: Plan execution
    - Path: internal/store/sqlite.go
      Note: Run brief persistence in parent sqlite store
ExternalSources: []
Summary: Map of bootstrap information flow between parent metawsm repo and spawned workspace, including required docmgr file copy handoff.
LastUpdated: 2026-02-07T09:18:21-08:00
WhatFor: ""
WhenToUse: ""
---


# Analysis: Parent-to-workspace information flow via docmgr

## Goal

Provide a precise map of how bootstrap context is created in the parent repository and what does or does not make it into a spawned workspace.

## Context

Current bootstrap runs start from the parent repo (for example `/Users/kball/workspaces/2026-02-07/metawsm/metawsm`) and then create a workspace root (for example `/Users/kball/workspaces/2026-02-07/metawsm-003-0260207-090419`) that contains one or more repo directories inside it (for example `metawsm/`).

The agent is launched from the workspace root, while most docmgr write operations happen in the parent process context.

### Required invariant (operator-provided)

If docmgr files are ticket-specific, they must be copied into the spawned workspace before the agent starts.

## Quick Reference

### End-to-end flow

1. Operator runs `metawsm bootstrap --ticket ... --repos ...` from parent repo.
2. Parent process collects intake answers into `RunBrief` (`goal/scope/done/constraints/merge intent`).
3. Parent process checks/creates ticket via docmgr (`docmgr list tickets`, `docmgr ticket create-ticket`).
4. Parent process executes orchestration steps:
   - `docmgr ticket list --ticket ...`
   - `wsm create ...`
   - `tmux new-session ...`
5. Parent process writes bootstrap brief doc with `docmgr doc add --doc-type reference`.
6. Agent starts in workspace root and is told to read ticket docs in "this workspace".

### Artifact flow table

| Artifact | Produced by | Stored in | Consumed by | Gap |
|---|---|---|---|---|
| Ticket workspace (`ttmp/.../METAWSM-###`) | Parent `docmgr ticket create-ticket` | Parent repo working tree | Humans, parent tooling | Must be copied/synced into spawned workspace for ticket-specific context |
| Bootstrap brief reference doc | Parent `createBootstrapBriefDoc` | Parent `ttmp/.../reference/...` | Humans, expected agent context | Created after workspace + tmux start; no explicit handoff into workspace |
| Run brief structured data | Parent `store.UpsertRunBrief` | Parent `.metawsm/metawsm.db` | Parent `status`, close checks | Not available in workspace unless explicitly copied/exported |
| Step plan (`docmgr ticket list`, `wsm create`, `tmux_start`) | Parent `buildPlan` | Parent DB (`steps`) | Parent orchestrator | Agent cannot directly inspect unless separately exported |
| Guidance request/response files | Agent + parent guide command | Workspace `.metawsm/*.json` | Parent `status`/`guide` | One-way contract works for runtime signals, but not for initial context handoff |

### Exact current handoff mismatch

- Docmgr ticket/brief content is authored in parent repo context.
- Tmux agent starts at workspace root path (`tmux ... -c <workspacePath>`), not necessarily repo root (`<workspacePath>/metawsm`).
- No bootstrap step currently copies or exports ticket/brief context from parent into workspace before agent execution.

Result: agent can start with incomplete or ambiguous context even though docs exist in parent ticket tree.

### Candidate remediation options

1. Make ticket-doc copy a required bootstrap step before `tmux_start`:
   - copy `ttmp/<ticket-path>/` from parent repo into workspace repo path,
   - preserve relative paths so agent prompt can use a stable location.
2. Add explicit context sync artifact:
   - export run brief + copied doc path pointers into `<workspace>/.metawsm/context.json`.
3. Start agent in repo root when there is exactly one repo:
   - use `<workspace>/<repo>` as working directory instead of workspace container root.
4. Add validation gate for context presence:
   - fail step 3 if required ticket doc files are missing from workspace copy target.

## Usage Examples

### Inspect parent-side ticket docs for a run ticket

```bash
docmgr ticket list --ticket METAWSM-003
docmgr doc list --ticket METAWSM-003
```

### Inspect workspace runtime context files

```bash
ls -la /Users/kball/workspaces/2026-02-07/metawsm-003-0260207-090419/.metawsm
```

### Compare where data lives today

```bash
# Parent run brief store
sqlite3 .metawsm/metawsm.db "select run_id,ticket,goal,scope,done_criteria from run_briefs order by updated_at desc limit 3;"

# Workspace files actually visible to agent
find /Users/kball/workspaces/2026-02-07/metawsm-003-0260207-090419 -maxdepth 3 -type f | rg "ttmp|\.metawsm"
```

## Related

- This analysis is for ticket `METAWSM-004` and describes the current state before implementation changes.
