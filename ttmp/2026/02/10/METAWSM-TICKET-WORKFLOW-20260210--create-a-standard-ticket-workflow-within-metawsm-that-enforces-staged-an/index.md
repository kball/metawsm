---
Title: Create a standard ticket workflow within metawsm that enforces staged an
Ticket: METAWSM-TICKET-WORKFLOW-20260210
Status: active
Topics:
    - core
    - cli
DocType: index
Intent: long-term
Owners: []
RelatedFiles:
    - Path: examples/policy.example.json
      Note: Documents staged workflow policy usage
    - Path: internal/orchestrator/git_pr_validation.go
      Note: Implements new ticket_workflow required check
    - Path: internal/orchestrator/git_pr_validation_test.go
      Note: Validates staged workflow check behavior
    - Path: internal/orchestrator/service.go
      Note: Passes doc-root context into commit/pr validation input
    - Path: internal/policy/policy.go
      Note: Adds ticket_workflow as default required check
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-10T07:04:56.872422-08:00
WhatFor: ""
WhenToUse: ""
---


# Create a standard ticket workflow within metawsm that enforces staged an

## Overview

<!-- Provide a brief overview of the ticket, its goals, and current status -->

## Key Links

- **Related Files**: See frontmatter RelatedFiles field
- **External Sources**: See frontmatter ExternalSources field

## Status

Current status: **active**

## Topics

- core
- cli

## Tasks

See [tasks.md](./tasks.md) for the current task list.

## Changelog

See [changelog.md](./changelog.md) for recent changes and decisions.

## Structure

- design/ - Architecture and design documents
- reference/ - Prompt packs, API contracts, context summaries
- playbooks/ - Command sequences and test procedures
- scripts/ - Temporary code and tooling
- various/ - Working notes and research
- archive/ - Deprecated or reference-only artifacts
