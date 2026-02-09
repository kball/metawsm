# Changelog

## 2026-02-08

- Initial workspace created


## 2026-02-08

Created METAWSM-009 and added initial design plan plus implementation task backlog for agent commit and GitHub PR automation.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/design-doc/01-plan-teach-agents-to-commit-and-open-github-prs.md — Initial phased plan and decisions
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/tasks.md — Seeded implementation task backlog


## 2026-02-08

Captured resolved requirements from operator feedback (per-repo/ticket PRs, all checks required, metawsm push+PR, human-only merge) and added credential strategy proposals with phased recommendation.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/design-doc/01-plan-teach-agents-to-commit-and-open-github-prs.md — Updated design decisions and resolved questions
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/01-credential-strategies-for-commit-and-pr-automation.md — Credential mode proposals and rollout guidance


## 2026-02-08

Confirmed Proposal A as V1 credential strategy and translated decisions into concrete implementation tasks for auth preflight, validation framework, per-repo/ticket PR fanout, and human-only merge enforcement.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/design-doc/01-plan-teach-agents-to-commit-and-open-github-prs.md — Resolved credential decision and updated open questions
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/01-credential-strategies-for-commit-and-pr-automation.md — Marked Proposal A as selected for V1
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/tasks.md — Added Proposal A task breakdown


## 2026-02-08

Step 1: Added git_pr policy contract and persisted run PR metadata (commit d3f13f6).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/policy/policy.go — New git_pr schema/defaults/validation
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite.go — Added run_pull_requests schema and CRUD methods
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded step details and validation outcomes


## 2026-02-08

Step 2: Added Proposal A auth preflight command (metawsm auth check) with run-scoped git identity checks (commit 6148470).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — New auth command and helper functions
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main_test.go — Auth command and repo-path tests
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded Step 2 implementation and validation

## 2026-02-08

Step 3: Exposed persisted run pull request metadata in metawsm status output and added status rendering tests (commit 283a68b).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Added PR wrappers and status section
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Added status PR section coverage
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded Step 3 details and compile fix

## 2026-02-08

Step 4: Implemented branch-prep and commit creation service primitives with dry-run previews and persisted commit metadata (commit 678b936)

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Added Commit service primitive and git branch/commit orchestration
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Added commit primitive coverage for dry-run
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/policy/policy.go — Added RenderGitBranch helper for policy-driven branch naming
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/policy/policy_test.go — Added branch template renderer unit tests


## 2026-02-08

Step 5: Added metawsm commit command wired to service commit primitives with dry-run previews (commit 9de30b7)

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Added commit command routing
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main_test.go — Added commit command selector validation test


## 2026-02-08

Step 6: Added gh-based PR service primitive, metawsm pr command, and credential/actor event recording for commit+pr flows (commit 180a976)

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Added metawsm pr command and CLI output for dry-run and created PR metadata
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main_test.go — Added pr command selector validation test
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Added OpenPullRequests service primitive plus PR defaults
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Added dry-run and fake-gh PR creation tests


## 2026-02-08

Step 7: integrated commit/pr readiness signals into operator loop with assist/auto execution behavior (commit b3587e3).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Added readiness parsing from status plus commit_ready/pr_ready events/actions
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/operator_llm.go — Added readiness intents and Execute semantics for rule decisions
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded Step 7 implementation details and validation


## 2026-02-08

Step 8: added commit/PR preflight rejection tests for invalid run state, git_pr.mode=off, and missing prepared metadata paths (commit 299a096).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Added four rejection-path tests for commit/pr preflight behavior
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded Step 8 implementation details and test validation


## 2026-02-08

Step 9: added operator/agent commit-PR playbook with Proposal A setup, assist/auto command flow, and troubleshooting guidance; checked tasks 10 and 19.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/playbook/01-operator-and-agent-commit-pr-workflow.md — New playbook covering setup
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded Step 9 documentation work
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/tasks.md — Marked tasks 10 and 19 complete


## 2026-02-08

Step 10: implemented git_pr validation framework and enforced required checks for commit/pr workflows (tasks 14-16, commit d31a862).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/git_pr_validation.go — New extensible validation checks and require_all semantics
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Commit/PR gate enforcement and validation report persistence
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Regression tests for check failures and require_all behavior
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/policy/policy.go — git_pr schema and validation updates for test commands and forbidden patterns


## 2026-02-08

Step 11: enforced human-only merge execution by requiring --human for non-dry-run merge command paths (task 18, commit 4dca4ec).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Added merge acknowledgement gate and updated operator hints
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main_test.go — Added merge acknowledgement and hint behavior tests


## 2026-02-08

Step 12: added multi-ticket workspace fanout for commit/pr workflows so run-level execution can process per-ticket metadata without requiring explicit ticket selection (task 17, commit 6ec9185).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Multi-ticket fanout logic in Commit/OpenPullRequests
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Fanout regression tests for multi-ticket runs


## 2026-02-08

Step 13: added push-before-PR execution plus end-to-end local-auth commit->push->PR coverage with local origin and fake gh (task 20, commit 627e397).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — PR workflow now pushes branch before gh pr create
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — End-to-end commit/push/PR test coverage


## 2026-02-08

Reopened METAWSM-009 with a Phase 2 hardening plan to remove manual commit/pr workarounds (dirty-tree base drift handling, SQLite lock contention, actor attribution fallback, and stronger preflight diagnostics).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/design-doc/02-phase-2-plan-remove-commit-pr-workflow-workarounds.md — New phase 2 design plan
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/tasks.md — Added phase 2 execution backlog items


## 2026-02-08

Expanded reopened Phase 2 backlog into implementation-ready sub-tasks (branch prep, sqlite lock handling, actor fallback, diagnostics, playbook, and e2e coverage).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/tasks.md — Replaced coarse Phase 2 items with atomic executable tasks


## 2026-02-08

Phase 2B storage hardening: added sqlite busy-timeout + retry/backoff behavior and a contention test that verifies lock recovery without manual reruns.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite.go — Added timeout/retry logic for sqlite query and exec paths
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite_test.go — Added write-lock contention test verifying retries and eventual success
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded Step 15 implementation notes and validation


## 2026-02-08

Phase 2B mutation locking: added run-level lock files for non-dry-run commit/pr operations and typed in-progress errors with regression tests for lock rejection.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Added run mutation lock acquisition and RunMutationInProgressError
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Added commit/pr lock rejection coverage
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded Step 16 mutation lock implementation


## 2026-02-08

Phase 2A/2C hardening: commit now handles stale-base dirty trees via snapshot-reset-reapply, actor fallback resolves from gh/git when --actor is omitted, and commit/pr outputs include preflight diagnostics and lock remediation hints.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Added lock-aware remediation and commit/pr preflight+actor output
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Added native snapshot branch prep
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Added stale-base success/conflict and actor fallback regression coverage
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded Step 17 implementation and validation details


## 2026-02-08

Phase 2D complete: added stale-base end-to-end commit->push->PR coverage and updated operator playbook with native snapshot handling, mutation lock semantics, and conflict recovery guidance.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Added stale-base end-to-end commit/push/PR regression test
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/playbook/01-operator-and-agent-commit-pr-workflow.md — Documented native stale-base and lock handling behavior
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded Step 18 completion summary


## 2026-02-08

Step 19: added review feedback model/store persistence primitives with reopen+dedupe tests; completed tasks 37-39 (commit 967c146).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/model/types.go — Review feedback lifecycle and record model
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite.go — run_review_feedback table and store methods
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/store/sqlite_test.go — Persistence and dedupe tests for new review feedback store


## 2026-02-08

Step 20: added git_pr.review_feedback policy defaults and validation; completed task 40 (commit e6cc5e8).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/examples/policy.example.json — Policy example updated with review_feedback block
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/policy/policy.go — Review feedback policy contract and guards
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/policy/policy_test.go — Validation/default coverage for review feedback policy


## 2026-02-08

Step 21: added orchestrator SyncReviewFeedback primitive for PR review comments with fake-gh tests; completed task 41 (commit 5178ee6).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Review-comment sync orchestration and persistence integration
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Sync primitive behavior coverage


## 2026-02-08

Step 22: dispatched queued review feedback through iterate flow and added queued->new->addressed lifecycle transitions; completed task 42 (commit 82dc694).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Dispatch helper and feedback status transition hooks
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Dispatch and lifecycle transition coverage


## 2026-02-08

Step 23: added review sync CLI, status/watch review feedback counters, and operator review_feedback_ready intent with capped auto execution; completed tasks 43-45 (commit 78a61bf).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Review sync command
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main_test.go — Command validation and operator decision coverage
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/operator_llm.go — New review_feedback_ready intent allowlist
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Status output includes review feedback counters
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded Step 23 implementation details


## 2026-02-08

Step 24: extended review sync to ingest top-level PR reviews with ignore-author filtering and actionable-state/body filtering, plus dedupe/dispatch coverage (commit f70193d).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/model/types.go — Added PR top-level review source type
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Dual-endpoint review sync and top-level normalization/filtering
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Top-level review ingestion/dedupe/dispatch regression coverage
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded Step 24 implementation details


## 2026-02-08

Step 25: documented V1.1 top-level review ingestion scope and completed METAWSM-006 live sync+dispatch follow-up (tasks 53-54).

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/design-doc/03-plan-ingest-pr-review-comments-into-ticket-feedback-loop.md — Updated V1.1 scope and endpoint details
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/playbook/01-operator-and-agent-commit-pr-workflow.md — Updated operator workflow notes for top-level review ingestion
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/02-diary.md — Recorded Step 25 docs and live follow-up execution
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/tasks.md — Checked tasks 53 and 54 complete

