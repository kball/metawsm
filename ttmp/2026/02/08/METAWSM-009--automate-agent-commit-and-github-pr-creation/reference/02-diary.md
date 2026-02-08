---
Title: Diary
Ticket: METAWSM-009
Status: active
Topics:
    - cli
    - core
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: examples/policy.example.json
      Note: |-
        Added git_pr policy block defaults for Proposal A rollout (commit d3f13f6)
        Added git_pr policy defaults (commit d3f13f6)
    - Path: internal/model/types.go
      Note: |-
        Added RunPullRequest model and pull request state enums (commit d3f13f6)
        Added RunPullRequest model and PR state enums (commit d3f13f6)
    - Path: internal/policy/policy.go
      Note: |-
        Added git_pr config schema/defaults/validation (commit d3f13f6)
        Added git_pr policy contract defaults/validation (commit d3f13f6)
    - Path: internal/policy/policy_test.go
      Note: |-
        Added tests for git_pr defaults and validation failures (commit d3f13f6)
        Added git_pr validation coverage (commit d3f13f6)
    - Path: internal/store/sqlite.go
      Note: |-
        Added run_pull_requests schema and store CRUD methods (commit d3f13f6)
        Added run_pull_requests schema and store methods (commit d3f13f6)
    - Path: internal/store/sqlite_test.go
      Note: |-
        Added persistence test for run pull request metadata across reopen (commit d3f13f6)
        Added run_pull_requests persistence test (commit d3f13f6)
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-08T11:22:26-08:00
WhatFor: Implementation diary for METAWSM-009 commit/PR automation rollout.
WhenToUse: Use when reviewing what was implemented, how it was validated, and what failed during each step.
---


# Diary

## Goal

Capture each implementation step for METAWSM-009 with exact code changes, test outcomes, failures, and review guidance.

## Step 1: Add git_pr Policy Contract and Persisted Run PR Metadata

This step established the foundation needed before command-level automation: a policy contract for commit/PR behavior and durable storage for per-run PR metadata. Without these two pieces, later CLI and operator actions would have no validated configuration surface or persisted state.

I implemented defaults and validation for Proposal A (`local_user_auth`) and added a new SQLite table/API for run-scoped PR records keyed by run/ticket/repo. This keeps future commit/PR actions auditable and restart-safe.

### Prompt Context

**User prompt (verbatim):** "ok go ahead and implement. Keep a diary and commit as you go."

**Assistant interpretation:** Start implementing METAWSM-009 now, break work into incremental commits, and keep documentation of each step in a diary.

**Inferred user intent:** Move from planning docs to real product changes for agent commit/PR automation while preserving traceability and reviewability.

**Commit (code):** d3f13f6 — "Add git_pr policy and persisted run PR metadata"

### What I did
- Added `git_pr` policy block to `internal/policy/policy.go` with:
  - `mode` (`off|assist|auto`)
  - `credential_mode` (`local_user_auth`)
  - `branch_template`
  - `require_all`
  - `required_checks`, `allowed_repos`, `default_labels`, `default_reviewers`
- Set defaults for Proposal A in `policy.Default()` and added strict validation checks in `policy.Validate()`.
- Added `RunPullRequest` and `PullRequestState` model types in `internal/model/types.go`.
- Added `run_pull_requests` table plus store methods in `internal/store/sqlite.go`:
  - `UpsertRunPullRequest`
  - `ListRunPullRequests`
- Added policy tests in `internal/policy/policy_test.go` for:
  - default values
  - invalid mode/credential mode
  - empty branch template
- Added store persistence coverage in `internal/store/sqlite_test.go` with reopen verification.
- Updated `examples/policy.example.json` to include the `git_pr` block.
- Ran focused tests for modified packages.

### Why
- Commit/PR automation needs validated policy to gate behavior safely.
- PR metadata persistence is required for auditability, operator visibility, and restart recovery.

### What worked
- `internal/policy` tests passed with the new `git_pr` schema and validation rules.
- `internal/store` tests passed with `run_pull_requests` persistence and reload behavior.
- Existing code paths remained compatible because missing `git_pr` fields still resolve through `policy.Default()`.

### What didn't work
- Initial test commands failed in sandbox due Go cache permissions.
- Command: `go test ./internal/policy -count=1`
- Error:
  - `open /Users/kball/Library/Caches/go-build/...: operation not permitted`
- Command: `go test ./internal/store -count=1`
- Error:
  - `open /Users/kball/Library/Caches/go-build/...: operation not permitted`
- Fix: reran with local cache overrides:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/store -count=1`

### What I learned
- The existing store/test architecture made it straightforward to add another persisted run-scoped state table without a migration framework change.
- Policy extensibility is easiest when defaults are complete and validation rejects partial/ambiguous states early.

### What was tricky to build
- Choosing a `git_pr` surface that is strict enough for safety but still forward-compatible (especially validation list fields and credential mode constraints).

### What warrants a second pair of eyes
- Whether `git_pr.mode` default should remain `assist` vs `off` for first release.
- Whether `run_pull_requests` should include additional identity fields now (for example PR head repo/fork metadata) before CLI wiring begins.

### What should be done in the future
- Wire this policy + persisted state into service and CLI command paths (`commit`, `pr`, and `auth check`) in the next step.

### Code review instructions
- Start in `internal/policy/policy.go`:
  - `Config.GitPR` shape
  - defaults in `Default()`
  - checks in `Validate()`
- Then review `internal/store/sqlite.go`:
  - schema for `run_pull_requests`
  - `UpsertRunPullRequest` and `ListRunPullRequests`
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/store -count=1`

### Technical details
- `git_pr` default block:
```json
{
  "mode": "assist",
  "credential_mode": "local_user_auth",
  "branch_template": "{ticket}/{repo}/{run}",
  "require_all": true,
  "required_checks": ["tests"]
}
```
- `run_pull_requests` primary key: `(run_id, ticket, repo)`.
