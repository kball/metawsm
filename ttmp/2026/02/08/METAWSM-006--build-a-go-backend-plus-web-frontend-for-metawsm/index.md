---
Title: Build a Go backend plus web frontend for metawsm
Ticket: METAWSM-006
Status: active
Topics:
    - core
    - cli
DocType: index
Intent: long-term
Owners: []
RelatedFiles:
    - Path: .metawsm/implementation-complete.json
      Note: Run completion sentinel
    - Path: .metawsm/validation-result.json
      Note: Run validation sentinel
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-08T07:32:22.60017-08:00
WhatFor: ""
WhenToUse: ""
---


# Build a Go backend plus web frontend for metawsm

## Overview

Implemented a first web dashboard stack for `metawsm`:
- a new `metawsm web` command that serves API + SPA,
- backend API endpoints for run summaries/details,
- frontend React/Vite dashboard UI,
- Go embed contract with disk fallback and `go generate` copy flow.

Go tests and Go builds pass in this environment. Frontend dependency install/build is network-blocked in this sandbox, and this is documented in the diary/changelog with exact command output.

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

Current task state: implementation and validation tasks are marked complete.

## Changelog

See [changelog.md](./changelog.md) for recent changes and decisions.

## Structure

- design/ - Architecture and design documents
- reference/ - Prompt packs, API contracts, context summaries
- playbooks/ - Command sequences and test procedures
- scripts/ - Temporary code and tooling
- various/ - Working notes and research
- archive/ - Deprecated or reference-only artifacts
