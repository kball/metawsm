---
Title: Bootstrap metawsm orchestration
Ticket: METAWSM-001
Status: active
Topics:
    - core
    - cli
DocType: index
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: Primary CLI surface including TUI
    - Path: internal/orchestrator/service.go
      Note: Core orchestration and lifecycle transitions
    - Path: internal/store/sqlite.go
      Note: Durable SQLite run/step/agent/event store
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-07T06:39:51.30244-08:00
WhatFor: ""
WhenToUse: ""
---


# Bootstrap metawsm orchestration

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
