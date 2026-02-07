---
Title: First bootstrap flow
Ticket: METAWSM-002
Status: active
Topics:
    - core
    - cli
DocType: index
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: CLI command surface to add bootstrap and guide commands
    - Path: internal/model/types.go
      Note: Run and step state models that need guidance-loop state extensions
    - Path: internal/orchestrator/service.go
      Note: Run planner/executor and HSM integration points for intake and guidance loop
    - Path: internal/store/sqlite.go
      Note: Durable run data layer to persist intake brief and Q&A transcript
ExternalSources: []
Summary: Ticket to implement the minimum bootstrap operator flow from ticket intake to merge-ready close
LastUpdated: 2026-02-07T07:26:37.343558-08:00
WhatFor: ""
WhenToUse: ""
---



# First bootstrap flow

## Overview

Define and implement the first end-to-end bootstrap operator flow:
- operator provides a ticket,
- metawsm asks clarifying questions until scope is actionable,
- metawsm creates workspace and starts implementation,
- metawsm asks for user guidance when blocked,
- flow continues until merge-ready close is complete.

This ticket intentionally targets minimum viable behavior using existing run/store/HSM foundations from `METAWSM-001`.

## Key Links

- **Related Files**: See frontmatter RelatedFiles field
- **External Sources**: See frontmatter ExternalSources field
- Design doc: `design-doc/01-minimum-bootstrap-flow.md`
- Prior implementation diary: `../METAWSM-001--bootstrap-metawsm-orchestration/reference/01-diary.md`

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
