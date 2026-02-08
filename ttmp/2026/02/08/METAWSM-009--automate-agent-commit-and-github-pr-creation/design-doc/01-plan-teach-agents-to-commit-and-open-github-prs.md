---
Title: 'Plan: teach agents to commit and open GitHub PRs'
Ticket: METAWSM-009
Status: active
Topics:
    - cli
    - core
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: Add CLI surfaces for commit/pr workflows and operator actions
    - Path: internal/orchestrator/service.go
      Note: Add orchestration primitives for commit and PR lifecycle actions
    - Path: internal/policy/policy.go
      Note: Add policy block controlling auto commit/PR behavior and guardrails
    - Path: internal/store/sqlite.go
      Note: Persist PR metadata and review lifecycle state for each run
    - Path: README.md
      Note: Document operator and agent workflows for Git commits and PRs
ExternalSources: []
Summary: 'Proposed phased plan for teaching metawsm agents to create commits and open GitHub PRs with guardrails and human merge control.'
LastUpdated: 2026-02-08T11:01:55-08:00
WhatFor: 'Define how to safely automate commit and PR creation from agent workspaces while preserving operator control and human merge authority.'
WhenToUse: 'Use when implementing or reviewing METAWSM-009 automation for commit and PR workflows.'
---

# Plan: teach agents to commit and open GitHub PRs

## Executive Summary

Add first-class `metawsm` support for post-implementation Git workflows so agents can:
1. prepare clean branch-based commits,
2. push those commits,
3. open well-structured GitHub PRs,
4. hand review and merge decisions back to a human.

Rollout should be staged: start operator-driven and dry-run heavy, then add bounded automation once policy and observability are in place.

## Problem Statement

Current runs can finish implementation, but branch/commit/PR steps are still manual. This creates friction:
1. operators must inspect each workspace and run repeated Git/GitHub commands;
2. metadata (commit messages, PR titles, linked tickets, test evidence) is inconsistent;
3. there is no persisted PR lifecycle state per run in `metawsm`;
4. agent autonomy ends before one of the highest-value outputs: a reviewable PR.

## Proposed Solution

Implement a commit+PR workflow layer with deterministic guards and explicit operator control.

### Scope

1. Branch management
- Standard branch naming: `<ticket>/<repo-slug>/<run-id-or-short-token>`.
- One PR per repo/ticket for multi-repo runs.
- Ensure workspace is on correct base branch before creating/switching branch.

2. Commit preparation
- Agent generates commit proposal from run diff + ticket brief.
- Deterministic checks before commit:
  - no merge conflicts,
  - no forbidden files (secrets, large artifacts, binaries),
  - required validation checks pass.
- Commit message template includes ticket ID and concise summary.

3. Validation policy
- V1 requirement: all configured tests/checks must pass before PR creation.
- Validation should be plugin-friendly/extensible so additional checks can be added without redesign.

4. Push + PR creation
- `metawsm` performs `git push` for prepared branches.
- Create PR via `gh pr create` with:
  - title template: `[TICKET] <summary>`,
  - body template from run brief + validation evidence + risks,
  - base branch from run spec/policy,
  - optional labels/reviewers from policy.

5. PR state persistence
- Store PR URL/number/head/base/status in SQLite linked to `run_id` and ticket+repo.
- Surface status in `metawsm status` and next-step hints.

6. Command surface
- Add explicit commands:
  - `metawsm commit --ticket|--run-id [--dry-run]`
  - `metawsm pr --ticket|--run-id [--dry-run]`
- Keep `merge` behavior human-only. Metawsm may prepare and open PRs but never auto-merge.

7. Operator integration
- `operator` may suggest `commit_ready` / `pr_ready` events.
- In assist mode, operator never executes commit/pr automatically.
- In auto mode, execution only if policy allows and deterministic gates pass.

## Design Decisions

1. One PR per repo/ticket
- Rationale: scales cleanly for multi-repo runs and preserves per-repo review ownership.

2. `metawsm` pushes commits and opens PRs
- Rationale: removes repetitive operator mechanics while still allowing policy gating.

3. Human-only merge policy
- Rationale: keeps final integration decision with human reviewers.

4. All tests/checks must pass before PR in V1
- Rationale: enforce baseline review quality and reduce known-bad PRs.

5. Validation architecture must be extensible
- Rationale: allows future checks (security, lint, license, policy) without replacing workflow.

6. Separate `commit` and `pr` commands from `merge`/`close`
- Rationale: clearer lifecycle and less risk of surprising side effects.

7. PR metadata persisted in store
- Rationale: run state remains authoritative across restarts.

## Alternatives Considered

1. Keep PR workflow fully manual
- Rejected: leaves operator toil unresolved.

2. Agent shells out to GitHub directly with no metawsm mediation
- Rejected: weak guardrails and no centralized run-state visibility.

3. Collapse commit/pr into `close`
- Rejected: too implicit; close should remain finalization, not PR drafting.

## Implementation Plan

### Phase 1: Data model and policy

1. Extend policy schema with `git_pr` block:
- mode (`off|assist|auto`),
- branch template,
- validation policy (`require_all=true`, check list),
- label/reviewer defaults,
- allowed repos.
2. Extend store schema with `run_pull_requests` table keyed by run/ticket/repo.
3. Add tests for policy validation and state persistence.

### Phase 2: Validation + service primitives

1. Add validation runner interface (`ValidationCheck`) and registry.
2. Implement initial required checks:
- configured test commands,
- dirty tree policy,
- forbidden file scan.
3. Add service operations for:
- branch preparation,
- commit creation (or dry-run preview),
- push,
- PR create/update.
4. Add structured event logging for each action and rejection reason.

### Phase 3: CLI commands

1. Add `metawsm commit` and `metawsm pr` commands.
2. Add dry-run output showing exact Git/GH commands and generated message/body.
3. Surface PR metadata and follow-up hints in `metawsm status`.

### Phase 4: Operator integration

1. Extend operator decision engine with commit/pr readiness signals.
2. In assist mode, print recommendations only.
3. In auto mode, allow execution only when policy + all required checks pass.
4. Keep merge intent/action human-only in all modes.

### Phase 5: Docs and rollout

1. Update README and playbook for operator + agent workflows.
2. Add end-to-end test scenario from completed run to pushed branch + open PR.
3. Roll out in assist mode first, then evaluate auto mode for selected repos.

## Resolved Questions

1. PR granularity: one PR per repo/ticket.
2. Validation gate: all tests/checks must pass before review.
3. Push ownership: `metawsm` should push commits and open PRs.
4. Merge ownership: merge remains human-only.
5. Credential strategy for V1: Proposal A (reuse local `gh` + git auth).

## Open Questions

1. Default policy for fork vs same-repo branches when write access is limited.
2. Retry semantics when push/PR succeeds partially across multi-repo runs.

## References

- `ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/reference/01-credential-strategies-for-commit-and-pr-automation.md`
- `ttmp/2026/02/08/METAWSM-007--create-orchestrator-operator-agent-for-ongoing-run-management/design-doc/01-implementation-plan-for-orchestrator-operator-agent.md`
- `ttmp/2026/02/08/METAWSM-006--build-a-go-backend-plus-web-frontend-for-metawsm/reference/01-bootstrap-brief-run-20260208-073222.md`
- `README.md`
