---
Title: Operator and agent commit/PR workflow
Ticket: METAWSM-009
Status: active
Topics:
    - cli
    - core
DocType: playbook
Intent: long-term
Owners: []
RelatedFiles:
    - Path: README.md
      Note: User-facing command examples and review_feedback policy fields
    - Path: cmd/metawsm/main.go
      Note: |-
        Defines auth
        Operator and review sync command behavior described in workflow
    - Path: examples/policy.example.json
      Note: Shows git_pr mode settings used for assist/auto behavior
    - Path: internal/orchestrator/service.go
      Note: |-
        Implements commit and pull-request service primitives referenced by workflow
        Commit/PR/review feedback lifecycle behavior used by workflow
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-08T15:08:30-08:00
WhatFor: Repeatable operator workflow for commit/PR automation plus PR review-feedback ingestion and dispatch.
WhenToUse: Use after a METAWSM run reaches completed state and you want to run commit/PR and review-feedback automation in assist or auto mode.
---



# Operator and agent commit/PR workflow

## Purpose

Provide a repeatable, low-risk workflow for:
- validating local auth prerequisites (Proposal A),
- preparing commit branches and commits from run workspaces,
- creating GitHub pull requests,
- ingesting PR review comments into queued iteration feedback,
- using operator assist/auto behavior for readiness signals.

## Environment Assumptions

- `metawsm`, `git`, and `gh` are installed on the operator machine.
- The run is already in `completed` status.
- Policy has `git_pr` configured (default is assist mode).
- Repositories in workspaces have reachable `origin` remotes.

### Proposal A setup (local user auth)

Run once per machine/account:

```bash
# Authenticate GitHub CLI for PR creation
gh auth login
gh auth status

# Ensure git identity is set for commits
git config --global user.name "Your Name"
git config --global user.email "you@example.com"
```

Run per ticket/run before commit/PR operations:

```bash
metawsm auth check --ticket METAWSM-009
```

## Commands

```bash
# 1) Confirm run state and review diffs/PR state
metawsm status --ticket METAWSM-009

# 2) Validate push/PR auth readiness (Proposal A)
metawsm auth check --ticket METAWSM-009

# 3) Commit workflow (preview, then execute)
metawsm commit --ticket METAWSM-009 --dry-run
metawsm commit --ticket METAWSM-009

# 4) PR workflow (preview, then execute)
metawsm pr --ticket METAWSM-009 --dry-run
metawsm pr --ticket METAWSM-009

# 5) Verify persisted PR metadata and next steps
metawsm status --ticket METAWSM-009

# 6) Sync review feedback from open PRs (preview, then execute)
metawsm review sync --ticket METAWSM-009 --dry-run
metawsm review sync --ticket METAWSM-009

# 7) Sync and dispatch queued review feedback to iterate workflow
metawsm review sync --ticket METAWSM-009 --dispatch --dry-run
metawsm review sync --ticket METAWSM-009 --dispatch
```

### Operator loop usage

Assist mode (recommendations only):

```bash
metawsm operator --ticket METAWSM-009 --dry-run
```

Auto mode (readiness actions execute when policy allows):

```bash
metawsm operator --ticket METAWSM-009
```

To enable commit/PR auto execution, set policy:

```json
{
  "git_pr": {
    "mode": "auto"
  }
}
```

## Exit Criteria

- `metawsm auth check` reports `Push ready: true` and `PR ready: true`.
- `metawsm commit` returns commit SHA(s) for targeted repo(s).
- `metawsm pr` returns PR URL/number for targeted repo(s).
- `metawsm status` `Pull Requests:` section shows `state=open` rows for the run/ticket.
- `metawsm status` `Review Feedback:` section reports queued/new/addressed counts.

## Notes

- `git_pr.mode=assist` surfaces `commit_ready`/`pr_ready` signals but does not auto-execute.
- `git_pr.review_feedback.mode=assist` surfaces `review_feedback_ready` signals but does not auto-execute.
- `git_pr.mode=off` blocks commit/PR workflows by design.
- `git_pr.review_feedback.auto_dispatch_cap_per_interval` limits auto sync/dispatch fanout per operator interval.
- Merge remains human-controlled; `metawsm` does not auto-merge.
- `metawsm commit` now natively handles stale-base dirty workspaces by snapshotting dirty changes, resetting to base, and reapplying snapshot changes.
- Commit/PR commands use a run-scoped mutation lock; overlapping non-dry-run `commit`/`pr` calls on the same run are rejected with an explicit "operation in progress" error.
- When `--actor` is omitted, actor attribution falls back to GitHub auth actor first, then git identity.
- Review feedback lifecycle is tracked as `queued -> new -> addressed` (addressed after commit + PR update path).

### Troubleshooting

- `auth check failed: push/pr not ready`
  - Re-run `gh auth login` and verify with `gh auth status`.
  - Ensure repo `origin` exists and git identity is configured.
- `git_pr.mode is off; ... workflow disabled`
  - Update policy `git_pr.mode` to `assist` or `auto`.
- `no prepared commit metadata found ... run commit first`
  - Run `metawsm commit` before `metawsm pr`.
- `review feedback sync disabled`
  - Set policy `git_pr.review_feedback.enabled=true`.
- `no queued review feedback found`
  - Re-run `metawsm review sync` first, then use `--dispatch`.
- `run <id> commit operation is already in progress`
  - Wait for the active commit/pr command to finish, then retry.
  - Avoid launching concurrent non-dry-run commit/pr commands for the same run.
- `failed to reapply workspace snapshot ... snapshot retained at stash@{...}`
  - Inspect conflicts in the workspace repo and resolve normally.
  - The stash reference is preserved; after resolving, continue from the workspace and rerun commit.
