---
Title: 'Plan: ingest PR review comments into ticket feedback loop'
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
      Note: Add review sync CLI and operator intent routing
    - Path: cmd/metawsm/operator_llm.go
      Note: Add review_feedback_ready intent allowlist and merge behavior
    - Path: internal/model/types.go
      Note: Add normalized review feedback model and statuses
    - Path: internal/orchestrator/service.go
      Note: Add review sync
    - Path: internal/policy/policy.go
      Note: Add and validate review-feedback policy controls under git_pr
    - Path: internal/store/sqlite.go
      Note: Persist and query PR review feedback items with status transitions
ExternalSources: []
Summary: Plan for syncing GitHub PR review feedback into ticket iteration so agents can address reviewer comments with minimal operator copy/paste.
LastUpdated: 2026-02-08T14:38:02-08:00
WhatFor: Add a first-class workflow for detecting GitHub PR review feedback and routing it into ticket iteration for agent follow-up.
WhenToUse: Use when implementing or operating automated review-feedback ingestion for METAWSM commit/PR runs.
---


# Plan: ingest PR review comments into ticket feedback loop

## Executive Summary

Extend the existing `METAWSM-009` commit/PR automation so `metawsm` can:
1. detect new review feedback on run-created pull requests,
2. persist and deduplicate those feedback items,
3. route actionable feedback into ticket docs via the existing iteration mechanism,
4. trigger an agent restart to address the feedback.

V1 should ship in assist-first mode with a manual sync command and optional operator auto-execution once behavior is validated.

## Problem Statement

`metawsm` already creates PRs and persists PR metadata, but review feedback handling is manual:
1. operators must read review comments in GitHub and copy/paste feedback into `metawsm iterate`,
2. there is no durable linkage between incoming PR review comments and ticket iteration history,
3. operator loop logic cannot determine when external review feedback is waiting.

This creates latency, inconsistency, and poor traceability for post-PR iterations.

## Proposed Solution

Add a review-feedback ingestion subsystem integrated with existing run PR state and iteration flow.

### End-to-end flow

1. Fetch review artifacts for each persisted open PR row in `run_pull_requests`.
2. Normalize and persist new comment/review items with a stable source key (GitHub item ID).
3. Build a ticket-scoped feedback summary from newly queued items.
4. Feed summary into existing `Iterate` flow so feedback lands in:
- `<workspace>/.metawsm/operator-feedback.md`
- `<workspace>/<doc_home_repo>/ttmp/.../<ticket>/reference/99-operator-feedback.md`
5. Restart run/agents using existing restart behavior from `Iterate`.
6. Mark ingested review items as queued/addressed to prevent replays.

### New command surface

Add:
`metawsm review sync [--run-id RUN_ID | --ticket TICKET] [--dry-run]`

Behavior:
1. Resolves run context and open PRs.
2. Calls GitHub via `gh api` for PR review comments (V1).
3. Prints sync summary in dry-run mode.
4. Persists and optionally dispatches feedback in execute mode.

### Operator loop integration

Add a new operator intent:
`review_feedback_ready`

Decision rule:
1. if run is complete,
2. and queued/unaddressed review feedback exists,
3. then signal `review_feedback_ready`.

Mode behavior:
1. `assist`: alert only.
2. `auto`: execute review sync + iterate dispatch automatically.
3. Auto mode applies a per-interval dispatch cap to prevent restart churn.

## Design Decisions

1. Reuse `gh` CLI APIs instead of introducing webhooks in V1.
Reason: aligns with existing local-user-auth model and avoids external infra dependencies.

2. Persist each feedback artifact with a stable source identifier.
Reason: enables idempotent sync, auditability, and explicit status transitions.

3. Reuse `Iterate` instead of inventing a parallel feedback application path.
Reason: keeps one canonical mechanism for writing operator feedback and restarting work.

4. Default feedback automation to assist mode.
Reason: reduces surprise/risk while validating quality of extracted review context.

5. Keep feedback filtering policy-driven.
Reason: allows teams to tune which comments are actionable (bot authors, resolved threads, etc).

6. V1 ingests only PR review comments.
Reason: narrower scope reduces ambiguity and avoids mixing general conversation with actionable code review items.

7. Mark feedback `addressed` only after a successful commit+PR update cycle.
Reason: dispatching feedback to an agent is not proof the requested change has been delivered for reviewer verification.

## Alternatives Considered

1. GitHub webhook receiver for real-time events.
Rejected for V1: requires hosted endpoint, secret management, and deployment complexity.

2. Parse review context from PR timeline text only.
Rejected: weak structure and higher parsing brittleness than API primitives.

3. Keep process fully manual (`status` hint only).
Rejected: does not reduce operator toil and prevents deterministic operator intents.

## Implementation Plan

### Phase 1: Data model and store

1. Add model type for review feedback item, including:
- run/ticket/repo/pr identifiers,
- source type/id/url,
- author/body/path/line metadata,
- ingestion status (`new|queued|addressed|ignored`),
- timestamps.
2. Add SQLite table and store methods:
- upsert/list by run,
- list queued by ticket,
- update status transitions.
3. Add store persistence tests (including dedupe behavior across reopen).

### Phase 2: GitHub sync service primitive

1. Add orchestrator service method to sync review feedback for run PRs.
2. Use `gh api` endpoints for:
- PR review comments only (V1 scope).
Issue-comment ingestion can be added in a later phase.
3. Normalize payloads and upsert only unseen/changed items.
4. Capture sync events in run events table for observability.

### Phase 3: Feedback dispatch into iteration

1. Add service method to compile queued feedback into deterministic markdown.
2. Route compiled feedback through `Iterate` path (single feedback block per ticket/repo batch).
3. Mark dispatched items as `queued` (or equivalent in-progress state) after iterate/restart succeeds.
4. Mark items `addressed` only after successful follow-up `commit` and `pr` update cycle.
5. Keep dry-run output explicit about what would be written and restarted.

### Phase 4: CLI and status surfaces

1. Add `metawsm review sync` command with `--dry-run`.
2. Extend `status` output with a `Review Feedback:` section including counts per status.
3. Add guidance hints for next steps when feedback is queued.

### Phase 5: Operator rule and intent

1. Add `review_feedback_ready` operator intent to rule engine and LLM allowlist.
2. In assist mode: emit alert and hints only.
3. In auto mode: execute sync + iterate dispatch when policy allows.

### Phase 6: Policy contract

1. Add policy block, for example:
`git_pr.review_feedback.enabled`, `mode`, `include_review_comments`, `ignore_authors`, `max_items_per_sync`, `auto_dispatch_cap_per_interval`.
2. Validate defaults and invalid combinations in policy tests.
3. Defaults:
- `include_review_comments=true`
- `ignore_authors=[]` (empty, explicitly configurable)
- `auto_dispatch_cap_per_interval=1`

### Phase 7: Validation and rollout

1. Add fake-`gh` integration tests for sync parsing and dedupe.
2. Add orchestrator tests for dispatch behavior and operator intent triggering.
3. Roll out:
- Step A: command + status only,
- Step B: assist intent in operator,
- Step C: selective auto enablement.

## Open Questions

1. Resolved: V1 includes PR review comments only.
2. Resolved: mark feedback `addressed` after successful commit and PR update cycle.
3. Resolved: default author filters remain empty; filtering is policy-configurable.
4. Resolved: auto mode includes a dispatch cap per operator interval.

## References

- `ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/design-doc/01-plan-teach-agents-to-commit-and-open-github-prs.md`
- `ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/design-doc/02-phase-2-plan-remove-commit-pr-workflow-workarounds.md`
- `ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/playbook/01-operator-and-agent-commit-pr-workflow.md`
- `internal/orchestrator/service.go`
- `internal/store/sqlite.go`
- `cmd/metawsm/main.go`
