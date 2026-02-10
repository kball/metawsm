---
Title: Implementation Plan
Ticket: METAWSM-TICKET-WORKFLOW-20260210
Status: active
Topics:
    - core
    - cli
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: examples/policy.example.json
      Note: Example policy shows required check and staged base prompt guidance
    - Path: internal/orchestrator/git_pr_validation.go
      Note: Defines contract-triggered staged workflow enforcement behavior
    - Path: internal/policy/policy.go
      Note: Default required checks include ticket_workflow
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-10T07:12:01.035312-08:00
WhatFor: ""
WhenToUse: ""
---


# Implementation Plan

## Executive Summary

Implement a new `git_pr` required check (`ticket_workflow`) that enforces staged ticket workflow artifacts before `metawsm commit` and `metawsm pr` proceed, when operator feedback declares the staged contract.

## Problem Statement

Current `git_pr` validation covers tests, forbidden files, and clean-tree state, but does not verify process conformance for ticket workflow stages (analysis, planning feedback, task tracking, diary updates).

## Proposed Solution

- Add `ticket_workflow` to the validation check registry.
- Pass workspace doc-root context into validation inputs from commit/PR service paths.
- Detect staged workflow contract from `reference/99-operator-feedback.md`.
- Enforce required ticket docs/signals:
  - analysis doc with relevant areas + feedback request/response (approved),
  - implementation plan doc with feedback request/response (approved),
  - fully checked `tasks.md`,
  - structured diary step entries,
  - step-based changelog updates.
- Register `ticket_workflow` in policy required-check allowlist and default required checks.
- Update tests and reference docs.

## Design Decisions

- Enforcement is implemented as a `git_pr` required check rather than a separate command.
  Rationale: preserves existing policy gate semantics and operator workflows.
- Enforcement activates only when operator feedback explicitly declares the staged contract.
  Rationale: avoids breaking older tickets while enforcing the new standard where requested.
- Doc validation is content- and section-based, not hardcoded to a single filename.
  Rationale: allows docmgr numbering/title flexibility across tickets.

## Alternatives Considered

- Enforce via a new standalone preflight command.
  Rejected: duplicates required-check logic and would be easier to bypass.
- Always enforce for every ticket unconditionally.
  Rejected for V1 migration safety; can be tightened later after fleet-wide template rollout.

## Implementation Plan

## Plan

1. Add `ticket_workflow` check implementation in `internal/orchestrator/git_pr_validation.go`.
2. Wire `DocRootPath` into commit/PR validation inputs from `internal/orchestrator/service.go`.
3. Update policy support/defaults in `internal/policy/policy.go`.
4. Add/adjust tests:
   - new ticket workflow validation unit tests,
   - policy defaults check update.
5. Update operator-facing docs (`README.md`, `examples/policy.example.json`).
6. Validate with `go test ./...`.

## Open Questions

- Should V2 make `ticket_workflow` unconditional instead of operator-feedback-triggered?

## Feedback Request

Please confirm the trigger rule: enforce only when `reference/99-operator-feedback.md` includes a required staged workflow contract.

## Feedback Response

Approved for V1; this maintains backward compatibility while making the required workflow enforceable for active tickets that declare it.

## References

- `../reference/100-codebase-relevance-analysis.md`
- `../reference/99-operator-feedback.md`
