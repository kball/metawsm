---
Title: Codebase Relevance Analysis
Ticket: METAWSM-TICKET-WORKFLOW-20260210
Status: active
Topics:
    - core
    - cli
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: internal/orchestrator/git_pr_validation.go
      Note: Added staged workflow check implementation
    - Path: internal/orchestrator/service.go
      Note: Propagated doc root into commit/PR validation input
    - Path: internal/policy/policy.go
      Note: Registered ticket_workflow check in default policy
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-10T07:12:01.027692-08:00
WhatFor: ""
WhenToUse: ""
---


# Codebase Relevance Analysis

## Goal

Identify the minimum code surfaces needed to enforce a staged ticket workflow at commit/PR time.

## Context

Operator feedback for this ticket requires a four-step flow: codebase relevance analysis + feedback, implementation plan + feedback, task breakdown + implementation diary, then PR/review iteration.

## Relevant Areas

- `internal/orchestrator/git_pr_validation.go`
  Why: Existing required-check framework for `metawsm commit` and `metawsm pr`; best enforcement insertion point.
- `internal/orchestrator/service.go`
  Why: Builds validation inputs in commit/PR paths; must pass doc-root context so workflow checks can inspect ticket docs.
- `internal/policy/policy.go`
  Why: Default policy and required-check allowlist; needed to register the new check.
- `internal/policy/policy_test.go`
  Why: Guards default policy behavior and required-check validation.
- `examples/policy.example.json`
  Why: Operator-facing reference policy should include the staged workflow guard and updated base prompt.
- `README.md`
  Why: Public command/policy reference should list the new supported required check.

## Unclear Areas

- None at this stage. The existing validation framework already supports pluggable checks and fail-fast semantics.

## Feedback Request

Proposed enforcement model:
1. Add `ticket_workflow` as a git/PR required check.
2. Trigger enforcement only when ticket operator feedback explicitly declares the required staged workflow contract.
3. Validate existence/completeness of analysis, implementation plan, tasks, diary, and changelog signals before commit/PR.

Please confirm this should be the contract scope for V1.

## Feedback Response

Approved. The required workflow is explicitly documented in `reference/99-operator-feedback.md`; implementing enforcement through required checks matches current architecture and minimizes scope.

## Quick Reference

Validation signal checklist for V1:
- Operator feedback includes `Required workflow` with steps 1-4.
- Analysis doc has `Relevant Areas`, `Feedback Request`, `Feedback Response`.
- Plan doc has `Plan`, `Feedback Request`, `Feedback Response`.
- Tasks are fully checked in `tasks.md`.
- Diary contains step entries with `Prompt Context`.
- Changelog contains step-oriented entries.

## Related

- `../design-doc/01-implementation-plan.md`
- `./99-operator-feedback.md`
- `./101-diary.md`
