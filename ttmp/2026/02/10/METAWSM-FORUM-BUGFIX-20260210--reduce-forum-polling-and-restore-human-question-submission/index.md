---
Title: Reduce forum polling and restore human question submission
Ticket: METAWSM-FORUM-BUGFIX-20260210
Status: active
Topics:
    - backend
    - chat
    - gui
    - websocket
    - core
DocType: index
Intent: long-term
Owners: []
RelatedFiles: []
ExternalSources: []
Summary: Ticket for forum bugfix design covering event-driven stream updates and human question submission from UI.
LastUpdated: 2026-02-10T12:00:00-08:00
WhatFor: Track and implement fixes for polling load and missing human ask workflow in forum explorer.
WhenToUse: Use for implementation planning, review, and validation of METAWSM forum bugfix scope.
---

# Reduce forum polling and restore human question submission

## Overview

This ticket addresses two reported forum issues:
- Websocket stream still causes polling-heavy behavior and unnecessary refresh traffic.
- Humans cannot submit new questions in the forum explorer UI.

Primary design is documented in:
- `design-doc/01-fix-plan-event-driven-forum-stream-and-human-ask-flow.md`

## Key Links

- **Related Files**: See frontmatter RelatedFiles field
- **External Sources**: See frontmatter ExternalSources field

## Status

Current status: **active**

## Topics

- backend
- chat
- gui
- websocket
- core

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
