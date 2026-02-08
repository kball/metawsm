---
Title: Codex default profile and base prompt
Ticket: METAWSM-005
Status: active
Topics:
    - core
    - cli
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: .metawsm/policy.json
      Note: Current codex command baseline the profile snippet replaces
    - Path: internal/orchestrator/service.go
      Note: Codex normalization and tmux launch behavior that profile compiler must preserve
    - Path: ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/design-doc/01-agent-profile-model-and-runner-integration.md
      Note: Primary design rationale for this reference
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-07T10:15:32.014806-08:00
WhatFor: ""
WhenToUse: ""
---


# Codex default profile and base prompt

## Goal

Define a copy/paste-ready baseline profile for the current Codex runner that enforces `docmgr` workflow discipline and diary maintenance.

## Context

Current policy in this repo uses a raw inline Codex command. The profile model proposed in METAWSM-005 moves that behavior into structured config (`runner`, `base_prompt`, `skills`) so the contract is explicit and maintainable.

## Quick Reference

### Proposed policy snippet

```json
{
  "agent_profiles": [
    {
      "name": "codex-default",
      "runner": "codex",
      "skills": ["docmgr", "diary"],
      "base_prompt": "You are the implementation agent for this ticket run.\n1) You start in the kickoff-selected documentation repository, and that repository contains the docmgr ttmp root.\n2) Use docmgr there to manage tasks/docs/changelog.\n3) Keep a structured implementation diary as you work.\n4) If blocked, write .metawsm/guidance-request.json.\n5) When done, write .metawsm/implementation-complete.json and .metawsm/validation-result.json with status=passed and done_criteria matching the run brief.",
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

### Skill source resolution

```text
1) .metawsm/skills/<skill>/SKILL.md
2) $HOME/.codex/skills/<skill>/SKILL.md
3) fail with explicit missing-skill error
```

### Recommended base prompt text (expanded)

```text
You are running inside a metawsm workspace for one ticket.

Kickoff contract:
- The operator selects the documentation repository using `--doc-repo` (defaults to first `--repos` entry).
- You are started in that repository directory.
- `ttmp/` for docmgr is rooted in that repository.

Operating requirements:
- Treat docmgr as the source of truth for task/document lifecycle.
- Start by reading the ticket index, tasks, and relevant design/reference docs.
- Keep tasks current as work progresses (add/check tasks).
- Update changelog entries for meaningful decisions or implementation milestones.
- Relate modified code files to the relevant ticket documents.

Diary requirements:
- Maintain a Diary document in the ticket workspace.
- Record each substantial step with what changed, why, commands run, failures, and validation evidence.
- Include exact error output when something fails.

Execution contracts:
- If blocked, write .metawsm/guidance-request.json with a concrete question and context.
- On completion, write .metawsm/implementation-complete.json and .metawsm/validation-result.json.
```

## Usage Examples

1. Define the profile in `.metawsm/policy.json`.
2. Assign `profile: codex-default` to the desired agents.
3. Run bootstrap as usual:

```bash
go run ./cmd/metawsm bootstrap --ticket METAWSM-005 --repos metawsm --agent agent
go run ./cmd/metawsm bootstrap --ticket METAWSM-005 --repos metawsm,workspace-manager --doc-repo metawsm --agent agent
```

4. Confirm agent behavior through produced artifacts:
- `ttmp/.../reference/*diary*.md` updated during implementation.
- `tasks.md` and `changelog.md` progress entries present.
- `.metawsm/guidance-request.json` only when blocked.

## Related

- `ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/design-doc/01-agent-profile-model-and-runner-integration.md`
- `.metawsm/policy.json`
- `internal/orchestrator/service.go`
