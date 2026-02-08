---
Title: Diary
Ticket: METAWSM-009
Status: active
Topics:
    - cli
    - core
DocType: reference
Intent: long-term
Owners: []
RelatedFiles:
    - Path: README.md
      Note: Documented new git_pr validation policy knobs (commit d31a862)
    - Path: cmd/metawsm/main.go
      Note: |-
        Added Proposal A auth check command and readiness helpers (commit 6148470)
        Added metawsm commit command and dry-run previews (commit 9de30b7)
        Added metawsm pr command with dry-run previews (commit 180a976)
        Integrated operator commit/pr readiness parsing
        Enforced --human acknowledgement for merge and updated run_done hints/usage (commit 4dca4ec)
    - Path: cmd/metawsm/main_test.go
      Note: |-
        Added auth check command and repo path resolution tests (commit 6148470)
        Added commit command selector validation test (commit 9de30b7)
        Added metawsm pr selector validation test (commit 180a976)
        Added readiness parsing/rule/hint tests for operator loop integration (commit b3587e3)
        Added merge human-ack and run_done hint coverage (commit 4dca4ec)
    - Path: cmd/metawsm/operator_llm.go
      Note: Added commit_ready/pr_ready intents and per-rule execute behavior (commit b3587e3)
    - Path: cmd/metawsm/operator_llm_test.go
      Note: Added merge-decision execute preservation coverage (commit b3587e3)
    - Path: examples/policy.example.json
      Note: |-
        Added git_pr policy block defaults for Proposal A rollout (commit d3f13f6)
        Added git_pr policy defaults (commit d3f13f6)
        Documented new git_pr validation configuration fields (commit d31a862)
    - Path: internal/model/types.go
      Note: |-
        Added RunPullRequest model and pull request state enums (commit d3f13f6)
        Added RunPullRequest model and PR state enums (commit d3f13f6)
    - Path: internal/orchestrator/git_pr_validation.go
      Note: Added extensible required-check validation framework and built-in checks (commit d31a862)
    - Path: internal/orchestrator/service.go
      Note: |-
        Surfaced persisted run PR metadata in status output (commit 283a68b)
        Commit service primitive implementation (commit 678b936)
        Added OpenPullRequests primitive and credential/actor run event recording for commit/pr actions (commit 180a976)
        Wired commit/PR gate enforcement and validation_json persistence (commit d31a862)
        Implemented multi-ticket workspace fanout for commit/pr flows (commit 6ec9185)
        Added push-before-PR execution and dry-run push previews (commit 627e397)
    - Path: internal/orchestrator/service_test.go
      Note: |-
        Added status test for pull request section (commit 283a68b)
        Commit primitive tests (commit 678b936)
        Added OpenPullRequests dry-run and fake-gh integration tests (commit 180a976)
        Added commit/pr preflight rejection coverage for mode-off and invalid-state scenarios (commit 299a096)
        Added validation-gate regression tests including require_all semantics and forbidden/clean-tree paths (commit d31a862)
        Added multi-ticket fanout commit/pr coverage and fixture support (commit 6ec9185)
        Added end-to-end commit/push/PR test and updated PR dry-run/real tests (commit 627e397)
    - Path: internal/policy/policy.go
      Note: |-
        Added git_pr config schema/defaults/validation (commit d3f13f6)
        Added git_pr policy contract defaults/validation (commit d3f13f6)
        Branch template rendering helper (commit 678b936)
        Extended git_pr schema with test_commands and forbidden_file_patterns plus supported-check validation (commit d31a862)
    - Path: internal/policy/policy_test.go
      Note: |-
        Added tests for git_pr defaults and validation failures (commit d3f13f6)
        Added git_pr validation coverage (commit d3f13f6)
        Branch template rendering tests (commit 678b936)
        Added policy validation coverage for required-check and command/pattern constraints (commit d31a862)
    - Path: internal/store/sqlite.go
      Note: |-
        Added run_pull_requests schema and store CRUD methods (commit d3f13f6)
        Added run_pull_requests schema and store methods (commit d3f13f6)
    - Path: internal/store/sqlite_test.go
      Note: |-
        Added persistence test for run pull request metadata across reopen (commit d3f13f6)
        Added run_pull_requests persistence test (commit d3f13f6)
    - Path: ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/playbook/01-operator-and-agent-commit-pr-workflow.md
      Note: Added operator/agent commit-PR workflow and Proposal A setup playbook guidance
ExternalSources: []
Summary: ""
LastUpdated: 2026-02-08T11:22:26-08:00
WhatFor: Implementation diary for METAWSM-009 commit/PR automation rollout.
WhenToUse: Use when reviewing what was implemented, how it was validated, and what failed during each step.
---














# Diary

## Goal

Capture each implementation step for METAWSM-009 with exact code changes, test outcomes, failures, and review guidance.

## Step 1: Add git_pr Policy Contract and Persisted Run PR Metadata

This step established the foundation needed before command-level automation: a policy contract for commit/PR behavior and durable storage for per-run PR metadata. Without these two pieces, later CLI and operator actions would have no validated configuration surface or persisted state.

I implemented defaults and validation for Proposal A (`local_user_auth`) and added a new SQLite table/API for run-scoped PR records keyed by run/ticket/repo. This keeps future commit/PR actions auditable and restart-safe.

### Prompt Context

**User prompt (verbatim):** "ok go ahead and implement. Keep a diary and commit as you go."

**Assistant interpretation:** Start implementing METAWSM-009 now, break work into incremental commits, and keep documentation of each step in a diary.

**Inferred user intent:** Move from planning docs to real product changes for agent commit/PR automation while preserving traceability and reviewability.

**Commit (code):** d3f13f6 — "Add git_pr policy and persisted run PR metadata"

### What I did
- Added `git_pr` policy block to `internal/policy/policy.go` with:
  - `mode` (`off|assist|auto`)
  - `credential_mode` (`local_user_auth`)
  - `branch_template`
  - `require_all`
  - `required_checks`, `allowed_repos`, `default_labels`, `default_reviewers`
- Set defaults for Proposal A in `policy.Default()` and added strict validation checks in `policy.Validate()`.
- Added `RunPullRequest` and `PullRequestState` model types in `internal/model/types.go`.
- Added `run_pull_requests` table plus store methods in `internal/store/sqlite.go`:
  - `UpsertRunPullRequest`
  - `ListRunPullRequests`
- Added policy tests in `internal/policy/policy_test.go` for:
  - default values
  - invalid mode/credential mode
  - empty branch template
- Added store persistence coverage in `internal/store/sqlite_test.go` with reopen verification.
- Updated `examples/policy.example.json` to include the `git_pr` block.
- Ran focused tests for modified packages.

### Why
- Commit/PR automation needs validated policy to gate behavior safely.
- PR metadata persistence is required for auditability, operator visibility, and restart recovery.

### What worked
- `internal/policy` tests passed with the new `git_pr` schema and validation rules.
- `internal/store` tests passed with `run_pull_requests` persistence and reload behavior.
- Existing code paths remained compatible because missing `git_pr` fields still resolve through `policy.Default()`.

### What didn't work
- Initial test commands failed in sandbox due Go cache permissions.
- Command: `go test ./internal/policy -count=1`
- Error:
  - `open /Users/kball/Library/Caches/go-build/...: operation not permitted`
- Command: `go test ./internal/store -count=1`
- Error:
  - `open /Users/kball/Library/Caches/go-build/...: operation not permitted`
- Fix: reran with local cache overrides:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/store -count=1`

### What I learned
- The existing store/test architecture made it straightforward to add another persisted run-scoped state table without a migration framework change.
- Policy extensibility is easiest when defaults are complete and validation rejects partial/ambiguous states early.

### What was tricky to build
- Choosing a `git_pr` surface that is strict enough for safety but still forward-compatible (especially validation list fields and credential mode constraints).

### What warrants a second pair of eyes
- Whether `git_pr.mode` default should remain `assist` vs `off` for first release.
- Whether `run_pull_requests` should include additional identity fields now (for example PR head repo/fork metadata) before CLI wiring begins.

### What should be done in the future
- Wire this policy + persisted state into service and CLI command paths (`commit`, `pr`, and `auth check`) in the next step.

### Code review instructions
- Start in `internal/policy/policy.go`:
  - `Config.GitPR` shape
  - defaults in `Default()`
  - checks in `Validate()`
- Then review `internal/store/sqlite.go`:
  - schema for `run_pull_requests`
  - `UpsertRunPullRequest` and `ListRunPullRequests`
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/store -count=1`

### Technical details
- `git_pr` default block:
```json
{
  "mode": "assist",
  "credential_mode": "local_user_auth",
  "branch_template": "{ticket}/{repo}/{run}",
  "require_all": true,
  "required_checks": ["tests"]
}
```
- `run_pull_requests` primary key: `(run_id, ticket, repo)`.

## Step 2: Add Proposal A `metawsm auth check` Command

This step added an explicit auth readiness command for Proposal A so operators can verify push/PR prerequisites before commit/PR automation attempts. It gives deterministic output for GitHub auth and run-scoped git identity checks.

I introduced a new `metawsm auth check` command that validates `gh` auth state, git `user.name`/`user.email`, and `origin` availability across run workspace repos, then reports `Push ready` and `PR ready` flags.

### Prompt Context

**User prompt (verbatim):** "ok go ahead and implement. Keep a diary and commit as you go."

**Assistant interpretation:** Continue implementing METAWSM-009 with incremental commits and keep the implementation diary up to date after each slice.

**Inferred user intent:** Get working Proposal A automation primitives now, with operational visibility and traceable progress.

**Commit (code):** 6148470 — "Add Proposal A auth check command"

### What I did
- Added `auth` command routing in `cmd/metawsm/main.go`.
- Implemented `metawsm auth check` with:
  - policy load + credential mode enforcement (`local_user_auth`),
  - `gh auth status` verification,
  - actor discovery via `gh api user --jq .login`,
  - run-scoped repo checks for git identity (`user.name`, `user.email`) and `origin` remote.
- Added helper functions:
  - `checkGitHubLocalAuth`
  - `checkRunGitCredentials`
  - `resolveWorkspaceRepoPath`
  - `gitConfigValue`
  - `gitRemoteOrigin`
- Updated CLI usage text to include `metawsm auth check`.
- Added tests in `cmd/metawsm/main_test.go` for:
  - required subcommand behavior,
  - repo path resolution behavior for nested and single-repo layouts.
- Ran focused package tests and a manual command invocation smoke test.

### Why
- Proposal A depends on local credentials and local git configuration.
- Operators need a deterministic preflight command before triggering push/PR behavior.

### What worked
- `go test ./cmd/metawsm -count=1` passed with cache overrides.
- `go test ./internal/policy -count=1` and `go test ./internal/store -count=1` stayed green after command integration.
- `go run ./cmd/metawsm auth check` produced clear readiness output and remediation detail.

### What didn't work
- Manual smoke check showed local `gh` auth token is invalid in this environment:
- Command: `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go run ./cmd/metawsm auth check`
- Output included:
  - `X github.com: authentication failed`
  - `The github.com token in oauth_token is no longer valid.`
- This is expected behavior from the new preflight command, not a command failure.
- While updating changelog notes via shell, unescaped backticks in a `docmgr changelog update --entry ...` string triggered shell command substitution:
- Error: `zsh:1: command not found: metawsm`
- Fix: replaced the malformed changelog text with plain `metawsm auth check` text (no backticks in shell argument).

### What I learned
- A run-aware auth check needs robust repo path resolution for both nested repo workspaces and single-repo root workspaces.
- Surfacing remediation details from CLI tools directly helps operators recover faster.

### What was tricky to build
- Balancing strict failure behavior (non-zero when not ready) with enough diagnostic detail to make fixes obvious.

### What warrants a second pair of eyes
- Whether `auth check` should fail hard when no run selector is provided and GitHub auth is unavailable, or support a softer informational mode.
- Output formatting consistency if we later add additional credential modes.

### What should be done in the future
- Add event recording for credential mode + actor in commit/pr action flows (task 13).
- Add support for future credential modes without changing current command UX.

### Code review instructions
- Start in `cmd/metawsm/main.go`:
  - command switch (`case "auth"`)
  - `authCommand`
  - helper functions for auth and repo checks
- Then review tests in `cmd/metawsm/main_test.go`:
  - `TestAuthCommandRequiresCheckSubcommand`
  - `TestResolveWorkspaceRepoPath*`
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Technical details
- Command usage:
```bash
metawsm auth check [--run-id RUN_ID | --ticket TICKET] [--policy PATH]
```
- Readiness summary fields:
  - `Credential mode`
  - `GitHub CLI` installed/authed/actor
  - per-repo readiness lines
  - `Push ready`
  - `PR ready`

## Step 3: Surface Persisted Run PR Metadata in Orchestrator Status

This step connected the new persisted PR state to user-visible status output so operators can see PR lifecycle context directly in `metawsm status` without manual DB inspection. The main objective was observability before commit/pr command automation is fully wired.

I added service wrappers for `run_pull_requests`, rendered a `Pull Requests:` section in status output, and added a test that verifies this section appears with expected ticket/repo/state/URL details.

### Prompt Context

**User prompt (verbatim):** "ok go ahead and implement. Keep a diary and commit as you go."

**Assistant interpretation:** Continue implementing METAWSM-009 in incremental slices and keep documentation and commits synchronized.

**Inferred user intent:** Ship practical end-user capabilities incrementally, not just internal data structures.

**Commit (code):** 283a68b — "Expose persisted run PRs in orchestrator status"

### What I did
- Added service wrappers in `internal/orchestrator/service.go`:
  - `UpsertRunPullRequest`
  - `ListRunPullRequests`
- Updated `Service.Status` rendering to include a `Pull Requests:` section when records exist.
- Added test `TestStatusShowsPersistedRunPullRequests` in `internal/orchestrator/service_test.go`.
- Ran focused tests:
  - `go test ./internal/orchestrator -count=1`
  - `go test ./cmd/metawsm -count=1`

### Why
- PR automation needs operator-facing visibility from the core status surface.
- This avoids hidden state and makes upcoming commit/pr workflows easier to review and debug.

### What worked
- New status section renders with ticket/repo/state/head/base/number/url/actor fields.
- Added test passed and existing orchestrator/cmd tests remained green.

### What didn't work
- First test run failed due a helper function typo introduced during status rendering.
- Error:
  - `undefined: emptyValue`
- Fix:
  - replaced references with a local service helper `valueOrDefault` and reran tests successfully.
- While writing changelog text, backticks in a `docmgr changelog update --entry ...` shell argument again triggered command substitution.
- Error: `zsh:1: command not found: metawsm`
- Fix: corrected the changelog line to plain `metawsm status` text without shell-interpreted backticks.

### What I learned
- Status rendering changes are low-risk if backed by precise string-presence tests, especially in this CLI-first workflow.

### What was tricky to build
- Keeping status output concise while still exposing enough PR metadata to be actionable for operators.

### What warrants a second pair of eyes
- Whether the displayed PR fields are sufficient for operator triage, or if we should include timestamp/error context directly in the section.

### What should be done in the future
- Wire commit/pr commands to create and update these records in real workflows.

### Code review instructions
- Start in `internal/orchestrator/service.go`:
  - PR wrapper methods
  - `Pull Requests:` render block in `Status`
- Then review `internal/orchestrator/service_test.go`:
  - `TestStatusShowsPersistedRunPullRequests`
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Technical details
- New status section is conditional and only shown when at least one `run_pull_requests` record exists for the run.

## Step 4: Add Commit Preparation Service Primitives and Policy-Driven Branch Rendering

This step implemented the first executable commit workflow layer in the orchestrator service so runs can deterministically prepare branches and create commits per workspace/repo. The intent was to complete the service primitive milestone before adding CLI command surfaces.

I added a `Service.Commit` primitive with dry-run previews, branch creation from policy templates, commit execution, and persistence updates to `run_pull_requests` so later PR creation can reuse the same run/ticket/repo metadata.

### Prompt Context

**User prompt (verbatim):** "use docmgr to pick up ticket METAWSM-009 --- look at the plan, diary, and tasks, and then continue implementing. Keep a diary and commit as you go."

**Assistant interpretation:** Continue the ticket from existing docs, implement the next planned code slice, and keep incremental diary + commit discipline.

**Inferred user intent:** Advance actual METAWSM-009 functionality now (not just planning), while preserving auditability in both git history and ticket docs.

**Commit (code):** 678b936 — "Add commit preparation primitives for run workspaces"

### What I did
- Added policy helper `RenderGitBranch` in `internal/policy/policy.go` to render/sanitize branch names from `git_pr.branch_template` placeholders (`{ticket}`, `{repo}`, `{run}`).
- Added branch-rendering tests in `internal/policy/policy_test.go` for default template behavior, custom templates, and empty-segment fallback.
- Added orchestrator commit primitives in `internal/orchestrator/service.go`:
  - new `CommitOptions`, `CommitResult`, `CommitRepoResult` types,
  - new `Service.Commit` method that resolves run/ticket/workspaces/repos, enforces readiness constraints, performs branch prep + commit (or dry-run preview),
  - helper functions for repo target resolution, allow-list filtering, base ref resolution, git command execution, and default commit message generation.
- Persisted commit metadata into `run_pull_requests` rows (head/base branch, commit SHA, credential mode, actor) and recorded commit events.
- Added orchestrator tests in `internal/orchestrator/service_test.go`:
  - `TestCommitDryRunPreviewsActionsForDirtyRepo`
  - `TestCommitCreatesBranchCommitAndPersistsPullRequestRow`
  - `TestCommitSkipsCleanRepoWithoutPersistingRow`
- Ran focused tests:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Why
- Task 3 required service-level branch prep and commit creation before command-surface wiring.
- A reusable branch renderer in policy avoids duplicating branch naming logic and keeps behavior tied to configurable templates.
- Persisting commit metadata at commit time prepares the PR layer to reference authoritative branch/commit state.

### What worked
- All targeted tests passed after the implementation.
- Dry-run and real commit paths both behaved deterministically in new orchestrator tests.
- `run_pull_requests` rows now capture commit context needed for subsequent PR creation.

### What didn't work
- N/A in this step; no failing test or command retries were required after initial implementation.

### What I learned
- The existing run/ticket/workspace abstractions in orchestrator made it straightforward to add a commit primitive without changing store schema.
- Policy-template rendering needed an explicit helper to keep branch naming predictable and testable.

### What was tricky to build
- Handling mixed workspace layouts (nested repo directory vs single-repo workspace root) while keeping repo labels stable for branch template rendering and persistence keys.

### What warrants a second pair of eyes
- Current `Service.Commit` gate requires run status `complete`; reviewers may want to confirm whether commit execution should also be allowed in `running` or `paused` states.
- The default commit message fallback (`ticket: first run brief goal line`) may need stronger normalization/length constraints before public CLI exposure.

### What should be done in the future
- Add `metawsm commit` CLI command wired to `Service.Commit` with explicit dry-run output formatting.
- Implement PR creation primitive (`gh pr create`) that consumes persisted commit metadata and updates PR URL/number/state.

### Code review instructions
- Start in `internal/orchestrator/service.go`:
  - `Service.Commit`
  - `resolveWorkspaceCommitRepoTargets`
  - `resolveCommitBaseRef`
  - `runGitCommand`
- Review branch template logic in `internal/policy/policy.go`:
  - `RenderGitBranch`
- Review test coverage in:
  - `internal/orchestrator/service_test.go`
  - `internal/policy/policy_test.go`
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Technical details
- Branch rendering defaults to `{ticket}/{repo}/{run}` and sanitizes each segment using the same token normalization rules as session-name rendering.
- Commit persistence behavior updates `run_pull_requests` keyed by `(run_id, ticket, repo)` and sets `pr_state` to `draft` when no prior PR state exists.
- Dry-run behavior returns concrete git actions (`checkout -B`, `add -A`, `commit -m`) without mutating repository state.

## Step 5: Add `metawsm commit` CLI Command with Dry-Run Previews

This step exposed the new service primitive through the CLI so operators can invoke commit preparation and execution using a first-class command. The goal was to complete the command-surface task immediately after the service-layer foundation landed.

I added `metawsm commit` with run/ticket selection, optional commit message and actor flags, and dry-run output that prints the exact git actions for each repo target.

### Prompt Context

**User prompt (verbatim):** "use docmgr to pick up ticket METAWSM-009 --- look at the plan, diary, and tasks, and then continue implementing. Keep a diary and commit as you go."

**Assistant interpretation:** Continue shipping the next planned task after service primitives, while preserving incremental commits and diary updates.

**Inferred user intent:** Move toward usable end-to-end commit/PR automation by turning backend primitives into operator-facing CLI workflows.

**Commit (code):** 9de30b7 — "Add metawsm commit command with dry-run previews"

### What I did
- Added `commit` routing in `cmd/metawsm/main.go` command switch.
- Implemented `commitCommand` in `cmd/metawsm/main.go` with flags:
  - `--run-id`, `--ticket`, `--db`, `--message`, `--actor`, `--dry-run`
- Wired CLI execution to `service.Commit(...)` using the new orchestrator primitive.
- Added command output formatting for:
  - dry-run mode (per-repo git command previews),
  - real mode (commit SHA output),
  - clean-repo skip reporting.
- Updated `printUsage()` to include `metawsm commit ...`.
- Added test `TestCommitCommandRequiresRunSelector` in `cmd/metawsm/main_test.go`.
- Ran focused tests:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`

### Why
- Task 5 required CLI exposure of commit primitives with dry-run previews.
- The operator workflow needs a command-level entry point before PR creation can be layered on top.

### What worked
- Command parsing and output behavior integrated cleanly with existing CLI patterns.
- `cmd/metawsm` and `internal/orchestrator` tests passed after final fix.

### What didn't work
- First `cmd/metawsm` test run failed due exact error-string mismatch in the new test assertion.
- Command:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`
- Error:
  - `--- FAIL: TestCommitCommandRequiresRunSelector (0.00s)`
  - `main_test.go:379: unexpected error: one of --run-id or --ticket is required`
- Fix:
  - updated assertion text in `TestCommitCommandRequiresRunSelector` to match the existing `requireRunSelector` error string.

### What I learned
- Existing selector and command patterns in `main.go` made adding new subcommands low-risk when reusing `requireRunSelector` and per-command flagsets.

### What was tricky to build
- Keeping output concise while still surfacing enough per-repo detail for dry-run trust (branch/base/message/actions) and non-dry-run confirmation (commit SHA).

### What warrants a second pair of eyes
- Whether the current `metawsm commit` output should include credential mode/actor explicitly in CLI output (currently persisted in store/events but not shown on command output).

### What should be done in the future
- Add `metawsm pr` command that consumes persisted commit metadata and creates PRs via `gh pr create`.
- Consider adding a `--json` output mode for commit/pr commands for automation workflows.

### Code review instructions
- Start in `cmd/metawsm/main.go`:
  - `case "commit"`
  - `commitCommand`
  - usage line in `printUsage`
- Review `cmd/metawsm/main_test.go`:
  - `TestCommitCommandRequiresRunSelector`
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`

### Technical details
- `metawsm commit` delegates to `orchestrator.CommitOptions` and prints per-repo results from `CommitResult.Repos`.
- Dry-run output includes each queued git action (`checkout -B`, `add -A`, `commit -m`) so operators can verify intent before mutation.

## Step 6: Add PR Service Primitive, `metawsm pr` Command, and Commit/PR Actor Event Metadata

This step completed the first end-to-end PR automation slice by adding a service primitive that invokes `gh pr create`, persists PR metadata, and exposes the workflow through a new `metawsm pr` CLI command. The intent was to close the next high-priority implementation tasks immediately after `metawsm commit` landed.

I also extended run event messages so commit and PR actions include credential mode and actor identity, satisfying Proposal A tracking requirements for who performed commit/PR operations.

### Prompt Context

**User prompt (verbatim):** "use docmgr to pick up ticket METAWSM-009 --- look at the plan, diary, and tasks, and then continue implementing. Keep a diary and commit as you go."

**Assistant interpretation:** Keep executing the next planned implementation slices and maintain incremental commits plus ticket diary updates.

**Inferred user intent:** Progress from commit-only support to practical commit+PR automation while preserving operational traceability.

**Commit (code):** 180a976 — "Add pull request service and metawsm pr command"

### What I did
- Added PR service types and primitive in `internal/orchestrator/service.go`:
  - `PullRequestOptions`, `PullRequestResult`, `PullRequestRepoResult`
  - `Service.OpenPullRequests(...)`
- Implemented PR primitive behavior:
  - resolve run/ticket/rows from persisted `run_pull_requests`,
  - render default PR title/body from run brief context,
  - support dry-run previews with exact `gh pr create` command output,
  - execute `gh pr create` in repo path for real mode,
  - parse PR URL/number from command output,
  - persist PR URL/number/state (`open`) back into `run_pull_requests`.
- Added helper utilities in service layer:
  - command preview/exec helpers,
  - PR URL parsing,
  - default PR summary/title/body generation.
- Added run event messages for PR creation and ensured commit/pr event messages include `credential_mode` and `actor` details.
- Added orchestrator tests in `internal/orchestrator/service_test.go`:
  - `TestOpenPullRequestsDryRunPreviewsCreateCommand`
  - `TestOpenPullRequestsCreatesAndPersistsMetadata` (uses a fake `gh` binary on PATH)
- Added CLI command in `cmd/metawsm/main.go`:
  - new `pr` command routing,
  - `prCommand` flags (`--run-id`, `--ticket`, `--title`, `--body`, `--actor`, `--dry-run`),
  - dry-run and real-mode output formatting.
- Updated CLI usage output with `metawsm pr ...`.
- Added CLI selector test in `cmd/metawsm/main_test.go`:
  - `TestPRCommandRequiresRunSelector`
- Ran focused tests:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Why
- Task 4 required a service primitive for GitHub PR creation through `gh`.
- Task 6 required a `metawsm pr` CLI surface with dry-run previews.
- Task 13 required recording credential mode + actor identity for commit/pr actions.

### What worked
- Service and CLI flows compiled and passed targeted tests.
- Fake `gh` test path provided deterministic PR creation verification without external network/auth dependencies.
- Persisted PR rows now include URL/number/state and actor/credential metadata.

### What didn't work
- N/A in this step; implementation and focused tests passed without iteration failures.

### What I learned
- Existing persisted `run_pull_requests` rows created by the commit workflow made PR creation wiring straightforward and restart-safe.
- A fake executable on `PATH` is an effective pattern for testing external CLI integrations (`gh`) in unit/integration tests.

### What was tricky to build
- Designing defaults that are useful but predictable for multi-repo runs (title/body composition, repo-specific fallback behavior, and skipping rows that already have PR URLs).

### What warrants a second pair of eyes
- Whether the current PR default title/body templates are the right long-term contract for reviewers (especially multi-repo ticket runs).
- Whether skipped-existing-PR behavior should evolve into a first-class update/edit path (`gh pr edit`) rather than skip.

### What should be done in the future
- Add validation gate framework and enforce required checks before commit/PR execution.
- Integrate commit/pr readiness signals into operator loop with assist/auto mode behavior controls.

### Code review instructions
- Start in `internal/orchestrator/service.go`:
  - `Service.OpenPullRequests`
  - `defaultPRSummary`, `defaultPRTitle`, `defaultPRBody`
  - `parsePRCreateOutput`
- Review PR service tests in `internal/orchestrator/service_test.go`:
  - `TestOpenPullRequestsDryRunPreviewsCreateCommand`
  - `TestOpenPullRequestsCreatesAndPersistsMetadata`
- Review CLI wiring in `cmd/metawsm/main.go`:
  - `case "pr"`
  - `prCommand`
  - usage line in `printUsage`
- Review CLI tests in `cmd/metawsm/main_test.go`:
  - `TestPRCommandRequiresRunSelector`
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Technical details
- PR creation executes `gh pr create` from the target repo directory and parses the first `.../pull/<number>` URL token from command output.
- Dry-run mode returns one fully-rendered shell preview action per target repo without mutating store rows.
- Real mode updates `run_pull_requests` with `pr_url`, `pr_number`, `pr_state=open`, `actor`, `credential_mode`, and emits `pr_created` run events.

## Step 7: Integrate Commit/PR Readiness Signals into Operator Loop

This step connected the operator supervision loop to the commit/PR workflow so completed runs can surface explicit readiness signals instead of stopping at merge-only guidance. The purpose was to make post-run Git automation actionable from the same operator loop that already handles stale and unhealthy conditions.

I added readiness detection from status output (`Diffs` and `Pull Requests`), introduced `commit_ready` and `pr_ready` operator intents, and wired auto-mode execution to call commit/PR primitives while keeping assist mode recommendation-only.

### Prompt Context

**User prompt (verbatim):** "use docmgr to pick up ticket METAWSM-009 --- look at the plan, diary, and tasks, and then continue implementing. Keep a diary and commit as you go."

**Assistant interpretation:** Continue implementing the next unchecked METAWSM-009 backlog item from the existing ticket docs, then record and commit progress incrementally.

**Inferred user intent:** Keep momentum on real implementation while preserving ticket traceability and small, reviewable commits.

**Commit (code):** b3587e3 — "Add operator commit/pr readiness signals"

### What I did
- Extended `watchSnapshot` parsing in `cmd/metawsm/main.go` to detect:
  - dirty repo diffs from the `Diffs:` status section,
  - draft/open PR counts from the `Pull Requests:` status section.
- Added `operatorIntentCommitReady` and `operatorIntentPRReady` in `cmd/metawsm/operator_llm.go` and expanded the LLM intent allowlist/prompt accordingly.
- Updated operator rule evaluation (`buildOperatorRuleDecision`) to emit readiness intents when:
  - run is `completed`,
  - `git_pr.mode` is not `off`,
  - commit-ready (`dirty diffs`) or pr-ready (`draft PR records`) conditions are met.
- Added per-decision `Execute` control on rule decisions so commit/PR readiness auto-executes only when `git_pr.mode=auto` and remains recommendation-only in assist mode.
- Wired `executeOperatorAction` for new intents:
  - `commit_ready` -> `service.Commit(...)`
  - `pr_ready` -> `service.OpenPullRequests(...)`
- Added operator event names and direction hints for `commit_ready` and `pr_ready`.
- Added/updated tests in:
  - `cmd/metawsm/main_test.go`
  - `cmd/metawsm/operator_llm_test.go`
- Ran focused validation:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Why
- Task 7 required integrating commit/PR readiness signals into the operator loop.
- Existing operator behavior handled health/staleness but not completed-run handoff into commit/PR workflow.

### What worked
- `cmd/metawsm` tests passed with new parsing, rule-decision, and intent-handling behavior.
- Assist mode now surfaces commit/pr readiness without executing side effects.
- Auto mode can execute commit/pr actions through existing orchestrator primitives.

### What didn't work
- N/A in this step; implementation and focused tests passed on the first validation pass.

### What I learned
- Using `status` output as the operator signal source made readiness integration straightforward without adding new database query paths.
- A per-rule `Execute` flag is cleaner than intent-based execution hardcoding once some intents are recommendation-only in one policy mode and executable in another.

### What was tricky to build
- Ensuring readiness intents do not override existing higher-priority operator concerns (guidance needed, stale-stop, unhealthy restart) while still triggering reliably on completed runs.

### What warrants a second pair of eyes
- Whether parsing status text is robust enough long term, or if operator readiness should move to structured service APIs for lower coupling.
- Whether auto-execution should include additional protections for multi-ticket runs before per-ticket fanout is implemented.

### What should be done in the future
- Add commit/preflight rejection tests (task 8) for explicit policy/validation failure cases.
- Implement validation framework tasks (14-16) so auto commit/pr executes only after required checks pass.

### Code review instructions
- Start in `cmd/metawsm/main.go`:
  - `watchSnapshot` additions
  - `parseWatchSnapshot`
  - `buildOperatorRuleDecision`
  - `operatorEventMessage`
  - `executeOperatorAction`
  - `buildWatchDirectionHints`
- Then review `cmd/metawsm/operator_llm.go`:
  - new intents
  - `operatorRuleDecision.Execute`
  - `mergeOperatorDecisions`
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Technical details
- New parsed readiness fields:
  - `HasDirtyDiffs`
  - `DraftPullRequests`
  - `OpenPullRequests`
- New operator events:
  - `commit_ready`
  - `pr_ready`
- Auto-execution policy:
  - `git_pr.mode=assist` -> readiness alerts only
  - `git_pr.mode=auto` -> readiness alerts + action execution

## Step 8: Add Commit/PR Preflight Rejection Coverage

This step focused on negative-path coverage for commit and PR workflows so policy and state guardrails are explicitly tested. The goal was to close the task for commit/preflight rejection tests and make regression risk lower as validation and operator auto-mode behavior continue to evolve.

I added new orchestrator service tests that assert commit/PR calls fail fast for non-completed runs, `git_pr.mode=off`, and PR creation without prepared commit metadata.

### Prompt Context

**User prompt (verbatim):** "use docmgr to pick up ticket METAWSM-009 --- look at the plan, diary, and tasks, and then continue implementing. Keep a diary and commit as you go."

**Assistant interpretation:** Continue implementing the next open task from METAWSM-009 and maintain incremental commit + diary updates.

**Inferred user intent:** Increase implementation completeness with reliable safeguards, not just happy-path functionality.

**Commit (code):** 299a096 — "Add commit and PR preflight rejection tests"

### What I did
- Added four new tests in `internal/orchestrator/service_test.go`:
  - `TestCommitRejectsWhenRunNotComplete`
  - `TestCommitRejectsWhenGitPRModeOff`
  - `TestOpenPullRequestsRejectsWithoutPreparedCommitMetadata`
  - `TestOpenPullRequestsRejectsWhenGitPRModeOff`
- Used lightweight run fixtures and direct store run creation where needed to set policy mode without adding production code changes.
- Ran focused validation:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Why
- Task 8 explicitly required coverage for commit/preflight rejection behavior.
- These failures are policy-critical and should remain deterministic as more automation gets added.

### What worked
- New tests passed and validated expected rejection messages for all targeted preflight conditions.
- Existing orchestrator and CLI package tests remained green.

### What didn't work
- N/A in this step; tests passed after initial implementation.

### What I learned
- The service methods already had clear preflight failure boundaries; the main missing piece was explicit regression coverage.

### What was tricky to build
- Setting up policy-mode-off cases required direct run fixture creation with custom policy JSON because the shared fixture helper defaults to a minimal policy payload.

### What warrants a second pair of eyes
- Error-string assertions currently use substring checks; reviewers may want to confirm whether these messages should be treated as stable API contracts or loosened further.

### What should be done in the future
- Implement validation framework checks (tasks 14-16) and add corresponding rejection-path tests for required-check failures.

### Code review instructions
- Review `internal/orchestrator/service_test.go`:
  - the four new `Test*Rejects*` cases near commit/PR tests
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Technical details
- Mode-off fixtures are created with run policy JSON:
  - `{"git_pr":{"mode":"off"}}`
- Missing-prepared-metadata PR preflight is asserted through:
  - `OpenPullRequests(...)` returning the `no prepared commit metadata found` error path.

## Step 9: Write Commit/PR Operator Playbook and Proposal A Setup Guidance

This step closed the operator documentation gap by adding a concrete playbook for commit/PR workflows and Proposal A setup. The objective was to make the new automation features usable without reverse-engineering command order from code or changelog entries.

I added a playbook document with prerequisites, command sequences for assist/auto operation, exit criteria, and troubleshooting for common auth/policy/preflight failures.

### Prompt Context

**User prompt (verbatim):** "use docmgr to pick up ticket METAWSM-009 --- look at the plan, diary, and tasks, and then continue implementing. Keep a diary and commit as you go."

**Assistant interpretation:** Continue progressing open METAWSM-009 tasks, including operational docs, while keeping diary/task/changelog state synchronized.

**Inferred user intent:** Ensure implementation is operable by humans and agents, not only technically complete in code.

**Commit (code):** N/A (documentation-only step)

### What I did
- Created playbook document:
  - `ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/playbook/01-operator-and-agent-commit-pr-workflow.md`
- Documented:
  - Proposal A setup (`gh auth login`, `gh auth status`, git identity)
  - Assist-mode commit/PR workflow sequence
  - Auto-mode operator usage and policy toggle (`git_pr.mode=auto`)
  - Exit criteria for successful commit/PR execution
  - Troubleshooting for common failure messages
- Checked tasks:
  - task 10 (`Write operator and agent playbook for commit/PR workflow`)
  - task 19 (`Add playbook section for Proposal A setup ...`)

### Why
- Completing task 7/8 without operator-facing playbook leaves adoption friction and operational ambiguity.
- Proposal A requires exact local setup steps; documenting them prevents repeated auth/identity failures.

### What worked
- New playbook covers both normal command flow and common preflight failures.
- Task backlog now marks documentation deliverables as complete.

### What didn't work
- N/A in this step; no implementation failures occurred.

### What I learned
- The operator loop and direct command surfaces are both needed in docs, because teams may choose gradual adoption from manual to auto mode.

### What was tricky to build
- Balancing concise command flow with enough troubleshooting detail to be immediately actionable for operators.

### What warrants a second pair of eyes
- Whether teams want additional examples for multi-ticket runs once per-ticket fanout task (17) is implemented.

### What should be done in the future
- Update this playbook after validation framework tasks (14-16) land, including examples for required check failures.

### Code review instructions
- Review playbook content and command sequencing in:
  - `ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/playbook/01-operator-and-agent-commit-pr-workflow.md`
- Ensure tasks 10 and 19 are checked in:
  - `ttmp/2026/02/08/METAWSM-009--automate-agent-commit-and-github-pr-creation/tasks.md`

### Technical details
- Documented policy switch for auto readiness execution:
  - `git_pr.mode=auto`
- Documented critical preflight dependency path:
  - `metawsm auth check` must be green before `metawsm commit` / `metawsm pr`.

## Step 10: Implement Validation Framework and Enforce Required Commit/PR Gates

This step implemented the remaining validation architecture tasks so commit/PR automation now enforces policy-configured checks before mutating git state or opening PRs. The focus was to add an extensible check runner with explicit `require_all` semantics, then wire it directly into `Service.Commit` and `Service.OpenPullRequests`.

I added a dedicated validation module with named checks (`tests`, `forbidden_files`, `clean_tree`), extended policy schema with test command and forbidden-pattern configuration, and added regression tests for failure and mixed-pass behavior. This closes tasks 14, 15, and 16.

### Prompt Context

**User prompt (verbatim):** "Ignore them for now, continue with METAWSM-009"

**Assistant interpretation:** Continue implementing the next open METAWSM-009 backlog items while ignoring unrelated untracked ticket directories.

**Inferred user intent:** Finish the remaining high-priority code tasks for commit/PR automation and keep the ticket record updated.

**Commit (code):** d31a862 — "Add git_pr validation framework and required gates"

### What I did
- Added `internal/orchestrator/git_pr_validation.go` with:
  - an extensible check interface,
  - policy-driven required-check runner,
  - `require_all` pass/fail semantics,
  - validation report serialization for persistence.
- Implemented validation checks:
  - `tests`: runs all configured `git_pr.test_commands` in repo context,
  - `forbidden_files`: blocks changed files matching `git_pr.forbidden_file_patterns`,
  - `clean_tree`: requires clean git working tree for PR workflow.
- Wired validation gates into `internal/orchestrator/service.go`:
  - commit path: checks run before branch/commit mutation,
  - PR path: checks run before `gh pr create` execution,
  - persisted validation report JSON into `run_pull_requests.validation_json`.
- Extended policy contract in `internal/policy/policy.go`:
  - added `git_pr.test_commands` and `git_pr.forbidden_file_patterns`,
  - expanded default `required_checks` to `tests`, `forbidden_files`, `clean_tree`,
  - validated supported check names and non-empty command/pattern entries.
- Updated policy tests and orchestrator tests to cover:
  - unsupported/invalid policy values,
  - failing test command rejections for commit and PR,
  - forbidden-file rejection,
  - `require_all=false` allowing mixed pass/fail outcomes,
  - clean-tree PR rejection.
- Updated `examples/policy.example.json` and `README.md` policy field docs for the new validation settings.
- Checked ticket tasks 14, 15, and 16 complete via `docmgr task check --ticket METAWSM-009 --id 14,15,16`.

### Why
- Tasks 14-16 required a reusable validation framework and actual enforcement of required checks before commit/PR operations.
- Without this gate, operator auto mode could perform commit/PR actions on invalid repos or unverified test states.

### What worked
- Focused tests passed:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`
- Commit/PR service methods now fail with clear validation errors when required checks do not pass.
- Validation outcomes are persisted per repo in run PR metadata for traceability.

### What didn't work
- Early exploration commands failed because I accidentally ran them from the parent workspace instead of `metawsm/`:
  - Command: `rg -n "type CommitOptions|func \(s \*Service\) Commit|OpenPullRequests|required_checks|require_all|validation|forbidden|dirty" internal/orchestrator/service.go internal/policy/policy.go internal/orchestrator/service_test.go internal/policy/policy_test.go`
  - Error: `rg: internal/orchestrator/service.go: No such file or directory (os error 2)`
- Similar path errors repeated once while scanning docs/files from the wrong directory.
- Fix: reran all commands with `workdir=/Users/kball/workspaces/2026-02-07/metawsm/metawsm`.

### What I learned
- A separate validation module keeps commit/PR orchestration code significantly clearer than embedding per-check logic in `service.go`.
- `require_all=false` is easiest to reason about when implemented as “at least one applicable required check passed.”

### What was tricky to build
- Adding meaningful defaults (`forbidden_files`, `clean_tree`) without breaking existing commit tests that intentionally operate on dirty repos before committing.
- Ensuring PR validation remains deterministic in tests that use fixture metadata and dry-run paths.

### What warrants a second pair of eyes
- The exact default forbidden-file pattern set may need tuning to reduce false positives in some repos.
- Whether `tests` should remain a pass when `git_pr.test_commands` is empty, or be treated as a policy error in stricter environments.

### What should be done in the future
- Implement task 17 (per repo/ticket branch+PR fanout orchestration for multi-repo runs).
- Implement task 18 (enforce human-only merge policy in operator and CLI surfaces).
- Implement task 20 (end-to-end local-auth commit+push+PR success test).

### Code review instructions
- Start with validation framework:
  - `internal/orchestrator/git_pr_validation.go`
- Then review service integration points:
  - `internal/orchestrator/service.go` (`Commit`, `OpenPullRequests` validation gate calls)
- Review policy schema and validation:
  - `internal/policy/policy.go`
  - `examples/policy.example.json`
  - `README.md`
- Review regression coverage:
  - `internal/orchestrator/service_test.go`
  - `internal/policy/policy_test.go`
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Technical details
- New policy keys:
  - `git_pr.required_checks` (supports `tests`, `forbidden_files`, `clean_tree`)
  - `git_pr.test_commands` (shell commands run in repo root)
  - `git_pr.forbidden_file_patterns` (glob patterns matched against changed files)
- Validation report persistence:
  - serialized check results stored in `run_pull_requests.validation_json`.
- Gate semantics:
  - `require_all=true`: all applicable required checks must pass,
  - `require_all=false`: at least one applicable required check must pass.

## Step 11: Enforce Human-Only Merge Execution in CLI Surfaces

This step tightened merge behavior to require explicit human acknowledgement when executing `metawsm merge` outside dry-run mode. The objective was to enforce the no-auto-merge policy at the command surface, not only through operator intent rules.

I added a required `--human` acknowledgement for non-dry-run merge execution, updated direction hints and usage output, and added CLI tests that lock this behavior in place.

### Prompt Context

**User prompt (verbatim):** "Ignore them for now, continue with METAWSM-009"

**Assistant interpretation:** Continue implementing remaining METAWSM-009 tasks after finishing validation work.

**Inferred user intent:** Keep advancing ticket completion with incremental, reviewable changes and strict safety constraints.

**Commit (code):** 4dca4ec — "Require human acknowledgement for merge execution"

### What I did
- Updated `cmd/metawsm/main.go` `mergeCommand` to require `--human` when not using `--dry-run`:
  - returns `merge requires --human acknowledgement; automated merge is disabled` otherwise.
- Updated operator direction hints (`run_done`) to include explicit human merge command:
  - `metawsm merge --run-id <id> --human`.
- Updated CLI usage text for merge:
  - `metawsm merge [--run-id RUN_ID | --ticket T1] [--dry-run] [--human]`.
- Added tests in `cmd/metawsm/main_test.go`:
  - `TestMergeCommandRequiresHumanAcknowledgement`
  - `TestBuildWatchDirectionHintsRunDoneIncludesHumanMerge`
- Ran focused validation:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`
- Checked task 18 complete:
  - `docmgr task check --ticket METAWSM-009 --id 18`

### Why
- Task 18 required explicit human-only merge policy enforcement in operator/CLI surfaces.
- The operator loop already avoids merge execution; this CLI acknowledgement closes the remaining accidental automation path.

### What worked
- Command-level guard works and is covered by tests.
- Existing package tests remained green.
- Merge hints now clearly direct users to human-acknowledged execution.

### What didn't work
- N/A in this step; implementation and tests passed on first pass.

### What I learned
- A small acknowledgement flag is an effective safety rail when a command should remain human-operated but still script-friendly for explicit usage.

### What was tricky to build
- Ensuring UX remains straightforward while making the policy constraint explicit in command output and hinting.

### What warrants a second pair of eyes
- Whether `--human` should also be required in additional surfaces (for example future TUI merge actions) if those paths are added.

### What should be done in the future
- Implement task 17 (per repo/ticket branch+PR fanout orchestration for multi-repo runs).
- Implement task 20 (end-to-end successful local-auth commit push + PR creation test).

### Code review instructions
- Review merge guard and user-facing text updates in:
  - `cmd/metawsm/main.go`
- Review test coverage in:
  - `cmd/metawsm/main_test.go`
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Technical details
- Non-dry-run merge now requires explicit CLI acknowledgement:
  - `metawsm merge --run-id <id> --human`
- Dry-run remains available without acknowledgement for planning/review.

## Step 12: Add Multi-Ticket Commit/PR Fanout by Workspace-Ticket Mapping

This step implemented per repo/ticket fanout behavior for multi-ticket runs so commit and PR workflows no longer require manual ticket selection when a run includes multiple ticket workspaces. The service now derives workspace-to-ticket ownership from persisted run steps and applies commit/PR actions with ticket-correct branch naming, metadata keys, and event messages.

I removed the prior single-ticket-only constraint in multi-ticket runs, added fanout-aware filtering for optional `--ticket`, and added tests that validate commit and PR dry-run fanout across two tickets/workspaces.

### Prompt Context

**User prompt (verbatim):** "Ignore them for now, continue with METAWSM-009"

**Assistant interpretation:** Continue implementing remaining ticket items after finishing validation and merge-policy work.

**Inferred user intent:** Close out open implementation tasks with concrete code and tests while keeping diary/task/changelog synchronized.

**Commit (code):** 6ec9185 — "Fan out commit and PR workflows across multi-ticket runs"

### What I did
- Updated `Service.Commit` in `internal/orchestrator/service.go` to:
  - support multi-ticket runs without requiring `--ticket`,
  - resolve workspace-ticket mapping from persisted run steps,
  - fan out commit operations per workspace ticket,
  - render branches and persist PR rows keyed by each workspace ticket.
- Updated `Service.OpenPullRequests` to:
  - support multi-ticket fanout when `--ticket` is omitted,
  - select candidates across all run tickets,
  - preserve correct per-row ticket values in title/body defaults, validation inputs, and event metadata.
- Added `resolveWorkspaceTickets` helper in service layer.
- Added orchestrator tests in `internal/orchestrator/service_test.go`:
  - `TestCommitFansOutAcrossWorkspaceTicketsWhenRunHasMultipleTickets`
  - `TestOpenPullRequestsFansOutAcrossTicketsWhenRunHasMultipleTickets`
- Added `createRunWithTicketsFixture` test helper for multi-ticket run fixtures with workspace-ticket step metadata.
- Ran focused validation:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`
- Checked task 17 complete:
  - `docmgr task check --ticket METAWSM-009 --id 17`

### Why
- Task 17 required per repo/ticket fanout orchestration for multi-repo/multi-ticket runs.
- Previous behavior failed for multi-ticket runs unless operators manually selected one ticket per call, and it incorrectly reused a single ticket across all workspace commit metadata.

### What worked
- Multi-ticket commit and PR fanout now works in service tests with per-workspace ticket attribution.
- Existing cmd and policy package tests remained green.

### What didn't work
- N/A in this step; implementation and test runs passed without iterative failures.

### What I learned
- Step metadata is the cleanest source of workspace-ticket ownership once runs are underway.
- Fanout support mostly required changing ticket selection and row filtering logic while leaving git/gh execution paths intact.

### What was tricky to build
- Keeping backward-compatible behavior for single-ticket runs while enabling multi-ticket fanout and preserving optional `--ticket` filtering.

### What warrants a second pair of eyes
- Whether ambiguous/missing workspace-ticket step mappings should fail hard (current behavior) or fall back more permissively in some run modes.

### What should be done in the future
- Implement task 20 (end-to-end successful local-auth commit push and PR creation test).

### Code review instructions
- Start in `internal/orchestrator/service.go`:
  - `Service.Commit`
  - `Service.OpenPullRequests`
  - `resolveWorkspaceTickets`
- Review test coverage in `internal/orchestrator/service_test.go`:
  - new multi-ticket fanout tests and fixture helper
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`

### Technical details
- Commit fanout now computes ticket per workspace and persists rows using `ticket|repo` keys derived from that mapping.
- PR fanout now selects candidates across selected tickets (all run tickets by default) and builds per-ticket PR defaults/events.

## Step 13: Push Branches Before PR Creation and Add End-to-End Local-Auth Workflow Test

This step completed the final open task by making PR creation execute a real branch push to `origin` before calling `gh pr create`, then adding an end-to-end service test that exercises commit -> push -> PR metadata persistence. The primary goal was to validate a realistic local-auth workflow instead of only unit-level fake PR creation.

I updated PR dry-run previews to include both push and PR commands, wired real execution to run `git push --set-upstream origin <head-branch>` first, and added an end-to-end orchestrator test with a local bare origin and fake `gh` binary.

### Prompt Context

**User prompt (verbatim):** "Ignore them for now, continue with METAWSM-009"

**Assistant interpretation:** Continue implementing remaining open ticket work until completion, with diary and incremental commits.

**Inferred user intent:** Finish METAWSM-009 end-to-end and leave no implementation task open.

**Commit (code):** 627e397 — "Push branches before PR creation and add end-to-end test"

### What I did
- Updated `internal/orchestrator/service.go` PR workflow to:
  - include push preview action in dry-run output,
  - execute `git push --set-upstream origin <head-branch>` before `gh pr create` in real mode.
- Updated existing PR dry-run test to expect both actions (push + PR create).
- Updated existing PR create/persist test to include:
  - local bare `origin` remote setup,
  - real head-branch push verification.
- Added new end-to-end test in `internal/orchestrator/service_test.go`:
  - `TestCommitAndOpenPullRequestsEndToEndPushesBranchAndPersistsMetadata`
  - uses real `Service.Commit` + `Service.OpenPullRequests`, local bare remote, and fake `gh` output.
- Ran focused validation:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./cmd/metawsm -count=1`
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/policy -count=1`
- Checked task 20 complete:
  - `docmgr task check --ticket METAWSM-009 --id 20`

### Why
- Task 20 required an end-to-end successful local-auth commit+push+PR creation test.
- PR creation without pushing the head branch is incomplete in realistic workflows.

### What worked
- Service now pushes before PR creation and tests validate branch presence on origin.
- End-to-end test passes with deterministic local fixtures and fake `gh` output.
- All focused package tests remained green.

### What didn't work
- N/A in this step; implementation and test runs passed without retries.

### What I learned
- Existing commit/PR service boundaries were sufficient for end-to-end orchestration once push was inserted before `gh pr create`.
- A local bare remote fixture provides reliable push validation without external network dependencies.

### What was tricky to build
- Updating earlier PR tests to represent realistic branch state and remote setup after introducing mandatory push-before-PR behavior.

### What warrants a second pair of eyes
- Whether push behavior should support non-`origin` remotes (for fork-based workflows) in a future credential-mode expansion.

### What should be done in the future
- N/A

### Code review instructions
- Start in `internal/orchestrator/service.go`:
  - PR push preview and pre-PR push execution
- Review test updates in `internal/orchestrator/service_test.go`:
  - dry-run push preview expectation
  - PR metadata/push verification test
  - end-to-end commit->push->PR test
- Validate with:
  - `GOCACHE=/tmp/metawsm-gocache GOMODCACHE=/tmp/metawsm-gomodcache go test ./internal/orchestrator -count=1`

### Technical details
- PR execution order is now:
  1. `git push --set-upstream origin <head-branch>`
  2. `gh pr create --base ... --head ... --title ... --body ...`
- Dry-run actions now include both commands in order.

## Step 14: Reopen METAWSM-009 for Native Workaround Handling and Break Phase 2 into Executable Tasks

This step reopens active implementation on top of the completed V1 flow. The focus is to remove real-world operator workarounds discovered during end-to-end execution (manual stash/rebase before commit, transient SQLite lock failures, and actor attribution gaps).

I added a dedicated Phase 2 design plan and replaced the coarse backlog items with atomic tasks that map directly to code slices and tests.

### Prompt Context

**User prompt (verbatim):** "Reopen METAWSM-009 -- add a new plan with the list of updates we'd need to make to natively handle the things you had to work around." and "Break those into tasks, then implement them, keeping a diary and committing as you go"

**Assistant interpretation:** Reopen the ticket with a concrete hardening plan, decompose work into executable tasks, then implement incrementally with diary + commits.

**Inferred user intent:** Ship Phase 2 reliability improvements now with clear tracking and reviewable incremental history.

### What I did
- Added new design doc:
  - `design-doc/02-phase-2-plan-remove-commit-pr-workflow-workarounds.md`
- Expanded Phase 2 backlog from 8 coarse items to 16 implementation-ready tasks in `tasks.md` covering:
  - branch prep robustness,
  - SQLite lock timeout/retry and mutation locking,
  - actor fallback resolution,
  - diagnostics/playbook/e2e coverage.
- Updated changelog with reopen + task decomposition note.

### Why
- The reopened Phase 2 must be tracked at the granularity of code-and-test slices to support commit-as-you-go execution and unambiguous review.

### What worked
- Ticket now shows open Phase 2 tasks with concrete implementation boundaries.

### What didn't work
- Initial `docmgr task add` sequence mistakenly used `--workdir` as a `docmgr` flag and failed.
- Fix: reran with shell working directory set via tool invocation, then tasks were added successfully.

### What should be done next
- Implement and commit each Phase 2 workstream in order:
  1. branch-prep native handling and regression tests,
  2. SQLite lock resilience + mutation lock semantics,
  3. actor fallback and diagnostics improvements,
  4. playbook/e2e updates.

## Step 15: Add SQLite Busy Timeout + Retry Backoff in Store Layer

This step implements the first hardening slice from Phase 2B by making store operations resilient to transient SQLite lock contention.

I added native busy-timeout behavior to all sqlite3 invocations, bounded retry/backoff for busy/locked failures, and a lock-contention test that proves retries happen and eventually succeed once the write lock is released.

### Prompt Context

**User prompt (verbatim):** "Break those into tasks, then implement them, keeping a diary and committing as you go"

**Assistant interpretation:** Implement Phase 2 tasks incrementally with tests, diary updates, and commits after each coherent slice.

**Inferred user intent:** Convert the lock-related workaround into first-class product behavior with regression coverage.

### What I did
- Updated `internal/store/sqlite.go`:
  - Added store-level settings for busy timeout and retry policy:
    - `BusyTimeoutMS`
    - `BusyRetryCount`
    - `BusyRetryBackoffMS`
  - Added sqlite CLI argument builder that applies `.timeout` across query and exec calls.
  - Refactored query/exec into single-attempt helpers plus retry wrappers.
  - Added lock/busy error detection helper (`isSQLiteBusyError`).
  - Added optional retry observer hook (test-only instrumentation in same package).
- Added `TestSQLiteStoreRetriesBusyWriteLock` in `internal/store/sqlite_test.go`:
  - holds a write lock with a long-lived sqlite3 session (`BEGIN IMMEDIATE`),
  - attempts a competing write through store API,
  - releases the lock and verifies write success and retry count.
- Ran validation:
  - `go test ./internal/store -count=1`
  - `go test ./internal/orchestrator -count=1`

### Why
- Real `database is locked` failures surfaced in CLI flows and required manual retries.
- Retry/backoff in the store layer is the narrowest point to harden all command paths consistently.

### What worked
- Store tests pass with lock contention simulation.
- Orchestrator tests remained green with no behavior regressions.

### What didn't work
- Initial sandboxed test execution could not access Go build cache (`operation not permitted`).
- Fix: reran tests with escalated permissions.

### What should be done next
- Implement run-level commit/pr mutation locking and typed in-progress errors (Phase 2B.4/2B.5).

## Step 16: Add Run-Level Commit/PR Mutation Locking

This step adds run-scoped mutation locking for non-dry-run `commit` and `pr` operations so concurrent invocations fail fast with a typed error instead of racing into inconsistent state updates.

### What I did
- Added `RunMutationInProgressError` in `internal/orchestrator/service.go` with lock path and holder metadata.
- Added lock helpers:
  - `acquireRunMutationLock`
  - `runMutationLockPath`
  - `sanitizeLockToken`
- Integrated locking in service flow:
  - `Service.Commit` now acquires lock for non-dry-run operations.
  - `Service.OpenPullRequests` now acquires lock for non-dry-run operations.
- Added orchestrator regression tests in `internal/orchestrator/service_test.go`:
  - `TestCommitRejectsWhenRunMutationLockExists`
  - `TestOpenPullRequestsRejectsWhenRunMutationLockExists`
- Ran validation:
  - `go test ./internal/orchestrator -count=1`

### Why
- Concurrent commit/pr command execution on the same run should produce an explicit "operation in progress" signal rather than lock-race behavior.

### What worked
- Both service paths now return typed lock errors when a run mutation lock exists.
- Existing orchestrator tests remained green.

### What didn't work
- N/A in this slice; no implementation rollback was needed.

### What should be done next
- Implement native branch-prep handling for stale-base dirty trees and add regression tests (Phase 2A.*).
