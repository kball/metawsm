---
Title: Diary
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
      Note: Local runtime policy migrated to codex-default profile with docmgr+diary skills
    - Path: README.md
      Note: Documented agent profile policy fields
    - Path: cmd/metawsm/main.go
      Note: Added --doc-repo kickoff flag for run/bootstrap
    - Path: examples/policy.example.json
      Note: Migrated example policy to profile-based codex configuration
    - Path: internal/model/types.go
      Note: |-
        Expanded AgentSpec with profile/runner/skills metadata
        Added run-spec doc_repo field
    - Path: internal/orchestrator/service.go
      Note: |-
        Wired ResolveAgents to use active policy path
        Routed tmux cwd and ticket sync/feedback paths through selected doc repo
    - Path: internal/orchestrator/service_test.go
      Note: Added/updated tests for doc-repo default/override and repo-local ttmp behavior
    - Path: internal/policy/policy.go
      Note: Implemented profile schema
    - Path: internal/policy/policy_test.go
      Note: Added profile-cutover and codex skill resolution tests with deterministic missing-skill coverage
    - Path: ttmp/2026/02/07/METAWSM-005--agent-profiles-for-runner-base-prompt-and-skills/tasks.md
      Note: Marked implementation/testing/cutover tasks complete
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-07T13:32:13.666913-08:00
WhatFor: ""
WhenToUse: ""
---




# Diary

## Goal

Capture the full implementation trail for METAWSM-005: policy/profile cutover, test changes, validation outcomes, and review-critical details.

## Step 1: Implement strict profile cutover and codex skill-aware command compilation

I implemented the all-at-once migration from agent raw commands to required profile references, then wired command assembly through profile runners. The new path compiles runtime commands from `agent_profiles` and validates profile integrity up front.

I also migrated the local policy config to the new schema with `codex-default` plus `docmgr` and `diary` skills, updated docs/examples, and verified the full Go test suite.

### Prompt Context

**User prompt (verbatim):** "ok build it, keep a diary as you go"

**Assistant interpretation:** Implement the design from METAWSM-005 now, and keep a detailed implementation diary while making and validating changes.

**Inferred user intent:** Complete the feature end-to-end with an auditable engineering trail so the profile system is immediately usable and reviewable.

**Commit (code):** N/A (not committed in this session)

### What I did
- Added profile-aware policy model in `internal/policy/policy.go`:
  - `agent_profiles` with `runner`, `base_prompt`, `skills`, `runner_options`.
  - `agents[].profile` required, with strict validation and profile existence checks.
  - Runner compilation for `codex` and `shell`.
  - Skill resolution order: repo-local `.metawsm/skills/<skill>/SKILL.md`, then `$HOME/.codex/skills/<skill>/SKILL.md`.
- Updated orchestration wiring in `internal/orchestrator/service.go` to pass policy path into `ResolveAgents` so skill resolution is anchored to the active policy location.
- Extended runtime model in `internal/model/types.go` (`AgentSpec`) to persist profile metadata (`profile`, `runner`, `skills`) alongside compiled `command`.
- Rewrote `internal/policy/policy_test.go` to cover:
  - new default policy validity/version,
  - required `agent.profile`,
  - codex command compilation with resolved skill paths,
  - missing-skill failure behavior.
- Migrated policy example schema in `examples/policy.example.json` to profile-based config with `codex-default`, `docmgr`, and `diary`.
- Updated policy docs in `README.md` (new profile fields).
- Migrated local runtime policy `.metawsm/policy.json` to `agent_profiles` + `agents[].profile`.
- Copied skill files into local runtime path for deterministic local resolution:
  - `.metawsm/skills/docmgr/SKILL.md`
  - `.metawsm/skills/diary/SKILL.md`
- Formatted code with:
  - `gofmt -w internal/model/types.go internal/policy/policy.go internal/policy/policy_test.go internal/orchestrator/service.go`
- Ran validation:
  - `go test ./...`

### Why
- The ticket goal is a hard cutover to agent profiles with no compatibility path for legacy raw command definitions.
- Profiles separate stable behavior contracts (runner/prompt/skills) from shell-escaping details and make policy review much clearer.
- Enforcing required `profile` mapping removes ambiguous dual-mode behavior.

### What worked
- Profile validation catches structural mistakes early (missing profile, unknown profile, unsupported runner, missing shell command/base prompt constraints).
- Codex command compilation now consistently injects the prompt and skill requirements from policy.
- Full test suite passes after the cutover (`go test ./...`).

### What didn't work
- Initial test run failed in `internal/policy`:
  - Command: `go test ./...`
  - Error: `--- FAIL: TestResolveAgentsFailsWhenSkillMissing ... expected missing skill error`
- Root cause: the test unintentionally resolved `docmgr` from the real home directory fallback.
- Fix: isolate `HOME` in the test with `t.Setenv("HOME", filepath.Join(root, "home"))`.

### What I learned
- Skill fallback to user-home paths is useful for local operation but requires explicit environment isolation in tests to keep failure cases deterministic.
- Passing the resolved policy path into agent resolution keeps profile-derived behavior anchored to the active config context.

### What was tricky to build
- Implementing strict cutover semantics while preserving good defaults and test stability required introducing a shell runner profile for defaults, while supporting codex profile compilation for actual runtime use.
- Keeping profile command assembly robust with shell quoting while preserving downstream codex normalization behavior required careful compatibility with existing tmux wrapper logic.

### What warrants a second pair of eyes
- `internal/policy/policy.go` command-assembly and skill-path resolution assumptions (especially repo-root derivation from policy path).
- Whether user-home skill fallback should remain required/strict vs optional in production workflows.
- Interaction between long codex prompts and shell command length in very large skill packs.

### What should be done in the future
- Consider adding a machine-readable “skills bundle hash” to run metadata for better auditability of what skill content/paths were used at execution time.

### Code review instructions
- Start in `internal/policy/policy.go`:
  - `Config`, `AgentProfile`, `Validate`, `ResolveAgents`, `compileProfileCommand`, `buildCodexPrompt`, `skillResolver`.
- Review orchestration integration in `internal/orchestrator/service.go` (`Run` call to `ResolveAgents`).
- Review model impact in `internal/model/types.go` (`AgentSpec`).
- Validate behavior with:
  - `go test ./...`

### Technical details
- New policy schema key: `agent_profiles`.
- `AgentSpec.Command` is now compiled from profile runner settings, not read from policy agent entries.
- Codex prompt composition includes:
  - `base_prompt`
  - resolved skill references as required context lines.

## Step 2: Route docmgr ttmp into a selected repo and make kickoff overrideable

I implemented the follow-up requirement that docmgr context must live inside one repository within a multi-repo workspace, not at workspace root. The orchestrator now carries a `doc_repo` kickoff choice (default first repo, overrideable by flag) and consistently uses it for tmux working directory and ticket doc sync targets.

I also updated the codex base prompt guidance to reflect this runtime contract: the agent starts in the selected documentation repo and uses that repo's `ttmp/` as docmgr root.

### Prompt Context

**User prompt (verbatim):** "I updated with an additional skill... this is looking pretty good except we want the ttmp to be inside one of the repos, even though wsm will be a folder with multiple repos. The repo that should be used for docmgr should be passed as a part of the kickoff (default to first, but be overrideable). This will mean the base prompt needs to be a bit different"

**Assistant interpretation:** Add a kickoff-time doc repo selector, default it to the first repo, route all workspace docmgr `ttmp` behavior into that repo, and adjust base prompt wording accordingly.

**Inferred user intent:** Ensure multi-repo workspaces have deterministic docmgr ownership and agent instructions that match actual directory layout.

**Commit (code):** N/A (not committed in this session)

### What I did
- Added `DocRepo` to runtime spec/options:
  - `internal/model/types.go` (`RunSpec.DocRepo`)
  - `internal/orchestrator/service.go` (`RunOptions.DocRepo`, defaulting/validation)
- Added kickoff CLI flag:
  - `cmd/metawsm/main.go` now accepts `--doc-repo` for `run` and `bootstrap`.
- Implemented doc repo resolution:
  - `resolveDocRepoPath` in `internal/orchestrator/service.go`.
  - Default behavior chooses first repo; explicit value must be in `--repos`.
- Updated execution paths to use selected doc repo:
  - `tmux_start` now uses `<workspace>/<doc_repo>` as working dir.
  - `restart` uses same doc repo working dir.
  - Bootstrap ticket sync writes to `<doc_repo>/ttmp/...`.
  - Iteration feedback ticket-doc writes look under `<doc_repo>/ttmp/...`.
- Updated tests in `internal/orchestrator/service_test.go`:
  - doc repo default persisted in run spec,
  - explicit doc repo override persisted,
  - invalid doc repo rejected,
  - sync destination under doc repo root,
  - restart/iteration expectations adjusted for repo-local `ttmp`.
- Updated prompts/docs/config:
  - `examples/policy.example.json` base prompt text updated for doc-repo-rooted `ttmp`.
  - `.metawsm/policy.json` base prompt updated similarly.
  - `README.md` documents `--doc-repo` behavior and examples.
- Ran validation:
  - `gofmt -w cmd/metawsm/main.go internal/model/types.go internal/orchestrator/service.go internal/orchestrator/service_test.go`
  - `go test ./...`

### Why
- The user requirement is explicit: docmgr state must be owned by one selected repo in multi-repo workspaces.
- Starting agents in that repo keeps docmgr root resolution and prompt expectations aligned without extra shell setup.
- Making selection explicit at kickoff prevents hidden heuristics and keeps behavior predictable.

### What worked
- Full test suite passed after the change.
- Dry-run specs now record `doc_repo` and support explicit override.
- Restart and iterate workflows use the same doc-repo working-dir semantics as initial tmux start.

### What didn't work
- No runtime failures encountered during this step.

### What I learned
- Repo-local docmgr routing touches more than bootstrap sync; restart and iteration feedback paths also need to follow the same doc-root contract to avoid split behavior.

### What was tricky to build
- Preserving existing behavior for synthetic/single-repo layouts while enforcing repo-local `ttmp` for multi-repo workspaces required careful path resolution and test fixture updates.

### What warrants a second pair of eyes
- `resolveDocRepoPath` behavior for unusual workspace layouts (single-repo root vs nested-repo folders) should be reviewed for long-term edge cases.

### What should be done in the future
- Consider exposing selected doc repo in `metawsm status` output so operators can verify docmgr ownership quickly.

### Code review instructions
- Start with kickoff and spec plumbing:
  - `cmd/metawsm/main.go`
  - `internal/model/types.go`
  - `internal/orchestrator/service.go` (`Run`, `executeSingleStep`, `Restart`, `recordIterationFeedback`, `resolveDocRepoPath`)
- Validate behavior with:
  - `go test ./...`

### Technical details
- New kickoff flag: `--doc-repo`.
- New run-spec field: `doc_repo`.
- Doc sync destination changed from `<workspace>/ttmp/...` to `<workspace>/<doc_repo>/ttmp/...`.
