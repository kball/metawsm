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

