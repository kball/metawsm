# Changelog

## 2026-02-07

- Initial workspace created


## 2026-02-07

Created METAWSM-005 and documented a concrete agent-profile design: add policy-level profiles with runner/base_prompt/skills, wire agents to profiles, and seed a codex-default profile that explicitly requires docmgr task management plus diary maintenance during implementation.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/design-doc/01-agent-profile-model-and-runner-integration.md — Detailed architecture
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/reference/01-codex-default-profile-and-base-prompt.md — Copy/paste policy snippet and base prompt text for codex-default
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/tasks.md — Tracked completed design tasks and remaining implementation tasks


## 2026-02-07

Scope update: remove migration window. Agent profiles are now a single-cutover design where agents must use required profile references; legacy raw command support is explicitly out of scope.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/design-doc/01-agent-profile-model-and-runner-integration.md — Updated schema


## 2026-02-07

Implemented strict agent-profile cutover: policy now requires agents[].profile and compiles runner commands from agent_profiles (codex/shell), including codex base prompt + skill path resolution. Orchestrator now resolves agents with policy-path context. Added policy tests for schema validation and skill-aware command assembly, migrated example policy docs, and updated local policy to codex-default with docmgr+diary skills.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/README.md — Policy field docs updated for agent_profiles and agents.profile
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/examples/policy.example.json — Example migrated to profile-based codex configuration
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/model/types.go — AgentSpec carries profile metadata alongside compiled command
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — ResolveAgents call updated to pass policy path
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/policy/policy.go — Profile schema
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/policy/policy_test.go — Coverage for required profile mapping
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/reference/02-diary.md — Recorded implementation details


## 2026-02-07

Implemented doc-repo-aware docmgr routing for multi-repo workspaces: added kickoff --doc-repo (default first repo), persisted doc_repo in run spec, started agents/restarts in selected repo, and moved bootstrap/iteration ticket-doc ttmp paths under that repo. Updated codex base prompt text to match this contract and added regression tests for default/override/validation behavior.

### Related Files

- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/.metawsm/policy.json — Codex base prompt updated for selected doc repo ttmp root
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/cmd/metawsm/main.go — Kickoff doc-repo flag for run/bootstrap and usage updates
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/examples/policy.example.json — Example prompt updated for doc-repo-aware docmgr root
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/model/types.go — RunSpec doc_repo field
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service.go — Doc-repo selection
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/internal/orchestrator/service_test.go — Coverage for doc_repo default/override validation and repo-local ttmp behavior
- /Users/kball/workspaces/2026-02-07/metawsm/metawsm/ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/reference/02-diary.md — Step 2 diary entry with prompt context and validation details

