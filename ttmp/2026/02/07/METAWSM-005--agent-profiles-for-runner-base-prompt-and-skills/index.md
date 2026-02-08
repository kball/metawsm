---
Title: Agent profiles for runner base-prompt and skills
Ticket: METAWSM-005
Status: active
Topics:
    - core
    - cli
DocType: index
Intent: long-term
Owners: []
RelatedFiles:
    - Path: cmd/metawsm/main.go
      Note: Kickoff doc-repo selection and CLI wiring
    - Path: internal/orchestrator/service.go
      Note: Doc-repo-aware tmux/sync/feedback behavior
    - Path: internal/orchestrator/service_test.go
      Note: Regression coverage for repo-local ttmp routing
    - Path: internal/policy/policy.go
      Note: |-
        Primary code seam for adding profiles
        Implemented profile schema and command compilation
    - Path: internal/policy/policy_test.go
      Note: Validation and command-assembly regression tests
    - Path: ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/design-doc/01-agent-profile-model-and-runner-integration.md
      Note: Design proposal for agent profile architecture
    - Path: ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/reference/01-codex-default-profile-and-base-prompt.md
      Note: Concrete codex default profile and prompt contract
    - Path: ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/reference/02-diary.md
      Note: Implementation diary and troubleshooting record
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-07T10:15:27.315254-08:00
WhatFor: ""
WhenToUse: ""
---




# Agent profiles for runner base-prompt and skills

## Overview

Define a first-class agent profile system for `metawsm` so agent behavior is configured as structured policy, not embedded shell strings.

This ticket proposes a profile model that binds:
- runner/harness (starting with `codex`),
- base prompt,
- skill set (`docmgr`, `diary` for the default profile).

The immediate goal is to preserve current Codex behavior while making workflow requirements explicit and reusable.

## Key Links

- **Related Files**: See frontmatter RelatedFiles field
- **External Sources**: See frontmatter ExternalSources field
- **Design**: `design-doc/01-agent-profile-model-and-runner-integration.md`
- **Reference**: `reference/01-codex-default-profile-and-base-prompt.md`

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
