---
Title: Agent profile model and runner integration
Ticket: METAWSM-005
Status: active
Topics:
    - core
    - cli
DocType: design-doc
Intent: long-term
Owners: []
RelatedFiles:
    - Path: .metawsm/policy.json
      Note: Current codex inline command to migrate into codex-default profile
    - Path: internal/model/types.go
      Note: AgentSpec model shape that will carry profile metadata
    - Path: internal/orchestrator/service.go
      Note: tmux start path and command normalization where profile compilation plugs in
    - Path: internal/policy/policy.go
      Note: Current policy schema and validation seam for agent profile support
    - Path: ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/reference/01-codex-default-profile-and-base-prompt.md
      Note: Concrete codex profile snippet and base prompt contract
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-07T10:15:32.014876-08:00
WhatFor: ""
WhenToUse: ""
---


# Agent profile model and runner integration

## Executive Summary

`metawsm` currently launches agents from raw policy commands. This works, but it tightly couples the runtime harness, behavioral prompt, and workflow conventions into a single shell string.

This design introduces first-class `agent_profiles` in policy so each agent can reference a reusable profile composed of:
- a runner/harness (`codex`),
- a base prompt,
- a set of skills.

The first target profile (`codex-default`) preserves current behavior while adding stronger process discipline: use `docmgr` to manage ticket tasks/docs and maintain a diary during implementation.

## Problem Statement

Current behavior in this repo:
- Policy schema has only `agents[].name` and `agents[].command` (`internal/policy/policy.go`).
- Runtime uses that command directly for `tmux_start` (`internal/orchestrator/service.go`).
- The active policy hardcodes a long inline Codex prompt (`.metawsm/policy.json`).

Issues:
- No reusable profile layer across agents/tickets.
- Prompt content is embedded in shell-escaped JSON strings, which is hard to review and evolve.
- Skills are implied by prompt text, not declared as explicit, inspectable configuration.
- No clear contract for where skill content is sourced (local repo copy vs user-home skills).
- In multi-repo workspaces, docmgr context was synced to workspace root `ttmp/` instead of a concrete repo root.

## Proposed Solution

Add a profile model in policy and resolve agent launch commands through profile compilation.

### Policy schema changes

Add:

```json
{
  "version": 2,
  "agent_profiles": [
    {
      "name": "codex-default",
      "runner": "codex",
      "base_prompt": "You are the implementation agent for this ticket...",
      "skills": ["docmgr", "diary"],
      "runner_options": {
        "full_auto": true
      }
    }
  ],
  "agents": [
    {
      "name": "agent",
      "profile": "codex-default"
    }
  ]
}
```

Update `agents[]` shape to:
- `name` (required),
- `profile` (required).

### Runtime assembly flow

1. `policy.Load/Validate` validates profile uniqueness, runner support, and agent-to-profile references.
2. `ResolveAgents` returns agent specs with `Profile` metadata (not only final command).
3. During kickoff, `doc_repo` is selected (default first `--repos` entry, overrideable via `--doc-repo`).
4. During `tmux_start`, orchestrator resolves workspace path and sets agent working directory to `<workspace>/<doc_repo>` (or workspace root fallback for single-repo root layouts), then launches the profile command.
5. Bootstrap context sync copies ticket docs to `<workspace>/<doc_repo>/ttmp/...` so docmgr state lives in the selected repository.
6. Command assembly from profile:
   - build final prompt = base prompt + run/ticket context + selected skills guidance,
   - create runner-specific command (initially codex),
   - apply existing codex normalization (`--skip-git-repo-check`).
7. Launch via existing tmux wrapping path.

### Skill resolution contract

For each listed skill name, resolve in this order:
1. Repo-local: `.metawsm/skills/<skill>/SKILL.md` (recommended for reproducible runs).
2. User-home: `$HOME/.codex/skills/<skill>/SKILL.md` (fallback).
3. If not found: fail launch with actionable error naming missing skill and expected paths.

This supports the user’s note that skills are accessible now, while enabling future copying into repo for deterministic team usage.

## Design Decisions

1. Profiles are policy-level objects, not ad-hoc CLI flags.
Reason: keeps runs reproducible and reviewable in source-controlled policy.

2. Runner is explicit (`runner: codex`) rather than encoded in command text.
Reason: allows harness-specific validation and command construction without string parsing.

3. No migration window; cut over in one change.
Reason: reduces dual-path complexity and keeps policy semantics unambiguous.

4. Resolve skills by name with deterministic search order.
Reason: balances immediate usability (home skills) with long-term reproducibility (repo-local skills).

5. Preserve existing bootstrap signaling requirements in base prompt.
Reason: keeps compatibility with current guidance/completion orchestration contracts.

## Alternatives Considered

1. Keep only `agents[].command` and store profile info inside shell strings.
Rejected: no schema-level validation, poor readability, and brittle escaping.

2. Generate AGENT.md only and keep a generic command.
Rejected: AGENT.md helps behavior, but it does not model runner or explicit skill set in policy.

3. Hardcode a single global profile in code.
Rejected: removes per-agent flexibility and prevents future multi-runner support.

## Implementation Plan

1. Extend policy types and validation for `agent_profiles` + required `agents[].profile`.
2. Extend `model.AgentSpec` and run serialization to carry profile identity.
3. Add runner/profile command compiler package with initial `codex` adapter.
4. Implement skill resolver with repo-local then home fallback.
5. Add tests for policy validation, missing skills, and codex command assembly.
6. Add kickoff `--doc-repo` wiring (`RunSpec.DocRepo`) with default-first/override behavior and validation.
7. Route bootstrap sync + iteration ticket-doc updates to `<doc_repo>/ttmp`.
8. Update `.metawsm/policy.json` to define `codex-default` profile and switch `agent` to `profile` in the same change.
9. Remove raw-command support paths and update docs for profile authoring.

## Open Questions

Should we materialize resolved skill content into workspace files for auditability, or keep in-memory assembly only for v1?

## References

- `.metawsm/policy.json`
- `internal/policy/policy.go`
- `internal/orchestrator/service.go`
- `ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/reference/01-codex-default-profile-and-base-prompt.md`
