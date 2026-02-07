---
Title: Bootstrap operator playbook
Ticket: METAWSM-002
Status: active
Topics:
    - core
    - cli
DocType: playbook
Intent: long-term
Owners: []
RelatedFiles:
    - Path: README.md
      Note: Signal contract and command reference mirrored by this playbook
    - Path: cmd/metawsm/main.go
      Note: Operator commands exercised in playbook (bootstrap/status/guide/restart/cleanup/close)
    - Path: internal/orchestrator/service.go
      Note: Guidance, restart, cleanup, and close-gate handling expected by playbook
ExternalSources: []
Summary: Step-by-step operator procedure to run, guide, complete, validate, and close bootstrap runs
LastUpdated: 2026-02-07T08:27:13-08:00
WhatFor: ""
WhenToUse: ""
---



# Bootstrap operator playbook

## Purpose

Run and validate the minimum bootstrap workflow end-to-end:
1. Start from a ticket + repos.
2. Let `metawsm` collect intake details.
3. Trigger and answer guidance.
4. Mark implementation complete.
5. Verify close gates for bootstrap-specific validation.

## Environment Assumptions

- Current directory is repository root: `metawsm/`.
- `docmgr`, `wsm`, `tmux`, `sqlite3`, and `git` are installed and on `PATH`.
- Policy initialized: `.metawsm/policy.json` exists (`metawsm policy-init` if missing).
- `wsm` registry contains repo names used in `--repos`.
- You can open the created workspace paths managed by `wsm`.

## Commands

```bash
# 1) Start bootstrap
go run ./cmd/metawsm bootstrap --ticket METAWSM-002 --repos metawsm

# During prompts, provide:
# - goal
# - scope
# - done criteria (example: "go test ./... -count=1")
# - constraints
# - merge intent

# 2) Save RUN_ID from output and inspect
go run ./cmd/metawsm status --run-id RUN_ID

# 3) Find workspace name from status output (agent@workspace), then path from wsm
go run ./cmd/metawsm status --run-id RUN_ID
wsm info <WORKSPACE_NAME_FROM_STATUS>

# 4) Simulate agent guidance request
cat > <WORKSPACE_PATH>/.metawsm/guidance-request.json <<'EOF'
{
  "run_id": "RUN_ID",
  "agent": "agent",
  "question": "Should we proceed with validation-file close gate?",
  "context": "Need operator decision before finalizing flow."
}
EOF

# 5) Status should show awaiting guidance + pending question
go run ./cmd/metawsm status --run-id RUN_ID

# 6) Answer guidance
go run ./cmd/metawsm guide --run-id RUN_ID --answer "Yes, require validation-result.json before close."

# 7) Mark implementation complete (sentinel)
cat > <WORKSPACE_PATH>/.metawsm/implementation-complete.json <<'EOF'
{
  "run_id": "RUN_ID",
  "agent": "agent",
  "summary": "Bootstrap flow implemented."
}
EOF

# 8) Provide validation result required by bootstrap close gate
cat > <WORKSPACE_PATH>/.metawsm/validation-result.json <<'EOF'
{
  "run_id": "RUN_ID",
  "status": "passed",
  "done_criteria": "go test ./... -count=1"
}
EOF

# If repo tracks this file, commit or otherwise ensure clean git state.
git -C <WORKSPACE_PATH> status --porcelain

# 9) Status should now transition to completed
go run ./cmd/metawsm status --run-id RUN_ID

# 10) Preview close actions
go run ./cmd/metawsm close --run-id RUN_ID --dry-run

# Optional: restart latest run for the ticket without re-intake prompts
go run ./cmd/metawsm restart --ticket METAWSM-002

# Optional: clean up latest run for the ticket (kills tmux sessions and deletes workspace)
go run ./cmd/metawsm cleanup --ticket METAWSM-002

# Optional: keep workspace but stop agent sessions
go run ./cmd/metawsm cleanup --ticket METAWSM-002 --keep-workspaces
```

## Exit Criteria

- `status` shows guidance request when `guidance-request.json` exists.
- `guide` command writes `.metawsm/guidance-response.json` and run returns to `running`.
- After completion + validation markers, run transitions to `completed`.
- `close --dry-run` succeeds without validation/guidance-block errors.

## Notes

- `--repos` is mandatory for `bootstrap`.
- If `--ticket` does not exist in docmgr, bootstrap auto-creates it.
- Bootstrap close gates block when:
  - pending guidance requests exist,
  - `.metawsm/validation-result.json` is missing,
  - validation status is not `"passed"`,
  - validation `done_criteria` does not match run brief `done_criteria`,
  - workspace git state is dirty.
