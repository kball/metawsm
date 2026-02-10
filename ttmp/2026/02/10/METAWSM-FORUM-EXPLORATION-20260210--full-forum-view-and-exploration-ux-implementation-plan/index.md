---
Title: Full forum view and exploration UX + implementation plan
Ticket: METAWSM-FORUM-EXPLORATION-20260210
Status: active
Topics:
    - core
    - backend
    - gui
    - chat
    - websocket
DocType: index
Intent: long-term
Owners: []
RelatedFiles:
    - Path: internal/orchestrator/service_forum.go
      Note: Primary service orchestration for forum commands
    - Path: internal/server/api.go
      Note: HTTP API contract surface for forum search
    - Path: internal/server/websocket.go
      Note: Forum stream update transport used by UI refresh behavior.
    - Path: internal/store/sqlite_forum.go
      Note: SQLite persistence and projections for threads
    - Path: ui/src/App.tsx
      Note: Current UI shell to evolve into explorer
    - Path: ui/src/styles.css
      Note: Styling surface for richer thread explorer and queue-focused interaction design.
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-10T08:09:01.930968-08:00
WhatFor: ""
WhenToUse: ""
---


# Full forum view and exploration UX + implementation plan

## Overview

<!-- Provide a brief overview of the ticket, its goals, and current status -->

## Key Links

- **Related Files**: See frontmatter RelatedFiles field
- **External Sources**: See frontmatter ExternalSources field

## Status

Current status: **active**

## Topics

- core
- backend
- gui
- chat
- websocket

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
